package manager

import (
	"context"
	"errors"
	"github.com/aren55555/signal-cli-go/internal/attachment"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Sending to a group (spec/service/groups.md §12, §13).

const protocolHintImplicit = protocol.ContentHintImplicit

type groupSendOpts struct {
	hint                  uint32
	urgent, online, story bool
}

// Group send errors (§12.1).
var (
	ErrGroupNotFound = errors.New("group not found")
	ErrNotAMember    = errors.New("not a member")
	ErrNotAllowed    = errors.New("not allowed")
)

// endorsements returns the stored endorsements, refreshing once when
// missing or within 2 hours of expiry (§12.2 step 3).
func (m *Groups) endorsements(ctx context.Context, rec *store.GroupRecord) (map[string]zkgroup.GroupSendEndorsement, uint64, bool) {
	usable := func(r *store.GroupRecord) bool {
		return len(r.Endorsements) > 0 && r.EndorsementExpiration-int64(2*time.Hour/time.Millisecond) >= m.now().UnixMilli()
	}
	if !usable(rec) {
		m.Refresh(ctx, rec, true)
		if r := m.Acct.Group(rec.GroupID); r != nil {
			rec = r
		}
		if !usable(rec) {
			return nil, 0, false
		}
	}
	out := map[string]zkgroup.GroupSendEndorsement{}
	for aci, b := range rec.Endorsements {
		if e, err := zkgroup.ParseGroupSendEndorsement(b); err == nil {
			out[aci] = e
		}
	}
	return out, uint64(rec.EndorsementExpiration / 1000), true
}

func (m *Groups) trusted(aci string) bool {
	id, ok := m.Acct.Identities[aci]
	return ok && id.Trust != store.Untrusted
}

// sendGroup is §12.2: sender key to qualifying members, fan-out to the
// rest (not for stories), transcripts.
func (m *Groups) sendGroup(ctx context.Context, rec *store.GroupRecord, recipients []string, content []byte, dm *signalpb.AppDataMessage, ts uint64, o groupSendOpts) []SendResult {
	s := m.Sender
	var unregistered []SendResult
	var rest []string
	for _, r := range recipients {
		if m.Acct.Recipients[r].Unregistered {
			unregistered = append(unregistered, SendResult{Dest: r, Err: ErrUnregistered})
		} else {
			rest = append(rest, r)
		}
	}
	g := Secret(rec)
	ends, exp, haveEnds := m.endorsements(ctx, rec)
	var targets []string
	if haveEnds {
		for _, r := range rest {
			if _, ok := ends[r]; ok && s.accessFor(ctx, r) != nil && m.trusted(r) {
				targets = append(targets, r)
			}
		}
	}
	if len(targets) < 2 {
		targets = nil
	}
	tokens := map[string][]byte{}
	for _, r := range rest {
		if e, ok := ends[r]; ok {
			tokens[r] = e.FullToken(g, exp)
		}
	}
	isTarget := map[string]bool{}
	for _, t := range targets {
		isTarget[t] = true
	}
	var results []SendResult
	var fanout []string
	for _, r := range rest {
		if !isTarget[r] {
			fanout = append(fanout, r)
		}
	}
	skSucceeded := false
	if len(targets) > 0 {
		dist, _ := uuidRaw(rec.DistributionID)
		groupID := rec.GroupID
		if o.story {
			groupID = rec.MasterKey // a signal-cli quirk kept for interop (stories.md §2.4.6)
		}
		res, err := s.sendSenderKey(ctx, targets, content, ts, senderKeyOpts{
			dist: protocol.DistributionID(dist), story: o.story, urgent: o.urgent, online: o.online, hint: o.hint,
			groupID: groupID, tokens: tokens,
			combinedToken: func(ts []string) []byte {
				var es []zkgroup.GroupSendEndorsement
				for _, t := range ts {
					es = append(es, ends[t])
				}
				return zkgroup.CombineEndorsements(es).FullToken(g, exp)
			},
		})
		var ok []SendResult
		if err != nil {
			if !o.story {
				fanout = append(fanout, targets...)
			}
		} else {
			for _, r := range res {
				switch {
				case r.Err == nil:
					ok = append(ok, r)
				case !o.story:
					fanout = append(fanout, r.Dest)
				}
			}
		}
		results = append(results, ok...)
		skSucceeded = len(ok) > 0
		if len(ok) > 0 && dm != nil && !o.story && s.Acct.IsMultiDevice() {
			s.groupTranscript(ctx, dm, ok, ts, false)
		}
	}
	if !o.story && (len(fanout) > 0 || (len(rest) == 0 && s.Acct.IsMultiDevice())) {
		// Group tokens give sealed access only if some fan-out recipient has
		// an access key, whose certificate is used (§12.4).
		withKey := false
		for _, r := range fanout {
			if s.accessFor(ctx, r) != nil {
				withKey = true
				break
			}
		}
		var fan []SendResult
		needsSync := false
		for _, r := range fanout {
			opts := deliverOpts{urgent: o.urgent, online: o.online, hint: o.hint, groupID: rec.GroupID}
			if withKey {
				opts.groupToken = tokens[r]
			}
			ns, _, err := s.deliver(ctx, r, content, ts, opts)
			needsSync = needsSync || ns
			fan = append(fan, SendResult{Dest: r, Err: err})
		}
		results = append(results, fan...)
		if dm != nil && (needsSync || s.Acct.IsMultiDevice()) {
			var ok []SendResult
			for _, r := range fan {
				if r.Err == nil {
					ok = append(ok, r)
				}
			}
			s.groupTranscript(ctx, dm, ok, ts, skSucceeded)
		}
	}
	return append(results, unregistered...)
}

// groupTranscript is the sent transcript of a group message: no
// destination, one status per successful recipient (sync.md §4.5).
func (s *Sender) groupTranscript(ctx context.Context, dm *signalpb.AppDataMessage, ok []SendResult, ts uint64, recipientUpdate bool) {
	sent := &signalpb.AppSent{Timestamp: proto.Uint64(ts), Message: dm, IsRecipientUpdate: proto.Bool(recipientUpdate)}
	for _, r := range ok {
		bin, _ := protocol.ServiceIDToBinary(r.Dest)
		sent.UnidentifiedStatus = append(sent.UnidentifiedStatus, &signalpb.AppUnidentifiedStatus{DestinationServiceIdBinary: bin, Unidentified: proto.Bool(false)})
	}
	if dm.GetExpireTimer() > 0 {
		sent.ExpirationStartTimestamp = proto.Uint64(uint64(s.now().UnixMilli()))
	}
	s.sendSync(ctx, &signalpb.AppSyncMessage{Sent: sent}, ts, false)
}

// memberGroup loads a group for sending: it must exist and we must be a
// full member (§12.1).
func (m *Groups) memberGroup(ctx context.Context, id []byte) (*store.GroupRecord, *groups.State, error) {
	rec := m.Group(ctx, id, false)
	if rec == nil {
		return nil, nil, ErrGroupNotFound
	}
	st := StateOf(rec)
	if st == nil || st.Member(m.Acct.Data.ACI) == nil {
		return rec, st, ErrNotAMember
	}
	return rec, st, nil
}

func (m *Groups) others(st *groups.State, withSelf bool) []string {
	var out []string
	for _, mem := range st.Members {
		if mem.ACI != m.Acct.Data.ACI || withSelf {
			out = append(out, mem.ACI)
		}
	}
	return out
}

// SendData sends a data message (or an edit) to a group (§12.1).
func (m *Groups) SendData(ctx context.Context, id []byte, d DataSend) (uint64, []SendResult, error) {
	rec, st, err := m.memberGroup(ctx, id)
	if err != nil {
		return 0, nil, err
	}
	if st.Announcements == groups.Enabled {
		if me := st.Member(m.Acct.Data.ACI); me.Role != groups.RoleAdmin && renderable(d.Message) {
			return 0, nil, ErrNotAllowed
		}
	}
	if !rec.ProfileSharing {
		rec.ProfileSharing = true
		rec = m.save(rec)
	}
	ts := d.Timestamp
	if ts == 0 {
		ts = m.Sender.timestamp()
	}
	dm := proto.Clone(d.Message).(*signalpb.AppDataMessage)
	dm.Timestamp = proto.Uint64(ts)
	dm.GroupV2 = &signalpb.AppGroupContextV2{MasterKey: rec.MasterKey, Revision: proto.Uint32(st.Revision)}
	if st.Timer > 0 && dm.GetFlags()&2 == 0 {
		dm.ExpireTimer = proto.Uint32(st.Timer)
	}
	dm.RequiredProtocolVersion = proto.Uint32(requiredVersion(dm))
	c := &signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: dm}}
	if d.EditTarget != 0 {
		c.Body = &signalpb.AppContent_EditMessage{EditMessage: &signalpb.AppEdit{TargetSentTimestamp: proto.Uint64(d.EditTarget), DataMessage: dm}}
	}
	b, err := proto.Marshal(c)
	if err != nil {
		return 0, nil, err
	}
	results := m.sendGroup(ctx, rec, m.others(st, d.NotifySelf), b, dm, ts, groupSendOpts{hint: protocol.ContentHintResendable, urgent: d.Urgent})
	return ts, results, nil
}

// renderable: what non-admins may not send in an announcement group.
func renderable(dm *signalpb.AppDataMessage) bool {
	return dm.Body != nil || len(dm.Attachments) > 0 || dm.Quote != nil || len(dm.Preview) > 0 || len(dm.BodyRanges) > 0 || dm.Sticker != nil
}

// SendTyping sends a typing message to a group (§12.1).
func (m *Groups) SendTyping(ctx context.Context, id []byte, stop bool) (uint64, []SendResult, error) {
	rec, st, err := m.memberGroup(ctx, id)
	if err != nil {
		return 0, nil, err
	}
	if st.Announcements == groups.Enabled && st.Member(m.Acct.Data.ACI).Role != groups.RoleAdmin {
		return 0, nil, ErrNotAllowed
	}
	ts := m.Sender.timestamp()
	action := signalpb.AppTyping_STARTED
	if stop {
		action = signalpb.AppTyping_STOPPED
	}
	b, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_TypingMessage{TypingMessage: &signalpb.AppTyping{
		Timestamp: proto.Uint64(ts), Action: action.Enum(), GroupId: rec.GroupID}}})
	return ts, m.sendGroup(ctx, rec, m.others(st, false), b, nil, ts, groupSendOpts{hint: protocolHintImplicit, online: true}), nil
}

// SendStory posts a group story (§13): sender key only, no fan-out.
func (m *Groups) SendStory(ctx context.Context, id []byte, src attachment.Source, allowsReplies bool) (uint64, []SendResult, error) {
	rec, st, err := m.memberGroup(ctx, id)
	if err != nil {
		return 0, nil, err
	}
	story, err := m.Sender.storyMessage(ctx, src, allowsReplies)
	if err != nil {
		return 0, nil, err
	}
	story.Group = &signalpb.AppGroupContextV2{MasterKey: rec.MasterKey, Revision: proto.Uint32(st.Revision)}
	ts := m.Sender.timestamp()
	b, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: story}})
	results := m.sendGroup(ctx, rec, m.others(st, false), b, nil, ts, groupSendOpts{hint: protocolHintImplicit, story: true})
	if m.Sender.Acct.IsMultiDevice() {
		var ok []string
		for _, r := range results {
			if r.Err == nil {
				ok = append(ok, r.Dest)
			}
		}
		if len(ok) > 0 {
			m.Sender.storyTranscriptDist(ctx, story, ok, ts, rec.DistributionID)
		}
	}
	return ts, results, nil
}

// handleGroupContexts updates group state for every group context in the
// content (groups.md §6.2) and applies §6.4's checks; false drops the
// message.
func (r *Receiver) handleGroupContexts(ctx context.Context, sender string, c *signalpb.AppContent) bool {
	sent := c.GetSyncMessage().GetSent()
	for _, gc := range []*signalpb.AppGroupContextV2{c.GetDataMessage().GetGroupV2(), c.GetEditMessage().GetDataMessage().GetGroupV2(),
		c.GetStoryMessage().GetGroup(), sent.GetMessage().GetGroupV2(), sent.GetStoryMessage().GetGroup()} {
		if gc == nil || len(gc.GetMasterKey()) != 32 {
			continue
		}
		before := r.Groups.Acct.Group(r.Groups.recordFor(gc.GetMasterKey()).GroupID)
		allowedBefore := before == nil || groupAllows(StateOf(before), sender, c)
		rec := r.Groups.HandleContext(ctx, gc)
		if rec.Blocked {
			return false
		}
		if sender != r.Acct.Data.ACI && !allowedBefore && !groupAllows(StateOf(rec), sender, c) {
			return false
		}
	}
	return true
}

// groupAllows is §6.4's membership rule for a sender.
func groupAllows(st *groups.State, sender string, c *signalpb.AppContent) bool {
	if st == nil {
		return true
	}
	dm := c.GetDataMessage()
	if st.Member(sender) == nil {
		isUpdate := dm != nil && len(dm.GetGroupV2().GetGroupChange()) > 0 && !renderable(dm)
		return st.PendingIndex(sender) >= 0 && isUpdate
	}
	if st.Announcements == groups.Enabled && st.Member(sender).Role != groups.RoleAdmin {
		return dm != nil && !renderable(dm)
	}
	return true
}
