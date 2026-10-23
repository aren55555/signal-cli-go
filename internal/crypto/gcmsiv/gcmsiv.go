// Package gcmsiv implements AES-GCM-SIV (RFC 8452), which sealed sender v2
// uses for its shared ciphertext (spec/protocol/sealed-sender.md §6). The
// standard library has AES-GCM but not GCM-SIV.
//
// POLYVAL's field multiplication is a constant-time shift-and-add (the
// authentication key is secret): every iteration does the same work,
// with conditions applied as masks. It favours clarity over speed, which
// is fine for message-sized inputs.
package gcmsiv

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

const (
	nonceSize = 12
	tagSize   = 16
)

// elem is an element of GF(2^128) mod x^128 + x^127 + x^126 + x^121 + 1,
// in RFC 8452's byte order: bit i of the little-endian 128-bit value is
// the coefficient of x^i.
type elem struct{ lo, hi uint64 }

func load(b []byte) elem {
	return elem{binary.LittleEndian.Uint64(b), binary.LittleEndian.Uint64(b[8:])}
}

func (e elem) bytes() []byte {
	out := binary.LittleEndian.AppendUint64(nil, e.lo)
	return binary.LittleEndian.AppendUint64(out, e.hi)
}

// Reduction constants: x^128 = x^127 + x^126 + x^121 + 1, and
// x^-128 = x^127 + x^124 + x^121 + x^114 + 1 (RFC 8452 §3).
var (
	reduceHi = uint64(1)<<63 | 1<<62 | 1<<57
	xInv128  = elem{lo: 1, hi: 1<<63 | 1<<60 | 1<<57 | 1<<50}
)

// mul returns a * b in the field.
func mul(a, b elem) elem {
	var r elem
	v := a
	for i := range 128 {
		w := b.lo
		if i >= 64 {
			w = b.hi
		}
		m := -(w >> (i % 64) & 1)
		r.lo ^= v.lo & m
		r.hi ^= v.hi & m
		// v *= x
		carry := -(v.hi >> 63)
		v.hi = v.hi<<1 | v.lo>>63
		v.lo <<= 1
		v.lo ^= 1 & carry
		v.hi ^= reduceHi & carry
	}
	return r
}

// polyval computes POLYVAL(H, blocks) over data, a multiple of 16 bytes:
// S_j = dot(S_{j-1} + X_j, H), with dot(a, b) = a * b * x^-128.
func polyval(h elem, data []byte) elem {
	hx := mul(h, xInv128)
	var s elem
	for i := 0; i < len(data); i += 16 {
		x := load(data[i:])
		s = mul(elem{s.lo ^ x.lo, s.hi ^ x.hi}, hx)
	}
	return s
}

func pad16(b []byte) []byte {
	if r := len(b) % 16; r != 0 {
		return append(b, make([]byte, 16-r)...)
	}
	return b
}

// polyvalInput returns S_s = POLYVAL(authKey, padded AAD || padded
// plaintext || length block).
func polyvalInput(authKey, aad, plaintext []byte) []byte {
	var in []byte
	in = append(in, pad16(append([]byte(nil), aad...))...)
	in = append(in, pad16(append([]byte(nil), plaintext...))...)
	in = binary.LittleEndian.AppendUint64(in, uint64(len(aad))*8)
	in = binary.LittleEndian.AppendUint64(in, uint64(len(plaintext))*8)
	return polyval(load(authKey), in).bytes()
}

// deriveKeys derives the per-nonce authentication key (16 bytes) and
// encryption key (the key-generating key's size) (RFC 8452 §4).
func deriveKeys(key, nonce []byte) (authKey, encKey []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	n := 4
	if len(key) == 32 {
		n = 6
	}
	var in, out [16]byte
	copy(in[4:], nonce)
	var material []byte
	for i := range n {
		binary.LittleEndian.PutUint32(in[:4], uint32(i))
		block.Encrypt(out[:], in[:])
		material = append(material, out[:8]...)
	}
	return material[:16], material[16:], nil
}

type aead struct {
	key []byte
}

// New returns AES-GCM-SIV with a 16- or 32-byte key-generating key.
func New(key []byte) (cipher.AEAD, error) {
	if len(key) != 16 && len(key) != 32 {
		return nil, errors.New("gcmsiv: key must be 16 or 32 bytes")
	}
	return &aead{key: append([]byte(nil), key...)}, nil
}

func (a *aead) NonceSize() int { return nonceSize }
func (a *aead) Overhead() int  { return tagSize }

// tag computes the tag for plaintext under the derived keys.
func tag(enc cipher.Block, authKey, nonce, plaintext, aad []byte) []byte {
	s := polyvalInput(authKey, aad, plaintext)
	for i := range nonceSize {
		s[i] ^= nonce[i]
	}
	s[15] &= 0x7f
	enc.Encrypt(s, s)
	return s
}

// ctr is AES-CTR with the GCM-SIV counter: the first 32 bits of the block,
// little-endian, wrapping.
func ctr(enc cipher.Block, tag, in []byte) []byte {
	var block, ks [16]byte
	copy(block[:], tag)
	block[15] |= 0x80
	out := make([]byte, len(in))
	for i := 0; i < len(in); i += 16 {
		enc.Encrypt(ks[:], block[:])
		binary.LittleEndian.PutUint32(block[:4], binary.LittleEndian.Uint32(block[:4])+1)
		subtle.XORBytes(out[i:], in[i:], ks[:])
	}
	return out
}

func (a *aead) keys(nonce []byte) ([]byte, cipher.Block) {
	if len(nonce) != nonceSize {
		panic("gcmsiv: incorrect nonce length")
	}
	authKey, encKey, err := deriveKeys(a.key, nonce)
	if err != nil {
		panic(err) // the key length was checked in New
	}
	enc, err := aes.NewCipher(encKey)
	if err != nil {
		panic(err)
	}
	return authKey, enc
}

func (a *aead) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	authKey, enc := a.keys(nonce)
	t := tag(enc, authKey, nonce, plaintext, additionalData)
	dst = append(dst, ctr(enc, t, plaintext)...)
	return append(dst, t...)
}

var errOpen = errors.New("gcmsiv: message authentication failed")

func (a *aead) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(ciphertext) < tagSize {
		return nil, errOpen
	}
	authKey, enc := a.keys(nonce)
	body, t := ciphertext[:len(ciphertext)-tagSize], ciphertext[len(ciphertext)-tagSize:]
	pt := ctr(enc, t, body)
	if subtle.ConstantTimeCompare(tag(enc, authKey, nonce, pt, additionalData), t) != 1 {
		clear(pt)
		return nil, errOpen
	}
	return append(dst, pt...), nil
}
