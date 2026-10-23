package cli

import (
	"errors"
	"regexp"

	"github.com/aren55555/signal-cli-go/internal/store"
)

// cliError is a failure reported as one stderr message with an exit code
// (spec/surface/cli/index.md §6).
type cliError struct {
	msg  string
	code int
}

func (e *cliError) Error() string { return e.msg }

func userError(msg string) error { return &cliError{msg: msg, code: 1} }

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
func selectAccount(r *store.Root, account string) (*store.Account, error) {
	entries, err := r.Accounts()
	if err != nil {
		return nil, &cliError{msg: "Failed to load local accounts file", code: 3}
	}
	if account == "" {
		switch len(entries) {
		case 0:
			return nil, userError("No local users found, you first need to register or link an account")
		case 1:
			account = entries[0].Number
		default:
			return nil, userError("Multiple users found, you need to specify an account (phone number) with -a")
		}
	}
	if !validNumber(account) && !uuidRe.MatchString(account) {
		return nil, userError("Invalid account (phone number), make sure you include the country code.")
	}
	notRegistered := userError("User " + account + " is not registered.")
	for _, e := range entries {
		if e.Number != account && e.UUID != account {
			continue
		}
		a, err := r.Load(e.Path)
		switch {
		case errors.Is(err, store.ErrNotOurs):
			return nil, notRegistered
		case errors.Is(err, store.ErrLocked):
			return nil, &cliError{msg: "Error loading state file for user " + account + ": " + err.Error() + " (OverlappingFileLockException)", code: 2}
		case err != nil:
			return nil, &cliError{msg: "Error loading state file for user " + account + ": " + err.Error() + " (IOException)", code: 2}
		}
		if !a.Data.Registered {
			a.Close()
			return nil, notRegistered
		}
		return a, nil
	}
	return nil, notRegistered
}
