// SPDX-License-Identifier: GPL-3.0-or-later

// Package username parses usernames, generates candidates and handles
// username links (spec/crypto/usernames.md §2, §4, §6). The hash and the
// proof are in internal/crypto/zkgroup.
package username

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"google.golang.org/protobuf/encoding/protowire"
)

// Error is a username error with libsignal's message and the Java class
// signal-cli surfaces (§2.5).
type Error struct{ Class, Message string }

func (e *Error) Error() string { return e.Message }

func errOf(class, msg string) *Error { return &Error{class, msg} }

var (
	ErrMissingSeparator     = errOf("MissingSeparatorException", "Username must contain a '.'")
	ErrNicknameEmpty        = errOf("CannotBeEmptyException", "Name cannot be empty")
	ErrNicknameDigit        = errOf("CannotStartWithDigitException", "Name cannot start with a digit")
	ErrNicknameCharacter    = errOf("BadNicknameCharacterException", "Nickname contains disallowed character")
	ErrNicknameTooShort     = errOf("NicknameTooShortException", "Nickname is too short")
	ErrNicknameTooLong      = errOf("NicknameTooLongException", "Nickname is too long")
	ErrDiscriminatorEmpty   = errOf("DiscriminatorCannotBeEmptyException", "Discriminator cannot be empty")
	ErrDiscriminatorZero    = errOf("DiscriminatorCannotBeZeroException", "Discriminator cannot be zero")
	ErrDiscriminatorSingle  = errOf("DiscriminatorCannotBeSingleDigitException", "Discriminator cannot be a single digit")
	ErrDiscriminatorLeading = errOf("DiscriminatorCannotHaveLeadingZerosException", "Discriminator cannot have leading zeros unless it would otherwise be a single digit")
	ErrDiscriminatorChar    = errOf("BadDiscriminatorCharacterException", "Discriminator must only be made up of digits")
	ErrDiscriminatorLarge   = errOf("DiscriminatorTooLargeException", "Value is too large to be a username discriminator")
)

// Username is a parsed username; Nickname keeps the case as typed.
type Username struct {
	Nickname      string
	Discriminator uint64
}

// String is the display form, discriminator padded to two digits (§2.6).
func (u Username) String() string { return fmt.Sprintf("%s.%02d", u.Nickname, u.Discriminator) }

// Hash is the 32-byte username hash.
func (u Username) Hash() ([]byte, error) {
	return zkgroup.UsernameHash(strings.ToLower(u.Nickname), u.Discriminator)
}

// Proof proves knowledge of u for its hash (§5).
func (u Username) Proof(r io.Reader) ([]byte, error) {
	if r == nil {
		r = rand.Reader
	}
	var z [32]byte
	if _, err := io.ReadFull(r, z[:]); err != nil {
		return nil, err
	}
	return zkgroup.UsernameProof(strings.ToLower(u.Nickname), u.Discriminator, z)
}

// checkNickname is §2.2; min and max are the soft limits (0 = none).
func checkNickname(n string, min, max int) error {
	if n == "" {
		return ErrNicknameEmpty
	}
	if n[0] >= '0' && n[0] <= '9' {
		return ErrNicknameDigit
	}
	count := 0
	for _, c := range n {
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return ErrNicknameCharacter
		}
		count++
	}
	if count > 48 {
		return ErrNicknameTooLong
	}
	if min > 0 && count < min {
		return ErrNicknameTooShort
	}
	if max > 0 && count > max {
		return ErrNicknameTooLong
	}
	return nil
}

func parseDiscriminator(s string) (uint64, error) {
	if s == "" {
		return 0, ErrDiscriminatorEmpty
	}
	if s[0] < '0' || s[0] > '9' {
		return 0, ErrDiscriminatorChar
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, ErrDiscriminatorChar
		}
	}
	d, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, ErrDiscriminatorLarge
	}
	switch {
	case d == 0:
		return 0, ErrDiscriminatorZero
	case len(s) == 1:
		return 0, ErrDiscriminatorSingle
	case len(s) >= 3 && s[0] == '0':
		return 0, ErrDiscriminatorLeading
	}
	return d, nil
}

// Parse parses a whole username: only the hard 48-character cap applies
// (§2.4).
func Parse(s string) (Username, error) {
	i := strings.LastIndexByte(s, '.')
	if i < 0 {
		return Username{}, ErrMissingSeparator
	}
	n, ds := s[:i], s[i+1:]
	// Order observed in the vectors (`.` → empty nickname, `x.01.` →
	// empty discriminator): nickname emptiness and first digit, then the
	// discriminator, then the remaining nickname rules. The spec lists the
	// nickname rules first; the reference-derived vectors win.
	switch {
	case n == "":
		return Username{}, ErrNicknameEmpty
	case n[0] >= '0' && n[0] <= '9':
		return Username{}, ErrNicknameDigit
	}
	d, err := parseDiscriminator(ds)
	if err != nil {
		return Username{}, err
	}
	if err := checkNickname(asciiLower(n), 0, 0); err != nil {
		return Username{}, err
	}
	return Username{Nickname: n, Discriminator: d}, nil
}

// asciiLower lowercases only A-Z.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

var candidateRanges = []struct {
	lo, hi uint64
	n      int
}{{1, 100, 4}, {100, 1000, 3}, {1000, 10000, 3}, {10000, 100000, 2}, {100000, 1000000, 2},
	{1000000, 10000000, 2}, {10000000, 100000000, 2}, {100000000, 1000000000, 2}}

// Candidates are the 20 usernames to try for a nickname (§4).
func Candidates(nickname string, min, max int, r io.Reader) ([]string, error) {
	if r == nil {
		r = rand.Reader
	}
	if err := checkNickname(asciiLower(nickname), min, max); err != nil {
		if nickname != "" && nickname[0] >= '0' && nickname[0] <= '9' {
			return nil, ErrNicknameDigit
		}
		return nil, err
	}
	var out []string
	for _, rg := range candidateRanges {
		seen := map[uint64]bool{}
		for len(seen) < rg.n {
			v, err := rand.Int(r, new(big.Int).SetUint64(rg.hi-rg.lo))
			if err != nil {
				return nil, err
			}
			d := rg.lo + v.Uint64()
			if !seen[d] {
				seen[d] = true
				out = append(out, Username{nickname, d}.String())
			}
		}
	}
	return out, nil
}

// Link errors (§2.5).
var (
	ErrLinkTooLong      = errOf("UsernameLinkInputDataTooLong", "The combined length of all input data is too long")
	ErrLinkEntropy      = errOf("UsernameLinkInvalidEntropyDataLength", "Invalid size of the entropy data")
	ErrLinkShort        = errOf("UsernameLinkInvalidLinkData", "Username link data size is too short: must contain IV, ciphertext, and HMAC")
	ErrLinkHMAC         = errOf("UsernameLinkInvalidLinkData", "HMAC on username link doesn't match the one calculated with the given entropy input")
	ErrLinkCiphertext   = errOf("UsernameLinkInvalidLinkData", "Ciphertext in the username link can't be decrypted")
	ErrLinkDecryptedBad = errOf("UsernameLinkInvalidLinkData", "Data decrypted from the username link is of invalid structure")
)

func linkKeys(entropy []byte) (aesKey, macKey []byte) {
	aesKey, _ = hkdf.Key(sha256.New, entropy, nil, "Signal Username Link Encryption Key", 32)
	macKey, _ = hkdf.Key(sha256.New, entropy, nil, "Signal Username Link Authentication Key", 32)
	return aesKey, macKey
}

// EncryptLink encrypts a username for a link (§6.3, §6.4).
func EncryptLink(entropy [32]byte, iv [16]byte, username string) ([]byte, error) {
	var p []byte
	p = protowire.AppendTag(p, 1, protowire.BytesType)
	p = protowire.AppendString(p, username)
	if pad := 48 - len(username); pad > 0 {
		p = protowire.AppendTag(p, 2, protowire.BytesType)
		p = protowire.AppendBytes(p, make([]byte, pad))
	}
	if len(p) >= 64 {
		return nil, ErrLinkTooLong
	}
	aesKey, macKey := linkKeys(entropy[:])
	n := 16 - len(p)%16
	p = append(p, bytes.Repeat([]byte{byte(n)}, n)...)
	block, _ := aes.NewCipher(aesKey)
	ct := make([]byte, len(p))
	cipher.NewCBCEncrypter(block, iv[:]).CryptBlocks(ct, p)
	out := append(append([]byte{}, iv[:]...), ct...)
	m := hmac.New(sha256.New, macKey)
	m.Write(out)
	return m.Sum(out), nil
}

// NewLinkBlob encrypts with a fresh random IV.
func NewLinkBlob(entropy [32]byte, username string) ([]byte, error) {
	var iv [16]byte
	if _, err := io.ReadFull(rand.Reader, iv[:]); err != nil {
		return nil, err
	}
	return EncryptLink(entropy, iv, username)
}

// DecryptLink decrypts a link blob (§6.5) to the username string.
func DecryptLink(entropy, blob []byte) (string, error) {
	if len(entropy) != 32 {
		return "", ErrLinkEntropy
	}
	if len(blob) <= 48 {
		return "", ErrLinkShort
	}
	aesKey, macKey := linkKeys(entropy)
	body, tag := blob[:len(blob)-32], blob[len(blob)-32:]
	m := hmac.New(sha256.New, macKey)
	m.Write(body)
	if !hmac.Equal(m.Sum(nil), tag) {
		return "", ErrLinkHMAC
	}
	iv, ct := body[:16], body[16:]
	if len(ct) == 0 || len(ct)%16 != 0 {
		return "", ErrLinkCiphertext
	}
	block, _ := aes.NewCipher(aesKey)
	p := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(p, ct)
	n := int(p[len(p)-1])
	if n == 0 || n > 16 || !bytes.Equal(p[len(p)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return "", ErrLinkCiphertext
	}
	p = p[:len(p)-n]
	var name string
	for len(p) > 0 {
		num, typ, l := protowire.ConsumeTag(p)
		if l < 0 {
			return "", ErrLinkDecryptedBad
		}
		p = p[l:]
		vl := protowire.ConsumeFieldValue(num, typ, p)
		if vl < 0 {
			return "", ErrLinkDecryptedBad
		}
		if num == 1 && typ == protowire.BytesType {
			v, _ := protowire.ConsumeBytes(p)
			name = string(v)
		}
		p = p[vl:]
	}
	return name, nil
}

// URL errors (§6.6).
var (
	ErrInvalidLink   = errors.New("Invalid username link")
	ErrInvalidBase64 = errors.New("Invalid base64 encoding")
)

var linkRe = regexp.MustCompile(`^(https://)?signal.me/?#eu/([a-zA-Z0-9+\-_/]+)$`)

// LinkURL is the canonical signal.me URL of a link (§6.6).
func LinkURL(entropy [32]byte, serverID string) string {
	b := append(append([]byte{}, entropy[:]...), uuidBytes(serverID)...)
	return "https://signal.me/#eu/" + base64.RawURLEncoding.EncodeToString(b)
}

// ParseLinkURL parses a signal.me username link into entropy and server
// ID, exactly as signal-cli does (§6.6).
func ParseLinkURL(s string) ([32]byte, string, error) {
	m := linkRe.FindStringSubmatch(s)
	if m == nil {
		return [32]byte{}, "", ErrInvalidLink
	}
	data := m[2]
	enc := base64.StdEncoding
	if strings.ContainsAny(data, "-_") {
		enc = base64.URLEncoding
	}
	if r := len(data) % 4; r != 0 {
		data += strings.Repeat("=", 4-r)
	}
	b, err := enc.DecodeString(data)
	if err != nil {
		return [32]byte{}, "", ErrInvalidBase64
	}
	if len(b) != 48 {
		return [32]byte{}, "", ErrInvalidLink
	}
	return [32]byte(b[:32]), formatUUID(b[32:]), nil
}

func uuidBytes(s string) []byte {
	b, _ := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if len(b) != 16 {
		return make([]byte, 16)
	}
	return b
}

func formatUUID(b []byte) string {
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
