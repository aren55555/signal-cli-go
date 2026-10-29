// SPDX-License-Identifier: GPL-3.0-or-later

// Package storage holds the storage-service keys and encryption
// (spec/service/storage.md §2, §3). The sync itself is in the manager.
package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
)

// ErrDecrypt is a decryption failure (bad tag or too short, §3).
var ErrDecrypt = errors.New("storage: decryption failed")

// MasterKey derives the master key from an account entropy pool (§2.1).
func MasterKey(aep string) [32]byte {
	k, _ := hkdf.Key(sha256.New, []byte(aep), nil, "20240801_SIGNAL_SVR_MASTER_KEY", 32)
	return [32]byte(k)
}

func hmacSHA256(key []byte, msg string) [32]byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return [32]byte(m.Sum(nil))
}

// StorageKey is the storage service key of a master key.
func StorageKey(master [32]byte) [32]byte { return hmacSHA256(master[:], "Storage Service Encryption") }

// ManifestKey is the key of manifest version v.
func ManifestKey(storageKey [32]byte, v uint64) [32]byte {
	return hmacSHA256(storageKey[:], "Manifest_"+strconv.FormatUint(v, 10))
}

// ItemKey is the key of an item: the recordIkm scheme when the manifest
// has a recordIkm, else the legacy scheme.
func ItemKey(storageKey [32]byte, recordIkm, raw []byte) [32]byte {
	if len(recordIkm) > 0 {
		k, _ := hkdf.Key(sha256.New, recordIkm, nil, "20240801_SIGNAL_STORAGE_SERVICE_ITEM_"+string(raw), 32)
		return [32]byte(k)
	}
	return hmacSHA256(storageKey[:], "Item_"+base64.StdEncoding.EncodeToString(raw))
}

func gcm(key [32]byte) cipher.AEAD {
	b, _ := aes.NewCipher(key[:])
	a, _ := cipher.NewGCM(b)
	return a
}

// Seal encrypts plain under key with iv (12 bytes): iv ‖ ct ‖ tag.
func Seal(key [32]byte, iv, plain []byte) []byte {
	return gcm(key).Seal(append([]byte{}, iv...), iv, plain, nil)
}

// SealRandom is Seal with a fresh random IV.
func SealRandom(key [32]byte, plain []byte) []byte {
	iv := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}
	return Seal(key, iv, plain)
}

// Open decrypts a value sealed by Seal.
func Open(key [32]byte, value []byte) ([]byte, error) {
	if len(value) < 12+16 {
		return nil, ErrDecrypt
	}
	p, err := gcm(key).Open(nil, value[:12], value[12:], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return p, nil
}

// NewRawID is a fresh random 16-byte storage ID.
func NewRawID() []byte {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(err)
	}
	return b
}
