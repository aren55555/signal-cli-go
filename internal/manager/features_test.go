package manager

import (
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

func envAt(ts uint64) *signalpb.ServiceEnvelope {
	return &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(ts)}
}

// TestValidation: content-features.md §7.1, stage V (dropped silently).
func TestValidation(t *testing.T) {
	good, _ := uuidRaw(bobACI)
	zero := make([]byte, 16)
	dm := func(f func(*signalpb.AppDataMessage)) *signalpb.AppContent {
		d := &signalpb.AppDataMessage{Timestamp: proto.Uint64(10), Body: proto.String("hello")}
		f(d)
		return &signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: d}}
	}
	cases := []struct {
		name string
		c    *signalpb.AppContent
		ok   bool
	}{
		{"plain", dm(func(*signalpb.AppDataMessage) {}), true},
		{"quote with zero author", dm(func(d *signalpb.AppDataMessage) {
			d.Quote = &signalpb.AppQuote{Id: proto.Uint64(1), AuthorAciBinary: zero}
		}), false},
		{"quote ok", dm(func(d *signalpb.AppDataMessage) {
			d.Quote = &signalpb.AppQuote{Id: proto.Uint64(1), AuthorAciBinary: good[:]}
		}), true},
		{"reaction without timestamp", dm(func(d *signalpb.AppDataMessage) {
			d.Reaction = &signalpb.AppReaction{Emoji: proto.String("x"), TargetAuthorAciBinary: good[:]}
		}), false},
		{"delete without timestamp", dm(func(d *signalpb.AppDataMessage) { d.Delete = &signalpb.AppDelete{} }), false},
		{"style range without length", dm(func(d *signalpb.AppDataMessage) {
			d.BodyRanges = []*signalpb.AppBodyRange{{Start: proto.Uint32(0), AssociatedValue: &signalpb.AppBodyRange_Style_{Style: signalpb.AppBodyRange_BOLD}}}
		}), false},
		{"range out of bounds", dm(func(d *signalpb.AppDataMessage) {
			d.BodyRanges = []*signalpb.AppBodyRange{{Start: proto.Uint32(3), Length: proto.Uint32(9), AssociatedValue: &signalpb.AppBodyRange_Style_{Style: signalpb.AppBodyRange_BOLD}}}
		}), false},
		{"poll with one option", dm(func(d *signalpb.AppDataMessage) {
			d.PollCreate = &signalpb.AppPollCreate{Question: proto.String("q"), AllowMultiple: proto.Bool(true), Options: []string{"a"}}
		}), false},
		{"poll ok", dm(func(d *signalpb.AppDataMessage) {
			d.PollCreate = &signalpb.AppPollCreate{Question: proto.String("q"), AllowMultiple: proto.Bool(true), Options: []string{"a", "b"}}
		}), true},
		{"vote with 11 indexes", dm(func(d *signalpb.AppDataMessage) {
			d.PollVote = &signalpb.AppPollVote{TargetAuthorAciBinary: good[:], TargetSentTimestamp: proto.Uint64(1), VoteCount: proto.Uint32(1), OptionIndexes: make([]uint32, 11)}
		}), false},
		{"pin without duration", dm(func(d *signalpb.AppDataMessage) {
			d.PinMessage = &signalpb.AppPinMessage{TargetAuthorAciBinary: good[:], TargetSentTimestamp: proto.Uint64(1)}
		}), false},
		{"protocol version 9", dm(func(d *signalpb.AppDataMessage) { d.RequiredProtocolVersion = proto.Uint32(9) }), false},
		{"edit with wrong inner timestamp", &signalpb.AppContent{Body: &signalpb.AppContent_EditMessage{EditMessage: &signalpb.AppEdit{
			TargetSentTimestamp: proto.Uint64(5), DataMessage: &signalpb.AppDataMessage{Timestamp: proto.Uint64(11)}}}}, false},
		{"edit ok", &signalpb.AppContent{Body: &signalpb.AppContent_EditMessage{EditMessage: &signalpb.AppEdit{
			TargetSentTimestamp: proto.Uint64(5), DataMessage: &signalpb.AppDataMessage{Timestamp: proto.Uint64(10)}}}}, true},
	}
	for _, c := range cases {
		if got := valid(c.c, envAt(10)); got != c.ok {
			t.Errorf("%s: valid = %v", c.name, got)
		}
	}
}

// TestTimerRule: §7.2 — a higher version replaces the timer; equal or
// lower versions change nothing; a first message creates the entry.
func TestTimerRule(t *testing.T) {
	a, _ := testAccount(t)
	apply := func(timer, version uint32, body bool) {
		d := &signalpb.AppDataMessage{Timestamp: proto.Uint64(1), ExpireTimerVersion: proto.Uint32(version)}
		if timer > 0 {
			d.ExpireTimer = proto.Uint32(timer)
		}
		if body {
			d.Body = proto.String("x")
		}
		applyTimer(a, bobACI, d)
	}
	apply(60, 1, true)
	if rc := a.Recipients[bobACI]; rc.Timer != 60 || rc.TimerVersion != 1 {
		t.Fatalf("created %+v", rc)
	}
	apply(0, 1, true) // a plain message without a timer: version 1 does not win
	if a.Recipients[bobACI].Timer != 60 {
		t.Error("timer cleared by an old version")
	}
	apply(0, 2, true)
	if rc := a.Recipients[bobACI]; rc.Timer != 0 || rc.TimerVersion != 2 {
		t.Errorf("higher version: %+v", rc)
	}
	apply(300, 3, false) // no body, no expiration flag: no change
	if a.Recipients[bobACI].Timer != 0 {
		t.Error("timer changed without a body or the flag")
	}
}

// TestPNIFlagAndSignature: §4.3-4.4.
func TestPNIFlagAndSignature(t *testing.T) {
	a, _ := testAccount(t)
	// Bob's own account, whose PNI signature we verify.
	b, _ := testAccountFor(t, "+15555550109", bobACI, "b0b0b0b0-0000-4000-8000-0000000000f9", 1, nil)
	a.Identities[bobACI] = store.Identity{Key: b.Data.ACIIdentity.Public}
	a.Identities["PNI:"+b.Data.PNI] = store.Identity{Key: b.Data.PNIIdentity.Public}
	sig, err := pniSignature(b)
	if err != nil {
		t.Fatal(err)
	}
	m := &Message{Sender: bobAddrFor(), ToPNI: true}
	c := &signalpb.AppContent{PniSignatureMessage: sig, Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{Timestamp: proto.Uint64(1)}}}
	applyPNI(a, m, c)
	rc := a.Recipients[bobACI]
	if !rc.NeedsPNISignature || !rc.PNIVerified || rc.PNI != b.Data.PNI {
		t.Errorf("after a verified signature to our PNI: %+v", rc)
	}
	m.ToPNI = false
	applyPNI(a, m, &signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{}}})
	if a.Recipients[bobACI].NeedsPNISignature {
		t.Error("flag not cleared by a message to our ACI")
	}
	// A forged signature verifies nothing.
	a2, _ := testAccount(t)
	a2.Identities[bobACI] = store.Identity{Key: b.Data.PNIIdentity.Public}
	a2.Identities["PNI:"+b.Data.PNI] = store.Identity{Key: b.Data.ACIIdentity.Public}
	applyPNI(a2, &Message{Sender: bobAddrFor()}, c)
	if a2.Recipients[bobACI].PNIVerified {
		t.Error("swapped keys verified")
	}
}
