// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"testing"
)

func roundTrip(t *testing.T, r *SessionRecord) *SessionRecord {
	t.Helper()
	b, err := r.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	back, err := LoadSessionRecord(b)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := back.Serialize()
	if !bytes.Equal(again, b) {
		t.Fatal("Serialize(Load(Serialize(r))) differs from Serialize(r)")
	}
	return back
}

// TestSerializeDifferential runs the same conversation twice with the
// same randomness, once in memory and once storing and reloading both
// records after every step. Every wire message and plaintext must match:
// nothing the session needs may be lost in its stored form. The script
// covers PreKey and Whisper messages, out-of-order delivery (skipped
// keys), several ratchet turns and SPQR epochs.
func TestSerializeDifferential(t *testing.T) {
	type step struct {
		who  byte // 'a' alice sends, 'b' bob sends, 'A'/'B' deliver queued to alice/bob
		text string
	}
	var script []step
	for i := range 30 {
		switch i % 5 {
		case 0, 1:
			script = append(script, step{'a', "a" + string(rune('0'+i%10))})
		case 2:
			script = append(script, step{'B', ""})
		case 3:
			script = append(script, step{'b', "b" + string(rune('0'+i%10))})
		case 4:
			script = append(script, step{'A', ""})
		}
	}
	run := func(reload bool) []string {
		d := newDuo(t, 77)
		var log []string
		var toBob []struct {
			typ  int
			wire []byte
		}
		var toAlice [][]byte
		for _, s := range script {
			switch s.who {
			case 'a':
				typ, wire := d.aliceSend(t, s.text, t0)
				toBob = append(toBob, struct {
					typ  int
					wire []byte
				}{typ, wire})
				log = append(log, string(wire))
			case 'b':
				w := d.bobSend(t, s.text)
				toAlice = append(toAlice, w)
				log = append(log, string(w))
			case 'B': // deliver newest first, so older ones become skipped keys
				for i := len(toBob) - 1; i >= 0; i-- {
					got, err := d.bobReceive(t, toBob[i].typ, toBob[i].wire)
					if err != nil {
						t.Fatalf("bob receive: %v", err)
					}
					log = append(log, got)
					if reload {
						d.bob = roundTrip(t, d.bob) // between out-of-order deliveries
					}
				}
				toBob = nil
			case 'A':
				for _, w := range toAlice {
					log = append(log, d.aliceReceive(t, w))
				}
				toAlice = nil
			}
			if reload {
				d.alice = roundTrip(t, d.alice)
				if d.bob.current != nil {
					d.bob = roundTrip(t, d.bob)
				}
			}
		}
		return log
	}
	plain, reloaded := run(false), run(true)
	if len(plain) != len(reloaded) {
		t.Fatalf("%d vs %d events", len(plain), len(reloaded))
	}
	for i := range plain {
		if plain[i] != reloaded[i] {
			t.Fatalf("event %d differs after reloading", i)
		}
	}
}
