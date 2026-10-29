// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/protocol"
)

// TestLinkRequestJSON: spec/service/linking.md §7 step 6 and keys.md §2.1:
// member order, gcmToken emitted as null, unpadded standard base64 for
// key material, and exactly five account attributes.
func TestLinkRequestJSON(t *testing.T) {
	id, _ := curve25519.GeneratePrivateKey(rand.Reader)
	now := time.Now()
	spk, _ := protocol.GenerateSignedPreKey(rand.Reader, id, 7, now)
	kpk, _ := protocol.GenerateKyberPreKey(rand.Reader, id, 9, now)
	req := LinkRequest{
		VerificationCode: "code",
		AccountAttributes: LinkAttributes{
			FetchesMessages: true, RegistrationID: 1234, PNIRegistrationID: 4321, Name: "bmFtZQ",
			Capabilities: LinkedDeviceCapabilities,
		},
		ACISignedPreKey: SignedPreKey(spk), PNISignedPreKey: SignedPreKey(spk),
		ACIPqLastResortPreKey: KyberPreKey(kpk), PNIPqLastResortPreKey: KyberPreKey(kpk),
	}
	b, err := Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	order := []string{`{"verificationCode":"code","accountAttributes":{"fetchesMessages":true,"registrationId":1234,"pniRegistrationId":4321,"name":"bmFtZQ","capabilities":{"storage":true,"versionedExpirationTimer":true,"attachmentBackfill":true,"spqr":true,"usernameChangeSyncMessage":true}},"aciSignedPreKey":{"keyId":7,"publicKey":"`,
		`"pniSignedPreKey":{"keyId":7,`, `"aciPqLastResortPreKey":{"keyId":9,`, `"pniPqLastResortPreKey":{"keyId":9,`, `"gcmToken":null}`}
	at := 0
	for _, part := range order {
		i := strings.Index(s[at:], part)
		if i < 0 {
			t.Fatalf("missing or out of order: %s\nin %s", part, s[:min(len(s), 600)])
		}
		at += i + len(part)
	}
	var back struct {
		ACISignedPreKey struct{ PublicKey, Signature string } `json:"aciSignedPreKey"`
		ACIPq           struct{ PublicKey, Signature string } `json:"aciPqLastResortPreKey"`
	}
	json.Unmarshal(b, &back)
	for name, v := range map[string]struct {
		s    string
		want []byte
	}{
		"ec key": {back.ACISignedPreKey.PublicKey, spk.PublicKey()}, "ec sig": {back.ACISignedPreKey.Signature, spk.Signature},
		"kyber key": {back.ACIPq.PublicKey, kpk.PublicKey()}, "kyber sig": {back.ACIPq.Signature, kpk.Signature},
	} {
		got, err := base64.RawStdEncoding.DecodeString(v.s)
		if err != nil || !bytes.Equal(got, v.want) {
			t.Errorf("%s: %q is not unpadded base64 of the key (%v)", name, v.s[:min(len(v.s), 20)], err)
		}
	}
}

// TestDecodeBase64Lenient: api.md §3, padded or not, URL-safe if it
// contains - or _.
func TestDecodeBase64Lenient(t *testing.T) {
	want := []byte{0xfb, 0xff, 0xfe}
	for _, s := range []string{"+//+", "-__-", "+//+==="} {
		if got, err := DecodeBase64(s); err != nil || !bytes.Equal(got, want) {
			t.Errorf("%q: %x, %v", s, got, err)
		}
	}
	if got, err := DecodeBase64("YQ=="); err != nil || string(got) != "a" {
		t.Errorf("padded: %q, %v", got, err)
	}
	if got, err := DecodeBase64("YQ"); err != nil || string(got) != "a" {
		t.Errorf("unpadded: %q, %v", got, err)
	}
}

// TestUnidentifiedAccessKey: api.md §5.1 test value.
func TestUnidentifiedAccessKey(t *testing.T) {
	if got := hex.EncodeToString(UnidentifiedAccessKey(make([]byte, 32))); got != "cea7403d4d606b6e074ec5d3baf39d18" {
		t.Errorf("UAK %s", got)
	}
}

// TestAccountAttributesJSON: registration.md §4.2, null members emitted,
// in this order.
func TestAccountAttributesJSON(t *testing.T) {
	b, _ := Marshal(AccountAttributes{RegistrationID: 1234, FetchesMessages: true, UnidentifiedAccessKey: make([]byte, 16),
		DiscoverableByPhoneNumber: true, Capabilities: LinkedDeviceCapabilities, Name: ptr("bmFtZQ"), PNIRegistrationID: 5678})
	want := `{"signalingKey":null,"registrationId":1234,"voice":false,"video":false,"fetchesMessages":true,"registrationLock":null,` +
		`"unidentifiedAccessKey":"AAAAAAAAAAAAAAAAAAAAAA==","unrestrictedUnidentifiedAccess":false,"discoverableByPhoneNumber":true,` +
		`"capabilities":{"storage":true,"versionedExpirationTimer":true,"attachmentBackfill":true,"spqr":true,"usernameChangeSyncMessage":true},` +
		`"name":"bmFtZQ","pniRegistrationId":5678,"recoveryPassword":null}`
	if string(b) != want {
		t.Errorf("got\n%s\nwant\n%s", b, want)
	}
}

func ptr[T any](v T) *T { return &v }

// TestPreKeyUploadJSON: keys.md §2.2, nulls emitted.
func TestPreKeyUploadJSON(t *testing.T) {
	b, _ := Marshal(PreKeyUpload{PreKeys: []PreKeyEntity{{KeyID: 3, PublicKey: "k"}}})
	if want := `{"preKeys":[{"keyId":3,"publicKey":"k"}],"signedPreKey":null,"pqLastResortPreKey":null,"pqPreKeys":null}`; string(b) != want {
		t.Errorf("got %s", b)
	}
}
