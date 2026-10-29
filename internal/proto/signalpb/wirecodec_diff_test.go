// SPDX-License-Identifier: GPL-3.0-or-later

package signalpb

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"google.golang.org/protobuf/proto"
)

// Differential tests: the generated code (protobuf-go) against the old
// hand-written codec in wirecodec_test.go.

func randBytes(r *rand.Rand, max int) []byte {
	b := make([]byte, r.IntN(max+1))
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}

// TestGeneratedEncodingMatchesOldCodec encodes random messages with
// protobuf-go and checks every field the old codec parses back.
func TestGeneratedEncodingMatchesOldCodec(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		m := &PreKeyRatchetMessage{
			BaseKey:         randBytes(r, 40),
			IdentityKey:     randBytes(r, 40),
			RatchetMessage:  randBytes(r, 200),
			RegistrationId:  proto.Uint32(r.Uint32()),
			SignedPreKeyId:  proto.Uint32(r.Uint32()),
			KyberPreKeyId:   proto.Uint32(r.Uint32()),
			KyberCiphertext: randBytes(r, 1600),
		}
		if r.IntN(2) == 0 {
			m.OneTimePreKeyId = proto.Uint32(r.Uint32() >> r.IntN(32))
		}
		b, err := proto.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		got := map[Number]Field{}
		var raw []byte
		for f, err := range Fields(b) {
			if err != nil {
				t.Fatalf("old codec rejects protobuf-go output: %v", err)
			}
			got[f.Num] = f
			raw = append(raw, f.Raw...)
		}
		if !bytes.Equal(raw, b) {
			t.Fatal("old codec's raw fields don't reassemble the encoding")
		}
		checkBytes := func(n Number, want []byte) {
			if f := got[n]; f.Type != BytesType || !bytes.Equal(f.Bytes, want) {
				t.Fatalf("field %d: %+v, want bytes %x", n, f, want)
			}
		}
		checkVarint := func(n Number, want *uint32) {
			f, ok := got[n]
			if want == nil {
				if ok {
					t.Fatalf("field %d present, want absent", n)
				}
				return
			}
			if f.Type != VarintType || f.Value != uint64(*want) {
				t.Fatalf("field %d: %+v, want %d", n, f, *want)
			}
		}
		checkVarint(1, m.OneTimePreKeyId)
		checkBytes(2, m.BaseKey)
		checkBytes(3, m.IdentityKey)
		checkBytes(4, m.RatchetMessage)
		checkVarint(5, m.RegistrationId)
		checkVarint(6, m.SignedPreKeyId)
		checkVarint(7, m.KyberPreKeyId)
		checkBytes(8, m.KyberCiphertext)
	}

	// A fixed64 and a oneof, via the sender-certificate body.
	body := &SenderCert_Body{
		DeviceId:    proto.Uint32(3),
		Expires:     proto.Uint64(0x0102030405060708),
		IdentityKey: []byte{5, 1},
		Signer:      &SenderCert_Body_KnownServerCertId{KnownServerCertId: 0x7357c357},
		SenderAci:   &SenderCert_Body_AciString{AciString: "aaaaaaaa-7000-11eb-b32a-33b8a8a487a6"},
	}
	b, err := proto.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for f, err := range Fields(b) {
		if err != nil {
			t.Fatal(err)
		}
		switch f.Num {
		case 3:
			if f.Type != Fixed64Type || f.Value != 0x0102030405060708 {
				t.Errorf("expires: %+v", f)
			}
		case 6:
			if f.Type != BytesType || string(f.Bytes) != body.GetAciString() {
				t.Errorf("aci: %+v", f)
			}
		case 8:
			if f.Type != VarintType || f.Value != 0x7357c357 {
				t.Errorf("known cert id: %+v", f)
			}
		}
		seen++
	}
	if seen != 5 {
		t.Errorf("old codec saw %d fields, want 5", seen)
	}
}

// FuzzDecodersAgree: on arbitrary input, protobuf-go and the old codec
// accept or reject together. PlaintextContentBody is used because nearly
// all input is unknown fields to it, so both decoders do structural
// parsing only. The one known difference is group depth: the old codec
// stops at MaxDepth (100), protobuf-go much deeper.
func FuzzDecodersAgree(f *testing.F) {
	for _, s := range [][]byte{
		{0x42, 0x02, 0x10, 0x01},
		{0x0b, 0x08, 0x01, 0x0c},
		{0x08, 0x96, 0x01, 0x12, 0x00},
		{0x0c},
		{0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02},
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		var oldErr error
		var raw []byte
		for fl, err := range Fields(in) {
			if err != nil {
				oldErr = err
				break
			}
			raw = append(raw, fl.Raw...)
		}
		if errors.Is(oldErr, ErrGroup) && bytes.Count(in, []byte{0x0b}) > MaxDepth/2 {
			return // possibly the depth limit, where the decoders legitimately differ
		}
		var m PlaintextContentBody
		newErr := proto.Unmarshal(in, &m)
		if (oldErr == nil) != (newErr == nil) {
			t.Fatalf("input %x: old codec error %v, protobuf-go error %v", in, oldErr, newErr)
		}
		if oldErr != nil {
			return
		}
		// Without a field 8 (the one known field), everything is an unknown
		// field, and protobuf-go must re-encode the same fields in the same
		// order. Not byte for byte: it normalizes non-minimal tag varints
		// (found by this fuzz target, see testdata/fuzz). That is why MACs
		// and signatures are always checked over the raw received bytes,
		// never over a re-marshalled message.
		if m.DecryptionFailureReport == nil {
			out, err := proto.Marshal(&m)
			if err != nil {
				t.Fatal(err)
			}
			if a, b := fieldList(t, in), fieldList(t, out); a != b {
				t.Fatalf("input %x re-encodes as %x:\n%s\nvs\n%s", in, out, a, b)
			}
		}
	})
}

// fieldList renders the fields the old codec sees, ignoring how each tag
// was encoded. Group contents are compared recursively.
func fieldList(t *testing.T, b []byte) string {
	t.Helper()
	var s []byte
	for f, err := range Fields(b) {
		if err != nil {
			t.Fatalf("fieldList(%x): %v", b, err)
		}
		inner := ""
		switch f.Type {
		case StartGroupType:
			inner = "{" + fieldList(t, f.Bytes) + "}"
		case BytesType:
			inner = string(f.Bytes)
		}
		s = fmt.Appendf(s, "%d/%d/%d/%q;", f.Num, f.Type, f.Value, inner)
	}
	return string(s)
}
