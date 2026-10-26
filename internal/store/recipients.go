package store

import (
	"maps"
	"slices"
	"strings"
)

// The recipient store keeps one Recipient per row. The map key is the
// row's primary identifier: its ACI if it has one, else "PNI:<uuid>",
// else its phone number, else "u:<username>". A row's other identifiers
// are in its fields (spec/service/contacts.md §2).

// Address is a set of identifiers that may belong to one recipient. PNI
// is the bare UUID; empty means absent.
type Address struct {
	ACI, PNI, Number, Username string
}

func (a Address) count() int {
	n := 0
	for _, s := range []string{a.ACI, a.PNI, a.Number, a.Username} {
		if s != "" {
			n++
		}
	}
	return n
}

// keyOf is the primary key for an address.
func keyOf(a Address) string {
	switch {
	case a.ACI != "":
		return a.ACI
	case a.PNI != "":
		return "PNI:" + a.PNI
	case a.Number != "":
		return a.Number
	case a.Username != "":
		return "u:" + a.Username
	}
	return ""
}

// AddressOf returns the identifiers of the row stored under key.
func (a *Account) AddressOf(key string) Address {
	rc := a.Recipients[key]
	ad := Address{PNI: rc.PNI, Number: rc.Number, Username: rc.Username}
	switch {
	case strings.HasPrefix(key, "PNI:"):
		ad.PNI = strings.TrimPrefix(key, "PNI:")
	case strings.HasPrefix(key, "+"), strings.HasPrefix(key, "u:"):
	default:
		ad.ACI = key
	}
	return ad
}

// setAddress moves the row under key to the key of ad and sets its
// identifier fields; it returns the new key.
func (a *Account) setAddress(key string, ad Address) string {
	rc := a.Recipients[key]
	old := a.AddressOf(key)
	if old.ACI != ad.ACI || old.PNI != ad.PNI {
		rc.PNIVerified = false
	}
	rc.PNI, rc.Number, rc.Username = ad.PNI, ad.Number, ad.Username
	delete(a.Recipients, key)
	nk := keyOf(ad)
	a.Recipients[nk] = rc
	return nk
}

func (a *Account) newRow(ad Address) string {
	a.recipientsNextID++
	k := keyOf(ad)
	a.Recipients[k] = Recipient{ID: a.recipientsNextID}
	return a.setAddress(k, ad)
}

// assignIDs gives rows without an ID one, in key order (rows from before
// IDs existed).
func (a *Account) assignIDs() {
	for _, k := range slices.Sorted(maps.Keys(a.Recipients)) {
		rc := a.Recipients[k]
		a.recipientsNextID = max(a.recipientsNextID, rc.ID)
	}
	for _, k := range slices.Sorted(maps.Keys(a.Recipients)) {
		if rc := a.Recipients[k]; rc.ID == 0 {
			a.recipientsNextID++
			rc.ID = a.recipientsNextID
			a.Recipients[k] = rc
		}
	}
}

// FindByACI, FindByPNI, FindByNumber and FindByUsername return the key of
// the row holding the identifier.
func (a *Account) FindByACI(aci string) (string, bool) {
	if aci == "" {
		return "", false
	}
	_, ok := a.Recipients[aci]
	return aci, ok
}

func (a *Account) FindByPNI(pni string) (string, bool) {
	return a.findBy(pni, func(ad Address) string { return ad.PNI })
}

func (a *Account) FindByNumber(number string) (string, bool) {
	return a.findBy(number, func(ad Address) string { return ad.Number })
}

func (a *Account) FindByUsername(u string) (string, bool) {
	return a.findBy(u, func(ad Address) string { return ad.Username })
}

func (a *Account) findBy(v string, get func(Address) string) (string, bool) {
	if v == "" {
		return "", false
	}
	for k := range a.Recipients {
		if get(a.AddressOf(k)) == v {
			return k, true
		}
	}
	return "", false
}

// ResolveLowTrust finds a recipient by ACI, then PNI, then number, without
// linking anything; if none exists it creates one with only the service ID
// (or only the number when there is none) (contacts.md §4.2).
func (a *Account) ResolveLowTrust(ad Address) string {
	if k, ok := a.FindByACI(ad.ACI); ok {
		return k
	}
	if k, ok := a.FindByPNI(ad.PNI); ok {
		return k
	}
	if k, ok := a.FindByNumber(ad.Number); ok {
		return k
	}
	if ad.ACI == "" && ad.PNI == "" {
		if k, ok := a.FindByUsername(ad.Username); ok {
			return k
		}
	}
	switch {
	case ad.ACI != "":
		return a.newRow(Address{ACI: ad.ACI})
	case ad.PNI != "":
		return a.newRow(Address{PNI: ad.PNI})
	case ad.Number != "":
		return a.newRow(Address{Number: ad.Number})
	}
	return a.newRow(Address{Username: ad.Username})
}

// overlay returns base with every identifier set in top taking its place.
func overlay(base, top Address) Address {
	if top.ACI != "" {
		base.ACI = top.ACI
	}
	if top.PNI != "" {
		base.PNI = top.PNI
	}
	if top.Number != "" {
		base.Number = top.Number
	}
	if top.Username != "" {
		base.Username = top.Username
	}
	return base
}

// strip clears from ad every identifier it shares with other.
func strip(ad, other Address) Address {
	if ad.ACI != "" && ad.ACI == other.ACI {
		ad.ACI = ""
	}
	if ad.PNI != "" && ad.PNI == other.PNI {
		ad.PNI = ""
	}
	if ad.Number != "" && ad.Number == other.Number {
		ad.Number = ""
	}
	if ad.Username != "" && ad.Username == other.Username {
		ad.Username = ""
	}
	return ad
}

// covers reports whether every identifier of o equals a's value.
func covers(a, o Address) bool {
	return (o.ACI == "" || o.ACI == a.ACI) && (o.PNI == "" || o.PNI == a.PNI) &&
		(o.Number == "" || o.Number == a.Number) && (o.Username == "" || o.Username == a.Username)
}

// ResolveHighTrust resolves an address whose identifiers a source vouches
// for, linking, merging and stripping rows (contacts.md §4.3). It returns
// the key of the resulting row.
func (a *Account) ResolveHighTrust(ad Address) string {
	if ad.count() <= 1 {
		return a.ResolveLowTrust(ad)
	}
	var m []string
	for k := range a.Recipients {
		o := a.AddressOf(k)
		if (ad.ACI != "" && o.ACI == ad.ACI) || (ad.PNI != "" && o.PNI == ad.PNI) ||
			(ad.Number != "" && o.Number == ad.Number) || (ad.Username != "" && o.Username == ad.Username) {
			m = append(m, k)
		}
	}
	slices.Sort(m)
	switch len(m) {
	case 0:
		return a.newRow(ad)
	case 1:
		k := m[0]
		r := a.AddressOf(k)
		switch {
		case covers(r, ad):
			return k
		case r.ACI == "" || (ad.ACI == "" && (ad.PNI == "" || r.PNI == ad.PNI)) || r.ACI == ad.ACI:
			return a.setAddress(k, overlay(r, ad))
		default:
			a.setAddress(k, strip(r, ad))
			return a.newRow(ad)
		}
	}

	target := ""
	for _, k := range m {
		if o := a.AddressOf(k); ad.ACI != "" && o.ACI == ad.ACI {
			target = k
		}
	}
	if target == "" && ad.PNI != "" {
		for _, k := range m {
			if a.AddressOf(k).PNI == ad.PNI {
				target = k
			}
		}
	}
	var merges, strips []string
	for _, k := range m {
		if k == target {
			continue
		}
		o := a.AddressOf(k)
		if covers(ad, o) || (o.ACI == "" && o.Username == "" && o.PNI != "" && o.Number != "") {
			merges = append(merges, k)
		} else {
			strips = append(strips, k)
		}
	}
	final := Address{}
	if target != "" {
		final = a.AddressOf(target)
	}
	for _, k := range merges {
		final = overlay(final, a.AddressOf(k))
	}
	final = overlay(final, ad)
	// Merge members lose their address before anything is re-keyed.
	merged := map[string]Recipient{}
	for _, k := range merges {
		merged[k] = a.Recipients[k]
		delete(a.Recipients, k)
	}
	for _, k := range strips {
		a.setAddress(k, strip(a.AddressOf(k), final))
	}
	var tk string
	if target != "" {
		tk = a.setAddress(target, final)
	} else {
		tk = a.newRow(final)
	}
	t := a.Recipients[tk]
	t.Unregistered, t.UnregisteredAt = false, 0
	for _, k := range merges {
		mergeInto(&t, merged[k])
	}
	a.Recipients[tk] = t
	return tk
}

// mergeInto copies the data of a merged row o into t (contacts.md §4.3).
func mergeInto(t *Recipient, o Recipient) {
	if !t.IsContact() && o.IsContact() {
		t.copyContact(o)
	}
	if len(t.ProfileKey) == 0 {
		t.ProfileKey = o.ProfileKey
	}
	if len(t.ProfileKeyCredential) == 0 {
		t.ProfileKeyCredential = o.ProfileKeyCredential
	}
	if t.Profile == nil {
		t.Profile = o.Profile
	}
	t.PNIVerified = t.PNIVerified || o.PNIVerified
}

// copyContact copies the contact columns of o.
func (r *Recipient) copyContact(o Recipient) {
	r.GivenName, r.FamilyName, r.NickName, r.NickGivenName, r.NickFamilyName = o.GivenName, o.FamilyName, o.NickName, o.NickGivenName, o.NickFamilyName
	r.Note, r.Color, r.Timer, r.TimerVersion, r.MuteUntil = o.Note, o.Color, o.Timer, o.TimerVersion, o.MuteUntil
	r.HideStory, r.Archived, r.Blocked, r.ProfileSharing, r.Hidden = o.HideStory, o.Archived, o.Blocked, o.ProfileSharing, o.Hidden
}

// IsContact is the is-contact predicate (contacts.md §1).
func (r Recipient) IsContact() bool {
	return r.GivenName != nil || r.FamilyName != nil || r.NickName != nil || r.NickGivenName != nil ||
		r.NickFamilyName != nil || r.Note != nil || r.Timer > 0 || r.ProfileSharing || r.Color != nil || r.Blocked || r.Archived
}

// MarkRegistered clears the unregistered mark.
func (a *Account) MarkRegistered(key string) {
	rc := a.Recipients[key]
	rc.Unregistered, rc.UnregisteredAt = false, 0
	a.Recipients[key] = rc
}

// MarkUnregistered marks a row unregistered at now (ms) and not
// discoverable; a row with both ACI and PNI is split, the PNI and number
// moving to a new row (contacts.md §4.4).
func (a *Account) MarkUnregistered(key string, now int64) {
	rc := a.Recipients[key]
	no := false
	rc.Unregistered, rc.UnregisteredAt, rc.Discoverable = true, now, &no
	a.Recipients[key] = rc
	ad := a.AddressOf(key)
	if ad.ACI != "" && ad.PNI != "" {
		a.setAddress(key, Address{ACI: ad.ACI, Username: ad.Username})
		a.newRow(Address{PNI: ad.PNI, Number: ad.Number})
	}
}

// ContactName joins given and family name (contacts.md §3.1).
func (r Recipient) ContactName() string { return joinNames(r.GivenName, r.FamilyName) }

// Nickname joins the nickname parts; "" means none (contacts.md §3.2).
func (r Recipient) Nickname() string { return joinNames(r.NickGivenName, r.NickFamilyName) }

func joinNames(g, f *string) string {
	gs, fs := "", ""
	if g != nil {
		gs = *g
	}
	if f != nil {
		fs = *f
	}
	switch {
	case gs == "":
		return fs
	case fs == "":
		return gs
	}
	return gs + " " + fs
}

// SortedRecipientKeys lists keys in row-ID order (the listing order).
func (a *Account) SortedRecipientKeys() []string {
	keys := slices.Collect(maps.Keys(a.Recipients))
	slices.SortFunc(keys, func(x, y string) int {
		return int(a.Recipients[x].ID - a.Recipients[y].ID)
	})
	return keys
}

// DeleteSessions removes every session with serviceID, in both the ACI
// and PNI stores ("<store>:<service ID>.<device>").
func (a *Account) DeleteSessions(serviceID string) {
	for k := range a.Sessions {
		_, rest, _ := strings.Cut(k, ":")
		if i := strings.LastIndexByte(rest, '.'); i >= 0 && rest[:i] == serviceID {
			delete(a.Sessions, k)
		}
	}
}

// DeleteSenderKeys removes the sender keys received from serviceID
// ("<service ID>.<device>.<distribution ID>").
func (a *Account) DeleteSenderKeys(serviceID string) {
	for k := range a.SenderKeys.Records {
		if strings.HasPrefix(k, serviceID+".") {
			delete(a.SenderKeys.Records, k)
			delete(a.SenderKeys.Created, k)
		}
	}
	a.SaveSenderKeys()
}
