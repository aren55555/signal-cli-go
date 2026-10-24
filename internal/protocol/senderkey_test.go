package protocol

import (
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

var testDistribution = DistributionID{0xd1, 0xd1, 0xd1, 0xd1, 0x70, 0x00, 0x11, 0xeb, 0xb3, 0x2a, 0x33, 0xb8, 0xa8, 0xa4, 0x87, 0xa6}

// TestSenderMessageKeys checks the derivation in sender-keys.md §2:
// HKDF-SHA256, no salt, info "WhisperGroup", 48 bytes, IV first.
func TestSenderMessageKeys(t *testing.T) {
	seed := bytes.Repeat([]byte{0x42}, 32)
	out, _ := hkdf.Key(sha256.New, seed, nil, "WhisperGroup", 48)
	iv, key := senderMessageKeys(seed)
	if !bytes.Equal(iv, out[:16]) || !bytes.Equal(key, out[16:]) {
		t.Error("sender message keys differ from the definition")
	}
}

type group struct {
	alice, bob *SenderKeyRecord
	rng        *testReader
}

func newGroup(t *testing.T) (*group, []byte) {
	t.Helper()
	g := &group{alice: new(SenderKeyRecord), bob: new(SenderKeyRecord), rng: &testReader{testRand(60)}}
	var skdm []byte
	var err error
	g.alice, skdm, err = g.alice.NewDistribution(g.rng, testDistribution)
	if err != nil {
		t.Fatal(err)
	}
	var id DistributionID
	g.bob, id, err = g.bob.ProcessDistribution(skdm)
	if err != nil || id != testDistribution {
		t.Fatalf("process: %v %x", err, id)
	}
	return g, skdm
}

func (g *group) send(t *testing.T, msg string) []byte {
	t.Helper()
	rec, skm, err := g.alice.Encrypt(g.rng, testDistribution, []byte(msg))
	if err != nil {
		t.Fatal(err)
	}
	g.alice = rec
	return skm
}

func (g *group) recv(t *testing.T, skm []byte, want string) {
	t.Helper()
	rec, pt, err := g.bob.Decrypt(skm)
	if err != nil {
		t.Fatalf("decrypt %q: %v", want, err)
	}
	if string(pt) != want {
		t.Fatalf("got %q, want %q", pt, want)
	}
	g.bob = rec
}

func TestGroupRoundTrip(t *testing.T) {
	g, _ := newGroup(t)
	var ms [][]byte
	for i := range 6 {
		ms = append(ms, g.send(t, fmt.Sprint("m", i)))
	}
	g.recv(t, ms[0], "m0")
	g.recv(t, ms[4], "m4")
	g.recv(t, ms[2], "m2")
	g.recv(t, ms[1], "m1")
	g.recv(t, ms[5], "m5")
	g.recv(t, ms[3], "m3")
	if _, _, err := g.bob.Decrypt(ms[2]); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("replay: %v", err)
	}
}

func TestGroupEncoding(t *testing.T) {
	g, skdm := newGroup(t)
	if skdm[0] != 0x33 {
		t.Errorf("SKDM version byte %#x", skdm[0])
	}
	skm := g.send(t, "hello")
	if skm[0] != 0x33 {
		t.Errorf("SKM version byte %#x", skm[0])
	}
	var pb signalpb.GroupSenderMessage
	if err := proto.Unmarshal(skm[1:len(skm)-64], &pb); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pb.DistributionId, testDistribution[:]) || pb.GetIteration() != 0 {
		t.Errorf("SKM fields %v", &pb)
	}
	// The signature covers the version byte and protobuf.
	st := g.alice.states[0]
	if !st.signingPub.Verify(skm[:len(skm)-64], skm[len(skm)-64:]) {
		t.Error("SKM signature does not verify over version byte and protobuf")
	}
}

func TestGroupLateJoiner(t *testing.T) {
	g := &group{alice: new(SenderKeyRecord), bob: new(SenderKeyRecord), rng: &testReader{testRand(61)}}
	var skdm []byte
	g.alice, skdm, _ = g.alice.NewDistribution(g.rng, testDistribution)
	early := g.send(t, "before")
	// A distribution made now starts at iteration 1.
	_, skdm2, _ := g.alice.NewDistribution(g.rng, testDistribution)
	if bytes.Equal(skdm, skdm2) {
		t.Fatal("the second SKDM did not move on")
	}
	g.bob, _, _ = g.bob.ProcessDistribution(skdm2)
	if _, _, err := g.bob.Decrypt(early); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("message before the SKDM's iteration: %v", err)
	}
	g.recv(t, g.send(t, "after"), "after")
}

func TestGroupLimits(t *testing.T) {
	g, _ := newGroup(t)
	var ms [][]byte
	for i := range MaxMessageKeys + 3 {
		ms = append(ms, g.send(t, fmt.Sprint(i)))
	}
	g.recv(t, ms[len(ms)-1], fmt.Sprint(len(ms)-1)) // skips 2002, keeps the newest 2000
	if _, _, err := g.bob.Decrypt(ms[1]); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("evicted key: %v", err)
	}
	g.recv(t, ms[2], "2")

	// Jump limit: drive the sender's chain directly.
	g2, _ := newGroup(t)
	g2.alice.states[0].iteration = MaxForwardJumps + 1
	for range MaxForwardJumps + 1 {
		_, g2.alice.states[0].chainKey = chainStep(g2.alice.states[0].chainKey)
	}
	if _, _, err := g2.bob.Decrypt(g2.send(t, "far")); err == nil {
		t.Error("jump of 25001 accepted")
	}
}

func TestGroupDistributionProcessing(t *testing.T) {
	g, skdm := newGroup(t)
	g.recv(t, g.send(t, "a"), "a")
	// The same SKDM again keeps the state's position.
	rec, _, err := g.bob.ProcessDistribution(skdm)
	if err != nil {
		t.Fatal(err)
	}
	g.bob = rec
	g.recv(t, g.send(t, "b"), "b")
	if len(g.bob.states) != 1 {
		t.Errorf("%d states after a repeated SKDM", len(g.bob.states))
	}

	// Same chain ID, different signing key: replaced.
	var pb signalpb.GroupSenderKeyHandoff
	if err := proto.Unmarshal(skdm[1:], &pb); err != nil {
		t.Fatal(err)
	}
	other, _ := curve25519.GeneratePrivateKey(testRand(62))
	pb.SigningKey = other.PublicKey().Bytes()
	body, _ := proto.Marshal(&pb)
	rec, _, err = g.bob.ProcessDistribution(append([]byte{0x33}, body...))
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.states) != 1 || !bytes.Equal(rec.states[0].signingPub.Bytes(), other.PublicKey().Bytes()) {
		t.Error("state with a new signing key did not replace the old one")
	}

	// At most 5 states.
	r := new(SenderKeyRecord)
	for i := range 7 {
		pb.ChainId = proto.Uint32(uint32(i))
		body, _ := proto.Marshal(&pb)
		if r, _, err = r.ProcessDistribution(append([]byte{0x33}, body...)); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.states) != MaxSenderKeyStates || r.states[0].chainID != 6 {
		t.Errorf("%d states, newest chain %d", len(r.states), r.states[0].chainID)
	}
}

func TestGroupFailures(t *testing.T) {
	g, _ := newGroup(t)
	skm := g.send(t, "x")
	bad := bytes.Clone(skm)
	bad[len(bad)-1] ^= 1
	before := g.bob
	if _, _, err := g.bob.Decrypt(bad); !errors.Is(err, ErrSignatureInvalid) {
		t.Errorf("bad signature: %v", err)
	}
	if g.bob != before {
		t.Error("record replaced")
	}
	g.recv(t, skm, "x")

	if _, _, err := new(SenderKeyRecord).Decrypt(skm); !errors.Is(err, ErrNoSenderKeyState) {
		t.Errorf("no state: %v", err)
	}
	if _, _, err := new(SenderKeyRecord).Encrypt(g.rng, testDistribution, []byte("x")); !errors.Is(err, ErrNoSenderKeyState) {
		t.Errorf("encrypt without state: %v", err)
	}
}

func TestGroupParseErrors(t *testing.T) {
	g, skdm := newGroup(t)
	skm := g.send(t, "x")
	var pb signalpb.GroupSenderKeyHandoff
	if err := proto.Unmarshal(skdm[1:], &pb); err != nil {
		t.Fatal(err)
	}
	mut := func(f func(*signalpb.GroupSenderKeyHandoff)) []byte {
		c := proto.Clone(&pb).(*signalpb.GroupSenderKeyHandoff)
		f(c)
		b, _ := proto.Marshal(c)
		return append([]byte{0x33}, b...)
	}
	for name, c := range map[string]struct {
		in  []byte
		err error
	}{
		"short":          {skdm[:64], ErrMessageTooShort},
		"version 2":      {append([]byte{0x23}, skdm[1:]...), ErrLegacyVersion},
		"version 4":      {append([]byte{0x43}, skdm[1:]...), ErrUnrecognizedVersion},
		"no chain id":    {mut(func(m *signalpb.GroupSenderKeyHandoff) { m.ChainId = nil }), ErrInvalidMessage},
		"short uuid":     {mut(func(m *signalpb.GroupSenderKeyHandoff) { m.DistributionId = m.DistributionId[:15] }), ErrInvalidMessage},
		"31-byte chain":  {mut(func(m *signalpb.GroupSenderKeyHandoff) { m.ChainKey = m.ChainKey[:31] }), ErrInvalidMessage},
		"34-byte signer": {mut(func(m *signalpb.GroupSenderKeyHandoff) { m.SigningKey = append(m.SigningKey, 0) }), ErrInvalidMessage},
	} {
		if _, _, err := new(SenderKeyRecord).ProcessDistribution(c.in); !errors.Is(err, c.err) {
			t.Errorf("SKDM %s: %v, want %v", name, err, c.err)
		}
	}
	if _, _, err := g.bob.Decrypt(skm[:64]); !errors.Is(err, ErrMessageTooShort) {
		t.Errorf("short SKM: %v", err)
	}
	if _, _, err := g.bob.Decrypt(append([]byte{0x43}, skm[1:]...)); !errors.Is(err, ErrUnrecognizedVersion) {
		t.Errorf("SKM version 4: %v", err)
	}
}

// FuzzGroupParsers: SKDMs and SKMs arrive from peers.
func FuzzGroupParsers(f *testing.F) {
	g, skdm := newGroup(&testing.T{})
	f.Add(skdm)
	f.Add(g.send(&testing.T{}, "seed"))
	f.Fuzz(func(t *testing.T, in []byte) {
		_, _, _ = new(SenderKeyRecord).ProcessDistribution(in)
		_, _, _ = g.bob.Decrypt(in)
	})
}

// TestSenderKeySerialize: a stored record (sender and receiver side, with
// skipped keys) reloads and keeps working.
func TestSenderKeySerialize(t *testing.T) {
	var id DistributionID
	alice, skdm, err := new(SenderKeyRecord).NewDistribution(rand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	bob, _, err := new(SenderKeyRecord).ProcessDistribution(skdm)
	if err != nil {
		t.Fatal(err)
	}
	var msgs [][]byte
	for i := 0; i < 3; i++ {
		var m []byte
		alice, m, _ = alice.Encrypt(rand.Reader, id, []byte{byte(i)})
		msgs = append(msgs, m)
	}
	bob, _, err = bob.Decrypt(msgs[2]) // skips 0 and 1
	if err != nil {
		t.Fatal(err)
	}
	reload := func(r *SenderKeyRecord) *SenderKeyRecord {
		b, err := r.Serialize()
		if err != nil {
			t.Fatal(err)
		}
		out, err := LoadSenderKeyRecord(b)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	alice, bob = reload(alice), reload(bob)
	if _, pt, err := bob.Decrypt(msgs[0]); err != nil || pt[0] != 0 {
		t.Fatalf("skipped key after reload: %v", err)
	}
	_, m, err := alice.Encrypt(rand.Reader, id, []byte{9})
	if err != nil {
		t.Fatal(err)
	}
	if _, pt, err := bob.Decrypt(m); err != nil || pt[0] != 9 {
		t.Fatalf("next message after reload: %v", err)
	}
}
