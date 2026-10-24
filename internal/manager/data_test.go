package manager

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

const dataBACI = "b0b0b0b0-0000-4000-8000-000000000002"

func dataWorld(t *testing.T) (*fakeChat, *Sender) {
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a2, a2Bundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	a1, a1Bundles := testAccountFor(t, testNumber, testACI, testPNI, 1, aID)
	b1, b1Bundles := testAccountFor(t, "+15555550101", dataBACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	fc := &fakeChat{t: t, sender: testACI, senderDev: 2, devices: map[string]map[uint32]fakeDevice{
		dataBACI: {1: {b1, b1Bundles["aci"]}}, testACI: {1: {a1, a1Bundles["aci"]}, 2: {a2, a2Bundles["aci"]}}}}
	return fc, &Sender{Acct: a2, Chat: fc}
}

func deliveredTo(t *testing.T, fc *fakeChat, dest string) []*signalpb.AppContent {
	var out []*signalpb.AppContent
	for _, d := range fc.delivered {
		if d.to == dest {
			var c signalpb.AppContent
			proto.Unmarshal(d.msg.Content, &c)
			out = append(out, &c)
		}
	}
	return out
}

// TestDataEnrichment: §1.3-1.4 — timestamp, timer version, profile key,
// protocol version from a mention, the transcript to A.1, and profile
// sharing turned on.
func TestDataEnrichment(t *testing.T) {
	fc, s := dataWorld(t)
	s.Acct.Recipients[dataBACI] = store.Recipient{Timer: 3600, TimerVersion: 4}
	bin, _ := uuidRaw(dataBACI)
	dm := &signalpb.AppDataMessage{Body: proto.String("hi @B"), BodyRanges: []*signalpb.AppBodyRange{
		{Start: proto.Uint32(3), Length: proto.Uint32(2), AssociatedValue: &signalpb.AppBodyRange_MentionAciBinary{MentionAciBinary: bin[:]}}}}
	ts, res := s.SendData(context.Background(), []string{dataBACI}, DataSend{Message: dm, Urgent: true})
	if res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	got := deliveredTo(t, fc, dataBACI)[0].GetDataMessage()
	if got.GetTimestamp() != ts || got.GetExpireTimer() != 3600 || got.GetExpireTimerVersion() != 4 || len(got.GetProfileKey()) != 32 || got.GetRequiredProtocolVersion() != 6 {
		t.Errorf("enriched %v", got)
	}
	sent := deliveredTo(t, fc, testACI)[0].GetSyncMessage().GetSent()
	if sent.GetMessage().GetBody() != "hi @B" || sent.GetDestinationServiceId() != dataBACI {
		t.Errorf("transcript %v", sent)
	}
	if !s.Acct.Recipients[dataBACI].ProfileSharing {
		t.Error("profile sharing not turned on")
	}
}

// TestEditAndPNISignature: an edit is Content.editMessage and never
// carries a PNI signature; a data message to a recipient that needs one
// does, and it verifies (§2.9, §4).
func TestEditAndPNISignature(t *testing.T) {
	fc, s := dataWorld(t)
	s.Acct.Recipients[dataBACI] = store.Recipient{NeedsPNISignature: true}
	ts, res := s.SendData(context.Background(), []string{dataBACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("orig")}, Urgent: true})
	if res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	c := deliveredTo(t, fc, dataBACI)[0]
	sig := c.GetPniSignatureMessage()
	pni, _ := uuidRaw(testPNI)
	if sig == nil || string(sig.GetPni()) != string(pni[:]) || !verifyPNISignature(s.Acct.Data.ACIIdentity.Public, s.Acct.Data.PNIIdentity.Public, sig.GetSignature()) {
		t.Errorf("PNI signature %v", sig)
	}
	if verifyPNISignature(s.Acct.Data.PNIIdentity.Public, s.Acct.Data.ACIIdentity.Public, sig.GetSignature()) {
		t.Error("swapped keys verified")
	}
	fc.delivered = nil
	_, res = s.SendData(context.Background(), []string{dataBACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("edited")}, EditTarget: ts, Urgent: true})
	if res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	e := deliveredTo(t, fc, dataBACI)[0]
	if e.GetEditMessage().GetTargetSentTimestamp() != ts || e.GetEditMessage().GetDataMessage().GetBody() != "edited" || e.PniSignatureMessage != nil || e.GetDataMessage() != nil {
		t.Errorf("edit %v", e)
	}
	if deliveredTo(t, fc, testACI)[0].GetSyncMessage().GetSent().GetEditMessage().GetTargetSentTimestamp() != ts {
		t.Error("edit transcript")
	}
}

// TestSendToSelf: without --notify-self only a transcript goes to the
// other devices, without a profile key (§1.3).
func TestSendToSelf(t *testing.T) {
	fc, s := dataWorld(t)
	_, res := s.SendData(context.Background(), []string{testACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("note")}, Urgent: true})
	if res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	got := deliveredTo(t, fc, testACI)
	if len(got) != 1 || got[0].GetSyncMessage().GetSent().GetMessage().GetBody() != "note" || got[0].GetSyncMessage().GetSent().GetMessage().ProfileKey != nil {
		t.Errorf("self send delivered %v", got)
	}
	for _, p := range fc.puts {
		if strings.Contains(p, `"type":1,`) && !strings.Contains(p, testACI) {
			t.Error("message to someone else")
		}
	}
}

// TestViewOnceTranscript: the transcript of a view-once message drops
// its attachments (sync.md, Sent field 3).
func TestViewOnceTranscript(t *testing.T) {
	fc, s := dataWorld(t)
	att := &signalpb.AppAttachmentPointer{ContentType: proto.String("image/png"), CdnNumber: proto.Uint32(3), Key: make([]byte, 64),
		AttachmentIdentifier: &signalpb.AppAttachmentPointer_CdnKey{CdnKey: "k"}}
	dm := &signalpb.AppDataMessage{IsViewOnce: proto.Bool(true), Attachments: []*signalpb.AppAttachmentPointer{att}}
	if _, res := s.SendData(context.Background(), []string{dataBACI}, DataSend{Message: dm, Urgent: true}); res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	if len(deliveredTo(t, fc, dataBACI)[0].GetDataMessage().GetAttachments()) != 1 {
		t.Error("recipient lost the attachment")
	}
	sent := deliveredTo(t, fc, testACI)[0].GetSyncMessage().GetSent().GetMessage()
	if len(sent.GetAttachments()) != 0 || !sent.GetIsViewOnce() {
		t.Errorf("transcript %v", sent)
	}
}
