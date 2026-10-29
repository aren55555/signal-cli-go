// SPDX-License-Identifier: GPL-3.0-or-later

package interop

import "testing"

func TestBlock(t *testing.T) {
	plain := "Envelope from: x (device: 1) to y\nTimestamp: 5 (t)\nServer timestamps: received: 6 (t) delivered: 7 (t)\nBody: a\n  Stored plaintext in: /d/attachments/f.png\n\n" +
		"Envelope from: x (device: 1) to y\nTimestamp: 8 (t)\nBody: b\n\n"
	if got, want := Block([]byte(plain), false, "5"), "Envelope from: x (device: 1) to y\nTimestamp: 5 (t)\nServer timestamps: received: 6 (t) delivered: <T>\nBody: a\n  Stored plaintext in: <DATA>/attachments/f.png"; got != want {
		t.Errorf("plain:\n%q\nwant\n%q", got, want)
	}
	if Block([]byte(plain), false, "9") != "" {
		t.Error("unknown timestamp matched")
	}
	js := `{"envelope":{"timestamp":5,"serverDeliveredTimestamp":7},"account":"a"}` + "\n" + `{"envelope":{"timestamp":8}}`
	if got := Block([]byte(js), true, "5"); got != `{"envelope":{"timestamp":5,"serverDeliveredTimestamp":0},"account":"a"}` {
		t.Errorf("json: %s", got)
	}
}
