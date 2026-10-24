package manager

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
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
