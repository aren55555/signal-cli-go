// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// TestOwnCaptureReplay decrypts, offline, envelopes the reference
// signal-cli 0.14.7 sent on staging to an account of ours. It is opt-in:
// SIGNAL_OWN_CAPTURE_DIR names a directory with
//
//	pre/        our data directory before the envelopes arrived
//	envelopes/  the envelopes, as saved by `tools/staging capture`
//	want.txt    one line per text the sender sent
//	kinds.txt   the kinds that must occur: sealed-message, sender-key, ...
//
// Captures hold live keys of staging test accounts and stay out of git
// (.data/ is ignored).
func TestOwnCaptureReplay(t *testing.T) {
	dir := os.Getenv("SIGNAL_OWN_CAPTURE_DIR")
	if dir == "" {
		t.Skip("SIGNAL_OWN_CAPTURE_DIR not set")
	}
	work := t.TempDir()
	if err := os.CopyFS(work, os.DirFS(filepath.Join(dir, "pre"))); err != nil {
		t.Fatal(err)
	}
	root := store.Open(filepath.Join(work, "data"))
	entries, err := root.Accounts()
	if err != nil || len(entries) != 1 {
		t.Fatalf("accounts: %v %d", err, len(entries))
	}
	a, err := root.Load(entries[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	cache := filepath.Join(a.Dir(), "msg-cache")
	if err := os.CopyFS(cache, os.DirFS(filepath.Join(dir, "envelopes"))); err != nil {
		t.Fatal(err)
	}
	var events []Event
	r := &Receiver{Acct: a, Decryptor: &Decryptor{Acct: a}, Sender: &Sender{Acct: a, Chat: offlineChat{}},
		OnEvent: func(e Event) { events = append(events, e) }, Log: func(string) {}}
	if err := r.RetryCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	var texts []string
	for _, e := range events {
		if e.Err != nil {
			t.Errorf("envelope %d: %v", e.Envelope.GetClientTimestamp(), e.Err)
			continue
		}
		m := e.Message
		var c signalpb.AppContent
		proto.Unmarshal(m.Content, &c)
		switch {
		case m.InnerType == envSenderKey:
			kinds["sender-key"] = true
		case m.Sealed && m.InnerType == envCiphertext:
			kinds["sealed-message"] = true
		case m.Sealed && m.InnerType == envPreKeyBundle:
			kinds["sealed-prekey"] = true
		case !m.Sealed && m.InnerType == envCiphertext:
			kinds["message"] = true
		}
		if c.GetStoryMessage() != nil {
			kinds["story"] = true
		}
		if c.SenderKeyDistributionMessage != nil {
			kinds["skdm"] = true
		}
		if b := c.GetDataMessage().GetBody(); b != "" {
			texts = append(texts, b)
		}
	}
	want, _ := os.ReadFile(filepath.Join(dir, "want.txt"))
	for _, w := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		if w != "" && !strings.Contains(strings.Join(texts, "\n"), w) {
			t.Errorf("text %q not decrypted", w)
		}
	}
	need, _ := os.ReadFile(filepath.Join(dir, "kinds.txt"))
	for _, k := range strings.Fields(string(need)) {
		if !kinds[k] {
			t.Errorf("no %s envelope (got %v)", k, kinds)
		}
	}
	t.Logf("%d envelopes, kinds %v", len(events), kinds)
}

// offlineChat answers everything without a network: receipts and the
// like go nowhere.
type offlineChat struct{}

func (offlineChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	return &chat.Response{Status: 503}, nil
}
