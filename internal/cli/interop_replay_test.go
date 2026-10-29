// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/interop"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// TestInteropReplay replays the captures of the live group interop runs
// (spec/testing/groups-interop.md §6) offline: each signal-cli-go device's
// data directory is restored from its pre-receive snapshot, the captured
// envelopes are fed to the receive loop with every network request
// failing, and the scenario message must render as it did live and as the
// reference device of the same account rendered it. Opt-in, since the
// captures hold live staging keys:
//
//	INTEROP_CAPTURES=.data/interop/run-20261029 go test ./internal/cli -run TestInteropReplay
func TestInteropReplay(t *testing.T) {
	root := os.Getenv("INTEROP_CAPTURES")
	if root == "" {
		t.Skip("INTEROP_CAPTURES not set")
	}
	scenarios, _ := filepath.Glob(filepath.Join(root, "*", "envelopes"))
	if len(scenarios) == 0 {
		t.Fatalf("no captures under %s", root)
	}
	partner := map[string]string{"A2": "A1", "A3": "A1", "B1": "B2", "C2": "C1"}
	replayed := 0
	for _, envs := range scenarios {
		scn := filepath.Dir(envs)
		tsb, err := os.ReadFile(filepath.Join(scn, "ts"))
		if err != nil {
			continue
		}
		ts := strings.TrimSpace(string(tsb))
		devs, _ := os.ReadDir(envs)
		for _, d := range devs {
			dev := d.Name()
			t.Run(filepath.Base(scn)+"/"+dev, func(t *testing.T) {
				if why, err := os.ReadFile(filepath.Join(scn, "skip-"+dev)); err == nil {
					t.Skip(strings.TrimSpace(string(why)))
				}
				out, isJSON := recorded(scn, dev)
				if out == nil {
					t.Skip("no recorded output")
				}
				live := interop.Block(out, isJSON, ts)
				if live == "" {
					t.Skip("the device did not render the scenario message live")
				}
				// The target is the reference device of the same account when
				// it rendered the message; otherwise our own live output.
				want, against := live, "our live rendering"
				if p, ok := partner[dev]; ok {
					if ref, _ := recorded(scn, p); ref != nil {
						if r := interop.Block(ref, isJSON, ts); r != "" {
							want, against = r, "the reference device "+p
						}
					}
				}
				got := interop.Block(replay(t, filepath.Join(scn, "pre", dev), filepath.Join(envs, dev), isJSON), isJSON, ts)
				if got != want {
					if strings.Contains(got, "Name: null") && !strings.Contains(want, "Name: null") {
						t.Skip("the live device fetched the group state from the server; offline replay cannot")
					}
					t.Errorf("replay differs from %s:\n got: %s\nwant: %s", against, got, want)
				}
				replayed++
			})
		}
	}
	t.Logf("%d device captures replayed", replayed)
}

func recorded(scn, dev string) ([]byte, bool) {
	if b, err := os.ReadFile(filepath.Join(scn, "out", dev+".json")); err == nil {
		return b, true
	}
	b, err := os.ReadFile(filepath.Join(scn, "out", dev+".txt"))
	if err != nil {
		return nil, false
	}
	return b, false
}

// offline answers every request with an error.
type offline struct{}

func (offline) Request(context.Context, string, string, []chat.Header, []byte) (*chat.Response, error) {
	return nil, errors.New("offline replay")
}

// captured feeds envelope files in name order, then queue-empty, then ends
// the loop.
type captured struct {
	files []string
	i     int
	empty bool
}

func (c *captured) Next(ctx context.Context) (*chat.ServerRequest, error) {
	if c.i < len(c.files) {
		b, err := os.ReadFile(c.files[c.i])
		c.i++
		if err != nil || len(b) < 8 {
			return nil, fmt.Errorf("capture %s: %v", c.files[c.i-1], err)
		}
		return chat.NewDelivery(chat.KindEnvelope, b[8:], binary.BigEndian.Uint64(b), func() error { return nil }), nil
	}
	if !c.empty {
		c.empty = true
		return chat.NewDelivery(chat.KindQueueEmpty, nil, 0, nil), nil
	}
	return nil, context.DeadlineExceeded
}

func replay(t *testing.T, pre, envs string, isJSON bool) []byte {
	dir := t.TempDir()
	if err := copyTree(pre, dir); err != nil {
		t.Fatal(err)
	}
	root := store.Open(filepath.Join(dir, "data"))
	idx, err := root.Accounts()
	if err != nil || len(idx) == 0 {
		t.Fatalf("snapshot has no account: %v", err)
	}
	a, err := root.Find(idx[0].Number)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	files, _ := filepath.Glob(filepath.Join(envs, "*.env"))
	sortByArrival(files)
	var buf bytes.Buffer
	g := &globals{dataDir: dir, account: idx[0].Number, serviceEnv: "staging", output: "plain-text"}
	if isJSON {
		g.output = "json"
	}
	e := &env{stdout: &buf, stderr: io.Discard, getenv: func(string) string { return "" }}
	n := &network{proxy: &url.URL{Scheme: "http", Host: "127.0.0.1:9"}} // nothing gets out
	r := newReceiver(e, g, n, a, offline{}, offline{}, receiveOpts{}, func(rc *RenderContext, ev manager.Event) {
		if g.json() {
			fmt.Fprintln(&buf, RenderJSON(rc, ev))
		} else {
			io.WriteString(&buf, RenderPlain(rc, ev))
		}
	})
	if err := r.Run(context.Background(), &captured{files: files}, 0, 0); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() || d.Name() == "lock" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
}

// sortByArrival orders capture files as the server received the
// envelopes. Envelopes of one receive share their delivery time, the
// file-name prefix, so the envelope's server timestamp decides, then the
// name (newer captures carry an arrival counter there).
func sortByArrival(files []string) {
	received := map[string]uint64{}
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && len(b) > 8 {
			var env signalpb.ServiceEnvelope
			if proto.Unmarshal(b[8:], &env) == nil {
				received[f] = env.GetServerTimestamp()
			}
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		if received[files[i]] != received[files[j]] {
			return received[files[i]] < received[files[j]]
		}
		return files[i] < files[j]
	})
}
