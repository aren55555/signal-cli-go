package manager

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// fakePipe hands out deliveries, then times out.
type fakePipe struct {
	items []func() *chat.ServerRequest // built when handed out
}

func (p *fakePipe) Next(ctx context.Context) (*chat.ServerRequest, error) {
	if len(p.items) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	r := p.items[0]
	p.items = p.items[1:]
	return r(), nil
}

type receiveWorld struct {
	a      *store.Account // our A.2
	b      *store.Account // B's single device
	fc     *fakeChat
	root   *curve25519.PrivateKey
	bob    *sender // B's side of the session with A.2
	pipe   *fakePipe
	acked  []uint64
	events []Event
	r      *Receiver
}

func newReceiveWorld(t *testing.T) *receiveWorld {
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a, aBundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	b, bBundles := testAccountFor(t, "+15555550101", sealedBACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	root, _ := curve25519.GeneratePrivateKey(rand.Reader)
	w := &receiveWorld{a: a, b: b, root: root, pipe: &fakePipe{}}
	w.fc = &fakeChat{t: t, sender: testACI, senderDev: 2, root: root,
		devices: map[string]map[uint32]fakeDevice{sealedBACI: {1: {b, bBundles["aci"]}}, testACI: {2: {a, aBundles["aci"]}}},
		cert:    mintCert(t, root, testACI, aID.PublicKey(), 2),
		// B's profile is unrestricted, so receipts can go sealed.
		profiles:   map[string]string{sealedBACI: profileJSON(bID.PublicKey().Bytes(), nil, true)},
		accessKeys: map[string][]byte{sealedBACI: make([]byte, 16)},
	}
	me := protocol.Address{ServiceID: testACI, DeviceID: 2}
	bAddr := protocol.Address{ServiceID: sealedBACI, DeviceID: 1}
	w.bob = newSender(t, sealedBACI, aBundles["aci"], bAddr, me)
	w.bob.id = bID
	// B needs the session record to decrypt our replies: rebuild it with
	// B's real identity.
	rec, err := protocol.NewInitiatorRecord(rand.Reader, bID, 77, aBundles["aci"], time.Now(), bAddr, me)
	if err != nil {
		t.Fatal(err)
	}
	w.bob.record = rec
	w.r = &Receiver{Acct: a, Decryptor: &Decryptor{Acct: a, Roots: []*curve25519.PublicKey{root.PublicKey()}},
		Sender:  &Sender{Acct: a, Chat: w.fc, Unauth: unauthChat{w.fc}},
		OnEvent: func(e Event) { w.events = append(w.events, e) }}
	return w
}

// envelope from B: kind "sealed", "plain" or "garbage".
func (w *receiveWorld) push(t *testing.T, kind string, c *signalpb.AppContent, ts uint64) {
	w.pipe.items = append(w.pipe.items, func() *chat.ServerRequest { return w.build(t, kind, c, ts) })
}

// build makes the envelope from B's current session (B may have
// decrypted our replies since).
func (w *receiveWorld) build(t *testing.T, kind string, c *signalpb.AppContent, ts uint64) *chat.ServerRequest {
	me := protocol.Address{ServiceID: testACI, DeviceID: 2}
	bAddr := protocol.Address{ServiceID: sealedBACI, DeviceID: 1}
	if sb, ok := w.b.Sessions[sessionKey(false, me)]; ok {
		w.bob.record, _ = protocol.LoadSessionRecord(sb)
	}
	pt, _ := proto.Marshal(c)
	env := &signalpb.ServiceEnvelope{DestinationServiceIdBinary: uuidBytes(testACI), ClientTimestamp: proto.Uint64(ts), ServerTimestamp: proto.Uint64(ts + 1)}
	switch kind {
	case "garbage":
		env.Type, env.SourceServiceIdBinary, env.SourceDeviceId = proto.Uint32(1), uuidBytes(sealedBACI), proto.Uint32(1)
		typ, wire := w.bob.encrypt(t, "x", bAddr, me)
		if typ != protocol.MessageTypeWhisper {
			t.Fatalf("garbage needs an acknowledged session, got type %d", typ)
		}
		env.Type = proto.Uint32(1)
		env.Content = append([]byte{}, wire...)
		env.Content[len(env.Content)-1] ^= 0xff // break the MAC
	default:
		next, typ, wire, err := w.bob.record.Encrypt(rand.Reader, padContent(pt), time.Now(), bAddr, me)
		if err != nil {
			t.Fatal(err)
		}
		w.bob.record = next
		sb, _ := next.Serialize()
		w.b.Sessions[sessionKey(false, me)] = sb
		env.Type = proto.Uint32(uint32(typ))
		env.SourceServiceIdBinary, env.SourceDeviceId = uuidBytes(sealedBACI), proto.Uint32(1)
		env.Content = wire
		if kind == "sealed" {
			cert := mintCert(t, w.root, sealedBACI, w.bob.id.PublicKey(), 1)
			sk := protocol.SealedContentPreKey
			if typ == protocol.MessageTypeWhisper {
				sk = protocol.SealedContentMessage
			}
			aPub, _ := curve25519.ParsePublicKey(w.a.Data.ACIIdentity.Public)
			sealed, err := protocol.SealV1(rand.Reader, w.bob.id, aPub, protocol.EncodeSealedContent(sk, cert, wire, protocol.ContentHintResendable, nil))
			if err != nil {
				t.Fatal(err)
			}
			env.Type, env.SourceServiceIdBinary, env.SourceDeviceId, env.Content = proto.Uint32(6), nil, nil, sealed
		}
	}
	b, _ := proto.Marshal(env)
	server := uint64(time.Now().UnixMilli())
	return chat.NewDelivery(chat.KindEnvelope, b, server, func() error {
		// Cached before the ack (pipe.md §4.1).
		files, _ := os.ReadDir(filepath.Join(w.a.Dir(), "msg-cache"))
		if len(files) == 0 {
			t.Errorf("envelope %d acked before it was cached", ts)
		}
		w.acked = append(w.acked, server)
		return nil
	})
}

func (w *receiveWorld) queueEmpty() {
	w.pipe.items = append(w.pipe.items, func() *chat.ServerRequest { return chat.NewDelivery(chat.KindQueueEmpty, nil, 0, nil) })
}

func dataContent(body string, ts uint64) *signalpb.AppContent {
	return &signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{
		Body: proto.String(body), Timestamp: proto.Uint64(ts), ProfileKey: make([]byte, 32)}}}
}

// B's view of what we sent it.
func (w *receiveWorld) bReceived(t *testing.T) []*signalpb.AppContent {
	var out []*signalpb.AppContent
	for _, d := range w.fc.delivered {
		var c signalpb.AppContent
		proto.Unmarshal(d.msg.Content, &c)
		out = append(out, &c)
	}
	return out
}

// TestReceiveFlow: envelopes are cached, acked, decrypted and reported in
// order; the queued delivery receipt (sealed messages only, one per
// sender) and the prekey refresh run at queue-empty; the cache ends empty
// (pipe.md §7, §7.1).
func TestReceiveFlow(t *testing.T) {
	w := newReceiveWorld(t)
	w.push(t, "sealed", dataContent("one", 1000), 1000)
	w.push(t, "plain", dataContent("two", 2000), 2000)
	w.push(t, "sealed", dataContent("three", 3000), 3000)
	w.push(t, "sealed", &signalpb.AppContent{Body: &signalpb.AppContent_TypingMessage{TypingMessage: &signalpb.AppTyping{Timestamp: proto.Uint64(4000)}}}, 4000)
	w.push(t, "sealed", dataContent("bad timestamp", 1), 5000) // DataMessage timestamp != envelope: dropped
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	if len(w.acked) != 5 {
		t.Errorf("acked %v", w.acked)
	}
	var bodies []string
	for _, e := range w.events {
		if e.Err != nil {
			t.Errorf("event error: %v", e.Err)
			continue
		}
		var c signalpb.AppContent
		proto.Unmarshal(e.Message.Content, &c)
		if c.GetDataMessage() != nil {
			bodies = append(bodies, c.GetDataMessage().GetBody())
		}
	}
	if strings.Join(bodies, ",") != "one,two,three" {
		t.Errorf("reported data messages %v", bodies)
	}
	got := w.bReceived(t)
	if len(got) != 1 || got[0].GetReceiptMessage().GetKind() != signalpb.AppReceipt_DELIVERY ||
		len(got[0].GetReceiptMessage().GetTimestamps()) != 2 || got[0].GetReceiptMessage().GetTimestamps()[1] != 3000 {
		t.Errorf("B received %v", got)
	}
	if !w.fc.delivered[0].msg.Sealed {
		t.Error("receipt not sealed")
	}
	refreshed := false
	for _, r := range w.fc.requests {
		refreshed = refreshed || strings.Contains(r, "GET /v2/keys?identity=aci")
	}
	if !refreshed {
		t.Errorf("no prekey refresh after a PreKey message: %v", w.fc.requests)
	}
	if files, _ := os.ReadDir(filepath.Join(w.a.Dir(), "msg-cache")); len(files) != 0 {
		t.Errorf("cache not emptied: %d files", len(files))
	}
}

// TestReadReceiptsAndCaughtUp: with --send-read-receipts a READ receipt
// follows the delivery receipt; after queue-empty, actions run straight
// after each envelope.
func TestReadReceiptsAndCaughtUp(t *testing.T) {
	w := newReceiveWorld(t)
	w.r.SendReadReceipts = true
	w.queueEmpty()
	w.push(t, "sealed", dataContent("late", 1000), 1000)
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	got := w.bReceived(t)
	if len(got) != 2 || got[0].GetReceiptMessage().GetKind() != signalpb.AppReceipt_DELIVERY || got[1].GetReceiptMessage().GetKind() != signalpb.AppReceipt_READ {
		t.Errorf("B received %v", got)
	}
}

// TestDecryptionFailure: a broken message from B gets session renewal (a
// NullMessage on a fresh session) and a retry receipt naming the message
// (pipe.md §7.2).
func TestDecryptionFailure(t *testing.T) {
	w := newReceiveWorld(t)
	w.queueEmpty()
	w.push(t, "sealed", dataContent("ok", 1000), 1000) // our receipt acknowledges B's session
	w.push(t, "garbage", nil, 2000)
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	var de *DecryptError
	if len(w.events) != 2 || !errors.As(w.events[1].Err, &de) || de.Timestamp != 2000 {
		t.Fatalf("events %+v", w.events)
	}
	var sawNull, sawRetry bool
	for _, c := range w.bReceived(t) {
		if c.GetNullMessage() != nil {
			sawNull = true
		}
		if dem := c.GetDecryptionErrorMessage(); dem != nil {
			var m signalpb.AppDecryptionErrorMessage
			proto.Unmarshal(dem, &m)
			sawRetry = m.GetTimestamp() == 2000 && m.GetDeviceId() == 1 && len(m.GetRatchetKey()) == 33
		}
	}
	if !sawNull || !sawRetry {
		t.Errorf("null message %v, retry receipt %v; B got %v", sawNull, sawRetry, w.bReceived(t))
	}
}

// TestCacheRetry: an envelope that failed with an untrusted identity stays
// cached and is retried at the next start; once trusted it decrypts and
// the cache empties (pipe.md §7 step 1).
func TestCacheRetry(t *testing.T) {
	w := newReceiveWorld(t)
	w.a.Identities[sealedBACI] = store.Identity{Key: make([]byte, 33), Trust: store.TrustedUnverified}
	w.push(t, "plain", dataContent("held", 1000), 1000)
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 1 || !errors.Is(w.events[0].Err, protocol.ErrUntrustedIdentity) {
		t.Fatalf("events %+v", w.events)
	}
	if files, _ := os.ReadDir(filepath.Join(w.a.Dir(), "msg-cache")); len(files) != 1 {
		t.Fatalf("untrusted envelope not kept (%d files)", len(files))
	}
	delete(w.a.Identities, sealedBACI)
	w.events = nil
	if err := w.r.RetryCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 1 || w.events[0].Err != nil {
		t.Fatalf("retry events %+v", w.events)
	}
	if files, _ := os.ReadDir(filepath.Join(w.a.Dir(), "msg-cache")); len(files) != 0 {
		t.Errorf("cache not emptied after a successful retry")
	}
}

func padContent(b []byte) []byte { return content.Pad(b) }

// TestStructureError: a pointer without a CDN number is reported with an
// exception and gets no receipt (content-features.md §7.1 stage X).
func TestStructureError(t *testing.T) {
	w := newReceiveWorld(t)
	c := dataContent("pic", 1000)
	c.GetDataMessage().Attachments = []*signalpb.AppAttachmentPointer{{Key: make([]byte, 64), AttachmentIdentifier: &signalpb.AppAttachmentPointer_CdnKey{CdnKey: "k"}}}
	w.push(t, "sealed", c, 1000)
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	var se *StructureError
	if len(w.events) != 1 || !errors.As(w.events[0].Err, &se) || se.Msg != "AttachmentPointer CDN number not set" || w.events[0].Message.Content != nil {
		t.Fatalf("events %+v", w.events)
	}
	if got := w.bReceived(t); len(got) != 0 {
		t.Errorf("receipt sent: %v", got)
	}
}
