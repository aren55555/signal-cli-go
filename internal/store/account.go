// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrNoAccount reports a number with no local account.
var ErrNoAccount = errors.New("store: no such account")

// Environments, as signal-cli names them.
const (
	EnvStaging = "STAGING"
	EnvLive    = "LIVE"
)

// Document versions. Raise one and add a migration when its layout
// changes.
const (
	indexVersion      = 2 // signal-cli's accounts.json version
	accountVersion    = 1
	keysVersion       = 1
	sessionsVersion   = 1
	senderKeysVersion = 1
	stickersVersion   = 1
	groupsVersion     = 1
	identitiesVersion = 1
)

var (
	indexMigrations      = Migrations{}
	accountMigrations    = Migrations{}
	keysMigrations       = Migrations{}
	sessionsMigrations   = Migrations{}
	senderKeysMigrations = Migrations{}
	identitiesMigrations = Migrations{}
)

// KeyPair is a Curve25519 key pair in serialized form (33-byte public,
// 32-byte private).
type KeyPair struct {
	Public  []byte `json:"public"`
	Private []byte `json:"private"`
}

// PreKeyState holds one identity's prekey counters and markers
// (spec/service/keys.md §1). Active IDs are -1 when unset.
type PreKeyState struct {
	NextPreKeyID            uint32 `json:"nextPreKeyId"`
	NextSignedPreKeyID      uint32 `json:"nextSignedPreKeyId"`
	NextKyberPreKeyID       uint32 `json:"nextKyberPreKeyId"`
	ActiveSignedPreKeyID    int64  `json:"activeSignedPreKeyId"`
	ActiveLastResortKyberID int64  `json:"activeLastResortKyberId"`
}

// AccountData is account.json.
type AccountData struct {
	Version                        int     `json:"version"`
	Environment                    string  `json:"environment"`
	Number                         string  `json:"number"`
	ACI                            string  `json:"aci,omitempty"`
	PNI                            string  `json:"pni,omitempty"`
	DeviceID                       int     `json:"deviceId"`
	Password                       string  `json:"password"`
	Registered                     bool    `json:"registered"`
	ACIIdentity                    KeyPair `json:"aciIdentity"`
	PNIIdentity                    KeyPair `json:"pniIdentity"`
	ACIRegistrationID              uint32  `json:"aciRegistrationId"`
	PNIRegistrationID              uint32  `json:"pniRegistrationId"`
	ProfileKey                     []byte  `json:"profileKey,omitempty"`
	AccountEntropyPool             string  `json:"accountEntropyPool,omitempty"`
	MediaRootBackupKey             []byte  `json:"mediaRootBackupKey,omitempty"`
	DeviceName                     string  `json:"deviceName,omitempty"` // encrypted, unpadded base64 (linked devices)
	UnrestrictedUnidentifiedAccess bool    `json:"unrestrictedUnidentifiedAccess,omitempty"`
	// PhoneNumberSharingMode: EVERYBODY, CONTACTS or NOBODY; "" is the
	// default and publishes false (profiles.md §1.2).
	PhoneNumberSharingMode string `json:"phoneNumberSharingMode,omitempty"`
	// Username and its link (spec/service/usernames.md §6.1).
	Username             string `json:"username,omitempty"`
	UsernameLinkEntropy  []byte `json:"usernameLinkEntropy,omitempty"`
	UsernameLinkServerID string `json:"usernameLinkServerId,omitempty"`
	// CDSI state (spec/service/cdsi.md §6.3): the token, the numbers it
	// covers (with last-seen ms) and the last full refresh (ms).
	CDSIToken             []byte           `json:"cdsiToken,omitempty"`
	CDSINumbers           map[string]int64 `json:"cdsiNumbers,omitempty"`
	LastRecipientsRefresh int64            `json:"lastRecipientsRefresh,omitempty"`
	// Registration lock (spec/crypto/pin.md §4.1): the PIN as typed and a
	// master key restored from SVR2.
	RegistrationLockPin string `json:"registrationLockPin,omitempty"`
	PinMasterKey        []byte `json:"pinMasterKey,omitempty"`
	Unlisted            bool   `json:"unlisted,omitempty"`
	SessionID           string `json:"sessionId,omitempty"` // verification session (registration.md §2.5)
	// SessionNumber is the number SessionID was created for: the account's
	// own (registration) or a new one (change-number.md §2.2).
	SessionNumber string `json:"sessionNumber,omitempty"`
	// LastPniChange is the server-delivered time of the last applied
	// pniChangeNumber sync message (sync.md §6.13).
	LastPniChange int64         `json:"lastPniChange,omitempty"`
	MultiDevice   bool          `json:"multiDevice,omitempty"` // sync messages are sent (sync.md §1)
	Configuration Configuration `json:"configuration"`
	ACIKeys       PreKeyState   `json:"aciKeys"`
	PNIKeys       PreKeyState   `json:"pniKeys"`
}

// Configuration is the account settings synced between devices (sync.md
// §6.6); nil means unset.
type Configuration struct {
	ReadReceipts                   *bool `json:"readReceipts,omitempty"`
	UnidentifiedDeliveryIndicators *bool `json:"unidentifiedDeliveryIndicators,omitempty"`
	TypingIndicators               *bool `json:"typingIndicators,omitempty"`
	LinkPreviews                   *bool `json:"linkPreviews,omitempty"`
}

// StoredKey is a serialized prekey record with its bookkeeping.
type StoredKey struct {
	Record     []byte `json:"record"`
	StaleAt    int64  `json:"staleAt,omitempty"` // ms; 0 = not stale
	LastResort bool   `json:"lastResort,omitempty"`
}

// IdentityKeys are one identity's local prekeys, by ID.
type IdentityKeys struct {
	PreKeys       map[uint32]StoredKey `json:"preKeys,omitempty"`
	SignedPreKeys map[uint32]StoredKey `json:"signedPreKeys,omitempty"`
	KyberPreKeys  map[uint32]StoredKey `json:"kyberPreKeys,omitempty"`
}

// KeysData is keys.json.
type KeysData struct {
	Version int          `json:"version"`
	ACI     IdentityKeys `json:"aci"`
	PNI     IdentityKeys `json:"pni"`
}

// Trust levels for remote identity keys.
const (
	TrustedUnverified = "TRUSTED_UNVERIFIED"
	TrustedVerified   = "TRUSTED_VERIFIED"
	Untrusted         = "UNTRUSTED"
)

// Identity is a remote identity key and our trust in it.
type Identity struct {
	Key   []byte `json:"key"`
	Trust string `json:"trust"`
	Added int64  `json:"added,omitempty"` // ms
}

type sessionsDoc struct {
	Version  int               `json:"version"`
	Sessions map[string][]byte `json:"sessions"`
}

type identitiesDoc struct {
	Version    int                 `json:"version"`
	Identities map[string]Identity `json:"identities"`
}

// IndexEntry is one account in signal-cli's accounts.json.
type IndexEntry struct {
	Path        string
	Environment string
	Number      string
	UUID        string // "" when null
}

type indexDoc struct {
	Accounts []IndexEntry
}

// Root is signal-cli's account directory (<DATA_DIR>/data).
type Root struct{ dir string }

// Open returns the root at dir; nothing is read or created until needed.
func Open(dir string) *Root { return &Root{dir: dir} }

// ErrNotOurs reports an index entry with no signal-cli-go account files
// (an account signal-cli itself created, not yet imported).
var ErrNotOurs = errors.New("store: account has no signal-cli-go data")

// Accounts lists the index; a missing index is an empty list.
func (r *Root) Accounts() ([]IndexEntry, error) {
	idx, err := r.index()
	if err != nil {
		return nil, err
	}
	return idx.Accounts, nil
}

func (r *Root) index() (*indexDoc, error) {
	b, err := os.ReadFile(filepath.Join(r.dir, "accounts.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &indexDoc{}, nil
	}
	if err != nil {
		return nil, err
	}
	var raw struct {
		Accounts []struct {
			Path        string  `json:"path"`
			Environment string  `json:"environment"`
			Number      string  `json:"number"`
			UUID        *string `json:"uuid"`
		} `json:"accounts"`
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("store: accounts.json: %w", err)
	}
	if raw.Version > indexVersion {
		return nil, fmt.Errorf("%w: accounts.json has version %d", ErrNewerVersion, raw.Version)
	}
	idx := &indexDoc{}
	for _, e := range raw.Accounts {
		ie := IndexEntry{Path: e.Path, Environment: e.Environment, Number: e.Number}
		if e.UUID != nil {
			ie.UUID = *e.UUID
		}
		idx.Accounts = append(idx.Accounts, ie)
	}
	return idx, nil
}

// saveIndex writes accounts.json as signal-cli does (Jackson's default
// pretty printer).
func (r *Root) saveIndex(idx *indexDoc) error {
	q := func(s string) string {
		b, _ := json.Marshal(s)
		return string(b)
	}
	var sb strings.Builder
	sb.WriteString("{\n  \"accounts\" : [ ")
	for i, e := range idx.Accounts {
		if i > 0 {
			sb.WriteString(", ")
		}
		uuid := "null"
		if e.UUID != "" {
			uuid = q(e.UUID)
		}
		fmt.Fprintf(&sb, "{\n    \"path\" : %s,\n    \"environment\" : %s,\n    \"number\" : %s,\n    \"uuid\" : %s\n  }",
			q(e.Path), q(e.Environment), q(e.Number), uuid)
	}
	if len(idx.Accounts) > 0 {
		sb.WriteString(" ")
	}
	fmt.Fprintf(&sb, "],\n  \"version\" : %d\n}", indexVersion)
	return WriteFileAtomic(filepath.Join(r.dir, "accounts.json"), []byte(sb.String()), 0o600)
}

// Account is an open, locked account directory.
type Account struct {
	root       *Root
	path       string
	lock       *FileLock
	Data       AccountData
	Keys       KeysData
	Sessions   map[string][]byte   // "aci:<address>.<device>" -> SessionRecord
	Identities map[string]Identity // service ID -> identity
	SenderKeys SenderKeys
	Stickers   []StickerPack
	Groups     []GroupRecord
	Recipients map[string]Recipient // primary key (recipients.go) -> recipient

	recipientsNextID int64
}

// Path is the account's directory name within the root.
func (a *Account) Path() string { return a.path }

func (a *Account) file(name string) string { return filepath.Join(a.Dir(), name) }

// accountDir is where signal-cli-go keeps an account: <path>.go/ beside
// signal-cli's own account file <path>.
func (r *Root) accountDir(path string) string { return filepath.Join(r.dir, path+".go") }

// Create makes and locks a new account directory for number.
func (r *Root) Create(number, env string) (*Account, error) {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return nil, err
	}
	idx, err := r.index()
	if err != nil {
		return nil, err
	}
	for _, e := range idx.Accounts {
		if e.Number == number {
			return nil, fmt.Errorf("store: an account for this number already exists")
		}
	}
	var id [4]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	path := hex.EncodeToString(id[:])
	if err := os.Mkdir(r.accountDir(path), 0o700); err != nil {
		return nil, err
	}
	lock, err := Lock(r.accountDir(path))
	if err != nil {
		return nil, err
	}
	a := &Account{root: r, path: path, lock: lock, Sessions: map[string][]byte{}, Identities: map[string]Identity{}, Recipients: map[string]Recipient{},
		SenderKeys: SenderKeys{Records: map[string][]byte{}, Created: map[string]int64{}, Shared: map[string]map[string]int64{}}}
	a.Data = AccountData{Environment: env, Number: number,
		ACIKeys: PreKeyState{ActiveSignedPreKeyID: -1, ActiveLastResortKyberID: -1},
		PNIKeys: PreKeyState{ActiveSignedPreKeyID: -1, ActiveLastResortKyberID: -1}}
	if err := a.Save(); err != nil {
		lock.Unlock()
		return nil, err
	}
	idx.Accounts = append(idx.Accounts, IndexEntry{Path: path, Environment: env, Number: number})
	if err := r.saveIndex(idx); err != nil {
		lock.Unlock()
		return nil, err
	}
	return a, nil
}

// Find opens the account for number.
func (r *Root) Find(number string) (*Account, error) {
	idx, err := r.index()
	if err != nil {
		return nil, err
	}
	for _, e := range idx.Accounts {
		if e.Number == number {
			return r.Load(e.Path)
		}
	}
	return nil, ErrNoAccount
}

// Load opens and locks the account directory at path.
func (r *Root) Load(path string) (*Account, error) {
	if fi, err := os.Stat(r.accountDir(path)); err != nil || !fi.IsDir() {
		return nil, ErrNotOurs
	}
	lock, err := Lock(r.accountDir(path))
	if err != nil {
		return nil, err
	}
	a := &Account{root: r, path: path, lock: lock}
	fail := func(err error) (*Account, error) { lock.Unlock(); return nil, err }
	if err := ReadJSON(a.file("account.json"), accountVersion, accountMigrations, &a.Data); err != nil {
		return fail(err)
	}
	if err := ReadJSON(a.file("keys.json"), keysVersion, keysMigrations, &a.Keys); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	var s sessionsDoc
	if err := ReadJSON(a.file("sessions.json"), sessionsVersion, sessionsMigrations, &s); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	var ids identitiesDoc
	if err := ReadJSON(a.file("identities.json"), identitiesVersion, identitiesMigrations, &ids); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	a.Sessions, a.Identities = s.Sessions, ids.Identities
	if a.Sessions == nil {
		a.Sessions = map[string][]byte{}
	}
	if err := a.loadRecipients(); err != nil {
		return fail(err)
	}
	if err := a.LoadSenderKeys(); err != nil {
		return fail(err)
	}
	if err := a.LoadStickers(); err != nil {
		return fail(err)
	}
	if err := a.LoadGroups(); err != nil {
		return fail(err)
	}
	if a.Identities == nil {
		a.Identities = map[string]Identity{}
	}
	return a, nil
}

// Save writes account.json and keeps the index's ACI current.
func (a *Account) Save() error {
	a.Data.Version = accountVersion
	if err := WriteJSON(a.file("account.json"), &a.Data); err != nil {
		return err
	}
	idx, err := a.root.index()
	if err != nil {
		return err
	}
	for i := range idx.Accounts {
		if idx.Accounts[i].Path == a.path && idx.Accounts[i].UUID != a.Data.ACI {
			idx.Accounts[i].UUID = a.Data.ACI
			return a.root.saveIndex(idx)
		}
	}
	return nil
}

// SaveKeys writes keys.json.
func (a *Account) SaveKeys() error {
	a.Keys.Version = keysVersion
	return WriteJSON(a.file("keys.json"), &a.Keys)
}

// SaveSessions writes sessions.json.
func (a *Account) SaveSessions() error {
	return WriteJSON(a.file("sessions.json"), &sessionsDoc{Version: sessionsVersion, Sessions: a.Sessions})
}

// SaveIdentities writes identities.json.
func (a *Account) SaveIdentities() error {
	return WriteJSON(a.file("identities.json"), &identitiesDoc{Version: identitiesVersion, Identities: a.Identities})
}

// Close releases the account's lock.
func (a *Account) Close() error {
	if a.lock == nil {
		return nil
	}
	err := a.lock.Unlock()
	a.lock = nil
	return err
}

// Delete closes a and removes it from the root (a partial link or
// registration, linking.md §7 step 8).
func (r *Root) Delete(a *Account) error {
	a.Close()
	idx, err := r.index()
	if err != nil {
		return err
	}
	idx.Accounts = slices.DeleteFunc(idx.Accounts, func(e IndexEntry) bool { return e.Path == a.path })
	if err := r.saveIndex(idx); err != nil {
		return err
	}
	return os.RemoveAll(a.Dir())
}

// Dir is the account directory's full path.
func (a *Account) Dir() string { return a.root.accountDir(a.path) }

// Unidentified-access modes of a stored profile (spec/service/send.md §3).
const (
	AccessUnknown      = "UNKNOWN"
	AccessDisabled     = "DISABLED"
	AccessEnabled      = "ENABLED"
	AccessUnrestricted = "UNRESTRICTED"
)

// Profile is the stored profile record (spec/service/profiles.md §1.1).
type Profile struct {
	LastUpdate   int64    `json:"lastUpdate"` // ms; set on failed fetches too; 0 = invalidated
	AccessMode   string   `json:"accessMode"`
	Capabilities []string `json:"capabilities,omitempty"`

	GivenName          *string `json:"givenName,omitempty"`
	FamilyName         *string `json:"familyName,omitempty"`
	About              *string `json:"about,omitempty"`
	AboutEmoji         *string `json:"aboutEmoji,omitempty"`
	AvatarURLPath      *string `json:"avatarUrlPath,omitempty"`
	MobileCoinAddress  []byte  `json:"mobileCoinAddress,omitempty"`
	PhoneNumberSharing *string `json:"phoneNumberSharing,omitempty"` // EVERYBODY, NOBODY or nil
}

// Recipient is one row of the recipient store (spec/service/contacts.md
// §2), stored under its primary key (recipients.go).
type Recipient struct {
	ID             int64    `json:"id,omitempty"` // stable row number, listing order
	ProfileKey     []byte   `json:"profileKey,omitempty"`
	Profile        *Profile `json:"profile,omitempty"`
	Unregistered   bool     `json:"unregistered,omitempty"`
	UnregisteredAt int64    `json:"unregisteredAt,omitempty"` // ms
	Discoverable   *bool    `json:"discoverable,omitempty"`   // from CDSI; nil = never checked
	Number         string   `json:"number,omitempty"`
	Username       string   `json:"username,omitempty"`
	PNI            string   `json:"pni,omitempty"` // bare UUID
	// NeedsPNISignature: our replies carry a PNI signature (content-features.md §4.3).
	NeedsPNISignature bool   `json:"needsPniSignature,omitempty"`
	PNIVerified       bool   `json:"pniVerified,omitempty"`
	Timer             uint32 `json:"timer,omitempty"`        // disappearing-message timer, seconds
	TimerVersion      uint32 `json:"timerVersion,omitempty"` // 0 means no contact entry yet
	ProfileSharing    bool   `json:"profileSharing,omitempty"`
	Blocked           bool   `json:"blocked,omitempty"`
	// ProfileKeyCredential is the stored expiring profile-key credential
	// (zkgroup-credentials.md §8.4).
	ProfileKeyCredential []byte `json:"profileKeyCredential,omitempty"`

	// Contact columns (contacts.md §2.1). A non-nil empty string counts as
	// set for the is-contact predicate.
	GivenName      *string `json:"givenName,omitempty"`
	FamilyName     *string `json:"familyName,omitempty"`
	NickName       *string `json:"nickName,omitempty"`
	NickGivenName  *string `json:"nickGivenName,omitempty"`
	NickFamilyName *string `json:"nickFamilyName,omitempty"`
	Note           *string `json:"note,omitempty"`
	Color          *string `json:"color,omitempty"`
	Archived       bool    `json:"archived,omitempty"`
	Hidden         bool    `json:"hidden,omitempty"`
	HideStory      bool    `json:"hideStory,omitempty"`
	MuteUntil      int64   `json:"muteUntil,omitempty"`

	// Storage-service ID and last remote record (ContactRecord, or
	// AccountRecord on the own row; storage.md §6.2).
	StorageID     []byte `json:"storageId,omitempty"`
	StorageRecord []byte `json:"storageRecord,omitempty"`
}

type recipientsDoc struct {
	Version    int                  `json:"version"`
	NextID     int64                `json:"nextId,omitempty"`
	Recipients map[string]Recipient `json:"recipients"`
}

const recipientsVersion = 1

var recipientsMigrations = Migrations{}

// SaveRecipients writes recipients.json.
func (a *Account) SaveRecipients() error {
	return WriteJSON(a.file("recipients.json"), &recipientsDoc{Version: recipientsVersion, NextID: a.recipientsNextID, Recipients: a.Recipients})
}

func (a *Account) loadRecipients() error {
	var d recipientsDoc
	if err := ReadJSON(a.file("recipients.json"), recipientsVersion, recipientsMigrations, &d); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.Recipients, a.recipientsNextID = d.Recipients, d.NextID
	if a.Recipients == nil {
		a.Recipients = map[string]Recipient{}
	}
	a.assignIDs()
	return nil
}

// IsMultiDevice reports whether sync messages go to other devices: always
// for a linked device, and for a primary once it has linked one.
func (a *Account) IsMultiDevice() bool { return a.Data.MultiDevice || a.Data.DeviceID > 1 }

// SenderKeys is the sender-key store (stories.md §3.2, §3.8).
type SenderKeys struct {
	// Records: "<service ID>.<device>.<distribution ID>" -> SenderKeyRecord;
	// our own sending keys are under our ACI and device.
	Records map[string][]byte `json:"records"`
	// Created: when each record key was first stored (ms).
	Created map[string]int64 `json:"created,omitempty"`
	// Shared: distribution ID -> "<service ID>.<device>" -> ms when our
	// current sender key was last sent to that device.
	Shared map[string]map[string]int64 `json:"shared"`
}

type senderKeysDoc struct {
	Version int `json:"version"`
	SenderKeys
}

// LoadSenderKeys reads senderkeys.json into a.SenderKeys (empty if absent).
func (a *Account) LoadSenderKeys() error {
	var d senderKeysDoc
	if err := ReadJSON(a.file("senderkeys.json"), senderKeysVersion, senderKeysMigrations, &d); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.SenderKeys = d.SenderKeys
	if a.SenderKeys.Records == nil {
		a.SenderKeys.Records = map[string][]byte{}
	}
	if a.SenderKeys.Created == nil {
		a.SenderKeys.Created = map[string]int64{}
	}
	if a.SenderKeys.Shared == nil {
		a.SenderKeys.Shared = map[string]map[string]int64{}
	}
	return nil
}

// SaveSenderKeys writes senderkeys.json.
func (a *Account) SaveSenderKeys() error {
	return WriteJSON(a.file("senderkeys.json"), &senderKeysDoc{Version: senderKeysVersion, SenderKeys: a.SenderKeys})
}

// StickerPack is a known sticker pack (attachments.md §6.6).
type StickerPack struct {
	ID        string `json:"id"` // lowercase hex
	Key       []byte `json:"key"`
	Installed bool   `json:"installed"`
	// Storage-service state (storage.md §6.2, §6.4.3).
	Position      uint32 `json:"position,omitempty"`
	DeletedAt     int64  `json:"deletedAt,omitempty"` // ms; non-zero = uninstalled tombstone
	StorageID     []byte `json:"storageId,omitempty"`
	StorageRecord []byte `json:"storageRecord,omitempty"`
}

type stickersDoc struct {
	Version int           `json:"version"`
	Packs   []StickerPack `json:"packs"` // in the order they became known
}

// LoadStickers reads stickers.json into a.Stickers.
func (a *Account) LoadStickers() error {
	var d stickersDoc
	if err := ReadJSON(a.file("stickers.json"), stickersVersion, Migrations{}, &d); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.Stickers = d.Packs
	return nil
}

// SaveStickers writes stickers.json.
func (a *Account) SaveStickers() error {
	return WriteJSON(a.file("stickers.json"), &stickersDoc{Version: stickersVersion, Packs: a.Stickers})
}

// StickerPackByID returns the index of a pack, or -1.
func (a *Account) StickerPackByID(id string) int {
	for i, p := range a.Stickers {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// GroupRecord is a stored v2 group (groups.md §5). State is the decrypted
// group as JSON (internal/groups.State), nil when never fetched or lost.
type GroupRecord struct {
	GroupID               []byte            `json:"groupId"`
	MasterKey             []byte            `json:"masterKey"`
	DistributionID        string            `json:"distributionId"`
	State                 json.RawMessage   `json:"state,omitempty"`
	Blocked               bool              `json:"blocked,omitempty"`
	ProfileSharing        bool              `json:"profileSharing,omitempty"`
	PermissionDenied      bool              `json:"permissionDenied,omitempty"`
	EndorsementExpiration int64             `json:"endorsementExpiration,omitempty"` // ms
	Endorsements          map[string][]byte `json:"endorsements,omitempty"`          // ACI -> 33-byte endorsement
	StorageID             []byte            `json:"storageId,omitempty"`
	StorageRecord         []byte            `json:"storageRecord,omitempty"` // last GroupV2Record
}

type groupsDoc struct {
	Version int           `json:"version"`
	Groups  []GroupRecord `json:"groups"`
}

// LoadGroups reads groups.json into a.Groups.
func (a *Account) LoadGroups() error {
	var d groupsDoc
	if err := ReadJSON(a.file("groups.json"), groupsVersion, Migrations{}, &d); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.Groups = d.Groups
	return nil
}

// SaveGroups writes groups.json.
func (a *Account) SaveGroups() error {
	return WriteJSON(a.file("groups.json"), &groupsDoc{Version: groupsVersion, Groups: a.Groups})
}

// Group returns the record with that group ID, or nil.
func (a *Account) Group(id []byte) *GroupRecord {
	for i := range a.Groups {
		if string(a.Groups[i].GroupID) == string(id) {
			return &a.Groups[i]
		}
	}
	return nil
}

// RemoveGroup deletes a record.
func (a *Account) RemoveGroup(id []byte) {
	for i := range a.Groups {
		if string(a.Groups[i].GroupID) == string(id) {
			a.Groups = append(a.Groups[:i], a.Groups[i+1:]...)
			return
		}
	}
}

// SetNumber changes the account's number, in account.json and in the
// accounts index (change-number.md §4).
func (a *Account) SetNumber(number string) error {
	a.Data.Number = number
	if err := a.Save(); err != nil {
		return err
	}
	idx, err := a.root.index()
	if err != nil {
		return err
	}
	for i := range idx.Accounts {
		if idx.Accounts[i].Path == a.path {
			idx.Accounts[i].Number = number
			return a.root.saveIndex(idx)
		}
	}
	return nil
}

// Root is the account's directory root.
func (a *Account) Root() *Root { return a.root }
