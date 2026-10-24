package attachment

import (
	"crypto/hkdf"
	"crypto/sha256"
)

// StickerKey derives the 64-byte blob key of a sticker pack from its
// 32-byte pack key (§6.6).
func StickerKey(packKey []byte) []byte {
	k, _ := hkdf.Key(sha256.New, packKey, nil, "Sticker Pack", 64)
	return k
}

// DecryptSticker verifies and decrypts a sticker or manifest blob: no
// digest, no size padding, PKCS#7 removed (§6.6).
func DecryptSticker(key, blob []byte) ([]byte, error) {
	body, err := verify(key, blob)
	if err != nil {
		return nil, err
	}
	return unpad(decryptCBC(key, blob, body))
}

// EncryptSticker encrypts a sticker or manifest under the pack key with a
// fresh IV, without size padding.
func EncryptSticker(key, plain []byte) ([]byte, error) {
	iv, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	blob, _ := encrypt(key, iv, plain, false)
	return blob, nil
}
