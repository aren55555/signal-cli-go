package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"errors"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// Labels and sizes from spec/protocol/double-ratchet.md §2, §3 and §5.
const (
	rootInfo       = "WhisperRatchet"
	messageKeyInfo = "WhisperMessageKeys"
	macSize        = 8
)

// rootStep is KDF_RK: HKDF-SHA256 with the root key as salt and the DH
// output as IKM, 64 bytes split into a new root key and a chain key.
func rootStep(rootKey, dhOut []byte) (newRoot, chainKey []byte) {
	out, err := hkdf.Key(sha256.New, dhOut, rootKey, rootInfo, 64)
	if err != nil {
		panic(err) // 64 bytes is always within HKDF's limit
	}
	return out[:32], out[32:]
}

// chainStep is KDF_CK: the message-key seed is HMAC(ck, 0x01) and the next
// chain key HMAC(ck, 0x02).
func chainStep(ck []byte) (seed, next []byte) {
	h := hmac.New(sha256.New, ck)
	h.Write([]byte{0x01})
	seed = h.Sum(nil)
	h.Reset()
	h.Write([]byte{0x02})
	return seed, h.Sum(nil)
}

type messageKeys struct {
	cipherKey, macKey, iv []byte
}

// deriveMessageKeys expands a seed with HKDF-SHA256, info
// "WhisperMessageKeys", 80 bytes split cipher key | MAC key | IV. The SPQR
// key for the message, if any, is the salt; nil means no salt.
func deriveMessageKeys(seed, spqrKey []byte) messageKeys {
	out, err := hkdf.Key(sha256.New, seed, spqrKey, messageKeyInfo, 80)
	if err != nil {
		panic(err)
	}
	return messageKeys{cipherKey: out[:32], macKey: out[32:64], iv: out[64:]}
}

// cbcEncrypt is AES-256-CBC with PKCS#7 padding (a full block when the
// plaintext is a multiple of 16 bytes).
func cbcEncrypt(key, iv, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	buf := make([]byte, len(plaintext)+pad)
	copy(buf, plaintext)
	for i := len(plaintext); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)
	return buf, nil
}

var errBadCiphertext = errors.New("protocol: invalid ciphertext")

// cbcDecrypt reverses cbcEncrypt. It rejects empty input, input that isn't
// a multiple of the block size, and bad padding. The MAC is always checked
// before this runs, so padding errors are no oracle.
func cbcDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errBadCiphertext
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pt := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ciphertext)
	pad := int(pt[len(pt)-1])
	if pad == 0 || pad > aes.BlockSize {
		return nil, errBadCiphertext
	}
	for _, b := range pt[len(pt)-pad:] {
		if int(b) != pad {
			return nil, errBadCiphertext
		}
	}
	return pt[:len(pt)-pad], nil
}

// messageMAC is the SignalMessage MAC: HMAC-SHA256 over the sender's and
// receiver's 33-byte identity keys and the serialized message (version
// byte and protobuf), truncated to 8 bytes.
func messageMAC(macKey []byte, sender, receiver *curve25519.PublicKey, serialized []byte) []byte {
	h := hmac.New(sha256.New, macKey)
	h.Write(sender.Bytes())
	h.Write(receiver.Bytes())
	h.Write(serialized)
	return h.Sum(nil)[:macSize]
}
