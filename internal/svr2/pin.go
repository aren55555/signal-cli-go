// Package svr2 holds the PIN cryptography (spec/crypto/pin.md) and the
// SVR2 enclave messages (spec/service/svr.md §3).
package svr2

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aren55555/signal-cli-go/internal/crypto/argon2"
)

// ErrUnsupportedPIN is a PIN that would need Unicode NFKD after digit
// mapping. Go's standard library has no normalization tables, so such
// PINs are refused rather than hashed differently (COMPAT deviation).
var ErrUnsupportedPIN = errors.New("PIN contains characters signal-cli-go cannot normalize; use digits or ASCII")

// isKotlinSpace is Kotlin's isWhitespace for one UTF-16 unit (§5 step 1).
func isKotlinSpace(r rune) bool {
	return (r >= 0x09 && r <= 0x0d) || (r >= 0x1c && r <= 0x1f) || (r <= 0xffff && unicode.In(r, unicode.Zs, unicode.Zl, unicode.Zp))
}

// digitValue is the numeric value of a BMP Nd character.
func digitValue(r rune) (int, bool) {
	if r > 0xffff || !unicode.Is(unicode.Nd, r) {
		return 0, false
	}
	for _, rg := range unicode.Nd.R16 {
		if uint16(r) >= rg.Lo && uint16(r) <= rg.Hi {
			return int(uint16(r)-rg.Lo) % 10, true
		}
	}
	return 0, false
}

// NormalizePIN is pin.md §5: trim, map an all-digit PIN to ASCII, NFKD
// (identity for ASCII), UTF-8.
func NormalizePIN(pin string) ([]byte, error) {
	s := strings.TrimFunc(pin, isKotlinSpace)
	allDigits := true
	for _, r := range s {
		if _, ok := digitValue(r); !ok {
			allDigits = false
			break
		}
	}
	if allDigits {
		var b strings.Builder
		for _, r := range s {
			v, _ := digitValue(r)
			b.WriteByte(byte('0' + v))
		}
		s = b.String()
	}
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return nil, ErrUnsupportedPIN
		}
	}
	return []byte(s), nil
}

// Salt is HKDF-SHA256(salt = BE64(group_id), IKM = username) (§6.2).
func Salt(username string, groupID uint64) []byte {
	var g [8]byte
	binary.BigEndian.PutUint64(g[:], groupID)
	k, _ := hkdf.Key(sha256.New, []byte(username), g[:], "", 32)
	return k
}

// PINHash is the Argon2id PIN hash: (encryption key, access key) (§6.1).
func PINHash(normalized, salt []byte) (encKey, accessKey [32]byte, err error) {
	tag, err := argon2.Key(argon2.Argon2id, normalized, salt, nil, nil, argon2.Params{Memory: 16384, Passes: 32, Lanes: 1, TagLen: 64})
	if err != nil {
		return encKey, accessKey, err
	}
	return [32]byte(tag[:32]), [32]byte(tag[32:]), nil
}

func mac(key []byte, msg ...[]byte) []byte {
	m := hmac.New(sha256.New, key)
	for _, p := range msg {
		m.Write(p)
	}
	return m.Sum(nil)
}

// EncryptMasterKey is HMAC-SHA256-SIV into 48 bytes (§7).
func EncryptMasterKey(k [32]byte, m [32]byte) []byte {
	ka, ke := mac(k[:], []byte("auth")), mac(k[:], []byte("enc"))
	iv := mac(ka, m[:])[:16]
	kx := mac(ke, iv)
	out := append([]byte{}, iv...)
	for i := range 32 {
		out = append(out, kx[i]^m[i])
	}
	return out
}

// ErrInvalidCiphertext is a blob whose IV does not verify.
var ErrInvalidCiphertext = errors.New("InvalidCiphertextException")

// DecryptMasterKey reverses EncryptMasterKey.
func DecryptMasterKey(k [32]byte, blob []byte) ([32]byte, error) {
	var m [32]byte
	if len(blob) != 48 {
		return m, ErrInvalidCiphertext
	}
	ka, ke := mac(k[:], []byte("auth")), mac(k[:], []byte("enc"))
	kx := mac(ke, blob[:16])
	for i := range 32 {
		m[i] = kx[i] ^ blob[16+i]
	}
	if subtle.ConstantTimeCompare(mac(ka, m[:])[:16], blob[:16]) != 1 {
		return [32]byte{}, ErrInvalidCiphertext
	}
	return m, nil
}

// RegistrationLock is the lowercase-hex registration lock token (§3).
func RegistrationLock(master [32]byte) string {
	return hex.EncodeToString(mac(master[:], []byte("Registration Lock")))
}

// RecoveryPassword is the padded base64 recovery password (§3).
func RecoveryPassword(master [32]byte) string {
	return base64.StdEncoding.EncodeToString(mac(master[:], []byte("Registration Recovery")))
}
