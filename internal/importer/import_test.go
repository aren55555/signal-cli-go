// SPDX-License-Identifier: GPL-3.0-or-later

package importer

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func openTest(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// schema is the account.db layout of spec/storage/signal-cli-account-db.md
// §2–§13 (column names, types and nullability from its tables).
var schema = []string{
	`CREATE TABLE recipient (_id INTEGER PRIMARY KEY AUTOINCREMENT, storage_id BLOB UNIQUE, storage_record BLOB,
		number TEXT UNIQUE, username TEXT UNIQUE, aci TEXT UNIQUE, pni TEXT UNIQUE, unregistered_timestamp INTEGER,
		discoverable INTEGER, profile_key BLOB, profile_key_credential BLOB, needs_pni_signature INTEGER NOT NULL DEFAULT 0,
		pni_signature_verified INTEGER NOT NULL DEFAULT 0, given_name TEXT, family_name TEXT, nick_name TEXT,
		nick_name_given_name TEXT, nick_name_family_name TEXT, note TEXT, color TEXT, expiration_time INTEGER NOT NULL DEFAULT 0,
		expiration_time_version INTEGER NOT NULL DEFAULT 1, mute_until INTEGER NOT NULL DEFAULT 0, blocked INTEGER NOT NULL DEFAULT 0,
		archived INTEGER NOT NULL DEFAULT 0, profile_sharing INTEGER NOT NULL DEFAULT 0, hide_story INTEGER NOT NULL DEFAULT 0,
		hidden INTEGER NOT NULL DEFAULT 0, profile_last_update_timestamp INTEGER NOT NULL DEFAULT 0, profile_given_name TEXT,
		profile_family_name TEXT, profile_about TEXT, profile_about_emoji TEXT, profile_avatar_url_path TEXT,
		profile_mobile_coin_address BLOB, profile_unidentified_access_mode TEXT, profile_capabilities TEXT,
		profile_phone_number_sharing TEXT) STRICT`,
	`CREATE TABLE key_value (_id INTEGER PRIMARY KEY, key TEXT NOT NULL UNIQUE, value ANY) STRICT`,
	`CREATE TABLE identity (_id INTEGER PRIMARY KEY, address TEXT NOT NULL UNIQUE, identity_key BLOB NOT NULL,
		added_timestamp INTEGER NOT NULL, trust_level INTEGER NOT NULL) STRICT`,
	`CREATE TABLE pre_key (_id INTEGER PRIMARY KEY, account_id_type INTEGER NOT NULL, key_id INTEGER NOT NULL,
		public_key BLOB NOT NULL, private_key BLOB NOT NULL, stale_timestamp INTEGER, UNIQUE(account_id_type, key_id)) STRICT`,
	`CREATE TABLE signed_pre_key (_id INTEGER PRIMARY KEY, account_id_type INTEGER NOT NULL, key_id INTEGER NOT NULL,
		public_key BLOB NOT NULL, private_key BLOB NOT NULL, signature BLOB NOT NULL, timestamp INTEGER DEFAULT 0,
		UNIQUE(account_id_type, key_id)) STRICT`,
	`CREATE TABLE kyber_pre_key (_id INTEGER PRIMARY KEY, account_id_type INTEGER NOT NULL, key_id INTEGER NOT NULL,
		serialized BLOB NOT NULL, is_last_resort INTEGER NOT NULL, stale_timestamp INTEGER, timestamp INTEGER DEFAULT 0,
		UNIQUE(account_id_type, key_id)) STRICT`,
	`CREATE TABLE session (_id INTEGER PRIMARY KEY, account_id_type INTEGER NOT NULL, address TEXT NOT NULL,
		device_id INTEGER NOT NULL, record BLOB NOT NULL, UNIQUE(account_id_type, address, device_id)) STRICT`,
	`CREATE TABLE sender_key (_id INTEGER PRIMARY KEY, address TEXT NOT NULL, device_id INTEGER NOT NULL,
		distribution_id BLOB NOT NULL, record BLOB NOT NULL, created_timestamp INTEGER NOT NULL,
		UNIQUE(address, device_id, distribution_id)) STRICT`,
	`CREATE TABLE sender_key_shared (_id INTEGER PRIMARY KEY, address TEXT NOT NULL, device_id INTEGER NOT NULL,
		distribution_id BLOB NOT NULL, timestamp INTEGER NOT NULL, UNIQUE(address, device_id, distribution_id)) STRICT`,
	`CREATE TABLE group_v2 (_id INTEGER PRIMARY KEY, storage_id BLOB UNIQUE, storage_record BLOB, group_id BLOB NOT NULL UNIQUE,
		master_key BLOB NOT NULL, group_data BLOB, distribution_id BLOB NOT NULL UNIQUE,
		endorsement_expiration_time INTEGER NOT NULL DEFAULT 0, blocked INTEGER NOT NULL DEFAULT 0,
		profile_sharing INTEGER NOT NULL DEFAULT 0, permission_denied INTEGER NOT NULL DEFAULT 0) STRICT`,
	`CREATE TABLE group_v2_member (_id INTEGER PRIMARY KEY, group_id INTEGER NOT NULL REFERENCES group_v2(_id) ON DELETE CASCADE,
		recipient_id INTEGER NOT NULL REFERENCES recipient(_id) ON DELETE CASCADE, endorsement BLOB NOT NULL,
		UNIQUE(group_id, recipient_id)) STRICT`,
	`CREATE TABLE group_v1 (_id INTEGER PRIMARY KEY, storage_id BLOB UNIQUE, storage_record BLOB, group_id BLOB NOT NULL UNIQUE,
		group_id_v2 BLOB UNIQUE, name TEXT, color TEXT, expiration_time INTEGER NOT NULL DEFAULT 0,
		blocked INTEGER NOT NULL DEFAULT 0, archived INTEGER NOT NULL DEFAULT 0) STRICT`,
	`CREATE TABLE storage_id (_id INTEGER PRIMARY KEY, type INTEGER NOT NULL, storage_id BLOB NOT NULL UNIQUE) STRICT`,
	`CREATE TABLE sticker (_id INTEGER PRIMARY KEY, pack_id BLOB NOT NULL UNIQUE, pack_key BLOB NOT NULL,
		installed INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0, deleted_timestamp INTEGER NOT NULL DEFAULT 0,
		storage_id BLOB UNIQUE, storage_record BLOB) STRICT`,
	`CREATE TABLE message_send_log_content (_id INTEGER PRIMARY KEY, group_id BLOB, timestamp INTEGER NOT NULL,
		content BLOB NOT NULL, content_hint INTEGER NOT NULL, urgent INTEGER NOT NULL) STRICT`,
	`CREATE TABLE message_send_log (_id INTEGER PRIMARY KEY, content_id INTEGER NOT NULL REFERENCES message_send_log_content(_id)
		ON DELETE CASCADE, address TEXT NOT NULL, device_id INTEGER NOT NULL) STRICT`,
	`CREATE TABLE cdsi (_id INTEGER PRIMARY KEY, number TEXT NOT NULL UNIQUE, last_seen_at INTEGER NOT NULL) STRICT`,
	`PRAGMA user_version = 30`,
}

const (
	selfACI  = "00000000-0000-4000-8000-000000000001"
	selfPNI  = "00000000-0000-4000-8000-000000000002"
	peerACI  = "00000000-0000-4000-8000-0000000000a1"
	peerPNI  = "00000000-0000-4000-8000-0000000000a2"
	selfNum  = "+15555550100"
	peerNum  = "+15555550101"
	distUUID = "11111111-2222-4333-8444-555555555555"
)

func rep(b byte, n int) []byte { return bytes.Repeat([]byte{b}, n) }

// fixture writes a signal-cli data directory with one linked account.
func fixture(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	b64 := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
	write(t, filepath.Join(src, "data", "accounts.json"), `{"accounts":[{"path":"616701","environment":"STAGING","number":"`+selfNum+`","uuid":"`+selfACI+`"}],"version":2}`)
	acct := map[string]any{
		"version": 10, "timestamp": 1, "serviceEnvironment": "STAGING", "registered": true, "number": selfNum,
		"username": nil, "encryptedDeviceName": "ZGV2aWNl", "deviceId": 2, "isMultiDevice": true, "password": "secretpassword",
		"aciAccountData": map[string]any{"serviceId": selfACI, "registrationId": 101, "identityPrivateKey": b64(rep(1, 32)),
			"identityPublicKey": b64(append([]byte{5}, rep(2, 32)...)), "nextPreKeyId": 11, "nextSignedPreKeyId": 12,
			"activeSignedPreKeyId": 4, "nextKyberPreKeyId": 13, "activeLastResortKyberPreKeyId": 6},
		"pniAccountData": map[string]any{"serviceId": "PNI:" + selfPNI, "registrationId": 102, "identityPrivateKey": b64(rep(3, 32)),
			"identityPublicKey": b64(append([]byte{5}, rep(4, 32)...)), "nextPreKeyId": 21, "nextSignedPreKeyId": 22,
			"activeSignedPreKeyId": -1, "nextKyberPreKeyId": 23, "activeLastResortKyberPreKeyId": -1},
		"registrationLockPin": nil, "pinMasterKey": nil, "storageKey": nil,
		"accountEntropyPool": "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01",
		"mediaRootBackupKey": b64(rep(5, 32)), "profileKey": b64(rep(6, 32)),
		"usernameLinkEntropy": nil, "usernameLinkServerId": nil,
	}
	b, _ := json.Marshal(acct)
	write(t, filepath.Join(src, "data", "616701"), string(b))
	dir := filepath.Join(src, "data", "616701.d")
	os.MkdirAll(dir, 0o700)
	db, err := openTest(filepath.Join(dir, "account.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	group, _ := proto.Marshal(&signalpb.DecryptedGroup{Title: "G", Revision: 2,
		Members: []*signalpb.DecryptedMember{{AciBytes: rep(0xa1, 16), Role: signalpb.GroupMember_ADMINISTRATOR}}})
	manifest, _ := proto.Marshal(&signalpb.ManifestRecord{Identifiers: []*signalpb.ManifestRecord_Identifier{{Raw: rep(9, 16), Type: 1}}, RecordIkm: rep(8, 32)})
	manifestFile, _ := proto.Marshal(&signalpb.StorageManifest{Version: 5, Value: manifest})
	write(t, filepath.Join(dir, "storage-manifest"), string(manifestFile))
	dist := []byte{0x11, 0x11, 0x11, 0x11, 0x22, 0x22, 0x43, 0x33, 0x84, 0x44, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55}
	stmts := append(schema,
		`INSERT INTO recipient (_id, number, aci, pni) VALUES (1, '`+selfNum+`', '`+selfACI+`', 'PNI:`+selfPNI+`')`,
		`INSERT INTO recipient (_id, number, username, aci, pni, profile_key, given_name, nick_name_given_name, expiration_time,
			expiration_time_version, blocked, profile_sharing, unregistered_timestamp, discoverable, profile_last_update_timestamp,
			profile_given_name, profile_unidentified_access_mode, profile_capabilities, profile_phone_number_sharing, storage_id)
			VALUES (2, '`+peerNum+`', 'peer.01', '`+peerACI+`', 'PNI:`+peerPNI+`', x'`+strings.Repeat("07", 32)+`', 'Pat', 'Patty', 60,
			3, 1, 1, 1700, 0, 1800, 'Pat P', 'ENABLED', 'storage,storageServiceEncryptionV2Capability', 'NOBODY', x'`+strings.Repeat("0a", 16)+`')`,
		`INSERT INTO recipient (_id) VALUES (5)`, // merge leftover
		`INSERT INTO recipient (_id, number) VALUES (6, '+15555550102')`,
		`UPDATE sqlite_sequence SET seq = 9 WHERE name = 'recipient'`,
		`INSERT INTO key_value (key, value) VALUES ('config-read-receipts', 1), ('config-typing-indicators', 0),
			('storage-manifest-version', 5), ('cdsi-token', x'0102'), ('last-recipients-refresh', 1900),
			('config-phone-number-sharing-mode', 'NOBODY'), ('unrestricted-unidentified-access', 1), ('config-link-previews', NULL)`,
		`INSERT INTO identity (_id, address, identity_key, added_timestamp, trust_level) VALUES
			(3, '`+peerACI+`', x'05`+strings.Repeat("0b", 32)+`', 1500, 0), (1, '`+selfACI+`', x'05`+strings.Repeat("02", 32)+`', 1000, 2)`,
		`INSERT INTO pre_key (account_id_type, key_id, public_key, private_key, stale_timestamp) VALUES
			(0, 3, x'05`+strings.Repeat("0c", 32)+`', x'`+strings.Repeat("0d", 32)+`', NULL), (1, 7, x'05`+strings.Repeat("0e", 32)+`', x'`+strings.Repeat("0f", 32)+`', 1234)`,
		`INSERT INTO signed_pre_key (account_id_type, key_id, public_key, private_key, signature, timestamp) VALUES
			(0, 4, x'05`+strings.Repeat("10", 32)+`', x'`+strings.Repeat("11", 32)+`', x'`+strings.Repeat("12", 64)+`', 4444)`,
		`INSERT INTO kyber_pre_key (account_id_type, key_id, serialized, is_last_resort) VALUES (0, 6, x'0a0b', 1), (0, 8, x'0c0d', 0)`,
		`INSERT INTO session (account_id_type, address, device_id, record) VALUES (0, '`+peerACI+`', 1, x'aa'), (1, 'PNI:`+peerPNI+`', 2, x'bb')`,
		`INSERT INTO sender_key (address, device_id, distribution_id, record, created_timestamp) VALUES ('`+peerACI+`', 1, x'`+hexOf(dist)+`', x'cc', 77)`,
		`INSERT INTO sender_key_shared (address, device_id, distribution_id, timestamp) VALUES ('`+peerACI+`', 1, x'`+hexOf(dist)+`', 88)`,
		`INSERT INTO group_v2 (_id, group_id, master_key, group_data, distribution_id, endorsement_expiration_time, profile_sharing)
			VALUES (1, x'`+strings.Repeat("20", 32)+`', x'`+strings.Repeat("21", 32)+`', x'`+hexOf(group)+`', x'`+hexOf(dist)+`', 99, 1)`,
		`INSERT INTO group_v2_member (group_id, recipient_id, endorsement) VALUES (1, 2, x'`+strings.Repeat("22", 33)+`')`,
		`INSERT INTO group_v1 (group_id, storage_id) VALUES (x'`+strings.Repeat("30", 16)+`', x'`+strings.Repeat("31", 16)+`')`,
		`INSERT INTO storage_id (type, storage_id) VALUES (8, x'`+strings.Repeat("32", 16)+`')`,
		`INSERT INTO sticker (pack_id, pack_key, installed, position) VALUES (x'`+strings.Repeat("40", 16)+`', x'`+strings.Repeat("41", 32)+`', 1, 1)`,
		`INSERT INTO message_send_log_content (_id, group_id, timestamp, content, content_hint, urgent) VALUES (1, NULL, 555, x'0801', 1, 1)`,
		`INSERT INTO message_send_log (content_id, address, device_id) VALUES (1, '`+peerACI+`', 1)`,
		`INSERT INTO cdsi (number, last_seen_at) VALUES ('`+peerNum+`', 66)`,
	)
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return src
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 2*len(b))
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&15])
	}
	return string(out)
}

func TestImportFixture(t *testing.T) {
	src := fixture(t)
	dst := t.TempDir()
	res, err := Import(Options{Source: src, Dest: dst, Open: openTest})
	if err != nil || len(res) != 1 || res[0].Err != nil {
		t.Fatalf("Import = %+v, %v", res, err)
	}
	if !strings.Contains(strings.Join(res[0].Skipped, ";"), "1 v1 group") {
		t.Errorf("skipped %q", res[0].Skipped)
	}
	a, err := store.Open(filepath.Join(dst, "data")).Find(selfNum)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.Path() != "616701" {
		t.Errorf("path %s", a.Path())
	}
	d := a.Data
	if d.Environment != "STAGING" || d.ACI != selfACI || d.PNI != selfPNI || d.DeviceID != 2 || d.Password != "secretpassword" ||
		!d.Registered || !d.MultiDevice || d.DeviceName != "ZGV2aWNl" || d.ACIRegistrationID != 101 || d.PNIRegistrationID != 102 ||
		!bytes.Equal(d.ACIIdentity.Private, rep(1, 32)) || len(d.PNIIdentity.Public) != 33 || !bytes.Equal(d.ProfileKey, rep(6, 32)) ||
		!bytes.Equal(d.MediaRootBackupKey, rep(5, 32)) || len(d.AccountEntropyPool) != 64 {
		t.Errorf("account data %+v", d)
	}
	if d.ACIKeys != (store.PreKeyState{NextPreKeyID: 11, NextSignedPreKeyID: 12, NextKyberPreKeyID: 13, ActiveSignedPreKeyID: 4, ActiveLastResortKyberID: 6}) ||
		d.PNIKeys.ActiveSignedPreKeyID != -1 || d.PNIKeys.NextPreKeyID != 21 {
		t.Errorf("prekey state %+v %+v", d.ACIKeys, d.PNIKeys)
	}
	if *d.Configuration.ReadReceipts != true || *d.Configuration.TypingIndicators != false || d.Configuration.LinkPreviews != nil ||
		d.PhoneNumberSharingMode != "NOBODY" || !d.UnrestrictedUnidentifiedAccess || !bytes.Equal(d.CDSIToken, []byte{1, 2}) ||
		d.LastRecipientsRefresh != 1900 || d.CDSINumbers[peerNum] != 66 {
		t.Errorf("settings %+v", d)
	}

	// Prekeys become libsignal records the protocol code can load.
	pk, err := protocol.LoadPreKeyRecord(a.Keys.ACI.PreKeys[3].Record)
	if err != nil || a.Keys.PNI.PreKeys[7].StaleAt != 1234 {
		t.Fatalf("prekeys: %v %+v", err, a.Keys)
	}
	if priv, err := pk.PrivateKey(); err != nil || priv == nil {
		t.Errorf("prekey private key: %v", err)
	}
	if _, err := protocol.LoadSignedPreKeyRecord(a.Keys.ACI.SignedPreKeys[4].Record); err != nil {
		t.Errorf("signed prekey: %v", err)
	}
	var spk signalpb.StoredSignedPreKey
	proto.Unmarshal(a.Keys.ACI.SignedPreKeys[4].Record, &spk)
	if spk.Timestamp != 4444 || !bytes.Equal(spk.Signature, rep(0x12, 64)) {
		t.Errorf("signed prekey record %+v", &spk)
	}
	if k := a.Keys.ACI.KyberPreKeys; !k[6].LastResort || k[8].LastResort || !bytes.Equal(k[6].Record, []byte{0x0a, 0x0b}) {
		t.Errorf("kyber %+v", k)
	}

	if !bytes.Equal(a.Sessions["aci:"+peerACI+".1"], []byte{0xaa}) || !bytes.Equal(a.Sessions["pni:PNI:"+peerPNI+".2"], []byte{0xbb}) {
		t.Errorf("sessions %v", a.Sessions)
	}
	if got := strings.Join(a.IdentityOrder(), ","); got != selfACI+","+peerACI {
		t.Errorf("identity order %s", got)
	}
	if id := a.Identities[peerACI]; id.Trust != store.Untrusted || id.Added != 1500 || a.Identities[selfACI].Trust != store.TrustedVerified {
		t.Errorf("identities %+v", a.Identities)
	}
	sk := peerACI + ".1." + distUUID
	if !bytes.Equal(a.SenderKeys.Records[sk], []byte{0xcc}) || a.SenderKeys.Created[sk] != 77 || a.SenderKeys.Shared[distUUID][peerACI+".1"] != 88 {
		t.Errorf("sender keys %+v", a.SenderKeys)
	}

	if len(a.Recipients) != 3 {
		t.Errorf("recipients %v", a.Recipients)
	}
	r := a.Recipients[peerACI]
	if r.ID != 2 || r.Number != peerNum || r.PNI != peerPNI || r.Username != "peer.01" || *r.GivenName != "Pat" || *r.NickGivenName != "Patty" ||
		r.Timer != 60 || r.TimerVersion != 3 || !r.Blocked || !r.ProfileSharing || !r.Unregistered || r.UnregisteredAt != 1700 ||
		r.Discoverable == nil || *r.Discoverable || !bytes.Equal(r.ProfileKey, rep(7, 32)) || !bytes.Equal(r.StorageID, rep(0x0a, 16)) {
		t.Errorf("peer %+v", r)
	}
	if p := r.Profile; p == nil || p.LastUpdate != 1800 || *p.GivenName != "Pat P" || p.AccessMode != store.AccessEnabled ||
		len(p.Capabilities) != 2 || *p.PhoneNumberSharing != "NOBODY" {
		t.Errorf("profile %+v", r.Profile)
	}
	if r := a.Recipients["+15555550102"]; r.ID != 6 || r.Profile != nil {
		t.Errorf("number-only row %+v", r)
	}
	if self := a.Recipients[selfACI]; self.ID != 1 || self.PNI != selfPNI {
		t.Errorf("self %+v", self)
	}

	if len(a.Groups) != 1 {
		t.Fatalf("groups %+v", a.Groups)
	}
	g := a.Groups[0]
	var st groups.State
	json.Unmarshal(g.State, &st)
	if g.DistributionID != distUUID || g.EndorsementExpiration != 99 || !g.ProfileSharing || st.Title != "G" || st.Revision != 2 ||
		st.Members[0].ACI != "a1a1a1a1-a1a1-a1a1-a1a1-a1a1a1a1a1a1" || !bytes.Equal(g.Endorsements[peerACI], rep(0x22, 33)) {
		t.Errorf("group %+v %+v", g, st)
	}
	if len(a.Stickers) != 1 || a.Stickers[0].ID != strings.Repeat("40", 16) || !a.Stickers[0].Installed || a.Stickers[0].Position != 1 {
		t.Errorf("stickers %+v", a.Stickers)
	}
	stg, _ := a.LoadStorage()
	if stg.Manifest != 5 || len(stg.IDs) != 1 || !bytes.Equal(stg.RecordIkm, rep(8, 32)) || len(stg.UnknownIDs) != 2 ||
		stg.UnknownIDs[0].Type != 8 || stg.UnknownIDs[1].Type != storageTypeGroupV1 {
		t.Errorf("storage %+v", stg)
	}
	if es, _ := a.SendLogLookup(peerACI, 1, 555, false); len(es) != 1 || es[0].Hint != 1 || !es[0].Urgent {
		t.Errorf("send log %+v", es)
	}

	// A new recipient row continues after signal-cli's sequence.
	k := a.ResolveHighTrust(store.Address{Number: "+15555550103"})
	if a.Recipients[k].ID != 10 {
		t.Errorf("new row ID %d, want 10", a.Recipients[k].ID)
	}
}

func TestImportRefusals(t *testing.T) {
	src := fixture(t)
	if _, err := Import(Options{Source: src, Dest: src, Open: openTest}); err == nil {
		t.Error("importing into the source accepted")
	}
	dst := t.TempDir()
	if res, err := Import(Options{Source: src, Dest: dst, Open: openTest}); err != nil || res[0].Err != nil {
		t.Fatalf("first import: %+v %v", res, err)
	}
	res, err := Import(Options{Source: src, Dest: dst, Open: openTest})
	if err != nil || res[0].Err == nil || !strings.Contains(res[0].Err.Error(), "already has an account") {
		t.Errorf("second import: %+v %v", res, err)
	}
	res, _ = Import(Options{Source: src, Dest: t.TempDir(), Numbers: []string{"+15555550199"}, Open: openTest})
	if len(res) != 1 || res[0].Err == nil {
		t.Errorf("unknown number: %+v", res)
	}
	db, _ := openTest(filepath.Join(src, "data", "616701.d", "account.db"))
	db.Exec("PRAGMA user_version = 29")
	db.Close()
	res, _ = Import(Options{Source: src, Dest: t.TempDir(), Open: openTest})
	if res[0].Err == nil || !strings.Contains(res[0].Err.Error(), "schema version 29") {
		t.Errorf("old schema: %+v", res)
	}
}
