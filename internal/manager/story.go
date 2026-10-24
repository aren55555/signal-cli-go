package manager

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"google.golang.org/protobuf/proto"
)

// Stories and the sender-key send path (spec/service/stories.md §2-3).

// MyStory is the My Story distribution ID, all zero (§2.3).
var MyStory protocol.DistributionID

// Story errors.
var (
	ErrNoStoryRecipients = errors.New("No eligible contacts found for story delivery")
	ErrStoryMedia        = errors.New("Stories only support image and video attachments")
	// ErrNoStoryAccess is gap G1: no recipient has an unidentified access,
	// so there is no sender certificate (signal-cli fails with a
	// NullPointerException).
	ErrNoStoryAccess = errors.New("no recipient has an unidentified access")
	// ErrNetworkFailure is the result of recipients left out after an
	// SKDM failure (§3.2).
	ErrNetworkFailure = errors.New("network failure")
)

// storyRecipients is §2.2: registered, unblocked contacts other than us.
func (s *Sender) storyRecipients() []string {
	var out []string
	for aci, rc := range s.Acct.Recipients {
		if aci == s.Acct.Data.ACI || rc.Unregistered || rc.Blocked || !isContact(rc) {
			continue
		}
		if _, ok := uuidRaw(aci); !ok {
			continue
		}
		out = append(out, aci)
	}
	slices.Sort(out)
	return out
}

// SendStory posts a My Story (§2.1): pick recipients, upload, then the
// sender-key send and, on a multi-device account, the transcript.
func (s *Sender) SendStory(ctx context.Context, src attachment.Source, allowsReplies bool) (uint64, []SendResult, error) {
	if !hasMediaType(src.ContentType) {
		return 0, nil, ErrStoryMedia
	}
	recipients := s.storyRecipients()
	if len(recipients) == 0 {
		return 0, nil, ErrNoStoryRecipients
	}
	ps, err := s.UploadAttachments(ctx, []attachment.Source{src}, 0)
	if err != nil {
		return 0, nil, err
	}
	ts := s.timestamp()
	story := &signalpb.AppStoryMessage{ProfileKey: s.Acct.Data.ProfileKey,
		Attachment: &signalpb.AppStoryMessage_FileAttachment{FileAttachment: ps[0]}, AllowsReplies: proto.Bool(allowsReplies)}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: story}})
	if err != nil {
		return 0, nil, err
	}
	results, err := s.sendSenderKey(ctx, MyStory, recipients, c, ts, true)
	if err != nil {
		return ts, nil, err
	}
	if s.Acct.IsMultiDevice() {
		if err := s.storyTranscript(ctx, story, recipients, ts); err != nil {
			// Logged by signal-cli, not reported.
			_ = err
		}
	}
	return ts, results, nil
}

func hasMediaType(t string) bool {
	return len(t) > 6 && (t[:6] == "image/" || t[:6] == "video/")
}

// storyTranscript is §2.5.
func (s *Sender) storyTranscript(ctx context.Context, story *signalpb.AppStoryMessage, recipients []string, ts uint64) error {
	self, _ := protocol.ServiceIDToBinary(s.Acct.Data.ACI)
	sent := &signalpb.AppSent{
		Timestamp:                  proto.Uint64(ts),
		DestinationServiceIdBinary: self,
		UnidentifiedStatus:         []*signalpb.AppUnidentifiedStatus{{DestinationServiceIdBinary: self, Unidentified: proto.Bool(false)}},
		StoryMessage:               story,
		IsRecipientUpdate:          proto.Bool(false),
	}
	if s.Acct.Data.Number != "" {
		sent.DestinationE164 = proto.String(s.Acct.Data.Number)
	}
	for _, r := range recipients {
		bin, _ := protocol.ServiceIDToBinary(r)
		sent.StoryMessageRecipients = append(sent.StoryMessageRecipients, &signalpb.AppStoryMessageRecipient{
			DestinationServiceIdBinary: bin, DistributionListIds: []string{MyStory.String()}, IsAllowedToReply: proto.Bool(story.GetAllowsReplies())})
	}
	return s.sendSync(ctx, &signalpb.AppSyncMessage{Sent: sent}, ts, false)
}

// device is one (service ID, device) destination.
type device struct {
	id  string
	dev uint32
}

func (d device) name() string { return d.id + "." + strconv.Itoa(int(d.dev)) }

// deviceSet is §3.1: every device with an active session plus device 1 of
// every recipient; and the sessions.
func (s *Sender) deviceSet(recipients []string) ([]device, map[device]*protocol.SessionRecord, error) {
	var all []device
	sessions := map[device]*protocol.SessionRecord{}
	for _, r := range recipients {
		recs, err := s.sessions(r)
		if err != nil {
			return nil, nil, err
		}
		ids := []uint32{1}
		for id, rec := range recs {
			sessions[device{r, id}] = rec
			if id != 1 {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		for _, id := range ids {
			all = append(all, device{r, id})
		}
	}
	return all, sessions, nil
}

// unshare deletes shared-with rows for a device across all distributions
// (§3.5, §3.7).
func (s *Sender) unshare(d device) {
	for _, rows := range s.Acct.SenderKeys.Shared {
		delete(rows, d.name())
	}
}

// ownSenderKey loads our sending record for a distribution.
func (s *Sender) ownSenderKey(dist protocol.DistributionID) (string, *protocol.SenderKeyRecord, error) {
	name := SenderKeyName(s.Acct.Data.ACI, uint32(s.Acct.Data.DeviceID), dist)
	rec := new(protocol.SenderKeyRecord)
	if b, ok := s.Acct.SenderKeys.Records[name]; ok {
		var err error
		if rec, err = protocol.LoadSenderKeyRecord(b); err != nil {
			return "", nil, err
		}
	}
	return name, rec, nil
}

func (s *Sender) saveSenderKey(name string, rec *protocol.SenderKeyRecord) error {
	b, err := rec.Serialize()
	if err != nil {
		return err
	}
	if _, ok := s.Acct.SenderKeys.Records[name]; !ok {
		s.Acct.SenderKeys.Created[name] = s.now().UnixMilli()
	}
	s.Acct.SenderKeys.Records[name] = b
	return s.Acct.SaveSenderKeys()
}

// sendSenderKey is §3: SKDM fan-out, sender-key encryption, sealed sender
// v2 and the multi-recipient request, up to 4 attempts. Per-recipient
// outcomes are results; an error means the whole send failed.
func (s *Sender) sendSenderKey(ctx context.Context, dist protocol.DistributionID, recipients []string, plaintext []byte, ts uint64, story bool) ([]SendResult, error) {
	if s.Unauth == nil {
		return nil, errors.New("no unauthenticated connection for the multi-recipient send")
	}
	shared := s.Acct.SenderKeys.Shared[dist.String()]
	if shared == nil {
		shared = map[string]int64{}
		s.Acct.SenderKeys.Shared[dist.String()] = shared
	}
	for attempt := 0; attempt < 4; attempt++ {
		all, sessions, err := s.deviceSet(recipients)
		if err != nil {
			return nil, err
		}
		// §3.2: who needs our key?
		var needs []string
		for _, r := range recipients {
			for _, d := range all {
				if d.id != r {
					continue
				}
				if _, ok := shared[d.name()]; !ok || sessions[d] == nil {
					needs = append(needs, r)
					break
				}
			}
		}
		if len(needs) > 0 {
			name, rec, err := s.ownSenderKey(dist)
			if err != nil {
				return nil, err
			}
			rec, skdm, err := rec.NewDistribution(s.rand(), dist)
			if err != nil {
				return nil, err
			}
			if err := s.saveSenderKey(name, rec); err != nil {
				return nil, err
			}
			c, _ := proto.Marshal(&signalpb.AppContent{SenderKeyDistributionMessage: skdm})
			failed := map[string]error{}
			for _, r := range needs {
				_, _, err := s.deliver(ctx, r, c, uint64(s.now().UnixMilli()), deliverOpts{story: story, hint: protocol.ContentHintImplicit})
				if err != nil {
					failed[r] = err
					continue
				}
				now := s.now().UnixMilli()
				for _, d := range all {
					if d.id == r {
						shared[d.name()] = now
					}
				}
			}
			if err := s.Acct.SaveSenderKeys(); err != nil {
				return nil, err
			}
			if len(failed) > 0 {
				// Any SKDM failure aborts the send (§3.2).
				var results []SendResult
				for _, r := range recipients {
					err, ok := failed[r]
					if !ok {
						err = ErrNetworkFailure
					}
					results = append(results, SendResult{Dest: r, Err: err})
				}
				return results, nil
			}
			if all, sessions, err = s.deviceSet(recipients); err != nil {
				return nil, err
			}
		}

		body, err := s.sealSenderKey(ctx, dist, recipients, all, sessions, plaintext)
		if err != nil {
			return nil, err
		}
		r, err := s.Unauth.Request(ctx, "PUT", multiRecipientPath(ts, false, false, story), []chat.Header{{Name: "content-type", Value: mrmContentType}}, body)
		if err != nil {
			return nil, err
		}
		res, err := parseMultiRecipientResponse(r.Status, r.Body)
		if err != nil {
			return nil, err
		}
		if res.Mismatches == nil {
			var results []SendResult
			for _, rcp := range recipients {
				var rerr error
				if slices.Contains(res.Unregistered, rcp) {
					rerr = ErrUnregistered
				}
				results = append(results, SendResult{Dest: rcp, Err: rerr})
			}
			return results, nil
		}
		if err := s.handleMismatches(ctx, res.Mismatches); err != nil {
			return nil, err
		}
	}
	return nil, ErrConflictsUnsolved
}

// handleMismatches is §3.5 for a 409/410.
func (s *Sender) handleMismatches(ctx context.Context, ms []mismatch) error {
	for _, m := range ms {
		if err := s.archive(m.Account, append(append([]uint32{}, m.Extra...), m.Stale...)); err != nil {
			return err
		}
		for _, list := range [][]uint32{m.Extra, m.Missing, m.Stale} {
			for _, id := range list {
				s.unshare(device{m.Account, id})
			}
		}
		for _, id := range m.Missing {
			if err := s.fetchBundles(ctx, m.Account, strconv.Itoa(int(id)), nil); err != nil {
				return err
			}
		}
	}
	return s.Acct.SaveSenderKeys()
}

// sealSenderKey is §3.3: pad, sender-key encrypt, USMC type 7 with our
// certificate and content hint IMPLICIT, sealed sender v2 to every device.
func (s *Sender) sealSenderKey(ctx context.Context, dist protocol.DistributionID, recipients []string, all []device, sessions map[device]*protocol.SessionRecord, plaintext []byte) ([]byte, error) {
	hasAccess := false
	for _, r := range recipients {
		if s.accessFor(ctx, r) != nil {
			hasAccess = true
			break
		}
	}
	cert := s.senderCertificate(ctx)
	if !hasAccess || cert == nil {
		return nil, ErrNoStoryAccess
	}
	name, rec, err := s.ownSenderKey(dist)
	if err != nil {
		return nil, err
	}
	rec, skm, err := rec.Encrypt(s.rand(), dist, content.Pad(plaintext))
	if err != nil {
		return nil, err
	}
	if err := s.saveSenderKey(name, rec); err != nil {
		return nil, err
	}
	usmc := protocol.EncodeSealedContent(protocol.SealedContentSenderKey, cert, skm, protocol.ContentHintImplicit, nil)
	var mrs []protocol.MultiRecipient
	index := map[string]int{}
	for _, d := range all {
		sess := sessions[d]
		if sess == nil {
			return nil, fmt.Errorf("no session for %s", d.name())
		}
		reg := sess.RemoteRegistrationID()
		if reg > 0x3fff {
			return nil, fmt.Errorf("invalid registration ID %d", reg)
		}
		i, ok := index[d.id]
		if !ok {
			var fixed protocol.ServiceIDFixed
			bin, valid := protocol.ServiceIDToBinary(d.id)
			if !valid {
				return nil, fmt.Errorf("invalid service ID %s", d.id)
			}
			copy(fixed[17-len(bin):], bin)
			mrs = append(mrs, protocol.MultiRecipient{ServiceID: fixed, IdentityKey: sess.RemoteIdentity()})
			i = len(mrs) - 1
			index[d.id] = i
		}
		mrs[i].Devices = append(mrs[i].Devices, protocol.RecipientDevice{ID: uint8(d.dev), RegistrationID: uint16(reg)})
	}
	identity, err := curve25519.NewPrivateKey(s.Acct.Data.ACIIdentity.Private)
	if err != nil {
		return nil, err
	}
	return protocol.SealV2(s.rand(), identity, mrs, nil, usmc)
}
