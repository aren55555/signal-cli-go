// SPDX-License-Identifier: GPL-3.0-or-later

package importer

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Reading account.db (spec/storage/signal-cli-account-db.md) into a
// store.Account. Columns are always named, since migrated databases order
// them differently (§14).

// dbVersion is the schema version signal-cli 0.14.7 writes (§1).
const dbVersion = 30

func checkVersion(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v != dbVersion {
		return fmt.Errorf("account.db has schema version %d; run any account command once with signal-cli 0.14.7 to migrate it to %d, then import again", v, dbVersion)
	}
	return nil
}

// each runs query and calls fn for every row.
func each(db *sql.DB, query string, fn func(*sql.Rows) error) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// keyValues reads the key_value table (§3); NULL values are dropped.
func keyValues(db *sql.DB) (map[string]any, error) {
	kv := map[string]any{}
	err := each(db, "SELECT key, value FROM key_value", func(r *sql.Rows) error {
		var k string
		var v any
		if err := r.Scan(&k, &v); err != nil {
			return err
		}
		if v != nil {
			kv[k] = v
		}
		return nil
	})
	return kv, err
}

func kvInt(kv map[string]any, k string) (int64, bool) {
	v, ok := kv[k].(int64)
	return v, ok
}

func kvBool(kv map[string]any, k string) *bool {
	v, ok := kv[k].(int64)
	if !ok {
		return nil
	}
	b := v != 0
	return &b
}

func kvString(kv map[string]any, k string) string {
	switch v := kv[k].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

func kvBytes(kv map[string]any, k string) []byte {
	v, _ := kv[k].([]byte)
	return v
}

func ptr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

// accountScope maps account_id_type to our identity (§1.1).
func accountScope(a *store.Account, t int) (*store.IdentityKeys, string, error) {
	switch t {
	case 0:
		return &a.Keys.ACI, "aci", nil
	case 1:
		return &a.Keys.PNI, "pni", nil
	}
	return nil, "", fmt.Errorf("unknown account_id_type %d", t)
}

func stale(n sql.NullInt64) int64 {
	if n.Valid {
		return n.Int64
	}
	return 0
}

// readPreKeys fills a.Keys from pre_key, signed_pre_key and kyber_pre_key
// (§5). EC prekeys are stored as raw key columns there and become
// libsignal records here.
func readPreKeys(db *sql.DB, a *store.Account) error {
	for _, k := range []*store.IdentityKeys{&a.Keys.ACI, &a.Keys.PNI} {
		k.PreKeys, k.SignedPreKeys, k.KyberPreKeys = map[uint32]store.StoredKey{}, map[uint32]store.StoredKey{}, map[uint32]store.StoredKey{}
	}
	err := each(db, "SELECT account_id_type, key_id, public_key, private_key, stale_timestamp FROM pre_key", func(r *sql.Rows) error {
		var t int
		var id uint32
		var pub, priv []byte
		var st sql.NullInt64
		if err := r.Scan(&t, &id, &pub, &priv, &st); err != nil {
			return err
		}
		keys, _, err := accountScope(a, t)
		if err != nil {
			return err
		}
		rec, err := proto.Marshal(&signalpb.StoredPreKey{Id: id, PublicKey: pub, PrivateKey: priv})
		if err != nil {
			return err
		}
		keys.PreKeys[id] = store.StoredKey{Record: rec, StaleAt: stale(st)}
		return nil
	})
	if err != nil {
		return fmt.Errorf("pre_key: %w", err)
	}
	err = each(db, "SELECT account_id_type, key_id, public_key, private_key, signature, timestamp FROM signed_pre_key", func(r *sql.Rows) error {
		var t int
		var id uint32
		var pub, priv, sig []byte
		var ts sql.NullInt64
		if err := r.Scan(&t, &id, &pub, &priv, &sig, &ts); err != nil {
			return err
		}
		keys, _, err := accountScope(a, t)
		if err != nil {
			return err
		}
		rec, err := proto.Marshal(&signalpb.StoredSignedPreKey{Id: id, PublicKey: pub, PrivateKey: priv, Signature: sig, Timestamp: uint64(ts.Int64)})
		if err != nil {
			return err
		}
		keys.SignedPreKeys[id] = store.StoredKey{Record: rec}
		return nil
	})
	if err != nil {
		return fmt.Errorf("signed_pre_key: %w", err)
	}
	err = each(db, "SELECT account_id_type, key_id, serialized, is_last_resort, stale_timestamp FROM kyber_pre_key", func(r *sql.Rows) error {
		var t int
		var id uint32
		var rec []byte
		var last bool
		var st sql.NullInt64
		if err := r.Scan(&t, &id, &rec, &last, &st); err != nil {
			return err
		}
		keys, _, err := accountScope(a, t)
		if err != nil {
			return err
		}
		keys.KyberPreKeys[id] = store.StoredKey{Record: rec, LastResort: last, StaleAt: stale(st)}
		return nil
	})
	if err != nil {
		return fmt.Errorf("kyber_pre_key: %w", err)
	}
	return nil
}

// readSessions fills a.Sessions (§6).
func readSessions(db *sql.DB, a *store.Account) error {
	return each(db, "SELECT account_id_type, address, device_id, record FROM session", func(r *sql.Rows) error {
		var t int
		var addr string
		var dev int64
		var rec []byte
		if err := r.Scan(&t, &addr, &dev, &rec); err != nil {
			return err
		}
		_, kind, err := accountScope(a, t)
		if err != nil {
			return err
		}
		a.Sessions[kind+":"+addr+"."+strconv.FormatInt(dev, 10)] = rec
		return nil
	})
}

// trustLevels are the identity.trust_level ordinals (§4).
var trustLevels = []string{store.Untrusted, store.TrustedUnverified, store.TrustedVerified}

// readIdentities fills a.Identities (§4).
func readIdentities(db *sql.DB, a *store.Account) error {
	return each(db, "SELECT address, identity_key, added_timestamp, trust_level FROM identity ORDER BY _id", func(r *sql.Rows) error {
		var addr string
		var key []byte
		var added int64
		var level int
		if err := r.Scan(&addr, &key, &added, &level); err != nil {
			return err
		}
		if level < 0 || level >= len(trustLevels) {
			return fmt.Errorf("identity %s: unknown trust level %d", addr, level)
		}
		a.PutIdentity(addr, store.Identity{Key: key, Trust: trustLevels[level], Added: added})
		return nil
	})
}

// formatUUID writes 16 bytes in the usual textual form.
func formatUUID(b []byte) (string, error) {
	if len(b) != 16 {
		return "", fmt.Errorf("UUID of %d bytes", len(b))
	}
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// readSenderKeys fills a.SenderKeys (§7).
func readSenderKeys(db *sql.DB, a *store.Account) error {
	err := each(db, "SELECT address, device_id, distribution_id, record, created_timestamp FROM sender_key", func(r *sql.Rows) error {
		var addr string
		var dev, created int64
		var dist, rec []byte
		if err := r.Scan(&addr, &dev, &dist, &rec, &created); err != nil {
			return err
		}
		d, err := formatUUID(dist)
		if err != nil {
			return err
		}
		k := addr + "." + strconv.FormatInt(dev, 10) + "." + d
		a.SenderKeys.Records[k], a.SenderKeys.Created[k] = rec, created
		return nil
	})
	if err != nil {
		return fmt.Errorf("sender_key: %w", err)
	}
	err = each(db, "SELECT address, device_id, distribution_id, timestamp FROM sender_key_shared", func(r *sql.Rows) error {
		var addr string
		var dev, ts int64
		var dist []byte
		if err := r.Scan(&addr, &dev, &dist, &ts); err != nil {
			return err
		}
		d, err := formatUUID(dist)
		if err != nil {
			return err
		}
		if a.SenderKeys.Shared[d] == nil {
			a.SenderKeys.Shared[d] = map[string]int64{}
		}
		a.SenderKeys.Shared[d][addr+"."+strconv.FormatInt(dev, 10)] = ts
		return nil
	})
	if err != nil {
		return fmt.Errorf("sender_key_shared: %w", err)
	}
	return nil
}

// recipientRow is what the importer keeps of a recipient row besides the
// store record: its identifiers, for group endorsements.
type recipientRow struct {
	aci string
}

// readRecipients fills a.Recipients (§2) and returns the rows by _id.
// Rows without ACI, PNI and number are merge leftovers and are skipped
// (§2.1).
func readRecipients(db *sql.DB, a *store.Account) (map[int64]recipientRow, error) {
	rows := map[int64]recipientRow{}
	const cols = `_id, storage_id, storage_record, number, username, aci, pni, unregistered_timestamp, discoverable,
		profile_key, profile_key_credential, needs_pni_signature, pni_signature_verified, given_name, family_name,
		nick_name, nick_name_given_name, nick_name_family_name, note, color, expiration_time, expiration_time_version,
		mute_until, blocked, archived, profile_sharing, hide_story, hidden, profile_last_update_timestamp,
		profile_given_name, profile_family_name, profile_about, profile_about_emoji, profile_avatar_url_path,
		profile_mobile_coin_address, profile_unidentified_access_mode, profile_capabilities, profile_phone_number_sharing`
	err := each(db, "SELECT "+cols+" FROM recipient ORDER BY _id", func(r *sql.Rows) error {
		var (
			id                                           int64
			storageID, storageRecord, pk, pkc, mobcoin   []byte
			number, username, aci, pni                   sql.NullString
			unregistered                                 sql.NullInt64
			discoverable                                 sql.NullBool
			needsPNISig, pniVerified                     bool
			given, family, nick, nickGiven, nickFamily   sql.NullString
			note, color                                  sql.NullString
			timer, timerVersion, mute                    int64
			blocked, archived, sharing, hideStory, hiddn bool
			lastUpdate                                   int64
			pGiven, pFamily, pAbout, pEmoji, pAvatar     sql.NullString
			pAccess, pCaps, pSharing                     sql.NullString
		)
		if err := r.Scan(&id, &storageID, &storageRecord, &number, &username, &aci, &pni, &unregistered, &discoverable,
			&pk, &pkc, &needsPNISig, &pniVerified, &given, &family, &nick, &nickGiven, &nickFamily, &note, &color,
			&timer, &timerVersion, &mute, &blocked, &archived, &sharing, &hideStory, &hiddn, &lastUpdate,
			&pGiven, &pFamily, &pAbout, &pEmoji, &pAvatar, &mobcoin, &pAccess, &pCaps, &pSharing); err != nil {
			return err
		}
		if !aci.Valid && !pni.Valid && !number.Valid {
			return nil
		}
		rc := store.Recipient{
			ID: id, ProfileKey: pk, ProfileKeyCredential: pkc,
			Number: number.String, Username: username.String, PNI: bareUUID(pni.String),
			NeedsPNISignature: needsPNISig, PNIVerified: pniVerified,
			Timer: uint32(timer), TimerVersion: uint32(timerVersion), MuteUntil: mute,
			Blocked: blocked, Archived: archived, ProfileSharing: sharing, HideStory: hideStory, Hidden: hiddn,
			GivenName: ptr(given), FamilyName: ptr(family), NickName: ptr(nick), NickGivenName: ptr(nickGiven),
			NickFamilyName: ptr(nickFamily), Note: ptr(note), Color: ptr(color),
			StorageID: storageID, StorageRecord: storageRecord,
		}
		if unregistered.Valid {
			rc.Unregistered, rc.UnregisteredAt = true, unregistered.Int64
		}
		if discoverable.Valid {
			rc.Discoverable = &discoverable.Bool
		}
		if pCaps.Valid {
			p := &store.Profile{LastUpdate: lastUpdate, AccessMode: store.AccessUnknown,
				GivenName: ptr(pGiven), FamilyName: ptr(pFamily), About: ptr(pAbout), AboutEmoji: ptr(pEmoji),
				AvatarURLPath: ptr(pAvatar), MobileCoinAddress: mobcoin, PhoneNumberSharing: ptr(pSharing)}
			switch pAccess.String {
			case store.AccessDisabled, store.AccessEnabled, store.AccessUnrestricted:
				p.AccessMode = pAccess.String
			}
			for _, c := range strings.Split(pCaps.String, ",") {
				if c != "" {
					p.Capabilities = append(p.Capabilities, c)
				}
			}
			rc.Profile = p
		}
		key := aci.String
		switch {
		case key != "":
		case pni.Valid:
			key = "PNI:" + rc.PNI
		default:
			key = rc.Number
		}
		if _, dup := a.Recipients[key]; dup {
			return fmt.Errorf("recipient %d: duplicate identifier", id)
		}
		a.Recipients[key] = rc
		rows[id] = recipientRow{aci: aci.String}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("recipient: %w", err)
	}
	var seq sql.NullInt64
	if err := db.QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'recipient'").Scan(&seq); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite_sequence: %w", err)
	}
	a.SetRecipientsNextID(seq.Int64)
	return rows, nil
}

// readGroups fills a.Groups from group_v2 and group_v2_member (§8). It
// returns the storage IDs of v1 groups, which are kept as unknown storage
// records since signal-cli-go has no v1 groups (§9).
func readGroups(db *sql.DB, a *store.Account, recipients map[int64]recipientRow) ([]store.StorageIdent, error) {
	byRow := map[int64]int{}
	err := each(db, `SELECT _id, storage_id, storage_record, group_id, master_key, group_data, distribution_id,
		endorsement_expiration_time, blocked, profile_sharing, permission_denied FROM group_v2 ORDER BY _id`, func(r *sql.Rows) error {
		var id, expiration int64
		var storageID, storageRecord, groupID, masterKey, data, dist []byte
		var blocked, sharing, denied bool
		if err := r.Scan(&id, &storageID, &storageRecord, &groupID, &masterKey, &data, &dist, &expiration, &blocked, &sharing, &denied); err != nil {
			return err
		}
		d, err := formatUUID(dist)
		if err != nil {
			return err
		}
		g := store.GroupRecord{GroupID: groupID, MasterKey: masterKey, DistributionID: d, Blocked: blocked, ProfileSharing: sharing,
			PermissionDenied: denied, EndorsementExpiration: expiration, StorageID: storageID, StorageRecord: storageRecord}
		if data != nil {
			var pb signalpb.DecryptedGroup
			if err := proto.Unmarshal(data, &pb); err != nil {
				return fmt.Errorf("group %d: group_data: %w", id, err)
			}
			st, err := groups.FromDecrypted(&pb)
			if err != nil {
				return fmt.Errorf("group %d: %w", id, err)
			}
			if g.State, err = json.Marshal(st); err != nil {
				return err
			}
		}
		byRow[id] = len(a.Groups)
		a.Groups = append(a.Groups, g)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("group_v2: %w", err)
	}
	err = each(db, "SELECT group_id, recipient_id, endorsement FROM group_v2_member", func(r *sql.Rows) error {
		var gid, rid int64
		var e []byte
		if err := r.Scan(&gid, &rid, &e); err != nil {
			return err
		}
		i, ok := byRow[gid]
		aci := recipients[rid].aci
		if !ok || aci == "" {
			return nil
		}
		if a.Groups[i].Endorsements == nil {
			a.Groups[i].Endorsements = map[string][]byte{}
		}
		a.Groups[i].Endorsements[aci] = e
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("group_v2_member: %w", err)
	}
	var v1 []store.StorageIdent
	err = each(db, "SELECT storage_id FROM group_v1 WHERE storage_id IS NOT NULL", func(r *sql.Rows) error {
		var id []byte
		if err := r.Scan(&id); err != nil {
			return err
		}
		v1 = append(v1, store.StorageIdent{Raw: id, Type: storageTypeGroupV1})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("group_v1: %w", err)
	}
	return v1, nil
}

// storageTypeGroupV1 is the manifest identifier type of a v1 group record
// (spec/service/storage.md §4.2).
const storageTypeGroupV1 = 2

// readStickers fills a.Stickers (§11).
func readStickers(db *sql.DB, a *store.Account) error {
	return each(db, "SELECT pack_id, pack_key, installed, position, deleted_timestamp, storage_id, storage_record FROM sticker ORDER BY _id", func(r *sql.Rows) error {
		var id, key, storageID, storageRecord []byte
		var installed bool
		var position, deleted int64
		if err := r.Scan(&id, &key, &installed, &position, &deleted, &storageID, &storageRecord); err != nil {
			return err
		}
		a.Stickers = append(a.Stickers, store.StickerPack{ID: hex.EncodeToString(id), Key: key, Installed: installed,
			Position: uint32(position), DeletedAt: deleted, StorageID: storageID, StorageRecord: storageRecord})
		return nil
	})
}

// readUnknownStorage reads the storage_id table (§10).
func readUnknownStorage(db *sql.DB) ([]store.StorageIdent, error) {
	var out []store.StorageIdent
	err := each(db, "SELECT type, storage_id FROM storage_id ORDER BY _id", func(r *sql.Rows) error {
		var t int32
		var id []byte
		if err := r.Scan(&t, &id); err != nil {
			return err
		}
		out = append(out, store.StorageIdent{Raw: id, Type: t})
		return nil
	})
	return out, err
}

// readCDSI reads the cdsi table (§13).
func readCDSI(db *sql.DB) (map[string]int64, error) {
	out := map[string]int64{}
	err := each(db, "SELECT number, last_seen_at FROM cdsi", func(r *sql.Rows) error {
		var n string
		var seen int64
		if err := r.Scan(&n, &seen); err != nil {
			return err
		}
		out[n] = seen
		return nil
	})
	return out, err
}

// readSendLog copies the message send log (§12) into ours.
func readSendLog(db *sql.DB, a *store.Account) error {
	entries := map[int64]*store.SendLogEntry{}
	var order []int64
	err := each(db, "SELECT _id, group_id, timestamp, content, content_hint, urgent FROM message_send_log_content ORDER BY _id", func(r *sql.Rows) error {
		var id, ts, hint int64
		var gid, content []byte
		var urgent bool
		if err := r.Scan(&id, &gid, &ts, &content, &hint, &urgent); err != nil {
			return err
		}
		entries[id] = &store.SendLogEntry{Timestamp: uint64(ts), Content: content, Hint: uint32(hint), Urgent: urgent, GroupID: gid}
		order = append(order, id)
		return nil
	})
	if err != nil {
		return fmt.Errorf("message_send_log_content: %w", err)
	}
	err = each(db, "SELECT content_id, address, device_id FROM message_send_log ORDER BY _id", func(r *sql.Rows) error {
		var cid, dev int64
		var addr string
		if err := r.Scan(&cid, &addr, &dev); err != nil {
			return err
		}
		if e := entries[cid]; e != nil {
			e.Recipients = append(e.Recipients, store.SendLogRecipient{Address: addr, Device: uint32(dev)})
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("message_send_log: %w", err)
	}
	for _, id := range order {
		if e := entries[id]; len(e.Recipients) > 0 {
			if _, err := a.SendLogAdd(*e); err != nil {
				return err
			}
		}
	}
	return nil
}
