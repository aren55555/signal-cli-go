package protocol

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// testConfig uses placeholder info strings. The libsignal values come from
// spec/protocol/double-ratchet.md.
var testConfig = RatchetConfig{
	RootInfo:       "test root",
	MessageKeyInfo: "test message keys",
	MaxSkip:        50,
}

type party struct {
	name string
	st   *RatchetState
	rng  *rand.ChaCha8 // one stream per party, so every new ratchet key differs
}

type wire struct {
	h   RatchetHeader
	ct  []byte
	msg string
}

func newPair(t *testing.T) (alice, bob *party) {
	t.Helper()
	r := testRand(10)
	sk := bytes.Repeat([]byte{0x33}, 32)
	bobRatchet, err := curve25519.GeneratePrivateKey(r)
	if err != nil {
		t.Fatal(err)
	}
	a, err := InitRatchetAlice(&testConfig, r, sk, bobRatchet.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	b := InitRatchetBob(&testConfig, sk, bobRatchet)
	return &party{"alice", a, testRand(11)}, &party{"bob", b, testRand(12)}
}

var ad = []byte("associated data")

func (p *party) send(t *testing.T, msg string) wire {
	t.Helper()
	h, ct, err := p.st.Encrypt(p.rng, []byte(msg), ad)
	if err != nil {
		t.Fatalf("%s encrypt: %v", p.name, err)
	}
	return wire{h, ct, msg}
}

func (p *party) recv(t *testing.T, w wire) {
	t.Helper()
	pt, err := p.st.Decrypt(p.rng, w.h, w.ct, ad)
	if err != nil {
		t.Fatalf("%s decrypt %q: %v", p.name, w.msg, err)
	}
	if string(pt) != w.msg {
		t.Fatalf("%s got %q, want %q", p.name, pt, w.msg)
	}
}

func TestRatchetConversation(t *testing.T) {
	alice, bob := newPair(t)
	for round := range 5 {
		for i := range round + 1 {
			bob.recv(t, alice.send(t, fmt.Sprintf("a%d.%d", round, i)))
		}
		for i := range 2 {
			alice.recv(t, bob.send(t, fmt.Sprintf("b%d.%d", round, i)))
		}
	}
}

// TestRatchetOutOfOrder follows section 2.6 of the Double Ratchet
// specification: skipped messages in the current chain and in the
// previous chain (via PN) decrypt later.
func TestRatchetOutOfOrder(t *testing.T) {
	alice, bob := newPair(t)
	bob.recv(t, alice.send(t, "a1"))
	b1, b2, b3 := bob.send(t, "b1"), bob.send(t, "b2"), bob.send(t, "b3")
	alice.recv(t, b1)
	a2 := alice.send(t, "a2")
	bob.recv(t, a2)
	b4 := bob.send(t, "b4") // new chain: b2 and b3 are skipped from the old one
	alice.recv(t, b4)
	alice.recv(t, b3)
	alice.recv(t, b2)

	// Within one chain, reversed.
	var ws []wire
	for i := range 6 {
		ws = append(ws, alice.send(t, fmt.Sprintf("x%d", i)))
	}
	for i := len(ws) - 1; i >= 0; i-- {
		bob.recv(t, ws[i])
	}
}

func TestRatchetRejectsReplay(t *testing.T) {
	alice, bob := newPair(t)
	w := alice.send(t, "once")
	bob.recv(t, w)
	if _, err := bob.st.Decrypt(testRand(1), w.h, w.ct, ad); err == nil {
		t.Error("replayed message decrypted")
	}
	// A replayed skipped message also fails the second time.
	w1, w2 := alice.send(t, "one"), alice.send(t, "two")
	bob.recv(t, w2)
	bob.recv(t, w1)
	if _, err := bob.st.Decrypt(testRand(1), w1.h, w1.ct, ad); err == nil {
		t.Error("replayed skipped message decrypted")
	}
}

// TestRatchetFailureLeavesStateUnchanged: a failed decryption must not
// change the state (section 3.5), so later messages still decrypt.
func TestRatchetFailureLeavesStateUnchanged(t *testing.T) {
	alice, bob := newPair(t)
	bob.recv(t, alice.send(t, "hello"))

	w := alice.send(t, "tampered")
	bad := bytes.Clone(w.ct)
	bad[len(bad)-1] ^= 1
	if _, err := bob.st.Decrypt(testRand(1), w.h, bad, ad); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	if _, err := bob.st.Decrypt(testRand(1), w.h, w.ct, []byte("other ad")); err == nil {
		t.Fatal("wrong associated data accepted")
	}
	bob.recv(t, w)

	// A forged header with a fresh ratchet key fails without ratcheting.
	r := testRand(42)
	forged, _ := curve25519.GeneratePrivateKey(r)
	w2 := alice.send(t, "after")
	fh := w2.h
	fh.DH = forged.PublicKey()
	if _, err := bob.st.Decrypt(testRand(1), fh, w2.ct, ad); err == nil {
		t.Fatal("forged ratchet key accepted")
	}
	bob.recv(t, w2)
	alice.recv(t, bob.send(t, "still in sync"))
}

func TestRatchetMaxSkip(t *testing.T) {
	alice, bob := newPair(t)
	var last wire
	for i := range testConfig.MaxSkip + 2 {
		last = alice.send(t, fmt.Sprintf("m%d", i))
	}
	if _, err := bob.st.Decrypt(testRand(1), last.h, last.ct, ad); err == nil {
		t.Fatal("message beyond MaxSkip accepted")
	}
	// Exactly MaxSkip skipped keys is fine.
	alice2, bob2 := newPair(t)
	for i := range testConfig.MaxSkip + 1 {
		last = alice2.send(t, fmt.Sprintf("m%d", i))
	}
	bob2.recv(t, last)
}

// TestRatchetKDFs checks KDF_RK and KDF_CK against the recommendations
// in section 7.2: HKDF-SHA256 with rk as salt and dh_out as IKM; HMAC-
// SHA256 keyed by ck over 0x01 (message key) and 0x02 (next chain key).
func TestRatchetKDFs(t *testing.T) {
	rk := bytes.Repeat([]byte{1}, 32)
	dhOut := bytes.Repeat([]byte{2}, 32)
	gotRK, gotCK := kdfRK(&testConfig, rk, dhOut)
	out, err := hkdf.Key(sha256.New, dhOut, rk, testConfig.RootInfo, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotRK, out[:32]) || !bytes.Equal(gotCK, out[32:]) {
		t.Error("KDF_RK mismatch")
	}

	ck := bytes.Repeat([]byte{3}, 32)
	nextCK, mk := kdfCK(ck)
	mac := func(b byte) []byte {
		h := hmac.New(sha256.New, ck)
		h.Write([]byte{b})
		return h.Sum(nil)
	}
	if !bytes.Equal(mk, mac(1)) || !bytes.Equal(nextCK, mac(2)) {
		t.Error("KDF_CK mismatch")
	}
}
