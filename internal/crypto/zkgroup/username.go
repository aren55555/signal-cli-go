package zkgroup

import (
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
)

// Username hashes and their proof (spec/crypto/usernames.md §3, §5).

// The three generators of §3.1, as hard-coded encodings.
var usernameGenerators = func() [3]*ristretto255.Element {
	var g [3]*ristretto255.Element
	for i, h := range []string{
		"60b993663a3daecc4c852f533547e305388c2a50a58393ea277de4abf3de543a",
		"f2b6f1c826fa3640206f3b58b2286bdefdfda6a54ff902f204a72de737d26157",
		"0606bd3abfce4e9617d448fb2caeb6cc028ec9a2b62b10b3d9eb2948da6f3f53",
	} {
		b, _ := hex.DecodeString(h)
		g[i] = new(ristretto255.Element)
		if _, err := g[i].SetCanonicalBytes(b); err != nil {
			panic(err)
		}
	}
	return g
}()

var errNicknameCharacter = errors.New("Nickname contains disallowed character")

func smallScalar(v uint64) *edwards25519.Scalar {
	var b [32]byte
	binary.LittleEndian.PutUint64(b[:], v)
	s, _ := edwards25519.NewScalar().SetCanonicalBytes(b[:])
	return s
}

// usernameScalars are the three witness scalars for a lowercased
// nickname and a discriminator (§3.2-3.4).
func usernameScalars(nickname string, disc uint64) (sha, nick, d *edwards25519.Scalar, err error) {
	if len(nickname) == 0 || len(nickname) > 48 {
		return nil, nil, nil, errNicknameCharacter
	}
	vals := make([]uint64, len(nickname))
	for i, c := range []byte(nickname) {
		switch {
		case c == '_':
			vals[i] = 1
		case c >= 'a' && c <= 'z':
			vals[i] = uint64(c-'a') + 2
		case c >= '0' && c <= '9':
			vals[i] = uint64(c-'0') + 28
		default:
			return nil, nil, nil, errNicknameCharacter
		}
	}
	nick = edwards25519.NewScalar()
	for i := len(vals) - 1; i >= 1; i-- {
		nick.MultiplyAdd(nick, smallScalar(37), smallScalar(vals[i]))
	}
	nick.MultiplyAdd(nick, smallScalar(27), smallScalar(vals[0]))

	h := sha512.New()
	h.Write([]byte(nickname))
	h.Write([]byte{0})
	var be [8]byte
	binary.BigEndian.PutUint64(be[:], disc)
	h.Write(be[:])
	sha = wideScalar(h.Sum(nil))
	return sha, nick, smallScalar(disc), nil
}

// UsernameHash is the 32-byte hash of a lowercased nickname and a
// discriminator (§3.5).
func UsernameHash(nickname string, disc uint64) ([]byte, error) {
	sha, nick, d, err := usernameScalars(nickname, disc)
	if err != nil {
		return nil, err
	}
	g := usernameGenerators
	h := new(ristretto255.Element).ScalarMult(sha, g[0])
	h.Add(h, new(ristretto255.Element).ScalarMult(nick, g[1]))
	h.Add(h, new(ristretto255.Element).ScalarMult(d, g[2]))
	return h.Bytes(), nil
}

func usernameStatement() *statement {
	return newStatement().eq("username_hash", "username_sha_scalar", "G1", "nickname_scalar", "G2", "discriminator_scalar", "G3")
}

func usernamePoints(hash *ristretto255.Element) map[string]*ristretto255.Element {
	g := usernameGenerators
	return map[string]*ristretto255.Element{"username_hash": hash, "G1": g[0], "G2": g[1], "G3": g[2]}
}

// UsernameProof proves knowledge of the username behind its hash, over
// the 32-byte hash as message (§5.3); 128 bytes.
func UsernameProof(nickname string, disc uint64, randomness [32]byte) ([]byte, error) {
	sha, nick, d, err := usernameScalars(nickname, disc)
	if err != nil {
		return nil, err
	}
	hb, _ := UsernameHash(nickname, disc)
	h := new(ristretto255.Element)
	h.SetCanonicalBytes(hb)
	scalars := map[string]*edwards25519.Scalar{"username_sha_scalar": sha, "nickname_scalar": nick, "discriminator_scalar": d}
	return usernameStatement().prove(scalars, usernamePoints(h), hb, randomness[:]), nil
}

// VerifyUsernameProof checks a proof against a 32-byte hash (§5.4).
func VerifyUsernameProof(proof, hash []byte) bool {
	if len(hash) != 32 {
		return false
	}
	h := new(ristretto255.Element)
	if _, err := h.SetCanonicalBytes(hash); err != nil {
		return false
	}
	return usernameStatement().verify(proof, usernamePoints(h), hash)
}
