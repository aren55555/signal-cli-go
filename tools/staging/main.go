// Command staging is a development harness for exercising signal-cli-go
// against Signal's staging service before the CLI exists. Account data
// goes under -data, which must be a gitignored scratch directory. Phone
// numbers are masked in all output. Staging only.
//
//	staging link  [-data DIR] [-name NAME] [-url-file FILE]
//	staging probe [-data DIR]
package main

import (
	"log"
	"os"
	"regexp"

	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

var digits = regexp.MustCompile(`\+?[0-9]{7,15}`)

func mask(s string) string { return digits.ReplaceAllString(s, "<NUMBER>") }

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal("usage: staging link|probe [flags]")
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "link":
		link(args)
	case "probe":
		probe(args)
	default:
		log.Fatalf("unknown command %q", cmd)
	}
}

func envProxy(host string) *websocket.Options {
	p, err := websocket.EnvProxy("https://" + host)
	if err != nil {
		log.Fatal(err)
	}
	return &websocket.Options{Proxy: p}
}
