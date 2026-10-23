package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// RatchetConfig holds the choices the Double Ratchet specification leaves
// to the application. The libsignal values come from
// spec/protocol/double-ratchet.md.
type RatchetConfig struct {
	RootInfo       string // HKDF info for KDF_RK
	MessageKeyInfo string // HKDF info for expanding a message key
	MaxSkip        int    // MAX_SKIP: most message keys skipped in one chain
}

// RatchetHeader is HEADER(dh_pair, pn, n).
type RatchetHeader struct {
	DH    *curve25519.PublicKey
	PN, N uint32
}

type skippedKey struct {
	dh [32]byte
	n  uint32
}

// RatchetState holds the state variables of section 3.2.
type RatchetState struct {
	cfg     *RatchetConfig
	dhs     *curve25519.PrivateKey
	dhr     *curve25519.PublicKey // nil until the first message is received
	rk      []byte
	cks     []byte // nil until there is a sending chain
	ckr     []byte // nil until there is a receiving chain
	ns, nr  uint32
	pn      uint32
	skipped map[skippedKey][]byte
}

// kdfRK is KDF_RK per section 7.2: HKDF-SHA256 with rk as salt and the DH
// output as IKM, split into a new root key and a chain key.
func kdfRK(cfg *RatchetConfig, rk, dhOut []byte) (newRK, ck []byte) {
	out, err := hkdf.Key(sha256.New, dhOut, rk, cfg.RootInfo, 64)
	if err != nil {
		panic(err) // 64 bytes is always within HKDF's limit
	}
	return out[:32], out[32:]
}

// kdfCK is KDF_CK per section 7.2: HMAC-SHA256 keyed by ck, over 0x02 for
// the next chain key and 0x01 for the message key.
func kdfCK(ck []byte) (nextCK, mk []byte) {
	h := hmac.New(sha256.New, ck)
	h.Write([]byte{0x01})
	mk = h.Sum(nil)
	h.Reset()
	h.Write([]byte{0x02})
	return h.Sum(nil), mk
}

// InitRatchetAlice is RatchetInitAlice: the initiator starts with the
// shared secret and the responder's ratchet public key.
func InitRatchetAlice(cfg *RatchetConfig, rand io.Reader, sk []byte, bobRatchet *curve25519.PublicKey) (*RatchetState, error) {
	dhs, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	s := &RatchetState{cfg: cfg, dhs: dhs, dhr: bobRatchet, skipped: map[skippedKey][]byte{}}
	out, err := dhs.Agree(bobRatchet)
	if err != nil {
		return nil, err
	}
	s.rk, s.cks = kdfRK(cfg, sk, out)
	return s, nil
}

// InitRatchetBob is RatchetInitBob: the responder starts with the shared
// secret and its own ratchet key pair.
func InitRatchetBob(cfg *RatchetConfig, sk []byte, bobRatchet *curve25519.PrivateKey) *RatchetState {
	return &RatchetState{cfg: cfg, dhs: bobRatchet, rk: append([]byte(nil), sk...), skipped: map[skippedKey][]byte{}}
}

func (s *RatchetState) clone() *RatchetState {
	c := *s
	c.skipped = maps.Clone(s.skipped)
	return &c
}

// Encrypt is RatchetEncrypt. rand is unused by the section 7.2
// construction but kept for message formats that need randomness.
func (s *RatchetState) Encrypt(rand io.Reader, plaintext, ad []byte) (RatchetHeader, []byte, error) {
	if s.cks == nil {
		return RatchetHeader{}, nil, errors.New("protocol: no sending chain")
	}
	var mk []byte
	s.cks, mk = kdfCK(s.cks)
	h := RatchetHeader{DH: s.dhs.PublicKey(), PN: s.pn, N: s.ns}
	s.ns++
	ct, err := s.seal(mk, plaintext, concat(ad, h))
	return h, ct, err
}

// Decrypt is RatchetDecrypt. On any failure the state is left exactly as
// it was (section 3.5); rand supplies the new ratchet key on a DH step.
func (s *RatchetState) Decrypt(rand io.Reader, h RatchetHeader, ciphertext, ad []byte) ([]byte, error) {
	t := s.clone()
	mk, err := t.receiveKey(rand, h)
	if err != nil {
		return nil, err
	}
	pt, err := t.open(mk, ciphertext, concat(ad, h))
	if err != nil {
		return nil, err
	}
	*s = *t
	return pt, nil
}

// receiveKey is RatchetReceiveKey.
func (s *RatchetState) receiveKey(rand io.Reader, h RatchetHeader) ([]byte, error) {
	if h.DH == nil {
		return nil, errors.New("protocol: header without ratchet key")
	}
	k := skippedKey{n: h.N}
	copy(k.dh[:], h.DH.U())
	if mk, ok := s.skipped[k]; ok {
		delete(s.skipped, k)
		return mk, nil
	}
	if s.dhr == nil || string(h.DH.U()) != string(s.dhr.U()) {
		if err := s.skipKeys(h.PN); err != nil {
			return nil, err
		}
		if err := s.dhRatchet(rand, h); err != nil {
			return nil, err
		}
	}
	if err := s.skipKeys(h.N); err != nil {
		return nil, err
	}
	if s.ckr == nil {
		return nil, errors.New("protocol: no receiving chain")
	}
	if h.N < s.nr {
		return nil, fmt.Errorf("protocol: duplicate message (counter %d)", h.N)
	}
	var mk []byte
	s.ckr, mk = kdfCK(s.ckr)
	s.nr++
	return mk, nil
}

// skipKeys is SkipMessageKeys: store the message keys of the current
// receiving chain up to (not including) counter until.
func (s *RatchetState) skipKeys(until uint32) error {
	if uint64(until) > uint64(s.nr)+uint64(s.cfg.MaxSkip) {
		return fmt.Errorf("protocol: message counter %d too far ahead of %d", until, s.nr)
	}
	if s.ckr == nil {
		return nil
	}
	for s.nr < until {
		var mk []byte
		s.ckr, mk = kdfCK(s.ckr)
		k := skippedKey{n: s.nr}
		copy(k.dh[:], s.dhr.U())
		s.skipped[k] = mk
		s.nr++
	}
	return nil
}

// dhRatchet is DHRatchet.
func (s *RatchetState) dhRatchet(rand io.Reader, h RatchetHeader) error {
	s.pn = s.ns
	s.ns, s.nr = 0, 0
	s.dhr = h.DH
	out, err := s.dhs.Agree(s.dhr)
	if err != nil {
		return err
	}
	s.rk, s.ckr = kdfRK(s.cfg, s.rk, out)
	if s.dhs, err = curve25519.GeneratePrivateKey(rand); err != nil {
		return err
	}
	if out, err = s.dhs.Agree(s.dhr); err != nil {
		return err
	}
	s.rk, s.cks = kdfRK(s.cfg, s.rk, out)
	return nil
}

// concat is CONCAT(ad, header): a length-prefixed ad, then the header as
// 33-byte ratchet key, PN and N (big-endian uint32).
func concat(ad []byte, h RatchetHeader) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(ad)))
	out = append(out, ad...)
	out = append(out, h.DH.Bytes()...)
	out = binary.BigEndian.AppendUint32(out, h.PN)
	return binary.BigEndian.AppendUint32(out, h.N)
}

// messageKeys expands a message key per section 7.2: HKDF-SHA256 with a
// zero salt to 80 bytes, split into a 32-byte AES key, a 32-byte HMAC key
// and a 16-byte IV.
func (s *RatchetState) messageKeys(mk []byte) (encKey, macKey, iv []byte) {
	out, err := hkdf.Key(sha256.New, mk, make([]byte, 32), s.cfg.MessageKeyInfo, 80)
	if err != nil {
		panic(err)
	}
	return out[:32], out[32:64], out[64:]
}

// seal is the section 7.2 ENCRYPT: AES-256-CBC with PKCS#7, then
// HMAC-SHA256 over ad || ciphertext appended.
func (s *RatchetState) seal(mk, plaintext, ad []byte) ([]byte, error) {
	encKey, macKey, iv := s.messageKeys(mk)
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	buf := append(append([]byte(nil), plaintext...), make([]byte, pad)...)
	for i := len(plaintext); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)
	m := hmac.New(sha256.New, macKey)
	m.Write(ad)
	m.Write(buf)
	return m.Sum(buf), nil
}

var errDecrypt = errors.New("protocol: message authentication failed")

// open reverses seal, checking the MAC before decrypting.
func (s *RatchetState) open(mk, ciphertext, ad []byte) ([]byte, error) {
	encKey, macKey, iv := s.messageKeys(mk)
	if len(ciphertext) < sha256.Size+aes.BlockSize || (len(ciphertext)-sha256.Size)%aes.BlockSize != 0 {
		return nil, errDecrypt
	}
	body, tag := ciphertext[:len(ciphertext)-sha256.Size], ciphertext[len(ciphertext)-sha256.Size:]
	m := hmac.New(sha256.New, macKey)
	m.Write(ad)
	m.Write(body)
	if !hmac.Equal(m.Sum(nil), tag) {
		return nil, errDecrypt
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	pt := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, body)
	pad := int(pt[len(pt)-1])
	if pad == 0 || pad > aes.BlockSize {
		return nil, errDecrypt
	}
	for _, b := range pt[len(pt)-pad:] {
		if int(b) != pad {
			return nil, errDecrypt
		}
	}
	return pt[:len(pt)-pad], nil
}
