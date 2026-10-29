// SPDX-License-Identifier: GPL-3.0-or-later

package spqr

import (
	"bytes"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"
)

// TestLabels checks every label against the hex table in protocol.md §1.
func TestLabels(t *testing.T) {
	for _, c := range []struct{ got, hex string }{
		{labelChainStart, "5369676e616c205051205261746368657420563120436861696e20205374617274"},
		{labelChainAddEpoch, "5369676e616c205051205261746368657420563120436861696e204164642045706f6368"},
		{labelChainNext, "5369676e616c205051205261746368657420563120436861696e204e657874"},
		{labelAuthUpdate, "5369676e616c5f5051434b415f56315f4d4c4b454d3736383a41757468656e74696361746f7220557064617465"},
		{labelHeaderMAC, "5369676e616c5f5051434b415f56315f4d4c4b454d3736383a656b686561646572"},
		{labelCiphertextMAC, "5369676e616c5f5051434b415f56315f4d4c4b454d3736383a63697068657274657874"},
		{labelSCKAKey, "5369676e616c5f5051434b415f56315f4d4c4b454d3736383a53434b41204b6579"},
	} {
		if hex.EncodeToString([]byte(c.got)) != c.hex {
			t.Errorf("label %q differs from the spec", c.got)
		}
	}
}

var authKey = bytes.Repeat([]byte{0xa5}, 32)

func pair(t *testing.T) (a, b *chain) {
	t.Helper()
	return newChain(authKey, true, 0, 0), newChain(authKey, false, 0, 0)
}

// TestChainStartAndNext recomputes epoch 0 and the first keys from the
// definitions in protocol.md §1 and §3.
func TestChainStartAndNext(t *testing.T) {
	a, b := pair(t)
	start, _ := hkdf.Key(sha256.New, authKey, make([]byte, 32), labelChainStart, 96)
	if !bytes.Equal(a.nextRoot, start[:32]) {
		t.Error("next_root differs")
	}
	info := binary.BigEndian.AppendUint32(nil, 1)
	next1, _ := hkdf.Key(sha256.New, start[32:64], make([]byte, 32), string(info)+labelChainNext, 64)

	idx, key, err := a.sendKey(0)
	if err != nil || idx != 1 || !bytes.Equal(key, next1[32:]) {
		t.Fatalf("A2B first send key: %d %x %v", idx, key, err)
	}
	// B2A receives A2B's keys with its receive half.
	got, err := b.recvKey(0, 1)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("B2A receive key 1: %v", err)
	}
	// And the other direction uses bytes 64..95.
	_, kb, _ := b.sendKey(0)
	if ka, err := a.recvKey(0, 1); err != nil || !bytes.Equal(ka, kb) {
		t.Fatalf("A2B receive of B2A key 1: %v", err)
	}
	if bytes.Equal(kb, key) {
		t.Error("both directions produced the same key")
	}
}

func TestChainEpochs(t *testing.T) {
	a, b := pair(t)
	secret := bytes.Repeat([]byte{0x77}, 32)
	if err := a.addEpoch(2, secret); err == nil {
		t.Error("adding epoch 2 to a chain at epoch 0 was accepted")
	}
	for _, c := range []*chain{a, b} {
		if err := c.addEpoch(1, secret); err != nil {
			t.Fatal(err)
		}
	}
	idx, k, err := a.sendKey(1)
	if err != nil || idx != 1 {
		t.Fatal(idx, err)
	}
	if got, err := b.recvKey(1, 1); err != nil || !bytes.Equal(got, k) {
		t.Fatalf("epoch-1 keys differ: %v", err)
	}
	if _, _, err := a.sendKey(0); !errors.Is(err, ErrSendKeyEpochDecreased) {
		t.Errorf("send epoch decrease: %v", err)
	}
	if _, _, err := a.sendKey(2); !errors.Is(err, ErrEpochOutOfRange) {
		t.Errorf("send for a future epoch: %v", err)
	}
	if _, err := b.recvKey(5, 1); !errors.Is(err, ErrEpochOutOfRange) {
		t.Errorf("receive for a future epoch: %v", err)
	}
}

func TestChainReceiveOrdering(t *testing.T) {
	a, b := pair(t)
	keys := map[uint32][]byte{}
	for range 10 {
		i, k, _ := a.sendKey(0)
		keys[i] = k
	}
	for _, i := range []uint32{5, 2, 10, 1, 3} {
		got, err := b.recvKey(0, i)
		if err != nil || !bytes.Equal(got, keys[i]) {
			t.Fatalf("index %d: %v", i, err)
		}
	}
	if _, err := b.recvKey(0, 5); !errors.Is(err, ErrKeyAlreadyRequested) {
		t.Errorf("reused index: %v", err)
	}
	if _, err := b.recvKey(0, 10); !errors.Is(err, ErrKeyAlreadyRequested) {
		t.Errorf("index equal to the counter: %v", err)
	}
	_, fresh := pair(t)
	if _, err := fresh.recvKey(0, 0); !errors.Is(err, ErrKeyAlreadyRequested) {
		t.Errorf("index 0: %v", err)
	}
}

func TestChainLimits(t *testing.T) {
	_, b := pair(t)
	b.maxJump, b.maxOOO = 100, 10
	if _, err := b.recvKey(0, 101); !errors.Is(err, ErrKeyJump) {
		t.Errorf("jump of 101 with max 100: %v", err)
	}
	if _, err := b.recvKey(0, 100); err != nil {
		t.Fatalf("jump of exactly 100: %v", err)
	}
	// Only indices within max_ooo of 100 were kept.
	if _, err := b.recvKey(0, 95); err != nil {
		t.Errorf("index 95: %v", err)
	}
	if _, err := b.recvKey(0, 89); !errors.Is(err, ErrKeyTrimmed) {
		t.Errorf("index 89 (older than max_ooo): %v", err)
	}
	if _, err := b.recvKey(0, 90); err != nil {
		t.Errorf("index 90 (90 + 10 >= 100, so stored): %v", err)
	}
}

func TestChainDefaults(t *testing.T) {
	c := newChain(authKey, true, 0, 0)
	if c.maxJump != 25000 || c.maxOOO != 2000 {
		t.Errorf("defaults %d, %d", c.maxJump, c.maxOOO)
	}
}

func TestAuthenticator(t *testing.T) {
	a := newAuthenticator(authKey)
	// Initialization: Update(1, auth key) from a zero root key.
	out, _ := hkdf.Key(sha256.New, append(make([]byte, 32), authKey...), make([]byte, 32),
		labelAuthUpdate+string(binary.BigEndian.AppendUint64(nil, 1)), 64)
	if !bytes.Equal(a.rootKey, out[:32]) || !bytes.Equal(a.macKey, out[32:]) {
		t.Fatal("initialization differs from protocol.md §4")
	}
	hdr := bytes.Repeat([]byte{1}, 64)
	m := a.headerMAC(1, hdr)
	if len(m) != 32 || !a.verifyHeaderMAC(1, hdr, m) || a.verifyHeaderMAC(2, hdr, m) {
		t.Error("header MAC")
	}
	ct := bytes.Repeat([]byte{2}, 1088)
	cm := a.ciphertextMAC(1, ct)
	b := a.clone()
	b.update(1, bytes.Repeat([]byte{3}, 32))
	if b.verifyCiphertextMAC(1, ct, cm) {
		t.Error("ciphertext MAC survived an update")
	}
	if !a.verifyCiphertextMAC(1, ct, cm) {
		t.Error("clone shares state with the original")
	}
}
