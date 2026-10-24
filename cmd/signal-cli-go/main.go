// Command signal-cli-go is a clean-room, standard-library-only
// reimplementation of signal-cli. See PLAN.md for status.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/aren55555/signal-cli-go/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(code)
}
