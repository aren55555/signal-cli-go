package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// link links signal-cli-go as a secondary device of an existing account:
// it prints the sgnl://linkdevice URL (also to -url-file), waits for the
// primary and stores the account.
func link(args []string) {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory (gitignored scratch)")
	name := fs.String("name", "cli", "device name")
	urlFile := fs.String("url-file", "", "also write the link URL here")
	fs.Parse(args)

	proxy := envProxy(provision.StagingHost).Proxy
	l := &manager.Linker{
		Root: store.Open(*data),
		REST: rest.New(rest.Config{Proxy: proxy}),
		OpenSocket: func(ctx context.Context) (*provision.Session, error) {
			return provision.Open(ctx, provision.Endpoint{Proxy: proxy})
		},
		DeviceName: *name,
		OnURL: func(u string) {
			fmt.Println(u)
			if *urlFile != "" {
				if err := os.WriteFile(*urlFile, []byte(u+"\n"), 0o600); err != nil {
					log.Fatal(err)
				}
			}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	acct, err := l.Link(ctx)
	if err != nil {
		log.Fatalf("link failed: %s", mask(err.Error()))
	}
	defer acct.Close()
	fmt.Printf("Associated with: <NUMBER> (device %d)\n", acct.Data.DeviceID)
}
