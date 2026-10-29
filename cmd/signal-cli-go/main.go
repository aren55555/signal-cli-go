// SPDX-License-Identifier: GPL-3.0-or-later

// Command signal-cli-go is a clean-room, standard-library-only
// reimplementation of signal-cli. See PLAN.md for status.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // log timestamps use the local zone (logging.md §2.2)

	"github.com/aren55555/signal-cli-go/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(code)
}
