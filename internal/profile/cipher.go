// Package profile implements Signal profile field encryption and the
// decoding of profile responses (spec/crypto/profile-cipher.md).
package profile

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"google.golang.org/protobuf/encoding/protowire"
)

// ErrDecrypt is a decryption failure: too short or a tag mismatch.
var ErrDecrypt = errors.New("profile: decryption failed")

// Field is a padded string field.
type Field int

const (
	FieldName Field = iota
	FieldAbout
	FieldAboutEmoji
)

const (
	nonceSize      = 12
	tagSize        = 16
	paymentContent = 554 // payment-address plaintext size (§4.2)
)

// PadTarget is the padded plaintext length for a value of n UTF-8 bytes
// (§2.1).
func PadTarget(f Field, n int) (int, error) {
	var t int
	switch f {
	case FieldName:
		t = 53
		if n > 53 {
			t = 257
		}
	case FieldAbout:
		switch {
		case n <= 128:
			t = 128
		case n < 254:
			t = 254
		default:
			t = 512
		}
	case FieldAboutEmoji:
		t = 32
	}
	if n > t {
		return 0, errors.New("Input is too long")
	}
	return t, nil
}

func gcm(key [32]byte) cipher.AEAD {
	b, _ := aes.NewCipher(key[:])
	a, _ := cipher.NewGCM(b)
	return a
}

func seal(key [32]byte, nonce [nonceSize]byte, plain []byte) []byte {
	return gcm(key).Seal(append([]byte{}, nonce[:]...), nonce[:], plain, nil)
}

func open(key [32]byte, ct []byte) ([]byte, error) {
	if len(ct) < nonceSize+tagSize+1 {
		return nil, fmt.Errorf("%w: too short: %d", ErrDecrypt, len(ct))
	}
	p, err := gcm(key).Open(nil, ct[:nonceSize], ct[nonceSize:], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return p, nil
}

// NewNonce returns 12 random bytes.
func NewNonce(r io.Reader) [nonceSize]byte {
	if r == nil {
		r = rand.Reader
	}
	var n [nonceSize]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		panic(err)
	}
	return n
}

// EncryptString pads value to its field's target and encrypts it.
func EncryptString(key [32]byte, nonce [nonceSize]byte, f Field, value string) ([]byte, error) {
	t, err := PadTarget(f, len(value))
	if err != nil {
		return nil, err
	}
	p := make([]byte, t)
	copy(p, value)
	return seal(key, nonce, p), nil
}

// DecryptString decrypts a string field, strips trailing zero bytes and
// decodes UTF-8 with replacement (§2.3).
func DecryptString(key [32]byte, ct []byte) (string, error) {
	p, err := open(key, ct)
	if err != nil {
		return "", err
	}
	end := len(p)
	for end > 0 && p[end-1] == 0 {
		end--
	}
	return strings.ToValidUTF8(string(p[:end]), string(utf8.RuneError)), nil
}

// EncryptBool encrypts the 1-byte phone-number-sharing flag (§3).
func EncryptBool(key [32]byte, nonce [nonceSize]byte, v bool) []byte {
	b := []byte{0}
	if v {
		b[0] = 1
	}
	return seal(key, nonce, b)
}

// DecryptBool is true when the first plaintext byte is non-zero.
func DecryptBool(key [32]byte, ct []byte) (bool, error) {
	p, err := open(key, ct)
	if err != nil {
		return false, err
	}
	return p[0] != 0, nil
}

// EncryptPaymentAddress builds, signs and encrypts a MobileCoin payment
// address (§4.1, §4.2).
func EncryptPaymentAddress(key [32]byte, nonce [nonceSize]byte, identity *curve25519.PrivateKey, address []byte, r io.Reader) ([]byte, error) {
	if r == nil {
		r = rand.Reader
	}
	sig, err := identity.Sign(r, address)
	if err != nil {
		return nil, err
	}
	var mc []byte
	mc = protowire.AppendTag(mc, 1, protowire.BytesType)
	mc = protowire.AppendBytes(mc, address)
	mc = protowire.AppendTag(mc, 2, protowire.BytesType)
	mc = protowire.AppendBytes(mc, sig)
	var pa []byte
	pa = protowire.AppendTag(pa, 1, protowire.BytesType)
	pa = protowire.AppendBytes(pa, mc)
	if 4+len(pa) > paymentContent {
		return nil, errors.New("Input is too long")
	}
	p := make([]byte, paymentContent)
	binary.LittleEndian.PutUint32(p, uint32(len(pa)))
	copy(p[4:], pa)
	return seal(key, nonce, p), nil
}

// DecryptPaymentAddress returns the verified public address. A decryption
// failure is an error; every softer problem yields a nil address (§4.3).
func DecryptPaymentAddress(key [32]byte, ct []byte, identity *curve25519.PublicKey) ([]byte, error) {
	p, err := open(key, ct)
	if err != nil {
		return nil, err
	}
	if identity == nil || len(p) < 4 {
		return nil, nil
	}
	l := int32(binary.LittleEndian.Uint32(p))
	if l < 0 || int(l) > len(p)-4 {
		return nil, nil
	}
	mc, ok := field(p[4:4+l], 1)
	if !ok {
		return nil, nil
	}
	addr, ok1 := field(mc, 1)
	sig, ok2 := field(mc, 2)
	if !ok1 || !ok2 || len(sig) != 64 || !identity.Verify(addr, sig) {
		return nil, nil
	}
	return addr, nil
}

// field returns the last occurrence of a length-delimited field; ok is
// false if the message does not parse or lacks the field.
func field(msg []byte, num protowire.Number) ([]byte, bool) {
	var out []byte
	found := false
	for len(msg) > 0 {
		n, typ, l := protowire.ConsumeTag(msg)
		if l < 0 {
			return nil, false
		}
		msg = msg[l:]
		vl := protowire.ConsumeFieldValue(n, typ, msg)
		if vl < 0 {
			return nil, false
		}
		if n == num && typ == protowire.BytesType {
			v, _ := protowire.ConsumeBytes(msg)
			out, found = v, true
		}
		msg = msg[vl:]
	}
	return out, found
}

// EncryptAvatar encrypts avatar bytes without padding (§5.1).
func EncryptAvatar(key [32]byte, nonce [nonceSize]byte, image []byte) []byte {
	return seal(key, nonce, image)
}

// DecryptAvatar decrypts and verifies a downloaded avatar. Unlike the
// reference, nothing is returned unless the tag verifies (§5.2).
func DecryptAvatar(key [32]byte, data []byte) ([]byte, error) {
	if len(data) < nonceSize+tagSize {
		return nil, ErrDecrypt
	}
	p, err := gcm(key).Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return p, nil
}

// Verifier is the unidentified-access verifier for a profile key (§7).
func Verifier(key [32]byte) []byte {
	m := hmac.New(sha256.New, api.UnidentifiedAccessKey(key[:]))
	m.Write(make([]byte, 32))
	return m.Sum(nil)
}

// SplitName splits a decrypted name on NUL with trailing empty parts
// dropped, as Java's String.split does (§2.4).
func SplitName(s string) (given, family *string) {
	parts := strings.Split(s, "\x00")
	for len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 1 && parts[0] == "" && s != "" {
		parts = []string{""} // all NULs: Java yields an empty array; given ""
	}
	g := parts[0]
	given = &g
	if len(parts) > 1 {
		f := parts[1]
		family = &f
	}
	return given, family
}

// JoinName is the wire form of given and family name (§2.2).
func JoinName(given, family *string) string {
	g := ""
	if given != nil {
		g = *given
	}
	if family == nil {
		return g
	}
	return g + "\x00" + *family
}

// DisplayName joins the non-empty parts with a space (§8).
func DisplayName(given, family *string) string {
	g, f := "", ""
	if given != nil {
		g = *given
	}
	if family != nil {
		f = *family
	}
	switch {
	case g == "":
		return f
	case f == "":
		return g
	}
	return g + " " + f
}

// Response is a profile response as the JSON layer parses it
// (spec/service/profiles.md §5.3).
type Response struct {
	IdentityKey        string           `json:"identityKey"`
	UnidentifiedAccess *string          `json:"unidentifiedAccess"`
	Unrestricted       bool             `json:"unrestrictedUnidentifiedAccess"`
	Capabilities       *map[string]bool `json:"capabilities"`
	UUID               string           `json:"uuid"`
	Name               *string          `json:"name"`
	About              *string          `json:"about"`
	AboutEmoji         *string          `json:"aboutEmoji"`
	Avatar             *string          `json:"avatar"`
	PaymentAddress     []byte           `json:"paymentAddress"` // base64; invalid fails the parse
	PhoneNumberSharing *string          `json:"phoneNumberSharing"`
	Credential         *string          `json:"credential"`
}

// ParseResponse parses a profile response body.
func ParseResponse(body []byte) (*Response, error) {
	var r Response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Access modes and capabilities as stored.
const (
	ModeUnknown      = "UNKNOWN"
	ModeDisabled     = "DISABLED"
	ModeEnabled      = "ENABLED"
	ModeUnrestricted = "UNRESTRICTED"

	CapStorage = "storage"
	CapSSRE2   = "storageServiceEncryptionV2Capability"
)

// Decoded is the profile record decoded from a response with a key.
type Decoded struct {
	GivenName, FamilyName, About, AboutEmoji *string
	AvatarURLPath                            *string
	MobileCoinAddress                        []byte
	PhoneNumberSharing                       *string // EVERYBODY, NOBODY or nil
	AccessMode                               string
	Capabilities                             []string
}

// IdentityKey parses the response's identity key; nil if unusable.
func (r *Response) IdentityPublicKey() *curve25519.PublicKey {
	b, err := api.DecodeBase64(r.IdentityKey)
	if err != nil {
		return nil
	}
	k, err := curve25519.ParsePublicKey(b)
	if err != nil {
		return nil
	}
	return k
}

// CapabilityList maps the response capabilities to the stored set
// (spec/service/profiles.md §5.4).
func (r *Response) CapabilityList() []string {
	var out []string
	if r.Capabilities == nil {
		return out
	}
	if (*r.Capabilities)["storage"] {
		out = append(out, CapStorage)
	}
	if (*r.Capabilities)["ssre2"] {
		out = append(out, CapSSRE2)
	}
	return out
}

// ModeWithoutKey is the access mode computed without a profile key.
func (r *Response) ModeWithoutKey() string {
	if r.Unrestricted {
		return ModeUnrestricted
	}
	return ModeDisabled
}

func decodeB64(s *string) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	b, err := base64.StdEncoding.DecodeString(*s)
	if err != nil {
		return nil, false
	}
	return b, true
}

// Decode decrypts every encrypted member with key (§6). Any decryption
// failure fails the whole decode with ErrDecrypt.
func Decode(key [32]byte, r *Response) (*Decoded, error) {
	d := &Decoded{AvatarURLPath: r.Avatar, Capabilities: r.CapabilityList()}
	str := func(member *string) (*string, error) {
		ct, ok := decodeB64(member)
		if !ok {
			return nil, nil
		}
		s, err := DecryptString(key, ct)
		if err != nil {
			return nil, err
		}
		return &s, nil
	}
	name, err := str(r.Name)
	if err != nil {
		return nil, err
	}
	if name != nil {
		d.GivenName, d.FamilyName = SplitName(*name)
	}
	if d.About, err = str(r.About); err != nil {
		return nil, err
	}
	if d.AboutEmoji, err = str(r.AboutEmoji); err != nil {
		return nil, err
	}
	if ct, ok := decodeB64(r.PhoneNumberSharing); ok {
		v, err := DecryptBool(key, ct)
		if err != nil {
			return nil, err
		}
		mode := "NOBODY"
		if v {
			mode = "EVERYBODY"
		}
		d.PhoneNumberSharing = &mode
	}
	if r.PaymentAddress != nil {
		if ik := r.IdentityPublicKey(); ik != nil {
			if d.MobileCoinAddress, err = DecryptPaymentAddress(key, r.PaymentAddress, ik); err != nil {
				return nil, err
			}
		}
	}
	switch {
	case r.Unrestricted:
		d.AccessMode = ModeUnrestricted
	case r.UnidentifiedAccess != nil && verifierMatches(key, *r.UnidentifiedAccess):
		d.AccessMode = ModeEnabled
	default:
		d.AccessMode = ModeDisabled
	}
	return d, nil
}

func verifierMatches(key [32]byte, member string) bool {
	v, err := base64.StdEncoding.DecodeString(member)
	if err != nil || len(v) == 0 {
		return false
	}
	return hmac.Equal(v, Verifier(key))
}
