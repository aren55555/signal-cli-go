// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// peer drives one side of a session at the message-key level: the header
// fields a SignalMessage would carry, and the derived keys.
type peer struct {
	name string
	st   *SessionState
	rng  interface{ Read([]byte) (int, error) }
}

type sent struct {
	ratchetKey *curve25519.PublicKey
	counter    uint32
	keys       messageKeys
}

func (p *peer) send(t *testing.T) sent {
	t.Helper()
	next, res, err := p.st.sendStep(nil)
	if err != nil {
		t.Fatalf("%s send: %v", p.name, err)
	}
	p.st = next
	return sent{res.ratchetKey, res.counter, res.keys}
}

func (p *peer) recv(t *testing.T, m sent) {
	t.Helper()
	next, keys, err := p.st.receiveStep(p.rng, m.ratchetKey, m.counter, nil)
	if err != nil {
		t.Fatalf("%s receive counter %d: %v", p.name, m.counter, err)
	}
	if !bytes.Equal(keys.cipherKey, m.keys.cipherKey) || !bytes.Equal(keys.macKey, m.keys.macKey) || !bytes.Equal(keys.iv, m.keys.iv) {
		t.Fatalf("%s: keys for counter %d differ", p.name, m.counter)
	}
	p.st = next
}

// newSession runs PQXDH and builds both session states per pqxdh.md §4.
func newSession(t *testing.T) (alice, bob *peer) {
	t.Helper()
	r := testRand(30)
	b := newBob(t, r, true)
	aliceID, _ := curve25519.GeneratePrivateKey(r)
	res, err := InitiatePQXDH(r, aliceID, b.bundle)
	if err != nil {
		t.Fatal(err)
	}
	as, err := NewAliceSession(r, res, aliceID.PublicKey(), b.bundle.IdentityKey, b.bundle.SignedPreKey)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := b.respond(t, aliceID.PublicKey(), res)
	if err != nil {
		t.Fatal(err)
	}
	bs := NewBobSession(secrets, b.signedPreKey, b.identity.PublicKey(), aliceID.PublicKey(), res.BaseKey)
	return &peer{"alice", as, testRand(31)}, &peer{"bob", bs, testRand(32)}
}

func TestSessionConversation(t *testing.T) {
	alice, bob := newSession(t)
	for round := range 4 {
		for range round + 1 {
			bob.recv(t, alice.send(t))
		}
		for range 2 {
			alice.recv(t, bob.send(t))
		}
	}
}

// TestSessionLateMessagesOnOldChains: libsignal keeps old receiving chains
// and ignores PN, so late messages from a previous chain still decrypt.
func TestSessionLateMessagesOnOldChains(t *testing.T) {
	alice, bob := newSession(t)
	bob.recv(t, alice.send(t))
	b1, b2, b3 := bob.send(t), bob.send(t), bob.send(t)
	alice.recv(t, b1)
	bob.recv(t, alice.send(t))
	b4 := bob.send(t)
	alice.recv(t, b4)
	alice.recv(t, b3)
	alice.recv(t, b2)

	var ms []sent
	for range 5 {
		ms = append(ms, alice.send(t))
	}
	for i := len(ms) - 1; i >= 0; i-- {
		bob.recv(t, ms[i])
	}
}

func TestSessionPreviousCounter(t *testing.T) {
	alice, bob := newSession(t)
	bob.recv(t, alice.send(t))
	for range 3 {
		alice.recv(t, bob.send(t))
	}
	// Alice sent 1 message on her first chain (index 1); PN = index - 1.
	if alice.st.previousCounter != 0 {
		t.Errorf("alice PN = %d, want 0", alice.st.previousCounter)
	}
	bob.recv(t, alice.send(t))
	// Bob sent 3 on his chain: PN = 2, not 3 (double-ratchet.md §2 note).
	if bob.st.previousCounter != 2 {
		t.Errorf("bob PN = %d, want 2", bob.st.previousCounter)
	}
}

func TestSessionDuplicate(t *testing.T) {
	alice, bob := newSession(t)
	m := alice.send(t)
	bob.recv(t, m)
	if _, _, err := bob.st.receiveStep(bob.rng, m.ratchetKey, m.counter, nil); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("replay: %v, want ErrDuplicateMessage", err)
	}
	m1, m2 := alice.send(t), alice.send(t)
	bob.recv(t, m2)
	bob.recv(t, m1)
	if _, _, err := bob.st.receiveStep(bob.rng, m1.ratchetKey, m1.counter, nil); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("replayed skipped message: %v", err)
	}
}

// TestSessionReceiveDoesNotMutate: receiveStep works on a copy, so an
// uncommitted or failed step leaves the original usable.
func TestSessionReceiveDoesNotMutate(t *testing.T) {
	alice, bob := newSession(t)
	bob.recv(t, alice.send(t))
	m := alice.send(t)
	orig := bob.st
	if _, _, err := orig.receiveStep(bob.rng, m.ratchetKey, m.counter, nil); err != nil {
		t.Fatal(err)
	}
	// Not committed: the same message is still new to the original.
	bob.st = orig
	bob.recv(t, m)

	// A ratchet step with an unknown key, not committed, changes nothing.
	r := testRand(77)
	forged, _ := curve25519.GeneratePrivateKey(r)
	before := len(bob.st.receivers)
	if _, _, err := bob.st.receiveStep(bob.rng, forged.PublicKey(), 0, nil); err != nil {
		t.Fatal(err) // keys come out (garbage); the MAC check would reject them
	}
	if len(bob.st.receivers) != before {
		t.Error("uncommitted ratchet step changed the receiver chains")
	}
	alice.recv(t, bob.send(t))
}

func TestSessionForwardJumpLimit(t *testing.T) {
	// Advancing Alice's sending chain with chainStep alone is cheap; the
	// expected keys for counter c are those of her chain key after c steps.
	jump := func(c uint32) (sent, *peer) {
		alice, bob := newSession(t)
		ck := alice.st.sendChain.key
		for range c {
			_, ck = chainStep(ck)
		}
		seed, _ := chainStep(ck)
		return sent{alice.st.senderRatchet.PublicKey(), c, deriveMessageKeys(seed, nil)}, bob
	}
	// Bob's new chain starts at 0: counter 25000 is a jump of exactly 25000.
	m, bob := jump(MaxForwardJumps)
	bob.recv(t, m)
	m, bob = jump(MaxForwardJumps + 1)
	if _, _, err := bob.st.receiveStep(bob.rng, m.ratchetKey, m.counter, nil); err == nil {
		t.Error("jump of 25001 accepted")
	}
}

func TestSessionSkippedKeyCap(t *testing.T) {
	alice, bob := newSession(t)
	var ms []sent
	for range MaxMessageKeys + 101 {
		ms = append(ms, alice.send(t))
	}
	bob.recv(t, ms[len(ms)-1]) // stores 2100 skipped seeds, keeps the newest 2000
	if n := len(bob.st.receivers[len(bob.st.receivers)-1].skipped); n != MaxMessageKeys {
		t.Fatalf("%d skipped keys kept, want %d", n, MaxMessageKeys)
	}
	if _, _, err := bob.st.receiveStep(bob.rng, ms[99].ratchetKey, ms[99].counter, nil); !errors.Is(err, ErrDuplicateMessage) {
		t.Errorf("evicted key: %v, want ErrDuplicateMessage", err)
	}
	bob.recv(t, ms[100])
}

func TestSessionReceiverChainLimit(t *testing.T) {
	alice, bob := newSession(t)
	for range 8 {
		bob.recv(t, alice.send(t))
		alice.recv(t, bob.send(t))
	}
	if n := len(alice.st.receivers); n != MaxReceiverChains {
		t.Errorf("alice keeps %d receiver chains, want %d", n, MaxReceiverChains)
	}
}
