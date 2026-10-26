package zkgroup

import (
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/usernames.json (spec/crypto/usernames.md §3, §5).
type usernameVectors struct {
	Hashes []struct {
		Username, Hash string
		SHA            string `json:"username_sha_scalar"`
		Nick           string `json:"nickname_scalar"`
		Disc           string `json:"discriminator_scalar"`
		Error          *struct{ Message string }
	}
	Proofs []struct {
		Username, Hash, Randomness, Proof string
	}
}

func splitUsername(t *testing.T, u string) (string, uint64) {
	i := strings.LastIndexByte(u, '.')
	d, err := strconv.ParseUint(u[i+1:], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(u[:i]), d
}

func TestUsernameHash(t *testing.T) {
	var v usernameVectors
	testvec.Load(t, "usernames.json", &v)
	for _, c := range v.Hashes {
		n, d := splitUsername(t, c.Username)
		sha, nick, disc, err := usernameScalars(n, d)
		if c.Error != nil {
			if err == nil || err.Error() != c.Error.Message {
				t.Errorf("%s: err %v, want %s", c.Username, err, c.Error.Message)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.Username, err)
		}
		if hex.EncodeToString(sha.Bytes()) != c.SHA || hex.EncodeToString(nick.Bytes()) != c.Nick || hex.EncodeToString(disc.Bytes()) != c.Disc {
			t.Errorf("%s: scalars %x %x %x", c.Username, sha.Bytes(), nick.Bytes(), disc.Bytes())
		}
		h, _ := UsernameHash(n, d)
		if hex.EncodeToString(h) != c.Hash {
			t.Errorf("%s: hash %x", c.Username, h)
		}
	}
}

func TestUsernameProof(t *testing.T) {
	var v usernameVectors
	testvec.Load(t, "usernames.json", &v)
	for _, c := range v.Proofs {
		n, d := splitUsername(t, c.Username)
		p, err := UsernameProof(n, d, [32]byte(testvec.Hex(t, c.Randomness)))
		if err != nil || hex.EncodeToString(p) != c.Proof {
			t.Errorf("%s: proof %x %v", c.Username, p, err)
		}
		hash := testvec.Hex(t, c.Hash)
		if !VerifyUsernameProof(p, hash) {
			t.Errorf("%s: own proof does not verify", c.Username)
		}
		bad := append([]byte{}, p...)
		bad[40] ^= 1
		if VerifyUsernameProof(bad, hash) || VerifyUsernameProof(p[:96], hash) || VerifyUsernameProof(p, hash[:31]) {
			t.Errorf("%s: a broken proof verified", c.Username)
		}
	}
	// The upstream nickname-scalar example (§3.3).
	if _, nick, _, _ := usernameScalars("_z9_", 1); hex.EncodeToString(nick.Bytes()[:4]) != hex.EncodeToString([]byte{0xa0, 0x23, 0x01, 0x00}) {
		t.Errorf("_z9_ nickname scalar %x, want 74656", nick.Bytes()[:4])
	}
}
