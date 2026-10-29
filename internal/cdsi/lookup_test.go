// SPDX-License-Identifier: GPL-3.0-or-later

package cdsi

import (
	"bytes"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

// fakeChannel plays a scripted server.
type fakeChannel struct {
	sent    [][]byte
	replies []any // []byte message or error
}

func (f *fakeChannel) Send(b []byte) error { f.sent = append(f.sent, b); return nil }
func (f *fakeChannel) Recv() ([]byte, error) {
	if len(f.replies) == 0 {
		return nil, &websocket.CloseError{Code: 1000}
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	if err, ok := r.(error); ok {
		return nil, err
	}
	return r.([]byte), nil
}

func TestLookupExchange(t *testing.T) {
	v := load(t)
	tokenMsg := mustHex(t, v.TokenResp.Encoded)
	final := mustHex(t, v.Final.Encoded)
	ch := &fakeChannel{replies: []any{tokenMsg, final}}
	var token []byte
	acked := false
	resp, err := Lookup(ch, &Request{New: []string{"+18005551001"}}, func(tk []byte) {
		token = tk
		acked = len(ch.sent) > 1
	})
	if err != nil {
		t.Fatal(err)
	}
	if token == nil || acked {
		t.Errorf("token handed over after the ack (or not at all)")
	}
	if len(ch.sent) != 2 || !bytes.Equal(ch.sent[1], []byte{0x38, 0x01}) {
		t.Errorf("sent %x", ch.sent)
	}
	if res, _ := resp.Results(); len(res) != 2 {
		t.Errorf("results %v", res)
	}

	// Rate limit after the ack: the token was still handed over.
	ch = &fakeChannel{replies: []any{tokenMsg, &websocket.CloseError{Code: 4008, Reason: `{"retry_after": 513}`}}}
	token = nil
	_, err = Lookup(ch, &Request{New: []string{"+18005551001"}}, func(tk []byte) { token = tk })
	var ce *CloseError
	if !errors.As(err, &ce) || ce.RetryAfter != 513 || token == nil {
		t.Errorf("4008: %v token=%x", err, token)
	}
	// Invalid token on the first reply.
	ch = &fakeChannel{replies: []any{&websocket.CloseError{Code: 4101}}}
	if _, err := Lookup(ch, &Request{New: []string{"+1"}}, nil); !errors.As(err, &ce) || !ce.IsInvalidToken() {
		t.Errorf("4101: %v", err)
	}
	// Empty token.
	ch = &fakeChannel{replies: []any{[]byte{}}}
	if _, err := Lookup(ch, &Request{New: []string{"+1"}}, nil); err == nil {
		t.Error("empty token accepted")
	}
}

func mustHex(t *testing.T, s string) []byte {
	b, err := hexDecode(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
