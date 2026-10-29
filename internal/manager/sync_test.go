// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

func TestSyncMemberPriority(t *testing.T) {
	cases := []struct {
		sm   *signalpb.AppSyncMessage
		want SyncMember
	}{
		{&signalpb.AppSyncMessage{Padding: []byte{1}}, SyncEmpty},
		{&signalpb.AppSyncMessage{DeleteForMe: []byte{1}}, SyncEmpty},
		{&signalpb.AppSyncMessage{Blocked: &signalpb.AppSyncMessage_Blocked{}, Read: []*signalpb.AppSyncMessage_Read{{}}}, SyncRead},
		{&signalpb.AppSyncMessage{FetchLatest: &signalpb.AppSyncMessage_FetchLatest{}, Keys: &signalpb.AppSyncMessage_Keys{}}, SyncKeys},
		{&signalpb.AppSyncMessage{MessageRequestResponse: &signalpb.AppMessageRequestResponse{}}, SyncEmpty},
		{&signalpb.AppSyncMessage{Contacts: &signalpb.AppSyncMessage_Contacts{}, Groups: &signalpb.AppSyncMessage_Groups{}}, SyncGroups},
		{&signalpb.AppSyncMessage{Sent: &signalpb.AppSent{}, Request: &signalpb.AppSyncMessage_Request{}}, SyncSent},
	}
	for i, c := range cases {
		if got := SyncMemberOf(c.sm); got != c.want {
			t.Errorf("case %d: %v, want %v", i, got, c.want)
		}
	}
}

func TestValidSync(t *testing.T) {
	bin, _ := uuidRaw(bobACI)
	env := envAt(10)
	sent := func(f func(*signalpb.AppSent)) *signalpb.AppSyncMessage {
		s := &signalpb.AppSent{Timestamp: proto.Uint64(10), DestinationServiceIdBinary: bin[:],
			Message: &signalpb.AppDataMessage{Timestamp: proto.Uint64(10), Body: proto.String("x")}}
		f(s)
		return &signalpb.AppSyncMessage{Sent: s}
	}
	cases := []struct {
		name   string
		sm     *signalpb.AppSyncMessage
		sender string
		ok     bool
	}{
		{"sent ok", sent(func(*signalpb.AppSent) {}), testACI, true},
		{"from someone else", sent(func(*signalpb.AppSent) {}), bobACI, false},
		{"sent without timestamp", sent(func(s *signalpb.AppSent) { s.Timestamp = nil }), testACI, false},
		{"sent without destination", sent(func(s *signalpb.AppSent) { s.DestinationServiceIdBinary = nil }), testACI, false},
		{"remote delete without destination", sent(func(s *signalpb.AppSent) {
			s.DestinationServiceIdBinary = nil
			s.Message.Delete = &signalpb.AppDelete{TargetSentTimestamp: proto.Uint64(1)}
		}), testACI, true},
		{"inner timestamp differs", sent(func(s *signalpb.AppSent) { s.Message.Timestamp = proto.Uint64(11) }), testACI, false},
		{"bad unidentified status", sent(func(s *signalpb.AppSent) {
			s.UnidentifiedStatus = []*signalpb.AppUnidentifiedStatus{{Unidentified: proto.Bool(false)}}
		}), testACI, false},
		{"read without timestamp", &signalpb.AppSyncMessage{Read: []*signalpb.AppSyncMessage_Read{{SenderAciBinary: bin[:]}}}, testACI, false},
		{"read ok", &signalpb.AppSyncMessage{Read: []*signalpb.AppSyncMessage_Read{{SenderAciBinary: bin[:], Timestamp: proto.Uint64(1)}}}, testACI, true},
		{"blocked bad aci", &signalpb.AppSyncMessage{Blocked: &signalpb.AppSyncMessage_Blocked{AcisBinary: [][]byte{{1, 2}}}}, testACI, false},
		{"mrr without thread", &signalpb.AppSyncMessage{MessageRequestResponse: &signalpb.AppMessageRequestResponse{Type: signalpb.AppMessageRequestResponse_ACCEPT.Enum()}}, testACI, false},
		{"sticker op without pack", &signalpb.AppSyncMessage{StickerPackOperation: []*signalpb.AppSyncMessage_StickerPackOperation{{}}}, testACI, false},
	}
	for _, c := range cases {
		if got := validSync(c.sm, env, c.sender, testACI); got != c.ok {
			t.Errorf("%s: valid = %v", c.name, got)
		}
	}
}

func TestContactStream(t *testing.T) {
	bin, _ := uuidRaw(bobACI)
	in := []ContactRecord{
		{Details: &signalpb.AppContactDetails{Number: proto.String("+15555550101"), AciBinary: bin[:], Name: proto.String("Bob B"), ExpireTimer: proto.Uint32(0),
			Avatar: &signalpb.AppContactDetails_Avatar{ContentType: proto.String("image/png"), Length: proto.Uint32(3)}}, Avatar: []byte("abc")},
		// Only aciBinary: dropped on reading (§5.1 quirk).
		{Details: &signalpb.AppContactDetails{AciBinary: bin[:], Name: proto.String("")}},
		{Details: &signalpb.AppContactDetails{Aci: proto.String(bobACI)}},
	}
	b, err := EncodeContacts(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeContacts(b)
	if err != nil || len(out) != 2 {
		t.Fatalf("%v, %d records", err, len(out))
	}
	if out[0].Details.GetName() != "Bob B" || string(out[0].Avatar) != "abc" || out[0].Details.ExpireTimer == nil || out[1].Details.GetAci() != bobACI {
		t.Errorf("decoded %v", out)
	}
	// Cutting into the first record is an error.
	if recs, err := DecodeContacts(b[:len(b)-30]); err == nil {
		t.Errorf("truncated stream accepted: %d records", len(recs))
	}
}

func TestApplySync(t *testing.T) {
	w := newReceiveWorld(t)
	a, r := w.a, w.r
	bin, _ := uuidRaw(sealedBACI)
	a.Data.MultiDevice = false
	a.Recipients[sealedBACI] = store.Recipient{Number: "+15555550101"}
	key := make([]byte, 33)
	key[0] = 5
	a.Identities[sealedBACI] = store.Identity{Key: key, Trust: store.TrustedUnverified}

	r.applySync(context.Background(), &signalpb.AppSyncMessage{Blocked: &signalpb.AppSyncMessage_Blocked{Numbers: []string{"+15555550101"}}})
	if !a.Data.MultiDevice || !a.Recipients[sealedBACI].Blocked {
		t.Errorf("blocked: multi=%v rc=%+v", a.Data.MultiDevice, a.Recipients[sealedBACI])
	}
	// A list without B does not unblock.
	r.applySync(context.Background(), &signalpb.AppSyncMessage{Blocked: &signalpb.AppSyncMessage_Blocked{}})
	if !a.Recipients[sealedBACI].Blocked {
		t.Error("unblocked by a blocked list")
	}

	r.applySync(context.Background(), &signalpb.AppSyncMessage{Verified: &signalpb.AppVerified{DestinationAciBinary: bin[:], IdentityKey: key, State: signalpb.AppVerified_VERIFIED.Enum()}})
	if a.Identities[sealedBACI].Trust != store.TrustedVerified {
		t.Errorf("verified: %v", a.Identities[sealedBACI].Trust)
	}
	other := append([]byte{}, key...)
	other[1] = 1
	r.applySync(context.Background(), &signalpb.AppSyncMessage{Verified: &signalpb.AppVerified{DestinationAciBinary: bin[:], IdentityKey: other, State: signalpb.AppVerified_UNVERIFIED.Enum()}})
	if a.Identities[sealedBACI].Trust != store.TrustedVerified {
		t.Error("verified with a different key applied")
	}

	r.applySync(context.Background(), &signalpb.AppSyncMessage{Configuration: &signalpb.AppSyncMessage_Configuration{ReadReceipts: proto.Bool(false)}})
	if c := a.Data.Configuration; c.ReadReceipts == nil || *c.ReadReceipts || c.TypingIndicators != nil {
		t.Errorf("configuration %+v", c)
	}

	r.applySync(context.Background(), &signalpb.AppSyncMessage{Keys: &signalpb.AppSyncMessage_Keys{AccountEntropyPool: proto.String(strings.Repeat("b", 64))}})
	if a.Data.AccountEntropyPool != strings.Repeat("b", 64) {
		t.Error("keys")
	}

	pk := bytes.Repeat([]byte{7}, 32)
	r.applySync(context.Background(), &signalpb.AppSyncMessage{Sent: &signalpb.AppSent{Timestamp: proto.Uint64(5), DestinationServiceIdBinary: bin[:],
		Message: &signalpb.AppDataMessage{Timestamp: proto.Uint64(5), Body: proto.String("x"), ExpireTimer: proto.Uint32(30), ExpireTimerVersion: proto.Uint32(3), ProfileKey: pk}}})
	if rc := a.Recipients[sealedBACI]; rc.Timer != 30 || rc.TimerVersion != 3 || !bytes.Equal(a.Data.ProfileKey, pk) {
		t.Errorf("sent: rc=%+v own key %x", rc, a.Data.ProfileKey)
	}

	// Requests are answered by a primary only.
	r.applySync(context.Background(), &signalpb.AppSyncMessage{Request: &signalpb.AppSyncMessage_Request{Type: signalpb.AppSyncMessage_Request_CONTACTS.Enum()}})
	if len(r.pending.syncAnswers) != 0 {
		t.Error("a linked device queued a sync answer")
	}
	a.Data.DeviceID = 1
	r.applySync(context.Background(), &signalpb.AppSyncMessage{Request: &signalpb.AppSyncMessage_Request{Type: signalpb.AppSyncMessage_Request_KEYS.Enum()}})
	if !r.pending.syncAnswers[signalpb.AppSyncMessage_Request_KEYS] {
		t.Error("primary did not queue the KEYS answer")
	}
}

// TestSyncFromOthersDropped: a sync message whose source is not our own
// ACI produces no event and does not set the multi-device flag.
func TestSyncFromOthersDropped(t *testing.T) {
	w := newReceiveWorld(t)
	w.a.Data.MultiDevice = false
	w.push(t, "plain", &signalpb.AppContent{Body: &signalpb.AppContent_SyncMessage{SyncMessage: &signalpb.AppSyncMessage{
		Configuration: &signalpb.AppSyncMessage_Configuration{ReadReceipts: proto.Bool(true)}}}}, 1000)
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 0 || w.a.Data.MultiDevice || w.a.Data.Configuration.ReadReceipts != nil {
		t.Errorf("events %d, multi %v", len(w.events), w.a.Data.MultiDevice)
	}
}

func TestSendKeysCreatesPool(t *testing.T) {
	_, s := dataWorld(t)
	s.Acct.Data.AccountEntropyPool, s.Acct.Data.MediaRootBackupKey = "", nil
	s.Acct.Data.MultiDevice = false
	if err := s.SendKeys(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := s.Acct.Data.AccountEntropyPool
	if len(p) != 64 || strings.Trim(p, aepAlphabet) != "" || len(s.Acct.Data.MediaRootBackupKey) != 32 {
		t.Errorf("pool %q", p)
	}
}

// TestPrimarySyncToNewDevice: a primary with no session with its other
// device sends an empty list, learns the device from the 409 and
// delivers on the retry (sync.md §1, §7).
func TestPrimarySyncToNewDevice(t *testing.T) {
	fc, s := dataWorld(t)
	var a1 *store.Account
	for id, d := range fc.devices[testACI] {
		if id == 1 {
			a1 = d.acct
		}
	}
	fc.sender, fc.senderDev = testACI, 1
	s = &Sender{Acct: a1, Chat: fc}
	a1.Data.MultiDevice = true
	if err := s.SendSync(context.Background(), &signalpb.AppSyncMessage{Configuration: &signalpb.AppSyncMessage_Configuration{}}, false); err != nil {
		t.Fatal(err)
	}
	got := deliveredTo(t, fc, testACI)
	if len(got) != 1 || got[0].GetSyncMessage().GetConfiguration() == nil || fc.delivered[0].device != 2 {
		t.Fatalf("delivered %+v", fc.delivered)
	}
}
