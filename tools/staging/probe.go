// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// onlyAccount opens the single account under <data>/data (signal-cli's
// layout, as the CLI uses it).
func onlyAccount(data string) *store.Account {
	r := store.Open(filepath.Join(data, "data"))
	entries, err := r.Accounts()
	if err != nil || len(entries) != 1 {
		log.Fatalf("want exactly one account in %s (%v)", data, err)
	}
	a, err := r.Load(entries[0].Path)
	if err != nil {
		log.Fatal(err)
	}
	return a
}

// probe opens the authenticated chat socket, lists the queued envelopes
// until queue-empty, and does NOT acknowledge them, so nothing is lost.
func probe(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	fs.Parse(args)
	a := onlyAccount(*data)
	defer a.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := chat.Dial(ctx, chat.Endpoint{
		Proxy:          envProxy(chat.StagingHost).Proxy,
		Credentials:    &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password},
		ReceiveStories: true,
	})
	if err != nil {
		log.Fatalf("connect: %s", mask(err.Error()))
	}
	defer c.Close()
	fmt.Printf("connected as device %d; clock skew %v\n", a.Data.DeviceID, time.Since(c.ServerTime()).Round(time.Millisecond))
	for {
		r, err := c.Next(ctx)
		if err != nil {
			log.Fatalf("next: %v", err)
		}
		if r.Kind == chat.KindQueueEmpty {
			fmt.Println("queue empty")
			return
		}
		var env signalpb.ServiceEnvelope
		if err := proto.Unmarshal(r.Body, &env); err != nil {
			fmt.Printf("envelope: %d bytes, unparsable: %v\n", len(r.Body), err)
			continue
		}
		fmt.Printf("envelope: type %v, %d-byte content, server time %d (not acked)\n", env.GetType(), len(env.GetContent()), r.ServerTimestamp)
	}
}
