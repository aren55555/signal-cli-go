// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/enclave"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/trust"
	"github.com/aren55555/signal-cli-go/internal/storage"
	"github.com/aren55555/signal-cli-go/internal/svr2"
)

// svrprobe restores the account's master key from both SVR2 enclaves with
// -pin and compares it with the local one (checks setPin end to end).
func svrprobe(args []string) {
	fs := flag.NewFlagSet("svrprobe", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	pin := fs.String("pin", "", "the PIN set with setPin")
	fs.Parse(args)
	a := onlyAccount(*data)
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := dialAuth(ctx, a, false)
	defer c.Close()
	dial := func(ctx context.Context, e svr2.Enclave, user, pass string) (cdsi.Channel, map[string][]byte, func(), error) {
		mr, _ := hex.DecodeString(e.MREnclave)
		conn, err := enclave.Dial(ctx, e.URL(), mr, enclave.DialOptions{Username: user, Password: pass, Roots: trust.RESTRoots(),
			Proxy: envProxy("svr2.staging.signal.org").Proxy})
		if err != nil {
			return nil, nil, nil, err
		}
		return conn, conn.Claims, func() { conn.Close() }, nil
	}
	r, err := c.Request(ctx, "GET", "/v2/svr/auth", nil, nil)
	if err != nil || r.Status != 200 {
		log.Fatalf("svr auth: %v", err)
	}
	var cred struct{ Username, Password string }
	if err := json.Unmarshal(r.Body, &cred); err != nil {
		log.Fatal(err)
	}
	s := &manager.Sender{Acct: a, Chat: c, DialSVR2: dial}
	m, err := s.RestoreMasterKey(ctx, cred.Username, cred.Password, *pin)
	if err != nil {
		log.Fatalf("restore: %v", err)
	}
	local := storage.MasterKey(a.Data.AccountEntropyPool)
	if len(a.Data.PinMasterKey) == 32 {
		local = [32]byte(a.Data.PinMasterKey)
	}
	fmt.Printf("restored master key matches local: %v\n", bytes.Equal(m[:], local[:]))
}
