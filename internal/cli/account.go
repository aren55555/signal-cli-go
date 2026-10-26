package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/store"
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
		if len(numbers) > 0 {
			return ioError("Unable to check if users are registered: phone number lookup (CDSI) is not implemented in signal-cli-go yet (UnsupportedOperationException)")
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		var items []string
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
