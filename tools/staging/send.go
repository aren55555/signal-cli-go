package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
)

// send sends a text message to an ACI (no CDSI yet), unsealed, with the
// sync transcript to our other devices.
func send(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	to := fs.String("to", "", "recipient ACI")
	msg := fs.String("m", "", "message text")
	fs.Parse(args)
	if *to == "" {
		log.Fatal("-to is required")
	}
	a := onlyAccount(*data)
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := chat.Dial(ctx, chat.Endpoint{
		Proxy:       envProxy(chat.StagingHost).Proxy,
		Credentials: &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password},
	})
	if err != nil {
		log.Fatalf("connect: %s", mask(err.Error()))
	}
	defer c.Close()
	u, err := chat.Dial(ctx, chat.Endpoint{Proxy: envProxy(chat.StagingHost).Proxy})
	if err != nil {
		log.Fatalf("connect (unauthenticated): %v", err)
	}
	defer u.Close()
	sender := &manager.Sender{Acct: a, Chat: c, Unauth: u}
	ts, err := sender.SendText(ctx, *to, *msg)
	if err != nil {
		log.Fatalf("send: %s", mask(err.Error()))
	}
	sealed := "unsealed"
	if p := a.Recipients[*to].Profile; p != nil && p.AccessMode != "UNKNOWN" && p.AccessMode != "DISABLED" {
		sealed = "sealed (access " + p.AccessMode + ")"
	}
	fmt.Println(ts, sealed)
}
