// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
)

// groupprobe checks the zkgroup credentials against staging: today's auth
// credential, our own expiring profile-key credential, and (with -fetch
// MASTERKEYHEX) a group state fetch.
func groupprobe(args []string) {
	fs := flag.NewFlagSet("groupprobe", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	fs.Parse(args)
	a := onlyAccount(*data)
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := dialAuth(ctx, a, false)
	defer c.Close()
	gs := &manager.GroupsService{Acct: a, Chat: c,
		REST: rest.New(rest.Config{BaseURL: "https://storage-staging.signal.org", Proxy: envProxy("storage-staging.signal.org").Proxy})}
	var mk [32]byte
	mk[0] = 1
	g := zkgroup.DeriveGroupSecretParams(mk)
	_, err := gs.GetGroup(ctx, g)
	fmt.Printf("auth credential + GET of a nonexistent group: %v\n", err)
	cred, err := (&manager.Sender{Acct: a, Chat: c}).ProfileKeyCredential(ctx, a.Data.ACI)
	if err != nil {
		log.Fatalf("own profile key credential: %v", err)
	}
	fmt.Printf("own profile key credential, expires %s\n", time.Unix(int64(cred.Expiration), 0).UTC())
}
