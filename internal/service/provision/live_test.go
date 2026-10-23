package provision

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

// TestLiveStagingAddress opens the provisioning socket on staging and
// waits for its address. Opt in with SIGNAL_LIVE_STAGING=1.
func TestLiveStagingAddress(t *testing.T) {
	if os.Getenv("SIGNAL_LIVE_STAGING") != "1" {
		t.Skip("set SIGNAL_LIVE_STAGING=1 to contact staging")
	}
	proxy, err := websocket.EnvProxy("https://" + StagingHost)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := Open(ctx, Endpoint{Proxy: proxy})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	addr, err := s.Address(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.URLEncoding.DecodeString(addr)
	if err != nil || len(raw) != 16 {
		t.Errorf("address %q is not 16 bytes of padded URL-safe base64 (%v)", addr, err)
	}
	k, _ := curve25519.GeneratePrivateKey(rand.Reader)
	if _, _, err := ParseLinkURL(LinkURL(addr, k.PublicKey())); err != nil {
		t.Error(err)
	}
	t.Logf("got a %d-character address", len(addr))
}
