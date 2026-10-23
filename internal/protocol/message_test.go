package protocol

import (
	"bytes"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

func testKeys(t *testing.T) (a, b, ratchet *curve25519.PrivateKey) {
	t.Helper()
	r := testRand(40)
	a, _ = curve25519.GeneratePrivateKey(r)
	b, _ = curve25519.GeneratePrivateKey(r)
	ratchet, _ = curve25519.GeneratePrivateKey(r)
	return a, b, ratchet
}

var testMACKey = bytes.Repeat([]byte{0x6d}, 32)

func sampleRatchetMessage(ratchet *curve25519.PrivateKey) *ratchetMessage {
	return &ratchetMessage{
		version:         4,
		ratchetKey:      ratchet.PublicKey(),
		counter:         0,
		previousCounter: 7,
		ciphertext:      bytes.Repeat([]byte{0xc1}, 32),
	}
}

func TestRatchetMessageRoundTrip(t *testing.T) {
	a, b, ratchet := testKeys(t)
	m := sampleRatchetMessage(ratchet)
	m.pqRatchet = []byte{1, 2, 3}
	wire := encodeRatchetMessage(m, testMACKey, a.PublicKey(), b.PublicKey())

	if wire[0] != 0x44 {
		t.Errorf("version byte %#x, want 0x44", wire[0])
	}
	got, err := parseRatchetMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.version != 4 || got.counter != 0 || got.previousCounter != 7 ||
		!bytes.Equal(got.ratchetKey.Bytes(), m.ratchetKey.Bytes()) ||
		!bytes.Equal(got.ciphertext, m.ciphertext) || !bytes.Equal(got.pqRatchet, m.pqRatchet) {
		t.Errorf("parsed %+v", got)
	}
	if !got.verifyMAC(testMACKey, a.PublicKey(), b.PublicKey()) {
		t.Error("MAC does not verify")
	}
	// The MAC binds direction: swapping sender and receiver fails.
	if got.verifyMAC(testMACKey, b.PublicKey(), a.PublicKey()) {
		t.Error("MAC verifies with sender and receiver swapped")
	}
	// Any byte flip breaks parsing or the MAC.
	for i := range wire {
		bad := bytes.Clone(wire)
		bad[i] ^= 0x01
		p, err := parseRatchetMessage(bad)
		if err == nil && p.verifyMAC(testMACKey, a.PublicKey(), b.PublicKey()) {
			t.Fatalf("flipping byte %d still verifies", i)
		}
	}
}

// TestRatchetMessageEncoding: ascending field order, a zero counter written
// explicitly, pq_ratchet omitted when empty (double-ratchet.md §4).
func TestRatchetMessageEncoding(t *testing.T) {
	a, b, ratchet := testKeys(t)
	wire := encodeRatchetMessage(sampleRatchetMessage(ratchet), testMACKey, a.PublicKey(), b.PublicKey())
	body := wire[1 : len(wire)-macSize]
	var want []byte
	want = append(want, 0x0a, 33)
	want = append(want, ratchet.PublicKey().Bytes()...)
	want = append(want, 0x10, 0x00) // counter 0, present
	want = append(want, 0x18, 0x07) // previous counter 7
	want = append(want, 0x22, 32)
	want = append(want, bytes.Repeat([]byte{0xc1}, 32)...)
	if !bytes.Equal(body, want) {
		t.Errorf("body\n%x\nwant\n%x", body, want)
	}
}

func TestRatchetMessageParseErrors(t *testing.T) {
	a, b, ratchet := testKeys(t)
	good := encodeRatchetMessage(sampleRatchetMessage(ratchet), testMACKey, a.PublicKey(), b.PublicKey())

	withVersion := func(v byte) []byte { w := bytes.Clone(good); w[0] = v; return w }
	encodeWithout := func(clear func(*signalpb.RatchetMessage)) []byte {
		pb := &signalpb.RatchetMessage{
			RatchetKey: ratchet.PublicKey().Bytes(),
			Counter:    proto.Uint32(1),
			Ciphertext: []byte{1},
		}
		clear(pb)
		body, err := proto.Marshal(pb)
		if err != nil {
			t.Fatal(err)
		}
		return append(append([]byte{0x44}, body...), make([]byte, macSize)...)
	}
	for name, c := range map[string]struct {
		in  []byte
		err error
	}{
		"empty":            {nil, ErrMessageTooShort},
		"8 bytes":          {good[:8], ErrMessageTooShort},
		"version 2":        {withVersion(0x24), ErrLegacyVersion},
		"version 5":        {withVersion(0x54), ErrUnrecognizedVersion},
		"no ratchet key":   {encodeWithout(func(m *signalpb.RatchetMessage) { m.RatchetKey = nil }), ErrInvalidMessage},
		"no counter":       {encodeWithout(func(m *signalpb.RatchetMessage) { m.Counter = nil }), ErrInvalidMessage},
		"no ciphertext":    {encodeWithout(func(m *signalpb.RatchetMessage) { m.Ciphertext = nil }), ErrInvalidMessage},
		"bad ratchet key":  {encodeWithout(func(m *signalpb.RatchetMessage) { m.RatchetKey = []byte{0x06, 1} }), ErrInvalidMessage},
		"garbage protobuf": {append([]byte{0x44, 0xff, 0xff}, make([]byte, macSize)...), ErrInvalidMessage},
	} {
		if _, err := parseRatchetMessage(c.in); !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", name, err, c.err)
		}
	}
	// The low nibble is ignored: 0x34 (version 3) parses.
	if m, err := parseRatchetMessage(withVersion(0x34)); err != nil || m.version != 3 {
		t.Errorf("version 3: %v", err)
	}
}

// TestMACOverRawBytes: the MAC is checked over the bytes as received. A
// sender that writes a non-minimal tag still verifies, which would fail if
// the message were re-encoded first (protobuf-go normalizes tags).
func TestMACOverRawBytes(t *testing.T) {
	a, b, ratchet := testKeys(t)
	body := []byte{0x8a, 0x00, 33} // field 1, tag written as two bytes
	body = append(body, ratchet.PublicKey().Bytes()...)
	body = append(body, 0x10, 0x05, 0x22, 0x01, 0xaa)
	serialized := append([]byte{0x44}, body...)
	wire := append(bytes.Clone(serialized), messageMAC(testMACKey, a.PublicKey(), b.PublicKey(), serialized)...)
	m, err := parseRatchetMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !m.verifyMAC(testMACKey, a.PublicKey(), b.PublicKey()) {
		t.Error("MAC over the raw bytes does not verify")
	}
}

func samplePreKeyMessage(t *testing.T) (*preKeyMessage, []byte) {
	a, b, ratchet := testKeys(t)
	inner := encodeRatchetMessage(sampleRatchetMessage(ratchet), testMACKey, a.PublicKey(), b.PublicKey())
	otk := uint32(0) // 0 is a valid one-time prekey ID
	m := &preKeyMessage{
		version:         4,
		registrationID:  1234,
		oneTimePreKeyID: &otk,
		signedPreKeyID:  5,
		kyberPreKeyID:   6,
		kyberCiphertext: append([]byte{0x08}, bytes.Repeat([]byte{0x99}, 1568)...),
		baseKey:         ratchet.PublicKey(),
		identityKey:     a.PublicKey(),
		message:         inner,
	}
	return m, encodePreKeyMessage(m)
}

func TestPreKeyMessageRoundTrip(t *testing.T) {
	m, wire := samplePreKeyMessage(t)
	if wire[0] != 0x44 {
		t.Errorf("version byte %#x", wire[0])
	}
	got, err := parsePreKeyMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.registrationID != 1234 || got.oneTimePreKeyID == nil || *got.oneTimePreKeyID != 0 ||
		got.signedPreKeyID != 5 || got.kyberPreKeyID != 6 || !bytes.Equal(got.kyberCiphertext, m.kyberCiphertext) ||
		!bytes.Equal(got.baseKey.Bytes(), m.baseKey.Bytes()) || !bytes.Equal(got.identityKey.Bytes(), m.identityKey.Bytes()) {
		t.Errorf("parsed %+v", got)
	}
	if got.inner == nil || got.inner.previousCounter != 7 {
		t.Error("embedded message not parsed")
	}
	// No one-time prekey: field 1 absent, not 0.
	m.oneTimePreKeyID = nil
	got, err = parsePreKeyMessage(encodePreKeyMessage(m))
	if err != nil || got.oneTimePreKeyID != nil {
		t.Errorf("absent one-time prekey: %v %v", got.oneTimePreKeyID, err)
	}
}

func TestPreKeyMessageParseErrors(t *testing.T) {
	_, wire := samplePreKeyMessage(t)
	var pb signalpb.PreKeyRatchetMessage
	if err := proto.Unmarshal(wire[1:], &pb); err != nil {
		t.Fatal(err)
	}
	mutate := func(f func(*signalpb.PreKeyRatchetMessage), version byte) []byte {
		c := proto.Clone(&pb).(*signalpb.PreKeyRatchetMessage)
		f(c)
		body, err := proto.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		return append([]byte{version}, body...)
	}
	for name, c := range map[string]struct {
		in  []byte
		err error
	}{
		"empty":            {nil, ErrMessageTooShort},
		"version 2":        {mutate(func(*signalpb.PreKeyRatchetMessage) {}, 0x24), ErrLegacyVersion},
		"version 5":        {mutate(func(*signalpb.PreKeyRatchetMessage) {}, 0x54), ErrUnrecognizedVersion},
		"no base key":      {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.BaseKey = nil }, 0x44), ErrInvalidMessage},
		"no identity key":  {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.IdentityKey = nil }, 0x44), ErrInvalidMessage},
		"no message":       {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.RatchetMessage = nil }, 0x44), ErrInvalidMessage},
		"no signed prekey": {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.SignedPreKeyId = nil }, 0x44), ErrInvalidMessage},
		"v4 without kyber": {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.KyberPreKeyId = nil; m.KyberCiphertext = nil }, 0x44), ErrInvalidMessage},
		"kyber id only":    {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.KyberCiphertext = nil }, 0x44), ErrInvalidMessage},
		"kyber ct only":    {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.KyberPreKeyId = nil }, 0x44), ErrInvalidMessage},
		"bad inner":        {mutate(func(m *signalpb.PreKeyRatchetMessage) { m.RatchetMessage = []byte{0x44} }, 0x44), ErrMessageTooShort},
	} {
		if _, err := parsePreKeyMessage(c.in); !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", name, err, c.err)
		}
	}
	// A missing registration ID reads as 0.
	m, err := parsePreKeyMessage(mutate(func(m *signalpb.PreKeyRatchetMessage) { m.RegistrationId = nil }, 0x44))
	if err != nil || m.registrationID != 0 {
		t.Errorf("missing registration ID: %v", err)
	}
	// Version 3 without Kyber fields is accepted at parse time.
	if _, err := parsePreKeyMessage(mutate(func(m *signalpb.PreKeyRatchetMessage) { m.KyberPreKeyId = nil; m.KyberCiphertext = nil }, 0x34)); err != nil {
		t.Errorf("v3 without kyber: %v", err)
	}
}

// FuzzParseMessages: the parsers handle untrusted bytes. They must not
// panic, and a parsed message's raw MAC input is a prefix of the input.
func FuzzParseMessages(f *testing.F) {
	t := &testing.T{}
	_, pre := samplePreKeyMessage(t)
	a, b, ratchet := testKeys(t)
	f.Add(encodeRatchetMessage(sampleRatchetMessage(ratchet), testMACKey, a.PublicKey(), b.PublicKey()))
	f.Add(pre)
	f.Add([]byte{0x44})
	f.Fuzz(func(t *testing.T, in []byte) {
		if m, err := parseRatchetMessage(in); err == nil {
			if !bytes.HasPrefix(in, m.serialized) || len(m.mac) != macSize {
				t.Fatalf("serialized/mac don't partition the input")
			}
		}
		if m, err := parsePreKeyMessage(in); err == nil && m.inner == nil {
			t.Fatal("prekey message without a parsed inner message")
		}
	})
}
