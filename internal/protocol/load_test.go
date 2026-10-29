// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type recordVector struct {
	Kind, Name, Record, Expected string
	RecordType                   string          `json:"record_type"`
	HasSenderChain               map[string]bool `json:"expected_has_sender_chain"`
	Ratio                        float64         `json:"require_pq_ratio"`
	Prefix                       string          `json:"alice_base_key_prefix"`
	UseNonPQ                     bool            `json:"expected_use_nonpq_session"`
	Parsed                       map[string]any  `json:"expected_parsed"`
}

func loadRecords(t *testing.T) []recordVector {
	var f struct{ Vectors []recordVector }
	testvec.Load(t, "ratchet_records.json", &f)
	return f.Vectors
}

func TestSessionUsabilityVectors(t *testing.T) {
	n := 0
	for _, v := range loadRecords(t) {
		switch v.Kind {
		case "session_usability":
			rec, err := LoadSessionRecord(testvec.Hex(t, v.Record))
			if err != nil {
				t.Fatalf("%s: %v", v.Name, err)
			}
			for ratio, want := range map[float64]bool{0: v.HasSenderChain["0.0"], 0.25: v.HasSenderChain["0.25"], 0.75: v.HasSenderChain["0.75"], 1: v.HasSenderChain["1.0"]} {
				if got := rec.HasUsableSenderChain(time.Unix(0, 0), ratio); got != want {
					t.Errorf("%s ratio %v: %v, want %v", v.Name, ratio, got, want)
				}
			}
			n++
		case "nonpq_session_ratio":
			base := append(testvec.Hex(t, v.Prefix), make([]byte, 29)...)
			if got := useNonPQSession(base, v.Ratio); got != v.UseNonPQ {
				t.Errorf("ratio %v prefix %s: %v, want %v", v.Ratio, v.Prefix, got, v.UseNonPQ)
			}
			n++
		}
	}
	if n != 12 {
		t.Errorf("ran %d vectors, want 12", n)
	}
}

func TestPreKeyRecordLoading(t *testing.T) {
	n := 0
	for _, v := range loadRecords(t) {
		if v.Kind != "prekey_record_parse" {
			continue
		}
		n++
		in := testvec.Hex(t, v.Record)
		var err, keyErr error
		switch v.RecordType {
		case "PreKeyRecord":
			var r *PreKeyRecord
			if r, err = LoadPreKeyRecord(in); err == nil {
				_, keyErr = r.PrivateKey()
			}
		case "SignedPreKeyRecord":
			var r *SignedPreKeyRecord
			if r, err = LoadSignedPreKeyRecord(in); err == nil {
				_, keyErr = r.PrivateKey()
			}
		case "KyberPreKeyRecord":
			var r *KyberPreKeyRecord
			if r, err = LoadKyberPreKeyRecord(in); err == nil {
				_, keyErr = r.SecretKey()
			}
		}
		if strings.HasPrefix(v.Expected, "parse fails") {
			if err == nil {
				t.Errorf("%s: parsed", v.Name)
			}
			continue
		}
		if err != nil || keyErr == nil {
			t.Errorf("%s: parse %v, key %v (want parse ok, key error)", v.Name, err, keyErr)
		}
	}
	if n != 6 {
		t.Errorf("ran %d prekey vectors", n)
	}
}

func TestCachedEnvelope(t *testing.T) {
	env, _ := proto.Marshal(&signalpb.ServiceEnvelope{Type: proto.Uint32(6), Content: []byte{1, 2}, ServerTimestamp: proto.Uint64(77)})
	file := binary.BigEndian.AppendUint32(nil, 9)
	file = binary.BigEndian.AppendUint64(file, 1234)
	file = append(file, env...)
	delivered, e, err := ParseCachedEnvelope(file)
	if err != nil || delivered != 1234 || e.GetType() != 6 || e.GetServerTimestamp() != 77 || !bytes.Equal(e.Content, []byte{1, 2}) {
		t.Fatalf("parsed %d %v %v", delivered, e, err)
	}
	old := bytes.Clone(file)
	binary.BigEndian.PutUint32(old, 8)
	if _, _, err := ParseCachedEnvelope(old); err == nil {
		t.Error("version 8 cache file accepted")
	}
	if _, _, err := ParseCachedEnvelope(file[:11]); err == nil {
		t.Error("truncated header accepted")
	}
}

// TestSessionRoundTripThroughStorage: a live session written in
// libsignal's storage format (by a test-only exporter) and loaded back
// keeps working in both directions, including a late message on an old
// chain, the SPQR state and the pending prekey info.
func TestSessionRoundTripThroughStorage(t *testing.T) {
	d := newDuo(t, 59)
	typ, w := d.aliceSend(t, "one", t0)
	if _, err := d.bobReceive(t, typ, w); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		d.aliceReceive(t, d.bobSend(t, "b"))
		typ, w = d.aliceSend(t, "a", t0)
		if _, err := d.bobReceive(t, typ, w); err != nil {
			t.Fatal(err)
		}
	}
	late := d.bobSend(t, "late")
	d.aliceReceive(t, d.bobSend(t, "newer"))

	reload := func(r *SessionRecord) *SessionRecord {
		t.Helper()
		b := exportRecordForTest(t, r)
		got, err := LoadSessionRecord(b)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		return got
	}
	d.alice, d.bob = reload(d.alice), reload(d.bob)
	if got := d.aliceReceive(t, late); got != "late" {
		t.Fatalf("late message after reload: %q", got)
	}
	for range 20 {
		d.aliceReceive(t, d.bobSend(t, "b"))
		typ, w = d.aliceSend(t, "a", t0)
		if _, err := d.bobReceive(t, typ, w); err != nil {
			t.Fatalf("after reload: %v", err)
		}
	}

	// A fresh, unacknowledged initiator record keeps sending prekey
	// messages after a reload.
	d2 := newDuo(t, 63)
	d2.alice = reload(d2.alice)
	if typ, w := d2.aliceSend(t, "x", t0); typ != MessageTypePreKey {
		t.Error("reloaded pending session doesn't send a prekey message")
	} else if got, err := d2.bobReceive(t, typ, w); err != nil || got != "x" {
		t.Errorf("bob: %q %v", got, err)
	}
}

// TestUndecodableSPQRStateFailsOnlyOnUse: libsignal decodes SPQR state
// lazily, so a record with bad SPQR bytes loads, and using it fails.
func TestUndecodableSPQRStateFailsOnlyOnUse(t *testing.T) {
	d := newDuo(t, 64)
	b := exportRecordForTest(t, d.alice)
	var pb signalpb.StoredSessionRecord
	_ = proto.Unmarshal(b, &pb)
	pb.Current.PqRatchetState = []byte{0x01}
	b, _ = proto.Marshal(&pb)
	rec, err := LoadSessionRecord(b)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, _, _, err := rec.Encrypt(d.rngA, []byte("x"), t0, aliceAddr, bobAddr); err == nil {
		t.Error("encrypt with undecodable SPQR state succeeded")
	}
	if _, err := LoadSessionRecord([]byte{0xff}); !errors.Is(err, ErrInvalidState) {
		t.Errorf("garbage record: %v", err)
	}
}

func TestSenderKeyRecordRoundTrip(t *testing.T) {
	g, _ := newGroup(t)
	g.recv(t, g.send(t, "a"), "a")
	skipped := g.send(t, "skipped")
	g.recv(t, g.send(t, "c"), "c")
	alice, err := LoadSenderKeyRecord(exportSenderKeyForTest(t, g.alice))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := LoadSenderKeyRecord(exportSenderKeyForTest(t, g.bob))
	if err != nil {
		t.Fatal(err)
	}
	g.alice, g.bob = alice, bob
	g.recv(t, skipped, "skipped")
	g.recv(t, g.send(t, "after"), "after")
}
