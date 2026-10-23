package protocol

import (
	"io"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/kyber"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// GeneratePreKey makes a one-time EC prekey.
func GeneratePreKey(rand io.Reader, id uint32) (*PreKeyRecord, error) {
	k, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	return &PreKeyRecord{ID: id, publicKey: k.PublicKey().Bytes(), privateKey: k.Bytes()}, nil
}

// GenerateSignedPreKey makes a signed EC prekey: the identity key's
// XEd25519 signature covers the 33-byte serialized public key
// (spec/service/keys.md §2.1).
func GenerateSignedPreKey(rand io.Reader, identity *curve25519.PrivateKey, id uint32, now time.Time) (*SignedPreKeyRecord, error) {
	k, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	pub := k.PublicKey().Bytes()
	sig, err := identity.Sign(rand, pub)
	if err != nil {
		return nil, err
	}
	return &SignedPreKeyRecord{ID: id, Signature: sig, Timestamp: uint64(now.UnixMilli()), publicKey: pub, privateKey: k.Bytes()}, nil
}

// GenerateKyberPreKey makes a Kyber1024 prekey (one-time or last-resort),
// signed over its 1569-byte serialized public key.
func GenerateKyberPreKey(rand io.Reader, identity *curve25519.PrivateKey, id uint32, now time.Time) (*KyberPreKeyRecord, error) {
	dk, err := kyber.GenerateKey1024(rand)
	if err != nil {
		return nil, err
	}
	sk := NewKEMSecretKey(dk)
	pub := sk.PublicKey().Bytes()
	sig, err := identity.Sign(rand, pub)
	if err != nil {
		return nil, err
	}
	return &KyberPreKeyRecord{ID: id, Signature: sig, Timestamp: uint64(now.UnixMilli()), publicKey: pub, privateKey: sk.Bytes()}, nil
}

func mustMarshal(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

// PublicKey returns the 33-byte serialized public key.
func (r *PreKeyRecord) PublicKey() []byte { return r.publicKey }

// Serialize returns the stored form, readable by LoadPreKeyRecord.
func (r *PreKeyRecord) Serialize() []byte {
	return mustMarshal(&signalpb.StoredPreKey{Id: r.ID, PublicKey: r.publicKey, PrivateKey: r.privateKey})
}

// PublicKey returns the 33-byte serialized public key.
func (r *SignedPreKeyRecord) PublicKey() []byte { return r.publicKey }

// Serialize returns the stored form, readable by LoadSignedPreKeyRecord.
func (r *SignedPreKeyRecord) Serialize() []byte {
	return mustMarshal(&signalpb.StoredSignedPreKey{Id: r.ID, PublicKey: r.publicKey, PrivateKey: r.privateKey, Signature: r.Signature, Timestamp: r.Timestamp})
}

// PublicKey returns the 1569-byte serialized public key.
func (r *KyberPreKeyRecord) PublicKey() []byte { return r.publicKey }

// Serialize returns the stored form, readable by LoadKyberPreKeyRecord.
func (r *KyberPreKeyRecord) Serialize() []byte {
	return mustMarshal(&signalpb.StoredSignedPreKey{Id: r.ID, PublicKey: r.publicKey, PrivateKey: r.privateKey, Signature: r.Signature, Timestamp: r.Timestamp})
}
