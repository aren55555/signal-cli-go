// SPDX-License-Identifier: GPL-3.0-or-later

package importer

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadIndex(t *testing.T) {
	dir := t.TempDir()
	if _, err := readIndex(dir); err == nil || !strings.Contains(err.Error(), "not a signal-cli data directory") {
		t.Errorf("missing index: %v", err)
	}
	write(t, filepath.Join(dir, "data", "accounts.json"), `{
  "accounts" : [ {
    "path" : "616701",
    "environment" : "STAGING",
    "number" : "+15555550100",
    "uuid" : null
  } ],
  "version" : 2
}`)
	es, err := readIndex(dir)
	if err != nil || len(es) != 1 || es[0].Path != "616701" || *es[0].Number != "+15555550100" || es[0].UUID != nil {
		t.Fatalf("readIndex = %+v, %v", es, err)
	}
	write(t, filepath.Join(dir, "data", "accounts.json"), `{"accounts":[],"version":3}`)
	if _, err := readIndex(dir); err == nil {
		t.Error("version 3 accepted")
	}
}

func TestReadAccountFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "acct")
	for _, c := range []struct{ body, err string }{
		{``, "not registered"},
		{`{"username":"+15555550100","registered":true}`, "no version"},
		{`{"version":9,"serviceEnvironment":"STAGING"}`, "version 9"},
		{`{"version":11,"serviceEnvironment":"STAGING"}`, "newer"},
		{`{"version":10,"serviceEnvironment":"TEST"}`, "unknown service environment"},
	} {
		write(t, p, c.body)
		if _, err := readAccountFile(p); err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%q: err %v, want %q", c.body, err, c.err)
		}
	}
	// The data-dir §3.2 example, with base64 keys.
	write(t, p, `{
  "version" : 10, "timestamp" : 1793292002700, "serviceEnvironment" : "STAGING", "registered" : true,
  "number" : "+15555550100", "username" : null, "encryptedDeviceName" : "abc", "deviceId" : 3, "isMultiDevice" : true,
  "password" : "UbffUm+Sy5hGlL2nQhUfLhvy",
  "aciAccountData" : { "serviceId" : "00000000-0000-4000-8000-000000000001", "registrationId" : 6851,
    "identityPrivateKey" : "oPyqJvqfMWt6HWpUkhY1galZGhU63s0BrlhF0UicEmk=",
    "identityPublicKey" : "BWLEyxpz0VrY5SRJeG2p9YYAkHDxG9kDZVNva0Dgy9Uo",
    "nextPreKeyId" : 13517850, "nextSignedPreKeyId" : 16355817, "activeSignedPreKeyId" : -1,
    "nextKyberPreKeyId" : 3877776, "activeLastResortKyberPreKeyId" : 7 },
  "pniAccountData" : { "serviceId" : "PNI:00000000-0000-4000-8000-000000000002" },
  "registrationLockPin" : null, "pinMasterKey" : null, "storageKey" : null, "accountEntropyPool" : null,
  "mediaRootBackupKey" : null, "profileKey" : "3qk/OHF9Wnnkt1yzKXNWGKlI5wH4K2wyEFROn4Sy1sY=",
  "usernameLinkEntropy" : null, "usernameLinkServerId" : null
}`)
	f, err := readAccountFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Registered || f.DeviceID != 3 || *f.EncryptedDeviceName != "abc" || f.ACI.RegistrationID != 6851 || len(f.ACI.IdentityPrivateKey) != 32 ||
		len(f.ACI.IdentityPublicKey) != 33 || f.ACI.ActiveSignedPreKeyID != -1 || f.ACI.ActiveLastResortKyberPreKeyID != 7 ||
		bareUUID(*f.PNI.ServiceID) != "00000000-0000-4000-8000-000000000002" || len(f.ProfileKey) != 32 || f.PinMasterKey != nil {
		t.Errorf("parsed %+v", f)
	}
}

// TestInUse takes signal-cli's kind of lock (a POSIX write lock over the
// whole file) in a child process and checks that inUse sees it.
func TestInUse(t *testing.T) {
	if os.Getenv("IMPORTER_LOCK_CHILD") != "" {
		f, err := os.OpenFile(os.Getenv("IMPORTER_LOCK_CHILD"), os.O_RDWR, 0)
		if err != nil {
			os.Exit(2)
		}
		lk := syscall.Flock_t{Type: syscall.F_WRLCK}
		if syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk) != nil {
			os.Exit(3)
		}
		os.Stdout.WriteString("locked\n")
		os.Stdin.Read(make([]byte, 1))
		os.Exit(0)
	}
	p := filepath.Join(t.TempDir(), "acct")
	write(t, p, "{}")
	if busy, err := inUse(p); err != nil || busy {
		t.Fatalf("unlocked file: busy=%v err=%v", busy, err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInUse$")
	cmd.Env = append(os.Environ(), "IMPORTER_LOCK_CHILD="+p)
	stdin, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); cmd.Wait() }()
	buf := make([]byte, 7)
	if _, err := out.Read(buf); err != nil || string(buf[:6]) != "locked" {
		t.Fatalf("child: %q %v", buf, err)
	}
	if busy, err := inUse(p); err != nil || !busy {
		t.Errorf("locked file: busy=%v err=%v", busy, err)
	}
	before, _ := os.ReadFile(p)
	if string(before) != "{}" {
		t.Error("inUse changed the file")
	}
}

func TestCopyMsgCache(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	v9 := binary.BigEndian.AppendUint32(nil, 9)
	v9 = binary.BigEndian.AppendUint64(v9, 1793292002700)
	v9 = append(v9, "envelope"...)
	write(t, filepath.Join(src, "12", "1793292002800_1793292002000"), string(v9))
	write(t, filepath.Join(src, "1793292002801_1"), string(binary.BigEndian.AppendUint32(nil, 8))+"old")
	old, err := copyMsgCache(src, filepath.Join(dst, "msg-cache"))
	if err != nil || old != 1 {
		t.Fatalf("copyMsgCache = %d, %v", old, err)
	}
	es, _ := os.ReadDir(filepath.Join(dst, "msg-cache"))
	if len(es) != 1 || !strings.HasSuffix(es[0].Name(), ".env") {
		t.Fatalf("converted %v", es)
	}
	b, _ := os.ReadFile(filepath.Join(dst, "msg-cache", es[0].Name()))
	if binary.BigEndian.Uint64(b) != 1793292002700 || string(b[8:]) != "envelope" {
		t.Errorf("converted file %x", b)
	}
	if n, err := copyMsgCache(filepath.Join(src, "missing"), dst); n != 0 || err != nil {
		t.Errorf("missing cache: %d %v", n, err)
	}
}
