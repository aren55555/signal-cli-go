// SPDX-License-Identifier: GPL-3.0-or-later

package zkgroup

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/zkgroup_groups.json (spec/crypto/zkgroup-groups.md §11).

type result struct {
	OK    bool   `json:"ok"`
	Value string `json:"value"`
	Error string `json:"error"`
}

type groupVectors struct {
	Lizard []struct{ Data, Point string }
	Slots  struct {
		Slots []string
	} `json:"identity_preimage_slots"`
	Generate []struct {
		Randomness string
		MasterKey  string `json:"master_key"`
	} `json:"generate_from_randomness"`
	GroupParams []struct {
		MasterKey       string `json:"master_key"`
		SecretParams    string `json:"secret_params"`
		PublicParams    string `json:"public_params"`
		GroupIdentifier string `json:"group_identifier"`
	} `json:"group_params"`
	UUIDCiphertexts []struct {
		MasterKey  string `json:"master_key"`
		Kind       string
		UUID       string
		Ciphertext string
		Decrypted  string `json:"decrypted_fixed_width_service_id"`
	} `json:"uuid_ciphertexts"`
	UUIDErrors struct {
		MasterKey string `json:"master_key"`
		Base      string `json:"valid_ciphertext_base"`
		Cases     []struct {
			Case       string
			Ciphertext string
			Result     result
		}
	} `json:"uuid_ciphertext_errors"`
	ProfileKeys []struct {
		MasterKey  string `json:"master_key"`
		ACI        string
		ProfileKey string `json:"profile_key"`
		Ciphertext string
		Decrypt    result
		WrongACI   result `json:"decrypt_with_aci_11111111-2222-4333-8444-555555555555"`
	} `json:"profile_key_ciphertexts"`
	Blob struct {
		Upstream  []blobVector
		Generated []blobVector
		Errors    struct {
			MasterKey string `json:"master_key"`
			Cases     []struct {
				Case       string
				Ciphertext string
				Result     result
			}
		}
		Crafted struct {
			MasterKey string `json:"master_key"`
			Cases     []struct {
				Blob    string
				Decrypt result `json:"decrypt_with_padding"`
			}
		} `json:"crafted_padding"`
	}
	Notary struct {
		Cases []struct {
			Name      string
			PublicKey string `json:"public_key"`
			Message   string
			Signature string
			Valid     bool
		}
	} `json:"notary_signatures"`
	ServerParams struct {
		Config map[string]map[string]struct {
			Base64     string
			Length     int
			Components map[string]any
		} `json:"signal_cli_config"`
	} `json:"server_public_params"`
}

type blobVector struct {
	Name               string
	MasterKey          string `json:"master_key"`
	GenerateRandomness string `json:"generate_randomness"`
	Padded             *bool
	PaddingLen         uint32 `json:"padding_len"`
	Plaintext          string
	Randomness         string
	Ciphertext         string
}

func loadGroups(t *testing.T) groupVectors {
	var v groupVectors
	testvec.Load(t, "zkgroup_groups.json", &v)
	return v
}

func key32(t *testing.T, s string) [32]byte {
	var k [32]byte
	b := testvec.Hex(t, s)
	if len(b) != 32 {
		t.Fatalf("not 32 bytes: %s", s)
	}
	copy(k[:], b)
	return k
}

func uuid16(t *testing.T, s string) [16]byte {
	var u [16]byte
	copy(u[:], testvec.Hex(t, strings.ReplaceAll(s, "-", "")))
	return u
}

func TestLizard(t *testing.T) {
	v := loadGroups(t)
	for _, c := range v.Lizard {
		var u [16]byte
		copy(u[:], testvec.Hex(t, c.Data))
		p := LizardEncode(u)
		if hex.EncodeToString(p.Bytes()) != c.Point {
			t.Errorf("encode %s: %x", c.Data, p.Bytes())
		}
		got, ok := LizardDecode(p)
		if !ok || got != u {
			t.Errorf("decode %s: %x %v", c.Data, got, ok)
		}
	}
}

func TestIdentityPreimageSlots(t *testing.T) {
	v := loadGroups(t)
	slots := preimageSlots(ristretto255.NewIdentityElement())
	var got, want []string
	for _, s := range slots {
		if s != nil {
			got = append(got, hex.EncodeToString(s.Bytes()))
		}
	}
	want = v.Slots.Slots
	if len(got) != 8 {
		t.Fatalf("%d defined slots", len(got))
	}
	count := func(list []string) map[string]int {
		m := map[string]int{}
		for _, s := range list {
			m[s]++
		}
		return m
	}
	g, w := count(got), count(want)
	for k, n := range w {
		if g[k] != n {
			t.Errorf("slot %s: %d times, want %d", k, g[k], n)
		}
	}
}

func TestGroupParams(t *testing.T) {
	v := loadGroups(t)
	for _, c := range v.Generate {
		if got := GenerateMasterKey(key32(t, c.Randomness)); hex.EncodeToString(got[:]) != c.MasterKey {
			t.Errorf("generate %s: %x", c.Randomness, got)
		}
	}
	for _, c := range v.GroupParams {
		g := DeriveGroupSecretParams(key32(t, c.MasterKey))
		if hex.EncodeToString(g.Serialize()) != c.SecretParams {
			t.Errorf("%s: secret params %x", c.MasterKey, g.Serialize())
		}
		if hex.EncodeToString(g.PublicParams()) != c.PublicParams {
			t.Errorf("%s: public params %x", c.MasterKey, g.PublicParams())
		}
		if hex.EncodeToString(g.GroupID[:]) != c.GroupIdentifier {
			t.Errorf("%s: group id %x", c.MasterKey, g.GroupID)
		}
	}
}

func TestServiceIDCiphertexts(t *testing.T) {
	v := loadGroups(t)
	if len(v.UUIDCiphertexts) == 0 {
		t.Fatal("no vectors")
	}
	for _, c := range v.UUIDCiphertexts {
		g := DeriveGroupSecretParams(key32(t, c.MasterKey))
		id := ServiceID{PNI: c.Kind == "PNI", UUID: uuid16(t, c.UUID)}
		ct := g.EncryptServiceID(id)
		if hex.EncodeToString(ct) != c.Ciphertext {
			t.Errorf("%s %s: %x", c.Kind, c.UUID, ct)
		}
		got, err := g.DecryptServiceID(ct)
		if err != nil || hex.EncodeToString(got.FixedWidth()) != c.Decrypted {
			t.Errorf("%s %s: decrypt %v %x", c.Kind, c.UUID, err, got.FixedWidth())
		}
	}
	e := v.UUIDErrors
	g := DeriveGroupSecretParams(key32(t, e.MasterKey))
	var base []byte
	for _, c := range v.UUIDCiphertexts {
		if c.MasterKey == e.MasterKey && c.Kind == "ACI" && c.UUID == "00010203-0405-0607-0809-0a0b0c0d0e0f" {
			base = testvec.Hex(t, c.Ciphertext)
		}
	}
	for _, c := range e.Cases {
		ct := testvec.Hex(t, c.Ciphertext)
		dec := g
		switch c.Case {
		case "wrong_group":
			ct, dec = base, DeriveGroupSecretParams([32]byte{})
		case "reserved1":
			ct = append([]byte{1}, base[1:]...)
		case "len64":
			ct = base[:64]
		case "len66":
			ct = append(bytes.Clone(base), 0)
		case "badpoint":
			ct = bytes.Clone(base)
			for i := 1; i < 33; i++ {
				ct[i] = 0xff
			}
		}
		_, err := dec.DecryptServiceID(ct)
		checkErr(t, c.Case, err, c.Result)
	}
}

func checkErr(t *testing.T, name string, err error, want result) {
	t.Helper()
	switch {
	case want.OK && err != nil:
		t.Errorf("%s: %v, want success", name, err)
	case !want.OK && err == nil:
		t.Errorf("%s: no error, want %s", name, want.Error)
	case want.Error == "deserialization_failure" && !errors.Is(err, ErrDeserialization):
		t.Errorf("%s: %v, want deserialization failure", name, err)
	case want.Error == "verification_failure" && !errors.Is(err, ErrVerification):
		t.Errorf("%s: %v, want verification failure", name, err)
	}
}

func TestProfileKeyCiphertexts(t *testing.T) {
	v := loadGroups(t)
	wrong := uuid16(t, "11111111-2222-4333-8444-555555555555")
	for _, c := range v.ProfileKeys {
		g := DeriveGroupSecretParams(key32(t, c.MasterKey))
		aci := uuid16(t, c.ACI)
		ct := g.EncryptProfileKey(key32(t, c.ProfileKey), aci)
		if hex.EncodeToString(ct) != c.Ciphertext {
			t.Errorf("%s: %x", c.ProfileKey, ct)
		}
		got, err := g.DecryptProfileKey(ct, aci)
		checkErr(t, "decrypt "+c.ProfileKey, err, c.Decrypt)
		if err == nil && hex.EncodeToString(got[:]) != c.Decrypt.Value {
			t.Errorf("%s: decrypted %x", c.ProfileKey, got)
		}
		_, err = g.DecryptProfileKey(ct, wrong)
		checkErr(t, "wrong ACI "+c.ProfileKey, err, c.WrongACI)
	}
}

func TestBlobs(t *testing.T) {
	v := loadGroups(t)
	for _, c := range append(append([]blobVector{}, v.Blob.Upstream...), v.Blob.Generated...) {
		var mk [32]byte
		if c.MasterKey != "" {
			mk = key32(t, c.MasterKey)
		} else {
			mk = GenerateMasterKey(key32(t, c.GenerateRandomness))
		}
		g := DeriveGroupSecretParams(mk)
		padded := c.Padded == nil || *c.Padded
		plain := testvec.Hex(t, c.Plaintext)
		if c.Randomness != "" {
			var got []byte
			if padded {
				got = g.EncryptBlob(key32(t, c.Randomness), plain, c.PaddingLen)
			} else {
				got = g.encryptBlobUnpadded(key32(t, c.Randomness), plain)
			}
			if hex.EncodeToString(got) != c.Ciphertext {
				t.Errorf("%s: encrypt %x", c.Name, got)
			}
		}
		var dec []byte
		var err error
		if padded {
			dec, err = g.DecryptBlob(testvec.Hex(t, c.Ciphertext))
		} else {
			dec, err = g.decryptBlobUnpadded(testvec.Hex(t, c.Ciphertext))
		}
		if err != nil || !bytes.Equal(dec, plain) {
			t.Errorf("%s: decrypt %v %x", c.Name, err, dec)
		}
	}
	g := DeriveGroupSecretParams(key32(t, v.Blob.Errors.MasterKey))
	for _, c := range v.Blob.Errors.Cases {
		ct := testvec.Hex(t, c.Ciphertext)
		if c.Case == "wrong_group" || c.Ciphertext == "" && c.Case != "len0" {
			continue // built by the jar from another group; covered by "tampered"
		}
		got, err := g.DecryptBlob(ct)
		checkErr(t, c.Case, err, c.Result)
		if err == nil && hex.EncodeToString(got) != c.Result.Value {
			t.Errorf("%s: %x", c.Case, got)
		}
	}
	g = DeriveGroupSecretParams(key32(t, v.Blob.Crafted.MasterKey))
	for i, c := range v.Blob.Crafted.Cases {
		got, err := g.DecryptBlob(testvec.Hex(t, c.Blob))
		checkErr(t, "crafted", err, c.Decrypt)
		if err == nil && hex.EncodeToString(got) != c.Decrypt.Value {
			t.Errorf("crafted %d: %x", i, got)
		}
	}
}

func TestNotarySignatures(t *testing.T) {
	v := loadGroups(t)
	for _, c := range v.Notary.Cases {
		pub, err := new(ristretto255.Element).SetCanonicalBytes(testvec.Hex(t, c.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		if got := VerifyNotarySignature(pub, testvec.Hex(t, c.Message), testvec.Hex(t, c.Signature)); got != c.Valid {
			t.Errorf("%s: %v", c.Name, got)
		}
		if c.Valid {
			sig := testvec.Hex(t, c.Signature)
			sig[31] = 0xff
			if VerifyNotarySignature(pub, testvec.Hex(t, c.Message), sig) {
				t.Errorf("%s: non-canonical scalar accepted", c.Name)
			}
		}
	}
}

func TestServerPublicParams(t *testing.T) {
	v := loadGroups(t)
	for env, blobs := range v.ServerParams.Config {
		zk := blobs["zkGroupServerPublicParams"]
		b, _ := base64.StdEncoding.DecodeString(zk.Base64)
		p, err := ParseServerPublicParams(b)
		if err != nil {
			t.Fatalf("%s: %v", env, err)
		}
		if hex.EncodeToString(p.SigPublicKey.Bytes()) != zk.Components["sig_public_key"] {
			t.Errorf("%s: sig key %x", env, p.SigPublicKey.Bytes())
		}
		if _, err := ParseServerPublicParams(b[:672]); !errors.Is(err, ErrDeserialization) {
			t.Errorf("%s short: %v", env, err)
		}
	}
	if p, err := ParseServerPublicParams(StagingServerPublicParams()); err != nil || p == nil {
		t.Errorf("built-in staging params: %v", err)
	}
}
