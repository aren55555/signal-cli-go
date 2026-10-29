// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/h2"
	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

func stubDial(t *testing.T, sock Socket, resp http.Header, err error) *websocket.Options {
	t.Helper()
	var got websocket.Options
	var gotURL string
	old := dialSocket
	dialSocket = func(ctx context.Context, url string, opts websocket.Options) (Socket, http.Header, error) {
		got, gotURL = opts, url
		return sock, resp, err
	}
	t.Cleanup(func() {
		dialSocket = old
		if gotURL != "" && gotURL != "wss://grpc.chat.staging.signal.org/v1/websocket/" {
			t.Errorf("dialled %q", gotURL)
		}
	})
	return &got
}

// TestDialHeaders: transport.md §3.4 and §4.
func TestDialHeaders(t *testing.T) {
	creds := &Credentials{ACI: "0b6ab9b6-0000-4000-8000-00000000c5d3", DeviceID: 1, Password: "pw"}
	for _, tc := range []struct {
		name    string
		ep      Endpoint
		auth    string
		stories string
	}{
		{"authenticated", Endpoint{Credentials: creds, ReceiveStories: true}, "Basic MGI2YWI5YjYtMDAwMC00MDAwLTgwMDAtMDAwMDAwMDBjNWQzLjE6cHc=", "true"},
		{"no stories", Endpoint{Credentials: creds}, "Basic MGI2YWI5YjYtMDAwMC00MDAwLTgwMDAtMDAwMDAwMDBjNWQzLjE6cHc=", "false"},
		{"unauthenticated", Endpoint{ReceiveStories: true}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := stubDial(t, newFake(), http.Header{"X-Signal-Timestamp": {"1700000000123"}}, nil)
			c, err := Dial(context.Background(), tc.ep)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			h := opts.Header
			if h.Get("Authorization") != tc.auth || h.Get("X-Signal-Receive-Stories") != tc.stories ||
				h.Get("User-Agent") != "Signal-Android/8.21.1 signal-cli/0.14.7 libsignal/0.99.1" || len(h) != 1+btoi(tc.auth != "")*2 {
				t.Errorf("headers %v", h)
			}
			if opts.TLSConfig == nil || opts.TLSConfig.RootCAs == nil {
				t.Error("chat roots not set")
			}
			if c.ServerTime() != time.UnixMilli(1700000000123) {
				t.Errorf("server time %v", c.ServerTime())
			}
		})
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestDialFailures: pipe.md §2 and transport.md §3.4. Retry-After wins
// over the status.
func TestDialFailures(t *testing.T) {
	status := func(code int, hdr ...h2.HeaderField) error {
		return fmt.Errorf("%w: %w", websocket.ErrHandshake, &h2.StatusError{Status: code, Header: hdr})
	}
	var retry *RetryLaterError
	for _, tc := range []struct {
		name  string
		err   error
		check func(error) bool
	}{
		{"403", status(403), func(err error) bool { return errors.Is(err, ErrDeregistered) }},
		{"499", status(499), func(err error) bool { return errors.Is(err, ErrExpired) }},
		{"retry-after", status(403, h2.HeaderField{Name: "retry-after", Value: "17"}), func(err error) bool {
			return errors.As(err, &retry) && retry.After == 17*time.Second && !errors.Is(err, ErrDeregistered)
		}},
		{"bad retry-after", status(429, h2.HeaderField{Name: "retry-after", Value: "Wed, 21 Oct 2026 07:28:00 GMT"}), func(err error) bool {
			return !errors.As(err, &retry) && errors.Is(err, websocket.ErrHandshake)
		}},
		{"500", status(500), func(err error) bool {
			return errors.Is(err, websocket.ErrHandshake) && !errors.Is(err, ErrDeregistered) && !errors.Is(err, ErrExpired)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubDial(t, nil, nil, tc.err)
			_, err := Dial(context.Background(), Endpoint{})
			if err == nil || !tc.check(err) {
				t.Errorf("Dial: %v", err)
			}
		})
	}
}
