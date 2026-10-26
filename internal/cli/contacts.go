package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/profile"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Contact commands (spec/service/contacts.md §6-§12).

type listContactsOpts struct {
	recipients         []string
	all                bool
	blocked            *bool
	name               *string
	detailed, internal bool
}

// recipientKey resolves a command argument to a recipient row key; ok is
// false for "not registered" (contacts.md §4.1).
func recipientKey(a *store.Account, arg string) (string, bool, error) {
	t, err := resolveRecipient(a, arg)
	if err != nil {
		return "", false, err
	}
	if t.aci == a.Data.ACI {
		if _, ok := a.Recipients[a.Data.ACI]; !ok {
			a.ResolveLowTrust(store.Address{ACI: a.Data.ACI})
		}
		return a.Data.ACI, true, nil
	}
	return a.ResolveLowTrust(store.Address{ACI: t.aci}), true, nil
}

// rowAddress is a row's address; the own row shows the account number.
func rowAddress(a *store.Account, key string) store.Address {
	ad := a.AddressOf(key)
	if key == a.Data.ACI && ad.Number == "" {
		ad.Number = a.Data.Number
	}
	return ad
}

func profileName(rc store.Recipient) string {
	if rc.Profile == nil {
		return ""
	}
	return profile.DisplayName(rc.Profile.GivenName, rc.Profile.FamilyName)
}

// selectContacts is §6.1 steps 2-4.
func selectContacts(a *store.Account, o listContactsOpts, only map[string]bool) []string {
	var out []string
	for _, k := range a.SortedRecipientKeys() {
		rc := a.Recipients[k]
		ad := rowAddress(a, k)
		if ad.Number == "" && ad.PNI == "" && ad.ACI == "" {
			continue
		}
		if !o.all && (rc.Unregistered || !rc.IsContact() || rc.Hidden) {
			continue
		}
		if o.blocked != nil && rc.Blocked != *o.blocked {
			continue
		}
		if only != nil && !only[k] {
			continue
		}
		if o.name != nil && *o.name != rc.ContactName() && *o.name != profileName(rc) {
			continue
		}
		out = append(out, k)
	}
	return out
}

// runListContacts is `listContacts` (contacts.md §6).
func runListContacts(ctx context.Context, e *env, g *globals, o listContactsOpts) error {
	return withAccount(g, func(a *store.Account) error {
		var only map[string]bool
		if len(o.recipients) > 0 {
			only = map[string]bool{}
			for _, r := range o.recipients {
				k, ok, err := recipientKey(a, r)
				if err != nil {
					return err
				}
				if ok {
					only[k] = true
				}
			}
			if len(only) == 0 {
				return printContacts(e, g, a, nil, o)
			}
		}
		rows := selectContacts(a, o, only)
		if len(rows) > 0 {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			s, err := openSession(ctx, e, g, a)
			if err != nil {
				return err
			}
			defer s.close()
			for _, k := range rows {
				switch {
				case only != nil:
					s.sender.RefreshProfile(ctx, k)
				case !a.Recipients[k].Unregistered:
					s.sender.Profile(ctx, k)
				}
			}
			rows = selectContacts(a, o, only)
		}
		return printContacts(e, g, a, rows, o)
	})
}

func orEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func printContacts(e *env, g *globals, a *store.Account, rows []string, o listContactsOpts) error {
	if g.json() {
		items := []string{}
		for _, k := range rows {
			items = append(items, contactJSON(a, k, o.internal).String())
		}
		fmt.Fprintln(e.stdout, "["+strings.Join(items, ",")+"]")
		return nil
	}
	for _, k := range rows {
		rc := a.Recipients[k]
		ad := rowAddress(a, k)
		exp := "disabled"
		if rc.Timer > 0 {
			exp = fmt.Sprintf("%ds", rc.Timer)
		}
		fmt.Fprintf(e.stdout, "Number: %s ACI: %s Name: %s Profile name: %s Username: %s Color: %s Blocked: %t Message expiration: %s\n",
			ad.Number, ad.ACI, rc.ContactName(), profileName(rc), ad.Username, orEmpty(rc.Color), rc.Blocked, exp)
		p := rc.Profile
		if p == nil {
			p = &store.Profile{}
		}
		if o.detailed {
			pni := ""
			if ad.PNI != "" {
				pni = "PNI:" + ad.PNI
			}
			fmt.Fprintf(e.stdout, "  PNI: %s Given name: %s Family name: %s, Nick name: %s Nick given name: %s Nick family name %s Note: %s Archived: %t Hidden: %t Profile sharing: %t About: %s About Emoji: %s Unregistered: %t\n",
				pni, orEmpty(rc.GivenName), orEmpty(rc.FamilyName), orEmpty(rc.NickName), orEmpty(rc.NickGivenName), orEmpty(rc.NickFamilyName), orEmpty(rc.Note),
				rc.Archived, rc.Hidden, rc.ProfileSharing, orEmpty(p.About), orEmpty(p.AboutEmoji), rc.Unregistered)
		}
		if o.internal {
			mode := p.AccessMode
			if mode == store.AccessUnknown {
				mode = ""
			}
			fmt.Fprintf(e.stdout, "  Capabilities: [%s] Unidentified access mode: %s Shares number: %s Discoverable by number: %s\n",
				strings.Join(p.Capabilities, ", "), mode, sharesNumber(p), boolOrEmpty(rc.Discoverable))
		}
	}
	return nil
}

func sharesNumber(p *store.Profile) string {
	if p.PhoneNumberSharing == nil {
		return ""
	}
	return fmt.Sprint(*p.PhoneNumberSharing == "EVERYBODY")
}

func boolOrEmpty(b *bool) string {
	if b == nil {
		return ""
	}
	return fmt.Sprint(*b)
}

func contactJSON(a *store.Account, k string, internal bool) *jsonObject {
	rc := a.Recipients[k]
	ad := rowAddress(a, k)
	o := &jsonObject{}
	o.add("number", strOrNull(ad.Number))
	o.add("uuid", strOrNull(ad.ACI))
	o.add("username", strOrNull(ad.Username))
	o.add("name", rc.ContactName())
	for _, f := range []struct {
		k string
		v *string
	}{{"givenName", rc.GivenName}, {"familyName", rc.FamilyName}, {"nickName", rc.NickName}, {"nickGivenName", rc.NickGivenName},
		{"nickFamilyName", rc.NickFamilyName}, {"note", rc.Note}, {"color", rc.Color}} {
		o.add(f.k, ptrOrNull(f.v))
	}
	o.add("isArchived", rc.Archived)
	o.add("isBlocked", rc.Blocked)
	o.add("isHidden", rc.Hidden)
	o.add("messageExpirationTime", rc.Timer)
	o.add("profileSharing", rc.ProfileSharing)
	o.add("unregistered", rc.Unregistered)
	p := rc.Profile
	if p == nil {
		p = &store.Profile{AccessMode: store.AccessUnknown}
	}
	po := &jsonObject{}
	po.add("lastUpdateTimestamp", p.LastUpdate)
	po.add("givenName", ptrOrNull(p.GivenName))
	po.add("familyName", ptrOrNull(p.FamilyName))
	po.add("about", ptrOrNull(p.About))
	po.add("aboutEmoji", ptrOrNull(p.AboutEmoji))
	po.add("hasAvatar", p.AvatarURLPath != nil)
	if p.MobileCoinAddress != nil {
		po.add("mobileCoinAddress", base64.StdEncoding.EncodeToString(p.MobileCoinAddress))
	} else {
		po.add("mobileCoinAddress", nil)
	}
	o.add("profile", po)
	if internal {
		io := &jsonObject{}
		io.add("capabilities", append([]string{}, p.Capabilities...))
		if p.AccessMode == "" || p.AccessMode == store.AccessUnknown {
			io.add("unidentifiedAccessMode", nil)
		} else {
			io.add("unidentifiedAccessMode", p.AccessMode)
		}
		if p.PhoneNumberSharing == nil {
			io.add("sharesPhoneNumber", nil)
		} else {
			io.add("sharesPhoneNumber", *p.PhoneNumberSharing == "EVERYBODY")
		}
		if rc.Discoverable == nil {
			io.add("discoverableByPhonenumber", nil)
		} else {
			io.add("discoverableByPhonenumber", *rc.Discoverable)
		}
		o.add("internal", io)
	}
	return o
}

func ptrOrNull(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// notRegistered is contacts.md §4.1's error.
func notRegistered(arg string) error { return userError("The user " + arg + " is not registered.") }

// runUpdateContact is `updateContact` (contacts.md §9).
func runUpdateContact(ctx context.Context, e *env, g *globals, recipient string, name *string, u manager.ContactUpdate) error {
	return withAccount(g, func(a *store.Account) error {
		k, ok, err := recipientKey(a, recipient)
		if err != nil {
			return err
		}
		if !ok {
			return notRegistered(recipient)
		}
		if u.GivenName == nil && name != nil {
			u.GivenName = name
			if u.FamilyName == nil {
				empty := ""
				u.FamilyName = &empty
			}
		}
		if u.Expiration == nil {
			s := &manager.Sender{Acct: a}
			return s.UpdateContact(ctx, k, u)
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		return s.sender.UpdateContact(ctx, k, u)
	})
}

// runRemoveContact is `removeContact` (contacts.md §10); an unresolvable
// recipient is a silent no-op.
func runRemoveContact(e *env, g *globals, recipient string, hide, forget bool) error {
	return withAccount(g, func(a *store.Account) error {
		t, err := resolveRecipient(a, recipient)
		var k string
		switch {
		case err == nil && t.aci != "":
			var ok bool
			if k, ok = a.FindByACI(t.aci); !ok {
				k = a.ResolveLowTrust(store.Address{ACI: t.aci})
			}
		case err != nil && strings.HasPrefix(err.Error(), "Invalid phone number"):
			return err
		default:
			n, _ := canonicalNumber(a, recipient)
			var ok bool
			if k, ok = a.FindByNumber(n); !ok {
				return nil
			}
		}
		err = (&manager.Sender{Acct: a}).RemoveContact(k, hide, forget)
		if errors.Is(err, manager.ErrDeleteSelf) {
			return &cliError{msg: "Exception in thread \"main\" java.lang.RuntimeException: Can't delete self recipient", code: 2}
		}
		return err
	})
}

// runBlock is `block` / `unblock` (contacts.md §7).
func runBlock(ctx context.Context, e *env, g *globals, recipients, groupArgs []string, blocked bool) error {
	return withAccount(g, func(a *store.Account) error {
		var keys []string
		for _, r := range recipients {
			k, ok, err := recipientKey(a, r)
			if err != nil {
				return err
			}
			if !ok {
				return notRegistered(r)
			}
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
		if len(recipients) == 0 && len(groupArgs) == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if len(recipients) > 0 {
			if err := s.sender.SetBlocked(ctx, keys, blocked, s.groups); err != nil {
				return &cliError{msg: fmt.Sprintf("Failed to sync block to linked devices: %v", err), code: 2}
			}
		}
		if len(groupArgs) > 0 {
			var ids [][]byte
			for _, arg := range groupArgs {
				id, err := parseGroupArg(arg)
				if err != nil {
					return err
				}
				if a.Group(id) == nil {
					fmt.Fprintf(e.stderr, "WARN  BlockCommand - Unknown group id: %s\n", arg)
					continue
				}
				ids = append(ids, id)
			}
			if err := s.sender.SetGroupsBlocked(ctx, ids, blocked, s.groups); err != nil {
				return &cliError{msg: fmt.Sprintf("Failed to sync block to linked devices: %v", err), code: 2}
			}
		}
		return nil
	})
}

// runGetAvatar is `getAvatar` (contacts.md §12, getAvatar.md).
func runGetAvatar(ctx context.Context, e *env, g *globals, contact, prof, group string) error {
	return withAccount(g, func(a *store.Account) error {
		dir := filepath.Join(g.dataDir, "avatars")
		var file string
		switch {
		case group != "":
			id, err := parseGroupArg(group)
			if err != nil {
				return err
			}
			file = filepath.Join(dir, "group-"+strings.ReplaceAll(base64.StdEncoding.EncodeToString(id), "/", "_"))
		default:
			arg := contact
			if arg == "" {
				arg = prof
			}
			k, ok, err := recipientKey(a, arg)
			if err != nil {
				return err
			}
			if !ok {
				return notRegistered(arg)
			}
			if prof != "" {
				ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				defer cancel()
				s, err := openSession(ctx, e, g, a)
				if err != nil {
					return err
				}
				defer s.close()
				s.sender.Profile(ctx, k)
				file = filepath.Join(dir, "profile-"+manager.AvatarID(a, k))
			} else {
				file = filepath.Join(dir, "contact-"+manager.AvatarID(a, k))
			}
		}
		data, err := os.ReadFile(file)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return userError("Could not find avatar")
		case err != nil:
			return &cliError{msg: "An error occurred reading avatar", code: 2}
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		if g.json() {
			o := &jsonObject{}
			o.add("data", b64)
			fmt.Fprintln(e.stdout, o.String())
		} else {
			fmt.Fprintln(e.stdout, b64)
		}
		return nil
	})
}
