package websocket

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestAcceptKey is the example in RFC 6455 section 1.3.
func TestAcceptKey(t *testing.T) {
	if got := acceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("accept key %q", got)
	}
}

// TestFrameExamples checks the frames of RFC 6455 section 5.7.
func TestFrameExamples(t *testing.T) {
	mask := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	for _, c := range []struct {
		name    string
		wire    string
		fin     bool
		op      opcode
		payload string
		mask    *[4]byte
	}{
		{"unmasked text", "81 05 48656c6c6f", true, opText, "Hello", nil},
		{"masked text", "81 85 37fa213d 7f9f4d5158", true, opText, "Hello", &mask},
		{"fragment 1", "01 03 48656c", false, opText, "Hel", nil},
		{"fragment 2", "80 02 6c6f", true, opContinuation, "lo", nil},
		{"unmasked ping", "89 05 48656c6c6f", true, opPing, "Hello", nil},
		{"masked pong", "8a 85 37fa213d 7f9f4d5158", true, opPong, "Hello", &mask},
	} {
		var buf bytes.Buffer
		if err := writeFrame(&buf, c.fin, c.op, []byte(c.payload), c.mask); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf.Bytes(), unhex(t, c.wire)) {
			t.Errorf("%s: encoded %x, want %s", c.name, buf.Bytes(), c.wire)
		}
		f, err := readFrame(bufio.NewReader(bytes.NewReader(unhex(t, c.wire))), 1<<20, c.mask != nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if f.fin != c.fin || f.op != c.op || string(f.payload) != c.payload {
			t.Errorf("%s: decoded %+v", c.name, f)
		}
	}
	// 256 bytes and 64 KiB use the 16-bit and 64-bit length forms.
	for _, n := range []int{256, 65536} {
		payload := bytes.Repeat([]byte{0xab}, n)
		var buf bytes.Buffer
		_ = writeFrame(&buf, true, opBinary, payload, nil)
		head := map[int]string{256: "82 7e 0100", 65536: "82 7f 0000000000010000"}[n]
		if !bytes.HasPrefix(buf.Bytes(), unhex(t, head)) || buf.Len() != len(unhex(t, head))+n {
			t.Errorf("%d-byte frame header %x", n, buf.Bytes()[:12])
		}
		f, err := readFrame(bufio.NewReader(&buf), 1<<20, false)
		if err != nil || !bytes.Equal(f.payload, payload) {
			t.Errorf("%d-byte frame: %v", n, err)
		}
	}
}

func TestFrameViolations(t *testing.T) {
	for name, c := range map[string]struct {
		wire       string
		wantMasked bool
		limit      int64
	}{
		"masked frame from server":   {"81 85 37fa213d 7f9f4d5158", false, 1 << 20},
		"unmasked frame to server":   {"81 05 48656c6c6f", true, 1 << 20},
		"reserved bit set":           {"c1 05 48656c6c6f", false, 1 << 20},
		"unknown opcode":             {"83 00", false, 1 << 20},
		"fragmented ping":            {"09 00", false, 1 << 20},
		"oversized ping":             {"89 7e 007e" + strings.Repeat("00", 126), false, 1 << 20},
		"over the size limit":        {"82 7e 0100" + strings.Repeat("00", 256), false, 255},
		"64-bit length with top bit": {"82 7f 8000000000000000", false, 1 << 20},
		"truncated payload":          {"82 05 0102", false, 1 << 20},
	} {
		_, err := readFrame(bufio.NewReader(bytes.NewReader(unhex(t, c.wire))), c.limit, c.wantMasked)
		if !errors.Is(err, ErrProtocol) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// fakeServer is a minimal WebSocket server for tests: it answers the
// handshake (optionally wrongly) and hands the raw connection to fn.
func fakeServer(t *testing.T, ln net.Listener, accept func(key string) string, status int, fn func(*bufio.ReadWriter)) {
	t.Helper()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		rw := bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))
		req, err := http.ReadRequest(rw.Reader)
		if err != nil {
			return
		}
		if req.Header.Get("Upgrade") != "websocket" || req.Header.Get("Sec-WebSocket-Version") != "13" ||
			!strings.EqualFold(req.Header.Get("Connection"), "Upgrade") || req.Header.Get("X-Test") != "yes" {
			fmt.Fprintf(rw, "HTTP/1.1 400 Bad Request\r\n\r\n")
			rw.Flush()
			return
		}
		fmt.Fprintf(rw, "HTTP/1.1 %d Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			status, accept(req.Header.Get("Sec-WebSocket-Key")))
		rw.Flush()
		if fn != nil {
			fn(rw)
		}
	}()
}

// selfSigned returns a TLS config for a server certificate valid for
// localhost and a pool that trusts it.
func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		DNSNames:              []string{"localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}

// connectProxy is a minimal HTTP CONNECT proxy that allows only target.
func connectProxy(t *testing.T, target string) (addr string, seen chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	seen = make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		seen <- req.Method + " " + req.Host
		if req.Method != http.MethodConnect || req.Host != target {
			fmt.Fprintf(c, "HTTP/1.1 403 Forbidden\r\n\r\n")
			return
		}
		up, err := net.Dial("tcp", target)
		if err != nil {
			return
		}
		defer up.Close()
		fmt.Fprintf(c, "HTTP/1.1 200 Connection established\r\n\r\n")
		go io.Copy(up, br)
		io.Copy(c, up)
	}()
	return ln.Addr().String(), seen
}

func TestDialThroughProxyAndExchange(t *testing.T) {
	serverTLS, pool := selfSigned(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	target := "localhost:" + port

	got := make(chan []byte, 1)
	fakeServer(t, ln, acceptKey, 101, func(rw *bufio.ReadWriter) {
		// Server: ping, then a fragmented binary message, then read the
		// client's message, then close.
		_ = writeFrame(rw, true, opPing, []byte("hb"), nil)
		_ = writeFrame(rw, false, opBinary, []byte("frag"), nil)
		_ = writeFrame(rw, true, opContinuation, []byte("mented"), nil)
		rw.Flush()
		for {
			f, err := readFrame(rw.Reader, 1<<20, true)
			if err != nil {
				return
			}
			switch f.op {
			case opPong:
				if string(f.payload) != "hb" {
					got <- []byte("bad pong")
				}
			case opBinary:
				got <- f.payload
			case opClose:
				_ = writeFrame(rw, true, opClose, f.payload, nil)
				rw.Flush()
				return
			}
		}
	})

	proxyAddr, seen := connectProxy(t, target)
	proxy, _ := url.Parse("http://" + proxyAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Dial(ctx, "wss://"+target+"/v1/websocket/", Options{
		Header:    http.Header{"X-Test": {"yes"}},
		Proxy:     proxy,
		TLSConfig: &tls.Config{RootCAs: pool},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s := <-seen; s != "CONNECT "+target {
		t.Errorf("proxy saw %q", s)
	}
	msg, err := c.Read()
	if err != nil || string(msg) != "fragmented" {
		t.Fatalf("read %q, %v", msg, err)
	}
	if err := c.Write([]byte("from client")); err != nil {
		t.Fatal(err)
	}
	if b := <-got; string(b) != "from client" {
		t.Errorf("server got %q", b)
	}
	if err := c.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	if _, err := c.Read(); !errors.Is(err, ErrClosed) {
		t.Errorf("read after close: %v", err)
	}
}

func TestHandshakeFailures(t *testing.T) {
	for name, c := range map[string]struct {
		accept func(string) string
		status int
		header http.Header
	}{
		"wrong accept key": {func(string) string { return "wrong" }, 101, http.Header{"X-Test": {"yes"}}},
		"status 200":       {acceptKey, 200, http.Header{"X-Test": {"yes"}}},
		"rejected request": {acceptKey, 101, nil},
	} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		fakeServer(t, ln, c.accept, c.status, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = Dial(ctx, "ws://"+ln.Addr().String()+"/", Options{Header: c.header})
		cancel()
		ln.Close()
		if !errors.Is(err, ErrHandshake) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestProxyRefusal(t *testing.T) {
	proxyAddr, _ := connectProxy(t, "allowed.example:443")
	proxy, _ := url.Parse("http://" + proxyAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Dial(ctx, "wss://blocked.example/", Options{Proxy: proxy}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("refused CONNECT: %v", err)
	}
}

// FuzzReadFrame: frames come from the network.
func FuzzReadFrame(f *testing.F) {
	f.Add([]byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'})
	f.Add([]byte{0x82, 0x7f, 0, 0, 0, 0, 0, 0, 1, 0})
	f.Fuzz(func(t *testing.T, in []byte) {
		fr, err := readFrame(bufio.NewReader(bytes.NewReader(in)), 1<<16, false)
		if err == nil && int64(len(fr.payload)) > 1<<16 {
			t.Fatal("payload over the limit")
		}
	})
}
