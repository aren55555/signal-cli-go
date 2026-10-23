package protocol

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// The tests check the provisioning and device-name ciphers against
// oracles written straight from spec/service/linking.md §3 and §4.5 with
// the standard library; no upstream vectors exist.

func oracleProvisionKeys(t *testing.T, shared []byte) (cipherKey, macKey []byte) {
	okm, err := hkdf.Key(sha256.New, shared, nil, "TextSecure Provisioning Message", 64)
	if err != nil {
		t.Fatal(err)
	}
	return okm[:32], okm[32:]
}

func oracleProvisionEncrypt(t *testing.T, shared, iv, msg []byte) []byte {
	ck, mk := oracleProvisionKeys(t, shared)
	pad := 16 - len(msg)%16
	pt := append(append([]byte{}, msg...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, _ := aes.NewCipher(ck)
	ct := make([]byte, len(pt))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, pt)
	body := append(append([]byte{0x01}, iv...), ct...)
	m := hmac.New(sha256.New, mk)
	m.Write(body)
	return m.Sum(body)
}

func TestProvisionCipher(t *testing.T) {
	secondary, _ := curve25519.GeneratePrivateKey(rand.Reader)
	msg := []byte("a ProvisionMessage, any length: 37 b")

	// Ours to the oracle.
	pub, body, err := EncryptProvision(rand.Reader, secondary.PublicKey(), msg)
	if err != nil {
		t.Fatal(err)
	}
	eph, err := curve25519.ParsePublicKey(pub)
	if err != nil || len(pub) != 33 {
		t.Fatalf("ephemeral key %x: %v", pub, err)
	}
	shared, _ := secondary.Agree(eph)
	if want := oracleProvisionEncrypt(t, shared, body[1:17], msg); !bytes.Equal(body, want) {
		t.Fatalf("body differs from the oracle\n%x\n%x", body, want)
	}

	// The oracle to ours.
	primaryEph, _ := curve25519.GeneratePrivateKey(rand.Reader)
	shared2, _ := primaryEph.Agree(secondary.PublicKey())
	body2 := oracleProvisionEncrypt(t, shared2, bytes.Repeat([]byte{7}, 16), msg)
	got, err := DecryptProvision(secondary, primaryEph.PublicKey().Bytes(), body2)
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("decrypt: %q, %v", got, err)
	}

	// Rejections (§3.5).
	flip := func(i int) []byte { b := append([]byte{}, body2...); b[i] ^= 1; return b }
	wrongVersion := append([]byte{}, body2...)
	wrongVersion[0] = 2
	for name, b := range map[string][]byte{
		"version":    wrongVersion,
		"iv":         flip(3),
		"ciphertext": flip(20),
		"mac":        flip(len(body2) - 1),
		"too short":  body2[:49],
		"empty":      nil,
	} {
		if _, err := DecryptProvision(secondary, primaryEph.PublicKey().Bytes(), b); !errors.Is(err, ErrProvisionDecrypt) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := DecryptProvision(secondary, nil, body2); !errors.Is(err, ErrProvisionDecrypt) {
		t.Errorf("missing public key: %v", err)
	}
	other, _ := curve25519.GeneratePrivateKey(rand.Reader)
	if _, err := DecryptProvision(other, primaryEph.PublicKey().Bytes(), body2); !errors.Is(err, ErrProvisionDecrypt) {
		t.Errorf("wrong recipient: %v", err)
	}
}

func oracleHMAC(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func TestDeviceName(t *testing.T) {
	identity, _ := curve25519.GeneratePrivateKey(rand.Reader)
	for _, name := range []string{"cli", "", "Ünïcødé laptop"} {
		enc, err := EncryptDeviceName(rand.Reader, identity.PublicKey(), name)
		if err != nil {
			t.Fatal(err)
		}
		var dn signalpb.DeviceName
		if err := proto.Unmarshal(enc, &dn); err != nil {
			t.Fatal(err)
		}
		// Oracle (§4.5).
		eph, err := curve25519.ParsePublicKey(dn.GetEphemeralPublic())
		if err != nil {
			t.Fatal(err)
		}
		master, _ := identity.Agree(eph)
		iv := oracleHMAC(oracleHMAC(master, []byte("auth")), []byte(name))[:16]
		key := oracleHMAC(oracleHMAC(master, []byte("cipher")), iv)
		block, _ := aes.NewCipher(key)
		ct := make([]byte, len(name))
		cipher.NewCTR(block, make([]byte, 16)).XORKeyStream(ct, []byte(name))
		if !bytes.Equal(dn.GetSyntheticIv(), iv) || !bytes.Equal(dn.GetCiphertext(), ct) {
			t.Errorf("%q: differs from the oracle", name)
		}
		got, err := DecryptDeviceName(identity, enc)
		if err != nil || got != name {
			t.Errorf("round trip %q: %q, %v", name, got, err)
		}
		dn.Ciphertext = append([]byte{}, dn.Ciphertext...)
		if len(dn.Ciphertext) > 0 {
			dn.Ciphertext[0] ^= 1
			bad, _ := proto.Marshal(&dn)
			if _, err := DecryptDeviceName(identity, bad); err == nil {
				t.Errorf("%q: tampered name accepted", name)
			}
		}
	}
}
