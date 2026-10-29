// SPDX-License-Identifier: GPL-3.0-or-later

// Package attachment implements attachment encryption, pointers, file
// naming, data URIs and long text (spec/service/attachments.md).
package attachment

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"math/big"
)

// buckets are the padded sizes above 541: floor(1.05^k), from 541 up to
// the last one below 2^31 (§2.2).
var buckets = func() []int64 {
	out := []int64{541}
	num, den := big.NewInt(1), big.NewInt(1)
	q := new(big.Int)
	for {
		num.Mul(num, big.NewInt(21))
		den.Mul(den, big.NewInt(20))
		v := q.Quo(num, den).Int64()
		if v >= 1<<31 {
			return out
		}
		if v > out[len(out)-1] {
			out = append(out, v)
		}
	}
}()

// PaddedSize is P(n), the size an n-byte attachment is padded to with 0x00
// bytes before encryption (§2.2).
func PaddedSize(n int64) int64 {
	for _, b := range buckets {
		if b >= n {
			return b
		}
	}
	return n
}

// CiphertextSize is C(p), the blob size for p padded plaintext bytes
// (§2.3).
func CiphertextSize(p int64) int64 { return 16 + 16*(p/16+1) + 32 }

// Encrypt pads plain, encrypts it under a fresh key and IV and returns the
// 64-byte key, the blob and its digest (§2.1-2.4).
func Encrypt(plain []byte) (key, blob, digest []byte, err error) {
	key, err = randomBytes(64)
	if err != nil {
		return nil, nil, nil, err
	}
	iv, err := randomBytes(16)
	if err != nil {
		return nil, nil, nil, err
	}
	blob, digest = encrypt(key, iv, plain, true)
	return key, blob, digest, nil
}

// encrypt is IV ‖ AES-256-CBC-PKCS7 ‖ HMAC-SHA256, with or without the
// 0x00 size padding; digest is SHA-256 of the whole blob.
func encrypt(key, iv, plain []byte, pad bool) (blob, digest []byte) {
	n := int64(len(plain))
	p := n
	if pad {
		p = PaddedSize(n)
	}
	k := 16 - int(p%16)
	data := make([]byte, p+int64(k))
	copy(data, plain)
	for i := p; i < int64(len(data)); i++ {
		data[i] = byte(k)
	}
	block, _ := aes.NewCipher(key[:32])
	blob = make([]byte, 16+len(data), 16+len(data)+32)
	copy(blob, iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(blob[16:], data)
	m := hmac.New(sha256.New, key[32:])
	m.Write(blob)
	blob = m.Sum(blob)
	d := sha256.Sum256(blob)
	return blob, d[:]
}

// verify checks the key size, length and MAC (§5.3 steps 2-3) and returns
// the CBC part.
func verify(key, blob []byte) ([]byte, error) {
	if len(key) != 64 {
		return nil, errors.New("Invalid combined key material size")
	}
	if len(blob) <= 48 {
		return nil, errors.New("Message shorter than crypto overhead!")
	}
	m := hmac.New(sha256.New, key[32:])
	m.Write(blob[:len(blob)-32])
	if !hmac.Equal(m.Sum(nil), blob[len(blob)-32:]) {
		return nil, errors.New("MAC doesn't match!")
	}
	body := blob[16 : len(blob)-32]
	if len(body)%16 != 0 {
		return nil, errors.New("Invalid ciphertext length")
	}
	return body, nil
}

func decryptCBC(key, blob, body []byte) []byte {
	block, _ := aes.NewCipher(key[:32])
	out := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, blob[:16]).CryptBlocks(out, body)
	return out
}

// Decrypt verifies a downloaded blob (MAC, then digest) and returns its
// first size plaintext bytes (§5.3). A size beyond the padded plaintext
// stops at the padded end.
func Decrypt(key, blob, digest []byte, size uint32) ([]byte, error) {
	if len(digest) == 0 {
		return nil, errors.New("Attachment pointer has no digest.")
	}
	body, err := verify(key, blob)
	if err != nil {
		return nil, err
	}
	if d := sha256.Sum256(blob); !bytes.Equal(d[:], digest) {
		return nil, errors.New("Digest doesn't match!")
	}
	out := decryptCBC(key, blob, body)
	if int64(size) <= int64(len(out))-16 {
		return out[:size], nil
	}
	// Reading to the end: PKCS#7 is checked and removed.
	out, err = unpad(out)
	if err != nil {
		return nil, err
	}
	if int64(size) < int64(len(out)) {
		out = out[:size]
	}
	return out, nil
}

func unpad(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("Bad padding")
	}
	k := int(b[len(b)-1])
	if k == 0 || k > 16 || k > len(b) {
		return nil, errors.New("Bad padding")
	}
	for _, c := range b[len(b)-k:] {
		if int(c) != k {
			return nil, errors.New("Bad padding")
		}
	}
	return b[:len(b)-k], nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}
