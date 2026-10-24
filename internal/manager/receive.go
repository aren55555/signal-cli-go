package manager

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Incoming yields server deliveries (a *chat.Conn).
type Incoming interface {
	Next(ctx context.Context) (*chat.ServerRequest, error)
}

// Event is one handled envelope, for output. Err is set when it could not
// be decrypted; Message is nil then.
type Event struct {
	Envelope        *signalpb.ServiceEnvelope
	ServerDelivered uint64
	Message         *Message
	Err             error
	// LongText is the long-text attachment body per DataMessage
	// timestamp; it replaces the body in output.
	LongText map[uint64]string
}

// untrustedRetention is how long an envelope refused for an untrusted
// identity stays cached (capture.md §3).
const untrustedRetention = 14 * 24 * time.Hour

// Receiver runs signal-cli's receive loop for one account (pipe.md §7).
type Receiver struct {
	Acct             *store.Account
	Decryptor        *Decryptor
	Sender           *Sender // for receipts, renewals and the prekey refresh
	SendReadReceipts bool
	Attachments      *AttachmentStore // nil: nothing is downloaded
	Stickers         *StickerStore    // nil: no sticker packs are fetched
	OnEvent          func(Event)
	Log              func(string)
	Now              func() time.Time

	caughtUp bool
	pending  actions
}

type actions struct {
	delivery map[string][]uint64 // sender ACI -> data message timestamps
	read     map[string][]uint64
	renew    map[string]bool // "toPNI|serviceID"
	retry    []retryReceipt
	refresh  bool
	// syncAnswers are the sync requests to answer (primary only).
	syncAnswers map[signalpb.AppSyncMessage_Request_Type]bool
	ownProfile  bool        // a FetchLatest LOCAL_PROFILE arrived
	stickers    [][2][]byte // sticker packs to download (id, key)
}

type retryReceipt struct {
	dest   string
	device uint32
	dem    *signalpb.AppDecryptionErrorMessage
}

func (r *Receiver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Receiver) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(fmt.Sprintf(format, args...))
	}
}

func (r *Receiver) cacheDir() string { return filepath.Join(r.Acct.Dir(), "msg-cache") }

// cache writes an envelope before it is acked: 8 bytes of server-delivered
// time, then the serialized envelope.
func (r *Receiver) cache(body []byte, delivered uint64) (string, error) {
	if err := os.MkdirAll(r.cacheDir(), 0o700); err != nil {
		return "", err
	}
	var id [4]byte
	rand.Read(id[:])
	path := filepath.Join(r.cacheDir(), fmt.Sprintf("%020d-%s.env", delivered, hex.EncodeToString(id[:])))
	data := binary.BigEndian.AppendUint64(nil, delivered)
	return path, store.WriteFileAtomic(path, append(data, body...), 0o600)
}

// RetryCache re-processes every cached envelope from earlier runs (pipe.md
// §7 step 1).
func (r *Receiver) RetryCache(ctx context.Context) error {
	entries, err := os.ReadDir(r.cacheDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	r.Decryptor.Retrying = true
	defer func() { r.Decryptor.Retrying = false }()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".env") {
			continue
		}
		path := filepath.Join(r.cacheDir(), e.Name())
		data, err := os.ReadFile(path)
		if err != nil || len(data) < 8 {
			os.Remove(path)
			continue
		}
		r.handle(ctx, path, data[8:], binary.BigEndian.Uint64(data))
	}
	r.flush(ctx)
	return nil
}

// Run waits for deliveries, each wait bounded by timeout (none if <= 0),
// until a wait times out, ctx ends or maxMessages envelopes were handled
// (unlimited if <= 0).
func (r *Receiver) Run(ctx context.Context, in Incoming, timeout time.Duration, maxMessages int) error {
	defer r.flush(context.WithoutCancel(ctx))
	handled := 0
	for {
		wctx, cancel := ctx, context.CancelFunc(func() {})
		if timeout > 0 {
			wctx, cancel = context.WithTimeout(ctx, timeout)
		}
		req, err := in.Next(wctx)
		cancel()
		switch {
		case err != nil && ctx.Err() != nil:
			return nil
		case err != nil && errors.Is(err, context.DeadlineExceeded):
			return nil
		case err != nil:
			return err
		}
		if req.Kind == chat.KindQueueEmpty {
			r.flush(ctx)
			r.caughtUp = true
			continue
		}
		path, err := r.cache(req.Body, req.ServerTimestamp)
		if err != nil {
			return fmt.Errorf("caching an envelope: %w", err)
		}
		if err := req.Ack(); err != nil {
			r.logf("Failed to ack envelope: %v", err)
		}
		r.handle(ctx, path, req.Body, req.ServerTimestamp)
		if r.caughtUp {
			r.flush(ctx)
		}
		if handled++; maxMessages > 0 && handled >= maxMessages {
			return nil
		}
	}
}

// handle decrypts one cached envelope, reports it, queues its follow-up
// actions and removes it from the cache unless it must be retried.
func (r *Receiver) handle(ctx context.Context, path string, body []byte, delivered uint64) {
	var env signalpb.ServiceEnvelope
	if err := proto.Unmarshal(body, &env); err != nil {
		os.Remove(path)
		r.logf("Dropping an unparsable envelope: %v", err)
		return
	}
	m, err := r.Decryptor.Decrypt(&env)
	if errors.Is(err, protocol.ErrUntrustedIdentity) && r.now().Sub(time.UnixMilli(int64(delivered))) < untrustedRetention {
		// Kept for a later retry (pipe.md §7.2).
	} else {
		os.Remove(path)
	}
	switch {
	case errors.Is(err, ErrSelfSend):
		return
	case err != nil:
		r.failed(&env, err)
		r.emit(Event{Envelope: &env, ServerDelivered: delivered, Err: err})
		return
	}
	if m.ServerReceipt {
		r.emit(Event{Envelope: &env, ServerDelivered: delivered, Message: m})
		return
	}
	if err := ApplyContent(r.Acct, m, r.now()); err != nil {
		r.logf("Storing message data failed: %v", err)
	}
	if m.InnerType == envPreKeyBundle {
		r.pending.refresh = true
	}
	var c signalpb.AppContent
	if proto.Unmarshal(m.Content, &c) != nil || !valid(&c, &env) {
		return // dropped silently (pipe.md §7 step 4)
	}
	if sender := m.Sender.ServiceID; sender != r.Acct.Data.ACI && r.Acct.Recipients[sender].Blocked {
		return // messages from blocked contacts are dropped (content-features.md §7.1)
	}
	if skdm := c.GetSenderKeyDistributionMessage(); skdm != nil {
		// Before anything else the Content holds (stories.md §3.8).
		if err := ProcessSenderKeyDistribution(r.Acct, m.Sender, skdm); err != nil {
			r.logf("Failed to process sender key distribution message: %v", err)
		}
	}
	if c.GetNullMessage() != nil {
		return // no output, no side effects (content-features.md §5)
	}
	if sm := c.GetSyncMessage(); sm != nil && !validSync(sm, &env, m.Sender.ServiceID, r.Acct.Data.ACI) {
		return // dropped silently (sync.md §2.1)
	}
	if err := convert(&c); err != nil {
		bare := *m
		bare.Content = nil
		r.emit(Event{Envelope: &env, ServerDelivered: delivered, Message: &bare, Err: err})
		return
	}
	applyPNI(r.Acct, m, &c)
	if sm := c.GetSyncMessage(); sm != nil {
		r.applySync(ctx, sm)
	}
	if c.GetEditMessage() != nil && c.GetPniSignatureMessage() != nil {
		// A PNI signature hides an edit (content-features.md §4.4 item 5).
		c.Body = nil
		if b, err := proto.Marshal(&c); err == nil {
			m.Content = b
		}
	}
	for _, dm := range []*signalpb.AppDataMessage{c.GetDataMessage(), c.GetEditMessage().GetDataMessage()} {
		if dm != nil && dm.GroupV2 == nil {
			applyTimer(r.Acct, m.Sender.ServiceID, dm)
		}
	}
	if dm := c.GetDataMessage(); dm != nil {
		// A delivery receipt only for sealed messages without a visible
		// source (§7.1).
		if m.Sealed && len(env.GetSourceServiceIdBinary()) == 0 && env.GetSourceServiceId() == "" {
			r.queueReceipt(&r.pending.delivery, m.Sender.ServiceID, dm.GetTimestamp())
		}
		if r.SendReadReceipts {
			r.queueReceipt(&r.pending.read, m.Sender.ServiceID, dm.GetTimestamp())
		}
	}
	if dem := c.GetDecryptionErrorMessage(); dem != nil {
		var d signalpb.AppDecryptionErrorMessage
		if proto.Unmarshal(dem, &d) == nil && d.GetDeviceId() == uint32(r.Acct.Data.DeviceID) {
			// No send log to resend from: renew the session (§7.3).
			r.queueRenewal(false, m.Sender.ServiceID)
		}
	}
	ev := Event{Envelope: &env, ServerDelivered: delivered, Message: m}
	sent := c.GetSyncMessage().GetSent()
	for _, dm := range []*signalpb.AppDataMessage{c.GetDataMessage(), c.GetEditMessage().GetDataMessage(), sent.GetMessage(), sent.GetEditMessage().GetDataMessage()} {
		r.noteSticker(dm.GetSticker())
		if text, ok := r.Attachments.FetchData(ctx, dm); ok {
			if ev.LongText == nil {
				ev.LongText = map[uint64]string{}
			}
			ev.LongText[dm.GetTimestamp()] = text
		}
	}
	r.Attachments.FetchStory(ctx, c.GetStoryMessage())
	r.Attachments.FetchStory(ctx, sent.GetStoryMessage())
	r.emit(ev)
}

// currentProtocolVersion is DataMessage.ProtocolVersion CURRENT
// (content.md §4.3).
const currentProtocolVersion = 8

func (r *Receiver) queueReceipt(m *map[string][]uint64, sender string, ts uint64) {
	if *m == nil {
		*m = map[string][]uint64{}
	}
	(*m)[sender] = append((*m)[sender], ts)
}

func (r *Receiver) queueRenewal(toPNI bool, sender string) {
	if r.pending.renew == nil {
		r.pending.renew = map[string]bool{}
	}
	key := "aci|" + sender
	if toPNI {
		key = "pni|" + sender
	}
	r.pending.renew[key] = true
}

// failed queues session renewal and a retry receipt for protocol failures
// from a known sender (pipe.md §7.2).
func (r *Receiver) failed(env *signalpb.ServiceEnvelope, err error) {
	var de *DecryptError
	if !errors.As(err, &de) || de.Sender.ServiceID == "" {
		return
	}
	if errors.Is(err, protocol.ErrInvalidSignedPreKeyID) || errors.Is(err, protocol.ErrInvalidKyberPreKeyID) || errors.Is(err, protocol.ErrInvalidPreKeyID) {
		r.pending.refresh = true
	}
	r.queueRenewal(de.ToPNI, de.Sender.ServiceID)
	if de.Sender.ServiceID == r.Acct.Data.ACI && de.Sender.DeviceID == uint32(r.Acct.Data.DeviceID) {
		return
	}
	if de.RatchetKey == nil {
		return // no DecryptionErrorMessage can be built
	}
	r.pending.retry = append(r.pending.retry, retryReceipt{dest: de.Sender.ServiceID, device: de.Sender.DeviceID,
		dem: &signalpb.AppDecryptionErrorMessage{RatchetKey: de.RatchetKey, Timestamp: proto.Uint64(de.Timestamp), DeviceId: proto.Uint32(de.Sender.DeviceID)}})
}

func (r *Receiver) emit(e Event) {
	if r.OnEvent != nil {
		r.OnEvent(e)
	}
}

// archiveAll archives every session with a peer under one of our
// identities.
func (r *Receiver) archiveAll(toPNI bool, peer string) {
	prefix := "aci:" + peer + "."
	if toPNI {
		prefix = "pni:" + peer + "."
	}
	for k, b := range r.Acct.Sessions {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rec, err := protocol.LoadSessionRecord(b)
		if err != nil {
			delete(r.Acct.Sessions, k)
			continue
		}
		if nb, err := rec.ArchiveCurrent().Serialize(); err == nil {
			r.Acct.Sessions[k] = nb
		}
	}
	r.Acct.SaveSessions()
}

// flush runs the queued actions (pipe.md §7 step 5).
func (r *Receiver) flush(ctx context.Context) {
	p := r.pending
	r.pending = actions{}
	for _, kind := range []struct {
		m    map[string][]uint64
		kind signalpb.AppReceipt_Kind
	}{{p.delivery, ReceiptDelivery}, {p.read, ReceiptRead}} {
		senders := make([]string, 0, len(kind.m))
		for s := range kind.m {
			senders = append(senders, s)
		}
		sort.Strings(senders)
		for _, s := range senders {
			if err := r.Sender.SendReceipt(ctx, s, kind.kind, kind.m[s]); err != nil {
				r.logf("Failed to send receipt to %s: %v", s, err)
			}
		}
	}
	renewals := make([]string, 0, len(p.renew))
	for k := range p.renew {
		renewals = append(renewals, k)
	}
	slices.Sort(renewals)
	for _, k := range renewals {
		kind, peer, _ := strings.Cut(k, "|")
		r.archiveAll(kind == "pni", peer)
		if err := r.Sender.SendNullMessage(ctx, peer); err != nil {
			r.logf("Failed to renew the session with %s: %v", peer, err)
		}
	}
	for _, rr := range p.retry {
		if err := r.Sender.SendRetryReceipt(ctx, rr.dest, rr.device, rr.dem); err != nil {
			r.logf("Failed to send a retry receipt to %s: %v", rr.dest, err)
		}
	}
	if len(p.syncAnswers) > 0 {
		r.answerSyncRequests(ctx, p.syncAnswers)
	}
	if p.ownProfile {
		r.Sender.RefreshProfile(ctx, r.Acct.Data.ACI)
	}
	if len(p.stickers) > 0 {
		r.downloadStickers(ctx, p.stickers)
	}
	if p.refresh {
		if err := RefreshPreKeys(ctx, r.Acct, r.Sender.Chat, r.now()); err != nil {
			r.logf("Failed to refresh prekeys: %v", err)
		}
	}
}
