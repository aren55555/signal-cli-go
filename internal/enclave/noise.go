// Package enclave implements the attested channel to SGX enclaves (CDSI,
// SVR2): the Noise handshake and transport (spec/enclave/noise.md) and
// the attestation checks (spec/enclave/attestation.md).
package enclave

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/chacha20poly1305"
)

// Protocol names (noise.md §1).
const (
	NKhfs = "Noise_NKhfs_25519+Kyber1024_ChaChaPoly_SHA256"
	NK    = "Noise_NK_25519_ChaChaPoly_SHA256"
)

// ErrHandshake is any failure of the second handshake message.
var ErrHandshake = errors.New("enclave: noise handshake failed")

const (
	maxMessage = 65535
	tagLen     = 16
	maxPlain   = maxMessage - tagLen
)

type cipherState struct {
	k   [32]byte
	n   uint64
	has bool
}

func (c *cipherState) nonce() []byte {
	var b [12]byte
	binary.LittleEndian.PutUint64(b[4:], c.n)
	return b[:]
}

func (c *cipherState) seal(ad, p []byte) []byte {
	a, _ := chacha20poly1305.New(c.k[:])
	out := a.Seal(nil, c.nonce(), p, ad)
	c.n++
	return out
}

func (c *cipherState) open(ad, ct []byte) ([]byte, error) {
	a, _ := chacha20poly1305.New(c.k[:])
	p, err := a.Open(nil, c.nonce(), ct, ad)
	if err != nil {
		return nil, err
	}
	c.n++
	return p, nil
}

type symmetric struct {
	ck, h [32]byte
	c     cipherState
}

func newSymmetric(name string) *symmetric {
	s := &symmetric{}
	if len(name) <= 32 {
		copy(s.h[:], name)
	} else {
		s.h = sha256.Sum256([]byte(name))
	}
	s.ck = s.h
	return s
}

func (s *symmetric) mixHash(d []byte) {
	h := sha256.New()
	h.Write(s.h[:])
	h.Write(d)
	h.Sum(s.h[:0])
}

func hmacSHA(key []byte, parts ...[]byte) []byte {
	m := hmac.New(sha256.New, key)
	for _, p := range parts {
		m.Write(p)
	}
	return m.Sum(nil)
}

func hkdf2(ck, ikm []byte) (o1, o2 []byte) {
	t := hmacSHA(ck, ikm)
	o1 = hmacSHA(t, []byte{1})
	o2 = hmacSHA(t, o1, []byte{2})
	return o1, o2
}

func (s *symmetric) mixKey(ikm []byte) {
	o1, o2 := hkdf2(s.ck[:], ikm)
	copy(s.ck[:], o1)
	s.c = cipherState{has: true}
	copy(s.c.k[:], o2)
}

func (s *symmetric) encryptAndHash(p []byte) []byte {
	out := p
	if s.c.has {
		out = s.c.seal(s.h[:], p)
	}
	s.mixHash(out)
	return out
}

func (s *symmetric) decryptAndHash(c []byte) ([]byte, error) {
	if !s.c.has {
		s.mixHash(c)
		return c, nil
	}
	p, err := s.c.open(s.h[:], c)
	if err != nil {
		return nil, err
	}
	s.mixHash(c)
	return p, nil
}

// Initiator is the client side of an NK or NKhfs handshake in progress.
type Initiator struct {
	name string
	s    *symmetric
	e    *ecdh.PrivateKey
	kem  *mlkem.DecapsulationKey1024
}

// Options fix the ephemeral values (tests); zero values draw randomness.
type Options struct {
	Ephemeral []byte // 32-byte X25519 private key
	KEMSeed   []byte // 64-byte ML-KEM seed d‖z
	Rand      io.Reader
}

// NewInitiator starts a handshake with the responder static key rs (the
// attested claim) and returns message 1 (noise.md §3).
func NewInitiator(name string, rs []byte, o Options) (*Initiator, []byte, error) {
	r := o.Rand
	if r == nil {
		r = rand.Reader
	}
	x := ecdh.X25519()
	remote, err := x.NewPublicKey(rs)
	if err != nil {
		return nil, nil, err
	}
	in := &Initiator{name: name, s: newSymmetric(name)}
	in.s.mixHash(nil) // empty prologue
	in.s.mixHash(rs)  // <- s
	if o.Ephemeral != nil {
		in.e, err = x.NewPrivateKey(o.Ephemeral)
	} else {
		in.e, err = x.GenerateKey(r)
	}
	if err != nil {
		return nil, nil, err
	}
	msg := append([]byte{}, in.e.PublicKey().Bytes()...)
	in.s.mixHash(in.e.PublicKey().Bytes())
	es, err := in.e.ECDH(remote)
	if err != nil {
		return nil, nil, err
	}
	in.s.mixKey(es)
	if name == NKhfs {
		if o.KEMSeed != nil {
			in.kem, err = mlkem.NewDecapsulationKey1024(o.KEMSeed)
		} else {
			in.kem, err = mlkem.GenerateKey1024()
		}
		if err != nil {
			return nil, nil, err
		}
		msg = append(msg, in.s.encryptAndHash(in.kem.EncapsulationKey().Bytes())...)
	}
	msg = append(msg, in.s.encryptAndHash(nil)...)
	return in, msg, nil
}

// Finish reads message 2 and returns the transport (noise.md §3).
func (in *Initiator) Finish(msg2 []byte) (*Transport, error) {
	want := 32 + tagLen
	if in.name == NKhfs {
		want += mlkem.CiphertextSize1024 + tagLen
	}
	if len(msg2) != want {
		return nil, ErrHandshake
	}
	re, err := ecdh.X25519().NewPublicKey(msg2[:32])
	if err != nil {
		return nil, ErrHandshake
	}
	in.s.mixHash(msg2[:32])
	ee, err := in.e.ECDH(re)
	if err != nil {
		return nil, ErrHandshake
	}
	in.s.mixKey(ee)
	rest := msg2[32:]
	if in.name == NKhfs {
		n := mlkem.CiphertextSize1024 + tagLen
		ct, err := in.s.decryptAndHash(rest[:n])
		if err != nil {
			return nil, ErrHandshake
		}
		ss, err := in.kem.Decapsulate(ct)
		if err != nil {
			return nil, ErrHandshake
		}
		in.s.mixKey(ss)
		rest = rest[n:]
	}
	if p, err := in.s.decryptAndHash(rest); err != nil || len(p) != 0 {
		return nil, ErrHandshake
	}
	k1, k2 := hkdf2(in.s.ck[:], nil)
	t := &Transport{hash: in.s.h}
	copy(t.send.k[:], k1)
	copy(t.recv.k[:], k2)
	t.send.has, t.recv.has = true, true
	return t, nil
}

// Transport carries application messages after the handshake.
type Transport struct {
	send, recv cipherState
	hash       [32]byte
}

// HandshakeHash is the final h.
func (t *Transport) HandshakeHash() [32]byte { return t.hash }

// Seal encrypts one application message as one frame of 65519-byte
// chunks, each its own Noise message (noise.md §5).
func (t *Transport) Seal(plain []byte) []byte {
	var out []byte
	for len(plain) > 0 {
		n := min(maxPlain, len(plain))
		out = append(out, t.send.seal(nil, plain[:n])...)
		plain = plain[n:]
	}
	return out
}

// Open decrypts one frame of 65535-byte Noise messages.
func (t *Transport) Open(frame []byte) ([]byte, error) {
	var out []byte
	for len(frame) > 0 {
		n := min(maxMessage, len(frame))
		if n < tagLen {
			return nil, errors.New("enclave: short noise message")
		}
		p, err := t.recv.open(nil, frame[:n])
		if err != nil {
			return nil, err
		}
		out = append(out, p...)
		frame = frame[n:]
	}
	return out, nil
}
