package manager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// senderKeyEnvelope seals a sender-key message from B to us (USMC type 7).
func (w *receiveWorld) senderKeyEnvelope(t *testing.T, skm []byte, ts uint64, story bool) *chat.ServerRequest {
	cert := mintCert(t, w.root, sealedBACI, w.bob.id.PublicKey(), 1)
	aPub, _ := curve25519.ParsePublicKey(w.a.Data.ACIIdentity.Public)
	sealed, err := protocol.SealV1(rand.Reader, w.bob.id, aPub, protocol.EncodeSealedContent(protocol.SealedContentSenderKey, cert, skm, protocol.ContentHintImplicit, nil))
	if err != nil {
		t.Fatal(err)
	}
	env := &signalpb.ServiceEnvelope{Type: proto.Uint32(6), DestinationServiceIdBinary: uuidBytes(testACI), ClientTimestamp: proto.Uint64(ts),
		ServerTimestamp: proto.Uint64(ts + 1), Content: sealed, Story: proto.Bool(story)}
	b, _ := proto.Marshal(env)
	return chat.NewDelivery(chat.KindEnvelope, b, uint64(time.Now().UnixMilli()), func() error { return nil })
}

// TestReceiveStory: an SKDM (1:1, story-flagged) then a sealed sender-key
// story: decrypted with the stored sender key, the profile key stored,
// the attachment downloaded, no receipt sent (stories.md §3.8, §5.2).
func TestReceiveStory(t *testing.T) {
	w := newReceiveWorld(t)
	cd := &fakeCDN{}
	up := &Sender{Chat: &formChat{cdnNo: 3}, CDN: cd}
	ps, err := up.UploadAttachments(context.Background(), []attachment.Source{{Data: []byte("jpeg"), ContentType: "image/jpeg"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	w.r.Attachments = &AttachmentStore{Dir: dir, CDN: cd}

	var myStory protocol.DistributionID
	bobKey, skdm, err := new(protocol.SenderKeyRecord).NewDistribution(rand.Reader, myStory)
	if err != nil {
		t.Fatal(err)
	}
	w.push(t, "sealed", &signalpb.AppContent{SenderKeyDistributionMessage: skdm}, 1000)
	pk := bytes.Repeat([]byte{4}, 32)
	story, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: &signalpb.AppStoryMessage{
		ProfileKey: pk, Attachment: &signalpb.AppStoryMessage_FileAttachment{FileAttachment: ps[0]}, AllowsReplies: proto.Bool(true)}}})
	_, skm, err := bobKey.Encrypt(rand.Reader, myStory, content.Pad(story))
	if err != nil {
		t.Fatal(err)
	}
	w.pipe.items = append(w.pipe.items, func() *chat.ServerRequest { return w.senderKeyEnvelope(t, skm, 2000, true) })
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 2 || w.events[1].Err != nil {
		t.Fatalf("events %+v", w.events)
	}
	var c signalpb.AppContent
	proto.Unmarshal(w.events[1].Message.Content, &c)
	if !c.GetStoryMessage().GetAllowsReplies() || !w.events[1].Message.Sealed {
		t.Errorf("story %v", &c)
	}
	if !bytes.Equal(w.a.Recipients[sealedBACI].ProfileKey, pk) {
		t.Error("story profile key not stored")
	}
	if b, err := os.ReadFile(filepath.Join(dir, ps[0].GetCdnKey()+".jpg")); err != nil || string(b) != "jpeg" {
		t.Errorf("story attachment: %v", err)
	}
	for _, rc := range w.bReceived(t) {
		if rc.GetReceiptMessage() != nil {
			t.Errorf("receipt sent: %v", rc)
		}
	}

}

// TestSenderKeyWithoutState: a sender-key message with no stored state is
// a decryption error (the retry path), not a crash.
func TestSenderKeyWithoutState(t *testing.T) {
	w := newReceiveWorld(t)
	var id protocol.DistributionID
	key, _, _ := new(protocol.SenderKeyRecord).NewDistribution(rand.Reader, id)
	_, skm, _ := key.Encrypt(rand.Reader, id, content.Pad([]byte{}))
	w.pipe.items = append(w.pipe.items, func() *chat.ServerRequest { return w.senderKeyEnvelope(t, skm, 1000, true) })
	w.queueEmpty()
	w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0)
	var de *DecryptError
	if len(w.events) != 1 || !errors.As(w.events[0].Err, &de) {
		t.Fatalf("events %+v", w.events)
	}
}

// TestStoryFlagRule: a story-flagged envelope needs story content
// (stories.md §1.5).
func TestStoryFlagRule(t *testing.T) {
	env := &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(10), Story: proto.Bool(true)}
	plain := dataContent("x", 10)
	if valid(plain, env) {
		t.Error("plain data message accepted as a story")
	}
	del := &signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{Timestamp: proto.Uint64(10),
		Delete: &signalpb.AppDelete{TargetSentTimestamp: proto.Uint64(1)}}}}
	skdm := &signalpb.AppContent{SenderKeyDistributionMessage: []byte{1}}
	story := &signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: &signalpb.AppStoryMessage{}}}
	for i, c := range []*signalpb.AppContent{del, skdm, story} {
		if !valid(c, env) {
			t.Errorf("case %d refused", i)
		}
	}
	noLocation := &signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: &signalpb.AppStoryMessage{
		Attachment: &signalpb.AppStoryMessage_FileAttachment{FileAttachment: &signalpb.AppAttachmentPointer{}}}}}
	if valid(noLocation, env) {
		t.Error("story attachment without location accepted")
	}
}

// receiveSKDM plays the receiving device's SKDM processing.
func (f *fakeChat) receiveSKDM(a *store.Account, msg *Message) {
	var c signalpb.AppContent
	if proto.Unmarshal(msg.Content, &c) == nil && c.SenderKeyDistributionMessage != nil {
		if err := ProcessSenderKeyDistribution(a, msg.Sender, c.SenderKeyDistributionMessage); err != nil {
			f.t.Errorf("SKDM: %v", err)
		}
	}
}

// multiRecipient plays PUT /v1/messages/multi_recipient: device check
// (409), then each device unseals its ReceivedMessage.
func (f *fakeChat) multiRecipient(unauth bool, path string, headers []chat.Header, body []byte) *chat.Response {
	f.mrPaths = append(f.mrPaths, path)
	if !unauth {
		f.t.Error("multi_recipient on the authenticated socket")
	}
	ct := ""
	for _, h := range headers {
		if h.Name == "content-type" {
			ct = h.Value
		}
	}
	if ct != mrmContentType {
		f.t.Errorf("content-type %q", ct)
	}
	sm, err := protocol.ParseSentMessage(body)
	if err != nil {
		f.t.Errorf("sent message: %v", err)
		return &chat.Response{Status: 400}
	}
	type entry struct {
		UUID    string              `json:"uuid"`
		Devices map[string][]uint32 `json:"devices"`
	}
	var conflicts []entry
	for _, r := range sm.Recipients {
		id, _ := protocol.ServiceIDFromBinary(r.ServiceID[:])
		var got []uint32
		for _, d := range r.Devices {
			got = append(got, uint32(d.ID))
		}
		var missing, extra []uint32
		for dev := range f.devices[id] {
			if !slices.Contains(got, dev) {
				missing = append(missing, dev)
			}
		}
		for _, dev := range got {
			if _, ok := f.devices[id][dev]; !ok {
				extra = append(extra, dev)
			}
		}
		if len(missing)+len(extra) > 0 {
			conflicts = append(conflicts, entry{id, map[string][]uint32{"missingDevices": missing, "extraDevices": extra}})
		}
	}
	if conflicts != nil {
		b, _ := json.Marshal(conflicts)
		return &chat.Response{Status: 409, Body: b}
	}
	ts, _ := strconv.ParseUint(strings.TrimPrefix(strings.Split(path, "&")[0], "/v1/messages/multi_recipient?ts="), 10, 64)
	for i, r := range sm.Recipients {
		id, _ := protocol.ServiceIDFromBinary(r.ServiceID[:])
		for _, rd := range r.Devices {
			d := f.devices[id][uint32(rd.ID)]
			if uint32(rd.RegistrationID) != d.bundle.RegistrationID {
				f.t.Errorf("registration ID %d for %s.%d", rd.RegistrationID, id, rd.ID)
			}
			env := &signalpb.ServiceEnvelope{Type: proto.Uint32(6), ServerTimestamp: proto.Uint64(1), DestinationServiceIdBinary: uuidBytes(id),
				Content: sm.ReceivedMessage(i), ClientTimestamp: proto.Uint64(ts), Story: proto.Bool(strings.Contains(path, "story=true"))}
			dec := &Decryptor{Acct: d.acct, Roots: []*curve25519.PublicKey{f.root.PublicKey()}}
			msg, err := dec.Decrypt(env)
			if err != nil {
				f.t.Errorf("device %s.%d cannot decrypt the multi-recipient message: %v", id, rd.ID, err)
				continue
			}
			f.delivered = append(f.delivered, delivery{id, uint32(rd.ID), msg})
		}
	}
	return &chat.Response{Status: 200, Body: []byte(`{"uuids404":[]}`)}
}

// TestSendStory: My Story to B (two devices, one with no session yet):
// SKDMs 1:1 with ?story=true, then one multi-recipient request on the
// unauthenticated socket that both devices decrypt; later stories send
// no SKDM once every device is marked; the transcript goes to A.1
// (stories.md §2-3).
func TestSendStory(t *testing.T) {
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a2, a2Bundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	a1, a1Bundles := testAccountFor(t, testNumber, testACI, testPNI, 1, aID)
	b1, b1Bundles := testAccountFor(t, "+15555550101", sealedBACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	b2, b2Bundles := testAccountFor(t, "+15555550101", sealedBACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 2, bID)
	root, _ := curve25519.GeneratePrivateKey(rand.Reader)
	fc := &fakeChat{t: t, sender: testACI, senderDev: 2, root: root,
		devices: map[string]map[uint32]fakeDevice{sealedBACI: {1: {b1, b1Bundles["aci"]}, 2: {b2, b2Bundles["aci"]}},
			testACI: {1: {a1, a1Bundles["aci"]}, 2: {a2, a2Bundles["aci"]}}},
		cert:       mintCert(t, root, testACI, aID.PublicKey(), 2),
		profiles:   map[string]string{sealedBACI: profileJSON(bID.PublicKey().Bytes(), nil, true)},
		accessKeys: map[string][]byte{sealedBACI: make([]byte, 16)},
	}
	cd := &fakeCDN{}
	s := &Sender{Acct: a2, Chat: fc, Unauth: unauthChat{fc}, CDN: cd}
	a2.Recipients[sealedBACI] = store.Recipient{ProfileSharing: true}
	a2.Recipients["c0c0c0c0-0000-4000-8000-000000000003"] = store.Recipient{} // not a contact
	src := attachment.Source{Data: []byte("img"), ContentType: "image/png"}

	ts, results, err := s.SendStory(context.Background(), src, true)
	if err != nil {
		t.Fatalf("send: %v (requests %v)", err, fc.requests)
	}
	if len(results) != 1 || results[0].Dest != sealedBACI || results[0].Err != nil {
		t.Fatalf("results %+v", results)
	}
	skdms, stories := 0, 0
	for _, d := range fc.delivered {
		var c signalpb.AppContent
		proto.Unmarshal(d.msg.Content, &c)
		switch {
		case c.SenderKeyDistributionMessage != nil:
			skdms++
		case c.GetStoryMessage() != nil && d.to == sealedBACI:
			stories++
			if !d.msg.Sealed || !c.GetStoryMessage().GetAllowsReplies() || c.GetStoryMessage().GetFileAttachment().GetContentType() != "image/png" {
				t.Errorf("story %v", &c)
			}
		case c.GetSyncMessage().GetSent().GetStoryMessage() != nil:
			sent := c.GetSyncMessage().GetSent()
			if sent.GetTimestamp() != ts || len(sent.GetStoryMessageRecipients()) != 1 || sent.GetStoryMessageRecipients()[0].GetDistributionListIds()[0] != MyStory.String() {
				t.Errorf("transcript %v", sent)
			}
		}
	}
	if skdms != 2 || stories != 2 {
		t.Errorf("%d SKDMs, %d stories delivered (requests %v)", skdms, stories, fc.requests)
	}
	if len(fc.mrPaths) != 1 || fc.mrPaths[0] != multiRecipientPath(ts, false, false, true) {
		t.Errorf("multi-recipient paths %v", fc.mrPaths)
	}
	if !slices.Contains(fc.requests, "unauth PUT /v1/messages/"+sealedBACI+"?story=true") {
		t.Errorf("no sealed SKDM put with story=true: %v", fc.requests)
	}

	// B.2 was found through the SKDM send's 409, so it was not marked
	// (§3.2): the second story sends the SKDM again, the third none.
	if _, _, err := s.SendStory(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	fc.delivered, fc.requests = nil, nil
	if _, _, err := s.SendStory(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	for _, r := range fc.requests {
		if strings.Contains(r, "?story=true") {
			t.Errorf("SKDM sent again: %s", r)
		}
	}
	if len(deliveredTo(t, fc, sealedBACI)) != 2 {
		t.Errorf("second story delivered %d times", len(deliveredTo(t, fc, sealedBACI)))
	}
}
