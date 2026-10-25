package profile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/profile_cipher.json (spec/crypto/profile-cipher.md §10).
type vectors struct {
	Keys    map[string]string
	Padding []struct {
		Field  string
		Length int `json:"utf8_length"`
		Target int
	} `json:"padding_targets"`
	Strings []struct {
		Field, Key, Nonce, Ciphertext string
		Plain                         string `json:"plaintext_utf8_hex"`
		Target                        int    `json:"target_length"`
	} `json:"string_fields"`
	Reference []struct {
		Field, Key, Nonce, Ciphertext string
		Plain                         string `json:"plaintext_utf8_hex"`
	} `json:"reference_encrypt_string"`
	Quirks []struct {
		Case, Key, Ciphertext, Error string
		Decrypted                    string `json:"decrypted_utf8_hex"`
		InputLength                  int    `json:"input_length"`
	} `json:"decrypt_string_quirks"`
	Bools []struct {
		Value      bool
		Key, Nonce string
		Ciphertext string
	} `json:"boolean_fields"`
	Payment struct {
		Key, Nonce, Ciphertext string
		Private                string `json:"identity_private_key"`
		Identity               string `json:"identity_key_33"`
		Address                string `json:"mobilecoin_public_address"`
		Proto                  string `json:"payment_address_proto"`
		Prefixed               string `json:"length_prefixed_plaintext_unpadded"`
	} `json:"payment_address"`
	Access struct {
		Key      string
		UAK      string `json:"access_key_16"`
		Verifier string `json:"verifier_32"`
	} `json:"unidentified_access"`
	Avatar []struct {
		Key, Plaintext, Nonce, Ciphertext string
		Case, Error                       string
	}
	Decode []struct {
		Case     string
		Response string `json:"response_json"`
		Result   *struct {
			GivenName, FamilyName, About, AboutEmoji, AvatarURLPath *string
			AvatarPath                                              *string `json:"avatarUrlPath"`
			MobileCoin                                              *string `json:"mobileCoinAddress"`
			Mode                                                    string  `json:"unidentifiedAccessMode"`
			Capabilities                                            []string
			Sharing                                                 *string `json:"phoneNumberSharingMode"`
		}
	} `json:"decode_profile"`
}

func load(t *testing.T) *vectors {
	var v vectors
	testvec.Load(t, "profile_cipher.json", &v)
	return &v
}

func key(t *testing.T, v *vectors, name string) [32]byte {
	return [32]byte(testvec.Hex(t, v.Keys[name]))
}

func fieldOf(t *testing.T, s string) Field {
	switch s {
	case "name":
		return FieldName
	case "about":
		return FieldAbout
	case "about_emoji", "aboutEmoji", "emoji":
		return FieldAboutEmoji
	}
	t.Fatalf("unknown field %q", s)
	return 0
}

func TestPaddingTargets(t *testing.T) {
	v := load(t)
	for _, c := range v.Padding {
		got, err := PadTarget(fieldOf(t, c.Field), c.Length)
		if c.Target == 0 {
			if err == nil {
				t.Errorf("%s %d: want error, got %d", c.Field, c.Length, got)
			}
			continue
		}
		if err != nil || got != c.Target {
			t.Errorf("%s %d: target %d %v, want %d", c.Field, c.Length, got, err, c.Target)
		}
	}
}

func TestStringFields(t *testing.T) {
	v := load(t)
	type sv struct{ field, key, nonce, plain, ct string }
	var all []sv
	for _, c := range v.Strings {
		all = append(all, sv{c.Field, c.Key, c.Nonce, c.Plain, c.Ciphertext})
	}
	for _, c := range v.Reference {
		all = append(all, sv{c.Field, c.Key, c.Nonce, c.Plain, c.Ciphertext})
	}
	for i, c := range all {
		k := key(t, v, c.key)
		got, err := EncryptString(k, [12]byte(testvec.Hex(t, c.nonce)), fieldOf(t, c.field), string(testvec.Hex(t, c.plain)))
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if hex.EncodeToString(got) != c.ct {
			t.Errorf("%d (%s): ciphertext %x", i, c.field, got)
		}
		back, err := DecryptString(k, got)
		// Trailing NULs of the value are indistinguishable from padding.
		want := string(bytes.TrimRight(testvec.Hex(t, c.plain), "\x00"))
		if err != nil || back != want {
			t.Errorf("%d: decrypt %q %v", i, back, err)
		}
	}
}

func TestDecryptQuirks(t *testing.T) {
	v := load(t)
	k1, k2 := key(t, v, "k1"), key(t, v, "k2")
	var first []byte
	for _, c := range v.Quirks {
		switch {
		case c.InputLength > 0:
			if _, err := DecryptString(k1, make([]byte, c.InputLength)); !errors.Is(err, ErrDecrypt) {
				t.Errorf("%s: %v", c.Case, err)
			}
		case c.Ciphertext == "":
			if _, err := DecryptString(k2, first); !errors.Is(err, ErrDecrypt) {
				t.Errorf("%s: %v", c.Case, err)
			}
		default:
			ct := testvec.Hex(t, c.Ciphertext)
			if first == nil {
				first = ct
			}
			got, err := DecryptString(key(t, v, c.Key), ct)
			if err != nil || hex.EncodeToString([]byte(got)) != c.Decrypted {
				t.Errorf("%s: %q %v", c.Case, got, err)
			}
		}
	}
}

func TestBooleanFields(t *testing.T) {
	v := load(t)
	for _, c := range v.Bools {
		k := key(t, v, c.Key)
		ct := testvec.Hex(t, c.Ciphertext)
		got, err := DecryptBool(k, ct)
		if err != nil || got != c.Value {
			t.Errorf("%x: %v %v, want %v", ct, got, err, c.Value)
		}
		if c.Nonce == "" {
			continue
		}
		if enc := EncryptBool(k, [12]byte(testvec.Hex(t, c.Nonce)), c.Value); !bytes.Equal(enc, ct) && len(ct) == 29 {
			// Only the canonical 0x00/0x01 cases are reproducible.
			if b, _ := DecryptBool(k, enc); b != c.Value {
				t.Errorf("encrypt %v", c.Value)
			}
		}
	}
}

func TestPaymentAddress(t *testing.T) {
	v := load(t)
	p := v.Payment
	k := key(t, v, p.Key)
	ik, err := curve25519.ParsePublicKey(testvec.Hex(t, p.Identity))
	if err != nil {
		t.Fatal(err)
	}
	addr, err := DecryptPaymentAddress(k, testvec.Hex(t, p.Ciphertext), ik)
	if err != nil || hex.EncodeToString(addr) != p.Address {
		t.Fatalf("decrypt %x %v", addr, err)
	}
	// Our own encryption: signature differs (random nonce) but verifies.
	priv, _ := curve25519.NewPrivateKey(testvec.Hex(t, p.Private))
	ct, err := EncryptPaymentAddress(k, [12]byte(testvec.Hex(t, p.Nonce)), priv, testvec.Hex(t, p.Address), nil)
	if err != nil || len(ct) != 582 {
		t.Fatalf("encrypt %d %v", len(ct), err)
	}
	if back, err := DecryptPaymentAddress(k, ct, ik); err != nil || hex.EncodeToString(back) != p.Address {
		t.Errorf("round trip %x %v", back, err)
	}
	other, _ := curve25519.GeneratePrivateKey(rand.Reader)
	if back, err := DecryptPaymentAddress(k, ct, other.PublicKey()); err != nil || back != nil {
		t.Errorf("wrong identity: %x %v", back, err)
	}
}

func TestUnidentifiedAccessVerifier(t *testing.T) {
	v := load(t)
	k := key(t, v, v.Access.Key)
	if got := hex.EncodeToString(Verifier(k)); got != v.Access.Verifier {
		t.Errorf("verifier %s", got)
	}
}

func TestAvatar(t *testing.T) {
	v := load(t)
	for _, c := range v.Avatar {
		if c.Ciphertext == "" {
			continue
		}
		k := key(t, v, c.Key)
		ct := testvec.Hex(t, c.Ciphertext)
		if got := EncryptAvatar(k, [12]byte(testvec.Hex(t, c.Nonce)), testvec.Hex(t, c.Plaintext)); !bytes.Equal(got, ct) {
			t.Errorf("encrypt %x", got)
		}
		got, err := DecryptAvatar(k, ct)
		if err != nil || !bytes.Equal(got, testvec.Hex(t, c.Plaintext)) {
			t.Errorf("decrypt %v", err)
		}
		bad := bytes.Clone(ct)
		bad[len(bad)-1] ^= 1
		if _, err := DecryptAvatar(k, bad); err == nil {
			t.Error("tampered tag accepted")
		}
	}
}

func strp(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestDecodeProfile(t *testing.T) {
	v := load(t)
	k := key(t, v, "k1")
	for _, c := range v.Decode {
		resp, err := ParseResponse([]byte(c.Response))
		if err != nil {
			t.Fatalf("%s: parse %v", c.Case, err)
		}
		got, err := Decode(k, resp)
		if c.Result == nil {
			if !errors.Is(err, ErrDecrypt) {
				t.Errorf("%s: want decode failure, got %+v %v", c.Case, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Case, err)
			continue
		}
		w := c.Result
		mc := "<nil>"
		if got.MobileCoinAddress != nil {
			mc = hex.EncodeToString(got.MobileCoinAddress)
		}
		caps := got.Capabilities
		if caps == nil {
			caps = []string{}
		}
		wantCaps := w.Capabilities
		if wantCaps == nil {
			wantCaps = []string{}
		}
		if strp(got.GivenName) != strp(w.GivenName) || strp(got.FamilyName) != strp(w.FamilyName) ||
			strp(got.About) != strp(w.About) || strp(got.AboutEmoji) != strp(w.AboutEmoji) ||
			strp(got.AvatarURLPath) != strp(w.AvatarPath) || mc != strp(w.MobileCoin) ||
			got.AccessMode != w.Mode || !slices.Equal(caps, wantCaps) || strp(got.PhoneNumberSharing) != strp(w.Sharing) {
			t.Errorf("%s:\n got %s/%s/%s/%s avatar=%s mc=%s mode=%s caps=%v sharing=%s\nwant %s/%s/%s/%s avatar=%s mc=%s mode=%s caps=%v sharing=%s",
				c.Case, strp(got.GivenName), strp(got.FamilyName), strp(got.About), strp(got.AboutEmoji), strp(got.AvatarURLPath), mc, got.AccessMode, caps, strp(got.PhoneNumberSharing),
				strp(w.GivenName), strp(w.FamilyName), strp(w.About), strp(w.AboutEmoji), strp(w.AvatarPath), strp(w.MobileCoin), w.Mode, wantCaps, strp(w.Sharing))
		}
	}
}

func TestSplitNameAndDisplay(t *testing.T) {
	s := func(v string) *string { return &v }
	for _, c := range []struct {
		in            string
		given, family *string
	}{
		{"", s(""), nil}, {"Alice", s("Alice"), nil}, {"Alice\x00Smith", s("Alice"), s("Smith")},
		{"\x00Smith", s(""), s("Smith")}, {"a\x00b\x00c", s("a"), s("b")}, {"A\x00\x00B", s("A"), s("")},
	} {
		g, f := SplitName(c.in)
		if strp(g) != strp(c.given) || strp(f) != strp(c.family) {
			t.Errorf("%q: %s/%s", c.in, strp(g), strp(f))
		}
	}
	if DisplayName(s("A"), s("B")) != "A B" || DisplayName(nil, s("B")) != "B" || DisplayName(s(""), nil) != "" {
		t.Error("display name")
	}
}
