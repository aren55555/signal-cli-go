package protocol

import (
	"errors"
	"fmt"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// Limits from spec/protocol/double-ratchet.md §6.
const (
	MaxForwardJumps   = 25000 // largest counter jump in one message (unlimited for a self-session)
	MaxMessageKeys    = 2000  // skipped-key entries kept per receiving chain
	MaxReceiverChains = 5     // receiving chains kept per session state
)

// ErrDuplicateMessage reports a counter that was already used. libsignal
// reports it before any MAC check and stops trying other session states.
var ErrDuplicateMessage = errors.New("protocol: duplicated message")

type chain struct {
	key   []byte
	index uint32 // index of the next key in the chain
}

type skippedSeed struct {
	index uint32
	seed  []byte
}

// receiverChain is a receiving chain keyed by the remote ratchet key, with
// its skipped message-key seeds, oldest first (libsignal keeps the list
// newest first; indices are unique per chain, so only the eviction end
// matters). Seeds, not expanded keys,
// are stored because the SPQR salt arrives with the message.
type receiverChain struct {
	ratchetKey *curve25519.PublicKey
	chain      chain
	skipped    []skippedSeed
}

// SessionState is one Double Ratchet session state as libsignal keeps it
// (double-ratchet.md, pqxdh.md §4). Unlike the generic algorithm it keeps
// several receiving chains and ignores PN: late messages from an older
// chain are handled by keeping that chain.
type SessionState struct {
	version        uint8
	localIdentity  *curve25519.PublicKey
	remoteIdentity *curve25519.PublicKey
	rootKey        []byte
	senderRatchet  *curve25519.PrivateKey
	sendChain      chain
	receivers      []*receiverChain // oldest first
	// previousCounter is libsignal's PN: the last index used on the previous
	// sending chain (not the count), floored at 0.
	previousCounter uint32
	aliceBaseKey    *curve25519.PublicKey
	selfSession     bool

	localRegistrationID  uint32
	remoteRegistrationID uint32
	// Pending prekey info: set while the session is unacknowledged, so
	// every outgoing message is a PreKeySignalMessage (pqxdh.md §4).
	pending      *pendingPreKey
	pendingKyber *pendingKyberPreKey
}

// NewAliceSession builds the initiator's state after PQXDH (pqxdh.md §4):
// a receiving chain on Bob's signed prekey with the handshake chain key,
// then one root step to a fresh sending ratchet key.
func NewAliceSession(rand io.Reader, init *PQXDHInitiation, localIdentity, remoteIdentity, bobSignedPreKey *curve25519.PublicKey) (*SessionState, error) {
	s := &SessionState{
		version:        4,
		localIdentity:  localIdentity,
		remoteIdentity: remoteIdentity,
		aliceBaseKey:   init.BaseKey,
		receivers: []*receiverChain{{
			ratchetKey: bobSignedPreKey,
			chain:      chain{key: init.Secrets.ChainKey},
		}},
	}
	ratchet, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	dh, err := ratchet.Agree(bobSignedPreKey)
	if err != nil {
		return nil, err
	}
	var ck []byte
	s.rootKey, ck = rootStep(init.Secrets.RootKey, dh)
	s.senderRatchet = ratchet
	s.sendChain = chain{key: ck}
	return s, nil
}

// NewBobSession builds the responder's state after PQXDH: the handshake
// root key, and a sending chain on the signed prekey with the handshake
// chain key. No receiving chain until Alice's message arrives.
func NewBobSession(secrets *PQXDHSecrets, signedPreKey *curve25519.PrivateKey, localIdentity, remoteIdentity, aliceBaseKey *curve25519.PublicKey) *SessionState {
	return &SessionState{
		version:        4,
		localIdentity:  localIdentity,
		remoteIdentity: remoteIdentity,
		aliceBaseKey:   aliceBaseKey,
		rootKey:        secrets.RootKey,
		senderRatchet:  signedPreKey,
		sendChain:      chain{key: secrets.ChainKey},
	}
}

// clone deep-copies the parts of the state that steps modify.
func (s *SessionState) clone() *SessionState {
	c := *s
	c.receivers = make([]*receiverChain, len(s.receivers))
	for i, r := range s.receivers {
		rc := *r
		rc.skipped = append([]skippedSeed(nil), r.skipped...)
		c.receivers[i] = &rc
	}
	return &c
}

type sendResult struct {
	ratchetKey      *curve25519.PublicKey
	counter         uint32
	previousCounter uint32
	keys            messageKeys
}

// sendStep derives the keys for the next outgoing message and returns the
// advanced state; the caller stores it only if encryption succeeds.
func (s *SessionState) sendStep(spqrKey []byte) (*SessionState, sendResult, error) {
	if s.sendChain.key == nil {
		return nil, sendResult{}, errors.New("protocol: session has no sending chain")
	}
	seed, next := chainStep(s.sendChain.key)
	res := sendResult{
		ratchetKey:      s.senderRatchet.PublicKey(),
		counter:         s.sendChain.index,
		previousCounter: s.previousCounter,
		keys:            deriveMessageKeys(seed, spqrKey),
	}
	t := s.clone()
	t.sendChain = chain{key: next, index: s.sendChain.index + 1}
	return t, res, nil
}

// receiveStep derives the keys for an incoming message with the given
// ratchet key and counter. It returns an updated copy; s is never
// modified. rand supplies the new sending ratchet key if a DH ratchet step
// is needed.
func (s *SessionState) receiveStep(rand io.Reader, ratchetKey *curve25519.PublicKey, counter uint32, spqrKey []byte) (*SessionState, messageKeys, error) {
	t := s.clone()
	rc := t.findReceiver(ratchetKey)
	if rc == nil {
		var err error
		if rc, err = t.dhRatchet(rand, ratchetKey); err != nil {
			return nil, messageKeys{}, err
		}
	}
	seed, err := t.seedFor(rc, counter)
	if err != nil {
		return nil, messageKeys{}, err
	}
	return t, deriveMessageKeys(seed, spqrKey), nil
}

func (s *SessionState) findReceiver(key *curve25519.PublicKey) *receiverChain {
	for _, rc := range s.receivers {
		if string(rc.ratchetKey.U()) == string(key.U()) {
			return rc
		}
	}
	return nil
}

// dhRatchet is the DH ratchet step of double-ratchet.md §2.
func (s *SessionState) dhRatchet(rand io.Reader, remote *curve25519.PublicKey) (*receiverChain, error) {
	dh, err := s.senderRatchet.Agree(remote)
	if err != nil {
		return nil, err
	}
	root1, ckr := rootStep(s.rootKey, dh)
	rc := &receiverChain{ratchetKey: remote, chain: chain{key: ckr}}
	s.receivers = append(s.receivers, rc)
	if len(s.receivers) > MaxReceiverChains {
		s.receivers = s.receivers[1:]
	}

	next, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	if dh, err = next.Agree(remote); err != nil {
		return nil, err
	}
	var cks []byte
	s.rootKey, cks = rootStep(root1, dh)
	s.previousCounter = 0
	if s.sendChain.index > 0 {
		s.previousCounter = s.sendChain.index - 1
	}
	s.senderRatchet = next
	s.sendChain = chain{key: cks}
	return rc, nil
}

// seedFor returns the message-key seed for counter on chain rc, using a
// stored skipped seed or advancing the chain and storing what it skips.
func (s *SessionState) seedFor(rc *receiverChain, counter uint32) ([]byte, error) {
	i := rc.chain.index
	if counter < i {
		for k := len(rc.skipped) - 1; k >= 0; k-- {
			if sk := rc.skipped[k]; sk.index == counter {
				rc.skipped = append(rc.skipped[:k], rc.skipped[k+1:]...)
				return sk.seed, nil
			}
		}
		return nil, fmt.Errorf("%w: counter %d, chain at %d", ErrDuplicateMessage, counter, i)
	}
	if !s.selfSession && counter-i > MaxForwardJumps {
		return nil, fmt.Errorf("protocol: message from too far into the future (counter %d, chain at %d)", counter, i)
	}
	key := rc.chain.key
	for idx := i; idx < counter; idx++ {
		seed, next := chainStep(key)
		rc.skipped = append(rc.skipped, skippedSeed{idx, seed})
		if len(rc.skipped) > MaxMessageKeys {
			rc.skipped = rc.skipped[1:] // drop the oldest
		}
		key = next
	}
	seed, next := chainStep(key)
	rc.chain = chain{key: next, index: counter + 1}
	return seed, nil
}
