// SPDX-License-Identifier: GPL-3.0-or-later

package groups

import (
	"bytes"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

const (
	aliceACI = "00010203-0405-0607-0809-0a0b0c0d0e0f" // the vectors' credential owner
	bobACI   = "b0b0b0b0-0000-4000-8000-000000000002"
	carolPNI = "PNI:c0c0c0c0-0000-4000-8000-000000000003"
)

type world struct {
	t          *testing.T
	g          *zkgroup.GroupSecretParams
	alicePres  []byte
	aliceKey   []byte
	randomness [32]byte
}

func newWorld(t *testing.T) *world {
	var v struct {
		Groups map[string]struct {
			MasterKey string `json:"master_key"`
		}
		Profile []struct {
			Group        string
			ProfileKey   string `json:"profile_key"`
			Presentation string
		} `json:"expiring_profile_key_credential"`
	}
	testvec.Load(t, "zkgroup_credentials.json", &v)
	var mk [32]byte
	copy(mk[:], testvec.Hex(t, v.Groups[v.Profile[0].Group].MasterKey))
	return &world{t: t, g: zkgroup.DeriveGroupSecretParams(mk), alicePres: testvec.Hex(t, v.Profile[0].Presentation),
		aliceKey: testvec.Hex(t, v.Profile[0].ProfileKey)}
}

func (w *world) sid(s string) []byte {
	id, err := ParseServiceID(s)
	if err != nil {
		w.t.Fatal(err)
	}
	return w.g.EncryptServiceID(id)
}

func (w *world) pk(aci string, key []byte) []byte {
	id, _ := ParseServiceID(aci)
	var k [32]byte
	copy(k[:], key)
	return w.g.EncryptProfileKey(k, id.UUID)
}

func (w *world) blob(b *signalpb.GroupAttributeBlob) []byte {
	raw, _ := proto.Marshal(b)
	w.randomness[0]++
	return w.g.EncryptBlob(w.randomness, raw, 0)
}

func TestDecryptGroup(t *testing.T) {
	w := newWorld(t)
	bobKey := bytes.Repeat([]byte{7}, 32)
	pb := &signalpb.Group{
		Title:                     w.blob(&signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_Title{Title: "  Team  "}}),
		Description:               w.blob(&signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_DescriptionText{DescriptionText: "d"}}),
		DisappearingMessagesTimer: w.blob(&signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_DisappearingMessagesDuration{DisappearingMessagesDuration: 3600}}),
		AccessControl:             &signalpb.GroupAccessControl{Attributes: signalpb.GroupAccessControl_MEMBER, Members: signalpb.GroupAccessControl_ADMINISTRATOR},
		Version:                   5,
		Members: []*signalpb.GroupMember{
			{Presentation: w.alicePres, Role: signalpb.GroupMember_ADMINISTRATOR, JoinedAtVersion: 0},
			{UserId: w.sid(bobACI), ProfileKey: w.pk(bobACI, bobKey), Role: signalpb.GroupMember_DEFAULT, JoinedAtVersion: 2},
		},
		MembersPendingProfileKey: []*signalpb.GroupMemberPendingProfileKey{
			{Member: &signalpb.GroupMember{UserId: w.sid(carolPNI), Role: signalpb.GroupMember_UNKNOWN}, AddedByUserId: w.sid(aliceACI), Timestamp: 9},
			{Member: &signalpb.GroupMember{UserId: []byte{1, 2, 3}}, AddedByUserId: w.sid(aliceACI)},
		},
		AnnouncementsOnly: true,
	}
	s, err := DecryptGroup(w.g, pb)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Team" || s.Description != "d" || s.Timer != 3600 || s.Revision != 5 || s.Announcements != Enabled ||
		s.Access == nil || s.Access.Members != AccessAdmin {
		t.Errorf("state %+v", s)
	}
	if len(s.Members) != 2 || s.Members[0].ACI != aliceACI || !bytes.Equal(s.Members[0].ProfileKey, w.aliceKey) ||
		s.Members[1].ACI != bobACI || !bytes.Equal(s.Members[1].ProfileKey, bobKey) || s.Members[1].JoinedAt != 2 {
		t.Errorf("members %+v", s.Members)
	}
	if len(s.Pending) != 2 || s.Pending[0].ServiceID != carolPNI || s.Pending[0].Role != RoleDefault || s.Pending[0].AddedBy != aliceACI ||
		s.Pending[1].ServiceID != UnknownACI {
		t.Errorf("pending %+v", s.Pending)
	}
	// A PNI as a full member makes the group invalid.
	pb.Members = append(pb.Members, &signalpb.GroupMember{UserId: w.sid(carolPNI), ProfileKey: w.pk(bobACI, bobKey)})
	if _, err := DecryptGroup(w.g, pb); !errors.Is(err, ErrInvalidGroup) {
		t.Errorf("PNI member: %v", err)
	}
	// Short and corrupt blobs decrypt to empty values.
	pb.Members = pb.Members[:2]
	pb.Title = []byte{1, 2, 3}
	pb.Description = append(w.blob(&signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_DescriptionText{DescriptionText: "x"}}), 0)
	pb.Description[0] ^= 1
	if s, err := DecryptGroup(w.g, pb); err != nil || s.Title != "" || s.Description != "" {
		t.Errorf("blobs: %v %+v", err, s)
	}
}

func TestDecryptAndApplyChange(t *testing.T) {
	w := newWorld(t)
	bobKey := bytes.Repeat([]byte{7}, 32)
	state := &State{Revision: 1, Members: []Member{{ACI: bobACI, Role: RoleAdmin, ProfileKey: bobKey}},
		Pending: []PendingMember{{ServiceID: carolPNI, Role: RoleDefault, Ciphertext: w.sid(carolPNI)}}}
	a := &signalpb.GroupChange_Actions{
		SourceUserId:                   w.sid(bobACI),
		Version:                        2,
		AddMembers:                     []*signalpb.GroupChange_Actions_AddMember{{Added: &signalpb.GroupMember{Presentation: w.alicePres, Role: signalpb.GroupMember_DEFAULT}}},
		ModifyTitle:                    &signalpb.GroupChange_Actions_ModifyTitle{Title: w.blob(&signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_Title{Title: "New"}})},
		DeleteMembersPendingProfileKey: []*signalpb.GroupChange_Actions_DeleteMemberPendingProfileKey{{DeletedUserId: w.sid(carolPNI)}},
		ModifyAddFromInviteLinkAccess:  &signalpb.GroupChange_Actions_ModifyAddFromInviteLinkAccess{AddFromInviteLinkAccess: signalpb.GroupAccessControl_ANY},
		ModifyInviteLinkPassword:       &signalpb.GroupChange_Actions_ModifyInviteLinkPassword{InviteLinkPassword: bytes.Repeat([]byte{1}, 16)},
		AddMembersBanned:               []*signalpb.GroupChange_Actions_AddMemberBanned{{Added: &signalpb.GroupMemberBanned{UserId: w.sid(carolPNI), Timestamp: 5}}},
	}
	c, err := DecryptActions(w.g, a, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Editor != bobACI || c.Revision != 2 || len(c.NewMembers) != 1 || c.NewMembers[0].ACI != aliceACI || c.NewMembers[0].JoinedAt != 2 ||
		*c.NewTitle != "New" || len(c.DeletePending) != 1 || c.DeletePending[0].ServiceID != carolPNI {
		t.Fatalf("change %+v", c)
	}
	n, err := Apply(state, c)
	if err != nil {
		t.Fatal(err)
	}
	if n.Revision != 2 || n.Title != "New" || len(n.Members) != 2 || len(n.Pending) != 0 || n.Access.AddFromInviteLink != AccessAny ||
		len(n.InvitePassword) != 16 || !n.IsBanned(carolPNI) {
		t.Errorf("applied %+v", n)
	}
	if len(state.Members) != 1 || len(state.Pending) != 1 {
		t.Error("Apply modified its input")
	}
	if aci, key, ok := ProfileKeysFromChange(c); ok {
		t.Errorf("bob added alice, so no key of the editor: %s %x", aci, key)
	}
	c.Editor = aliceACI
	if aci, key, ok := ProfileKeysFromChange(c); !ok || aci != aliceACI || !bytes.Equal(key, w.aliceKey) {
		t.Error("editor's key not found")
	}
	// Wrong revision, unknown member role change.
	if _, err := Apply(n, c); !errors.Is(err, ErrNotApplicable) {
		t.Errorf("same revision: %v", err)
	}
	bad := &Change{Revision: 3, ModifyRoles: []RoleChange{{ACI: "d0d0d0d0-0000-4000-8000-000000000004", Role: RoleAdmin}}}
	if _, err := Apply(n, bad); !errors.Is(err, ErrNotApplicable) {
		t.Errorf("role for a non-member: %v", err)
	}
	// Remove alice, promote via roles.
	n2, err := Apply(n, &Change{Revision: 3, DeleteMembers: []string{aliceACI}, ModifyRoles: []RoleChange{{bobACI, RoleDefault}}})
	if err != nil || len(n2.Members) != 1 || n2.Members[0].Role != RoleDefault {
		t.Errorf("delete/role: %v %+v", err, n2)
	}
}
