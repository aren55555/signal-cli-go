// SPDX-License-Identifier: GPL-3.0-or-later

package groups

import (
	"fmt"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// FromDecrypted converts signal-cli's stored DecryptedGroup (the
// group_v2.group_data column, spec/service/groups.md §2.6) into a State.
// Only the importer reads this form.
func FromDecrypted(pb *signalpb.DecryptedGroup) (*State, error) {
	s := &State{
		Title:          pb.GetTitle(),
		Description:    pb.GetDescription(),
		Avatar:         pb.GetAvatar(),
		Timer:          pb.GetDisappearingMessagesTimer().GetDuration(),
		Revision:       pb.GetRevision(),
		InvitePassword: pb.GetInviteLinkPassword(),
		Announcements:  int(pb.GetIsAnnouncementGroup()),
		Terminated:     pb.GetTerminated(),
	}
	if ac := pb.GetAccessControl(); ac != nil {
		s.Access = &Access{int32(ac.GetAttributes()), int32(ac.GetMembers()), int32(ac.GetAddFromInviteLink()), int32(ac.GetMemberLabel())}
	}
	for _, m := range pb.GetMembers() {
		aci, err := binaryACI(m.GetAciBytes())
		if err != nil {
			return nil, err
		}
		mem := Member{ACI: aci, Role: int32(m.GetRole()), ProfileKey: m.GetProfileKey(), JoinedAt: m.GetJoinedAtRevision(),
			LabelEmoji: m.GetLabelEmoji(), LabelString: m.GetLabelString()}
		if b := m.GetPniBytes(); len(b) > 0 {
			id, err := binaryServiceID(b)
			if err != nil {
				return nil, err
			}
			id.PNI = true
			mem.PNI = ServiceIDString(id)
		}
		s.Members = append(s.Members, mem)
	}
	for _, p := range pb.GetPendingMembers() {
		id := UnknownACI
		if sid, err := binaryServiceID(p.GetServiceIdBytes()); err == nil {
			id = ServiceIDString(sid)
		}
		by, err := binaryACI(p.GetAddedByAci())
		if err != nil {
			return nil, err
		}
		s.Pending = append(s.Pending, PendingMember{ServiceID: id, Role: normalRole(p.GetRole()), AddedBy: by,
			Timestamp: p.GetTimestamp(), Ciphertext: p.GetServiceIdCipherText()})
	}
	for _, r := range pb.GetRequestingMembers() {
		aci, err := binaryACI(r.GetAciBytes())
		if err != nil {
			return nil, err
		}
		s.Requesting = append(s.Requesting, RequestingMember{ACI: aci, ProfileKey: r.GetProfileKey(), Timestamp: r.GetTimestamp()})
	}
	for _, b := range pb.GetBannedMembers() {
		id, err := binaryServiceID(b.GetServiceIdBytes())
		if err != nil {
			return nil, err
		}
		s.Banned = append(s.Banned, BannedMember{ServiceID: ServiceIDString(id), Timestamp: b.GetTimestamp()})
	}
	return s, nil
}

// binaryServiceID parses the service-ID binary form: 16 bytes for an
// ACI, 0x01 and 16 bytes for a PNI.
func binaryServiceID(b []byte) (zkgroup.ServiceID, error) {
	var id zkgroup.ServiceID
	switch {
	case len(b) == 16:
	case len(b) == 17 && b[0] == 0x01:
		id.PNI, b = true, b[1:]
	default:
		return id, fmt.Errorf("groups: bad service ID binary %x", b)
	}
	copy(id.UUID[:], b)
	return id, nil
}

func binaryACI(b []byte) (string, error) {
	id, err := binaryServiceID(b)
	if err != nil || id.PNI {
		return "", fmt.Errorf("groups: bad ACI binary %x", b)
	}
	return ServiceIDString(id), nil
}
