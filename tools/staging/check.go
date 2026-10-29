// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
)

func dialAuth(ctx context.Context, a *store.Account, keepAlive bool) *chat.Conn {
	ep := chat.Endpoint{
		Proxy:          envProxy(chat.StagingHost).Proxy,
		Credentials:    &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password},
		ReceiveStories: true,
	}
	if keepAlive {
		ep.KeepAlive = chat.KeepAlive
	}
	c, err := chat.Dial(ctx, ep)
	if err != nil {
		log.Fatalf("connect: %s", mask(err.Error()))
	}
	return c
}

// check runs the start-up account check: attributes and prekey refresh.
func check(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	fs.Parse(args)
	a := onlyAccount(*data)
	defer a.Close()
	if a.Data.DeviceID != 1 && a.Data.DeviceName == "" {
		// Linked before the store kept the encrypted name.
		pub, _ := curve25519.ParsePublicKey(a.Data.ACIIdentity.Public)
		enc, err := protocol.EncryptDeviceName(rand.Reader, pub, "cli")
		if err != nil {
			log.Fatal(err)
		}
		a.Data.DeviceName = base64.RawStdEncoding.EncodeToString(enc)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := dialAuth(ctx, a, false)
	defer c.Close()
	if err := manager.CheckAccount(ctx, a, c, time.Now()); err != nil {
		log.Fatalf("check: %v", err)
	}
	fmt.Printf("ok: %d ACI one-time EC keys, %d Kyber stored\n", len(a.Keys.ACI.PreKeys), len(a.Keys.ACI.KyberPreKeys))
}
