package manager

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

func uuidBytes(s string) []byte {
	b, _ := hex.DecodeString(strings.ReplaceAll(strings.TrimPrefix(s, "PNI:"), "-", ""))
	return b
}

// testAccount is a linked device's store with one identity's worth of
// keys on each of ACI and PNI, and the public bundles a sender would fetch.
func testAccount(t *testing.T) (*store.Account, map[string]*protocol.PreKeyBundle) {
	return testAccountFor(t, testNumber, testACI, testPNI, 2, nil)
}

func testAccountFor(t *testing.T, number, aci, pni string, device int, aciIdentity *curve25519.PrivateKey) (*store.Account, map[string]*protocol.PreKeyBundle) {
	t.Helper()
	a, err := store.Open(t.TempDir()).Create(number, store.EnvStaging)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.Data.ACI, a.Data.PNI, a.Data.DeviceID = aci, pni, device
	a.Data.ProfileKey = make([]byte, 32)
	bundles := map[string]*protocol.PreKeyBundle{}
	for _, side := range []struct {
		name  string
		pair  *store.KeyPair
		regID *uint32
		keys  *store.IdentityKeys
	}{{"aci", &a.Data.ACIIdentity, &a.Data.ACIRegistrationID, &a.Keys.ACI}, {"pni", &a.Data.PNIIdentity, &a.Data.PNIRegistrationID, &a.Keys.PNI}} {
		id, _ := curve25519.GeneratePrivateKey(rand.Reader)
		if side.name == "aci" && aciIdentity != nil {
			id = aciIdentity
		}
		*side.pair = store.KeyPair{Public: id.PublicKey().Bytes(), Private: id.Bytes()}
		*side.regID = 77
		spk, _ := protocol.GenerateSignedPreKey(rand.Reader, id, 5, time.Now())
		kpk, _ := protocol.GenerateKyberPreKey(rand.Reader, id, 6, time.Now())
		otk, _ := protocol.GeneratePreKey(rand.Reader, 8)
		side.keys.SignedPreKeys = map[uint32]store.StoredKey{5: {Record: spk.Serialize()}}
		side.keys.KyberPreKeys = map[uint32]store.StoredKey{6: {Record: kpk.Serialize(), LastResort: true}}
		side.keys.PreKeys = map[uint32]store.StoredKey{8: {Record: otk.Serialize()}}
		spub, _ := curve25519.ParsePublicKey(spk.PublicKey())
		kpub, _ := protocol.ParseKEMPublicKey(kpk.PublicKey())
		opub, _ := curve25519.ParsePublicKey(otk.PublicKey())
		eight := uint32(8)
		bundles[side.name] = &protocol.PreKeyBundle{RegistrationID: 77, IdentityKey: id.PublicKey(), SignedPreKeyID: 5, SignedPreKey: spub,
			SignedPreKeySignature: spk.Signature, PreKeyID: &eight, PreKey: opub, KyberPreKeyID: 6, KyberPreKey: kpub, KyberPreKeySignature: kpk.Signature}
	}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveKeys(); err != nil {
		t.Fatal(err)
	}
	return a, bundles
}

type sender struct {
	aci    string
	id     *curve25519.PrivateKey
	record *protocol.SessionRecord
}

func newSender(t *testing.T, aci string, bundle *protocol.PreKeyBundle, local, remote protocol.Address) *sender {
	id, _ := curve25519.GeneratePrivateKey(rand.Reader)
	rec, err := protocol.NewInitiatorRecord(rand.Reader, id, 99, bundle, time.Now(), local, remote)
	if err != nil {
		t.Fatal(err)
	}
	return &sender{aci: aci, id: id, record: rec}
}

func (s *sender) encrypt(t *testing.T, text string, local, remote protocol.Address) (int, []byte) {
	pt, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{Body: proto.String(text)}}})
	next, typ, wire, err := s.record.Encrypt(rand.Reader, content.Pad(pt), time.Now(), local, remote)
	if err != nil {
		t.Fatal(err)
	}
	s.record = next
	return typ, wire
}

func bodyOf(t *testing.T, m *Message) string {
	var c signalpb.AppContent
	if err := proto.Unmarshal(m.Content, &c); err != nil {
		t.Fatal(err)
	}
	return c.GetDataMessage().GetBody()
}

const bobACI = "b0b0b0b0-0000-4000-8000-000000000001"

func TestDecryptPreKeyAndWhisper(t *testing.T) {
	a, bundles := testAccount(t)
	d := &Decryptor{Acct: a}
	me := protocol.Address{ServiceID: testACI, DeviceID: 2}
	bob := protocol.Address{ServiceID: bobACI, DeviceID: 1}
	s := newSender(t, bobACI, bundles["aci"], bob, me)

	typ, wire := s.encrypt(t, "first", bob, me)
	if typ != protocol.MessageTypePreKey {
		t.Fatalf("type %d", typ)
	}
	env := &signalpb.ServiceEnvelope{Type: proto.Uint32(3), SourceServiceIdBinary: uuidBytes(bobACI), SourceDeviceId: proto.Uint32(1),
		DestinationServiceIdBinary: uuidBytes(testACI), Content: wire, ClientTimestamp: proto.Uint64(1), ServerTimestamp: proto.Uint64(2)}
	m, err := d.Decrypt(env)
	if err != nil {
		t.Fatal(err)
	}
	if bodyOf(t, m) != "first" || m.Sender != bob || m.Sealed || m.ToPNI {
		t.Errorf("message %+v", m)
	}
	if _, ok := a.Keys.ACI.PreKeys[8]; ok {
		t.Error("one-time prekey not consumed")
	}
	if _, ok := a.Keys.ACI.KyberPreKeys[6]; !ok {
		t.Error("last-resort Kyber prekey removed")
	}
	if id := a.Identities[bobACI]; !bytes.Equal(id.Key, s.id.PublicKey().Bytes()) || id.Trust != store.TrustedUnverified {
		t.Errorf("identity %+v", id)
	}

	// The state is on disk: a fresh load continues the session.
	path := a.Path()
	a.Close()
	b, err := store.Open(strings.TrimSuffix(a.Dir(), "/"+path)).Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d = &Decryptor{Acct: b}
	_, wire = s.encrypt(t, "second", bob, me) // still a PreKey message: unacknowledged
	env.Content = wire
	if m, err := d.Decrypt(env); err != nil || bodyOf(t, m) != "second" {
		t.Fatalf("second: %v", err)
	}
}

func TestDecryptToPNIAndSealed(t *testing.T) {
	a, bundles := testAccount(t)
	root, _ := curve25519.GeneratePrivateKey(rand.Reader)
	d := &Decryptor{Acct: a, Roots: []*curve25519.PublicKey{root.PublicKey()}}

	// A message addressed to the PNI uses the PNI identity and keys.
	mePNI := protocol.Address{ServiceID: "PNI:" + testPNI, DeviceID: 2}
	bob := protocol.Address{ServiceID: bobACI, DeviceID: 1}
	s := newSender(t, bobACI, bundles["pni"], bob, mePNI)
	_, wire := s.encrypt(t, "to pni", bob, mePNI)
	m, err := d.Decrypt(&signalpb.ServiceEnvelope{Type: proto.Uint32(3), SourceServiceIdBinary: uuidBytes(bobACI), SourceDeviceId: proto.Uint32(1),
		DestinationServiceIdBinary: append([]byte{1}, uuidBytes(testPNI)...), Content: wire})
	if err != nil || bodyOf(t, m) != "to pni" || !m.ToPNI {
		t.Fatalf("to PNI: %+v, %v", m, err)
	}
	if _, ok := a.Keys.PNI.PreKeys[8]; ok {
		t.Error("PNI one-time prekey not consumed")
	}

	// Sealed sender v1 around a PreKey message to the ACI.
	const carolACI = "ca401ca4-0000-4000-8000-000000000003"
	me := protocol.Address{ServiceID: testACI, DeviceID: 2}
	carol := protocol.Address{ServiceID: carolACI, DeviceID: 3}
	c := newSender(t, carolACI, bundles["aci"], carol, me)
	_, inner := c.encrypt(t, "sealed hello", carol, me)
	cert := mintCert(t, root, carolACI, c.id.PublicKey(), 3)
	usmc := protocol.EncodeSealedContent(protocol.SealedContentPreKey, cert, inner, 0, nil)
	aciPub, _ := curve25519.ParsePublicKey(a.Data.ACIIdentity.Public)
	sealed, err := protocol.SealV1(rand.Reader, c.id, aciPub, usmc)
	if err != nil {
		t.Fatal(err)
	}
	m, err = d.Decrypt(&signalpb.ServiceEnvelope{Type: proto.Uint32(6), DestinationServiceIdBinary: uuidBytes(testACI), Content: sealed, ServerTimestamp: proto.Uint64(1000)})
	if err != nil || bodyOf(t, m) != "sealed hello" || !m.Sealed || m.Sender != carol {
		t.Fatalf("sealed: %+v, %v", m, err)
	}
}

// TestUntrustedIdentityChangesNothing: a sender whose identity key
// changed is refused (trust on first use) and no state is stored.
func TestUntrustedIdentityChangesNothing(t *testing.T) {
	a, bundles := testAccount(t)
	d := &Decryptor{Acct: a}
	me := protocol.Address{ServiceID: testACI, DeviceID: 2}
	bob := protocol.Address{ServiceID: bobACI, DeviceID: 1}
	a.Identities[bobACI] = store.Identity{Key: bytes.Repeat([]byte{5}, 33), Trust: store.TrustedUnverified}
	s := newSender(t, bobACI, bundles["aci"], bob, me)
	_, wire := s.encrypt(t, "hi", bob, me)
	_, err := d.Decrypt(&signalpb.ServiceEnvelope{Type: proto.Uint32(3), SourceServiceIdBinary: uuidBytes(bobACI), SourceDeviceId: proto.Uint32(1),
		DestinationServiceIdBinary: uuidBytes(testACI), Content: wire})
	if !errors.Is(err, protocol.ErrUntrustedIdentity) {
		t.Fatalf("decrypt: %v", err)
	}
	if _, ok := a.Keys.ACI.PreKeys[8]; !ok || len(a.Sessions) != 0 {
		t.Error("state changed after a refused message")
	}
}

func mintCert(t *testing.T, root *curve25519.PrivateKey, aci string, identity *curve25519.PublicKey, device uint32) []byte {
	server, _ := curve25519.GeneratePrivateKey(rand.Reader)
	sbody, _ := proto.Marshal(&signalpb.ServerCert_Body{KeyId: proto.Uint32(1), PublicKey: server.PublicKey().Bytes()})
	ssig, _ := root.Sign(rand.Reader, sbody)
	scert, _ := proto.Marshal(&signalpb.ServerCert{Body: sbody, Signature: ssig})
	body, _ := proto.Marshal(&signalpb.SenderCert_Body{
		DeviceId: proto.Uint32(device), Expires: proto.Uint64(1 << 62), IdentityKey: identity.Bytes(),
		Signer:    &signalpb.SenderCert_Body_ServerCert{ServerCert: scert},
		SenderAci: &signalpb.SenderCert_Body_AciString{AciString: aci},
	})
	sig, _ := server.Sign(rand.Reader, body)
	out, _ := proto.Marshal(&signalpb.SenderCert{Body: body, Signature: sig})
	return out
}

// TestServerDeliveryReceipt: type 5 is not decrypted and changes nothing
// (pipe.md §7 step 4).
func TestServerDeliveryReceipt(t *testing.T) {
	a, _ := testAccount(t)
	m, err := (&Decryptor{Acct: a}).Decrypt(&signalpb.ServiceEnvelope{Type: proto.Uint32(5), SourceServiceIdBinary: uuidBytes(bobACI),
		SourceDeviceId: proto.Uint32(1), ClientTimestamp: proto.Uint64(42), DestinationServiceIdBinary: uuidBytes(testACI)})
	if err != nil || !m.ServerReceipt || m.Content != nil || m.ClientTimestamp != 42 || m.Sender.ServiceID != bobACI {
		t.Fatalf("receipt %+v, %v", m, err)
	}
	if len(a.Sessions) != 0 || len(a.Identities) != 0 {
		t.Error("a receipt changed state")
	}
}
