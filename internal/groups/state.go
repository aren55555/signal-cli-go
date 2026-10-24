package groups

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// Decrypted group state and changes (spec/service/groups.md §2.6, §4).
// Service IDs are strings: a bare UUID for an ACI, "PNI:" + UUID for a
// PNI.

// UnknownACI is what an undecryptable invitee becomes (§4.1).
const UnknownACI = "00000000-0000-0000-0000-000000000000"

// Roles.
const (
	RoleUnknown = int32(signalpb.GroupMember_UNKNOWN)
	RoleDefault = int32(signalpb.GroupMember_DEFAULT)
	RoleAdmin   = int32(signalpb.GroupMember_ADMINISTRATOR)
)

// Access levels.
const (
	AccessUnknown       = int32(signalpb.GroupAccessControl_UNKNOWN)
	AccessAny           = int32(signalpb.GroupAccessControl_ANY)
	AccessMember        = int32(signalpb.GroupAccessControl_MEMBER)
	AccessAdmin         = int32(signalpb.GroupAccessControl_ADMINISTRATOR)
	AccessUnsatisfiable = int32(signalpb.GroupAccessControl_UNSATISFIABLE)
)

// Enabled states of the announcements flag.
const (
	EnabledUnknown = 0
	Enabled        = 1
	Disabled       = 2
)

// Member is a full member.
type Member struct {
	ACI         string `json:"aci"`
	Role        int32  `json:"role"`
	ProfileKey  []byte `json:"profileKey,omitempty"`
	JoinedAt    uint32 `json:"joinedAt"`
	PNI         string `json:"pni,omitempty"`
	LabelEmoji  string `json:"labelEmoji,omitempty"`
	LabelString string `json:"labelString,omitempty"`
}

// PendingMember is an invitee.
type PendingMember struct {
	ServiceID  string `json:"serviceId"`
	Role       int32  `json:"role"`
	AddedBy    string `json:"addedBy"`
	Timestamp  uint64 `json:"timestamp"`
	Ciphertext []byte `json:"ciphertext"`
}

// RequestingMember asked to join through a link.
type RequestingMember struct {
	ACI        string `json:"aci"`
	ProfileKey []byte `json:"profileKey,omitempty"`
	Timestamp  uint64 `json:"timestamp"`
}

// BannedMember is a banned service ID.
type BannedMember struct {
	ServiceID string `json:"serviceId"`
	Timestamp uint64 `json:"timestamp"`
}

// Access is the access control; nil in State means none set.
type Access struct {
	Attributes        int32 `json:"attributes"`
	Members           int32 `json:"members"`
	AddFromInviteLink int32 `json:"addFromInviteLink"`
	MemberLabel       int32 `json:"memberLabel"`
}

// State is a decrypted group.
type State struct {
	Title          string             `json:"title"`
	Description    string             `json:"description"`
	Avatar         string             `json:"avatar"`
	Timer          uint32             `json:"timer"`
	Access         *Access            `json:"access,omitempty"`
	Revision       uint32             `json:"revision"`
	Members        []Member           `json:"members"`
	Pending        []PendingMember    `json:"pending,omitempty"`
	Requesting     []RequestingMember `json:"requesting,omitempty"`
	InvitePassword []byte             `json:"invitePassword,omitempty"`
	Announcements  int                `json:"announcements"`
	Banned         []BannedMember     `json:"banned,omitempty"`
	Terminated     bool               `json:"terminated,omitempty"`
}

// Member returns the full member with that ACI, or nil.
func (s *State) Member(aci string) *Member {
	for i := range s.Members {
		if s.Members[i].ACI == aci {
			return &s.Members[i]
		}
	}
	return nil
}

// PendingIndex returns the invitee with that service ID, or -1.
func (s *State) PendingIndex(id string) int {
	for i, p := range s.Pending {
		if p.ServiceID == id {
			return i
		}
	}
	return -1
}

// RequestingIndex returns the requester with that ACI, or -1.
func (s *State) RequestingIndex(aci string) int {
	for i, r := range s.Requesting {
		if r.ACI == aci {
			return i
		}
	}
	return -1
}

// IsBanned reports a ban for a service ID.
func (s *State) IsBanned(id string) bool {
	for _, b := range s.Banned {
		if b.ServiceID == id {
			return true
		}
	}
	return false
}

// ServiceIDString formats a zkgroup service ID.
func ServiceIDString(id zkgroup.ServiceID) string {
	h := hex.EncodeToString(id.UUID[:])
	u := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	if id.PNI {
		return "PNI:" + u
	}
	return u
}

// ParseServiceID is the inverse of ServiceIDString.
func ParseServiceID(s string) (zkgroup.ServiceID, error) {
	var id zkgroup.ServiceID
	if rest, ok := strings.CutPrefix(s, "PNI:"); ok {
		id.PNI, s = true, rest
	}
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(b) != 16 || len(s) != 36 {
		return id, fmt.Errorf("groups: bad service ID %q", s)
	}
	copy(id.UUID[:], b)
	return id, nil
}

// ErrInvalidGroup: the group or change could not be decrypted (§4.1).
var ErrInvalidGroup = errors.New("groups: invalid group content")

// decryptor decrypts the fields of one group.
type decryptor struct{ g *zkgroup.GroupSecretParams }

func (d decryptor) serviceID(ct []byte) (string, error) {
	id, err := d.g.DecryptServiceID(ct)
	if err != nil {
		return "", ErrInvalidGroup
	}
	return ServiceIDString(id), nil
}

func (d decryptor) aci(ct []byte) (string, error) {
	id, err := d.g.DecryptServiceID(ct)
	if err != nil || id.PNI {
		return "", ErrInvalidGroup
	}
	return ServiceIDString(id), nil
}

// orUnknown decrypts an invitee's service ID, or gives the unknown ACI.
func (d decryptor) orUnknown(ct []byte) string {
	id, err := d.serviceID(ct)
	if err != nil {
		return UnknownACI
	}
	return id
}

func (d decryptor) profileKey(ct []byte, aci string) ([]byte, error) {
	id, err := ParseServiceID(aci)
	if err != nil {
		return nil, ErrInvalidGroup
	}
	k, err := d.g.DecryptProfileKey(ct, id.UUID)
	if err != nil {
		return nil, ErrInvalidGroup
	}
	return k[:], nil
}

// presentation reads ACI and profile key from a profile-key credential
// presentation (zkgroup-credentials.md §8.5: ciphertexts at 585 and 649).
func (d decryptor) presentation(p []byte) (aci string, key []byte, err error) {
	if len(p) != 721 || p[0] != 0x03 {
		return "", nil, ErrInvalidGroup
	}
	if aci, err = d.aci(append([]byte{0}, p[585:649]...)); err != nil {
		return "", nil, err
	}
	key, err = d.profileKey(append([]byte{0}, p[649:713]...), aci)
	return aci, key, err
}

// memberIdentity is the ACI and profile key of a member entry: from the
// presentation if any, else from the two ciphertexts.
func (d decryptor) memberIdentity(userID, profileKey, presentation []byte) (string, []byte, error) {
	if len(presentation) > 0 {
		return d.presentation(presentation)
	}
	aci, err := d.aci(userID)
	if err != nil {
		return "", nil, err
	}
	key, err := d.profileKey(profileKey, aci)
	return aci, key, err
}

// blob decrypts a GroupAttributeBlob; short, undecryptable or unparsable
// input gives an empty blob (§4.1).
func (d decryptor) blob(b []byte) *signalpb.GroupAttributeBlob {
	var out signalpb.GroupAttributeBlob
	if len(b) < 29 {
		return &out
	}
	plain, err := d.g.DecryptBlob(b)
	if err != nil || proto.Unmarshal(plain, &out) != nil {
		return &signalpb.GroupAttributeBlob{}
	}
	return &out
}

func (d decryptor) rawBlob(b []byte) string {
	if len(b) < 29 {
		return ""
	}
	plain, err := d.g.DecryptBlob(b)
	if err != nil {
		return ""
	}
	return string(plain)
}

func announcements(on bool) int {
	if on {
		return Enabled
	}
	return Disabled
}

func normalRole(r signalpb.GroupMember_Role) int32 {
	if r == signalpb.GroupMember_DEFAULT || r == signalpb.GroupMember_ADMINISTRATOR {
		return int32(r)
	}
	return RoleDefault
}

// DecryptGroup is §4.1.
func DecryptGroup(g *zkgroup.GroupSecretParams, pb *signalpb.Group) (*State, error) {
	d := decryptor{g}
	s := &State{
		Title:          strings.TrimSpace(d.blob(pb.GetTitle()).GetTitle()),
		Description:    strings.TrimSpace(d.blob(pb.GetDescription()).GetDescriptionText()),
		Avatar:         pb.GetAvatarUrl(),
		Timer:          d.blob(pb.GetDisappearingMessagesTimer()).GetDisappearingMessagesDuration(),
		Revision:       pb.GetVersion(),
		InvitePassword: pb.GetInviteLinkPassword(),
		Announcements:  announcements(pb.GetAnnouncementsOnly()),
		Terminated:     pb.GetTerminated(),
	}
	if ac := pb.GetAccessControl(); ac != nil {
		s.Access = &Access{int32(ac.GetAttributes()), int32(ac.GetMembers()), int32(ac.GetAddFromInviteLink()), int32(ac.GetMemberLabel())}
	}
	for _, m := range pb.GetMembers() {
		aci, key, err := d.memberIdentity(m.GetUserId(), m.GetProfileKey(), m.GetPresentation())
		if err != nil {
			return nil, err
		}
		s.Members = append(s.Members, Member{ACI: aci, Role: int32(m.GetRole()), ProfileKey: key, JoinedAt: m.GetJoinedAtVersion(),
			LabelEmoji: d.rawBlob(m.GetLabelEmoji()), LabelString: d.rawBlob(m.GetLabelString())})
	}
	for _, p := range pb.GetMembersPendingProfileKey() {
		by, err := d.aci(p.GetAddedByUserId())
		if err != nil {
			return nil, err
		}
		s.Pending = append(s.Pending, PendingMember{ServiceID: d.orUnknown(p.GetMember().GetUserId()), Role: normalRole(p.GetMember().GetRole()),
			AddedBy: by, Timestamp: p.GetTimestamp(), Ciphertext: p.GetMember().GetUserId()})
	}
	for _, r := range pb.GetMembersPendingAdminApproval() {
		aci, key, err := d.memberIdentity(r.GetUserId(), r.GetProfileKey(), r.GetPresentation())
		if err != nil {
			return nil, err
		}
		ts := r.GetTimestamp()
		if len(r.GetPresentation()) > 0 {
			ts = 0
		}
		s.Requesting = append(s.Requesting, RequestingMember{ACI: aci, ProfileKey: key, Timestamp: ts})
	}
	for _, b := range pb.GetMembersBanned() {
		id, err := d.serviceID(b.GetUserId())
		if err != nil {
			return nil, err
		}
		s.Banned = append(s.Banned, BannedMember{ServiceID: id, Timestamp: b.GetTimestamp()})
	}
	return s, nil
}

// RoleChange sets a role.
type RoleChange struct {
	ACI  string
	Role int32
}

// PendingRemoval removes an invitee by ciphertext.
type PendingRemoval struct {
	ServiceID  string
	Ciphertext []byte
}

// LabelChange sets a member label.
type LabelChange struct {
	ACI                     string
	LabelEmoji, LabelString string
}

// Change is a decrypted GroupChange.Actions (§2.6, §4.2).
type Change struct {
	Editor               string
	Revision             uint32
	NewMembers           []Member
	JoinFromInviteLink   []bool
	DeleteMembers        []string
	ModifyRoles          []RoleChange
	ModifiedProfileKeys  []Member
	NewPending           []PendingMember
	DeletePending        []PendingRemoval
	PromotePending       []Member
	NewTitle             *string
	NewAvatar            *string
	NewTimer             *uint32
	NewAttributeAccess   int32
	NewMemberAccess      int32
	NewInviteLinkAccess  int32
	NewMemberLabelAccess int32
	NewRequesting        []RequestingMember
	DeleteRequesting     []string
	PromoteRequesting    []RoleChange
	NewInvitePassword    []byte
	NewDescription       *string
	NewAnnouncements     int
	NewBanned            []BannedMember
	DeleteBanned         []string
	PromotePniAci        []Member
	Labels               []LabelChange
	Terminate            bool
}

// HighestKnownEpoch is the newest change epoch understood (§2.4).
const HighestKnownEpoch = 7

// ErrUnknownEpoch: the change uses features newer than epoch 7.
var ErrUnknownEpoch = errors.New("groups: change epoch too new")

// DecryptSignedChange is §4.2's verified mode: epoch, notary signature
// over the raw actions bytes, then the group ID.
func DecryptSignedChange(g *zkgroup.GroupSecretParams, server *zkgroup.ServerPublicParams, c *signalpb.GroupChange) (*Change, error) {
	if c.GetChangeEpoch() > HighestKnownEpoch {
		return nil, ErrUnknownEpoch
	}
	if !zkgroup.VerifyNotarySignature(server.SigPublicKey, c.GetActions(), c.GetServerSignature()) {
		return nil, ErrInvalidGroup
	}
	var a signalpb.GroupChange_Actions
	if err := proto.Unmarshal(c.GetActions(), &a); err != nil {
		return nil, ErrInvalidGroup
	}
	if string(a.GetGroupId()) != string(g.GroupID[:]) {
		return nil, ErrInvalidGroup
	}
	return DecryptActions(g, &a, "")
}

// DecryptTrustedChange is §4.2's already-trusted mode (history pages).
func DecryptTrustedChange(g *zkgroup.GroupSecretParams, c *signalpb.GroupChange) (*Change, error) {
	if c.GetChangeEpoch() > HighestKnownEpoch {
		return nil, ErrUnknownEpoch
	}
	var a signalpb.GroupChange_Actions
	if err := proto.Unmarshal(c.GetActions(), &a); err != nil {
		return nil, ErrInvalidGroup
	}
	return DecryptActions(g, &a, "")
}

// DecryptActions decrypts actions; a non-empty editor overrides
// sourceUserId (own unsigned actions).
func DecryptActions(g *zkgroup.GroupSecretParams, a *signalpb.GroupChange_Actions, editor string) (*Change, error) {
	d := decryptor{g}
	c := &Change{Revision: a.GetVersion(), Editor: editor}
	if editor == "" && len(a.GetSourceUserId()) > 0 {
		id, err := d.serviceID(a.GetSourceUserId())
		if err != nil {
			return nil, err
		}
		c.Editor = id
	}
	for _, am := range a.GetAddMembers() {
		m := am.GetAdded()
		aci, key, err := d.memberIdentity(m.GetUserId(), m.GetProfileKey(), m.GetPresentation())
		if err != nil {
			return nil, err
		}
		c.NewMembers = append(c.NewMembers, Member{ACI: aci, Role: int32(m.GetRole()), ProfileKey: key, JoinedAt: c.Revision})
		c.JoinFromInviteLink = append(c.JoinFromInviteLink, am.GetJoinFromInviteLink())
	}
	for _, dm := range a.GetDeleteMembers() {
		aci, err := d.aci(dm.GetDeletedUserId())
		if err != nil {
			return nil, err
		}
		c.DeleteMembers = append(c.DeleteMembers, aci)
	}
	for _, mr := range a.GetModifyMemberRoles() {
		aci, err := d.aci(mr.GetUserId())
		if err != nil {
			return nil, err
		}
		c.ModifyRoles = append(c.ModifyRoles, RoleChange{aci, int32(mr.GetRole())})
	}
	const minusOne = ^uint32(0)
	fromPresentationOr := func(pres, uid, pk []byte) (string, []byte, error) {
		if len(uid) == 0 || len(pk) == 0 {
			return d.presentation(pres)
		}
		return d.memberIdentity(uid, pk, nil)
	}
	for _, pk := range a.GetModifyMemberProfileKeys() {
		aci, key, err := fromPresentationOr(pk.GetPresentation(), pk.GetUserId(), pk.GetProfileKey())
		if err != nil {
			return nil, err
		}
		c.ModifiedProfileKeys = append(c.ModifiedProfileKeys, Member{ACI: aci, Role: RoleUnknown, ProfileKey: key, JoinedAt: minusOne})
	}
	for _, ap := range a.GetAddMembersPendingProfileKey() {
		p := ap.GetAdded()
		pm := PendingMember{ServiceID: d.orUnknown(p.GetMember().GetUserId()), Role: normalRole(p.GetMember().GetRole()),
			Timestamp: p.GetTimestamp(), Ciphertext: p.GetMember().GetUserId()}
		if len(p.GetAddedByUserId()) > 0 {
			by, err := d.aci(p.GetAddedByUserId())
			if err != nil {
				return nil, err
			}
			pm.AddedBy = by
		}
		c.NewPending = append(c.NewPending, pm)
	}
	for _, dp := range a.GetDeleteMembersPendingProfileKey() {
		c.DeletePending = append(c.DeletePending, PendingRemoval{ServiceID: d.orUnknown(dp.GetDeletedUserId()), Ciphertext: dp.GetDeletedUserId()})
	}
	for _, pp := range a.GetPromoteMembersPendingProfileKey() {
		aci, key, err := fromPresentationOr(pp.GetPresentation(), pp.GetUserId(), pp.GetProfileKey())
		if err != nil {
			return nil, err
		}
		c.PromotePending = append(c.PromotePending, Member{ACI: aci, Role: RoleDefault, ProfileKey: key, JoinedAt: minusOne})
	}
	if mt := a.GetModifyTitle(); mt != nil {
		t := strings.TrimSpace(d.blob(mt.GetTitle()).GetTitle())
		c.NewTitle = &t
	}
	if ma := a.GetModifyAvatar(); ma != nil {
		v := ma.GetAvatar()
		c.NewAvatar = &v
	}
	if mt := a.GetModifyDisappearingMessageTimer(); mt != nil {
		v := d.blob(mt.GetTimer()).GetDisappearingMessagesDuration()
		c.NewTimer = &v
	}
	c.NewAttributeAccess = int32(a.GetModifyAttributesAccess().GetAttributesAccess())
	c.NewMemberAccess = int32(a.GetModifyMemberAccess().GetMembersAccess())
	c.NewInviteLinkAccess = int32(a.GetModifyAddFromInviteLinkAccess().GetAddFromInviteLinkAccess())
	c.NewMemberLabelAccess = int32(a.GetModifyMemberLabelAccess().GetMemberLabelAccess())
	for _, ar := range a.GetAddMembersPendingAdminApproval() {
		r := ar.GetAdded()
		aci, key, err := d.memberIdentity(r.GetUserId(), r.GetProfileKey(), r.GetPresentation())
		if err != nil {
			return nil, err
		}
		c.NewRequesting = append(c.NewRequesting, RequestingMember{ACI: aci, ProfileKey: key, Timestamp: r.GetTimestamp()})
	}
	for _, dr := range a.GetDeleteMembersPendingAdminApproval() {
		aci, err := d.aci(dr.GetDeletedUserId())
		if err != nil {
			return nil, err
		}
		c.DeleteRequesting = append(c.DeleteRequesting, aci)
	}
	for _, pr := range a.GetPromoteMembersPendingAdminApproval() {
		aci, err := d.aci(pr.GetUserId())
		if err != nil {
			return nil, err
		}
		c.PromoteRequesting = append(c.PromoteRequesting, RoleChange{aci, int32(pr.GetRole())})
	}
	if mp := a.GetModifyInviteLinkPassword(); mp != nil {
		c.NewInvitePassword = mp.GetInviteLinkPassword()
	}
	if md := a.GetModifyDescription(); md != nil {
		v := strings.TrimSpace(d.blob(md.GetDescription()).GetDescriptionText())
		c.NewDescription = &v
	}
	if ma := a.GetModifyAnnouncementsOnly(); ma != nil {
		c.NewAnnouncements = announcements(ma.GetAnnouncementsOnly())
	}
	for _, ab := range a.GetAddMembersBanned() {
		id, err := d.serviceID(ab.GetAdded().GetUserId())
		if err != nil {
			return nil, err
		}
		c.NewBanned = append(c.NewBanned, BannedMember{ServiceID: id, Timestamp: ab.GetAdded().GetTimestamp()})
	}
	for _, db := range a.GetDeleteMembersBanned() {
		id, err := d.serviceID(db.GetDeletedUserId())
		if err != nil {
			return nil, err
		}
		c.DeleteBanned = append(c.DeleteBanned, id)
	}
	for _, pp := range a.GetPromoteMembersPendingPniAciProfileKey() {
		aci, err := d.aci(pp.GetUserId())
		if err != nil {
			return nil, err
		}
		pni, err := d.serviceID(pp.GetPni())
		if err != nil || !strings.HasPrefix(pni, "PNI:") {
			return nil, ErrInvalidGroup
		}
		key, err := d.profileKey(pp.GetProfileKey(), aci)
		if err != nil {
			return nil, err
		}
		c.PromotePniAci = append(c.PromotePniAci, Member{ACI: aci, Role: RoleDefault, ProfileKey: key, JoinedAt: c.Revision, PNI: pni})
		c.Editor = aci
	}
	for _, ml := range a.GetModifyMemberLabels() {
		aci, err := d.aci(ml.GetUserId())
		if err != nil {
			return nil, err
		}
		c.Labels = append(c.Labels, LabelChange{aci, d.rawBlob(ml.GetLabelEmoji()), d.rawBlob(ml.GetLabelString())})
	}
	c.Terminate = a.GetTerminateGroup() != nil
	// PNI editor fix-up (§4.2).
	if strings.HasPrefix(c.Editor, "PNI:") {
		switch {
		case len(c.NewMembers) == 1:
			if c.JoinFromInviteLink[0] {
				c.Editor = c.NewMembers[0].ACI
			} else {
				c.Editor = ""
			}
		case len(c.DeletePending) == 0 && len(c.PromotePniAci) == 0:
			c.Editor = ""
		}
	}
	return c, nil
}

// ErrNotApplicable: the change does not fit the state (§4.3).
var ErrNotApplicable = errors.New("groups: change cannot be applied")

// Apply returns the state after the change (§4.3); s is not modified.
func Apply(s *State, c *Change) (*State, error) {
	if c.Revision != s.Revision+1 {
		return nil, ErrNotApplicable
	}
	n := clone(s)
	n.Revision = c.Revision
	for _, m := range c.NewMembers {
		if e := n.Member(m.ACI); e != nil {
			*e = m
		} else {
			n.Members = append(n.Members, m)
		}
		n.dropNonMembers(m.ACI)
	}
	for _, aci := range c.DeleteMembers {
		for i := range n.Members {
			if n.Members[i].ACI == aci {
				n.Members = append(n.Members[:i], n.Members[i+1:]...)
				break
			}
		}
	}
	for _, r := range c.ModifyRoles {
		m := n.Member(r.ACI)
		if m == nil || (r.Role != RoleDefault && r.Role != RoleAdmin) {
			return nil, ErrNotApplicable
		}
		m.Role = r.Role
	}
	for _, pk := range c.ModifiedProfileKeys {
		m := n.Member(pk.ACI)
		if m == nil {
			return nil, ErrNotApplicable
		}
		m.ProfileKey = pk.ProfileKey
	}
	for _, p := range c.NewPending {
		if n.Member(p.ServiceID) != nil {
			return nil, ErrNotApplicable
		}
		dup := false
		for _, e := range n.Pending {
			if string(e.Ciphertext) == string(p.Ciphertext) {
				dup = true
			}
		}
		if !dup {
			n.Pending = append(n.Pending, p)
		}
	}
	for _, r := range c.DeletePending {
		for i := range n.Pending {
			if string(n.Pending[i].Ciphertext) == string(r.Ciphertext) {
				n.Pending = append(n.Pending[:i], n.Pending[i+1:]...)
				break
			}
		}
	}
	for _, m := range c.PromotePending {
		i := n.PendingIndex(m.ACI)
		if i < 0 {
			return nil, ErrNotApplicable
		}
		n.Pending = append(n.Pending[:i], n.Pending[i+1:]...)
		n.Members = append(n.Members, m)
	}
	if c.NewTitle != nil {
		n.Title = *c.NewTitle
	}
	if c.NewDescription != nil {
		n.Description = *c.NewDescription
	}
	if c.NewAvatar != nil {
		n.Avatar = *c.NewAvatar
	}
	if c.NewTimer != nil {
		n.Timer = *c.NewTimer
	}
	if c.NewAnnouncements != EnabledUnknown {
		n.Announcements = c.NewAnnouncements
	}
	setAccess := func(v int32, f func(*Access)) {
		if v == AccessUnknown {
			return
		}
		if n.Access == nil {
			n.Access = &Access{}
		}
		f(n.Access)
	}
	setAccess(c.NewAttributeAccess, func(a *Access) { a.Attributes = c.NewAttributeAccess })
	setAccess(c.NewMemberAccess, func(a *Access) { a.Members = c.NewMemberAccess })
	setAccess(c.NewInviteLinkAccess, func(a *Access) { a.AddFromInviteLink = c.NewInviteLinkAccess })
	setAccess(c.NewMemberLabelAccess, func(a *Access) { a.MemberLabel = c.NewMemberLabelAccess })
	n.Requesting = append(n.Requesting, c.NewRequesting...)
	for _, aci := range c.DeleteRequesting {
		if i := n.RequestingIndex(aci); i >= 0 {
			n.Requesting = append(n.Requesting[:i], n.Requesting[i+1:]...)
		}
	}
	for _, p := range c.PromoteRequesting {
		if p.Role != RoleDefault && p.Role != RoleAdmin {
			return nil, ErrNotApplicable
		}
		i := n.RequestingIndex(p.ACI)
		if i < 0 {
			continue
		}
		r := n.Requesting[i]
		n.Requesting = append(n.Requesting[:i], n.Requesting[i+1:]...)
		n.Members = append(n.Members, Member{ACI: r.ACI, Role: p.Role, ProfileKey: r.ProfileKey})
	}
	if len(c.NewInvitePassword) > 0 {
		n.InvitePassword = c.NewInvitePassword
	}
	for _, b := range c.NewBanned {
		if !n.IsBanned(b.ServiceID) {
			n.Banned = append(n.Banned, b)
		}
	}
	for _, id := range c.DeleteBanned {
		for i := range n.Banned {
			if n.Banned[i].ServiceID == id {
				n.Banned = append(n.Banned[:i], n.Banned[i+1:]...)
				break
			}
		}
	}
	for _, m := range c.PromotePniAci {
		i := n.PendingIndex(m.PNI)
		if i < 0 {
			return nil, ErrNotApplicable
		}
		n.Pending = append(n.Pending[:i], n.Pending[i+1:]...)
		n.Members = append(n.Members, m)
	}
	for _, l := range c.Labels {
		m := n.Member(l.ACI)
		if m == nil {
			return nil, ErrNotApplicable
		}
		m.LabelEmoji, m.LabelString = l.LabelEmoji, l.LabelString
	}
	if c.Terminate {
		n.Terminated = true
	}
	return n, nil
}

// dropNonMembers removes invitee and requester entries of a new member.
func (s *State) dropNonMembers(aci string) {
	if i := s.PendingIndex(aci); i >= 0 {
		s.Pending = append(s.Pending[:i], s.Pending[i+1:]...)
	}
	if i := s.RequestingIndex(aci); i >= 0 {
		s.Requesting = append(s.Requesting[:i], s.Requesting[i+1:]...)
	}
}

func clone(s *State) *State {
	n := *s
	n.Members = append([]Member(nil), s.Members...)
	n.Pending = append([]PendingMember(nil), s.Pending...)
	n.Requesting = append([]RequestingMember(nil), s.Requesting...)
	n.Banned = append([]BannedMember(nil), s.Banned...)
	if s.Access != nil {
		a := *s.Access
		n.Access = &a
	}
	return &n
}

// ProfileKeysFromChange returns the editor's authoritative profile key
// carried by a change, if any (§4.4).
func ProfileKeysFromChange(c *Change) (aci string, key []byte, ok bool) {
	if c.Editor == "" {
		return "", nil, false
	}
	for _, list := range [][]Member{c.NewMembers, c.PromotePending, c.ModifiedProfileKeys, c.PromotePniAci} {
		for _, m := range list {
			if m.ACI == c.Editor && len(m.ProfileKey) == 32 {
				return m.ACI, m.ProfileKey, true
			}
		}
	}
	for _, r := range c.NewRequesting {
		if r.ACI == c.Editor && len(r.ProfileKey) == 32 {
			return r.ACI, r.ProfileKey, true
		}
	}
	return "", nil, false
}
