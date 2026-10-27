package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/svr2"
	"github.com/aren55555/signal-cli-go/internal/username"
)

type updateAccountOpts struct {
	deviceName                           *string
	unrestricted, discoverable, numShare *bool
	username                             *string
	deleteUsername                       bool
}

// runUpdateAccount is `updateAccount` (updateAccount.md, usernames.md §7.1).
func runUpdateAccount(ctx context.Context, e *env, g *globals, o updateAccountOpts) error {
	return withAccount(g, func(a *store.Account) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if o.deviceName != nil {
			if err := s.sender.UpdateDeviceName(ctx, uint32(a.Data.DeviceID), *o.deviceName); err != nil {
				return ioError("UpdateAccount error: %v", err)
			}
		}
		if o.unrestricted != nil {
			a.Data.UnrestrictedUnidentifiedAccess = *o.unrestricted
		}
		if o.discoverable != nil {
			a.Data.Unlisted = !*o.discoverable
		}
		if o.numShare != nil {
			a.Data.PhoneNumberSharingMode = "NOBODY"
			if *o.numShare {
				a.Data.PhoneNumberSharingMode = "EVERYBODY"
			}
		}
		if err := a.Save(); err != nil {
			return ioError("UpdateAccount error: %v", err)
		}
		if err := s.sender.UpdateAccountAttributes(ctx); err != nil {
			return ioError("UpdateAccount error: %v", err)
		}
		switch {
		case o.username != nil:
			name, err := s.sender.SetUsername(ctx, *o.username)
			var ue *username.Error
			var se *manager.UsernameStatusError
			switch {
			case errors.As(err, &ue):
				return userError(fmt.Sprintf("Invalid username: %s (%s)", ue.Message, ue.Class))
			case errors.As(err, &se):
				return userError("Invalid username: Username is malformed or already taken")
			case err != nil:
				return ioError("Failed to set username: %v", err)
			}
			link := manager.UsernameLinkURL(a)
			if g.json() {
				obj := &jsonObject{}
				obj.add("username", name)
				if link != "" {
					obj.add("usernameLink", link)
				}
				fmt.Fprintln(e.stdout, obj.String())
			} else {
				if link == "" {
					link = "-"
				}
				fmt.Fprintf(e.stdout, "Your new username: %s (%s)\n", name, link)
			}
		case o.deleteUsername:
			if err := s.sender.DeleteUsername(ctx); err != nil {
				return ioError("Failed to delete username: %v", err)
			}
		}
		return nil
	})
}

// runGetUserStatus is `getUserStatus` (getUserStatus.md, contacts.md §8).
// Phone numbers need CDSI, which comes later in milestone 7.
func runGetUserStatus(ctx context.Context, e *env, g *globals, numbers, usernames []string) error {
	return withAccount(g, func(a *store.Account) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		var items []string
		if len(numbers) > 0 {
			canon := map[string]string{}
			var valid []string
			for _, in := range numbers {
				if c, err := canonicalNumber(a, in); err == nil {
					canon[in] = c
					if !slices.Contains(valid, c) {
						valid = append(valid, c)
					}
				}
			}
			full := len(valid) > manager.MaxOneOffLookup
			req := valid
			if full {
				req = append(s.sender.StoredNumbers(), valid...)
			}
			res, err := s.sender.LookupNumbers(ctx, req, full)
			var ce *cdsi.CloseError
			switch {
			case errors.As(err, &ce) && ce.IsRateLimit():
				at := time.Now().Add(time.Duration(ce.RetryAfter) * time.Second).UnixMilli()
				return &cliError{msg: "Rate limit reached\nNext attempt may be tried at " + ts(uint64(at)), code: 5}
			case errors.As(err, &ce):
				return ioError("Unable to check if users are registered: %v (%s)", err, cdsiExceptionName(ce))
			case err != nil:
				return ioError("Unable to check if users are registered: %v (IOException)", err)
			}
			for _, in := range numbers {
				c, ok := canon[in]
				entry, found := res[c]
				uuid := entry.ACI
				if uuid == "" {
					uuid = entry.PNI
				}
				unrestricted := false
				if found {
					key := entry.ACI
					if key == "" {
						key = "PNI:" + entry.PNI
					}
					if p := s.sender.Profile(ctx, key); p != nil && p.AccessMode == store.AccessUnrestricted {
						unrestricted = true
					}
				}
				if g.json() {
					o := &jsonObject{}
					o.add("recipient", in)
					if ok {
						o.add("number", c)
					}
					o.add("uuid", strOrNull(uuid))
					o.add("isRegistered", found)
					items = append(items, o.String())
					continue
				}
				suffix := ""
				if unrestricted {
					suffix = " (unrestricted sealed sender)"
				}
				fmt.Fprintf(e.stdout, "%s: %t%s\n", in, found, suffix)
			}
		}
		for _, u := range usernames {
			k, ok, err := s.sender.ResolveUsername(ctx, u, true)
			var ue *username.Error
			switch {
			case errors.As(err, &ue):
				return &cliError{msg: fmt.Sprintf("Exception in thread \"main\" java.lang.AssertionError: org.signal.libsignal.usernames.%s: %s", ue.Class, ue.Message), code: 2}
			case err != nil:
				return ioError("Unable to check if users are registered: %v (IOException)", err)
			}
			aci := ""
			unrestricted := false
			if ok {
				aci = a.AddressOf(k).ACI
				if p := s.sender.Profile(ctx, k); p != nil && p.AccessMode == store.AccessUnrestricted {
					unrestricted = true
				}
			}
			if g.json() {
				o := &jsonObject{}
				o.add("recipient", u)
				o.add("username", u)
				o.add("uuid", strOrNull(aci))
				o.add("isRegistered", aci != "")
				items = append(items, o.String())
				continue
			}
			suffix := ""
			if unrestricted {
				suffix = " (unrestricted sealed sender)"
			}
			fmt.Fprintf(e.stdout, "%s: %t%s\n", u, aci != "", suffix)
		}
		if g.json() {
			fmt.Fprintln(e.stdout, "["+strings.Join(items, ",")+"]")
		}
		return nil
	})
}

// cdsiExceptionName is the Java class signal-cli reports (cdsi.md §4.5).
func cdsiExceptionName(ce *cdsi.CloseError) string {
	switch {
	case ce.IsInvalidToken():
		return "CdsiInvalidTokenException"
	case ce.IsInvalidArgument():
		return "CdsiInvalidArgumentException"
	}
	return "IOException"
}

// runSetPin is `setPin PIN` / `removePin` (svr.md §4.1, §4.2, §5.2).
func runSetPin(ctx context.Context, e *env, g *globals, pin string, remove bool) error {
	return withAccount(g, func(a *store.Account) error {
		if a.Data.DeviceID != 1 {
			return userError("This command doesn't work on linked devices.")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		s.sender.Log = func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  PinHelper - "+f+"\n", args...) }
		if remove {
			if err := s.sender.RemovePin(ctx); err != nil {
				return ioError("Remove pin error: %v", err)
			}
			return nil
		}
		err = s.sender.SetPin(ctx, pin)
		switch {
		case errors.Is(err, svr2.ErrUnsupportedPIN):
			return userError(err.Error())
		case err != nil:
			return ioError("Set pin error: %v", err)
		}
		return nil
	})
}
