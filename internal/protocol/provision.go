package protocol

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// ErrProvisionDecrypt reports a provisioning message that fails any check
// of spec/service/linking.md §3.5.
var ErrProvisionDecrypt = errors.New("protocol: failed to decrypt provisioning message")

const (
	provisionInfo    = "TextSecure Provisioning Message"
	provisionVersion = 0x01
)

func provisionKeys(shared []byte) (cipherKey, macKey []byte) {
	okm := hkdfInfo(shared, provisionInfo, 64)
	return okm[:32], okm[32:]
}

// EncryptProvision encrypts a serialized ProvisionMessage to the
// secondary's public key (linking.md §3.4). It returns the ephemeral
// public key (33 bytes) and the body for a ProvisionEnvelope.
func EncryptProvision(rand io.Reader, recipient *curve25519.PublicKey, msg []byte) (publicKey, body []byte, err error) {
	eph, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, nil, err
	}
	shared, err := eph.Agree(recipient)
	if err != nil {
		return nil, nil, err
	}
	cipherKey, macKey := provisionKeys(shared)
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand, iv); err != nil {
		return nil, nil, err
	}
	ct, err := cbcEncrypt(cipherKey, iv, msg)
	if err != nil {
		return nil, nil, err
	}
	body = append(append([]byte{provisionVersion}, iv...), ct...)
	m := hmac.New(sha256.New, macKey)
	m.Write(body)
	return eph.PublicKey().Bytes(), m.Sum(body), nil
}

// DecryptProvision reverses EncryptProvision with the secondary's
// temporary private key (linking.md §3.5).
func DecryptProvision(priv *curve25519.PrivateKey, publicKey, body []byte) ([]byte, error) {
	if len(publicKey) == 0 || len(body)-1-aes.BlockSize-sha256.Size <= 0 || body[0] != provisionVersion {
		return nil, ErrProvisionDecrypt
	}
	eph, err := curve25519.ParsePublicKey(publicKey)
	if err != nil {
		return nil, ErrProvisionDecrypt
	}
	shared, err := priv.Agree(eph)
	if err != nil {
		return nil, ErrProvisionDecrypt
	}
	cipherKey, macKey := provisionKeys(shared)
	signed, tag := body[:len(body)-sha256.Size], body[len(body)-sha256.Size:]
	m := hmac.New(sha256.New, macKey)
	m.Write(signed)
	if !hmac.Equal(m.Sum(nil), tag) {
		return nil, ErrProvisionDecrypt
	}
	pt, err := cbcDecrypt(cipherKey, signed[1:1+aes.BlockSize], signed[1+aes.BlockSize:])
	if err != nil {
		return nil, ErrProvisionDecrypt
	}
	return pt, nil
}

// ErrDeviceName reports an encrypted device name that does not decrypt.
var ErrDeviceName = errors.New("protocol: invalid encrypted device name")

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// deviceNameKey and deviceNameIV derive the AES key and the synthetic IV
// (linking.md §4.5 steps 2-3).
func deviceNameKey(master, syntheticIV []byte) []byte {
	return hmacSHA256(hmacSHA256(master, []byte("cipher")), syntheticIV)
}

func deviceNameIV(master []byte, name []byte) []byte {
	return hmacSHA256(hmacSHA256(master, []byte("auth")), name)[:16]
}

// EncryptDeviceName encrypts name to the account's ACI identity key and
// returns the serialized DeviceName (linking.md §4.5).
func EncryptDeviceName(rand io.Reader, aciIdentity *curve25519.PublicKey, name string) ([]byte, error) {
	eph, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	master, err := eph.Agree(aciIdentity)
	if err != nil {
		return nil, err
	}
	iv := deviceNameIV(master, []byte(name))
	return proto.Marshal(&signalpb.DeviceName{
		EphemeralPublic: eph.PublicKey().Bytes(),
		SyntheticIv:     iv,
		Ciphertext:      aesCTR(deviceNameKey(master, iv), []byte(name)),
	})
}

// DecryptDeviceName decrypts a serialized DeviceName with the ACI identity
// private key, checking the synthetic IV.
func DecryptDeviceName(aciIdentity *curve25519.PrivateKey, serialized []byte) (string, error) {
	var dn signalpb.DeviceName
	if err := proto.Unmarshal(serialized, &dn); err != nil || len(dn.GetSyntheticIv()) != 16 {
		return "", ErrDeviceName
	}
	eph, err := curve25519.ParsePublicKey(dn.GetEphemeralPublic())
	if err != nil {
		return "", ErrDeviceName
	}
	master, err := aciIdentity.Agree(eph)
	if err != nil {
		return "", ErrDeviceName
	}
	name := aesCTR(deviceNameKey(master, dn.GetSyntheticIv()), dn.GetCiphertext())
	if !hmac.Equal(deviceNameIV(master, name), dn.GetSyntheticIv()) {
		return "", ErrDeviceName
	}
	return string(name), nil
}
