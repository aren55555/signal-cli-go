// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/environment"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/store"
)

var (
	// E.164: '+', a non-zero digit, 7 to 15 digits in all. signal-cli asks
	// libphonenumber; this is the structural check (no per-country rules).
	e164Re = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

func validNumber(s string) bool { return e164Re.MatchString(s) }

// selectAccount applies index.md §2's rules for a local command: with no
// -a the single local account, else the named one (number or ACI), which
// must exist and be registered.
func selectAccount(g *globals, r *store.Root, account string) (*store.Account, error) {
	entries, err := r.Accounts()
	if err != nil {
		return nil, errcode.AccountsFileLoad.Wrap(err)
	}
	entries = visible(g, entries)
	if account == "" {
		switch len(entries) {
		case 0:
			return nil, errcode.NoLocalUsers
		case 1:
			account = entries[0].Number
		default:
			return nil, errcode.MultipleUsers
		}
	}
	if !validNumber(account) && !uuidRe.MatchString(account) {
		return nil, errcode.InvalidAccountNumber
	}
	notRegistered := errcode.UserNotRegistered.New(account)
	for _, e := range entries {
		if e.Number != account && e.UUID != account {
			continue
		}
		a, err := r.Load(e.Path)
		switch {
		case errors.Is(err, store.ErrNotOurs):
			return nil, notRegistered
		case errors.Is(err, store.ErrLocked):
			return nil, errcode.StateFileLocked.Wrap(err, account, err)
		case err != nil:
			return nil, errcode.StateFileIO.Wrap(err, account, err)
		}
		if !a.Data.Registered {
			a.Close()
			return nil, notRegistered
		}
		return a, nil
	}
	return nil, notRegistered
}

// runListAccounts is `listAccounts` (spec/surface/cli/listAccounts.md):
// the registered local accounts; others are skipped with a warning.
func runListAccounts(e *env, g *globals) error {
	if g.account != "" {
		return errcode.MultiAccountOnly
	}
	numbers, err := multiAccountStart(e, g)
	if err != nil {
		return err
	}
	if g.json() {
		items := make([]string, len(numbers))
		for i, n := range numbers {
			o := &jsonObject{}
			o.add("number", n)
			items[i] = o.String()
		}
		fmt.Fprintln(e.stdout, "["+strings.Join(items, ",")+"]")
		return nil
	}
	for _, n := range numbers {
		fmt.Fprintf(e.stdout, "Number: %s\n", n)
	}
	return nil
}

// runDeleteLocalAccountData is `deleteLocalAccountData`
// (deleteLocalAccountData.md): refused while the account is registered
// unless ignoreRegistered.
func runDeleteLocalAccountData(e *env, g *globals, ignoreRegistered bool) error {
	account, err := registrationAccount(g)
	if err != nil {
		return err
	}
	root := g.root()
	a, err := root.Find(account)
	switch {
	case errors.Is(err, store.ErrNoAccount), errors.Is(err, store.ErrNotOurs):
		return nil
	case err != nil:
		return errcode.DeletionFailed.WrapText(err)
	}
	if a.Data.Registered && !ignoreRegistered {
		a.Close()
		return errcode.NotDeletingRegistered
	}
	if err := root.Delete(a); err != nil {
		return errcode.DeletionFailed.WrapText(err)
	}
	return nil
}

// multiAccountStart is the start of a command running in multi-account
// mode (index.md, account selection rule 2): the data directory is
// created, and each local account that is not registered is skipped
// with a warning. It returns the registered accounts' numbers.
func multiAccountStart(e *env, g *globals) ([]string, error) {
	if err := os.MkdirAll(filepath.Join(g.dataDir, "data"), 0o700); err != nil {
		return nil, errcode.AccountsListRead.Wrap(err)
	}
	root := g.root()
	entries, err := root.Accounts()
	if err != nil {
		return nil, errcode.AccountsFileLoad.Wrap(err)
	}
	entries = visible(g, entries)
	var numbers []string
	for _, en := range entries {
		a, err := root.Load(en.Path)
		if err != nil || !a.Data.Registered {
			if a != nil {
				a.Close()
			}
			fmt.Fprintf(e.stderr, "WARN  MultiAccountManager - Ignoring %s: User is not registered. (NotRegisteredException)\n", en.Number)
			continue
		}
		numbers = append(numbers, a.Data.Number)
		a.Close()
	}
	return numbers, nil
}

// visible keeps the index entries of the run's environment, or with none
// recorded (data-dir §2.2): the others do not exist for this run.
func visible(g *globals, entries []store.IndexEntry) []store.IndexEntry {
	want := environment.AccountName(g.serviceEnv)
	var out []store.IndexEntry
	for _, e := range entries {
		if e.Environment == "" || e.Environment == want {
			out = append(out, e)
		}
	}
	return out
}
