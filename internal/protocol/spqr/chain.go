// SPDX-License-Identifier: GPL-3.0-or-later

package spqr

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// KDF labels (protocol.md §1). Note the two spaces in the chain-start
// label.
const (
	labelChainStart    = "Signal PQ Ratchet V1 Chain  Start"
	labelChainAddEpoch = "Signal PQ Ratchet V1 Chain Add Epoch"
	labelChainNext     = "Signal PQ Ratchet V1 Chain Next"
	labelAuthUpdate    = "Signal_PQCKA_V1_MLKEM768:Authenticator Update"
	labelHeaderMAC     = "Signal_PQCKA_V1_MLKEM768:ekheader"
	labelCiphertextMAC = "Signal_PQCKA_V1_MLKEM768:ciphertext"
	labelSCKAKey       = "Signal_PQCKA_V1_MLKEM768:SCKA Key"
)

// Errors (protocol.md §9). All abort the operation; the caller keeps no
// state change.
var (
	ErrSendKeyEpochDecreased = errors.New("spqr: send key epoch decreased")
	ErrEpochOutOfRange       = errors.New("spqr: epoch out of range")
	ErrKeyJump               = errors.New("spqr: key index jumps too far")
	ErrKeyTrimmed            = errors.New("spqr: key index too old")
	ErrKeyAlreadyRequested   = errors.New("spqr: key already requested")
	ErrMacVerifyFailed       = errors.New("spqr: MAC verification failed")
	ErrErroneousData         = errors.New("spqr: erroneous data received")
	ErrMsgDecode             = errMsgDecode
	ErrVersionMismatch       = errors.New("spqr: version mismatch")
	ErrMinimumVersion        = errors.New("spqr: message below the minimum version")
	ErrChainNotAvailable     = errors.New("spqr: chain not available")
)

// Library defaults for a chain parameter of 0 (protocol.md §3.5).
const (
	defaultMaxJump = 25000
	defaultMaxOOO  = 2000
)

var zeroSalt = make([]byte, 32)

func hkdfKey(salt, ikm []byte, info string, n int) []byte {
	out, err := hkdf.Key(sha256.New, ikm, salt, info, n)
	if err != nil {
		panic(err) // lengths here are always within HKDF's limit
	}
	return out
}

func be64(n uint64) string { return string(binary.BigEndian.AppendUint64(nil, n)) }

type skipped struct {
	index uint32
	key   []byte
}

// half is one direction of a link: a counter, the next chain value, and
// (for receiving) skipped keys.
type half struct {
	counter uint32
	next    []byte // nil once erased
	store   []skipped
}

// step advances the half and returns (index, key) for the new counter.
func (h *half) step() (uint32, []byte) {
	h.counter++
	info := string(binary.BigEndian.AppendUint32(nil, h.counter)) + labelChainNext
	out := hkdfKey(zeroSalt, h.next, info, 64)
	h.next = out[:32]
	return h.counter, out[32:]
}

type link struct {
	epoch      uint64
	send, recv half
}

// chain is SPQR's symmetric chain (protocol.md §3): one link per retained
// epoch, each yielding per-message keys in both directions.
type chain struct {
	a2b             bool
	currentEpoch    uint64
	sendEpoch       uint64
	nextRoot        []byte
	maxJump, maxOOO uint32
	links           []*link // oldest first
}

func newChain(authKey []byte, a2b bool, maxJump, maxOOO uint32) *chain {
	c := newChainParams(a2b, maxJump, maxOOO)
	c.appendLink(0, hkdfKey(zeroSalt, authKey, labelChainStart, 96))
	return c
}

// newChainParams returns an empty chain with the parameters (0 meaning
// the library default).
func newChainParams(a2b bool, maxJump, maxOOO uint32) *chain {
	if maxJump == 0 {
		maxJump = defaultMaxJump
	}
	if maxOOO == 0 {
		maxOOO = defaultMaxOOO
	}
	return &chain{a2b: a2b, maxJump: maxJump, maxOOO: maxOOO}
}

// appendLink splits 96 bytes into next_root and the A2B and B2A seeds.
func (c *chain) appendLink(epoch uint64, out []byte) {
	c.nextRoot = out[:32]
	a2b, b2a := out[32:64], out[64:96]
	l := &link{epoch: epoch}
	if c.a2b {
		l.send.next, l.recv.next = a2b, b2a
	} else {
		l.send.next, l.recv.next = b2a, a2b
	}
	c.links = append(c.links, l)
}

func (c *chain) addEpoch(n uint64, secret []byte) error {
	if n != c.currentEpoch+1 {
		return fmt.Errorf("spqr: epoch secret for %d added to a chain at epoch %d", n, c.currentEpoch)
	}
	c.appendLink(n, hkdfKey(c.nextRoot, secret, labelChainAddEpoch, 96))
	c.currentEpoch = n
	return nil
}

func (c *chain) find(epoch uint64) (int, error) {
	if epoch > c.currentEpoch {
		return 0, fmt.Errorf("%w: %d", ErrEpochOutOfRange, epoch)
	}
	for i, l := range c.links {
		if l.epoch == epoch {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: %d was dropped", ErrEpochOutOfRange, epoch)
}

// sendKey returns the next sending key for chain epoch e (§3.4).
func (c *chain) sendKey(e uint64) (uint32, []byte, error) {
	if e < c.sendEpoch {
		return 0, nil, ErrSendKeyEpochDecreased
	}
	i, err := c.find(e)
	if err != nil {
		return 0, nil, err
	}
	if e != c.sendEpoch {
		c.sendEpoch = e
		// Keep at most one link older than e, and erase the send halves of
		// older links. Links are dropped only here.
		if i > 1 {
			c.links = c.links[i-1:]
			i = 1
		}
		for _, l := range c.links[:i] {
			l.send.next = nil
		}
	}
	idx, key := c.links[i].send.step()
	return idx, key, nil
}

// recvKey returns the receiving key for (epoch e, index idx) (§3.5).
func (c *chain) recvKey(e uint64, idx uint32) ([]byte, error) {
	li, err := c.find(e)
	if err != nil {
		return nil, err
	}
	h := &c.links[li].recv
	cur := h.counter
	switch {
	case idx > cur:
		if idx-cur > c.maxJump {
			return nil, fmt.Errorf("%w: %d past %d", ErrKeyJump, idx, cur)
		}
		if uint64(idx) > uint64(cur)+uint64(c.maxOOO) {
			h.store = nil
		}
		for h.counter+1 < idx {
			i, k := h.step()
			if uint64(i)+uint64(c.maxOOO) >= uint64(idx) {
				h.store = append(h.store, skipped{i, k})
			}
		}
		c.gc(h)
		_, k := h.step()
		return k, nil
	case idx < cur:
		if uint64(idx)+uint64(c.maxOOO) < uint64(cur) {
			return nil, fmt.Errorf("%w: %d", ErrKeyTrimmed, idx)
		}
		for k, s := range h.store {
			if s.index == idx {
				h.store = append(h.store[:k], h.store[k+1:]...)
				return s.key, nil
			}
		}
		return nil, fmt.Errorf("%w: %d", ErrKeyAlreadyRequested, idx)
	default:
		return nil, fmt.Errorf("%w: %d", ErrKeyAlreadyRequested, idx)
	}
}

// gc trims the store once it reaches floor(max_ooo * 11/10) + 1 entries,
// dropping indices below counter - max_ooo.
func (c *chain) gc(h *half) {
	if uint64(len(h.store)) < uint64(c.maxOOO)*11/10+1 {
		return
	}
	kept := h.store[:0]
	for _, s := range h.store {
		if int64(s.index) >= int64(h.counter)-int64(c.maxOOO) {
			kept = append(kept, s)
		}
	}
	h.store = kept
}

func (c *chain) clone() *chain {
	n := *c
	n.links = make([]*link, len(c.links))
	for i, l := range c.links {
		cl := *l
		cl.recv.store = append([]skipped(nil), l.recv.store...)
		cl.send.store = append([]skipped(nil), l.send.store...)
		n.links[i] = &cl
	}
	return &n
}

// authenticator MACs the header and ciphertext of each epoch (§4).
type authenticator struct {
	rootKey, macKey []byte
}

func newAuthenticator(authKey []byte) *authenticator {
	a := &authenticator{rootKey: make([]byte, 32)}
	a.update(1, authKey)
	return a
}

func (a *authenticator) clone() *authenticator {
	return &authenticator{rootKey: append([]byte(nil), a.rootKey...), macKey: append([]byte(nil), a.macKey...)}
}

func (a *authenticator) update(epoch uint64, k []byte) {
	ikm := append(append([]byte(nil), a.rootKey...), k...)
	out := hkdfKey(zeroSalt, ikm, labelAuthUpdate+be64(epoch), 64)
	a.rootKey, a.macKey = out[:32], out[32:]
}

func (a *authenticator) mac(label string, epoch uint64, data []byte) []byte {
	m := hmac.New(sha256.New, a.macKey)
	m.Write([]byte(label))
	m.Write([]byte(be64(epoch)))
	m.Write(data)
	return m.Sum(nil)
}

func (a *authenticator) headerMAC(epoch uint64, hdr []byte) []byte {
	return a.mac(labelHeaderMAC, epoch, hdr)
}

func (a *authenticator) verifyHeaderMAC(epoch uint64, hdr, mac []byte) bool {
	return hmac.Equal(a.headerMAC(epoch, hdr), mac)
}

func (a *authenticator) ciphertextMAC(epoch uint64, ct []byte) []byte {
	return a.mac(labelCiphertextMAC, epoch, ct)
}

func (a *authenticator) verifyCiphertextMAC(epoch uint64, ct, mac []byte) bool {
	return hmac.Equal(a.ciphertextMAC(epoch, ct), mac)
}

// sckaKey is the epoch secret S_e derived from the ML-KEM shared secret.
func sckaKey(k []byte, epoch uint64) []byte {
	return hkdfKey(zeroSalt, k, labelSCKAKey+be64(epoch), 32)
}
