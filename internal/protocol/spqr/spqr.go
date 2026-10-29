// SPDX-License-Identifier: GPL-3.0-or-later

package spqr

import (
	"crypto/sha3"
	"errors"
	"fmt"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/kyber"
)

// Stream lengths (protocol.md §5).
const (
	hdrStreamLen = kyber.HeaderSizeMLKEM768 + 32 // hdr || header MAC
	ekStreamLen  = kyber.EKPayloadSizeMLKEM768
	ct1StreamLen = kyber.Ct1SizeMLKEM768
	ct2StreamLen = kyber.Ct2SizeMLKEM768 + 32 // ct2 || ciphertext MAC
)

// stateKind is one of the 11 SCKA states (protocol.md §7).
type stateKind int

const (
	// send_ek role
	keysUnsampled stateKind = iota
	keysSampled
	headerSent
	ct1Received
	ekSentCt1Received
	// send_ct role
	noHeaderReceived
	headerReceived
	ct1Sampled
	ekReceivedCt1Sampled
	ct1Acknowledged
	ct2Sampled
)

func (k stateKind) String() string {
	return [...]string{"KeysUnsampled", "KeysSampled", "HeaderSent", "Ct1Received", "EkSentCt1Received",
		"NoHeaderReceived", "HeaderReceived", "Ct1Sampled", "EkReceivedCt1Sampled", "Ct1Acknowledged", "Ct2Sampled"}[k]
}

// streamEnc is an encoder with its emission counter; the chunk index is
// the counter's low 16 bits.
type streamEnc struct {
	enc  *encoder
	next uint32
}

func (s *streamEnc) emit() (uint16, []byte) {
	c := uint16(s.next)
	s.next++
	ch := s.enc.chunk(c)
	return c, ch[:]
}

func newStream(data []byte) *streamEnc {
	e, err := newEncoder(data)
	if err != nil {
		panic(err) // SPQR stream lengths are fixed and valid
	}
	return &streamEnc{enc: e}
}

func newDec(n int) *decoder {
	d, err := newDecoder(n)
	if err != nil {
		panic(err)
	}
	return d
}

func (d *decoder) clone() *decoder {
	if d == nil {
		return nil
	}
	c := *d
	for j := range c.pts {
		c.pts[j] = append([]point(nil), d.pts[j]...)
	}
	return &c
}

// scka is the SCKA state. Which fields are meaningful depends on kind.
type scka struct {
	kind  stateKind
	epoch uint64
	auth  *authenticator

	dk        []byte // send_ek: 2400-byte decapsulation key
	ekPayload []byte // send_ek: until the Ek encoder starts
	hdr       []byte // send_ct: received hdr
	ct1       []byte
	ct2       []byte // send_ct: computed once the ek payload is checked
	enc       *kyber.IncrementalEncapsulation

	hdrEnc, ekEnc, ct1Enc, ct2Enc *streamEnc
	hdrDec, ekDec, ct1Dec, ct2Dec *decoder
}

func (s *scka) sendEK() bool { return s.kind <= ekSentCt1Received }

func (s *scka) clone() *scka {
	c := *s
	c.auth = s.auth.clone()
	for _, p := range []**streamEnc{&c.hdrEnc, &c.ekEnc, &c.ct1Enc, &c.ct2Enc} {
		if *p != nil {
			cp := **p
			*p = &cp
		}
	}
	c.hdrDec, c.ekDec, c.ct1Dec, c.ct2Dec = s.hdrDec.clone(), s.ekDec.clone(), s.ct1Dec.clone(), s.ct2Dec.clone()
	return &c
}

func initialSCKA(authKey []byte, a2b bool) *scka {
	s := &scka{epoch: 1, auth: newAuthenticator(authKey)}
	if a2b {
		s.kind = keysUnsampled
	} else {
		s.kind = noHeaderReceived
		s.hdrDec = newDec(hdrStreamLen)
	}
	return s
}

type epochSecret struct {
	epoch  uint64
	secret []byte
}

// outgoing is the SCKA part of a message.
type outgoing struct {
	payload    payloadType
	chunkIndex uint16
	chunk      []byte
}

func chunkMsg(p payloadType, s *streamEnc) outgoing {
	c, ch := s.emit()
	return outgoing{payload: p, chunkIndex: c, chunk: ch}
}

// send runs the SCKA send step on s (already a private copy).
func (s *scka) send(rand io.Reader) (outgoing, *epochSecret, error) {
	switch s.kind {
	case keysUnsampled:
		var seed [kyber.SeedSize]byte
		if _, err := io.ReadFull(rand, seed[:]); err != nil {
			return outgoing{}, nil, err
		}
		ek, dk, err := kyber.GenerateKeyMLKEM768(seed[:])
		if err != nil {
			return outgoing{}, nil, err
		}
		h := sha3.Sum256(ek)
		hdr := append(append([]byte(nil), ek[kyber.EKPayloadSizeMLKEM768:]...), h[:]...)
		s.hdrEnc = newStream(append(hdr, s.auth.headerMAC(s.epoch, hdr)...))
		s.ekPayload, s.dk = ek[:kyber.EKPayloadSizeMLKEM768], dk
		s.kind = keysSampled
		return chunkMsg(payloadHdr, s.hdrEnc), nil, nil
	case keysSampled:
		return chunkMsg(payloadHdr, s.hdrEnc), nil, nil
	case headerSent:
		return chunkMsg(payloadEk, s.ekEnc), nil, nil
	case ct1Received:
		return chunkMsg(payloadEkCt1Ack, s.ekEnc), nil, nil
	case ekSentCt1Received:
		return outgoing{payload: payloadCt1Ack}, nil, nil
	case noHeaderReceived, ct1Acknowledged:
		return outgoing{payload: payloadNone}, nil, nil
	case headerReceived:
		var m [32]byte
		if _, err := io.ReadFull(rand, m[:]); err != nil {
			return outgoing{}, nil, err
		}
		enc, ct1, k, err := kyber.StartEncapsulationMLKEM768(s.hdr, m[:])
		if err != nil {
			return outgoing{}, nil, err
		}
		secret := sckaKey(k, s.epoch)
		s.auth.update(s.epoch, secret)
		s.enc, s.ct1 = enc, ct1
		s.ct1Enc = newStream(ct1)
		s.kind = ct1Sampled
		return chunkMsg(payloadCt1, s.ct1Enc), &epochSecret{s.epoch, secret}, nil
	case ct1Sampled, ekReceivedCt1Sampled:
		return chunkMsg(payloadCt1, s.ct1Enc), nil, nil
	case ct2Sampled:
		return chunkMsg(payloadCt2, s.ct2Enc), nil, nil
	}
	return outgoing{}, nil, fmt.Errorf("spqr: bad state %d", s.kind)
}

// checkEK checks a complete ek payload against the header and computes
// ct2 (finishing the encapsulation; the bytes are the same whether ct2
// is computed now or when the acknowledgement arrives).
func (s *scka) checkEK(payload []byte) error {
	ct2, err := s.enc.Finish(payload)
	if errors.Is(err, kyber.ErrHeaderMismatch) {
		return ErrErroneousData
	} else if err != nil {
		return err
	}
	s.ct2 = ct2
	return nil
}

func (s *scka) startCt2() {
	ct := append(append([]byte(nil), s.ct1...), s.ct2...)
	s.ct2Enc = newStream(append(append([]byte(nil), s.ct2...), s.auth.ciphertextMAC(s.epoch, ct)...))
	s.kind = ct2Sampled
}

// receive runs the SCKA receive step on s (already a private copy).
func (s *scka) receive(m *message) (*epochSecret, error) {
	if s.kind == ct2Sampled {
		switch {
		case m.epoch == s.epoch+1:
			*s = scka{kind: keysUnsampled, epoch: s.epoch + 1, auth: s.auth}
		case m.epoch > s.epoch+1:
			return nil, fmt.Errorf("%w: message epoch %d at %d", ErrEpochOutOfRange, m.epoch, s.epoch)
		}
		return nil, nil
	}
	switch {
	case m.epoch > s.epoch:
		return nil, fmt.Errorf("%w: message epoch %d at %d", ErrEpochOutOfRange, m.epoch, s.epoch)
	case m.epoch < s.epoch:
		return nil, nil
	}

	switch s.kind {
	case keysSampled:
		if m.payload == payloadCt1 {
			s.ct1Dec = newDec(ct1StreamLen)
			s.ct1Dec.add(m.chunkIndex, m.chunk)
			s.ekEnc = newStream(s.ekPayload)
			s.ekPayload, s.hdrEnc = nil, nil
			s.kind = headerSent
		}
	case headerSent:
		if m.payload == payloadCt1 {
			s.ct1Dec.add(m.chunkIndex, m.chunk)
			if s.ct1Dec.complete() {
				s.ct1, s.ct1Dec = s.ct1Dec.bytes(), nil
				s.kind = ct1Received
			}
		}
	case ct1Received:
		if m.payload == payloadCt2 {
			s.ct2Dec = newDec(ct2StreamLen)
			s.ct2Dec.add(m.chunkIndex, m.chunk)
			s.ekEnc = nil
			s.kind = ekSentCt1Received
		}
	case ekSentCt1Received:
		if m.payload != payloadCt2 {
			break
		}
		s.ct2Dec.add(m.chunkIndex, m.chunk)
		if !s.ct2Dec.complete() {
			break
		}
		stream := s.ct2Dec.bytes()
		ct := append(append([]byte(nil), s.ct1...), stream[:kyber.Ct2SizeMLKEM768]...)
		k, err := kyber.DecapsulateMLKEM768(s.dk, ct)
		if err != nil {
			return nil, err
		}
		secret := sckaKey(k, s.epoch)
		s.auth.update(s.epoch, secret)
		if !s.auth.verifyCiphertextMAC(s.epoch, ct, stream[kyber.Ct2SizeMLKEM768:]) {
			return nil, ErrMacVerifyFailed
		}
		done := s.epoch
		*s = scka{kind: noHeaderReceived, epoch: s.epoch + 1, auth: s.auth, hdrDec: newDec(hdrStreamLen)}
		return &epochSecret{done, secret}, nil
	case noHeaderReceived:
		if m.payload != payloadHdr {
			break
		}
		s.hdrDec.add(m.chunkIndex, m.chunk)
		if !s.hdrDec.complete() {
			break
		}
		stream := s.hdrDec.bytes()
		hdr, mac := stream[:kyber.HeaderSizeMLKEM768], stream[kyber.HeaderSizeMLKEM768:]
		if !s.auth.verifyHeaderMAC(s.epoch, hdr, mac) {
			return nil, ErrMacVerifyFailed
		}
		s.hdr, s.hdrDec = hdr, nil
		s.ekDec = newDec(ekStreamLen)
		s.kind = headerReceived
	case ct1Sampled:
		ack := m.payload == payloadEkCt1Ack
		if m.payload != payloadEk && !ack {
			break
		}
		s.ekDec.add(m.chunkIndex, m.chunk)
		switch {
		case s.ekDec.complete():
			payload := s.ekDec.bytes()
			if err := s.checkEK(payload); err != nil {
				return nil, err
			}
			s.ekDec = nil
			if ack {
				s.startCt2()
			} else {
				s.ekPayload = payload // stored with the state (state.md §2)
				s.kind = ekReceivedCt1Sampled
			}
		case ack:
			s.kind = ct1Acknowledged
		}
	case ekReceivedCt1Sampled:
		if m.payload == payloadCt1Ack || m.payload == payloadEkCt1Ack {
			s.startCt2()
		}
	case ct1Acknowledged:
		if m.payload != payloadEk && m.payload != payloadEkCt1Ack {
			break
		}
		s.ekDec.add(m.chunkIndex, m.chunk)
		if s.ekDec.complete() {
			if err := s.checkEK(s.ekDec.bytes()); err != nil {
				return nil, err
			}
			s.ekDec = nil
			s.startCt2()
		}
	}
	return nil, nil // keysUnsampled, headerReceived, and unlisted payloads: ignore
}

// Params are the chain parameters; 0 means the library default (25000
// and 2000).
type Params struct {
	MaxJump, MaxOOO uint32
}

// negotiation is kept from initialization until the first successful
// receive (protocol.md §2.1).
type negotiation struct {
	authKey    []byte
	a2b        bool
	minVersion int
	params     Params
}

// State is the top-level SPQR state. States are values: Send and Receive
// return a new state and never modify the receiver, so the caller keeps
// the old one if the enclosing message fails.
type State struct {
	neg   *negotiation
	chain *chain
	scka  *scka // nil: V0
}

// New initializes SPQR as libsignal does: version V1, minimum version V1.
// a2b is true for the session initiator.
func New(authKey []byte, a2b bool, p Params) *State {
	return &State{
		neg:  &negotiation{authKey: append([]byte(nil), authKey...), a2b: a2b, minVersion: 1, params: p},
		scka: initialSCKA(authKey, a2b),
	}
}

func (s *State) clone() *State {
	n := *s
	if s.chain != nil {
		n.chain = s.chain.clone()
	}
	if s.scka != nil {
		n.scka = s.scka.clone()
	}
	return &n
}

// chainFor returns the stored chain or creates it from the negotiation
// record; nil with no error means "no chain" (minimum version V0).
func (s *State) chainFor() (*chain, error) {
	switch {
	case s.chain != nil:
		return s.chain, nil
	case s.neg != nil && s.neg.minVersion >= 1:
		return newChain(s.neg.authKey, s.neg.a2b, s.neg.params.MaxJump, s.neg.params.MaxOOO), nil
	case s.neg != nil:
		return nil, nil
	}
	return nil, ErrChainNotAvailable
}

// Send produces the SPQR message for the next outgoing SignalMessage and
// the 32-byte key to use as its message-key salt (nil if none).
func (s *State) Send(rand io.Reader) (next *State, msg, key []byte, err error) {
	if s.scka == nil {
		return &State{}, nil, nil, nil
	}
	n := s.clone()
	out, secret, err := n.scka.send(rand)
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := n.chainFor()
	if err != nil {
		return nil, nil, nil, err
	}
	m := message{epoch: n.scka.epoch, payload: out.payload, chunkIndex: out.chunkIndex, chunk: out.chunk}
	if c != nil {
		if secret != nil {
			if err := c.addEpoch(secret.epoch, secret.secret); err != nil {
				return nil, nil, nil, err
			}
		}
		idx, k, err := c.sendKey(m.epoch - 1)
		if err != nil {
			return nil, nil, nil, err
		}
		m.keyIndex, key = idx, k
		n.chain = c
	}
	return n, m.encode(), key, nil
}

// Receive processes the SPQR message of an incoming SignalMessage and
// returns the key to use as its message-key salt (nil if none).
func (s *State) Receive(msg []byte) (next *State, key []byte, err error) {
	version := 0
	switch {
	case len(msg) == 0, msg[0] == 0:
	case msg[0] == 1:
		version = 1
	default:
		return s, nil, nil // unknown future version: ignored
	}
	n := s.clone()
	stateVersion := 0
	if n.scka != nil {
		stateVersion = 1
	}
	if version < stateVersion {
		switch {
		case n.neg == nil:
			return nil, nil, ErrVersionMismatch
		case version < n.neg.minVersion:
			return nil, nil, ErrMinimumVersion
		}
		// Downgrade to V0.
		c, err := n.chainFor()
		if err != nil {
			return nil, nil, err
		}
		n.scka, n.chain, n.neg = nil, c, nil
	}
	if n.scka == nil {
		return &State{}, nil, nil
	}
	m, err := parseMessage(msg)
	if err != nil {
		return nil, nil, err
	}
	secret, err := n.scka.receive(m)
	if err != nil {
		return nil, nil, err
	}
	c, err := n.chainFor()
	if err != nil {
		return nil, nil, err
	}
	if c == nil {
		return nil, nil, ErrChainNotAvailable
	}
	if secret != nil {
		if err := c.addEpoch(secret.epoch, secret.secret); err != nil {
			return nil, nil, err
		}
	}
	if !(m.epoch == 1 && m.keyIndex == 0) {
		if key, err = c.recvKey(m.epoch-1, m.keyIndex); err != nil {
			return nil, nil, err
		}
	}
	n.chain, n.neg = c, nil // receiving ends negotiation
	return n, key, nil
}

// Epoch returns the SCKA epoch (0 for a V0 state).
func (s *State) Epoch() uint64 {
	if s.scka == nil {
		return 0
	}
	return s.scka.epoch
}

// WithMaxJump returns a copy of a state with the chain's max_jump replaced,
// for libsignal's self sessions. It applies to the negotiation record and
// any chain already created.
func (s *State) WithMaxJump(maxJump uint32) *State {
	n := s.clone()
	if n.neg != nil {
		neg := *n.neg
		neg.params.MaxJump = maxJump
		n.neg = &neg
	}
	if n.chain != nil {
		n.chain.maxJump = maxJump
	}
	return n
}
