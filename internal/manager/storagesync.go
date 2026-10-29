// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/storage"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// The storage-service sync (spec/service/storage.md §5, §6).
//
// Deviations from signal-cli 0.14.7, both deliberate:
//   - an entity gets a new storage ID when its record built from local
//     state differs from the last stored record, not on every local
//     store (the remote outcome is the same, minus pointless rewrites);
//   - a read batch with an undecryptable item aborts the sync instead of
//     letting the next write delete those items remotely (§9 Gap 4).
// v1 groups are not supported, so GroupV1 records are kept as
// unprocessed IDs.

// Record types (§4.2).
const (
	storageContact = 1
	storageGroupV1 = 2
	storageGroupV2 = 3
	storageAccount = 4
	storageSticker = 6
)

// Storage runs syncs for one account.
type Storage struct {
	Sender *Sender
	Groups *Groups
	REST   Raw // the storage host
	Log    func(format string, args ...any)
}

func (st *Storage) logf(f string, args ...any) {
	if st.Log != nil {
		st.Log(f, args...)
	}
}

// errStorageConflict is a 409 on a write or force push.
var errStorageConflict = errors.New("storage conflict")

// StorageKey is the storage key of §2.2; ok is false without one.
func StorageKeyOf(a *store.Account) ([32]byte, bool) {
	if a.Data.AccountEntropyPool == "" {
		return [32]byte{}, false
	}
	return storage.StorageKey(storage.MasterKey(a.Data.AccountEntropyPool)), true
}

func (st *Storage) auth(ctx context.Context) (string, error) {
	r, err := st.Sender.Chat.Request(ctx, "GET", "/v1/storage/auth", nil, nil)
	if err != nil {
		return "", err
	}
	if r.Status != 200 {
		return "", fmt.Errorf("storage auth: status %d", r.Status)
	}
	var c struct{ Username, Password string }
	if err := json.Unmarshal(r.Body, &c); err != nil {
		return "", err
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.Username+":"+c.Password)), nil
}

func (st *Storage) request(ctx context.Context, method, path string, body proto.Message) (int, []byte, error) {
	h, err := st.auth(ctx)
	if err != nil {
		return 0, nil, err
	}
	headers := map[string]string{"Authorization": h}
	var b []byte
	if body != nil {
		if b, err = proto.Marshal(body); err != nil {
			return 0, nil, err
		}
		headers["Content-Type"] = "application/x-protobuf"
	}
	r, err := st.REST.Raw(ctx, method, path, headers, b)
	if err != nil {
		return 0, nil, err
	}
	return r.Status, r.Body, nil
}

// manifest is a decrypted manifest.
type manifest struct {
	version   uint64
	ids       []store.StorageIdent
	recordIkm []byte
}

func identsOf(m *signalpb.ManifestRecord) []store.StorageIdent {
	var out []store.StorageIdent
	for _, id := range m.GetIdentifiers() {
		out = append(out, store.StorageIdent{Raw: id.GetRaw(), Type: id.GetType()})
	}
	return out
}

// Sync is one storage sync job (§6.4). Errors are returned for the
// caller to log as "Failed to sync storage data".
func (st *Storage) Sync(ctx context.Context) error {
	s := st.Sender
	a := s.Acct
	primary := a.Data.DeviceID == 1
	key, ok := StorageKeyOf(a)
	if !ok {
		if !primary {
			return s.SendSync(ctx, &signalpb.AppSyncMessage{Request: &signalpb.AppSyncMessage_Request{Type: signalpb.AppSyncMessage_Request_KEYS.Enum()}}, false)
		}
		return nil
	}
	local, err := a.LoadStorage()
	if err != nil {
		return err
	}
	ml := &manifest{version: uint64(max(local.Manifest, 0)), ids: local.IDs, recordIkm: local.RecordIkm}
	origL := local.Manifest

	// 2. Fetch.
	status, body, err := st.request(ctx, "GET", "/v1/storage/manifest/version/"+strconv.FormatInt(local.Manifest, 10), nil)
	if err != nil {
		return err
	}
	var r *manifest
	forcePush := false
	switch status {
	case 200:
		var sm signalpb.StorageManifest
		if err := proto.Unmarshal(body, &sm); err != nil {
			return err
		}
		plain, derr := storage.Open(storage.ManifestKey(key, sm.GetVersion()), sm.GetValue())
		if derr != nil {
			st.logf("Manifest couldn't be decrypted.")
			if !primary {
				return s.SendSync(ctx, &signalpb.AppSyncMessage{Request: &signalpb.AppSyncMessage_Request{Type: signalpb.AppSyncMessage_Request_KEYS.Enum()}}, false)
			}
			forcePush = true
			break
		}
		var mr signalpb.ManifestRecord
		if err := proto.Unmarshal(plain, &mr); err != nil {
			return err
		}
		r = &manifest{version: mr.GetVersion(), ids: identsOf(&mr), recordIkm: mr.GetRecordIkm()}
		local.Manifest, local.IDs, local.RecordIkm = int64(r.version), r.ids, r.recordIkm
		if err := a.SaveStorage(local); err != nil {
			return err
		}
	case 204:
		r = ml
	default:
		return fmt.Errorf("storage manifest: status %d", status)
	}

	// 3. Read.
	if r != nil && r != ml && int64(r.version) > origL {
		remoteOnly, _, mismatch := diffIDs(r.ids, ml.ids)
		if mismatch && primary {
			forcePush = true
		}
		if err := st.read(ctx, key, r, remoteOnly, local); err != nil {
			return err
		}
	}

	// 5. Fill missing IDs (and rotate changed records).
	st.assignIDs()

	// 6. Decide.
	written := false
	if r == nil {
		if primary {
			forcePush = true
		}
	} else {
		w, err := st.write(ctx, key, r, local)
		if errors.Is(err, errStorageConflict) {
			return nil
		}
		if err != nil {
			return err
		}
		written = w
	}
	if forcePush {
		err := st.forcePush(ctx, key, local)
		if errors.Is(err, errStorageConflict) {
			return nil
		}
		if err != nil {
			return err
		}
		written = true
	}
	if written {
		if err := s.SendFetchLatest(ctx, signalpb.AppSyncMessage_FetchLatest_STORAGE_MANIFEST); err != nil {
			st.logf("Failed to send storage manifest sync: %v", err)
		}
	}
	return nil
}

// diffIDs is §6.4.1: remote-only and local-only raw IDs, and whether a
// type mismatch or duplicate was seen.
func diffIDs(remote, local []store.StorageIdent) (remoteOnly, localOnly []store.StorageIdent, mismatch bool) {
	index := func(list []store.StorageIdent) map[string]store.StorageIdent {
		m := map[string]store.StorageIdent{}
		for _, id := range list {
			k := string(id.Raw)
			if _, dup := m[k]; dup {
				mismatch = true
			}
			m[k] = id
		}
		return m
	}
	rm, lm := index(remote), index(local)
	for _, id := range remote {
		l, ok := lm[string(id.Raw)]
		switch {
		case !ok:
			remoteOnly = append(remoteOnly, id)
		case l.Type != id.Type && knownType(l.Type):
			mismatch = true
		}
	}
	for _, id := range local {
		if _, ok := rm[string(id.Raw)]; !ok {
			localOnly = append(localOnly, id)
		}
	}
	return remoteOnly, localOnly, mismatch
}

func knownType(t int32) bool {
	return t == storageContact || t == storageGroupV2 || t == storageAccount || t == storageSticker
}

// readItems fetches and decrypts items (§5.2, §6.4.2 step 1). An
// undecryptable item fails the whole call (our deviation: the sync
// stops instead of deleting).
func (st *Storage) readItems(ctx context.Context, key [32]byte, m *manifest, ids []store.StorageIdent) (map[string]*signalpb.StorageRecord, error) {
	out := map[string]*signalpb.StorageRecord{}
	var want [][]byte
	for _, id := range ids {
		if id.Type >= 1 && id.Type <= 9 {
			want = append(want, id.Raw)
		}
	}
	for len(want) > 0 {
		n := min(1000, len(want))
		status, body, err := st.request(ctx, "PUT", "/v1/storage/read", &signalpb.StorageReadOperation{ReadKey: want[:n]})
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("storage read: status %d", status)
		}
		var items signalpb.StorageItems
		if err := proto.Unmarshal(body, &items); err != nil {
			return nil, err
		}
		for _, it := range items.GetItems() {
			plain, err := storage.Open(storage.ItemKey(key, m.recordIkm, it.GetKey()), it.GetValue())
			if err != nil {
				st.logf("Failed to read storage records, ignoring.")
				return nil, errors.New("undecryptable storage item; not syncing")
			}
			var rec signalpb.StorageRecord
			if err := proto.Unmarshal(plain, &rec); err != nil {
				return nil, err
			}
			out[string(it.GetKey())] = &rec
		}
		want = want[n:]
	}
	return out, nil
}

// read processes remote-only records (§6.4.2).
func (st *Storage) read(ctx context.Context, key [32]byte, r *manifest, remoteOnly []store.StorageIdent, local *store.StorageState) error {
	a := st.Sender.Acct
	recs, err := st.readItems(ctx, key, r, remoteOnly)
	if err != nil {
		return err
	}
	matched := map[string]bool{}
	for _, id := range remoteOnly {
		rec, ok := recs[string(id.Raw)]
		if !ok {
			continue
		}
		switch {
		case rec.GetAccount() != nil:
			st.processAccount(id.Raw, rec.GetAccount(), matched)
		case rec.GetGroupV2() != nil:
			st.processGroupV2(id.Raw, rec.GetGroupV2(), matched)
		case rec.GetContact() != nil:
			st.processContact(ctx, id.Raw, rec.GetContact(), matched)
		case rec.GetStickerPack() != nil:
			st.processSticker(id.Raw, rec.GetStickerPack(), matched)
		default:
			local.UnknownIDs = append(slicesDeleteRaw(local.UnknownIDs, id.Raw), id)
		}
	}
	a.SaveRecipients()
	a.SaveGroups()
	a.SaveStickers()
	return a.SaveStorage(local)
}

func slicesDeleteRaw(list []store.StorageIdent, raw []byte) []store.StorageIdent {
	var out []store.StorageIdent
	for _, id := range list {
		if !bytes.Equal(id.Raw, raw) {
			out = append(out, id)
		}
	}
	return out
}

// pickResult is the merge outcome rule of §6.4.2: R (remote ID) if the
// merge equals R, Lc (local ID) if it equals Lc, else M with a new ID.
func pickResult(merged, remote, localRec proto.Message, remoteID, localID []byte) (proto.Message, []byte) {
	switch {
	case proto.Equal(merged, remote):
		return remote, remoteID
	case localRec != nil && proto.Equal(merged, localRec):
		return localRec, localID
	}
	return merged, storage.NewRawID()
}

// clearStorageID removes raw from every entity before it is assigned.
func (st *Storage) clearStorageID(raw []byte) {
	a := st.Sender.Acct
	for k, rc := range a.Recipients {
		if bytes.Equal(rc.StorageID, raw) {
			rc.StorageID = nil
			a.Recipients[k] = rc
		}
	}
	for i := range a.Groups {
		if bytes.Equal(a.Groups[i].StorageID, raw) {
			a.Groups[i].StorageID = nil
		}
	}
	for i := range a.Stickers {
		if bytes.Equal(a.Stickers[i].StorageID, raw) {
			a.Stickers[i].StorageID = nil
		}
	}
}

// --- contacts -------------------------------------------------------------

var e164Re = regexp.MustCompile(`^\+[1-9][0-9]{6,18}$`)

func storedContact(rc store.Recipient) *signalpb.ContactRecord {
	c := &signalpb.ContactRecord{}
	if len(rc.StorageRecord) > 0 {
		proto.Unmarshal(rc.StorageRecord, c)
	}
	return c
}

func uuidString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func recordACI(c *signalpb.ContactRecord) string {
	if s := uuidString(c.GetAciBinary()); s != "" {
		return s
	}
	if _, ok := uuidRaw(c.GetAci()); ok {
		return strings.ToLower(c.GetAci())
	}
	return ""
}

func recordPNI(c *signalpb.ContactRecord) string {
	b := c.GetPniBinary()
	if len(b) == 17 && b[0] == 1 {
		b = b[1:]
	}
	if s := uuidString(b); s != "" {
		return s
	}
	if _, ok := uuidRaw(c.GetPni()); ok {
		return strings.ToLower(c.GetPni())
	}
	return ""
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// contactRecord builds the local → remote record (§6.4.3).
func (st *Storage) contactRecord(key string) *signalpb.ContactRecord {
	a := st.Sender.Acct
	rc := a.Recipients[key]
	ad := a.AddressOf(key)
	c := storedContact(rc)
	c.E164, c.Username, c.PniSignatureVerified = ad.Number, ad.Username, rc.PNIVerified
	c.ProfileKey = rc.ProfileKey
	c.Aci, c.AciBinary, c.Pni, c.PniBinary = "", nil, "", nil
	if u, ok := uuidRaw(ad.ACI); ok {
		c.Aci, c.AciBinary = ad.ACI, u[:]
	}
	if u, ok := uuidRaw(ad.PNI); ok {
		c.Pni, c.PniBinary = ad.PNI, u[:]
	}
	if rc.Profile != nil {
		c.GivenName, c.FamilyName = str(rc.Profile.GivenName), str(rc.Profile.FamilyName)
	}
	if rc.IsContact() || rc.Hidden {
		c.SystemGivenName, c.SystemFamilyName, c.SystemNickname, c.Note = str(rc.GivenName), str(rc.FamilyName), str(rc.NickName), str(rc.Note)
		c.Nickname = nil
		if str(rc.NickGivenName) != "" || str(rc.NickFamilyName) != "" {
			c.Nickname = &signalpb.ContactRecord_Name{Given: str(rc.NickGivenName), Family: str(rc.NickFamilyName)}
		}
		c.Blocked, c.Whitelisted, c.MutedUntilTimestamp, c.HideStory, c.Archived, c.Hidden = rc.Blocked, rc.ProfileSharing, uint64(rc.MuteUntil), rc.HideStory, rc.Archived, rc.Hidden
	}
	c.UnregisteredAtTimestamp = uint64(rc.UnregisteredAt)
	c.IdentityKey, c.IdentityState = nil, signalpb.ContactRecord_DEFAULT
	if ad.ACI != "" {
		if id, ok := a.Identities[ad.ACI]; ok {
			c.IdentityKey = id.Key
			switch id.Trust {
			case store.TrustedVerified:
				c.IdentityState = signalpb.ContactRecord_VERIFIED
			case store.Untrusted:
				c.IdentityState = signalpb.ContactRecord_UNVERIFIED
			}
		}
	}
	return c
}

func (st *Storage) processContact(ctx context.Context, raw []byte, r *signalpb.ContactRecord, matched map[string]bool) {
	a := st.Sender.Acct
	aci, pni := recordACI(r), recordPNI(r)
	if (aci == "" && pni == "") || (aci != "" && aci == a.Data.ACI) || (pni != "" && pni == a.Data.PNI) ||
		(r.GetE164() != "" && (r.GetE164() == a.Data.Number || !e164Re.MatchString(r.GetE164()))) {
		return // invalid
	}
	key := a.ResolveHighTrust(store.Address{ACI: aci, PNI: pni, Number: r.GetE164(), Username: r.GetUsername()})
	if matched["c"+key] {
		return
	}
	matched["c"+key] = true
	rc := a.Recipients[key]
	if rc.StorageID == nil {
		rc.StorageID = storage.NewRawID()
		a.Recipients[key] = rc
	}
	lc := st.contactRecord(key)
	primary := a.Data.DeviceID == 1

	m := proto.Clone(r).(*signalpb.ContactRecord)
	if r.GetGivenName() == "" && r.GetFamilyName() == "" {
		m.GivenName, m.FamilyName = lc.GetGivenName(), lc.GetFamilyName()
	}
	if len(r.GetIdentityKey()) > 0 && (!primary || r.GetIdentityState() != lc.GetIdentityState() || len(lc.GetIdentityKey()) == 0) {
		m.IdentityKey, m.IdentityState = r.GetIdentityKey(), r.GetIdentityState()
	} else {
		m.IdentityState = lc.GetIdentityState()
		m.IdentityKey = nil
		if (recordACI(lc) != "" || recordPNI(lc) != "") && len(lc.GetIdentityKey()) > 0 && len(r.GetIdentityKey()) > 0 {
			m.IdentityKey = lc.GetIdentityKey()
		}
	}
	if lp, rp := recordPNI(lc), recordPNI(r); lp != "" && rp == "" {
		m.Pni = lp
	}
	if r.GetE164() == "" {
		m.E164 = lc.GetE164()
	}
	if primary {
		m.SystemGivenName, m.SystemFamilyName = lc.GetSystemGivenName(), lc.GetSystemFamilyName()
	}
	if len(r.GetProfileKey()) != 32 {
		m.ProfileKey = lc.GetProfileKey()
	}
	if r.GetUsername() == "" {
		m.Username = lc.GetUsername()
	}
	m.PniSignatureVerified = r.GetPniSignatureVerified() || lc.GetPniSignatureVerified()
	if la := recordACI(lc); la != "" {
		if len(r.GetAciBinary()) == 0 && r.GetAci() == "" {
			u, _ := uuidRaw(la)
			m.Aci, m.AciBinary = la, u[:]
		} else {
			u, _ := uuidRaw(la)
			if r.GetAci() != "" {
				m.Aci = la
			}
			if len(r.GetAciBinary()) > 0 {
				m.AciBinary = u[:]
			}
		}
	}
	res, id := pickResult(m, r, lc, raw, rc.StorageID)
	result := res.(*signalpb.ContactRecord)
	if bytes.Equal(id, rc.StorageID) && proto.Equal(result, lc) {
		return
	}
	st.applyContact(key, result)
	st.clearStorageID(id)
	rc = a.Recipients[key]
	rc.StorageID = id
	rc.StorageRecord, _ = proto.Marshal(result)
	a.Recipients[key] = rc
}

// applyContact is "local update from a contact record" (§6.4.3).
func (st *Storage) applyContact(key string, r *signalpb.ContactRecord) {
	a := st.Sender.Acct
	rc := a.Recipients[key]
	rc.Blocked, rc.ProfileSharing, rc.Archived, rc.Hidden, rc.HideStory = r.GetBlocked(), r.GetWhitelisted(), r.GetArchived(), r.GetHidden(), r.GetHideStory()
	rc.MuteUntil, rc.UnregisteredAt = int64(r.GetMutedUntilTimestamp()), int64(r.GetUnregisteredAtTimestamp())
	rc.Unregistered = rc.UnregisteredAt != 0
	rc.GivenName, rc.FamilyName, rc.NickName = nonEmpty(r.GetSystemGivenName()), nonEmpty(r.GetSystemFamilyName()), nonEmpty(r.GetSystemNickname())
	rc.NickGivenName, rc.NickFamilyName, rc.Note = nonEmpty(r.GetNickname().GetGiven()), nonEmpty(r.GetNickname().GetFamily()), nonEmpty(r.GetNote())
	if g, f := r.GetGivenName(), r.GetFamilyName(); rc.Profile == nil || str(rc.Profile.GivenName) != g || str(rc.Profile.FamilyName) != f {
		p := copyProfile(rc.Profile)
		if rc.Profile != nil {
			p.PhoneNumberSharing = rc.Profile.PhoneNumberSharing
		}
		p.GivenName, p.FamilyName = &g, &f
		rc.Profile = p
	}
	a.Recipients[key] = rc
	if pk := r.GetProfileKey(); len(pk) > 0 {
		if len(pk) == 32 {
			StoreProfileKey(a, key, pk)
		} else {
			st.logf("Received invalid contact profile key from storage")
		}
	}
	rc = a.Recipients[key]
	if ik := r.GetIdentityKey(); len(ik) > 0 {
		if aci := a.AddressOf(key).ACI; aci != "" {
			if len(ik) != 33 || ik[0] != 5 {
				st.logf("Received invalid contact identity key from storage")
			} else {
				trust := store.TrustedUnverified
				switch r.GetIdentityState() {
				case signalpb.ContactRecord_VERIFIED:
					trust = store.TrustedVerified
				case signalpb.ContactRecord_UNVERIFIED:
					trust = store.Untrusted
				}
				a.PutIdentity(aci, store.Identity{Key: ik, Trust: trust, Added: st.Sender.now().UnixMilli()})
				a.SaveIdentities()
			}
		}
	}
	rc.PNIVerified = r.GetPniSignatureVerified()
	a.Recipients[key] = rc
}

// --- account --------------------------------------------------------------

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func (st *Storage) accountRecord() *signalpb.AccountRecord {
	a := st.Sender.Acct
	rc := a.Recipients[a.Data.ACI]
	r := &signalpb.AccountRecord{}
	if len(rc.StorageRecord) > 0 {
		proto.Unmarshal(rc.StorageRecord, r)
	}
	if len(a.Data.ProfileKey) == 32 {
		r.ProfileKey = a.Data.ProfileKey
	}
	if p := rc.Profile; p != nil {
		r.GivenName, r.FamilyName, r.AvatarUrlPath = str(p.GivenName), str(p.FamilyName), str(p.AvatarURLPath)
	}
	r.TypingIndicators, r.ReadReceipts = boolOr(a.Data.Configuration.TypingIndicators, true), boolOr(a.Data.Configuration.ReadReceipts, true)
	r.SealedSenderIndicators, r.LinkPreviews = boolOr(a.Data.Configuration.UnidentifiedDeliveryIndicators, true), boolOr(a.Data.Configuration.LinkPreviews, true)
	r.UnlistedPhoneNumber = a.Data.Unlisted
	switch a.Data.PhoneNumberSharingMode {
	case "EVERYBODY":
		r.PhoneNumberSharingMode = 1
	case "CONTACTS", "NOBODY":
		r.PhoneNumberSharingMode = 2
	default:
		r.PhoneNumberSharingMode = 0
	}
	r.Username = a.Data.Username
	if len(a.Data.UsernameLinkEntropy) == 32 && a.Data.UsernameLinkServerID != "" {
		id, _ := uuidRaw(a.Data.UsernameLinkServerID)
		color := int32(0)
		if r.UsernameLink != nil {
			color = r.UsernameLink.Color
		}
		r.UsernameLink = &signalpb.AccountRecord_UsernameLink{Entropy: a.Data.UsernameLinkEntropy, ServerId: id[:], Color: color}
	}
	return r
}

func (st *Storage) processAccount(raw []byte, r *signalpb.AccountRecord, matched map[string]bool) {
	if matched["account"] {
		return
	}
	matched["account"] = true
	a := st.Sender.Acct
	a.EnsureSelf()
	rc := a.Recipients[a.Data.ACI]
	if rc.StorageID == nil {
		rc.StorageID = storage.NewRawID()
		a.Recipients[a.Data.ACI] = rc
	}
	lc := st.accountRecord()
	m := proto.Clone(r).(*signalpb.AccountRecord)
	if r.GetGivenName() == "" && r.GetFamilyName() == "" {
		m.GivenName, m.FamilyName = lc.GetGivenName(), lc.GetFamilyName()
	}
	if r.GetAvatarUrlPath() == "" {
		m.AvatarUrlPath = lc.GetAvatarUrlPath()
	}
	if len(r.GetProfileKey()) == 0 {
		m.ProfileKey = lc.GetProfileKey()
	}
	if r.GetSubscriberCurrencyCode() == "" {
		m.SubscriberCurrencyCode = lc.GetSubscriberCurrencyCode()
	}
	if len(r.GetPreferredReactionEmoji()) == 0 {
		m.PreferredReactionEmoji = lc.GetPreferredReactionEmoji()
	}
	m.HasViewedOnboardingStory = r.GetHasViewedOnboardingStory() || lc.GetHasViewedOnboardingStory()
	m.HasSeenGroupStoryEducationSheet = r.GetHasSeenGroupStoryEducationSheet() || lc.GetHasSeenGroupStoryEducationSheet()
	m.HasCompletedUsernameOnboarding = r.GetHasCompletedUsernameOnboarding() || lc.GetHasCompletedUsernameOnboarding()
	if r.GetStoryViewReceiptsEnabled() == signalpb.StorageOptionalBool_OPTIONAL_BOOL_UNSET {
		m.StoryViewReceiptsEnabled = lc.GetStoryViewReceiptsEnabled()
	}
	src := r.GetPayments()
	if src == nil || len(src.GetEntropy()) == 0 {
		src = lc.GetPayments()
	}
	pay := &signalpb.AccountRecord_Payments{}
	if len(src.GetEntropy()) == 32 {
		pay.Enabled, pay.Entropy = src.GetEnabled(), src.GetEntropy()
	}
	m.Payments = pay
	if len(r.GetSubscriberId()) == 0 {
		m.SubscriberId, m.SubscriberCurrencyCode = lc.GetSubscriberId(), lc.GetSubscriberCurrencyCode()
	}
	if len(m.GetSubscriberId()) != 32 || strings.TrimSpace(m.GetSubscriberCurrencyCode()) == "" {
		m.SubscriberId, m.SubscriberCurrencyCode = nil, ""
	}
	bsd := r.GetBackupSubscriberData()
	if len(bsd.GetSubscriberId()) == 0 {
		bsd = lc.GetBackupSubscriberData()
	}
	if bsd != nil && (len(bsd.GetSubscriberId()) != 32 || (strings.TrimSpace(bsd.GetPurchaseToken()) == "" && bsd.GetOriginalTransactionId() == 0)) {
		bsd = nil
	}
	m.BackupSubscriberData = bsd

	res, id := pickResult(m, r, lc, raw, rc.StorageID)
	result := res.(*signalpb.AccountRecord)
	if bytes.Equal(id, rc.StorageID) && proto.Equal(result, lc) {
		return
	}
	st.applyAccount(result)
	st.clearStorageID(id)
	rc = a.Recipients[a.Data.ACI]
	rc.StorageID = id
	rc.StorageRecord, _ = proto.Marshal(result)
	a.Recipients[a.Data.ACI] = rc
}

func (st *Storage) applyAccount(r *signalpb.AccountRecord) {
	a := st.Sender.Acct
	b := func(v bool) *bool { return &v }
	a.Data.Configuration.ReadReceipts, a.Data.Configuration.TypingIndicators = b(r.GetReadReceipts()), b(r.GetTypingIndicators())
	a.Data.Configuration.UnidentifiedDeliveryIndicators, a.Data.Configuration.LinkPreviews = b(r.GetSealedSenderIndicators()), b(r.GetLinkPreviews())
	a.Data.Unlisted = r.GetUnlistedPhoneNumber()
	switch r.GetPhoneNumberSharingMode() {
	case 1:
		a.Data.PhoneNumberSharingMode = "EVERYBODY"
	case 2:
		a.Data.PhoneNumberSharingMode = "NOBODY"
	default:
		a.Data.PhoneNumberSharingMode = ""
	}
	a.Data.Username = r.GetUsername()
	if l := r.GetUsernameLink(); l != nil {
		a.Data.UsernameLinkEntropy, a.Data.UsernameLinkServerID = l.GetEntropy(), uuidString(l.GetServerId())
	}
	if pk := r.GetProfileKey(); len(pk) == 32 {
		a.Data.ProfileKey = pk
	}
	a.Save()
	rc := a.Recipients[a.Data.ACI]
	p := copyProfile(rc.Profile)
	if rc.Profile != nil {
		p.PhoneNumberSharing = rc.Profile.PhoneNumberSharing
	}
	g, f := r.GetGivenName(), r.GetFamilyName()
	p.GivenName, p.FamilyName = &g, &f
	rc.Profile = p
	a.Recipients[a.Data.ACI] = rc
}

// --- groups and sticker packs ---------------------------------------------

func (st *Storage) groupRecord(rec *store.GroupRecord) *signalpb.GroupV2Record {
	r := &signalpb.GroupV2Record{}
	if len(rec.StorageRecord) > 0 {
		proto.Unmarshal(rec.StorageRecord, r)
	}
	r.MasterKey, r.Blocked, r.Whitelisted = rec.MasterKey, rec.Blocked, rec.ProfileSharing
	return r
}

func (st *Storage) processGroupV2(raw []byte, r *signalpb.GroupV2Record, matched map[string]bool) {
	if len(r.GetMasterKey()) != 32 {
		return
	}
	a := st.Sender.Acct
	rec := (&Groups{Acct: a}).recordFor(r.GetMasterKey())
	if stored := a.Group(rec.GroupID); stored != nil {
		rec = stored
	} else {
		a.Groups = append(a.Groups, *rec)
		rec = a.Group(rec.GroupID)
	}
	k := "g" + string(rec.GroupID)
	if matched[k] {
		return
	}
	matched[k] = true
	lc := st.groupRecord(rec)
	res, id := pickResult(r, r, lc, raw, rec.StorageID)
	result := res.(*signalpb.GroupV2Record)
	if bytes.Equal(id, rec.StorageID) && proto.Equal(result, lc) {
		return
	}
	st.clearStorageID(id)
	rec = a.Group(rec.GroupID)
	rec.Blocked, rec.ProfileSharing = result.GetBlocked(), result.GetWhitelisted()
	rec.StorageID = id
	rec.StorageRecord, _ = proto.Marshal(result)
}

func (st *Storage) stickerRecord(p *store.StickerPack) *signalpb.StickerPackRecord {
	r := &signalpb.StickerPackRecord{}
	if len(p.StorageRecord) > 0 {
		proto.Unmarshal(p.StorageRecord, r)
	}
	r.PackId, _ = hex.DecodeString(p.ID)
	if p.DeletedAt != 0 {
		r.PackKey, r.Position, r.DeletedAtTimestamp = nil, 0, uint64(p.DeletedAt)
	} else {
		r.PackKey, r.Position, r.DeletedAtTimestamp = p.Key, p.Position, 0
	}
	return r
}

func (st *Storage) processSticker(raw []byte, r *signalpb.StickerPackRecord, matched map[string]bool) {
	if len(r.GetPackId()) != 16 || (r.GetDeletedAtTimestamp() == 0 && len(r.GetPackKey()) != 32) {
		return
	}
	a := st.Sender.Acct
	id := hex.EncodeToString(r.GetPackId())
	var p *store.StickerPack
	for i := range a.Stickers {
		if a.Stickers[i].ID == id {
			p = &a.Stickers[i]
		}
	}
	if p == nil {
		a.Stickers = append(a.Stickers, store.StickerPack{ID: id})
		p = &a.Stickers[len(a.Stickers)-1]
	}
	if matched["s"+id] {
		return
	}
	matched["s"+id] = true
	lc := st.stickerRecord(p)
	result, sid := proto.Message(r), raw
	if r.GetDeletedAtTimestamp() != 0 && p.DeletedAt != 0 && uint64(p.DeletedAt) > r.GetDeletedAtTimestamp() && p.StorageID != nil {
		result, sid = lc, p.StorageID
	}
	rr := result.(*signalpb.StickerPackRecord)
	st.clearStorageID(sid)
	for i := range a.Stickers {
		if a.Stickers[i].ID == id {
			p = &a.Stickers[i]
		}
	}
	p.Installed, p.DeletedAt = rr.GetDeletedAtTimestamp() == 0, int64(rr.GetDeletedAtTimestamp())
	p.Position = rr.GetPosition()
	if len(rr.GetPackKey()) > 0 {
		p.Key = rr.GetPackKey()
	}
	p.StorageID = sid
	p.StorageRecord, _ = proto.Marshal(rr)
}

// --- IDs, write, force push -------------------------------------------------

// assignIDs gives eligible entities without an ID one (§6.3 last bullet)
// and a new one to entities whose built record changed.
func (st *Storage) assignIDs() {
	a := st.Sender.Acct
	a.EnsureSelf()
	for k, rc := range a.Recipients {
		ad := a.AddressOf(k)
		if k == a.Data.ACI {
			if rc.StorageID == nil || !sameRecord(rc.StorageRecord, st.accountRecord()) {
				rc.StorageID = storage.NewRawID()
				a.Recipients[k] = rc
			}
			continue
		}
		if ad.ACI == "" && ad.PNI == "" {
			continue
		}
		if (rc.StorageID == nil && !rc.Unregistered) || (rc.StorageID != nil && !sameRecord(rc.StorageRecord, st.contactRecord(k))) {
			rc.StorageID = storage.NewRawID()
			a.Recipients[k] = rc
		}
	}
	for i := range a.Groups {
		g := &a.Groups[i]
		if g.StorageID == nil || !sameRecord(g.StorageRecord, st.groupRecord(g)) {
			g.StorageID = storage.NewRawID()
		}
	}
	for i := range a.Stickers {
		p := &a.Stickers[i]
		if (p.Installed || p.DeletedAt != 0) && (p.StorageID == nil || !sameRecord(p.StorageRecord, st.stickerRecord(p))) {
			p.StorageID = storage.NewRawID()
		}
	}
	a.SaveRecipients()
	a.SaveGroups()
	a.SaveStickers()
}

func sameRecord(stored []byte, built proto.Message) bool {
	if len(stored) == 0 {
		return false
	}
	old := built.ProtoReflect().New().Interface()
	if proto.Unmarshal(stored, old) != nil {
		return false
	}
	return proto.Equal(old, built)
}

type localEntry struct {
	id  store.StorageIdent
	rec *signalpb.StorageRecord // nil for unknown IDs
	set func(raw, record []byte)
}

// localSet is §6.2's local ID set with the records to insert.
func (st *Storage) localSet(local *store.StorageState) []localEntry {
	a := st.Sender.Acct
	var out []localEntry
	for _, id := range local.UnknownIDs {
		out = append(out, localEntry{id: id})
	}
	for i := range a.Groups {
		g := &a.Groups[i]
		if g.StorageID == nil {
			continue
		}
		gid := g.GroupID
		r := st.groupRecord(g)
		out = append(out, localEntry{store.StorageIdent{Raw: g.StorageID, Type: storageGroupV2}, &signalpb.StorageRecord{Record: &signalpb.StorageRecord_GroupV2{GroupV2: r}},
			func(raw, rec []byte) {
				if g := a.Group(gid); g != nil {
					g.StorageID, g.StorageRecord = raw, rec
				}
			}})
	}
	for _, k := range a.SortedRecipientKeys() {
		rc := a.Recipients[k]
		ad := a.AddressOf(k)
		if k == a.Data.ACI || rc.StorageID == nil || (ad.ACI == "" && ad.PNI == "") {
			continue
		}
		key := k
		out = append(out, localEntry{store.StorageIdent{Raw: rc.StorageID, Type: storageContact}, &signalpb.StorageRecord{Record: &signalpb.StorageRecord_Contact{Contact: st.contactRecord(k)}},
			func(raw, rec []byte) {
				rc := a.Recipients[key]
				rc.StorageID, rc.StorageRecord = raw, rec
				a.Recipients[key] = rc
			}})
	}
	for i := range a.Stickers {
		p := &a.Stickers[i]
		if p.StorageID == nil {
			continue
		}
		pid := p.ID
		out = append(out, localEntry{store.StorageIdent{Raw: p.StorageID, Type: storageSticker}, &signalpb.StorageRecord{Record: &signalpb.StorageRecord_StickerPack{StickerPack: st.stickerRecord(p)}},
			func(raw, rec []byte) {
				for i := range a.Stickers {
					if a.Stickers[i].ID == pid {
						a.Stickers[i].StorageID, a.Stickers[i].StorageRecord = raw, rec
					}
				}
			}})
	}
	self := a.Recipients[a.Data.ACI]
	out = append(out, localEntry{store.StorageIdent{Raw: self.StorageID, Type: storageAccount}, &signalpb.StorageRecord{Record: &signalpb.StorageRecord_Account{Account: st.accountRecord()}},
		func(raw, rec []byte) {
			rc := a.Recipients[a.Data.ACI]
			rc.StorageID, rc.StorageRecord = raw, rec
			a.Recipients[a.Data.ACI] = rc
		}})
	return out
}

func innerBytes(r *signalpb.StorageRecord) []byte {
	var m proto.Message
	switch {
	case r.GetContact() != nil:
		m = r.GetContact()
	case r.GetAccount() != nil:
		m = r.GetAccount()
	case r.GetGroupV2() != nil:
		m = r.GetGroupV2()
	case r.GetStickerPack() != nil:
		m = r.GetStickerPack()
	default:
		return nil
	}
	b, _ := proto.Marshal(m)
	return b
}

func (st *Storage) put(ctx context.Context, key [32]byte, version uint64, recordIkm []byte, entries []localEntry, inserts []localEntry, deletes [][]byte, clearAll bool) error {
	a := st.Sender.Acct
	mr := &signalpb.ManifestRecord{Version: version, SourceDevice: uint32(a.Data.DeviceID), RecordIkm: recordIkm}
	for _, e := range entries {
		mr.Identifiers = append(mr.Identifiers, &signalpb.ManifestRecord_Identifier{Raw: e.id.Raw, Type: e.id.Type})
	}
	plain, err := proto.Marshal(mr)
	if err != nil {
		return err
	}
	op := &signalpb.StorageWriteOperation{Manifest: &signalpb.StorageManifest{Version: version, Value: storage.SealRandom(storage.ManifestKey(key, version), plain)},
		DeleteKey: deletes, ClearAll: clearAll}
	for _, e := range inserts {
		b, err := proto.Marshal(e.rec)
		if err != nil {
			return err
		}
		op.InsertItem = append(op.InsertItem, &signalpb.StorageItem{Key: e.id.Raw, Value: storage.SealRandom(storage.ItemKey(key, recordIkm, e.id.Raw), b)})
	}
	status, _, err := st.request(ctx, "PUT", "/v1/storage", op)
	if err != nil {
		return err
	}
	switch status {
	case 200:
	case 409:
		return errStorageConflict
	default:
		return fmt.Errorf("storage write: status %d", status)
	}
	for _, e := range inserts {
		e.set(e.id.Raw, innerBytes(e.rec))
	}
	local, err := a.LoadStorage()
	if err != nil {
		return err
	}
	local.Manifest, local.RecordIkm, local.IDs = int64(version), recordIkm, nil
	for _, e := range entries {
		local.IDs = append(local.IDs, e.id)
	}
	if clearAll {
		local.UnknownIDs = nil
	}
	a.SaveRecipients()
	a.SaveGroups()
	a.SaveStickers()
	return a.SaveStorage(local)
}

// write is §6.5; it reports whether anything was sent.
func (st *Storage) write(ctx context.Context, key [32]byte, r *manifest, local *store.StorageState) (bool, error) {
	entries := st.localSet(local)
	var ids []store.StorageIdent
	for _, e := range entries {
		ids = append(ids, e.id)
	}
	remoteOnly, localOnly, _ := diffIDs(r.ids, ids)
	only := map[string]bool{}
	for _, id := range localOnly {
		only[string(id.Raw)] = true
	}
	var inserts []localEntry
	var kept []localEntry
	for _, e := range entries {
		if e.rec == nil && only[string(e.id.Raw)] {
			continue // local-only unknown IDs are dropped (§6.5 step 2)
		}
		kept = append(kept, e)
		if e.rec != nil && only[string(e.id.Raw)] {
			inserts = append(inserts, e)
		}
	}
	var deletes [][]byte
	for _, id := range remoteOnly {
		deletes = append(deletes, id.Raw)
	}
	if len(inserts) == 0 && len(deletes) == 0 {
		return false, nil
	}
	return true, st.put(ctx, key, r.version+1, r.recordIkm, kept, inserts, deletes, false)
}

// forcePush is §6.6 (primary only).
func (st *Storage) forcePush(ctx context.Context, key [32]byte, local *store.StorageState) error {
	status, body, err := st.request(ctx, "GET", "/v1/storage/manifest", nil)
	if err != nil {
		return err
	}
	var current uint64
	switch status {
	case 200:
		var sm signalpb.StorageManifest
		if err := proto.Unmarshal(body, &sm); err != nil {
			return err
		}
		current = sm.GetVersion()
	case 404:
	default:
		return fmt.Errorf("storage manifest: status %d", status)
	}
	local.UnknownIDs = nil
	entries := st.localSet(local)
	for i := range entries {
		entries[i].id.Raw = storage.NewRawID()
	}
	return st.put(ctx, key, current+1, nil, entries, entries, nil, current+1 > 1)
}
