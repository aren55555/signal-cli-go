package enclave

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

// Conn is an attested, Noise-encrypted enclave connection
// (attestation.md §2, noise.md §6).
type Conn struct {
	ws     *websocket.Conn
	t      *Transport
	Claims map[string][]byte
}

// DialOptions configure an enclave connection.
type DialOptions struct {
	Username, Password string // Basic auth from the chat service
	Roots              *x509.CertPool
	Proxy              *url.URL
	UserAgent          string
	Now                time.Time // attestation time; zero = now
}

// Dial connects to rawURL, verifies the enclave against mrenclave and
// completes the NKhfs handshake.
func Dial(ctx context.Context, rawURL string, mrenclave []byte, o DialOptions) (*Conn, error) {
	h := http.Header{}
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(o.Username+":"+o.Password)))
	if o.UserAgent != "" {
		h.Set("User-Agent", o.UserAgent)
	}
	ws, err := websocket.Dial(ctx, rawURL, websocket.Options{Header: h, Proxy: o.Proxy, TLSConfig: &tls.Config{RootCAs: o.Roots, MinVersion: tls.VersionTLS12}, MaxMessage: 11 << 20})
	if err != nil {
		return nil, err
	}
	c := &Conn{ws: ws}
	fail := func(err error) (*Conn, error) {
		ws.Close()
		return nil, err
	}
	first, err := ws.Read()
	if err != nil {
		return fail(err)
	}
	ev, en, err := ParseHandshakeStart(first)
	if err != nil {
		return fail(err)
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	if c.Claims, err = Verify(ev, en, mrenclave, now); err != nil {
		return fail(err)
	}
	pk := c.Claims["pk"]
	if len(pk) != 32 {
		return fail(&DataError{"Claims do not contain a 32-byte pk"})
	}
	in, msg1, err := NewInitiator(NKhfs, pk, Options{})
	if err != nil {
		return fail(err)
	}
	if err := ws.Write(msg1); err != nil {
		return fail(err)
	}
	msg2, err := ws.Read()
	if err != nil {
		return fail(err)
	}
	if c.t, err = in.Finish(msg2); err != nil {
		return fail(err)
	}
	return c, nil
}

// Send encrypts and sends one application message.
func (c *Conn) Send(p []byte) error { return c.ws.Write(c.t.Seal(p)) }

// Recv reads and decrypts one application message. A close from the
// server is returned as *websocket.CloseError.
func (c *Conn) Recv() ([]byte, error) {
	f, err := c.ws.Read()
	if err != nil {
		return nil, err
	}
	return c.t.Open(f)
}

// Close closes with 1000.
func (c *Conn) Close() error { return c.ws.Close() }

// CloseCode extracts a server close code from a Recv error: code 0 and
// ok true for a close without status; ok false for other errors.
func CloseCode(err error) (code int, reason string, ok bool) {
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return ce.Code, ce.Reason, true
	}
	if errors.Is(err, websocket.ErrClosed) {
		return 0, "", true
	}
	return 0, "", false
}
