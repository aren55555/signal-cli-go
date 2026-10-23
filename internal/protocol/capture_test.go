package protocol

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// TestCaptureReplay decrypts, offline, envelopes the reference signal-cli
// 0.14.7 received on Signal staging (spec/protocol/capture.md §6). It is
// opt-in: SIGNAL_CAPTURE_DIR names a directory with
//
//	pre/      the recipient's data directory before receiving
//	post/     the same after one `receive --trust-new-identities never`
//	db.json   pre/'s account.db exported by tools/capture-export
//
// Captures hold live keys of staging test accounts and stay out of git
// (.data/ is ignored). Every cached envelope must decrypt, and each
// plaintext must contain one of the texts the sender sent.
func TestCaptureReplay(t *testing.T) {
	dir := os.Getenv("SIGNAL_CAPTURE_DIR")
	if dir == "" {
		t.Skip("SIGNAL_CAPTURE_DIR not set")
	}
	ids := loadCaptureAccount(t, filepath.Join(dir, "pre"))
	stores := [2]captureStore{loadCaptureStore(t, filepath.Join(dir, "db.json"), 0), loadCaptureStore(t, filepath.Join(dir, "db.json"), 1)}
	envs := loadCachedEnvelopes(t, filepath.Join(dir, "post"))
	if len(envs) == 0 {
		t.Fatal("no cached envelopes")
	}

	want := []string{
		"capture-1: hello from A",
		"capture-2: second message on the same session",
		// Sent as "capture-3: unicode ✓ ünïcödé 🔐", but the reference JVM ran
		// with LANG unset and turned every non-ASCII character of the argument
		// into U+FFFD before sending; only the ASCII prefix survived. Later
		// captures run the oracle with LANG=C.UTF-8.
		"capture-3: unicode ",
		"capture-4: long message long message",
	}
	records := map[Address]*SessionRecord{}
	trust := func(*curve25519.PublicKey) bool { return true }
	rng := &testReader{testRand(99)} // a DH ratchet step needs fresh keys; never sent anywhere
	found := map[string]bool{}
	kinds := map[string]int{}

	for _, e := range envs {
		// The identity, prekeys and local address are those of the destination
		// service ID: ACI, or PNI when the sender addressed the phone number
		// (sealed-sender.md §8; account_id_type 1 in the database).
		dest, src := e.GetDestinationServiceId(), e.GetSourceServiceId()
		if b := e.GetDestinationServiceIdBinary(); dest == "" && len(b) > 0 {
			var err error
			if dest, err = ServiceIDFromBinary(b); err != nil {
				t.Fatal(err)
			}
		}
		if b := e.GetSourceServiceIdBinary(); src == "" && len(b) > 0 {
			var err error
			if src, err = ServiceIDFromBinary(b); err != nil {
				t.Fatal(err)
			}
		}
		idx := 0
		if strings.HasPrefix(dest, "PNI:") {
			idx = 1
			kinds["to PNI"]++
		}
		acct, store := ids[idx], stores[idx]
		local := Address{ServiceID: acct.serviceID, DeviceID: acct.device}
		content, kind := e.GetContent(), int(e.GetType())
		remote := Address{ServiceID: src, DeviceID: e.GetSourceDeviceId()}
		if kind == 6 { // unidentified sender
			sealed, err := Unseal(acct.identity, TrustRoots(Staging), e.GetServerTimestamp(), content)
			if err != nil {
				t.Fatalf("envelope %d: unseal: %v", e.GetClientTimestamp(), err)
			}
			remote = Address{ServiceID: sealed.Sender.SenderUUID, DeviceID: sealed.Sender.DeviceID}
			content = sealed.Content
			switch sealed.Kind {
			case SealedContentPreKey:
				kind = 3
			case SealedContentMessage:
				kind = 1
			default:
				t.Fatalf("envelope %d: sealed kind %d not covered", e.GetClientTimestamp(), sealed.Kind)
			}
			kinds["sealed"]++
		}
		rec := records[remote]
		if rec == nil {
			rec = new(SessionRecord)
		}
		var pt []byte
		var err error
		switch kind {
		case 3:
			kinds["prekey"]++
			rec, pt, _, err = rec.DecryptPreKey(rng, content, LocalIdentity{Key: acct.identity, RegistrationID: acct.registrationID}, store, trust, local, remote)
		case 1:
			kinds["whisper"]++
			rec, pt, err = rec.Decrypt(rng, content, local, remote)
		default:
			t.Fatalf("envelope %d: type %d not covered", e.GetClientTimestamp(), kind)
		}
		if err != nil {
			t.Fatalf("envelope %d (type %d): %v", e.GetClientTimestamp(), kind, err)
		}
		records[remote] = rec

		trimmed := bytes.TrimRight(pt, "\x00")
		if len(trimmed) == 0 || trimmed[len(trimmed)-1] != 0x80 {
			t.Errorf("envelope %d: plaintext lacks the 0x80 padding boundary", e.GetClientTimestamp())
		}
		ok := false
		for _, w := range want {
			if bytes.Contains(pt, []byte(w)) {
				found[w], ok = true, true
			}
		}
		if !ok {
			t.Errorf("envelope %d: plaintext contains none of the sent texts", e.GetClientTimestamp())
		}
	}
	for _, w := range want {
		if !found[w] {
			t.Errorf("text %q was not recovered", w)
		}
	}
	t.Logf("decrypted %d envelopes: %v", len(envs), kinds)
}

type captureAccount struct {
	serviceID      string
	device         uint32
	registrationID uint32
	identity       *curve25519.PrivateKey
}

// loadCaptureAccount returns the ACI and PNI identities, in that order.
func loadCaptureAccount(t *testing.T, root string) [2]captureAccount {
	t.Helper()
	var index struct {
		Accounts []struct{ Path string }
	}
	readJSON(t, filepath.Join(root, "data", "accounts.json"), &index)
	if len(index.Accounts) != 1 {
		t.Fatalf("%d accounts in the capture", len(index.Accounts))
	}
	type identity struct {
		ServiceID          string `json:"serviceId"`
		RegistrationID     uint32 `json:"registrationId"`
		IdentityPrivateKey string `json:"identityPrivateKey"`
		IdentityPublicKey  string `json:"identityPublicKey"`
	}
	var a struct {
		DeviceID uint32   `json:"deviceId"`
		ACI      identity `json:"aciAccountData"`
		PNI      identity `json:"pniAccountData"`
	}
	readJSON(t, filepath.Join(root, "data", index.Accounts[0].Path), &a)
	var out [2]captureAccount
	for i, id := range []identity{a.ACI, a.PNI} {
		priv, err := base64.StdEncoding.DecodeString(id.IdentityPrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		key, err := curve25519.NewPrivateKey(priv)
		if err != nil {
			t.Fatal(err)
		}
		pub, _ := base64.StdEncoding.DecodeString(id.IdentityPublicKey)
		if !bytes.Equal(key.PublicKey().Bytes(), pub) {
			t.Fatalf("identity %d: key pair does not match", i)
		}
		out[i] = captureAccount{serviceID: id.ServiceID, device: a.DeviceID, registrationID: id.RegistrationID, identity: key}
	}
	return out
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// captureStore serves one identity's prekeys from the export
// (account_id_type 0 for ACI, 1 for PNI).
type captureStore struct {
	preKeys, signedPreKeys map[uint32]string // key_id -> private key hex (raw 32 bytes)
	kyber                  map[uint32]string // key_id -> KyberPreKeyRecord hex
}

func loadCaptureStore(t *testing.T, path string, accountType int) captureStore {
	t.Helper()
	type row struct {
		AccountIDType int    `json:"account_id_type"`
		KeyID         uint32 `json:"key_id"`
		PrivateKey    string `json:"private_key"`
		Serialized    string `json:"serialized"`
	}
	var db struct {
		PreKey       []row `json:"pre_key"`
		SignedPreKey []row `json:"signed_pre_key"`
		KyberPreKey  []row `json:"kyber_pre_key"`
	}
	readJSON(t, path, &db)
	s := captureStore{map[uint32]string{}, map[uint32]string{}, map[uint32]string{}}
	for _, r := range db.PreKey {
		if r.AccountIDType == accountType {
			s.preKeys[r.KeyID] = r.PrivateKey
		}
	}
	for _, r := range db.SignedPreKey {
		if r.AccountIDType == accountType {
			s.signedPreKeys[r.KeyID] = r.PrivateKey
		}
	}
	for _, r := range db.KyberPreKey {
		if r.AccountIDType == accountType {
			s.kyber[r.KeyID] = r.Serialized
		}
	}
	return s
}

func rawPrivate(h string) (*curve25519.PrivateKey, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	return curve25519.NewPrivateKey(b)
}

func (s captureStore) SignedPreKey(id uint32) (*curve25519.PrivateKey, error) {
	h, ok := s.signedPreKeys[id]
	if !ok {
		return nil, ErrInvalidSignedPreKeyID
	}
	return rawPrivate(h)
}

func (s captureStore) PreKey(id uint32) (*curve25519.PrivateKey, error) {
	h, ok := s.preKeys[id]
	if !ok {
		return nil, ErrInvalidPreKeyID
	}
	return rawPrivate(h)
}

func (s captureStore) KyberPreKey(id uint32) (*KEMSecretKey, error) {
	h, ok := s.kyber[id]
	if !ok {
		return nil, ErrInvalidKyberPreKeyID
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	r, err := LoadKyberPreKeyRecord(b)
	if err != nil {
		return nil, err
	}
	return r.SecretKey()
}

// loadCachedEnvelopes reads every msg-cache file, sorted by the sender's
// timestamp (the order the messages were sent).
func loadCachedEnvelopes(t *testing.T, root string) []*signalpb.ServiceEnvelope {
	t.Helper()
	var envs []*signalpb.ServiceEnvelope
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(path, string(filepath.Separator)+"msg-cache"+string(filepath.Separator)) {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, env, err := ParseCachedEnvelope(b)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		envs = append(envs, env)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].GetClientTimestamp() < envs[j].GetClientTimestamp() })
	return envs
}
