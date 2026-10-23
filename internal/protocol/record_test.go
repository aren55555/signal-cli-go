package protocol

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/protocol/spqr"
)

var (
	t0        = time.Unix(1_800_000_000, 0)
	aliceAddr = Address{ServiceID: "aaaaaaaa-7000-11eb-b32a-33b8a8a487a6", DeviceID: 1}
	bobAddr   = Address{ServiceID: "bbbbbbbb-7000-11eb-b32a-33b8a8a487a6", DeviceID: 2}
)

// bobStore is an in-memory PreKeyLookup with Bob's keys from newBob.
type bobStore struct{ b *bob }

func (s bobStore) SignedPreKey(id uint32) (*curve25519.PrivateKey, error) {
	if id != s.b.bundle.SignedPreKeyID {
		return nil, ErrInvalidSignedPreKeyID
	}
	return s.b.signedPreKey, nil
}

func (s bobStore) KyberPreKey(id uint32) (*KEMSecretKey, error) {
	if id != s.b.bundle.KyberPreKeyID {
		return nil, ErrInvalidKyberPreKeyID
	}
	return s.b.kem, nil
}

func (s bobStore) PreKey(id uint32) (*curve25519.PrivateKey, error) {
	if s.b.bundle.PreKeyID == nil || id != *s.b.bundle.PreKeyID || s.b.oneTime == nil {
		return nil, ErrInvalidPreKeyID
	}
	return s.b.oneTime, nil
}

type duo struct {
	alice, bob  *SessionRecord
	b           *bob
	aliceID     *curve25519.PrivateKey
	rngA, rngB  *testReader
	trustAll    func(*curve25519.PublicKey) bool
	lastBobUsed *UsedPreKeys
	aliceRegID  uint32
	bobRegID    uint32
}

type testReader struct {
	r interface{ Read([]byte) (int, error) }
}

func (t *testReader) Read(p []byte) (int, error) { return t.r.Read(p) }

func newDuo(t *testing.T, seed uint64) *duo {
	t.Helper()
	r := testRand(seed)
	d := &duo{b: newBob(t, r, true), rngA: &testReader{testRand(seed + 100)}, rngB: &testReader{testRand(seed + 200)},
		trustAll: func(*curve25519.PublicKey) bool { return true }, aliceRegID: 111, bobRegID: 222}
	d.aliceID, _ = curve25519.GeneratePrivateKey(r)
	d.b.bundle.RegistrationID = d.bobRegID
	rec, err := NewInitiatorRecord(d.rngA, d.aliceID, d.aliceRegID, d.b.bundle, t0)
	if err != nil {
		t.Fatal(err)
	}
	d.alice = rec
	d.bob = new(SessionRecord)
	return d
}

func (d *duo) bobKeys() LocalIdentity {
	return LocalIdentity{Key: d.b.identity, RegistrationID: d.bobRegID}
}

// aliceSend encrypts with Alice's current state and commits.
func (d *duo) aliceSend(t *testing.T, msg string, now time.Time) (int, []byte) {
	t.Helper()
	next, typ, wire, err := d.alice.Encrypt(d.rngA, []byte(msg), now, aliceAddr, bobAddr)
	if err != nil {
		t.Fatalf("alice encrypt: %v", err)
	}
	d.alice = next
	return typ, wire
}

func (d *duo) bobSend(t *testing.T, msg string) []byte {
	t.Helper()
	next, typ, wire, err := d.bob.Encrypt(d.rngB, []byte(msg), t0, bobAddr, aliceAddr)
	if err != nil {
		t.Fatalf("bob encrypt: %v", err)
	}
	if typ != MessageTypeWhisper {
		t.Fatalf("bob sent type %d", typ)
	}
	d.bob = next
	return wire
}

// bobReceive handles either message type and commits on success.
func (d *duo) bobReceive(t *testing.T, typ int, wire []byte) (string, error) {
	t.Helper()
	switch typ {
	case MessageTypePreKey:
		next, pt, used, err := d.bob.DecryptPreKey(d.rngB, wire, d.bobKeys(), bobStore{d.b}, d.trustAll, bobAddr, aliceAddr)
		if err != nil {
			return "", err
		}
		d.bob, d.lastBobUsed = next, used
		return string(pt), nil
	default:
		next, pt, err := d.bob.Decrypt(d.rngB, wire, bobAddr, aliceAddr)
		if err != nil {
			return "", err
		}
		d.bob = next
		return string(pt), nil
	}
}

func (d *duo) aliceReceive(t *testing.T, wire []byte) string {
	t.Helper()
	next, pt, err := d.alice.Decrypt(d.rngA, wire, aliceAddr, bobAddr)
	if err != nil {
		t.Fatalf("alice decrypt: %v", err)
	}
	d.alice = next
	return string(pt)
}

func TestRecordFullExchange(t *testing.T) {
	d := newDuo(t, 50)
	typ, wire := d.aliceSend(t, "hello bob", t0)
	if typ != MessageTypePreKey {
		t.Fatalf("first message type %d, want prekey", typ)
	}
	got, err := d.bobReceive(t, typ, wire)
	if err != nil || got != "hello bob" {
		t.Fatalf("bob got %q, %v", got, err)
	}
	if d.lastBobUsed == nil || d.lastBobUsed.OneTimePreKeyID == nil || *d.lastBobUsed.OneTimePreKeyID != 3 ||
		d.lastBobUsed.KyberPreKeyID != 9 || d.lastBobUsed.SignedPreKeyID != 7 {
		t.Errorf("used prekeys %+v", d.lastBobUsed)
	}
	if d.bob.current.remoteRegistrationID != d.aliceRegID {
		t.Errorf("bob's remote registration ID %d", d.bob.current.remoteRegistrationID)
	}
	if got := d.aliceReceive(t, d.bobSend(t, "hi alice")); got != "hi alice" {
		t.Fatalf("alice got %q", got)
	}
	// Alice heard back: no more prekey messages.
	typ, wire = d.aliceSend(t, "second", t0)
	if typ != MessageTypeWhisper {
		t.Fatalf("after a reply, type %d, want whisper", typ)
	}
	if got, err := d.bobReceive(t, typ, wire); err != nil || got != "second" {
		t.Fatalf("bob got %q, %v", got, err)
	}
}

func TestRecordRepeatedPreKeyMessages(t *testing.T) {
	d := newDuo(t, 51)
	typ1, w1 := d.aliceSend(t, "one", t0)
	typ2, w2 := d.aliceSend(t, "two", t0)
	if typ2 != MessageTypePreKey {
		t.Fatal("second unacknowledged message is not a prekey message")
	}
	if _, err := d.bobReceive(t, typ1, w1); err != nil {
		t.Fatal(err)
	}
	// The second matches the session by base key: no prekeys used again.
	if got, err := d.bobReceive(t, typ2, w2); err != nil || got != "two" {
		t.Fatalf("second: %q, %v", got, err)
	}
	if d.lastBobUsed != nil {
		t.Errorf("prekeys used again: %+v", d.lastBobUsed)
	}
	// Replaying the first is a duplicate, and changes nothing.
	before := d.bob
	if _, err := d.bobReceive(t, typ1, w1); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("replay: %v, want ErrDuplicateMessage", err)
	}
	if d.bob != before {
		t.Error("record replaced after a failed decrypt")
	}
}

func TestRecordFailuresChangeNothing(t *testing.T) {
	d := newDuo(t, 52)
	typ, wire := d.aliceSend(t, "x", t0)

	bad := bytes.Clone(wire)
	// The last byte is in the Kyber ciphertext (field 8 comes last):
	// implicit rejection gives different keys, so the inner MAC fails.
	bad[len(bad)-1] ^= 1
	if _, err := d.bobReceive(t, typ, bad); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("bad MAC: %v", err)
	}
	if d.bob.current != nil {
		t.Error("a session was stored after a failed prekey decrypt")
	}

	d.trustAll = func(*curve25519.PublicKey) bool { return false }
	if _, err := d.bobReceive(t, typ, wire); !errors.Is(err, ErrUntrustedIdentity) {
		t.Errorf("untrusted: %v", err)
	}
	d.trustAll = func(*curve25519.PublicKey) bool { return true }
	if got, err := d.bobReceive(t, typ, wire); err != nil || got != "x" {
		t.Fatalf("after failures: %q, %v", got, err)
	}

	// A tampered whisper message from Bob leaves Alice's record as is.
	w := d.bobSend(t, "y")
	w2 := bytes.Clone(w)
	w2[len(w2)-1] ^= 1
	if _, _, err := d.alice.Decrypt(d.rngA, w2, aliceAddr, bobAddr); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("tampered whisper: %v", err)
	}
	if got := d.aliceReceive(t, w); got != "y" {
		t.Fatal(got)
	}
}

// TestRecordArchiveAndPromotion: Alice starts a second session; Bob
// archives the first, and a late message from the first session still
// decrypts, promoting its state (double-ratchet.md §8).
func TestRecordArchiveAndPromotion(t *testing.T) {
	d := newDuo(t, 53)
	typ, w := d.aliceSend(t, "s1-a", t0)
	if _, err := d.bobReceive(t, typ, w); err != nil {
		t.Fatal(err)
	}
	d.aliceReceive(t, d.bobSend(t, "s1-b"))
	_, late := d.aliceSend(t, "s1-late", t0) // a whisper message on session 1

	// Alice starts session 2 from a fresh bundle (same Bob keys, new
	// one-time prekey).
	r := testRand(99)
	d.b.oneTime, _ = curve25519.GeneratePrivateKey(r)
	id := uint32(4)
	d.b.bundle.PreKeyID, d.b.bundle.PreKey = &id, d.b.oneTime.PublicKey()
	rec2, err := NewInitiatorRecord(d.rngA, d.aliceID, d.aliceRegID, d.b.bundle, t0)
	if err != nil {
		t.Fatal(err)
	}
	d.alice = rec2
	typ, w = d.aliceSend(t, "s2-a", t0)
	if got, err := d.bobReceive(t, typ, w); err != nil || got != "s2-a" {
		t.Fatalf("session 2: %q, %v", got, err)
	}
	if len(d.bob.previous) != 1 {
		t.Fatalf("bob has %d archived states, want 1", len(d.bob.previous))
	}
	s2 := d.bob.current
	if got, err := d.bobReceive(t, MessageTypeWhisper, late); err != nil || got != "s1-late" {
		t.Fatalf("late session-1 message: %q, %v", got, err)
	}
	if d.bob.current == s2 || len(d.bob.previous) != 1 {
		t.Error("the session-1 state was not promoted")
	}
}

func TestRecordArchiveLimit(t *testing.T) {
	rec := new(SessionRecord)
	for i := range MaxArchivedStates + 5 {
		rec = rec.withCurrent(&SessionState{version: 4, rootKey: []byte{byte(i)}})
	}
	if len(rec.previous) != MaxArchivedStates {
		t.Errorf("%d archived states, want %d", len(rec.previous), MaxArchivedStates)
	}
	// Newest first: the most recently archived is the one before current.
	if rec.previous[0].rootKey[0] != byte(MaxArchivedStates+3) {
		t.Errorf("archive order: front holds state %d", rec.previous[0].rootKey[0])
	}
}

func TestRecordUnacknowledgedSessionExpires(t *testing.T) {
	d := newDuo(t, 54)
	if _, _, _, err := d.alice.Encrypt(d.rngA, []byte("late"), t0.Add(MaxUnacknowledgedSessionAge+time.Second), aliceAddr, bobAddr); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("31-day-old pending session: %v, want ErrSessionNotFound", err)
	}
	if _, _, _, err := d.alice.Encrypt(d.rngA, []byte("ok"), t0.Add(MaxUnacknowledgedSessionAge), aliceAddr, bobAddr); err != nil {
		t.Errorf("exactly 30 days: %v", err)
	}
}

func TestAddressBinding(t *testing.T) {
	b, ok := addressBinding(aliceAddr, Address{ServiceID: "PNI:bbbbbbbb-7000-11eb-b32a-33b8a8a487a6", DeviceID: 9})
	if !ok || len(b) != 36 {
		t.Fatalf("binding %x %v", b, ok)
	}
	if b[0] != 0x00 || b[17] != 1 || b[18] != 0x01 || b[35] != 9 || b[1] != 0xaa || b[19] != 0xbb {
		t.Errorf("layout %x", b)
	}
	if _, ok := addressBinding(Address{ServiceID: "+15555550123", DeviceID: 1}, bobAddr); ok {
		t.Error("a non-service-ID name produced a binding")
	}

	// The prekey message's inner message carries the binding; Bob with
	// the wrong idea of who is talking rejects it.
	d := newDuo(t, 55)
	typ, wire := d.aliceSend(t, "bound", t0)
	pm, err := parsePreKeyMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := addressBinding(aliceAddr, bobAddr); !bytes.Equal(pm.inner.addresses, want) {
		t.Errorf("inner addresses %x, want %x", pm.inner.addresses, want)
	}
	wrong := Address{ServiceID: "cccccccc-7000-11eb-b32a-33b8a8a487a6", DeviceID: 1}
	if _, _, _, err := d.bob.DecryptPreKey(d.rngB, wire, d.bobKeys(), bobStore{d.b}, d.trustAll, bobAddr, wrong); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("wrong sender address: %v", err)
	}
	if got, err := d.bobReceive(t, typ, wire); err != nil || got != "bound" {
		t.Fatalf("right address: %q, %v", got, err)
	}
	// Whisper messages carry no binding.
	if m, err := parseRatchetMessage(d.bobSend(t, "plain")); err != nil || m.addresses != nil {
		t.Errorf("whisper message binding: %x, %v", m.addresses, err)
	}
}

func TestRecordRejectsNewV3Session(t *testing.T) {
	d := newDuo(t, 56)
	_, wire := d.aliceSend(t, "v3", t0)
	wire = bytes.Clone(wire)
	wire[0] = 0x34
	if _, err := d.bobReceive(t, MessageTypePreKey, wire); !errors.Is(err, ErrInvalidMessage) {
		t.Errorf("new v3 session: %v", err)
	}
}

// TestSPQRInSession: every SignalMessage carries a V1 SPQR message, and a
// long conversation advances SPQR epochs on both sides (session.md §3, §5).
func TestSPQRInSession(t *testing.T) {
	d := newDuo(t, 57)
	typ, w := d.aliceSend(t, "start", t0)
	if _, err := d.bobReceive(t, typ, w); err != nil {
		t.Fatal(err)
	}
	pm, _ := parsePreKeyMessage(w)
	if len(pm.inner.pqRatchet) == 0 || pm.inner.pqRatchet[0] != 0x01 {
		t.Fatalf("first message pq_ratchet %x", pm.inner.pqRatchet)
	}
	for i := range 400 {
		w := d.bobSend(t, "b")
		if m, _ := parseRatchetMessage(w); len(m.pqRatchet) == 0 || m.pqRatchet[0] != 0x01 {
			t.Fatalf("message %d without SPQR data", i)
		}
		d.aliceReceive(t, w)
		typ, w = d.aliceSend(t, "a", t0)
		if _, err := d.bobReceive(t, typ, w); err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
	}
	ea, eb := d.alice.current.pq.Epoch(), d.bob.current.pq.Epoch()
	if ea < 4 || eb < 4 {
		t.Errorf("SPQR epochs after 400 rounds: alice %d, bob %d", ea, eb)
	}
}

// TestPeerWithoutSPQR: a message with no SPQR data (an SPQR V0 sender)
// fails to decrypt in a V1 session (session.md §7).
func TestPeerWithoutSPQR(t *testing.T) {
	d := newDuo(t, 58)
	typ, w := d.aliceSend(t, "x", t0)
	if _, err := d.bobReceive(t, typ, w); err != nil {
		t.Fatal(err)
	}
	d.aliceReceive(t, d.bobSend(t, "y"))
	cur := d.alice.current.clone()
	cur.pq = &spqr.State{} // V0: empty SPQR messages, no salt
	d.alice = d.alice.replaceCurrent(cur)
	_, w = d.aliceSend(t, "no spqr", t0)
	if m, _ := parseRatchetMessage(w); len(m.pqRatchet) != 0 {
		t.Fatalf("V0 sender wrote pq_ratchet %x", m.pqRatchet)
	}
	if _, err := d.bobReceive(t, MessageTypeWhisper, w); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("message without SPQR: %v, want ErrDecryptionFailed", err)
	}
}

func TestServiceIDFromBinary(t *testing.T) {
	aci, _ := serviceIDBinary(aliceAddr.ServiceID)
	for _, c := range []struct {
		in   []byte
		want string
	}{
		{aci[1:], aliceAddr.ServiceID},
		{aci, aliceAddr.ServiceID},
		{append([]byte{0x01}, aci[1:]...), "PNI:" + aliceAddr.ServiceID},
	} {
		if got, err := ServiceIDFromBinary(c.in); err != nil || got != c.want {
			t.Errorf("%x: %q, %v", c.in, got, err)
		}
		// And back.
		if b, ok := serviceIDBinary(c.want); !ok || !bytes.Equal(b[1:], aci[1:]) {
			t.Errorf("%q does not convert back", c.want)
		}
	}
	for _, bad := range [][]byte{nil, make([]byte, 15), append([]byte{0x02}, aci[1:]...)} {
		if _, err := ServiceIDFromBinary(bad); err == nil {
			t.Errorf("%x accepted", bad)
		}
	}
}
