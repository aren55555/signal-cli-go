package websocket

import (
	"bufio"
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/service/h2"
)

// DialH2 opens a WebSocket over HTTP/2 extended CONNECT (RFC 8441), the
// way libsignal-net reaches Signal's chat service
// (spec/service/transport.md): TLS 1.3 with ALPN h2, a CONNECT request with
// :protocol "websocket", and WebSocket frames carried on the stream. There
// is no HTTP/1.1 upgrade handshake (no key or accept). ctx bounds the dial
// and handshake only; the open stream lives until Close. A non-2xx
// answer is an ErrHandshake wrapping *h2.StatusError, which carries the
// status and headers.
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
	s, err := hc.OpenConnect(ctx, "websocket", u.RequestURI(), h2Headers(opts.Header))
	if err != nil {
		hc.Close()
		return nil, fmt.Errorf("%w: %w", ErrHandshake, err)
	}
	limit := opts.MaxMessage
	if limit == 0 {
		limit = 16 << 20
	}
	resp := http.Header{}
	for _, h := range s.Header() {
		resp.Add(h.Name, h.Value)
	}
	return &Conn{
		r: bufio.NewReader(s),
		w: s,
		closeFn: func() error {
			s.Close()
			return hc.Close()
		},
		maxMessage: limit,
		respHeader: resp,
		h2:         hc,
	}, nil
}

// libsignalHeaderOrder is the order libsignal adds its request headers
// (spec/service/transport.md §3.4); sec-websocket-version comes last.
var libsignalHeaderOrder = []string{"authorization", "x-signal-receive-stories", "accept-language", "user-agent"}

// h2Headers lowers hdr to HTTP/2 header fields: libsignal's headers in its
// order, then any others sorted, then sec-websocket-version.
// Authorization is marked never-indexed.
func h2Headers(hdr http.Header) []h2.HeaderField {
	values := map[string][]string{}
	for k, vs := range hdr {
		n := strings.ToLower(k)
		values[n] = append(values[n], vs...)
	}
	names := slices.Collect(maps.Keys(values))
	rank := func(n string) int {
		if i := slices.Index(libsignalHeaderOrder, n); i >= 0 {
			return i
		}
		return len(libsignalHeaderOrder)
	}
	slices.SortFunc(names, func(a, b string) int {
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra - rb
		}
		return strings.Compare(a, b)
	})
	var out []h2.HeaderField
	for _, n := range names {
		for _, v := range values[n] {
			out = append(out, h2.HeaderField{Name: n, Value: v, Sensitive: n == "authorization"})
		}
	}
	return append(out, h2.HeaderField{Name: "sec-websocket-version", Value: "13"})
}

// H2 returns the HTTP/2 connection carrying an RFC 8441 socket (nil for
// an HTTP/1.1 one), for other requests on the same connection.
func (c *Conn) H2() *h2.Conn { return c.h2 }

// UpgradeHeaders converts headers to the h2 form used for the upgrade.
func UpgradeHeaders(hdr http.Header) []h2.HeaderField {
	out := h2Headers(hdr)
	return out[:len(out)-1] // without sec-websocket-version
}
