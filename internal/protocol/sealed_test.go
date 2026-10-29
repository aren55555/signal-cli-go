// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// libsignal's test trust root is the public key of the all-zero private
// key, and its test server key that of the all-0xFF key
// (sealed-sender.md §4.2), so tests can mint valid certificates.
func testRoot(t *testing.T) *curve25519.PrivateKey {
	k, err := curve25519.NewPrivateKey(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

const testUUID = "aaaaaaaa-7000-11eb-b32a-33b8a8a487a6"

// mintSenderCert returns a serialized SenderCertificate with an embedded
// ServerCertificate (key ID keyID) signed by root.
func mintSenderCert(t *testing.T, root *curve25519.PrivateKey, keyID uint32, identity *curve25519.PublicKey, device uint32, expires uint64) []byte {
	t.Helper()
	r := testRand(70)
	server, _ := curve25519.NewPrivateKey(bytes.Repeat([]byte{0xff}, 32))
	sbody, _ := proto.Marshal(&signalpb.ServerCert_Body{KeyId: proto.Uint32(keyID), PublicKey: server.PublicKey().Bytes()})
	ssig, _ := root.Sign(r, sbody)
	scert, _ := proto.Marshal(&signalpb.ServerCert{Body: sbody, Signature: ssig})
	body, _ := proto.Marshal(&signalpb.SenderCert_Body{
		DeviceId:    proto.Uint32(device),
		Expires:     proto.Uint64(expires),
		IdentityKey: identity.Bytes(),
		Signer:      &signalpb.SenderCert_Body_ServerCert{ServerCert: scert},
		SenderAci:   &signalpb.SenderCert_Body_AciString{AciString: testUUID},
	})
	sig, _ := server.Sign(r, body)
	out, _ := proto.Marshal(&signalpb.SenderCert{Body: body, Signature: sig})
	return out
}

func TestTrustRootsAndKnownCerts(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind, Name, Environment, Serialized string
			PublicKey                           string `json:"public_key"`
			TrustRoot                           string `json:"trust_root"`
			ID                                  uint32
		}
	}
	testvec.Load(t, "sealed_sender_certificates.json", &f)
	roots := map[string][]string{}
	for _, v := range f.Vectors {
		switch v.Kind {
		case "trust_root":
			roots[v.Environment] = append(roots[v.Environment], v.PublicKey)
		case "known_server_certificate":
			c, err := knownServerCert(v.ID)
			if err != nil {
				t.Fatalf("%s: %v", v.Name, err)
			}
			if hex.EncodeToString(c.raw) != v.Serialized {
				t.Errorf("%s: embedded certificate differs from the vector", v.Name)
			}
			root, _ := curve25519.ParsePublicKey(testvec.Hex(t, v.TrustRoot))
			if !c.validFor([]*curve25519.PublicKey{root}) {
				t.Errorf("%s: does not validate under its trust root", v.Name)
			}
		}
	}
	for env, want := range map[Environment]string{Production: "production", Staging: "staging"} {
		var got []string
		for _, k := range TrustRoots(env) {
			got = append(got, hex.EncodeToString(k.Bytes()))
		}
		if len(got) != 2 || got[0] != roots[want][0] || got[1] != roots[want][1] {
			t.Errorf("%s roots %v, want %v", want, got, roots[want])
		}
	}
	if _, err := knownServerCert(12345); !errors.Is(err, ErrUnknownServerCertificate) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestSenderCertVectorValidation(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind, Name string
			TrustRoot  string         `json:"trust_root"`
			Serialized string         `json:"serialized_sender_certificate"`
			Fields     map[string]any `json:"expected_fields"`
			Validation []struct {
				Time  uint64 `json:"validation_time"`
				Valid bool   `json:"expected_valid"`
			}
		}
	}
	testvec.Load(t, "sealed_sender_certificates.json", &f)
	n := 0
	for _, v := range f.Vectors {
		if v.Kind != "sender_certificate" {
			continue
		}
		n++
		c, err := ParseSenderCertificate(testvec.Hex(t, v.Serialized))
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if c.SenderUUID != v.Fields["sender_uuid"] || c.DeviceID != 1 || c.Expires != 31337 {
			t.Errorf("%s: parsed %q %d %d", v.Name, c.SenderUUID, c.DeviceID, c.Expires)
		}
		root, _ := curve25519.ParsePublicKey(testvec.Hex(t, v.TrustRoot))
		for _, val := range v.Validation {
			if got := c.Validate([]*curve25519.PublicKey{root}, val.Time); got != val.Valid {
				t.Errorf("%s at %d: valid=%v, want %v", v.Name, val.Time, got, val.Valid)
			}
		}
		// Not valid under the real roots.
		if c.Validate(TrustRoots(Production), 0) || c.Validate(TrustRoots(Staging), 0) {
			t.Errorf("%s: a test certificate validated under real roots", v.Name)
		}
	}
	if n != 3 {
		t.Errorf("ran %d sender-certificate vectors", n)
	}
}

func TestSenderCertRules(t *testing.T) {
	root := testRoot(t)
	roots := []*curve25519.PublicKey{root.PublicKey()}
	id, _ := curve25519.GeneratePrivateKey(testRand(71))
	c, err := ParseSenderCertificate(mintSenderCert(t, root, 7, id.PublicKey(), 3, 1000))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Validate(roots, 1000) || c.Validate(roots, 1001) {
		t.Error("expiry: valid at expires, invalid after")
	}
	other, _ := curve25519.GeneratePrivateKey(testRand(72))
	if c.Validate([]*curve25519.PublicKey{other.PublicKey()}, 0) {
		t.Error("validated under the wrong root")
	}
	// Any one matching root out of several is enough.
	if !c.Validate([]*curve25519.PublicKey{other.PublicKey(), root.PublicKey()}, 0) {
		t.Error("second root not tried")
	}
	revoked, _ := ParseSenderCertificate(mintSenderCert(t, root, 0xDEADC357, id.PublicKey(), 3, 1000))
	if revoked.Validate(roots, 0) {
		t.Error("revoked server key ID validated")
	}
	if _, err := ParseSenderCertificate(mintSenderCert(t, root, 7, id.PublicKey(), 128, 1000)); err == nil {
		t.Error("device 128 accepted")
	}
	// A tampered body fails the server key's signature.
	var pb signalpb.SenderCert
	_ = proto.Unmarshal(mintSenderCert(t, root, 7, id.PublicKey(), 3, 1000), &pb)
	pb.Body = append(pb.Body, 0x7a, 0x00) // an extra (unknown) field
	b, _ := proto.Marshal(&pb)
	if c, err := ParseSenderCertificate(b); err != nil || c.Validate(roots, 0) {
		t.Errorf("tampered certificate: %v", err)
	}
}

func TestAESCTRVector(t *testing.T) {
	var f struct {
		Vectors []struct{ Key, Plaintext, Ciphertext string }
	}
	testvec.Load(t, "sealed_sender_primitives.json", &f)
	v := f.Vectors[0]
	got := aesCTR(testvec.Hex(t, v.Key), testvec.Hex(t, v.Plaintext))
	if hex.EncodeToString(got) != v.Ciphertext {
		t.Errorf("AES-CTR %x, want %s", got, v.Ciphertext)
	}
}

type sealedParty struct {
	id   *curve25519.PrivateKey
	cert []byte
}

func sealedSetup(t *testing.T) (alice, bob, carol sealedParty, roots []*curve25519.PublicKey) {
	t.Helper()
	root := testRoot(t)
	mk := func(seed uint64) sealedParty {
		k, _ := curve25519.GeneratePrivateKey(testRand(seed))
		return sealedParty{k, mintSenderCert(t, root, 7, k.PublicKey(), 2, 1<<40)}
	}
	return mk(80), mk(81), mk(82), []*curve25519.PublicKey{root.PublicKey()}
}

func usmc(t *testing.T, cert []byte, content string) []byte {
	return EncodeSealedContent(SealedContentMessage, cert, []byte(content), ContentHintResendable, []byte("group"))
}

func TestSealedSenderV1(t *testing.T) {
	alice, bob, carol, roots := sealedSetup(t)
	r := &testReader{testRand(83)}
	msg, err := SealV1(r, alice.id, bob.id.PublicKey(), usmc(t, alice.cert, "hi bob"))
	if err != nil {
		t.Fatal(err)
	}
	if msg[0] != 0x11 {
		t.Errorf("version byte %#x", msg[0])
	}
	got, err := Unseal(bob.id, roots, 0, msg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Content) != "hi bob" || got.Kind != SealedContentMessage || got.ContentHint != ContentHintResendable ||
		string(got.GroupID) != "group" || got.Sender.SenderUUID != testUUID || got.Sender.DeviceID != 2 {
		t.Errorf("unsealed %+v", got)
	}
	// High nibble 0 is accepted as v1 (a libsignal quirk).
	if _, err := Unseal(bob.id, roots, 0, append([]byte{0x01}, msg[1:]...)); err != nil {
		t.Errorf("version byte 0x01: %v", err)
	}
	if _, err := Unseal(carol.id, roots, 0, msg); err == nil {
		t.Error("unsealed with the wrong recipient key")
	}
	// The certificate must name the identity that sealed the message.
	forged, _ := SealV1(r, alice.id, bob.id.PublicKey(), usmc(t, carol.cert, "spoof"))
	if _, err := Unseal(bob.id, roots, 0, forged); !errors.Is(err, ErrSealedSender) {
		t.Errorf("certificate of another identity: %v", err)
	}
	// Expired at the envelope's server timestamp.
	if _, err := Unseal(bob.id, roots, 1<<40+1, msg); !errors.Is(err, ErrSealedSender) {
		t.Errorf("expired certificate: %v", err)
	}
	for i := 1; i < len(msg); i++ {
		bad := bytes.Clone(msg)
		bad[i] ^= 1
		if _, err := Unseal(bob.id, roots, 0, bad); err == nil {
			t.Fatalf("flipping byte %d still unseals", i)
		}
	}
	for name, in := range map[string][]byte{"empty": nil, "version 3": append([]byte{0x33}, msg[1:]...)} {
		if _, err := Unseal(bob.id, roots, 0, in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSealedSenderV2(t *testing.T) {
	alice, bob, carol, roots := sealedSetup(t)
	bobID := ServiceIDFixed{0x00, 0xbb}
	carolID := ServiceIDFixed{0x01, 0xcc}
	excluded := ServiceIDFixed{0x00, 0xdd}
	sent, err := SealV2(&testReader{testRand(84)}, alice.id, []MultiRecipient{
		{ServiceID: bobID, IdentityKey: bob.id.PublicKey(), Devices: []RecipientDevice{{1, 100}, {2, 200}}},
		{ServiceID: carolID, IdentityKey: carol.id.PublicKey(), Devices: []RecipientDevice{{1, 0x3fff}}},
	}, []ServiceIDFixed{excluded}, usmc(t, alice.cert, "hi all"))
	if err != nil {
		t.Fatal(err)
	}
	if sent[0] != 0x23 {
		t.Errorf("version byte %#x", sent[0])
	}
	sm, err := ParseSentMessage(sent)
	if err != nil {
		t.Fatal(err)
	}
	if len(sm.Recipients) != 2 || len(sm.Excluded) != 1 || sm.Excluded[0] != excluded ||
		len(sm.Recipients[0].Devices) != 2 || sm.Recipients[0].Devices[1] != (RecipientDevice{2, 200}) {
		t.Fatalf("parsed %+v", sm)
	}
	for i, p := range []sealedParty{bob, carol} {
		recv := sm.ReceivedMessage(i)
		if recv[0] != 0x22 {
			t.Errorf("received version %#x", recv[0])
		}
		got, err := Unseal(p.id, roots, 0, recv)
		if err != nil || string(got.Content) != "hi all" {
			t.Fatalf("recipient %d: %q, %v", i, got.Content, err)
		}
	}
	// Bob's copy doesn't open with Carol's key.
	if _, err := Unseal(carol.id, roots, 0, sm.ReceivedMessage(0)); err == nil {
		t.Error("another recipient's copy unsealed")
	}
	// The AT binds the sender identity in the certificate.
	spoof, _ := SealV2(&testReader{testRand(85)}, alice.id, []MultiRecipient{
		{ServiceID: bobID, IdentityKey: bob.id.PublicKey(), Devices: []RecipientDevice{{1, 1}}},
	}, nil, usmc(t, carol.cert, "spoof"))
	sm2, _ := ParseSentMessage(spoof)
	if _, err := Unseal(bob.id, roots, 0, sm2.ReceivedMessage(0)); !errors.Is(err, ErrSealedSender) {
		t.Errorf("certificate of another identity: %v", err)
	}
	if _, err := Unseal(bob.id, roots, 0, sm.ReceivedMessage(0)[:80]); err == nil {
		t.Error("80-byte received message accepted")
	}
	if _, err := SealV2(&testReader{testRand(86)}, alice.id, []MultiRecipient{
		{ServiceID: bobID, IdentityKey: bob.id.PublicKey(), Devices: []RecipientDevice{{1, 0x4000}}},
	}, nil, []byte("x")); err == nil {
		t.Error("registration ID above 0x3fff accepted")
	}
}

func TestSentMessageVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			Name, Input string
			Error       string `json:"expected_error"`
			Recipients  []struct {
				ServiceID string     `json:"service_id_fixed_width"`
				Devices   [][]uint32 `json:"devices"`
				Received  string     `json:"received_message"`
			} `json:"expected_recipients"`
			Excluded []string `json:"expected_excluded_recipients"`
		}
	}
	testvec.Load(t, "sealed_sender_v2_sent_message.json", &f)
	if len(f.Vectors) != 16 {
		t.Fatalf("got %d vectors", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		sm, err := ParseSentMessage(testvec.Hex(t, v.Input))
		if v.Error != "" {
			if err == nil {
				t.Errorf("%s: accepted, want %s", v.Name, v.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", v.Name, err)
			continue
		}
		if len(sm.Recipients) != len(v.Recipients) || len(sm.Excluded) != len(v.Excluded) {
			t.Errorf("%s: %d recipients, %d excluded", v.Name, len(sm.Recipients), len(sm.Excluded))
			continue
		}
		for i, want := range v.Recipients {
			got := sm.Recipients[i]
			if hex.EncodeToString(got.ServiceID[:]) != want.ServiceID {
				t.Errorf("%s #%d: service ID %x", v.Name, i, got.ServiceID)
			}
			if len(got.Devices) != len(want.Devices) {
				t.Errorf("%s #%d: %d devices", v.Name, i, len(got.Devices))
				continue
			}
			for j, d := range want.Devices {
				if uint32(got.Devices[j].ID) != d[0] || uint32(got.Devices[j].RegistrationID) != d[1] {
					t.Errorf("%s #%d device %d: %+v, want %v", v.Name, i, j, got.Devices[j], d)
				}
			}
			if hex.EncodeToString(sm.ReceivedMessage(i)) != want.Received {
				t.Errorf("%s #%d: received message differs", v.Name, i)
			}
		}
		for i, want := range v.Excluded {
			if hex.EncodeToString(sm.Excluded[i][:]) != want {
				t.Errorf("%s: excluded %x", v.Name, sm.Excluded[i])
			}
		}
	}
}

// FuzzSealedParsers: sealed messages, SentMessages and certificates come
// from the network.
func FuzzSealedParsers(f *testing.F) {
	t := &testing.T{}
	alice, bob, _, roots := sealedSetup(t)
	v1, _ := SealV1(&testReader{testRand(90)}, alice.id, bob.id.PublicKey(), usmc(t, alice.cert, "x"))
	sent, _ := SealV2(&testReader{testRand(91)}, alice.id, []MultiRecipient{
		{ServiceID: ServiceIDFixed{0, 1}, IdentityKey: bob.id.PublicKey(), Devices: []RecipientDevice{{1, 1}}},
	}, nil, usmc(t, alice.cert, "y"))
	f.Add(v1)
	f.Add(sent)
	f.Add(alice.cert)
	f.Fuzz(func(t *testing.T, in []byte) {
		_, _ = Unseal(bob.id, roots, 0, in)
		if sm, err := ParseSentMessage(in); err == nil {
			for i := range sm.Recipients {
				_, _ = Unseal(bob.id, roots, 0, sm.ReceivedMessage(i))
			}
		}
		_, _ = ParseSenderCertificate(in)
	})
}
