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
	identitiesVersion = 1
)

var (
	indexMigrations      = Migrations{}
	accountMigrations    = Migrations{}
	keysMigrations       = Migrations{}
	sessionsMigrations   = Migrations{}
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
	Version                        int         `json:"version"`
	Environment                    string      `json:"environment"`
	Number                         string      `json:"number"`
	ACI                            string      `json:"aci,omitempty"`
	PNI                            string      `json:"pni,omitempty"`
	DeviceID                       int         `json:"deviceId"`
	Password                       string      `json:"password"`
	Registered                     bool        `json:"registered"`
	ACIIdentity                    KeyPair     `json:"aciIdentity"`
	PNIIdentity                    KeyPair     `json:"pniIdentity"`
	ACIRegistrationID              uint32      `json:"aciRegistrationId"`
	PNIRegistrationID              uint32      `json:"pniRegistrationId"`
	ProfileKey                     []byte      `json:"profileKey,omitempty"`
	AccountEntropyPool             string      `json:"accountEntropyPool,omitempty"`
	MediaRootBackupKey             []byte      `json:"mediaRootBackupKey,omitempty"`
	DeviceName                     string      `json:"deviceName,omitempty"` // encrypted, unpadded base64 (linked devices)
	UnrestrictedUnidentifiedAccess bool        `json:"unrestrictedUnidentifiedAccess,omitempty"`
	Unlisted                       bool        `json:"unlisted,omitempty"`
	ACIKeys                        PreKeyState `json:"aciKeys"`
	PNIKeys                        PreKeyState `json:"pniKeys"`
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
	Sessions   map[string][]byte    // "aci:<address>.<device>" -> SessionRecord
	Identities map[string]Identity  // service ID -> identity
	Recipients map[string]Recipient // ACI -> recipient
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
	a := &Account{root: r, path: path, lock: lock, Sessions: map[string][]byte{}, Identities: map[string]Identity{}, Recipients: map[string]Recipient{}}
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

// Profile is what we keep of a fetched profile.
type Profile struct {
	LastUpdate   int64    `json:"lastUpdate"` // ms; set on failed fetches too
	AccessMode   string   `json:"accessMode"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Recipient is what we know about a peer, keyed by ACI.
type Recipient struct {
	ProfileKey   []byte   `json:"profileKey,omitempty"`
	Profile      *Profile `json:"profile,omitempty"`
	Unregistered bool     `json:"unregistered,omitempty"`
}

type recipientsDoc struct {
	Version    int                  `json:"version"`
	Recipients map[string]Recipient `json:"recipients"`
}

const recipientsVersion = 1

var recipientsMigrations = Migrations{}

// SaveRecipients writes recipients.json.
func (a *Account) SaveRecipients() error {
	return WriteJSON(a.file("recipients.json"), &recipientsDoc{Version: recipientsVersion, Recipients: a.Recipients})
}

func (a *Account) loadRecipients() error {
	var d recipientsDoc
	if err := ReadJSON(a.file("recipients.json"), recipientsVersion, recipientsMigrations, &d); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.Recipients = d.Recipients
	if a.Recipients == nil {
		a.Recipients = map[string]Recipient{}
	}
	return nil
}
