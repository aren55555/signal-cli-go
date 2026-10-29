// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// capture saves every queued envelope, undecrypted, to -out and to the
// account's msg-cache (so a later `receive` still processes it), then
// acks it. With a snapshot of the data directory taken first, the
// envelopes can be decrypted offline again and again (the replay test in
// internal/manager).
func capture(args []string) {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	out := fs.String("out", "", "directory for the captured envelopes")
	linger := fs.Duration("linger", 5*time.Second, "keep listening this long after queue-empty")
	fs.Parse(args)
	if *out == "" {
		log.Fatal("-out is required")
	}
	a := onlyAccount(*data)
	defer a.Close()
	cacheDir := filepath.Join(a.Dir(), "msg-cache")
	for _, d := range []string{*out, cacheDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			log.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c, err := chat.Dial(ctx, chat.Endpoint{
		Proxy:          envProxy(chat.StagingHost).Proxy,
		Credentials:    &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password},
		ReceiveStories: true,
		KeepAlive:      chat.KeepAlive,
	})
	if err != nil {
		log.Fatalf("connect: %s", mask(err.Error()))
	}
	defer c.Close()
	var stop <-chan time.Time
	n := 0
	for {
		wctx, wcancel := context.WithCancel(ctx)
		if stop != nil {
			go func(s <-chan time.Time) {
				select {
				case <-s:
					wcancel()
				case <-wctx.Done():
				}
			}(stop)
		}
		r, err := c.Next(wctx)
		wcancel()
		if err != nil {
			if stop != nil {
				fmt.Printf("captured %d envelopes\n", n)
				return
			}
			log.Fatalf("next: %v", err)
		}
		if r.Kind == chat.KindQueueEmpty {
			stop = time.After(*linger)
			continue
		}
		var env signalpb.ServiceEnvelope
		if err := proto.Unmarshal(r.Body, &env); err != nil {
			fmt.Printf("unparsable envelope (%v), not acked\n", err)
			continue
		}
		file := binary.BigEndian.AppendUint64(nil, r.ServerTimestamp)
		file = append(file, r.Body...)
		var id [4]byte
		rand.Read(id[:])
		name := fmt.Sprintf("%020d-%s.env", r.ServerTimestamp, hex.EncodeToString(id[:]))
		for _, d := range []string{*out, cacheDir} {
			if err := store.WriteFileAtomic(filepath.Join(d, name), file, 0o600); err != nil {
				log.Fatal(err)
			}
		}
		if err := r.Ack(); err != nil {
			log.Fatalf("ack: %v", err)
		}
		n++
		fmt.Printf("envelope %d: type %d, story %v, %d bytes\n", n, env.GetType(), env.GetStory(), len(r.Body))
	}
}
