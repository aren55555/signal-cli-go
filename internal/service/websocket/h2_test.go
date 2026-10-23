package websocket

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Go's HTTP/2 server only offers extended CONNECT with
// GODEBUG=http2xconnect=1, read at process start. The client side, which is
// what signal-cli-go uses, needs no such setting. So the tests run the
// server in a child process (this test binary, re-executed) and keep the
// setting out of the code under test.
func TestMain(m *testing.M) {
	if os.Getenv("WS_H2_HELPER") == "1" {
		runH2Helper()
		return
	}
	os.Exit(m.Run())
}

// runH2Helper serves RFC 8441 WebSockets over HTTP/2 on localhost and
// prints "<port> <base64 certificate DER>" on stdout.
func runH2Helper() {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
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
	srv := &http.Server{Handler: http.HandlerFunc(h2Handler), Protocols: new(http.Protocols)}
	srv.Protocols.SetHTTP2(true)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	fmt.Printf("%s %s\n", port, base64.StdEncoding.EncodeToString(der))
	_ = srv.Serve(ln)
}

func h2Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect || r.Proto != "HTTP/2.0" || r.Header.Get(":protocol") != "websocket" ||
		r.Header.Get("Sec-WebSocket-Version") != "13" || r.URL.Path != "/v1/websocket/" || r.Header.Get("Authorization") != "Basic dGVzdA==" {
		http.Error(w, fmt.Sprintf("bad request %s %s %q", r.Method, r.Proto, r.URL.Path), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	fl := w.(http.Flusher)
	fl.Flush()
	_ = writeFrame(w, true, opPing, []byte("hb"), nil)
	_ = writeFrame(w, false, opBinary, []byte("over "), nil)
	_ = writeFrame(w, true, opContinuation, []byte("h2"), nil)
	fl.Flush()
	br := bufio.NewReader(r.Body)
	for {
		f, err := readFrame(br, 1<<20, true)
		if err != nil {
			return
		}
		switch f.op {
		case opBinary: // echo back reversed, to prove a round trip
			b := []byte(string(f.payload))
			for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
				b[i], b[j] = b[j], b[i]
			}
			_ = writeFrame(w, true, opBinary, b, nil)
			fl.Flush()
		case opClose:
			_ = writeFrame(w, true, opClose, f.payload, nil)
			fl.Flush()
			return
		}
	}
}

func startH2Helper(t *testing.T) (port string, pool *x509.CertPool) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "WS_H2_HELPER=1", "GODEBUG=http2xconnect=1")
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
	fields := strings.Fields(line)
	der, _ := base64.StdEncoding.DecodeString(fields[1])
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(cert)
	return fields[0], pool
}

func TestDialH2ThroughProxy(t *testing.T) {
	port, pool := startH2Helper(t)
	target := "localhost:" + port
	proxyAddr, seen := connectProxy(t, target)
	proxy, _ := url.Parse("http://" + proxyAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := DialH2(ctx, "wss://"+target+"/v1/websocket/", Options{
		Header:    http.Header{"Authorization": {"Basic dGVzdA=="}},
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
	if err != nil || string(msg) != "over h2" {
		t.Fatalf("read %q, %v", msg, err)
	}
	if err := c.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if msg, err := c.Read(); err != nil || string(msg) != "cba" {
		t.Fatalf("echo %q, %v", msg, err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestDialH2Rejected(t *testing.T) {
	port, pool := startH2Helper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// No authorization header: the helper answers 400.
	_, err := DialH2(ctx, "wss://localhost:"+port+"/v1/websocket/", Options{TLSConfig: &tls.Config{RootCAs: pool}})
	if !errors.Is(err, ErrHandshake) || !strings.Contains(err.Error(), "400") {
		t.Errorf("rejected handshake: %v", err)
	}
}

func TestDialH2NeedsH2Server(t *testing.T) {
	// An HTTP/1.1-only TLS server: extended CONNECT must fail cleanly.
	serverTLS, pool := selfSigned(t)
	serverTLS.NextProtos = []string{"http/1.1"}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go http.Serve(ln, http.NotFoundHandler())
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := DialH2(ctx, "wss://localhost:"+port+"/", Options{TLSConfig: &tls.Config{RootCAs: pool}}); err == nil {
		t.Error("extended CONNECT to an HTTP/1.1 server succeeded")
	}
}
