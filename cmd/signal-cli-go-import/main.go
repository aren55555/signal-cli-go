// SPDX-License-Identifier: GPL-3.0-or-later

// Command signal-cli-go-import copies accounts from a signal-cli 0.14.7
// data directory into a signal-cli-go data directory. The source is only
// read. This is the one command that links a SQLite driver
// (modernc.org/sqlite, pure Go); signal-cli-go itself does not.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/importer"
	_ "modernc.org/sqlite"
)

type numbers []string

func (n *numbers) String() string     { return strings.Join(*n, ",") }
func (n *numbers) Set(v string) error { *n = append(*n, v); return nil }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv)) }

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("signal-cli-go-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", defaultDataDir(getenv), "signal-cli data directory to read")
	to := fs.String("to", "", "signal-cli-go data directory to write (required, must differ from -from)")
	var accounts numbers
	fs.Var(&accounts, "a", "import only this account number (repeatable; default all)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: signal-cli-go-import [-from DIR] -to DIR [-a NUMBER]...\n\n"+
			"Copies signal-cli 0.14.7 accounts into a signal-cli-go data directory.\n"+
			"Stop signal-cli first; the source directory is never modified.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *to == "" || fs.NArg() > 0 {
		fs.Usage()
		return 2
	}
	results, err := importer.Import(importer.Options{Source: *from, Dest: *to, Numbers: accounts, Open: openSQLite})
	failed := false
	for _, r := range results {
		if r.Err != nil {
			failed = true
			fmt.Fprintf(stderr, "%s: not imported: %v\n", r.Number, r.Err)
			continue
		}
		fmt.Fprintf(stdout, "%s: imported\n", r.Number)
		for _, s := range r.Skipped {
			fmt.Fprintf(stdout, "  not carried over: %s\n", s)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Fprintln(stderr, "no accounts to import")
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

// openSQLite opens the importer's private copy of a database.
func openSQLite(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// defaultDataDir is signal-cli's default: $XDG_DATA_HOME/signal-cli, or
// ~/.local/share/signal-cli (spec/surface/cli/index.md §6).
func defaultDataDir(getenv func(string) string) string {
	if x := getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "signal-cli")
	}
	return filepath.Join(getenv("HOME"), ".local", "share", "signal-cli")
}
