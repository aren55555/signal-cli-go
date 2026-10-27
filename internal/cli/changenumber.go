package cli

import (
	"context"
	"errors"
	"time"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// runStartChangeNumber is `startChangeNumber` (change-number.md §2).
func runStartChangeNumber(ctx context.Context, e *env, g *globals, number string, voice bool, captcha string) error {
	return withAccount(g, func(a *store.Account) error {
		if a.Data.DeviceID != 1 {
			return errcode.NotPrimaryDevice
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		n, err := e.network(g)
		if err != nil {
			return err
		}
		err = e.registrar(g, n).StartChangeNumber(ctx, a, number, captcha, voice)
		return changeNumberError(err)
	})
}

// changeNumberError maps the session flow's errors like register's,
// with server failures as "Failed to change number" (§2.3).
func changeNumberError(err error) error {
	var cn *manager.ChangeNumberError
	var ce *manager.CaptchaRequiredError
	var rl *rest.RateLimitError
	var mu *manager.MethodUnavailableError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &cn):
		return errcode.ChangeNumberFailed.Wrap(err, cn.Message, cn.Class)
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
	return errcode.ChangeNumberFailed.Wrap(err, err, javaKind(err))
}

// runFinishChangeNumber is `finishChangeNumber` (change-number.md §3).
// The registration-lock PIN has no effect in the reference (§3.9).
func runFinishChangeNumber(ctx context.Context, e *env, g *globals, number, code string) error {
	return withAccount(g, func(a *store.Account) error {
		if a.Data.DeviceID != 1 {
			return errcode.NotPrimaryDevice
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		n, err := e.network(g)
		if err != nil {
			return err
		}
		rc := rest.New(rest.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent})
		return changeNumberError(s.sender.FinishChangeNumber(ctx, rc, number, code))
	})
}

// runSubmitChallenge is `submitRateLimitChallenge`
// (rate-limit-challenge.md §2).
func runSubmitChallenge(ctx context.Context, e *env, g *globals, challenge, captcha string) error {
	return withAccount(g, func(a *store.Account) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if err := s.sender.SubmitChallenge(ctx, challenge, captcha); err != nil {
			return errcode.SubmitChallengeFailed.WrapText(err)
		}
		return nil
	})
}
