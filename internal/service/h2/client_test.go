package h2

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
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The tests use Go's own HTTP/2 server as an independent peer. Its
// extended-CONNECT support is enabled only by GODEBUG=http2xconnect=1 at
// process start, so the server runs in a child process (this test binary
// re-executed).
func TestMain(m *testing.M) {
	switch os.Getenv("H2_HELPER") {
	case "xconnect", "plain":
		runHelper()
		return
	}
	os.Exit(m.Run())
}

func runHelper() {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS13,
	})
	if err != nil {
		panic(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(echoHandler), Protocols: new(http.Protocols)}
	srv.Protocols.SetHTTP2(true)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	fmt.Printf("%s %s\n", port, base64.StdEncoding.EncodeToString(der))
	_ = srv.Serve(ln)
}

// echoHandler accepts an extended CONNECT with :protocol "test-proto" and
// header x-test: yes, then echoes the stream back.
func echoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect || r.Header.Get(":protocol") != "test-proto" || r.URL.Path != "/echo" || r.Header.Get("X-Test") != "yes" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	fl := w.(http.Flusher)
	fl.Flush()
	buf := make([]byte, 16<<10)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			fl.Flush()
		}
		if err != nil {
			return
		}
	}
}

func startHelper(t *testing.T, mode string) (port string, pool *x509.CertPool) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "H2_HELPER="+mode)
	if mode == "xconnect" {
		cmd.Env = append(cmd.Env, "GODEBUG=http2xconnect=1")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(line)
	der, _ := base64.StdEncoding.DecodeString(f[1])
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(cert)
	return f[0], pool
}

func open(t *testing.T, port string, pool *x509.CertPool, proxy *url.URL, hdr []HeaderField) (*Conn, *Stream, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Dial(ctx, "localhost:"+port, Options{TLSConfig: &tls.Config{RootCAs: pool}, Proxy: proxy})
	if err != nil {
		return nil, nil, err
	}
	s, err := c.OpenConnect(ctx, "test-proto", "/echo", hdr)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	return c, s, nil
}

var testHeaders = []HeaderField{{Name: "x-test", Value: "yes"}}

func TestConnectEcho(t *testing.T) {
	port, pool := startHelper(t, "xconnect")
	c, s, err := open(t, port, pool, nil, testHeaders)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := s.Write([]byte("hello over h2")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 13)
	if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "hello over h2" {
		t.Fatalf("echo %q, %v", buf, err)
	}
}

// TestConnectLargeTransfer moves 1 MiB each way at once, more than any
// initial flow-control window, so both directions must honour
// WINDOW_UPDATE and split frames.
func TestConnectLargeTransfer(t *testing.T) {
	port, pool := startHelper(t, "xconnect")
	c, s, err := open(t, port, pool, nil, testHeaders)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	data := make([]byte, 1<<20)
	r := mrand.New(mrand.NewPCG(1, 2))
	for i := range data {
		data[i] = byte(r.Uint32())
	}
	werr := make(chan error, 1)
	go func() {
		for off := 0; off < len(data); off += 50000 {
			end := min(off+50000, len(data))
			if _, err := s.Write(data[off:end]); err != nil {
				werr <- err
				return
			}
		}
		werr <- nil
	}()
	got := make([]byte, len(data))
	if _, err := io.ReadFull(s, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := <-werr; err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("echoed data differs")
	}
}

func TestConnectRejected(t *testing.T) {
	port, pool := startHelper(t, "xconnect")
	_, _, err := open(t, port, pool, nil, nil) // no x-test header: 400
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 400 {
		t.Errorf("rejected CONNECT: %v", err)
	}
}

func TestNoExtendedConnect(t *testing.T) {
	port, pool := startHelper(t, "plain")
	if _, _, err := open(t, port, pool, nil, testHeaders); !errors.Is(err, ErrExtendedConnectUnsupported) {
		t.Errorf("server without extended CONNECT: %v", err)
	}
}

// connectProxy is a minimal HTTP CONNECT proxy allowing only target.
func connectProxy(t *testing.T, target string) (*url.URL, chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	seen := make(chan string, 1)
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
		fmt.Fprintf(c, "HTTP/1.1 200 OK\r\n\r\n")
		go io.Copy(up, br)
		io.Copy(c, up)
	}()
	u, _ := url.Parse("http://" + ln.Addr().String())
	return u, seen
}

func TestConnectThroughProxy(t *testing.T) {
	port, pool := startHelper(t, "xconnect")
	proxy, seen := connectProxy(t, "localhost:"+port)
	c, s, err := open(t, port, pool, proxy, testHeaders)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := <-seen; got != "CONNECT localhost:"+port {
		t.Errorf("proxy saw %q", got)
	}
	s.Write([]byte("via proxy"))
	buf := make([]byte, 9)
	if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "via proxy" {
		t.Errorf("echo %q, %v", buf, err)
	}
}

// FuzzReadFrame: frames come from the server.
func FuzzReadFrame(f *testing.F) {
	f.Add([]byte{0, 0, 0, 4, 0, 0, 0, 0, 0})
	f.Add([]byte{0, 0, 5, 0, 1, 0, 0, 0, 1, 'h', 'e', 'l', 'l', 'o'})
	f.Fuzz(func(t *testing.T, in []byte) {
		fr, err := readFrame(bufio.NewReader(bytes.NewReader(in)), 1<<14)
		if err == nil && len(fr.payload) > 1<<14 {
			t.Fatal("payload over the frame size limit")
		}
	})
}

// scriptedPeer is the server end of a net.Pipe: it consumes the client
// preface and SETTINGS and answers with settings of its own.
func scriptedPeer(t *testing.T, settings map[uint16]uint32) (*Conn, net.Conn, *bufio.Reader) {
	t.Helper()
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	br := bufio.NewReader(srv)
	ready := make(chan error, 1)
	go func() {
		pre := make([]byte, len(clientPreface))
		if _, err := io.ReadFull(br, pre); err != nil || string(pre) != clientPreface {
			ready <- fmt.Errorf("preface %q, %v", pre, err)
			return
		}
		if f, err := readFrame(br, 1<<14); err != nil || f.typ != frameSettings {
			ready <- fmt.Errorf("client SETTINGS: %+v, %v", f, err)
			return
		}
		var p []byte
		for id, v := range settings {
			p = binary.BigEndian.AppendUint16(p, id)
			p = binary.BigEndian.AppendUint32(p, v)
		}
		_, err := srv.Write(appendFrame(nil, frameSettings, 0, 0, p))
		ready <- err
	}()
	c, err := newConn(cli, "example.org")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	return c, srv, br
}

// nextFrame skips SETTINGS acks and WINDOW_UPDATEs from the client.
func nextFrame(t *testing.T, br *bufio.Reader) frame {
	t.Helper()
	for {
		f, err := readFrame(br, 1<<14)
		if err != nil {
			t.Fatal(err)
		}
		if f.typ != frameSettings && f.typ != frameWindowUpdate {
			return f
		}
	}
}

// TestSendRespectsFlowControl: with a 10-byte stream window the client
// sends 10 bytes, then waits for WINDOW_UPDATE before sending the rest
// (RFC 9113 §6.9). The stdlib server's windows are too large to show this.
func TestSendRespectsFlowControl(t *testing.T) {
	c, srv, br := scriptedPeer(t, map[uint16]uint32{settingEnableConnectProtocol: 1, settingInitialWindowSize: 10})
	type res struct {
		s   *Stream
		err error
	}
	opened := make(chan res, 1)
	go func() {
		s, err := c.OpenConnect(context.Background(), "websocket", "/x", nil)
		opened <- res{s, err}
	}()
	h := nextFrame(t, br)
	if h.typ != frameHeaders || h.flags&flagEndHeaders == 0 {
		t.Fatalf("got frame %+v, want HEADERS", h)
	}
	fields, err := NewDecoder(4096).Decode(h.payload)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{":method": "CONNECT", ":protocol": "websocket", ":scheme": "https", ":path": "/x", ":authority": "example.org"}
	for _, f := range fields {
		if v, ok := want[f.Name]; ok && v == f.Value {
			delete(want, f.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("HEADERS %v missing %v", fields, want)
	}
	// :status 200 is static index 8.
	srv.Write(appendFrame(nil, frameHeaders, flagEndHeaders, h.stream, []byte{0x88}))
	r := <-opened
	if r.err != nil {
		t.Fatal(r.err)
	}
	wrote := make(chan error, 1)
	go func() { _, err := r.s.Write(bytes.Repeat([]byte{'a'}, 25)); wrote <- err }()
	if d := nextFrame(t, br); d.typ != frameData || len(d.payload) != 10 {
		t.Fatalf("first DATA %d bytes (type %d), want 10", len(d.payload), d.typ)
	}
	select {
	case <-wrote:
		t.Fatal("Write finished past the window")
	case <-time.After(100 * time.Millisecond):
	}
	srv.Write(appendFrame(nil, frameWindowUpdate, 0, h.stream, binary.BigEndian.AppendUint32(nil, 100)))
	if d := nextFrame(t, br); d.typ != frameData || len(d.payload) != 15 {
		t.Fatalf("second DATA %d bytes, want 15", len(d.payload))
	}
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
}
