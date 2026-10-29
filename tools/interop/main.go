// SPDX-License-Identifier: GPL-3.0-or-later

// Command interop holds the helpers of the live group interop runs
// (spec/testing/groups-interop.md): store edits on signal-cli-go data
// directories for the S3 and S5 faults, and the extraction and
// normalization of receive output for the rendering comparison. The
// driver is run.sh beside it. Not a shipped command.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/interop"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch args := os.Args[2:]; os.Args[1] {
	case "group-id":
		err = withAccount(args, 3, func(a *store.Account) error {
			for _, g := range a.Groups {
				if st := manager.StateOf(&g); st != nil && st.Title == args[2] {
					fmt.Println(base64.StdEncoding.EncodeToString(g.GroupID))
					return nil
				}
			}
			return fmt.Errorf("no group titled %q", args[2])
		})
	case "expire-endorsements":
		err = withGroup(args, 4, func(a *store.Account, g *store.GroupRecord) error {
			d, err := time.ParseDuration(args[3])
			if err != nil {
				return err
			}
			g.EndorsementExpiration = time.Now().Add(d).UnixMilli()
			return a.SaveGroups()
		})
	case "drop-endorsement":
		err = withGroup(args, 4, func(a *store.Account, g *store.GroupRecord) error {
			if _, ok := g.Endorsements[args[3]]; !ok {
				return fmt.Errorf("no endorsement for %s", args[3])
			}
			delete(g.Endorsements, args[3])
			// Keep the refresh from bringing it back: a far expiry means the
			// stored set counts as usable (groups.md §12.2 step 3).
			return a.SaveGroups()
		})
	case "drop-sender-keys":
		err = withAccount(args, 3, func(a *store.Account) error {
			n := 0
			for k := range a.SenderKeys.Records {
				if strings.HasPrefix(k, args[2]+".") {
					delete(a.SenderKeys.Records, k)
					delete(a.SenderKeys.Created, k)
					n++
				}
			}
			if n == 0 {
				return fmt.Errorf("no sender keys received from %s", args[2])
			}
			fmt.Printf("dropped %d sender key record(s)\n", n)
			return a.SaveSenderKeys()
		})
	case "mark-shared":
		err = withGroup(args, 4, func(a *store.Account, g *store.GroupRecord) error {
			// Start a new sender key for the group, so the next send hands
			// it out, and pretend this member already has it.
			own := manager.SenderKeyName(a.Data.ACI, uint32(a.Data.DeviceID), mustDist(g.DistributionID))
			delete(a.SenderKeys.Records, own)
			delete(a.SenderKeys.Created, own)
			shared := map[string]int64{}
			a.SenderKeys.Shared[g.DistributionID] = shared
			n := 0
			for k := range a.Sessions {
				if rest, ok := strings.CutPrefix(k, "aci:"+args[3]+"."); ok {
					shared[args[3]+"."+rest] = time.Now().UnixMilli()
					n++
				}
			}
			if n == 0 {
				return fmt.Errorf("no sessions with %s", args[3])
			}
			fmt.Printf("marked %d device(s) as holding our key\n", n)
			return a.SaveSenderKeys()
		})
	case "endorsement-expiry":
		err = withGroup(args, 3, func(a *store.Account, g *store.GroupRecord) error {
			fmt.Println(g.EndorsementExpiration, len(g.Endorsements))
			return nil
		})
	case "aci":
		err = withAccount(args, 2, func(a *store.Account) error { fmt.Println(a.Data.ACI); return nil })
	case "block":
		if len(args) != 3 {
			usage()
		}
		err = block(args[0], args[1], args[2])
	case "paths":
		if len(args) != 2 {
			usage()
		}
		err = paths(args[0], args[1])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "interop:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: interop CMD ARGS
  group-id DATADIR NUMBER TITLE
  expire-endorsements DATADIR NUMBER GROUPID DURATION
  drop-endorsement DATADIR NUMBER GROUPID MEMBER_ACI
  drop-sender-keys DATADIR NUMBER SENDER_SERVICE_ID
  mark-shared DATADIR NUMBER GROUPID MEMBER_ACI   new own sender key; member marked as holding it
  endorsement-expiry DATADIR NUMBER GROUPID   prints expiry (ms) and count
  aci DATADIR NUMBER
  block OUTPUT_FILE plain|json TIMESTAMP   normalized output for one envelope
  paths STDERR_LOG TIMESTAMP              Envelope path lines for one message
`)
	os.Exit(2)
}

func withAccount(args []string, n int, fn func(*store.Account) error) error {
	if len(args) != n {
		usage()
	}
	a, err := store.Open(filepath.Join(args[0], "data")).Find(args[1])
	if err != nil {
		return err
	}
	defer a.Close()
	return fn(a)
}

func withGroup(args []string, n int, fn func(*store.Account, *store.GroupRecord) error) error {
	return withAccount(args, n, func(a *store.Account) error {
		id, err := base64.StdEncoding.DecodeString(args[2])
		if err != nil {
			return err
		}
		g := a.Group(id)
		if g == nil {
			return fmt.Errorf("no group %s", args[2])
		}
		return fn(a, g)
	})
}

// block prints the normalized output for the envelopes whose timestamp is
// ts: plain-text blocks start at "Envelope from:" and end at a blank line
// at the top level; JSON output is one object per line.
func block(file, format, ts string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if format != "json" && format != "plain" {
		usage()
	}
	out := interop.Block(b, format == "json", ts)
	if out == "" {
		return fmt.Errorf("no envelope with timestamp %s in %s", ts, file)
	}
	fmt.Println(out)
	return nil
}

// paths prints the "Envelope path:" lines of a -v log for timestamp ts.
func paths(file, ts string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := strconv.ParseUint(ts, 10, 64); err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if l := sc.Text(); strings.Contains(l, "Envelope path: ") && strings.HasSuffix(l, ", timestamp "+ts) {
			fmt.Println(l[strings.Index(l, "Envelope path: "):])
		}
	}
	return sc.Err()
}

func mustDist(s string) protocol.DistributionID {
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(b) != 16 {
		fmt.Fprintln(os.Stderr, "interop: bad distribution id", s)
		os.Exit(1)
	}
	return protocol.DistributionID(b)
}
