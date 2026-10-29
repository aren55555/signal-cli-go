// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

// TestLiveStagingUnauthenticated opens the unauthenticated chat socket on
// Signal's staging service and makes one keep-alive request. Opt in with
// SIGNAL_LIVE_STAGING=1; it never touches production.
func TestLiveStagingUnauthenticated(t *testing.T) {
	if os.Getenv("SIGNAL_LIVE_STAGING") != "1" {
		t.Skip("set SIGNAL_LIVE_STAGING=1 to contact staging")
	}
	proxy, err := websocket.EnvProxy("https://" + StagingHost)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := Dial(ctx, Endpoint{Host: StagingHost, Proxy: proxy})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if st := c.ServerTime(); st.IsZero() {
		t.Error("no x-signal-timestamp")
	} else {
		t.Logf("server clock skew %v", time.Since(st).Round(time.Millisecond))
	}
	r, err := c.Request(ctx, "GET", "/v1/keepalive", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("keepalive: %d %q, %d-byte body", r.Status, r.Message, len(r.Body))
	if r.Status != 200 {
		t.Errorf("keepalive status %d", r.Status)
	}
}
