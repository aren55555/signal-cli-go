// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountLifecycle(t *testing.T) {
	root := t.TempDir()
	r := Open(root)
	if _, err := r.Find("+15555550100"); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("find in empty root: %v", err)
	}
	a, err := r.Create("+15555550100", EnvStaging)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := r.Create("+15555550100", EnvStaging); err == nil {
		t.Error("duplicate number accepted")
	}
	a.Data.ACI = "0b6ab9b6-0000-4000-8000-00000000c5d3"
	a.Data.DeviceID = 2
	a.Data.Password = "pw"
	a.Data.ACIIdentity = KeyPair{Public: []byte{5, 1}, Private: []byte{2}}
	a.Data.ACIKeys = PreKeyState{NextPreKeyID: 10, ActiveSignedPreKeyID: -1, ActiveLastResortKyberID: 7}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	a.Keys.ACI.SignedPreKeys = map[uint32]StoredKey{7: {Record: []byte{1, 2}}}
	a.Sessions = map[string][]byte{"aci:x.1": {9}}
	a.Identities = map[string]Identity{"x": {Key: []byte{5, 9}, Trust: TrustedUnverified}}
	if err := a.SaveKeys(); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSessions(); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveIdentities(); err != nil {
		t.Fatal(err)
	}

	// While open, the account is locked against other processes.
	if _, err := r.Load(a.Path()); !errors.Is(err, ErrLocked) {
		t.Errorf("second open: %v", err)
	}
	a.Close()

	b, err := r.Find("+15555550100")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Data.ACI != a.Data.ACI || b.Data.DeviceID != 2 || b.Data.Password != "pw" || string(b.Data.ACIIdentity.Public) != "\x05\x01" ||
		b.Data.ACIKeys.ActiveLastResortKyberID != 7 || b.Data.Environment != EnvStaging || b.Data.Number != "+15555550100" {
		t.Errorf("reloaded %+v", b.Data)
	}
	if string(b.Keys.ACI.SignedPreKeys[7].Record) != "\x01\x02" || string(b.Sessions["aci:x.1"]) != "\x09" || b.Identities["x"].Trust != TrustedUnverified {
		t.Errorf("reloaded keys %+v sessions %v identities %v", b.Keys, b.Sessions, b.Identities)
	}
	idx, _ := os.ReadFile(filepath.Join(root, "accounts.json"))
	if len(idx) == 0 {
		t.Error("no index")
	}
	if fi, _ := os.Stat(filepath.Join(b.Dir(), "account.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("account file mode %v", fi.Mode())
	}
	if err := r.Delete(b); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Find("+15555550100"); !errors.Is(err, ErrNoAccount) {
		t.Errorf("after delete: %v", err)
	}
}

func TestRecipients(t *testing.T) {
	r := Open(t.TempDir())
	a, err := r.Create("+15555550100", EnvStaging)
	if err != nil {
		t.Fatal(err)
	}
	a.Recipients["x"] = Recipient{ProfileKey: []byte{1}, Profile: &Profile{LastUpdate: 5, AccessMode: AccessEnabled}}
	if err := a.SaveRecipients(); err != nil {
		t.Fatal(err)
	}
	path := a.Path()
	a.Close()
	b, err := r.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if rc := b.Recipients["x"]; string(rc.ProfileKey) != "\x01" || rc.Profile.AccessMode != AccessEnabled {
		t.Errorf("reloaded %+v", rc)
	}
}

// TestIndexIsSignalCLIFormat: the index is signal-cli's accounts.json
// (version 2, path/environment/number/uuid, Jackson's pretty printer), so
// account selection matches signal-cli on any data directory; our own
// files live in <path>.go/ next to where signal-cli keeps its account file.
func TestIndexIsSignalCLIFormat(t *testing.T) {
	root := t.TempDir()
	r := Open(root)
	a, err := r.Create("+15555550100", EnvStaging)
	if err != nil {
		t.Fatal(err)
	}
	a.Data.ACI = "0b6ab9b6-0000-4000-8000-00000000c5d3"
	a.Save()
	b, _ := r.Create("+15555550101", EnvLive)
	b.Close()
	a.Close()
	got, _ := os.ReadFile(filepath.Join(root, "accounts.json"))
	want := "{\n  \"accounts\" : [ {\n    \"path\" : \"" + a.Path() + "\",\n    \"environment\" : \"STAGING\",\n    \"number\" : \"+15555550100\",\n    \"uuid\" : \"0b6ab9b6-0000-4000-8000-00000000c5d3\"\n  }, {\n    \"path\" : \"" + b.Path() + "\",\n    \"environment\" : \"LIVE\",\n    \"number\" : \"+15555550101\",\n    \"uuid\" : null\n  } ],\n  \"version\" : 2\n}"
	if string(got) != want {
		t.Errorf("index\n%s\nwant\n%s", got, want)
	}
	if fi, err := os.Stat(filepath.Join(root, a.Path()+".go", "account.json")); err != nil || fi.IsDir() {
		t.Errorf("account file: %v", err)
	}

	// signal-cli's own index entries without our files are listed but not
	// loadable as ours.
	os.WriteFile(filepath.Join(root, "accounts.json"), []byte("{\n  \"accounts\" : [ {\n    \"path\" : \"111111\",\n    \"environment\" : \"LIVE\",\n    \"number\" : \"+15555550111\",\n    \"uuid\" : null\n  } ],\n  \"version\" : 2\n}\n"), 0o600)
	entries, err := r.Accounts()
	if err != nil || len(entries) != 1 || entries[0].Number != "+15555550111" || entries[0].UUID != "" {
		t.Fatalf("entries %+v, %v", entries, err)
	}
	if _, err := r.Find("+15555550111"); !errors.Is(err, ErrNotOurs) {
		t.Errorf("foreign account: %v", err)
	}
	empty := Open(filepath.Join(root, "none"))
	if e, err := empty.Accounts(); err != nil || len(e) != 0 {
		t.Errorf("missing index: %v %v", e, err)
	}
}

// TestIdentityRows: updates keep an identity's row, new addresses go
// last, and rowless entries from older stores are numbered in service-ID
// order after the numbered ones.
func TestIdentityRows(t *testing.T) {
	a := &Account{Identities: map[string]Identity{
		"c": {Key: []byte{1}}, "a": {Key: []byte{2}}, "z": {Key: []byte{3}, Row: 7},
	}}
	a.numberIdentities()
	if got := strings.Join(a.IdentityOrder(), ","); got != "z,a,c" {
		t.Fatalf("order %s", got)
	}
	a.PutIdentity("b", Identity{Key: []byte{4}})
	a.PutIdentity("z", Identity{Key: []byte{5}, Trust: TrustedVerified})
	if got := strings.Join(a.IdentityOrder(), ","); got != "z,a,c,b" {
		t.Errorf("order after updates %s", got)
	}
	if a.Identities["b"].Row != 10 || a.Identities["z"].Row != 7 || a.Identities["z"].Trust != TrustedVerified {
		t.Errorf("rows %+v", a.Identities)
	}
}
