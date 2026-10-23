package store

import (
	"errors"
	"os"
	"path/filepath"
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
	if fi, _ := os.Stat(filepath.Join(root, b.Path(), "account.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("account file mode %v", fi.Mode())
	}
	if err := r.Delete(b); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Find("+15555550100"); !errors.Is(err, ErrNoAccount) {
		t.Errorf("after delete: %v", err)
	}
}
