// SPDX-License-Identifier: GPL-3.0-or-later

// Package provision is the secondary side of device linking: the
// provisioning WebSocket and the sgnl://linkdevice URL
// (spec/service/linking.md §2, §5).
package provision

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/environment"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/agent"
	"github.com/aren55555/signal-cli-go/internal/service/trust"
	"github.com/aren55555/signal-cli-go/internal/service/websocket"
	"google.golang.org/protobuf/proto"
)

// Errors.
var (
	ErrNoAddress   = errors.New("provision: no device id received")
	ErrNoMessage   = errors.New("provision: no provisioning message received")
	ErrInvalidLink = errors.New("provision: invalid device link")
)

// StagingHost is the REST host whose provisioning socket is used.
var StagingHost = environment.Staging.ProvisioningHost

// Socket is a message-oriented transport: a WebSocket.
type Socket interface {
	Read() ([]byte, error)
	Write([]byte) error
	Close() error
}

type timing struct{ keepAlive, address, message time.Duration }

// defaultTiming is §2.4: keep-alive every 30 s, the address within 10 s,
// the message within the socket's 90 s lifespan.
var defaultTiming = timing{keepAlive: 30 * time.Second, address: 10 * time.Second, message: 90 * time.Second}

// Endpoint describes the provisioning socket to open.
type Endpoint struct {
	Host              string // default StagingHost
	Proxy             *url.URL
	Roots             *x509.CertPool // default trust.RESTRoots()
	UserAgentOverride string
}

// Session is an open provisioning socket.
type Session struct {
	s      Socket
	opened time.Time
	t      timing
	wmu    sync.Mutex

	addr  chan string
	env   chan *signalpb.ProvisionEnvelope
	done  chan struct{}
	once  sync.Once
	err   error
	gotAd string
}

// Open opens the provisioning socket: a plain TLS WebSocket with the REST
// trust roots and signal-cli's User-Agent, no credentials (§2.1).
func Open(ctx context.Context, ep Endpoint) (*Session, error) {
	host := ep.Host
	if host == "" {
		host = StagingHost
	}
	roots := ep.Roots
	if roots == nil {
		roots = trust.RESTRoots()
	}
	c, err := websocket.Dial(ctx, "wss://"+host+"/v1/websocket/provisioning/", websocket.Options{
		Header:    http.Header{"User-Agent": {agent.String(ep.UserAgentOverride)}},
		Proxy:     ep.Proxy,
		TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	})
	if err != nil {
		return nil, err
	}
	return NewSession(c), nil
}

func newSession(s Socket, t timing) *Session {
	ps := &Session{s: s, opened: time.Now(), t: t, addr: make(chan string, 1),
		env: make(chan *signalpb.ProvisionEnvelope, 1), done: make(chan struct{})}
	go ps.readLoop()
	go ps.keepAlive()
	return ps
}

func (ps *Session) fail(err error) {
	ps.once.Do(func() {
		ps.err = err
		close(ps.done)
		ps.s.Close()
	})
}

// Close closes the socket.
func (ps *Session) Close() error {
	ps.fail(errors.New("provision: closed"))
	return nil
}

func (ps *Session) write(m *signalpb.ChatWebSocketMessage) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	ps.wmu.Lock()
	defer ps.wmu.Unlock()
	return ps.s.Write(b)
}

func (ps *Session) readLoop() {
	for {
		b, err := ps.s.Read()
		if err != nil {
			ps.fail(err)
			return
		}
		var m signalpb.ChatWebSocketMessage
		if proto.Unmarshal(b, &m) != nil || m.GetType() != signalpb.ChatWebSocketMessage_REQUEST {
			continue // keep-alive responses and anything unparsable
		}
		q := m.GetRequest()
		// Acknowledge before interpreting (§2.3).
		if err := ps.write(&signalpb.ChatWebSocketMessage{
			Type:     signalpb.ChatWebSocketMessage_RESPONSE.Enum(),
			Response: &signalpb.ChatWebSocketResponse{Id: proto.Uint64(q.GetId()), Status: proto.Uint32(200), Message: proto.String("OK")},
		}); err != nil {
			ps.fail(err)
			return
		}
		if q.GetVerb() != "PUT" || q.Body == nil {
			continue
		}
		switch q.GetPath() {
		case "/v1/address":
			var a signalpb.ProvisioningAddress
			if proto.Unmarshal(q.Body, &a) != nil || a.Address == nil {
				ps.fail(ErrNoAddress)
				return
			}
			select {
			case ps.addr <- a.GetAddress():
			default:
			}
		case "/v1/message":
			e := &signalpb.ProvisionEnvelope{}
			if err := proto.Unmarshal(q.Body, e); err != nil {
				ps.fail(fmt.Errorf("provision: bad envelope: %w", err))
				return
			}
			select {
			case ps.env <- e:
			default:
			}
		}
	}
}

func (ps *Session) keepAlive() {
	t := time.NewTicker(ps.t.keepAlive)
	defer t.Stop()
	for {
		select {
		case <-ps.done:
			return
		case now := <-t.C:
			err := ps.write(&signalpb.ChatWebSocketMessage{
				Type:    signalpb.ChatWebSocketMessage_REQUEST.Enum(),
				Request: &signalpb.ChatWebSocketRequest{Verb: proto.String("GET"), Path: proto.String("/v1/keepalive"), Id: proto.Uint64(uint64(now.UnixMilli()))},
			})
			if err != nil {
				ps.fail(err)
				return
			}
		}
	}
}

// Address waits for the provisioning address (at most 10 s after open).
func (ps *Session) Address(ctx context.Context) (string, error) {
	if ps.gotAd != "" {
		return ps.gotAd, nil
	}
	return waitFor(ctx, ps, ps.addr, ps.t.address, ErrNoAddress, func(a string) { ps.gotAd = a })
}

// Envelope waits for the encrypted ProvisionEnvelope (at most 90 s after
// open).
func (ps *Session) Envelope(ctx context.Context) (*signalpb.ProvisionEnvelope, error) {
	return waitFor(ctx, ps, ps.env, ps.t.message, ErrNoMessage, nil)
}

func waitFor[T any](ctx context.Context, ps *Session, ch <-chan T, limit time.Duration, timeoutErr error, keep func(T)) (T, error) {
	var zero T
	timer := time.NewTimer(max(time.Until(ps.opened.Add(limit)), 0))
	defer timer.Stop()
	select {
	case v := <-ch:
		if keep != nil {
			keep(v)
		}
		return v, nil
	case <-timer.C:
		return zero, timeoutErr
	case <-ps.done:
		return zero, ps.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// LinkURL is the sgnl://linkdevice URL for an address and the secondary's
// temporary public key (§5): the key in unpadded standard base64, both
// values form-URL-encoded.
func LinkURL(address string, pub *curve25519.PublicKey) string {
	return "sgnl://linkdevice?uuid=" + url.QueryEscape(address) +
		"&pub_key=" + url.QueryEscape(base64.RawStdEncoding.EncodeToString(pub.Bytes()))
}

// ParseLinkURL is the primary's parse of a link URL (§5): query
// parameters split by hand, uuid and pub_key required, the key decoded
// with or without padding. The scheme and host are not checked.
func ParseLinkURL(raw string) (address string, pub *curve25519.PublicKey, err error) {
	_, query, ok := strings.Cut(raw, "?")
	if !ok {
		return "", nil, fmt.Errorf("%w: no query", ErrInvalidLink)
	}
	params := map[string]string{}
	for _, part := range strings.Split(query, "&") {
		k, v, _ := strings.Cut(part, "=")
		dk, err1 := url.QueryUnescape(k)
		dv, err2 := url.QueryUnescape(v)
		if err1 != nil || err2 != nil {
			return "", nil, fmt.Errorf("%w: bad encoding", ErrInvalidLink)
		}
		params[dk] = dv
	}
	address, key := params["uuid"], params["pub_key"]
	if address == "" || key == "" {
		return "", nil, fmt.Errorf("%w: missing uuid or pub_key", ErrInvalidLink)
	}
	kb, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(key, "="))
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidLink, err)
	}
	if pub, err = curve25519.ParsePublicKey(kb); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidLink, err)
	}
	return address, pub, nil
}

// NewSession runs the provisioning protocol on an already open socket.
func NewSession(s Socket) *Session { return newSession(s, defaultTiming) }
