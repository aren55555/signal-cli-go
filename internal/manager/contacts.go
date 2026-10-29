// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"errors"
	"io"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"google.golang.org/protobuf/proto"
)

// Contact operations (spec/service/contacts.md §7, §9, §10) and the
// profile-key rotation they can trigger (profiles.md §7).

// sendMessageRequestResponse sets profile sharing locally and syncs a
// message request response of type t to the own devices (sync.md §6.9),
// for a person (aci) or a group (groupID).
func (s *Sender) sendMessageRequestResponse(ctx context.Context, aci string, groupID []byte, t signalpb.AppMessageRequestResponse_Type, sharing bool) error {
	a := s.Acct
	if groupID != nil {
		if rec := a.Group(groupID); rec != nil {
			rec.ProfileSharing = sharing
			a.SaveGroups()
		}
	} else {
		rc := a.Recipients[aci]
		rc.ProfileSharing = sharing
		a.Recipients[aci] = rc
		if err := a.SaveRecipients(); err != nil {
			return err
		}
	}
	if !a.IsMultiDevice() {
		return nil
	}
	m := &signalpb.AppMessageRequestResponse{Type: t.Enum()}
	if groupID != nil {
		m.GroupId = groupID
	} else {
		bin, ok := uuidRaw(aci)
		if !ok {
			return nil // no service ID: skipped (contacts.md §7.1)
		}
		m.ThreadAciBinary = bin[:]
	}
	return s.sendSync(ctx, &signalpb.AppSyncMessage{MessageRequestResponse: m}, 0, false)
}

// SendMessageRequestResponse is the person form for
// sendMessageRequestResponse: ACCEPT or DELETE.
func (s *Sender) SendMessageRequestResponse(ctx context.Context, aci string, accept bool) error {
	t := signalpb.AppMessageRequestResponse_DELETE
	if accept {
		t = signalpb.AppMessageRequestResponse_ACCEPT
	}
	return s.sendMessageRequestResponse(ctx, aci, nil, t, accept)
}

// unblockAndAccept goes on the wire as 0 (contacts.md §7.1).
const unblockAndAccept = signalpb.AppMessageRequestResponse_Type(0)

// sharesGroup reports whether aci and the account are both members of a
// stored group.
func (s *Sender) sharesGroup(aci string) bool {
	for i := range s.Acct.Groups {
		st := StateOf(&s.Acct.Groups[i])
		if st != nil && st.Member(aci) != nil && st.Member(s.Acct.Data.ACI) != nil {
			return true
		}
	}
	return false
}

// SetBlocked blocks or unblocks recipients (contacts.md §7.1): per
// changed recipient the contact flag and a message request response,
// then a rotation if a newly blocked contact shares no group, then the
// blocked-list sync (always).
func (s *Sender) SetBlocked(ctx context.Context, keys []string, blocked bool, gm *Groups) error {
	a := s.Acct
	rotate := false
	for _, k := range keys {
		rc := a.Recipients[k]
		if rc.Blocked == blocked {
			continue
		}
		if !rc.IsContact() {
			rc.Unregistered, rc.UnregisteredAt = false, 0 // the copied record is empty (§2.3)
		}
		rc.Blocked = blocked
		if blocked {
			rc.ProfileSharing = false
		}
		a.Recipients[k] = rc
		a.SaveRecipients()
		aci := a.AddressOf(k).ACI
		t, sharing := signalpb.AppMessageRequestResponse_BLOCK, false
		if !blocked {
			t, sharing = unblockAndAccept, true
		}
		if aci != "" {
			if err := s.sendMessageRequestResponse(ctx, aci, nil, t, sharing); err != nil {
				return err
			}
		}
		if blocked && !s.sharesGroup(aci) {
			rotate = true
		}
	}
	if rotate {
		if err := s.RotateProfileKey(ctx, gm); err != nil {
			return err
		}
	}
	return s.SendBlocked(ctx)
}

// SetGroupsBlocked is §7.2: any newly blocked group triggers a rotation.
func (s *Sender) SetGroupsBlocked(ctx context.Context, ids [][]byte, blocked bool, gm *Groups) error {
	a := s.Acct
	rotate := false
	for _, id := range ids {
		rec := a.Group(id)
		if rec == nil || rec.Blocked == blocked {
			continue
		}
		rec.Blocked = blocked
		a.SaveGroups()
		t, sharing := signalpb.AppMessageRequestResponse_BLOCK, false
		if !blocked {
			t, sharing = unblockAndAccept, true
		}
		if err := s.sendMessageRequestResponse(ctx, "", id, t, sharing); err != nil {
			return err
		}
		rotate = rotate || blocked
	}
	if rotate {
		if err := s.RotateProfileKey(ctx, gm); err != nil {
			return err
		}
	}
	return s.SendBlocked(ctx)
}

// RotateProfileKey is profiles.md §7: a new key, account attributes, a
// profile write in rotation mode, profile-key updates to every
// non-blocked recipient with profile sharing, and the group updates.
func (s *Sender) RotateProfileKey(ctx context.Context, gm *Groups) error {
	a := s.Acct
	s.Profile(ctx, a.Data.ACI)
	key := make([]byte, 32)
	if _, err := io.ReadFull(s.rand(), key); err != nil {
		return err
	}
	a.Data.ProfileKey = key
	if err := a.Save(); err != nil {
		return err
	}
	rc := a.Recipients[a.Data.ACI]
	rc.ProfileKeyCredential = nil
	a.Recipients[a.Data.ACI] = rc
	b, err := api.Marshal(accountAttributes(a))
	if err != nil {
		return err
	}
	r, err := s.Chat.Request(ctx, "PUT", "/v1/accounts/attributes", jsonHeader, b)
	if err != nil {
		return err
	}
	if r.Status < 200 || r.Status > 299 {
		return &SendStatusError{Status: r.Status}
	}
	if err := s.WriteProfile(ctx, ProfileUpdate{Avatar: AvatarRotate}); err != nil {
		return err
	}
	for _, k := range a.SortedRecipientKeys() {
		rc := a.Recipients[k]
		aci := a.AddressOf(k).ACI
		if aci == "" || aci == a.Data.ACI || !rc.ProfileSharing || rc.Blocked {
			continue
		}
		s.SendProfileKeyUpdate(ctx, aci)
	}
	if gm != nil {
		gm.UpdateOwnProfileKey(ctx)
	}
	return nil
}

// SendProfileKeyUpdate sends the dedicated profile-key update
// (content.md §4.4): flags PROFILE_KEY_UPDATE, implicit, not urgent.
func (s *Sender) SendProfileKeyUpdate(ctx context.Context, aci string) error {
	ts := s.timestamp()
	dm := &signalpb.AppDataMessage{Flags: proto.Uint32(4), ProfileKey: s.Acct.Data.ProfileKey, Timestamp: proto.Uint64(ts),
		ExpireTimerVersion: proto.Uint32(1), RequiredProtocolVersion: proto.Uint32(0)}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: dm}})
	if err != nil {
		return err
	}
	_, _, err = s.send(ctx, aci, c, ts, false, protocol.ContentHintImplicit)
	return err
}

// ContactUpdate is updateContact's input (contacts.md §9); nil fields are
// not supplied.
type ContactUpdate struct {
	GivenName, FamilyName, NickGivenName, NickFamilyName, Note *string
	Expiration                                                 *uint32
}

// UpdateContact is contacts.md §9.
func (s *Sender) UpdateContact(ctx context.Context, key string, u ContactUpdate) error {
	a := s.Acct
	if u.Expiration != nil {
		rc := a.Recipients[key]
		if !(rc.IsContact() && rc.Timer == *u.Expiration) {
			v := uint32(1)
			if rc.IsContact() {
				v = rc.TimerVersion + 1
				if rc.TimerVersion == 0 || rc.TimerVersion >= 2147483647 {
					v = max(rc.TimerVersion, 1)
				}
			}
			if !rc.IsContact() {
				rc.Unregistered, rc.UnregisteredAt = false, 0
			}
			rc.Timer, rc.TimerVersion = *u.Expiration, v
			a.Recipients[key] = rc
			a.SaveRecipients()
		}
		if aci := a.AddressOf(key).ACI; aci != "" {
			s.SendData(ctx, []string{aci}, DataSend{Message: &signalpb.AppDataMessage{Flags: proto.Uint32(2)}, Urgent: true})
		}
	}
	rc := a.Recipients[key]
	if !rc.IsContact() {
		rc.Unregistered, rc.UnregisteredAt = false, 0
	}
	rc.Hidden = false
	for _, f := range []struct{ dst, src **string }{{&rc.GivenName, &u.GivenName}, {&rc.FamilyName, &u.FamilyName},
		{&rc.NickGivenName, &u.NickGivenName}, {&rc.NickFamilyName, &u.NickFamilyName}, {&rc.Note, &u.Note}} {
		if *f.src != nil {
			v := **f.src
			*f.dst = &v
		}
	}
	a.Recipients[key] = rc
	return a.SaveRecipients()
}

// ErrDeleteSelf is removeContact --forget on the own recipient.
var ErrDeleteSelf = errors.New("Can't delete self recipient")

// RemoveContact is contacts.md §10.
func (s *Sender) RemoveContact(key string, hide, forget bool) error {
	a := s.Acct
	rc := a.Recipients[key]
	switch {
	case forget:
		if key == a.Data.ACI {
			return ErrDeleteSelf
		}
		ad := a.AddressOf(key)
		delete(a.Recipients, key)
		for _, id := range []string{ad.ACI, "PNI:" + ad.PNI} {
			if id == "" || id == "PNI:" {
				continue
			}
			delete(a.Identities, id)
			a.DeleteSessions(id)
			a.DeleteSenderKeys(id)
		}
		a.SaveIdentities()
		a.SaveSessions()
	case hide:
		if !rc.IsContact() {
			rc.Unregistered, rc.UnregisteredAt = false, 0
		}
		rc.Hidden = true
		a.Recipients[key] = rc
	default:
		rc.GivenName, rc.FamilyName, rc.NickName, rc.NickGivenName, rc.NickFamilyName, rc.Note, rc.Color = nil, nil, nil, nil, nil, nil, nil
		rc.Timer, rc.TimerVersion, rc.MuteUntil = 0, 1, 0
		rc.HideStory, rc.ProfileSharing, rc.Blocked, rc.Archived, rc.Hidden = false, false, false, false, false
		rc.Unregistered, rc.UnregisteredAt = false, 0
		a.Recipients[key] = rc
	}
	return a.SaveRecipients()
}
