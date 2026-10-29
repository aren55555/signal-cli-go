// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"testing"
)

const (
	aciA = "aaaaaaaa-0000-4000-8000-000000000001"
	aciB = "bbbbbbbb-0000-4000-8000-000000000002"
	pniA = "aaaaaaaa-0000-4000-8000-0000000000f1"
	pniB = "bbbbbbbb-0000-4000-8000-0000000000f2"
	numA = "+12025550101"
	numB = "+12025550102"
)

func newStore(t *testing.T) *Account {
	t.Helper()
	a, err := Open(t.TempDir()).Create("+12025550100", EnvStaging)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func (a *Account) only(t *testing.T, want ...Address) {
	t.Helper()
	if len(a.Recipients) != len(want) {
		t.Fatalf("%d rows, want %d: %v", len(a.Recipients), len(want), a.Recipients)
	}
	for _, w := range want {
		k := keyOf(w)
		if _, ok := a.Recipients[k]; !ok || a.AddressOf(k) != w {
			t.Errorf("no row %+v (have %v)", w, a.Recipients)
		}
	}
}

func TestLowTrust(t *testing.T) {
	a := newStore(t)
	// A number given with a service ID is dropped when creating.
	k := a.ResolveLowTrust(Address{ACI: aciA, Number: numA})
	if k != aciA {
		t.Fatalf("key %s", k)
	}
	a.only(t, Address{ACI: aciA})
	// Lookup order ACI, PNI, number; nothing is linked.
	k2 := a.ResolveLowTrust(Address{Number: numB})
	a.ResolveLowTrust(Address{PNI: pniB, Number: numB})
	// PNI unknown, so the number row is found; no link, no new row.
	a.only(t, Address{ACI: aciA}, Address{Number: numB})
	if k2 != numB || a.Recipients[k2].ID <= a.Recipients[aciA].ID {
		t.Errorf("IDs: %v", a.Recipients)
	}
}

func TestHighTrustCreateAndUpdate(t *testing.T) {
	a := newStore(t)
	a.ResolveHighTrust(Address{ACI: aciA, PNI: pniA, Number: numA})
	a.only(t, Address{ACI: aciA, PNI: pniA, Number: numA})

	// One match without ACI is updated to the union.
	b := newStore(t)
	k := b.ResolveLowTrust(Address{Number: numB})
	rc := b.Recipients[k]
	rc.ProfileSharing = true
	b.Recipients[k] = rc
	nk := b.ResolveHighTrust(Address{ACI: aciB, PNI: pniB, Number: numB})
	b.only(t, Address{ACI: aciB, PNI: pniB, Number: numB})
	if !b.Recipients[nk].ProfileSharing {
		t.Error("row data lost on re-key")
	}

	// One match with a different ACI: the shared number is stripped and
	// a new row created.
	c := newStore(t)
	c.ResolveHighTrust(Address{ACI: aciA, Number: numA})
	c.ResolveHighTrust(Address{ACI: aciB, Number: numA})
	c.only(t, Address{ACI: aciA}, Address{ACI: aciB, Number: numA})
}

func TestHighTrustMerge(t *testing.T) {
	a := newStore(t)
	ka := a.ResolveLowTrust(Address{ACI: aciA})
	kp := a.ResolveLowTrust(Address{PNI: pniA})
	rc := a.Recipients[kp]
	rc.ProfileKey = []byte{1}
	a.Recipients[kp] = rc
	rc = a.Recipients[ka]
	rc.Unregistered, rc.UnregisteredAt = true, 5
	a.Recipients[ka] = rc
	nk := a.ResolveHighTrust(Address{ACI: aciA, PNI: pniA, Number: numA})
	a.only(t, Address{ACI: aciA, PNI: pniA, Number: numA})
	if got := a.Recipients[nk]; len(got.ProfileKey) != 1 || got.Unregistered {
		t.Errorf("merged row %+v", got)
	}

	// A member with an identifier beyond A's is stripped, not merged.
	b := newStore(t)
	b.ResolveHighTrust(Address{ACI: aciB, Number: numA})
	b.ResolveLowTrust(Address{PNI: pniA})
	b.ResolveHighTrust(Address{PNI: pniA, Number: numA})
	b.only(t, Address{ACI: aciB}, Address{PNI: pniA, Number: numA})
}

func TestMarkUnregisteredSplits(t *testing.T) {
	a := newStore(t)
	k := a.ResolveHighTrust(Address{ACI: aciA, PNI: pniA, Number: numA})
	a.MarkUnregistered(k, 77)
	a.only(t, Address{ACI: aciA}, Address{PNI: pniA, Number: numA})
	rc := a.Recipients[aciA]
	if !rc.Unregistered || rc.UnregisteredAt != 77 || rc.Discoverable == nil || *rc.Discoverable {
		t.Errorf("%+v", rc)
	}
	a.MarkRegistered(aciA)
	if a.Recipients[aciA].Unregistered {
		t.Error("still unregistered")
	}
}

func TestIsContactAndNames(t *testing.T) {
	empty, g, f := "", "Ann", "Lee"
	if (Recipient{Hidden: true}).IsContact() {
		t.Error("hidden alone is a contact")
	}
	if !(Recipient{Note: &empty}).IsContact() {
		t.Error("empty note does not count")
	}
	r := Recipient{GivenName: &g, FamilyName: &f, NickGivenName: &empty}
	if r.ContactName() != "Ann Lee" || r.Nickname() != "" {
		t.Errorf("%q %q", r.ContactName(), r.Nickname())
	}
}

func TestRecipientIDsPersist(t *testing.T) {
	a := newStore(t)
	a.ResolveLowTrust(Address{ACI: aciA})
	a.ResolveLowTrust(Address{Number: numB})
	if err := a.SaveRecipients(); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{aciA: a.Recipients[aciA].ID, numB: a.Recipients[numB].ID}
	a.Recipients = nil
	if err := a.loadRecipients(); err != nil {
		t.Fatal(err)
	}
	k := a.ResolveLowTrust(Address{ACI: aciB})
	for key, id := range ids {
		if a.Recipients[key].ID != id {
			t.Errorf("%s id %d, want %d", key, a.Recipients[key].ID, id)
		}
	}
	if a.Recipients[k].ID <= max(ids[aciA], ids[numB]) {
		t.Error("new ID reused")
	}
}

// TestPNINumberMemberMerges: a member holding only a PNI and a number is
// merged even when A does not carry that number (the server links them).
func TestPNINumberMemberMerges(t *testing.T) {
	a := newStore(t)
	a.ResolveLowTrust(Address{ACI: aciA})
	a.ResolveHighTrust(Address{PNI: pniA, Number: numA})
	a.ResolveHighTrust(Address{ACI: aciA, PNI: pniA})
	a.only(t, Address{ACI: aciA, PNI: pniA, Number: numA})
}

// TestEnsureSelfUsername: the self row carries the account's username
// and follows changes to it. A cleared account username leaves the row's
// alone: a reference account (capture-a, 2026-10-29) has no username in
// its account file but keeps one on its self row.
func TestEnsureSelfUsername(t *testing.T) {
	a := newStore(t)
	a.Data.ACI = aciA
	a.Data.Username = "self.01"
	if !a.EnsureSelf() {
		t.Fatal("self row not created")
	}
	k, _ := a.FindByACI(aciA)
	if a.Recipients[k].Username != "self.01" {
		t.Errorf("username %q", a.Recipients[k].Username)
	}
	if a.EnsureSelf() {
		t.Error("no change expected")
	}
	a.Data.Username = ""
	if a.EnsureSelf() || a.Recipients[k].Username != "self.01" {
		t.Error("a cleared username changed the self row")
	}
	a.Data.Username = "self.02"
	if !a.EnsureSelf() || a.Recipients[k].Username != "self.02" {
		t.Error("a new username was not copied")
	}
}
