package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	return New(Config{BaseURL: srv.URL, Roots: pool})
}

// TestRequestHeaders: api.md §2.3 and transport.md §4.
func TestRequestHeaders(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.ProtoMajor != 2:
			http.Error(w, "want h2, got "+r.Proto, 400)
		case r.Method != "PUT" || r.URL.Path != "/v1/devices/link":
			http.Error(w, "path", 400)
		case r.Header.Get("User-Agent") != "Signal-Android/8.21.1 signal-cli/0.14.7" || r.Header.Get("X-Signal-Agent") != "Signal-Android/8.21.1 signal-cli/0.14.7":
			http.Error(w, "agent "+r.Header.Get("User-Agent"), 400)
		case r.Header.Get("Content-Type") != "application/json; charset=utf-8":
			http.Error(w, "content type", 400)
		case r.Header.Get("Authorization") != "Basic KzE1NTU1NTUwMTAwOnB3":
			http.Error(w, "auth "+r.Header.Get("Authorization"), 400)
		case string(body) != `{"b":null,"a":1}`:
			http.Error(w, "body "+string(body), 400)
		default:
			w.Write([]byte(`{"deviceId":2}`))
		}
	})
	type req struct {
		B *string `json:"b"`
		A int     `json:"a"`
	}
	var out struct {
		DeviceID int `json:"deviceId"`
	}
	err := c.Do(context.Background(), "PUT", "/v1/devices/link", &Basic{User: "+15555550100", Password: "pw"}, req{A: 1}, &out)
	if err != nil || out.DeviceID != 2 {
		t.Fatalf("Do: %+v, %v", out, err)
	}
}

func TestNoBodyNoAuth(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "" || r.Header.Get("Authorization") != "" || r.ContentLength > 0 {
			http.Error(w, "unexpected body or auth", 400)
			return
		}
		w.WriteHeader(204)
	})
	if err := c.Do(context.Background(), "GET", "/v1/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

// TestStatusMapping: api.md §4.1.
func TestStatusMapping(t *testing.T) {
	var status int
	var hdr http.Header
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		for k, v := range hdr {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
		w.Write([]byte(`{"why":"x"}`))
	})
	var rl *RateLimitError
	var se *StatusError
	for _, tc := range []struct {
		status int
		hdr    http.Header
		check  func(error) bool
	}{
		{429, http.Header{"Retry-After": {"30"}}, func(err error) bool { return errors.As(err, &rl) && rl.After == 30*time.Second }},
		{413, nil, func(err error) bool { return errors.As(err, &rl) && rl.After == 0 }},
		{429, http.Header{"Retry-After": {"soon"}}, func(err error) bool { return errors.As(err, &rl) && rl.After == 0 }},
		{401, nil, func(err error) bool { return errors.Is(err, ErrAuthorization) }},
		{403, nil, func(err error) bool { return errors.Is(err, ErrAuthorization) }},
		{409, nil, func(err error) bool {
			return errors.As(err, &se) && se.Status == 409 && string(se.Body) == `{"why":"x"}`
		}},
		{202, nil, func(err error) bool { return err == nil }},
	} {
		status, hdr = tc.status, tc.hdr
		if err := c.Do(context.Background(), "GET", "/", nil, nil, nil); !tc.check(err) {
			t.Errorf("%d %v: %v", tc.status, tc.hdr, err)
		}
	}
}
