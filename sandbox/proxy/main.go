// Command egress-proxy is a tiny allowlisting HTTP forward proxy.
//
// The sandbox's agent container is attached only to an internal network
// with no route to the outside world. The one thing it can reach is this
// proxy, which lives on both the internal and the external network and
// forwards traffic only to hosts listed in an allowlist file
// (sandbox/ALLOWED_HOSTS). Everything else is refused with 403 and logged.
//
// Supported: CONNECT tunnels (HTTPS, WebSocket over TLS) to ports 443 and
// 80, and plain absolute-URI HTTP requests. Standard library only.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	listen := flag.String("listen", ":3128", "address to listen on")
	allowPath := flag.String("allow", "/etc/egress/ALLOWED_HOSTS", "allowlist file, one host per line")
	flag.Parse()

	rules, err := loadAllowlist(*allowPath)
	if err != nil {
		log.Fatalf("egress-proxy: %v", err)
	}
	log.Printf("egress-proxy: listening on %s with %d allowed host patterns from %s", *listen, len(rules.patterns), *allowPath)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           &proxy{rules: rules},
		ReadHeaderTimeout: 30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// allowlist holds host patterns. A pattern is either an exact host name or
// "*.example.com", which matches any proper subdomain of example.com.
type allowlist struct {
	patterns []string
}

func loadAllowlist(path string) (*allowlist, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	al := &allowlist{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		al.patterns = append(al.patterns, strings.TrimSuffix(line, "."))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(al.patterns) == 0 {
		return nil, errors.New("allowlist is empty; refusing to start")
	}
	return al, nil
}

func (al *allowlist) allows(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, p := range al.patterns {
		if host == p {
			return true
		}
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(host, p[1:]) && len(host) > len(p)-1 {
			return true
		}
	}
	return false
}

type proxy struct {
	rules *allowlist
	once  sync.Once
	tr    *http.Transport
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	if !r.URL.IsAbs() {
		http.Error(w, "egress-proxy: this is a forward proxy; send absolute URIs or CONNECT", http.StatusBadRequest)
		return
	}
	p.handleForward(w, r)
}

func (p *proxy) deny(w http.ResponseWriter, r *http.Request, host string) {
	log.Printf("DENY  %s %s (host %q not in allowlist)", r.Method, r.Host, host)
	http.Error(w, fmt.Sprintf("egress-proxy: host %q is not in sandbox/ALLOWED_HOSTS", host), http.StatusForbidden)
}

func (p *proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		http.Error(w, "egress-proxy: CONNECT target must be host:port", http.StatusBadRequest)
		return
	}
	if !p.rules.allows(host) {
		p.deny(w, r, host)
		return
	}
	if port != "443" && port != "80" {
		log.Printf("DENY  CONNECT %s (port %s not permitted)", r.Host, port)
		http.Error(w, "egress-proxy: only ports 443 and 80 are permitted", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		log.Printf("ERROR CONNECT %s: dial: %v", r.Host, err)
		http.Error(w, "egress-proxy: upstream dial failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "egress-proxy: hijack unsupported", http.StatusInternalServerError)
		return
	}
	client, clientBuf, err := hj.Hijack()
	if err != nil {
		log.Printf("ERROR CONNECT %s: hijack: %v", r.Host, err)
		return
	}
	defer client.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	log.Printf("ALLOW CONNECT %s", r.Host)

	// Anything the client already sent after its headers must go upstream.
	if n := clientBuf.Reader.Buffered(); n > 0 {
		buffered, _ := clientBuf.Reader.Peek(n)
		if _, err := upstream.Write(buffered); err != nil {
			return
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go pipe(&wg, upstream, client)
	go pipe(&wg, client, upstream)
	wg.Wait()
}

// pipe copies src to dst and then half-closes dst so the peer sees EOF.
func pipe(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	if tc, ok := dst.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	} else {
		_ = dst.Close()
	}
}

func (p *proxy) handleForward(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Hostname()
	if !p.rules.allows(host) {
		p.deny(w, r, host)
		return
	}
	p.once.Do(func() {
		p.tr = &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
		}
	})

	out := r.Clone(r.Context())
	out.RequestURI = ""
	stripHopByHop(out.Header)
	resp, err := p.tr.RoundTrip(out)
	if err != nil {
		log.Printf("ERROR %s %s: %v", r.Method, r.URL, err)
		http.Error(w, "egress-proxy: upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	log.Printf("ALLOW %s %s -> %d", r.Method, r.URL, resp.StatusCode)

	stripHopByHop(resp.Header)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

var hopByHop = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func stripHopByHop(h http.Header) {
	for _, tok := range h.Values("Connection") {
		for _, name := range strings.Split(tok, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(name)
			}
		}
	}
	for _, k := range hopByHop {
		h.Del(k)
	}
}
