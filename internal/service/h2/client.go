package h2

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/aren55555/signal-cli-go/internal/service/tunnel"
)

// Errors.
var (
	// ErrProtocol reports a peer that broke RFC 9113's framing rules.
	ErrProtocol = errors.New("h2: protocol error")
	// ErrExtendedConnectUnsupported reports a server that did not offer
	// SETTINGS_ENABLE_CONNECT_PROTOCOL (RFC 8441 §3).
	ErrExtendedConnectUnsupported = errors.New("h2: server does not support extended CONNECT")
	// ErrClosed reports a closed connection or stream.
	ErrClosed = errors.New("h2: closed")
)

// StatusError reports a non-2xx response to an extended CONNECT, with the
// response's headers (pseudo-headers removed).
type StatusError struct {
	Status int
	Header []HeaderField
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("h2: CONNECT answered with status %d", e.Status)
}

// Frame types and flags (RFC 9113 §6).
const (
	frameData         = 0x0
	frameHeaders      = 0x1
	frameRSTStream    = 0x3
	frameSettings     = 0x4
	framePushPromise  = 0x5
	framePing         = 0x6
	frameGoAway       = 0x7
	frameWindowUpdate = 0x8
	frameContinuation = 0x9

	flagEndStream  = 0x1
	flagAck        = 0x1
	flagEndHeaders = 0x4
	flagPadded     = 0x8
	flagPriority   = 0x20
)

// Settings (RFC 9113 §6.5.2, RFC 8441 §3).
const (
	settingHeaderTableSize       = 0x1
	settingEnablePush            = 0x2
	settingInitialWindowSize     = 0x4
	settingMaxFrameSize          = 0x5
	settingEnableConnectProtocol = 0x8
)

const (
	clientPreface     = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	defaultWindow     = 65535
	defaultFrameSize  = 16384
	ourStreamWindow   = 1 << 20 // advertised INITIAL_WINDOW_SIZE
	maxAcceptedFrame  = defaultFrameSize
	errCodeCancel     = 0x8
	errCodeProtocol   = 0x1
	maxHeaderBlockLen = 256 << 10
)

type frame struct {
	typ, flags byte
	stream     uint32
	payload    []byte
}

// readFrame reads one frame, refusing payloads over maxSize.
func readFrame(r *bufio.Reader, maxSize uint32) (frame, error) {
	var h [9]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return frame{}, err
	}
	n := uint32(h[0])<<16 | uint32(h[1])<<8 | uint32(h[2])
	if n > maxSize {
		return frame{}, fmt.Errorf("%w: %d-byte frame over the %d-byte limit", ErrProtocol, n, maxSize)
	}
	f := frame{typ: h[3], flags: h[4], stream: binary.BigEndian.Uint32(h[5:]) & 0x7fffffff, payload: make([]byte, n)}
	if _, err := io.ReadFull(r, f.payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return frame{}, err
	}
	return f, nil
}

func appendFrame(dst []byte, typ, flags byte, stream uint32, payload []byte) []byte {
	n := len(payload)
	dst = append(dst, byte(n>>16), byte(n>>8), byte(n), typ, flags)
	dst = binary.BigEndian.AppendUint32(dst, stream&0x7fffffff)
	return append(dst, payload...)
}

// stripPadding removes the pad length byte and padding of a PADDED frame.
func stripPadding(f frame) ([]byte, int, error) {
	p := f.payload
	if f.flags&flagPadded == 0 {
		return p, 0, nil
	}
	if len(p) == 0 || int(p[0]) >= len(p) {
		return nil, 0, fmt.Errorf("%w: bad padding", ErrProtocol)
	}
	pad := int(p[0])
	return p[1 : len(p)-pad], pad + 1, nil
}

// Options configure Dial.
type Options struct {
	TLSConfig *tls.Config // RootCAs and the like; ServerName, ALPN and TLS 1.3 are set here
	Proxy     *url.URL    // HTTP CONNECT proxy; nil dials directly
}

// Conn is a client HTTP/2 connection supporting extended CONNECT streams.
type Conn struct {
	nc        net.Conn
	authority string

	wmu sync.Mutex // serializes frame writes
	bw  *bufio.Writer
	enc *Encoder

	mu            sync.Mutex
	cond          *sync.Cond
	err           error // set once the connection fails or closes
	settingsSeen  bool
	enableConnect bool
	peerFrameSize uint32
	peerWindow    int64 // INITIAL_WINDOW_SIZE for new streams
	connWindow    int64 // our send window on the connection
	nextStream    uint32
	streams       map[uint32]*Stream
}

// Dial opens an HTTP/2 connection to addr ("host:port") over TLS 1.3 with
// ALPN h2.
func Dial(ctx context.Context, addr string, opts Options) (*Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	raw, err := tunnel.Dial(ctx, addr, opts.Proxy)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{}
	if opts.TLSConfig != nil {
		cfg = opts.TLSConfig.Clone()
	}
	cfg.ServerName = host
	cfg.MinVersion = tls.VersionTLS13
	cfg.NextProtos = []string{"h2"}
	tc := tls.Client(raw, cfg)
	if err := tc.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("h2: TLS: %w", err)
	}
	if p := tc.ConnectionState().NegotiatedProtocol; p != "h2" {
		tc.Close()
		return nil, fmt.Errorf("h2: server negotiated %q, not h2", p)
	}
	authority := addr
	if port == "443" {
		authority = host
	}
	return newConn(tc, authority)
}

// newConn starts HTTP/2 on an established transport: it sends the preface
// and our SETTINGS and starts the read loop.
func newConn(nc net.Conn, authority string) (*Conn, error) {
	c := &Conn{
		nc: nc, authority: authority, bw: bufio.NewWriter(nc), enc: NewEncoder(0, true),
		peerFrameSize: defaultFrameSize, peerWindow: defaultWindow, connWindow: defaultWindow,
		nextStream: 1, streams: map[uint32]*Stream{},
	}
	c.cond = sync.NewCond(&c.mu)
	var settings []byte
	for _, s := range [][2]uint32{{settingEnablePush, 0}, {settingInitialWindowSize, ourStreamWindow}} {
		settings = binary.BigEndian.AppendUint16(settings, uint16(s[0]))
		settings = binary.BigEndian.AppendUint32(settings, s[1])
	}
	out := append([]byte(clientPreface), appendFrame(nil, frameSettings, 0, 0, settings)...)
	if err := c.write(out); err != nil {
		nc.Close()
		return nil, err
	}
	go c.readLoop(bufio.NewReader(nc))
	return c, nil
}

func (c *Conn) write(b []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.bw.Write(b); err != nil {
		return err
	}
	return c.bw.Flush()
}

func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.cond.Broadcast()
	c.mu.Unlock()
	c.nc.Close()
}

// Close closes the connection and all its streams.
func (c *Conn) Close() error {
	c.fail(ErrClosed)
	return nil
}

func (c *Conn) readLoop(r *bufio.Reader) {
	dec := NewDecoder(4096)
	var block []byte
	var blockStream uint32
	var blockEnd bool
	for {
		f, err := readFrame(r, maxAcceptedFrame)
		if err != nil {
			c.fail(err)
			return
		}
		if block != nil && f.typ != frameContinuation {
			c.fail(fmt.Errorf("%w: expected CONTINUATION", ErrProtocol))
			return
		}
		switch f.typ {
		case frameSettings:
			err = c.handleSettings(f)
		case framePing:
			if f.flags&flagAck == 0 && len(f.payload) == 8 {
				err = c.write(appendFrame(nil, framePing, flagAck, 0, f.payload))
			}
		case frameWindowUpdate:
			err = c.handleWindowUpdate(f)
		case frameHeaders:
			var p []byte
			if p, _, err = stripPadding(f); err == nil {
				if f.flags&flagPriority != 0 {
					if len(p) < 5 {
						err = fmt.Errorf("%w: short HEADERS", ErrProtocol)
						break
					}
					p = p[5:]
				}
				block, blockStream, blockEnd = append([]byte{}, p...), f.stream, f.flags&flagEndStream != 0
				if f.flags&flagEndHeaders != 0 {
					err = c.deliverHeaders(dec, blockStream, block, blockEnd)
					block = nil
				}
			}
		case frameContinuation:
			if block == nil || f.stream != blockStream {
				err = fmt.Errorf("%w: unexpected CONTINUATION", ErrProtocol)
				break
			}
			if block = append(block, f.payload...); len(block) > maxHeaderBlockLen {
				err = fmt.Errorf("%w: header block too large", ErrProtocol)
				break
			}
			if f.flags&flagEndHeaders != 0 {
				err = c.deliverHeaders(dec, blockStream, block, blockEnd)
				block = nil
			}
		case frameData:
			err = c.handleData(f)
		case frameRSTStream:
			c.endStream(f.stream, fmt.Errorf("%w: stream reset by peer", ErrClosed))
		case frameGoAway:
			c.fail(fmt.Errorf("%w: GOAWAY from peer", ErrClosed))
			return
		case framePushPromise:
			err = fmt.Errorf("%w: PUSH_PROMISE with push disabled", ErrProtocol)
		}
		if err != nil {
			c.fail(err)
			return
		}
	}
}

func (c *Conn) handleSettings(f frame) error {
	if f.stream != 0 || len(f.payload)%6 != 0 {
		return fmt.Errorf("%w: bad SETTINGS", ErrProtocol)
	}
	if f.flags&flagAck != 0 {
		return nil
	}
	c.mu.Lock()
	for p := f.payload; len(p) > 0; p = p[6:] {
		id, v := binary.BigEndian.Uint16(p), binary.BigEndian.Uint32(p[2:])
		switch id {
		case settingInitialWindowSize:
			if v > 1<<31-1 {
				c.mu.Unlock()
				return fmt.Errorf("%w: window size %d", ErrProtocol, v)
			}
			delta := int64(v) - c.peerWindow
			c.peerWindow = int64(v)
			for _, s := range c.streams {
				s.sendWindow += delta
			}
		case settingMaxFrameSize:
			if v < defaultFrameSize || v > 1<<24-1 {
				c.mu.Unlock()
				return fmt.Errorf("%w: max frame size %d", ErrProtocol, v)
			}
			c.peerFrameSize = v
		case settingEnableConnectProtocol:
			c.enableConnect = v == 1
		}
	}
	c.settingsSeen = true
	c.cond.Broadcast()
	c.mu.Unlock()
	return c.write(appendFrame(nil, frameSettings, flagAck, 0, nil))
}

func (c *Conn) handleWindowUpdate(f frame) error {
	if len(f.payload) != 4 {
		return fmt.Errorf("%w: bad WINDOW_UPDATE", ErrProtocol)
	}
	inc := int64(binary.BigEndian.Uint32(f.payload) & 0x7fffffff)
	c.mu.Lock()
	defer c.mu.Unlock()
	if f.stream == 0 {
		c.connWindow += inc
	} else if s := c.streams[f.stream]; s != nil {
		s.sendWindow += inc
	}
	c.cond.Broadcast()
	return nil
}

func (c *Conn) deliverHeaders(dec *Decoder, stream uint32, block []byte, endStream bool) error {
	fields, err := dec.Decode(block) // always decode, to keep HPACK state in step
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.streams[stream]
	if s == nil {
		return nil
	}
	if s.status == 0 {
		for _, h := range fields {
			if h.Name == ":status" {
				fmt.Sscanf(h.Value, "%d", &s.status)
			} else if !strings.HasPrefix(h.Name, ":") {
				s.header = append(s.header, h)
			}
		}
		if s.status == 0 {
			s.status = -1 // malformed response
		}
	}
	if endStream {
		s.remoteDone = true
	}
	c.cond.Broadcast()
	return nil
}

func (c *Conn) handleData(f frame) error {
	data, padding, err := stripPadding(f)
	if err != nil {
		return err
	}
	c.mu.Lock()
	s := c.streams[f.stream]
	if s != nil {
		s.buf = append(s.buf, data...)
		if f.flags&flagEndStream != 0 {
			s.remoteDone = true
		}
		c.cond.Broadcast()
	}
	c.mu.Unlock()
	// Credit padding (and data for unknown streams) back at once; data for a
	// live stream is credited as the application reads it.
	credit := padding
	if s == nil {
		credit += len(data)
	}
	if credit > 0 {
		return c.windowUpdate(0, credit)
	}
	return nil
}

func (c *Conn) windowUpdate(stream uint32, n int) error {
	var b []byte
	b = appendFrame(b, frameWindowUpdate, 0, 0, binary.BigEndian.AppendUint32(nil, uint32(n)))
	if stream != 0 {
		b = appendFrame(b, frameWindowUpdate, 0, stream, binary.BigEndian.AppendUint32(nil, uint32(n)))
	}
	return c.write(b)
}

func (c *Conn) endStream(id uint32, err error) {
	c.mu.Lock()
	if s := c.streams[id]; s != nil && s.err == nil {
		s.err = err
	}
	c.cond.Broadcast()
	c.mu.Unlock()
}

// Stream is an extended-CONNECT stream: a bidirectional byte stream.
type Stream struct {
	c          *Conn
	id         uint32
	status     int
	header     []HeaderField // response headers of the first header block
	sendWindow int64
	buf        []byte
	remoteDone bool
	localDone  bool
	err        error
}

// OpenConnect sends an extended CONNECT (RFC 8441) for protocol and path
// and waits for a 2xx response. Header names must be lowercase (HTTP/2
// requires it).
func (c *Conn) OpenConnect(ctx context.Context, protocol, path string, headers []HeaderField) (*Stream, error) {
	stop := context.AfterFunc(ctx, func() {
		c.mu.Lock()
		c.cond.Broadcast()
		c.mu.Unlock()
	})
	defer stop()

	c.mu.Lock()
	for !c.settingsSeen && c.err == nil && ctx.Err() == nil {
		c.cond.Wait()
	}
	switch {
	case c.err != nil:
		err := c.err
		c.mu.Unlock()
		return nil, err
	case ctx.Err() != nil:
		c.mu.Unlock()
		return nil, ctx.Err()
	case !c.enableConnect:
		c.mu.Unlock()
		return nil, ErrExtendedConnectUnsupported
	}
	s := &Stream{c: c, id: c.nextStream, sendWindow: c.peerWindow}
	c.nextStream += 2
	c.streams[s.id] = s
	frameSize := int(c.peerFrameSize)
	c.mu.Unlock()

	fields := []HeaderField{
		{Name: ":method", Value: "CONNECT"},
		{Name: ":protocol", Value: protocol},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: path},
		{Name: ":authority", Value: c.authority},
	}
	for _, h := range headers {
		h.Name = strings.ToLower(h.Name)
		fields = append(fields, h)
	}
	c.wmu.Lock()
	block := c.enc.Encode(nil, fields)
	var out []byte
	first := true
	for len(block) > 0 || first {
		n := min(len(block), frameSize)
		typ, flags := byte(frameContinuation), byte(0)
		if first {
			typ = frameHeaders
		}
		if n == len(block) {
			flags |= flagEndHeaders
		}
		out = appendFrame(out, typ, flags, s.id, block[:n])
		block, first = block[n:], false
	}
	_, err := c.bw.Write(out)
	if err == nil {
		err = c.bw.Flush()
	}
	c.wmu.Unlock()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	for s.status == 0 && s.err == nil && c.err == nil && ctx.Err() == nil {
		c.cond.Wait()
	}
	status, serr := s.status, s.err
	if serr == nil {
		serr = c.err
	}
	c.mu.Unlock()
	switch {
	case status != 0 && (status < 200 || status > 299):
		// A refusal is often followed at once by RST_STREAM; the status
		// wins over the reset.
		s.reset()
		return nil, &StatusError{Status: status, Header: s.header}
	case serr != nil:
		return nil, serr
	case ctx.Err() != nil:
		s.reset()
		return nil, ctx.Err()
	}
	return s, nil
}

// reset cancels the stream.
func (s *Stream) reset() {
	_ = s.c.write(appendFrame(nil, frameRSTStream, 0, s.id, binary.BigEndian.AppendUint32(nil, errCodeCancel)))
	s.c.endStream(s.id, ErrClosed)
}

// Read reads stream data; it returns io.EOF once the server ends the
// stream.
func (s *Stream) Read(p []byte) (int, error) {
	c := s.c
	c.mu.Lock()
	for len(s.buf) == 0 && !s.remoteDone && s.err == nil && c.err == nil {
		c.cond.Wait()
	}
	if len(s.buf) == 0 {
		err := s.err
		if err == nil && s.remoteDone {
			err = io.EOF
		}
		if err == nil {
			err = c.err
		}
		c.mu.Unlock()
		return 0, err
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	done := s.remoteDone
	c.mu.Unlock()
	if !done {
		if err := c.windowUpdate(s.id, n); err != nil {
			return n, err
		}
	}
	return n, nil
}

// Write sends p as DATA frames, waiting for flow-control window as needed.
func (s *Stream) Write(p []byte) (int, error) {
	c := s.c
	written := 0
	for len(p) > 0 {
		c.mu.Lock()
		for (s.sendWindow <= 0 || c.connWindow <= 0) && s.err == nil && c.err == nil && !s.localDone {
			c.cond.Wait()
		}
		if err := s.err; err != nil || c.err != nil || s.localDone {
			if err == nil {
				err = c.err
			}
			if err == nil {
				err = ErrClosed
			}
			c.mu.Unlock()
			return written, err
		}
		n := min(int64(len(p)), s.sendWindow, c.connWindow, int64(c.peerFrameSize))
		s.sendWindow -= n
		c.connWindow -= n
		c.mu.Unlock()
		if err := c.write(appendFrame(nil, frameData, 0, s.id, p[:n])); err != nil {
			return written, err
		}
		p, written = p[n:], written+int(n)
	}
	return written, nil
}

// Close ends our side of the stream (an empty DATA frame with END_STREAM).
func (s *Stream) Close() error {
	c := s.c
	c.mu.Lock()
	if s.localDone || c.err != nil {
		c.mu.Unlock()
		return nil
	}
	s.localDone = true
	c.cond.Broadcast()
	c.mu.Unlock()
	return c.write(appendFrame(nil, frameData, flagEndStream, s.id, nil))
}

// Header returns the response headers, without pseudo-headers.
func (s *Stream) Header() []HeaderField { return s.header }
