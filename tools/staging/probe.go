package main

import (
	"context"
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

// onlyAccount opens the single account in a data directory.
func onlyAccount(data string) *store.Account {
	entries, err := os.ReadDir(data)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(data, e.Name(), "account.json")); err == nil {
				a, err := store.Open(data).Load(e.Name())
				if err != nil {
					log.Fatal(err)
				}
				return a
			}
		}
	}
	log.Fatalf("no account in %s", data)
	return nil
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
