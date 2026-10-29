// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
)

// profile refetches a recipient's profile (versioned when its key is
// known) and prints the stored access mode.
func profile(args []string) {
	fs := flag.NewFlagSet("profile", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	to := fs.String("to", "", "recipient ACI")
	fs.Parse(args)
	a := onlyAccount(*data)
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := dialAuth(ctx, a, false)
	defer c.Close()
	p := (&manager.Sender{Acct: a, Chat: c}).RefreshProfile(ctx, *to)
	fmt.Printf("versioned=%v mode=%s capabilities=%v\n", len(a.Recipients[*to].ProfileKey) == 32, p.AccessMode, p.Capabilities)
}
