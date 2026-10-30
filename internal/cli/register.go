// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/svr2"
	"os"
	"path/filepath"
	"time"

	"github.com/aren55555/signal-cli-go/internal/errcode"
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
			return "", errcode.AccountsFileLoad.Wrap(err)
		}
		entries = visible(g, entries)
		switch len(entries) {
		case 0:
			return "", errcode.NoLocalUsers
		case 1:
			account = entries[0].Number
		default:
			return "", errcode.MultipleUsers
		}
	}
	if !validNumber(account) {
		return "", errcode.InvalidAccountNumber
	}
	return account, nil
}

func atLine(prefix string, t time.Time) string {
	return fmt.Sprintf("%s %s", prefix, ts(uint64(t.UnixMilli())))
}

func (e *env) registrar(g *globals, n *network) *manager.Registrar {
	return &manager.Registrar{
		Root:     g.root(),
		Env:      n.env,
		REST:     n.rest(),
		Language: "en-US",
		Log:      func(s string) { fmt.Fprintln(e.stderr, "DEBUG RegistrationManagerImpl - "+s) },
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
		return errcode.AccountsListRead.Wrap(err)
	}
	err = e.registrar(g, n).Register(ctx, account, captcha, voice, reregister)
	var ce *manager.CaptchaRequiredError
	var rl *rest.RateLimitError
	var mu *manager.MethodUnavailableError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		next := ""
		if !ce.NextAt.IsZero() {
			next = "\n" + atLine("Next Captcha may be provided at", ce.NextAt)
		}
		if ce.Given {
			return errcode.CaptchaInvalid.Wrap(err, next)
		}
		return errcode.CaptchaRequired.Wrap(err, next)
	case errors.Is(err, manager.ErrCaptchaRejected), errors.Is(err, manager.ErrCaptchaInvalid):
		return errcode.CaptchaInvalid.Wrap(err, "")
	case errors.As(err, &rl):
		next := ""
		if rl.After > 0 {
			next = "\n" + atLine("Next attempt may be tried at", time.Now().Add(rl.After))
		}
		return errcode.RegisterRateLimited.Wrap(err, next)
	case errors.As(err, &mu):
		suffix := ""
		if mu.Voice {
			suffix = ": Before requesting voice verification you need to request SMS verification and wait a minute."
		}
		return errcode.RegisterMethodUnavailable.Wrap(err, suffix)
	}
	return errcode.RegisterFailed.Wrap(err, err, javaKind(err))
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
		return errcode.VerifyNoSession.Wrap(err)
	case errors.As(err, &locked):
		return errcode.PinLocked.Wrap(err, locked.TimeRemaining/3600000)
	case errors.As(err, &mismatch):
		return errcode.PinMismatch.Wrap(err, mismatch.Tries)
	case errors.Is(err, manager.ErrPinDataDeleted):
		return errcode.PinDataDeleted.Wrap(err)
	case errors.Is(err, svr2.ErrUnsupportedPIN):
		return errcode.PinUnsupported.Wrap(err)
	case errors.Is(err, manager.ErrRegLockStillFailed):
		return errcode.VerifyRegLockFailed.Wrap(err)
	case err != nil:
		return errcode.VerifyFailed.WrapText(err)
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
