package websocket

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/service/h2"
)

// DialH2 opens a WebSocket over HTTP/2 extended CONNECT (RFC 8441), the
// way libsignal-net reaches Signal's chat service
// (spec/service/transport.md): TLS 1.3 with ALPN h2, a CONNECT request with
// :protocol "websocket", and WebSocket frames carried on the stream. There
// is no HTTP/1.1 upgrade handshake (no key or accept). ctx bounds the dial
// and handshake only; the open stream lives until Close.
func DialH2(ctx context.Context, rawURL string, opts Options) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "wss" {
		return nil, fmt.Errorf("websocket: HTTP/2 WebSockets need wss, got %q", u.Scheme)
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "443")
	}
	hc, err := h2.Dial(ctx, addr, h2.Options{TLSConfig: opts.TLSConfig, Proxy: opts.Proxy})
	if err != nil {
		return nil, err
	}
	headers := []h2.HeaderField{{Name: "sec-websocket-version", Value: "13"}}
	for k, vs := range opts.Header {
		name := strings.ToLower(k)
		for _, v := range vs {
			headers = append(headers, h2.HeaderField{Name: name, Value: v, Sensitive: name == "authorization"})
		}
	}
	s, err := hc.OpenConnect(ctx, "websocket", u.RequestURI(), headers)
	if err != nil {
		hc.Close()
		return nil, fmt.Errorf("%w: %w", ErrHandshake, err)
	}
	limit := opts.MaxMessage
	if limit == 0 {
		limit = 16 << 20
	}
	return &Conn{
		r: bufio.NewReader(s),
		w: s,
		closeFn: func() error {
			s.Close()
			return hc.Close()
		},
		maxMessage: limit,
	}, nil
}
