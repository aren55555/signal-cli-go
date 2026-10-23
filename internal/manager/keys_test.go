package manager

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// keyServer answers the account check's requests.
type keyServer struct {
	counts     map[string]api.PreKeyCounts
	uploads    map[string]api.PreKeyUpload
	attributes []byte
	status     map[string]int // "PUT /v2/keys?identity=pni" -> status
	log        []string
}

func (k *keyServer) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	key := verb + " " + path
	k.log = append(k.log, key)
	if st, ok := k.status[key]; ok {
		return &chat.Response{Status: st}, nil
	}
	switch {
	case key == "PUT /v1/accounts/attributes":
		k.attributes = body
		if len(headers) != 1 || headers[0].Name != "content-type" {
			return &chat.Response{Status: 400}, nil
		}
		return &chat.Response{Status: 204}, nil
	case verb == "GET" && strings.HasPrefix(path, "/v2/keys?identity="):
		b, _ := json.Marshal(k.counts[strings.TrimPrefix(path, "/v2/keys?identity=")])
		return &chat.Response{Status: 200, Body: b}, nil
	case verb == "PUT" && strings.HasPrefix(path, "/v2/keys?identity="):
		var u api.PreKeyUpload
		json.Unmarshal(body, &u)
		if k.uploads == nil {
			k.uploads = map[string]api.PreKeyUpload{}
		}
		k.uploads[strings.TrimPrefix(path, "/v2/keys?identity=")] = u
		return &chat.Response{Status: 200}, nil
	}
	return &chat.Response{Status: 404}, nil
}

func linkedAccount(t *testing.T, now time.Time) *store.Account {
	a, _ := testAccount(t)
	a.Data.DeviceName = "bmFtZQ"
	for _, s := range []*store.PreKeyState{&a.Data.ACIKeys, &a.Data.PNIKeys} {
		*s = store.PreKeyState{NextPreKeyID: 0xFFFFF0, NextSignedPreKeyID: 6, NextKyberPreKeyID: 7, ActiveSignedPreKeyID: 5, ActiveLastResortKyberID: 6}
	}
	// testAccount's signed and last-resort keys were made "now".
	return a
}

// TestAccountCheckFreshLink: the first check after linking uploads 100 EC
// and 100 Kyber one-time keys per identity and nothing else (keys.md
// §3.2-3.3), after PUT /v1/accounts/attributes (api.md §4.3).
func TestAccountCheckFreshLink(t *testing.T) {
	now := time.Now()
	a := linkedAccount(t, now)
	for _, k := range []*store.IdentityKeys{&a.Keys.ACI, &a.Keys.PNI} { // an old one-time key, clear of the new IDs
		k.PreKeys = map[uint32]store.StoredKey{500: k.PreKeys[8]}
	}
	ks := &keyServer{}
	if err := CheckAccount(context.Background(), a, ks, now); err != nil {
		t.Fatal(err)
	}
	wantLog := []string{"PUT /v1/accounts/attributes", "GET /v2/keys?identity=aci", "PUT /v2/keys?identity=aci", "GET /v2/keys?identity=pni", "PUT /v2/keys?identity=pni"}
	if strings.Join(ks.log, ",") != strings.Join(wantLog, ",") {
		t.Errorf("requests %v", ks.log)
	}
	var attrs api.AccountAttributes
	json.Unmarshal(ks.attributes, &attrs)
	if attrs.RegistrationID != 77 || !attrs.FetchesMessages || attrs.Name == nil || *attrs.Name != "bmFtZQ" ||
		attrs.Capabilities != api.LinkedDeviceCapabilities || len(attrs.UnidentifiedAccessKey) != 16 || !attrs.DiscoverableByPhoneNumber {
		t.Errorf("attributes %s", ks.attributes)
	}
	for _, side := range []struct {
		name  string
		state store.PreKeyState
		keys  store.IdentityKeys
		id    []byte
	}{{"aci", a.Data.ACIKeys, a.Keys.ACI, a.Data.ACIIdentity.Public}, {"pni", a.Data.PNIKeys, a.Keys.PNI, a.Data.PNIIdentity.Public}} {
		u := ks.uploads[side.name]
		if len(u.PreKeys) != 100 || len(u.PqPreKeys) != 100 || u.SignedPreKey != nil || u.PqLastResortPreKey != nil {
			t.Errorf("%s upload: %d EC, %d Kyber, signed %v, last resort %v", side.name, len(u.PreKeys), len(u.PqPreKeys), u.SignedPreKey, u.PqLastResortPreKey)
			continue
		}
		// IDs run from the counter and wrap modulo 0xFFFFFF.
		if u.PreKeys[0].KeyID != 0xFFFFF0 || u.PreKeys[15].KeyID != 0xFFFFFF%0xFFFFFF || u.PreKeys[16].KeyID != 1 || u.PqPreKeys[0].KeyID != 7 {
			t.Errorf("%s IDs: %d %d %d, kyber %d", side.name, u.PreKeys[0].KeyID, u.PreKeys[15].KeyID, u.PreKeys[16].KeyID, u.PqPreKeys[0].KeyID)
		}
		idKey, _ := curve25519.ParsePublicKey(side.id)
		pub, _ := api.DecodeBase64(u.PqPreKeys[3].PublicKey)
		sig, _ := api.DecodeBase64(u.PqPreKeys[3].Signature)
		if !idKey.Verify(pub, sig) {
			t.Errorf("%s Kyber one-time key signature", side.name)
		}
		if side.state.NextPreKeyID != (0xFFFFF0+100)%0xFFFFFF || side.state.NextKyberPreKeyID != 107 || side.state.ActiveSignedPreKeyID != 5 {
			t.Errorf("%s counters %+v", side.name, side.state)
		}
		if len(side.keys.KyberPreKeys) != 101 || len(side.keys.PreKeys) != 101 {
			t.Errorf("%s stored %d Kyber, %d EC", side.name, len(side.keys.KyberPreKeys), len(side.keys.PreKeys))
		}
		if old := side.keys.PreKeys[500]; old.StaleAt != now.UnixMilli() {
			t.Errorf("%s: existing one-time key not marked stale (%d)", side.name, old.StaleAt)
		}
		if k := side.keys.PreKeys[0xFFFFF0]; k.StaleAt != 0 {
			t.Errorf("%s: new key marked stale", side.name)
		}
		if _, err := protocol.LoadPreKeyRecord(side.keys.PreKeys[1].Record); err != nil {
			t.Errorf("%s stored key: %v", side.name, err)
		}
	}

	// A second check with enough keys on the server and fresh signed keys
	// uploads nothing.
	ks2 := &keyServer{counts: map[string]api.PreKeyCounts{"aci": {Count: 100, PqCount: 100}, "pni": {Count: 10, PqCount: 10}}}
	if err := CheckAccount(context.Background(), a, ks2, now); err != nil {
		t.Fatal(err)
	}
	if len(ks2.uploads) != 0 {
		t.Errorf("uploaded %v", ks2.uploads)
	}
}

// TestRotation: signed and last-resort keys older than 2 days rotate; the
// last-resort ID follows the one-time Kyber keys generated in the pass.
func TestRotation(t *testing.T) {
	now := time.Now().Add(3 * 24 * time.Hour)
	a := linkedAccount(t, now)
	ks := &keyServer{counts: map[string]api.PreKeyCounts{"aci": {Count: 50, PqCount: 0}, "pni": {Count: 50, PqCount: 50}}}
	if err := CheckAccount(context.Background(), a, ks, now); err != nil {
		t.Fatal(err)
	}
	u := ks.uploads["aci"]
	if u.PreKeys != nil || len(u.PqPreKeys) != 100 || u.SignedPreKey == nil || u.SignedPreKey.KeyID != 6 || u.PqLastResortPreKey == nil || u.PqLastResortPreKey.KeyID != 107 {
		t.Fatalf("aci upload: %+v", u)
	}
	if a.Data.ACIKeys.ActiveSignedPreKeyID != 6 || a.Data.ACIKeys.ActiveLastResortKyberID != 107 || a.Data.ACIKeys.NextSignedPreKeyID != 7 || a.Data.ACIKeys.NextKyberPreKeyID != 108 {
		t.Errorf("aci state %+v", a.Data.ACIKeys)
	}
	if _, ok := a.Keys.ACI.SignedPreKeys[5]; !ok {
		t.Error("old signed prekey dropped (it stays for late PreKey messages)")
	}
	if !a.Keys.ACI.KyberPreKeys[107].LastResort {
		t.Error("new last-resort key not marked")
	}
	p := ks.uploads["pni"]
	if p.PreKeys != nil || p.PqPreKeys != nil || p.SignedPreKey.KeyID != 6 || p.PqLastResortPreKey.KeyID != 7 {
		t.Errorf("pni upload %+v", p)
	}
}

// TestUploadErrors: 401 and PNI 422 are ignored and change nothing; an
// attributes 401 marks the account unregistered (keys.md §2.3, api.md
// §4.3).
func TestUploadErrors(t *testing.T) {
	now := time.Now()
	a := linkedAccount(t, now)
	ks := &keyServer{status: map[string]int{"PUT /v2/keys?identity=aci": 401, "PUT /v2/keys?identity=pni": 422}}
	if err := CheckAccount(context.Background(), a, ks, now); err != nil {
		t.Fatalf("ignored errors surfaced: %v", err)
	}
	if a.Data.ACIKeys.NextPreKeyID != 0xFFFFF0 || len(a.Keys.ACI.PreKeys) != 1 {
		t.Error("a rejected upload was stored")
	}
	ks = &keyServer{status: map[string]int{"PUT /v2/keys?identity=aci": 422}}
	if err := CheckAccount(context.Background(), a, ks, now); err == nil {
		t.Error("ACI 422 should fail")
	}
	a.Data.Registered = true
	ks = &keyServer{status: map[string]int{"PUT /v1/accounts/attributes": 403}}
	if err := CheckAccount(context.Background(), a, ks, now); !errors.Is(err, ErrNotRegistered) || a.Data.Registered {
		t.Errorf("attributes 403: %v, registered %v", err, a.Data.Registered)
	}
}
