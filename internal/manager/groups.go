// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Groups v2 operations (spec/service/groups.md §5-11).

// Groups manages an account's v2 groups.
type Groups struct {
	Acct          *store.Account
	Svc           *GroupsService
	Sender        *Sender
	AvatarsDir    string
	IgnoreAvatars bool
	Log           func(format string, args ...any)
}

func (m *Groups) logf(format string, args ...any) {
	if m.Log != nil {
		m.Log(format, args...)
	}
}

func (m *Groups) now() time.Time { return m.Svc.now() }

// Secret derives a record's group secret params.
func Secret(rec *store.GroupRecord) *zkgroup.GroupSecretParams {
	var mk [32]byte
	copy(mk[:], rec.MasterKey)
	return zkgroup.DeriveGroupSecretParams(mk)
}

// StateOf decodes a record's state; nil without one.
func StateOf(rec *store.GroupRecord) *groups.State {
	if len(rec.State) == 0 {
		return nil
	}
	var s groups.State
	if json.Unmarshal(rec.State, &s) != nil {
		return nil
	}
	return &s
}

func setState(rec *store.GroupRecord, s *groups.State) {
	if s == nil {
		rec.State = nil
		return
	}
	rec.State, _ = json.Marshal(s)
	rec.PermissionDenied = false
}

func newDistributionID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return formatUUIDBytes(b[:])
}

// recordFor returns the stored record for a master key, or a new
// unsaved one (§6.2 step 1).
func (m *Groups) recordFor(masterKey []byte) *store.GroupRecord {
	var mk [32]byte
	copy(mk[:], masterKey)
	id := zkgroup.DeriveGroupSecretParams(mk).GroupID
	if rec := m.Acct.Group(id[:]); rec != nil {
		return rec
	}
	return &store.GroupRecord{GroupID: id[:], MasterKey: append([]byte{}, masterKey...), DistributionID: newDistributionID()}
}

// save stores a record (inserting it if new) and returns the stored copy.
func (m *Groups) save(rec *store.GroupRecord) *store.GroupRecord {
	if stored := m.Acct.Group(rec.GroupID); stored != nil {
		if stored != rec {
			*stored = *rec
		}
	} else {
		m.Acct.Groups = append(m.Acct.Groups, *rec)
	}
	if err := m.Acct.SaveGroups(); err != nil {
		m.logf("Storing group failed: %v", err)
	}
	return m.Acct.Group(rec.GroupID)
}

// Group returns a record by group ID after the lazy fill (§6.1); nil if
// unknown.
func (m *Groups) Group(ctx context.Context, id []byte, force bool) *store.GroupRecord {
	rec := m.Acct.Group(id)
	if rec == nil {
		return nil
	}
	m.Refresh(ctx, rec, force)
	return m.Acct.Group(id)
}

// Refresh is §6.1's fill/refresh.
func (m *Groups) Refresh(ctx context.Context, rec *store.GroupRecord, force bool) {
	if !force && (rec.PermissionDenied || len(rec.State) > 0) {
		return
	}
	g := Secret(rec)
	resp, err := m.Svc.GetGroup(ctx, g)
	if IsNotInGroup(err) {
		rec.PermissionDenied = true
		m.save(rec)
		return
	}
	if err != nil {
		m.logf("Failed to retrieve Group V2 info, ignoring: %v", err)
		return
	}
	st, err := groups.DecryptGroup(g, resp.GetGroup())
	if err != nil {
		m.logf("Failed to retrieve Group V2 info, ignoring: %v", err)
		return
	}
	old := StateOf(rec)
	m.storeEndorsements(rec, g, resp.GetGroupSendEndorsementsResponse(), st)
	m.historyKeys(ctx, rec, g, old, st)
	m.learnMemberKeys(st)
	m.downloadAvatar(ctx, rec, g, st.Avatar)
	setState(rec, st)
	m.save(rec)
}

// storeEndorsements receives an endorsement response for the full
// members and stores every member's endorsement (§5, crypto §9.4).
func (m *Groups) storeEndorsements(rec *store.GroupRecord, g *zkgroup.GroupSecretParams, resp []byte, st *groups.State) {
	if len(resp) == 0 || st.Member(m.Acct.Data.ACI) == nil {
		return
	}
	var members []zkgroup.ServiceID
	for _, mem := range st.Members {
		id, err := groups.ParseServiceID(mem.ACI)
		if err != nil {
			return
		}
		members = append(members, id)
	}
	res, err := zkgroup.ReceiveGroupSendEndorsements(paramsFor(m.Acct), g, resp, members, uint64(m.now().Unix()))
	if err != nil {
		m.logf("Failed to receive group send endorsements: %v", err)
		return
	}
	rec.Endorsements = map[string][]byte{}
	for i, mem := range st.Members {
		rec.Endorsements[mem.ACI] = res.Endorsements[i].Serialize()
	}
	rec.EndorsementExpiration = int64(res.Expiration) * 1000
}

// historyKeys is §6.3: authoritative profile keys from the change log,
// stored where none is known.
func (m *Groups) historyKeys(ctx context.Context, rec *store.GroupRecord, g *zkgroup.GroupSecretParams, old, st *groups.State) {
	start := st.Revision
	if self := st.Member(m.Acct.Data.ACI); self != nil {
		start = self.JoinedAt
	}
	if old != nil && old.Revision > start {
		start = old.Revision
	}
	entries, err := m.Svc.Logs(ctx, g, start, rec.EndorsementExpiration)
	if err != nil && !IsNotInGroup(err) {
		m.logf("Failed to retrieve Group V2 history, ignoring: %v", err)
	}
	keys := map[string][]byte{}
	for _, e := range entries {
		if e.GetGroupChange() == nil {
			continue
		}
		c, err := groups.DecryptTrustedChange(g, e.GetGroupChange())
		if err != nil {
			continue
		}
		if aci, key, ok := groups.ProfileKeysFromChange(c); ok {
			keys[aci] = key
		}
	}
	changed := false
	for aci, key := range keys {
		if rc := m.Acct.Recipients[aci]; len(rc.ProfileKey) != 32 && aci != m.Acct.Data.ACI {
			rc.ProfileKey = key
			m.Acct.Recipients[aci] = rc
			changed = true
		}
	}
	if changed {
		m.Acct.SaveRecipients()
	}
}

// learnMemberKeys stores members' and requesters' profile keys where none
// is known (§4.4).
func (m *Groups) learnMemberKeys(st *groups.State) {
	changed := false
	learn := func(aci string, key []byte) {
		if len(key) != 32 || aci == m.Acct.Data.ACI {
			return
		}
		if rc := m.Acct.Recipients[aci]; len(rc.ProfileKey) != 32 {
			rc.ProfileKey = key
			m.Acct.Recipients[aci] = rc
			changed = true
		}
	}
	for _, mem := range st.Members {
		learn(mem.ACI, mem.ProfileKey)
	}
	for _, r := range st.Requesting {
		learn(r.ACI, r.ProfileKey)
	}
	if changed {
		m.Acct.SaveRecipients()
	}
}

// AvatarFile is the stored avatar of a group (§3.5.7).
func (m *Groups) AvatarFile(id []byte) string {
	return filepath.Join(m.AvatarsDir, "group-"+strings.ReplaceAll(groups.FormatGroupID(id), "/", "_"))
}

func (m *Groups) downloadAvatar(ctx context.Context, rec *store.GroupRecord, g *zkgroup.GroupSecretParams, key string) {
	if key == "" || m.AvatarsDir == "" {
		return
	}
	img, err := m.Svc.DownloadAvatar(ctx, g, key)
	if err == nil {
		err = os.MkdirAll(m.AvatarsDir, 0o700)
	}
	if err == nil {
		err = os.WriteFile(m.AvatarFile(rec.GroupID), img, 0o644)
	}
	if err != nil {
		m.logf("Failed to download avatar for group %s, ignoring: %v", groups.FormatGroupID(rec.GroupID), err)
	}
}

// HandleContext is §6.2 for an incoming GroupContextV2. It returns the
// stored record.
func (m *Groups) HandleContext(ctx context.Context, gc *signalpb.AppGroupContextV2) *store.GroupRecord {
	rec := m.recordFor(gc.GetMasterKey())
	rev := gc.GetRevision()
	st := StateOf(rec)
	if st != nil && st.Revision >= rev {
		return rec
	}
	g := Secret(rec)
	var next *groups.State
	if len(gc.GetGroupChange()) > 0 && st != nil && st.Revision+1 == rev {
		var signed signalpb.GroupChange
		if proto.Unmarshal(gc.GetGroupChange(), &signed) == nil {
			if c, err := groups.DecryptSignedChange(g, paramsFor(m.Acct), &signed); err == nil {
				if aci, key, ok := groups.ProfileKeysFromChange(c); ok && aci != m.Acct.Data.ACI {
					rc := m.Acct.Recipients[aci]
					rc.ProfileKey = key
					m.Acct.Recipients[aci] = rc
					m.Acct.SaveRecipients()
				}
				next, _ = groups.Apply(st, c)
			}
		}
	}
	if next == nil {
		if resp, err := m.Svc.GetGroup(ctx, g); err == nil {
			if s, err := groups.DecryptGroup(g, resp.GetGroup()); err == nil {
				m.historyKeys(ctx, rec, g, st, s)
				m.storeEndorsements(rec, g, resp.GetGroupSendEndorsementsResponse(), s)
				next = s
			}
		} else if !IsNotInGroup(err) {
			m.logf("Failed to retrieve Group V2 info, ignoring: %v", err)
		}
	}
	if next != nil {
		m.learnMemberKeys(next)
		if !m.IgnoreAvatars {
			m.downloadAvatar(ctx, rec, g, next.Avatar)
		}
	}
	// The state is replaced even when nothing was obtained (§6.2 step 5).
	setState(rec, next)
	return m.save(rec)
}

// Group operation errors (§7-10).
var (
	ErrNoOwnCredential   = errors.New("Cannot create a V2 group as self does not have a versioned profile")
	ErrNoOwnCredentialJn = errors.New("Cannot join a V2 group as self does not have a versioned profile")
	ErrCreateFailed      = errors.New("Failed to create group")
	ErrLastAdmin         = errors.New("last admin")
	ErrStillMember       = errors.New("The local group information cannot be removed, as the user is still a member of the group")
	ErrAlreadyRequested  = errors.New("You have already requested to join the group.")
)

// GroupLinkInactiveError is a join link that is not active.
type GroupLinkInactiveError struct{ Reason string }

func (e *GroupLinkInactiveError) Error() string {
	return "Group link inactive (reason: " + e.Reason + ")"
}

func (m *Groups) random32() [32]byte { return m.Svc.random32() }

func (m *Groups) blob(g *zkgroup.GroupSecretParams, b *signalpb.GroupAttributeBlob) []byte {
	raw, _ := proto.Marshal(b)
	return g.EncryptBlob(m.random32(), raw, 0)
}

func (m *Groups) sid(g *zkgroup.GroupSecretParams, id string) []byte {
	sid, err := groups.ParseServiceID(id)
	if err != nil {
		return nil
	}
	return g.EncryptServiceID(sid)
}

// fetchCredentials gets profile-key credentials for members (§7 step 2).
func (m *Groups) fetchCredentials(ctx context.Context, acis []string) map[string]*zkgroup.ExpiringProfileKeyCredential {
	out := map[string]*zkgroup.ExpiringProfileKeyCredential{}
	for _, aci := range acis {
		if c, err := m.Sender.ProfileKeyCredential(ctx, aci); err == nil {
			out[aci] = c
		}
	}
	return out
}

// GroupResult is the outcome of a group command that sends a message.
type GroupResult struct {
	Record    *store.GroupRecord
	Timestamp uint64
	Results   []SendResult
}

// Create is §7: a new group with us as admin, members with credentials,
// invitees for the rest.
func (m *Groups) Create(ctx context.Context, name string, members []string, avatar []byte) (*GroupResult, error) {
	self := m.Acct.Data.ACI
	own, err := m.Sender.ProfileKeyCredential(ctx, self)
	if err != nil {
		m.logf("Cannot create a V2 group as self does not have a versioned profile")
		return nil, ErrNoOwnCredential
	}
	var others []string
	for _, aci := range members {
		if aci != self {
			others = append(others, aci)
		}
	}
	creds := m.fetchCredentials(ctx, others)
	var mk [32]byte
	rand.Read(mk[:])
	g := zkgroup.DeriveGroupSecretParams(mk)
	params := paramsFor(m.Acct)
	group := &signalpb.Group{
		PublicKey:                 g.PublicParams(),
		Title:                     m.blob(g, &signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_Title{Title: name}}),
		DisappearingMessagesTimer: m.blob(g, &signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_DisappearingMessagesDuration{}}),
		AccessControl:             &signalpb.GroupAccessControl{Attributes: signalpb.GroupAccessControl_MEMBER, Members: signalpb.GroupAccessControl_MEMBER},
		Members:                   []*signalpb.GroupMember{{Role: signalpb.GroupMember_ADMINISTRATOR, Presentation: own.Present(params, g, m.random32())}},
	}
	for _, aci := range others {
		if c, ok := creds[aci]; ok {
			group.Members = append(group.Members, &signalpb.GroupMember{Role: signalpb.GroupMember_DEFAULT, Presentation: c.Present(params, g, m.random32())})
		} else {
			group.MembersPendingProfileKey = append(group.MembersPendingProfileKey, &signalpb.GroupMemberPendingProfileKey{
				Member: &signalpb.GroupMember{UserId: m.sid(g, aci), Role: signalpb.GroupMember_DEFAULT}})
		}
	}
	if avatar != nil {
		key, err := m.Svc.UploadAvatar(ctx, g, avatar)
		if err != nil {
			m.logf("Failed to create V2 group: %v", err)
			return nil, ErrCreateFailed
		}
		group.AvatarUrl = key
	}
	if err := m.Svc.Create(ctx, g, group); err != nil {
		m.logf("Failed to create V2 group: %v", err)
		return nil, ErrCreateFailed
	}
	resp, err := m.Svc.GetGroup(ctx, g)
	if err != nil {
		m.logf("Failed to create V2 group: %v", err)
		return nil, ErrCreateFailed
	}
	st, err := groups.DecryptGroup(g, resp.GetGroup())
	if err != nil {
		m.logf("Failed to create V2 group: %v", err)
		return nil, ErrCreateFailed
	}
	rec := &store.GroupRecord{GroupID: g.GroupID[:], MasterKey: mk[:], DistributionID: newDistributionID(), ProfileSharing: true}
	m.storeEndorsements(rec, g, resp.GetGroupSendEndorsementsResponse(), st)
	setState(rec, st)
	rec = m.save(rec)
	if avatar != nil && m.AvatarsDir != "" {
		os.MkdirAll(m.AvatarsDir, 0o700)
		os.WriteFile(m.AvatarFile(rec.GroupID), avatar, 0o644)
	}
	ts, results := m.sendUpdate(ctx, rec, nil, st, nil)
	return &GroupResult{Record: rec, Timestamp: ts, Results: results}, nil
}

// commit is §8.2: apply locally, PATCH, keep the local state, send the
// update message.
func (m *Groups) commit(ctx context.Context, rec *store.GroupRecord, a *signalpb.GroupChange_Actions) (*GroupResult, error) {
	st := StateOf(rec)
	if st == nil {
		return nil, errors.New("no group state")
	}
	g := Secret(rec)
	a.Version = st.Revision + 1
	// Accepting an invitation sent to our PNI carries only a presentation;
	// the server adds the PNI and the ciphertexts, so that change is
	// applied from the server's signed copy instead (groups.md §8.2 row 1).
	fromServer := len(a.GetPromoteMembersPendingPniAciProfileKey()) > 0
	var next *groups.State
	if !fromServer {
		c, err := groups.DecryptActions(g, a, m.Acct.Data.ACI)
		if err != nil {
			return nil, err
		}
		if next, err = groups.Apply(st, c); err != nil {
			return nil, err
		}
	}
	resp, err := m.Svc.Patch(ctx, g, a, nil)
	if err != nil {
		return nil, err
	}
	if fromServer {
		c, err := groups.DecryptTrustedChange(g, resp.GetGroupChange())
		if err != nil {
			return nil, err
		}
		if next, err = groups.Apply(st, c); err != nil {
			return nil, err
		}
	}
	setState(rec, next)
	m.storeEndorsements(rec, g, resp.GetGroupSendEndorsementsResponse(), next)
	rec = m.save(rec)
	signed, _ := proto.Marshal(resp.GetGroupChange())
	ts, results := m.sendUpdate(ctx, rec, st, next, signed)
	return &GroupResult{Record: rec, Timestamp: ts, Results: results}, nil
}

// updateRecipients are members and invitees before and after, minus self
// (§8.3), including invitees known only by their PNI (the reference sends
// to them too: live 2026-10-29, its invitations by PNI reached A and B).
func (m *Groups) updateRecipients(states ...*groups.State) []string {
	seen := map[string]bool{m.Acct.Data.ACI: true, "PNI:" + m.Acct.Data.PNI: true}
	var out []string
	add := func(id string) {
		if !seen[id] && id != groups.UnknownACI {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, st := range states {
		if st == nil {
			continue
		}
		for _, mem := range st.Members {
			add(mem.ACI)
		}
		for _, p := range st.Pending {
			add(p.ServiceID)
		}
	}
	return out
}

// sendUpdate sends the group-update message (§8.3).
func (m *Groups) sendUpdate(ctx context.Context, rec *store.GroupRecord, before, after *groups.State, signed []byte) (uint64, []SendResult) {
	gc := &signalpb.AppGroupContextV2{MasterKey: rec.MasterKey, Revision: proto.Uint32(after.Revision)}
	if len(signed) > 0 && len(signed) <= 2048 {
		gc.GroupChange = signed
	}
	ts := uint64(m.now().UnixMilli())
	dm := &signalpb.AppDataMessage{Timestamp: proto.Uint64(ts), GroupV2: gc, ExpireTimerVersion: proto.Uint32(1)}
	if after.Timer > 0 {
		dm.ExpireTimer = proto.Uint32(after.Timer)
	}
	c, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: dm}})
	results := m.sendGroup(ctx, rec, m.updateRecipients(before, after), c, dm, ts, groupSendOpts{hint: protocolHintImplicit, urgent: true})
	return ts, results
}

// UpdateOptions are updateGroup's changes (§8.1).
type UpdateOptions struct {
	Name, Description *string
	Avatar            []byte
	Add, Remove       []string // ACIs
	Admins, NoAdmins  []string
	Ban, Unban        []string
	ResetLink         bool
	Link              string // "", "enabled", "enabled-with-approval", "disabled"
	AddMemberPerm     string // "", "every-member", "only-admins"
	EditDetailsPerm   string
	SendMessagesPerm  string
	Expiration        *uint32
	LabelEmoji        *string
	Label             *string
}

func accessFromPerm(p string) signalpb.GroupAccessControl_AccessRequired {
	if p == "only-admins" {
		return signalpb.GroupAccessControl_ADMINISTRATOR
	}
	return signalpb.GroupAccessControl_MEMBER
}

func newInvitePassword() []byte {
	b := make([]byte, 16)
	rand.Read(b)
	return b
}

// Update runs §8.1's steps, one PATCH each, with one conflict retry.
func (m *Groups) Update(ctx context.Context, rec *store.GroupRecord, o UpdateOptions) (*GroupResult, error) {
	res, err := m.updateSteps(ctx, rec, o)
	if IsConflict(err) {
		m.Refresh(ctx, rec, true)
		rec = m.Acct.Group(rec.GroupID)
		res, err = m.updateSteps(ctx, rec, o)
	}
	return res, err
}

func (m *Groups) updateSteps(ctx context.Context, rec *store.GroupRecord, o UpdateOptions) (*GroupResult, error) {
	var last *GroupResult
	self := m.Acct.Data.ACI
	params := paramsFor(m.Acct)
	step := func(a *signalpb.GroupChange_Actions) error {
		r, err := m.commit(ctx, rec, a)
		if err != nil {
			return err
		}
		rec, last = r.Record, r
		return nil
	}
	g := Secret(rec)
	st := func() *groups.State { return StateOf(rec) }
	contains := func(list []string, x string) bool {
		for _, v := range list {
			if v == x {
				return true
			}
		}
		return false
	}
	// 1. accept an invitation
	if s := st(); s != nil && s.Member(self) == nil {
		byACI := s.PendingIndex(self) >= 0
		byPNI := s.PendingIndex("PNI:"+m.Acct.Data.PNI) >= 0
		if byACI || byPNI {
			own, err := m.Sender.ProfileKeyCredential(ctx, self)
			if err != nil {
				return nil, ErrNoOwnCredentialJn
			}
			pres := own.Present(params, g, m.random32())
			a := &signalpb.GroupChange_Actions{}
			if byACI {
				a.PromoteMembersPendingProfileKey = []*signalpb.GroupChange_Actions_PromoteMemberPendingProfileKey{{Presentation: pres}}
			} else {
				a.PromoteMembersPendingPniAciProfileKey = []*signalpb.GroupChange_Actions_PromoteMemberPendingPniAciProfileKey{{Presentation: pres}}
			}
			if err := step(a); err != nil {
				return last, err
			}
		}
	}
	// 2. approve requesting members
	var approve, add []string
	for _, aci := range o.Add {
		s := st()
		switch {
		case s.Member(aci) != nil:
		case s.RequestingIndex(aci) >= 0:
			approve = append(approve, aci)
		default:
			add = append(add, aci)
		}
	}
	if len(approve) > 0 {
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range approve {
			a.PromoteMembersPendingAdminApproval = append(a.PromoteMembersPendingAdminApproval,
				&signalpb.GroupChange_Actions_PromoteMemberPendingAdminApproval{UserId: m.sid(g, aci), Role: signalpb.GroupMember_DEFAULT})
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	// 3. add members (credential) or invite (none); unban as needed
	if len(add) > 0 {
		creds := m.fetchCredentials(ctx, add)
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range add {
			if c, ok := creds[aci]; ok {
				a.AddMembers = append(a.AddMembers, &signalpb.GroupChange_Actions_AddMember{Added: &signalpb.GroupMember{Role: signalpb.GroupMember_DEFAULT, Presentation: c.Present(params, g, m.random32())}})
			} else {
				a.AddMembersPendingProfileKey = append(a.AddMembersPendingProfileKey, &signalpb.GroupChange_Actions_AddMemberPendingProfileKey{
					Added: &signalpb.GroupMemberPendingProfileKey{Member: &signalpb.GroupMember{UserId: m.sid(g, aci), Role: signalpb.GroupMember_DEFAULT}, AddedByUserId: m.sid(g, self)}})
			}
			if st().IsBanned(aci) {
				a.DeleteMembersBanned = append(a.DeleteMembersBanned, &signalpb.GroupChange_Actions_DeleteMemberBanned{DeletedUserId: m.sid(g, aci)})
			}
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	// 4. remove full members (-r and --ban, minus -m, minus self)
	var remove []string
	for _, aci := range append(append([]string{}, o.Remove...), o.Ban...) {
		if aci != self && !contains(o.Add, aci) && st().Member(aci) != nil && !contains(remove, aci) {
			remove = append(remove, aci)
		}
	}
	if len(remove) > 0 {
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range remove {
			a.DeleteMembers = append(a.DeleteMembers, &signalpb.GroupChange_Actions_DeleteMember{DeletedUserId: m.sid(g, aci)})
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	// 5. revoke invitations, 6. refuse requests
	var revoke [][]byte
	var refuse []string
	for _, aci := range o.Remove {
		s := st()
		if i := s.PendingIndex(aci); i >= 0 {
			revoke = append(revoke, s.Pending[i].Ciphertext)
		}
		if s.RequestingIndex(aci) >= 0 {
			refuse = append(refuse, aci)
		}
	}
	if len(revoke) > 0 {
		a := &signalpb.GroupChange_Actions{}
		for _, ct := range revoke {
			a.DeleteMembersPendingProfileKey = append(a.DeleteMembersPendingProfileKey, &signalpb.GroupChange_Actions_DeleteMemberPendingProfileKey{DeletedUserId: ct})
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	if len(refuse) > 0 {
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range refuse {
			a.DeleteMembersPendingAdminApproval = append(a.DeleteMembersPendingAdminApproval, &signalpb.GroupChange_Actions_DeleteMemberPendingAdminApproval{DeletedUserId: m.sid(g, aci)})
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	// 7, 8. roles
	roles := func(list []string, want int32, pick func(*groups.Member) bool) error {
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range list {
			if mem := st().Member(aci); mem != nil && pick(mem) {
				a.ModifyMemberRoles = append(a.ModifyMemberRoles, &signalpb.GroupChange_Actions_ModifyMemberRole{UserId: m.sid(g, aci), Role: signalpb.GroupMember_Role(want)})
			}
		}
		if len(a.ModifyMemberRoles) == 0 {
			return nil
		}
		return step(a)
	}
	if err := roles(o.Admins, groups.RoleAdmin, func(mem *groups.Member) bool { return mem.Role != groups.RoleAdmin }); err != nil {
		return last, err
	}
	if err := roles(o.NoAdmins, groups.RoleDefault, func(mem *groups.Member) bool { return mem.Role == groups.RoleAdmin }); err != nil {
		return last, err
	}
	// 9. ban, 10. unban
	var ban []string
	for _, aci := range o.Ban {
		if !st().IsBanned(aci) {
			ban = append(ban, aci)
		}
	}
	if len(ban) > 0 {
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range ban {
			a.AddMembersBanned = append(a.AddMembersBanned, &signalpb.GroupChange_Actions_AddMemberBanned{Added: &signalpb.GroupMemberBanned{UserId: m.sid(g, aci)}})
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	var unban []string
	for _, aci := range o.Unban {
		if st().IsBanned(aci) {
			unban = append(unban, aci)
		}
	}
	if len(unban) > 0 {
		a := &signalpb.GroupChange_Actions{}
		for _, aci := range unban {
			a.DeleteMembersBanned = append(a.DeleteMembersBanned, &signalpb.GroupChange_Actions_DeleteMemberBanned{DeletedUserId: m.sid(g, aci)})
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	// 11. reset link
	if o.ResetLink {
		if err := step(&signalpb.GroupChange_Actions{ModifyInviteLinkPassword: &signalpb.GroupChange_Actions_ModifyInviteLinkPassword{InviteLinkPassword: newInvitePassword()}}); err != nil {
			return last, err
		}
	}
	// 12. link
	if o.Link != "" {
		access := map[string]signalpb.GroupAccessControl_AccessRequired{"enabled": signalpb.GroupAccessControl_ANY,
			"enabled-with-approval": signalpb.GroupAccessControl_ADMINISTRATOR, "disabled": signalpb.GroupAccessControl_UNSATISFIABLE}[o.Link]
		a := &signalpb.GroupChange_Actions{ModifyAddFromInviteLinkAccess: &signalpb.GroupChange_Actions_ModifyAddFromInviteLinkAccess{AddFromInviteLinkAccess: access}}
		if o.Link != "disabled" && len(st().InvitePassword) == 0 {
			a.ModifyInviteLinkPassword = &signalpb.GroupChange_Actions_ModifyInviteLinkPassword{InviteLinkPassword: newInvitePassword()}
		}
		if err := step(a); err != nil {
			return last, err
		}
	}
	// 13-16. permissions, timer, announcements
	if o.AddMemberPerm != "" {
		if err := step(&signalpb.GroupChange_Actions{ModifyMemberAccess: &signalpb.GroupChange_Actions_ModifyMembersAccess{MembersAccess: accessFromPerm(o.AddMemberPerm)}}); err != nil {
			return last, err
		}
	}
	if o.EditDetailsPerm != "" {
		if err := step(&signalpb.GroupChange_Actions{ModifyAttributesAccess: &signalpb.GroupChange_Actions_ModifyAttributesAccess{AttributesAccess: accessFromPerm(o.EditDetailsPerm)}}); err != nil {
			return last, err
		}
	}
	if o.Expiration != nil {
		timer := m.blob(g, &signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_DisappearingMessagesDuration{DisappearingMessagesDuration: *o.Expiration}})
		if err := step(&signalpb.GroupChange_Actions{ModifyDisappearingMessageTimer: &signalpb.GroupChange_Actions_ModifyDisappearingMessageTimer{Timer: timer}}); err != nil {
			return last, err
		}
	}
	if o.SendMessagesPerm != "" {
		if err := step(&signalpb.GroupChange_Actions{ModifyAnnouncementsOnly: &signalpb.GroupChange_Actions_ModifyAnnouncementsOnly{AnnouncementsOnly: o.SendMessagesPerm == "only-admins"}}); err != nil {
			return last, err
		}
	}
	// 17. own member label
	if o.LabelEmoji != nil || o.Label != nil {
		mem := st().Member(self)
		if mem == nil {
			return last, errors.New("not a member")
		}
		emoji, text := mem.LabelEmoji, ""
		if o.LabelEmoji != nil {
			emoji = *o.LabelEmoji
		}
		if o.Label != nil {
			text = *o.Label
		} else {
			text = mem.LabelString
		}
		raw := func(s string) []byte {
			if s == "" {
				return nil
			}
			return g.EncryptBlob(m.random32(), []byte(s), 0)
		}
		if err := step(&signalpb.GroupChange_Actions{ModifyMemberLabels: []*signalpb.GroupChange_Actions_ModifyMemberLabel{{UserId: m.sid(g, self), LabelEmoji: raw(emoji), LabelString: raw(text)}}}); err != nil {
			return last, err
		}
	}
	// 18. title, description, avatar
	if o.Name != nil || o.Description != nil || o.Avatar != nil {
		a := &signalpb.GroupChange_Actions{}
		if o.Name != nil {
			a.ModifyTitle = &signalpb.GroupChange_Actions_ModifyTitle{Title: m.blob(g, &signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_Title{Title: *o.Name}})}
		}
		if o.Description != nil {
			a.ModifyDescription = &signalpb.GroupChange_Actions_ModifyDescription{Description: m.blob(g, &signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_DescriptionText{DescriptionText: *o.Description}})}
		}
		if o.Avatar != nil {
			key, err := m.Svc.UploadAvatar(ctx, g, o.Avatar)
			if err != nil {
				return last, err
			}
			a.ModifyAvatar = &signalpb.GroupChange_Actions_ModifyAvatar{Avatar: key}
		}
		if err := step(a); err != nil {
			return last, err
		}
		if o.Avatar != nil && m.AvatarsDir != "" {
			os.MkdirAll(m.AvatarsDir, 0o700)
			os.WriteFile(m.AvatarFile(rec.GroupID), o.Avatar, 0o644)
		}
	}
	return last, nil
}

// Join is §9: request or join through an invite link.
func (m *Groups) Join(ctx context.Context, link string) (*GroupResult, bool, error) {
	mk, password, err := groups.ParseInviteLink(link)
	if err != nil {
		return nil, false, err
	}
	g := zkgroup.DeriveGroupSecretParams(mk)
	info, err := m.Svc.JoinInfo(ctx, g, password)
	var ge *GroupError
	if errors.As(err, &ge) && ge.Class == "ForbiddenException" {
		reason := "UNKNOWN"
		if strings.EqualFold(ge.Reason, "banned") {
			reason = "BANNED"
		}
		return nil, false, &GroupLinkInactiveError{Reason: reason}
	}
	if err != nil {
		return nil, false, err
	}
	if info.GetPendingAdminApproval() {
		return nil, false, ErrAlreadyRequested
	}
	own, err := m.Sender.ProfileKeyCredential(ctx, m.Acct.Data.ACI)
	if err != nil {
		return nil, false, ErrNoOwnCredentialJn
	}
	pres := own.Present(paramsFor(m.Acct), g, m.random32())
	a := &signalpb.GroupChange_Actions{Version: info.GetVersion() + 1}
	if info.GetAddFromInviteLink() == signalpb.GroupAccessControl_ADMINISTRATOR {
		a.AddMembersPendingAdminApproval = []*signalpb.GroupChange_Actions_AddMemberPendingAdminApproval{{Added: &signalpb.GroupMemberPendingAdminApproval{Presentation: pres}}}
	} else {
		a.AddMembers = []*signalpb.GroupChange_Actions_AddMember{{Added: &signalpb.GroupMember{Role: signalpb.GroupMember_DEFAULT, Presentation: pres}}}
	}
	resp, err := m.Svc.Patch(ctx, g, a, password)
	if err != nil {
		return nil, false, err
	}
	signed, _ := proto.Marshal(resp.GetGroupChange())
	rec := m.HandleContext(ctx, &signalpb.AppGroupContextV2{MasterKey: mk[:], Revision: proto.Uint32(info.GetVersion() + 1), GroupChange: signed})
	st := StateOf(rec)
	if st == nil || st.Member(m.Acct.Data.ACI) == nil {
		return &GroupResult{Record: rec}, true, nil
	}
	if !rec.ProfileSharing {
		rec.ProfileSharing = true
		rec = m.save(rec)
	}
	m.storeEndorsements(rec, g, resp.GetGroupSendEndorsementsResponse(), st)
	m.save(rec)
	ts, results := m.sendUpdate(ctx, rec, nil, st, signed)
	return &GroupResult{Record: rec, Timestamp: ts, Results: results}, false, nil
}

// Quit is §10: leave (or revoke the own invitation), handing admin to
// others when we are the last admin.
func (m *Groups) Quit(ctx context.Context, rec *store.GroupRecord, admins []string) (*GroupResult, error) {
	self := m.Acct.Data.ACI
	st := StateOf(rec)
	if st == nil {
		return nil, errors.New("no group state")
	}
	var newAdmins []string
	for _, aci := range admins {
		if mem := st.Member(aci); mem != nil && mem.Role != groups.RoleAdmin {
			newAdmins = append(newAdmins, aci)
		}
	}
	if me := st.Member(self); me != nil && me.Role == groups.RoleAdmin && len(st.Members) > 1 && len(newAdmins) == 0 {
		adminCount := 0
		for _, mem := range st.Members {
			if mem.Role == groups.RoleAdmin {
				adminCount++
			}
		}
		if adminCount == 1 {
			return nil, ErrLastAdmin
		}
	}
	g := Secret(rec)
	a := &signalpb.GroupChange_Actions{}
	if i := st.PendingIndex(self); i >= 0 {
		a.DeleteMembersPendingProfileKey = []*signalpb.GroupChange_Actions_DeleteMemberPendingProfileKey{{DeletedUserId: st.Pending[i].Ciphertext}}
	} else if i := st.PendingIndex("PNI:" + m.Acct.Data.PNI); i >= 0 {
		a.DeleteMembersPendingProfileKey = []*signalpb.GroupChange_Actions_DeleteMemberPendingProfileKey{{DeletedUserId: st.Pending[i].Ciphertext}}
	} else {
		a.DeleteMembers = []*signalpb.GroupChange_Actions_DeleteMember{{DeletedUserId: m.sid(g, self)}}
		for _, aci := range newAdmins {
			a.ModifyMemberRoles = append(a.ModifyMemberRoles, &signalpb.GroupChange_Actions_ModifyMemberRole{UserId: m.sid(g, aci), Role: signalpb.GroupMember_ADMINISTRATOR})
		}
	}
	res, err := m.commitQuit(ctx, rec, a)
	if IsConflict(err) {
		m.Refresh(ctx, rec, true)
		rec = m.Acct.Group(rec.GroupID)
		res, err = m.commitQuit(ctx, rec, a)
	}
	return res, err
}

// commitQuit commits a leave; the update goes to the new state's members.
func (m *Groups) commitQuit(ctx context.Context, rec *store.GroupRecord, a *signalpb.GroupChange_Actions) (*GroupResult, error) {
	st := StateOf(rec)
	g := Secret(rec)
	a.Version = st.Revision + 1
	c, err := groups.DecryptActions(g, a, m.Acct.Data.ACI)
	if err != nil {
		return nil, err
	}
	next, err := groups.Apply(st, c)
	if err != nil {
		return nil, err
	}
	resp, err := m.Svc.Patch(ctx, g, a, nil)
	if err != nil {
		return nil, err
	}
	setState(rec, next)
	rec = m.save(rec)
	signed, _ := proto.Marshal(resp.GetGroupChange())
	ts, results := m.sendUpdate(ctx, rec, nil, next, signed)
	return &GroupResult{Record: rec, Timestamp: ts, Results: results}, nil
}

// Delete removes the local record of a group we are no longer in (§10
// step 8).
func (m *Groups) Delete(rec *store.GroupRecord) error {
	if st := StateOf(rec); st != nil && st.Member(m.Acct.Data.ACI) != nil {
		return ErrStillMember
	}
	os.Remove(m.AvatarFile(rec.GroupID))
	m.Acct.RemoveGroup(rec.GroupID)
	return m.Acct.SaveGroups()
}

// InviteLink is §5's invite link, or "" when there is none.
func InviteLink(rec *store.GroupRecord) string {
	st := StateOf(rec)
	if st == nil || len(st.InvitePassword) == 0 {
		return ""
	}
	if st.Access != nil && st.Access.AddFromInviteLink != groups.AccessUnknown &&
		st.Access.AddFromInviteLink != groups.AccessAny && st.Access.AddFromInviteLink != groups.AccessAdmin {
		return ""
	}
	return groups.InviteLinkURL(rec.MasterKey, st.InvitePassword)
}

// UpdateOwnProfileKey publishes a rotated own profile key to every v2
// group where the account is a full member with profile sharing on
// (groups.md §11). Per-group failures are logged and ignored.
func (m *Groups) UpdateOwnProfileKey(ctx context.Context) {
	self := m.Acct.Data.ACI
	for _, id := range m.groupIDs() {
		rec := m.Acct.Group(id)
		if rec == nil || !rec.ProfileSharing {
			continue
		}
		if st := StateOf(rec); st == nil || st.Member(self) == nil {
			continue
		}
		err := m.updateOwnKey(ctx, id)
		if IsConflict(err) {
			err = m.updateOwnKey(ctx, id)
		}
		if err != nil {
			m.logf("Failed to update group profile key: %v", err)
		}
	}
}

func (m *Groups) groupIDs() [][]byte {
	var ids [][]byte
	for _, rec := range m.Acct.Groups {
		ids = append(ids, rec.GroupID)
	}
	return ids
}

func (m *Groups) updateOwnKey(ctx context.Context, id []byte) error {
	rec := m.Acct.Group(id)
	m.Refresh(ctx, rec, true)
	rec = m.Acct.Group(id)
	st := StateOf(rec)
	if st == nil {
		return errors.New("no group state")
	}
	me := st.Member(m.Acct.Data.ACI)
	if me == nil || bytes.Equal(me.ProfileKey, m.Acct.Data.ProfileKey) {
		return nil
	}
	own, err := m.Sender.ProfileKeyCredential(ctx, m.Acct.Data.ACI)
	if err != nil {
		return err
	}
	pres := own.Present(paramsFor(m.Acct), Secret(rec), m.random32())
	_, err = m.commit(ctx, rec, &signalpb.GroupChange_Actions{
		ModifyMemberProfileKeys: []*signalpb.GroupChange_Actions_ModifyMemberProfileKey{{Presentation: pres}}})
	return err
}
