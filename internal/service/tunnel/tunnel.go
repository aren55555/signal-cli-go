// Package tunnel dials TCP connections, directly or through an HTTP
// CONNECT proxy. The sandbox reaches the Internet only through such a
// proxy, and every Signal transport (WebSockets, HTTP/2) dials through
// this.
package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Dial connects to host ("name:port") directly, or through proxy if it is
// non-nil. ctx bounds the dial and the CONNECT exchange.
func Dial(ctx context.Context, host string, proxy *url.URL) (net.Conn, error) {
	var d net.Dialer
	if proxy == nil {
		return d.DialContext(ctx, "tcp", host)
	}
	paddr := proxy.Host
	if proxy.Port() == "" {
		paddr = net.JoinHostPort(proxy.Hostname(), "80")
	}
	c, err := d.DialContext(ctx, "tcp", paddr)
	if err != nil {
		return nil, err
	}
	if dl, has := ctx.Deadline(); has {
		c.SetDeadline(dl)
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: host}, Host: host, Header: http.Header{}}
	if err := req.Write(c); err != nil {
		c.Close()
		return nil, err
	}
	// Read the response byte by byte up to the blank line, so no tunnelled
	// bytes are buffered away.
	resp, err := http.ReadResponse(bufio.NewReaderSize(&oneByte{c}, 16), req)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("tunnel: proxy: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.Close()
		return nil, fmt.Errorf("tunnel: proxy refused CONNECT %s: %s", host, resp.Status)
	}
	c.SetDeadline(time.Time{})
	return c, nil
}

// EnvProxy returns the proxy for rawURL from HTTPS_PROXY / HTTP_PROXY /
// NO_PROXY, as net/http does (ws maps to http, wss to https).
func EnvProxy(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	probe := *u
	if s, ok := map[string]string{"ws": "http", "wss": "https"}[u.Scheme]; ok {
		probe.Scheme = s
	}
	return http.ProxyFromEnvironment(&http.Request{URL: &probe})
}

// oneByte reads at most one byte per call, so a bufio.Reader over it
// never reads past what it is asked for.
type oneByte struct{ r io.Reader }

func (o *oneByte) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return o.r.Read(p)
}
