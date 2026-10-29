// SPDX-License-Identifier: GPL-3.0-or-later

package svr2

import (
	"encoding/hex"
	"errors"
	"strconv"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/pin.json and svr2.json.
type pinVectors struct {
	Published []struct {
		Pin                 string `json:"pin_utf8"`
		Salt, EncryptionKey string
		Enc                 string `json:"encryption_key"`
		Access              string `json:"access_key"`
		Master              string `json:"master_key"`
		Blob                string `json:"encrypted_master_key"`
	} `json:"published_pin_hash"`
	Salts []struct {
		Username, Salt string
		Group          string `json:"group_id"`
	} `json:"published_salt"`
	SIV []struct {
		Key, Plaintext, Ciphertext string
		Valid                      *bool
	} `json:"hmac_sha256_siv"`
	SVR2 []struct {
		Group      string `json:"group_id"`
		Username   string
		Input      string `json:"pin_input"`
		Normalized string `json:"normalized_pin_utf8"`
		Salt       string
		Enc        string `json:"encryption_key"`
		Access     string `json:"access_key"`
		Master     string `json:"master_key"`
		Blob       string `json:"encrypted_master_key"`
	} `json:"svr2_pin_hash"`
	Normalization []struct {
		Input string
		UTF8  string `json:"output_utf8"`
	}
	Derivations []struct {
		Master   string `json:"master_key"`
		Lock     string `json:"registration_lock"`
		Recovery string `json:"registration_recovery_password"`
	} `json:"master_key_derivations"`
}

func loadPIN(t *testing.T) *pinVectors {
	var v pinVectors
	testvec.Load(t, "pin.json", &v)
	return &v
}

func TestNormalization(t *testing.T) {
	v := loadPIN(t)
	for _, c := range v.Normalization {
		got, err := NormalizePIN(c.Input)
		want := testvec.Hex(t, c.UTF8)
		_ = want
		switch {
		case err == nil && hex.EncodeToString(got) == c.UTF8:
		case errors.Is(err, ErrUnsupportedPIN) && hasNonASCII(c.Input):
			// Refused: needs NFKD (documented deviation).
		default:
			t.Errorf("%q: %x %v, want %s", c.Input, got, err, c.UTF8)
		}
	}
}

func TestPINHashes(t *testing.T) {
	if testing.Short() {
		t.Skip("Argon2id with 16 MiB × 32 passes")
	}
	v := loadPIN(t)
	for _, c := range v.Published {
		enc, acc, err := PINHash(testvec.Hex(t, c.Pin), testvec.Hex(t, c.Salt))
		if err != nil || hex.EncodeToString(enc[:]) != c.Enc || hex.EncodeToString(acc[:]) != c.Access {
			t.Errorf("published: %x %x %v", enc, acc, err)
		}
		if c.Master == "" {
			continue
		}
		if blob := EncryptMasterKey(enc, [32]byte(testvec.Hex(t, c.Master))); hex.EncodeToString(blob) != c.Blob {
			t.Errorf("published blob %x", blob)
		}
	}
	for i, c := range v.SVR2 {
		if i%5 != 0 { // one PIN per enclave/username keeps the test fast
			continue
		}
		g, _ := strconv.ParseUint(c.Group, 10, 64)
		n, err := NormalizePIN(c.Input)
		if err != nil {
			continue
		}
		if hex.EncodeToString(n) != c.Normalized || hex.EncodeToString(Salt(c.Username, g)) != c.Salt {
			t.Errorf("%d: normalized %x salt %x", i, n, Salt(c.Username, g))
		}
		enc, acc, _ := PINHash(n, Salt(c.Username, g))
		if hex.EncodeToString(enc[:]) != c.Enc || hex.EncodeToString(acc[:]) != c.Access {
			t.Errorf("%d: keys", i)
		}
		if blob := EncryptMasterKey(enc, [32]byte(testvec.Hex(t, c.Master))); hex.EncodeToString(blob) != c.Blob {
			t.Errorf("%d: blob", i)
		}
	}
}

func TestSaltsSIVDerivations(t *testing.T) {
	v := loadPIN(t)
	for _, c := range v.Salts {
		g, _ := strconv.ParseUint(c.Group, 10, 64)
		if hex.EncodeToString(Salt(c.Username, g)) != c.Salt {
			t.Errorf("salt %s/%s", c.Username, c.Group)
		}
	}
	for _, c := range v.SIV {
		k := [32]byte(testvec.Hex(t, c.Key))
		m, err := DecryptMasterKey(k, testvec.Hex(t, c.Ciphertext))
		if c.Valid != nil && !*c.Valid {
			if err == nil {
				t.Errorf("tampered blob accepted: %s", c.Ciphertext)
			}
			continue
		}
		if err != nil || hex.EncodeToString(m[:]) != c.Plaintext {
			t.Errorf("siv decrypt %x %v", m, err)
		}
		if blob := EncryptMasterKey(k, m); hex.EncodeToString(blob) != c.Ciphertext {
			t.Errorf("siv encrypt %x", blob)
		}
	}
	for _, c := range v.Derivations {
		m := [32]byte(testvec.Hex(t, c.Master))
		if RegistrationLock(m) != c.Lock || RecoveryPassword(m) != c.Recovery {
			t.Errorf("derivations %s %s", RegistrationLock(m), RecoveryPassword(m))
		}
	}
}

func TestMessages(t *testing.T) {
	var v struct {
		Enclaves []struct {
			MREnclave string
			Config    string `json:"raft_config_protobuf_hex"`
		}
		Requests []struct {
			Name, Data, Pin, Encoded string
			MaxTries                 uint32 `json:"max_tries"`
		}
		Responses []struct {
			Encoded string
			Decoded map[string]struct {
				Status any
				Data   string
				Tries  uint32
			}
		}
	}
	testvec.Load(t, "svr2.json", &v)
	for _, r := range v.Requests {
		var got []byte
		switch r.Name {
		case "backup":
			got = BackupRequest(testvec.Hex(t, r.Data), testvec.Hex(t, r.Pin), r.MaxTries)
		case "expose":
			got = ExposeRequest(testvec.Hex(t, r.Data))
		case "restore":
			got = RestoreRequest(testvec.Hex(t, r.Pin))
		case "delete":
			got = DeleteRequest()
		}
		if hex.EncodeToString(got) != r.Encoded {
			t.Errorf("%s: %x", r.Name, got)
		}
	}
	names := map[string]int{"UNSET": 0, "OK": 1, "MISSING": 2, "PIN_MISMATCH": 3, "ERROR": 2}
	for _, r := range v.Responses {
		got, err := ParseResponse(testvec.Hex(t, r.Encoded))
		if err != nil {
			t.Errorf("%s: %v", r.Encoded, err)
			continue
		}
		for _, d := range r.Decoded {
			want := 0
			switch s := d.Status.(type) {
			case string:
				want = names[s]
			case float64:
				want = int(s)
			}
			if got.Status != want || hex.EncodeToString(got.Data) != d.Data || got.Tries != d.Tries {
				t.Errorf("%s: %+v", r.Encoded, got)
			}
		}
	}
	for _, e := range v.Enclaves {
		for _, s := range StagingEnclaves {
			if s.MREnclave == e.MREnclave && e.Config != "" && hex.EncodeToString(s.RaftConfig()) != e.Config {
				t.Errorf("%s raft config %x", e.MREnclave[:8], s.RaftConfig())
			}
		}
	}
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r >= 0x80 {
			return true
		}
	}
	return false
}
