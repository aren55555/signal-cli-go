// SPDX-License-Identifier: GPL-3.0-or-later

package importer

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// OpenFunc opens a SQLite database file for reading and writing (the
// importer only ever hands it a private copy).
type OpenFunc func(path string) (*sql.DB, error)

// Options select what to import.
type Options struct {
	Source, Dest string   // data directories (the -d/--config directory of each client)
	Numbers      []string // accounts to import; empty means all
	Open         OpenFunc
	Log          func(format string, args ...any)
}

// Result reports one account.
type Result struct {
	Number, Path string
	Err          error
	Skipped      []string // state that was not carried over
}

// Import copies the selected accounts from Source into Dest, then the
// shared media directories. It never writes to Source.
func Import(o Options) ([]Result, error) {
	src, err := filepath.Abs(o.Source)
	if err != nil {
		return nil, err
	}
	dst, err := filepath.Abs(o.Dest)
	if err != nil {
		return nil, err
	}
	if src == dst {
		return nil, errors.New("the destination must be a different data directory than the source")
	}
	entries, err := readIndex(src)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range o.Numbers {
		want[n] = true
	}
	root := store.Open(filepath.Join(dst, "data"))
	var results []Result
	for _, e := range entries {
		number := ""
		if e.Number != nil {
			number = *e.Number
		}
		if len(want) > 0 && !want[number] {
			continue
		}
		delete(want, number)
		res := Result{Number: number, Path: e.Path}
		res.Skipped, res.Err = importAccount(o, src, root, e)
		results = append(results, res)
	}
	for n := range want {
		results = append(results, Result{Number: n, Err: errors.New("no such account in the source")})
	}
	if err := copyMedia(src, dst); err != nil {
		return results, err
	}
	return results, nil
}

func importAccount(o Options, src string, root *store.Root, e indexEntry) ([]string, error) {
	if e.Number == nil {
		return nil, errors.New("the index entry has no number")
	}
	file := filepath.Join(src, "data", e.Path)
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		if fi, err := os.Stat(file + ".go"); err == nil && fi.IsDir() {
			return nil, errors.New("this is already a signal-cli-go account; nothing to import")
		}
		return nil, errors.New("signal-cli's account file is missing")
	}
	if busy, err := inUse(file); err != nil {
		return nil, err
	} else if busy {
		return nil, errors.New("signal-cli is running for this account; stop it and import again")
	}
	f, err := readAccountFile(file)
	if err != nil {
		return nil, err
	}
	if !f.Registered || f.DeviceID <= 0 {
		return nil, errNotRegistered
	}
	if f.Number == nil || *f.Number != *e.Number {
		return nil, errors.New("the account file's number does not match accounts.json")
	}
	if existing, _ := root.Accounts(); len(existing) > 0 {
		for _, x := range existing {
			if x.Number == *e.Number {
				return nil, errors.New("the destination already has an account for this number")
			}
		}
	}
	tmp, err := os.MkdirTemp("", "signal-cli-go-import-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	dbDir := filepath.Join(src, "data", e.Path+".d")
	if err := copyDatabase(dbDir, tmp); err != nil {
		return nil, err
	}
	db, err := o.Open(filepath.Join(tmp, "account.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := checkVersion(db); err != nil {
		return nil, err
	}

	a, err := root.CreateAt(e.Path, *e.Number, f.ServiceEnvironment)
	if err != nil {
		return nil, err
	}
	skipped, err := fill(a, f, db, dbDir)
	if err == nil {
		err = save(a)
	}
	if err != nil {
		root.Delete(a)
		return nil, err
	}
	return skipped, a.Close()
}

// fill converts everything into a.
func fill(a *store.Account, f *accountFile, db *sql.DB, dbDir string) ([]string, error) {
	var skipped []string
	kv, err := keyValues(db)
	if err != nil {
		return nil, fmt.Errorf("key_value: %w", err)
	}
	d := &a.Data
	d.DeviceID, d.Password, d.Registered, d.MultiDevice = f.DeviceID, f.Password, true, f.IsMultiDevice
	if f.ACI.ServiceID != nil {
		d.ACI = *f.ACI.ServiceID
	}
	if f.PNI.ServiceID != nil {
		d.PNI = bareUUID(*f.PNI.ServiceID)
	}
	d.ACIIdentity = store.KeyPair{Public: f.ACI.IdentityPublicKey, Private: f.ACI.IdentityPrivateKey}
	d.PNIIdentity = store.KeyPair{Public: f.PNI.IdentityPublicKey, Private: f.PNI.IdentityPrivateKey}
	if d.ACI == "" || len(d.ACIIdentity.Private) != 32 || len(d.ACIIdentity.Public) != 33 {
		return nil, errors.New("the account has no ACI or ACI identity key")
	}
	if d.PNI == "" || len(d.PNIIdentity.Private) != 32 || len(d.PNIIdentity.Public) != 33 {
		return nil, errors.New("the account has no PNI or PNI identity key; let signal-cli 0.14.7 finish its account check first")
	}
	d.ACIRegistrationID, d.PNIRegistrationID = f.ACI.RegistrationID, f.PNI.RegistrationID
	d.ACIKeys = store.PreKeyState{NextPreKeyID: f.ACI.NextPreKeyID, NextSignedPreKeyID: f.ACI.NextSignedPreKeyID,
		NextKyberPreKeyID: f.ACI.NextKyberPreKeyID, ActiveSignedPreKeyID: f.ACI.ActiveSignedPreKeyID, ActiveLastResortKyberID: f.ACI.ActiveLastResortKyberPreKeyID}
	d.PNIKeys = store.PreKeyState{NextPreKeyID: f.PNI.NextPreKeyID, NextSignedPreKeyID: f.PNI.NextSignedPreKeyID,
		NextKyberPreKeyID: f.PNI.NextKyberPreKeyID, ActiveSignedPreKeyID: f.PNI.ActiveSignedPreKeyID, ActiveLastResortKyberID: f.PNI.ActiveLastResortKyberPreKeyID}
	d.ProfileKey, d.MediaRootBackupKey, d.PinMasterKey = f.ProfileKey, f.MediaRootBackupKey, f.PinMasterKey
	if f.AccountEntropyPool != nil {
		d.AccountEntropyPool = *f.AccountEntropyPool
	}
	if f.EncryptedDeviceName != nil {
		d.DeviceName = *f.EncryptedDeviceName
	}
	if f.Username != nil {
		d.Username = *f.Username
	}
	if f.UsernameLinkServerID != nil && f.UsernameLinkEntropy != nil {
		d.UsernameLinkEntropy, d.UsernameLinkServerID = f.UsernameLinkEntropy, *f.UsernameLinkServerID
	}
	if f.RegistrationLockPin != nil {
		d.RegistrationLockPin = *f.RegistrationLockPin
	}
	if f.StorageKey != nil && d.AccountEntropyPool == "" {
		skipped = append(skipped, "storage key without an account entropy pool (storage sync resumes once the primary sends its keys)")
	}

	// key_value (§3).
	d.SessionID, d.SessionNumber = kvString(kv, "verification-session-id"), kvString(kv, "verification-session-number")
	d.LastPniChange, _ = kvInt(kv, "last-applied-pni-change-server-timestamp")
	d.CDSIToken = kvBytes(kv, "cdsi-token")
	d.LastRecipientsRefresh, _ = kvInt(kv, "last-recipients-refresh")
	if b := kvBool(kv, "unrestricted-unidentified-access"); b != nil {
		d.UnrestrictedUnidentifiedAccess = *b
	}
	if b := kvBool(kv, "config-phone-number-unlisted"); b != nil {
		d.Unlisted = *b
	}
	d.PhoneNumberSharingMode = kvString(kv, "config-phone-number-sharing-mode")
	d.Configuration = store.Configuration{
		ReadReceipts:                   kvBool(kv, "config-read-receipts"),
		UnidentifiedDeliveryIndicators: kvBool(kv, "config-unidentified-delivery-indicators"),
		TypingIndicators:               kvBool(kv, "config-typing-indicators"),
		LinkPreviews:                   kvBool(kv, "config-link-previews"),
	}
	if kvString(kv, "username-link-color") != "" {
		skipped = append(skipped, "username link colour")
	}
	if d.CDSINumbers, err = readCDSI(db); err != nil {
		return nil, fmt.Errorf("cdsi: %w", err)
	}

	if err := readPreKeys(db, a); err != nil {
		return nil, err
	}
	if err := readSessions(db, a); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	if err := readIdentities(db, a); err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}
	if err := readSenderKeys(db, a); err != nil {
		return nil, err
	}
	rows, err := readRecipients(db, a)
	if err != nil {
		return nil, err
	}
	a.EnsureSelf()
	v1, err := readGroups(db, a, rows)
	if err != nil {
		return nil, err
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM group_v1").Scan(&n); err == nil && n > 0 {
		skipped = append(skipped, fmt.Sprintf("%d v1 group(s) (signal-cli-go has no v1 groups)", n))
	}
	if err := readStickers(db, a); err != nil {
		return nil, fmt.Errorf("sticker: %w", err)
	}
	if err := readSendLog(db, a); err != nil {
		return nil, err
	}

	// Storage service state (§10, data-dir §4.1).
	st := &store.StorageState{Manifest: -1}
	if v, ok := kvInt(kv, "storage-manifest-version"); ok {
		st.Manifest = v
	}
	if st.UnknownIDs, err = readUnknownStorage(db); err != nil {
		return nil, fmt.Errorf("storage_id: %w", err)
	}
	st.UnknownIDs = append(st.UnknownIDs, v1...)
	if b, err := os.ReadFile(filepath.Join(dbDir, "storage-manifest")); err == nil {
		var m signalpb.StorageManifest
		var rec signalpb.ManifestRecord
		if proto.Unmarshal(b, &m) != nil || proto.Unmarshal(m.GetValue(), &rec) != nil {
			skipped = append(skipped, "unreadable storage-manifest (a full storage sync follows)")
			st.Manifest = -1
		} else {
			for _, id := range rec.GetIdentifiers() {
				st.IDs = append(st.IDs, store.StorageIdent{Raw: id.GetRaw(), Type: int32(id.GetType())})
			}
			st.RecordIkm = rec.GetRecordIkm()
		}
	} else if st.Manifest >= 0 {
		st.Manifest = -1 // no manifest to compare with: sync from scratch
	}
	if err := a.SaveStorage(st); err != nil {
		return nil, err
	}

	if n, err := copyMsgCache(filepath.Join(dbDir, "msg-cache"), filepath.Join(a.Dir(), "msg-cache")); err != nil {
		return nil, fmt.Errorf("msg-cache: %w", err)
	} else if n > 0 {
		skipped = append(skipped, fmt.Sprintf("%d cached envelope(s) in an old format", n))
	}
	return skipped, nil
}

func save(a *store.Account) error {
	for _, f := range []func() error{a.Save, a.SaveKeys, a.SaveSessions, a.SaveIdentities, a.SaveRecipients,
		a.SaveSenderKeys, a.SaveStickers, a.SaveGroups} {
		if err := f(); err != nil {
			return err
		}
	}
	return nil
}

// copyDatabase copies account.db and its write-ahead log (which can hold
// committed data, data-dir §4) into dir. The shared-memory index is left
// out; SQLite rebuilds it from the log.
func copyDatabase(from, dir string) error {
	if err := copyFile(filepath.Join(from, "account.db"), filepath.Join(dir, "account.db")); err != nil {
		return err
	}
	err := copyFile(filepath.Join(from, "account.db-wal"), filepath.Join(dir, "account.db-wal"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
