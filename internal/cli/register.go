package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/svr2"
	"os"
	"path/filepath"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// registrationAccount applies index.md §2's rules for registration
// commands: a phone number, given or the single local account.
func registrationAccount(g *globals) (string, error) {
	account := g.account
	if account == "" {
		entries, err := g.root().Accounts()
		if err != nil {
			return "", &cliError{msg: "Failed to load local accounts file", code: 3}
		}
		switch len(entries) {
		case 0:
			return "", userError("No local users found, you first need to register or link an account")
		case 1:
			account = entries[0].Number
		default:
			return "", userError("Multiple users found, you need to specify an account (phone number) with -a")
		}
	}
	if !validNumber(account) {
		return "", userError("Invalid account (phone number), make sure you include the country code.")
	}
	return account, nil
}

func atLine(prefix string, t time.Time) string {
	return fmt.Sprintf("%s %s", prefix, ts(uint64(t.UnixMilli())))
}

func (e *env) registrar(g *globals, n *network) *manager.Registrar {
	return &manager.Registrar{
		Root:     g.root(),
		REST:     rest.New(rest.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent}),
		Language: "en-US",
		Log: func(s string) {
			if g.verbose > 0 {
				fmt.Fprintln(e.stderr, "DEBUG RegistrationManager - "+s)
			}
		},
		PostRegister: func(ctx context.Context, a *store.Account) error {
			c, err := n.dialChat(ctx, a, false)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := manager.RefreshPreKeys(ctx, a, c, time.Now()); err != nil {
				return err
			}
			return manager.UploadInitialProfile(ctx, a, c)
		},
		DialSVR2: n.svr2Dialer(),
		Info:     func(s string) { fmt.Fprintln(e.stderr, "INFO  RegistrationManagerImpl - "+s) },
		Reactivate: func(ctx context.Context, a *store.Account) error {
			c, err := n.dialChat(ctx, a, false)
			if err != nil {
				return err
			}
			defer c.Close()
			return (&manager.Sender{Acct: a, Chat: c}).UpdateAccountAttributes(ctx)
		},
	}
}

// runRegister is `register` (spec/surface/cli/register.md).
func runRegister(ctx context.Context, e *env, g *globals, voice bool, captcha string, reregister bool) error {
	account, err := registrationAccount(g)
	if err != nil {
		return err
	}
	n, err := e.network(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(g.dataDir, "data"), 0o700); err != nil {
		return ioError("Failed to read local accounts list")
	}
	err = e.registrar(g, n).Register(ctx, account, captcha, voice, reregister)
	var ce *manager.CaptchaRequiredError
	var rl *rest.RateLimitError
	var mu *manager.MethodUnavailableError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		msg := "Captcha required for verification, use --captcha CAPTCHA\n" +
			"To get the token, go to https://signalcaptchas.org/registration/generate.html\n" +
			"After solving the captcha, right-click on the \"Open Signal\" link and copy the link."
		if ce.Given {
			msg = "Invalid captcha given."
		}
		if !ce.NextAt.IsZero() {
			msg += "\n" + atLine("Next Captcha may be provided at", ce.NextAt)
		}
		return userError(msg)
	case errors.Is(err, manager.ErrCaptchaRejected), errors.Is(err, manager.ErrCaptchaInvalid):
		return userError("Invalid captcha given.")
	case errors.As(err, &rl):
		msg := "Rate limit reached"
		if rl.After > 0 {
			msg += "\n" + atLine("Next attempt may be tried at", time.Now().Add(rl.After))
		}
		return &cliError{msg: msg, code: 5}
	case errors.As(err, &mu):
		msg := "Failed to register: Verification method not available"
		if mu.Voice {
			msg += ": Before requesting voice verification you need to request SMS verification and wait a minute."
		}
		return userError(msg)
	}
	return ioError("Failed to register: %v (%s)", err, javaKind(err))
}

// runVerify is `verify CODE` (spec/surface/cli/verify.md).
func runVerify(ctx context.Context, e *env, g *globals, code, pin string) error {
	account, err := registrationAccount(g)
	if err != nil {
		return err
	}
	n, err := e.network(g)
	if err != nil {
		return err
	}
	a, err := e.registrar(g, n).Verify(ctx, account, code, pin)
	if a != nil {
		defer a.Close()
	}
	var locked *manager.LockedError
	var mismatch *manager.PinMismatchError
	switch {
	case errors.Is(err, store.ErrNoAccount):
		return &cliError{msg: "Verify error: " + manager.ErrNoSession.Error(), code: 3}
	case errors.As(err, &locked), errors.As(err, &mismatch), errors.Is(err, manager.ErrPinDataDeleted), errors.Is(err, svr2.ErrUnsupportedPIN):
		return userError(err.Error())
	case errors.Is(err, manager.ErrRegLockStillFailed):
		return &cliError{msg: "Exception in thread \"main\" java.lang.AssertionError: " + err.Error(), code: 2}
	case err != nil:
		return &cliError{msg: "Verify error: " + err.Error(), code: 3}
	}
	return nil
}

// javaKind names an error kind the way the reference's messages do.
func javaKind(err error) string {
	var se *rest.StatusError
	if errors.As(err, &se) {
		return "NonSuccessfulResponseCodeException"
	}
	return "IOException"
}
