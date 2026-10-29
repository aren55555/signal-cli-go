// SPDX-License-Identifier: GPL-3.0-or-later

package cdn

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/service/agent"
)

type recorded struct {
	method, uri string
	header      http.Header
	body        []byte
}

type fakeCDN struct {
	mu   sync.Mutex
	reqs []recorded
	data map[string][]byte // tus/GCS uploads by path
	srv  *httptest.Server
}

func newFake(t *testing.T) (*fakeCDN, *Client) {
	f := &fakeCDN{data: map[string][]byte{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(f.srv.Certificate())
	base := f.srv.URL
	c := New(Config{Bases: map[uint32]string{0: base + "/c0", 2: base + "/c2", 3: base + "/c3"}, Roots: pool, UserAgentOverride: "test-agent"})
	return f, c
}

func (f *fakeCDN) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, recorded{r.Method, r.RequestURI, r.Header.Clone(), body})
	switch {
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/c2/"):
		w.Header().Set("Location", "https://storage.example/session/abc?upload_id=xyz")
		w.WriteHeader(201)
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/c3/"):
		w.Header().Set("Location", "https://cdn3.example/attachments/KEY")
		w.WriteHeader(201)
	case r.Method == "PUT" && r.Header.Get("Content-Range") != "" && strings.HasPrefix(r.Header.Get("Content-Range"), "bytes */"):
		w.WriteHeader(308)
	case r.Method == "PUT":
		f.data[r.URL.Path] = body
		w.WriteHeader(200)
	case r.Method == "HEAD":
		w.Header().Set("Upload-Offset", "0")
		w.WriteHeader(200)
	case r.Method == "PATCH":
		f.data[r.URL.Path] = body
		w.WriteHeader(204)
	case r.Method == "GET" && r.URL.Path == "/c3/attachments/big":
		w.Write(make([]byte, 100))
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/c3/attachments/"):
		w.Write([]byte("blob:" + r.URL.RawPath))
	default:
		w.WriteHeader(404)
	}
}

func TestUploadCDN2(t *testing.T) {
	f, c := newFake(t)
	form := Form{CDN: 2, Key: "KEY", Headers: map[string]string{"host": "storage.example", "x-goog-resumable": "start", "x-goog-content-length-range": "1,592"},
		SignedUploadLocation: "https://storage.example/bucket/KEY?X-Goog-Signature=s%2Fig"}
	loc, err := c.Create(context.Background(), form)
	if err != nil {
		t.Fatal(err)
	}
	blob := bytes.Repeat([]byte{7}, 592)
	if err := c.Upload(context.Background(), form, loc, blob); err != nil {
		t.Fatal(err)
	}
	r := f.reqs
	if len(r) != 3 {
		t.Fatalf("%d requests", len(r))
	}
	if r[0].method != "POST" || r[0].uri != "/c2/bucket/KEY?X-Goog-Signature=s%2Fig" || r[0].header.Get("x-goog-resumable") != "start" ||
		r[0].header.Get("Content-Type") != "application/octet-stream" || r[0].header.Get("X-Signal-Agent") != agent.String("test-agent") ||
		r[0].header.Get("User-Agent") != agent.String("test-agent") || len(r[0].body) != 0 || r[0].header.Get("Authorization") != "" {
		t.Errorf("create: %+v", r[0])
	}
	if r[1].method != "PUT" || r[1].uri != "/c2/session/abc?upload_id=xyz" || r[1].header.Get("Content-Range") != "bytes */592" || r[1].header.Get("x-goog-resumable") != "" {
		t.Errorf("probe: %+v", r[1])
	}
	if r[2].header.Get("Content-Range") != "bytes 0-591/592" || !bytes.Equal(r[2].body, blob) || r[2].header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("data: %+v", r[2].header)
	}
}

func TestUploadCDN3(t *testing.T) {
	f, c := newFake(t)
	form := Form{CDN: 3, Key: "KEY", Headers: map[string]string{"Authorization": "Bearer jwt", "Upload-Metadata": "filename S0VZ"},
		SignedUploadLocation: "https://tus.example/attachments"}
	loc, err := c.Create(context.Background(), form)
	if err != nil {
		t.Fatal(err)
	}
	blob := []byte("ciphertext")
	if err := c.Upload(context.Background(), form, loc, blob); err != nil {
		t.Fatal(err)
	}
	r := f.reqs
	if len(r) != 3 {
		t.Fatalf("%d requests", len(r))
	}
	if r[0].uri != "/c3/attachments" || r[0].header.Get("Upload-Defer-Length") != "1" || r[0].header.Get("Tus-Resumable") != "1.0.0" ||
		r[0].header.Get("Authorization") != "Bearer jwt" || r[0].header.Get("Content-Length") != "0" {
		t.Errorf("create: %+v", r[0])
	}
	if r[1].method != "HEAD" || r[1].uri != "/c3/attachments/KEY" || r[1].header.Get("Authorization") != "Bearer jwt" || r[1].header.Get("Tus-Resumable") != "1.0.0" {
		t.Errorf("head: %+v", r[1])
	}
	p := r[2]
	if p.method != "PATCH" || p.header.Get("Upload-Offset") != "0" || p.header.Get("Upload-Length") != fmt.Sprint(len(blob)) ||
		p.header.Get("Content-Type") != "application/offset+octet-stream" || p.header.Get("Upload-Metadata") != "filename S0VZ" || !bytes.Equal(p.body, blob) {
		t.Errorf("patch: %+v", p)
	}
}

func TestUnknownCDN(t *testing.T) {
	_, c := newFake(t)
	if _, err := c.Create(context.Background(), Form{CDN: 7, SignedUploadLocation: "https://x/y"}); err == nil || err.Error() != "Unknown CDN version" {
		t.Errorf("cdn 7: %v", err)
	}
	if _, err := c.Download(context.Background(), 5, "x", 10); err == nil || err.Error() != "Attempted to download from unsupported CDN number" {
		t.Errorf("download cdn 5: %v", err)
	}
}

func TestDownload(t *testing.T) {
	f, c := newFake(t)
	got, err := c.Download(context.Background(), 3, "a b/c+d=", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if f.reqs[0].uri != "/c3/attachments/a+b%2Fc%2Bd%3D" || f.reqs[0].header.Get("Authorization") != "" {
		t.Errorf("uri %s", f.reqs[0].uri)
	}
	if !strings.HasPrefix(string(got), "blob:") {
		t.Errorf("body %q", got)
	}
	if _, err := c.Download(context.Background(), 3, "big", 99); err == nil {
		t.Error("cap not enforced")
	}
	if _, err := c.Download(context.Background(), 3, "big", 100); err != nil {
		t.Errorf("exact cap: %v", err)
	}
	if _, err := c.Download(context.Background(), 2, "missing", 100); err == nil || err.Error() != "Response code: 404" {
		t.Errorf("404: %v", err)
	}
}
