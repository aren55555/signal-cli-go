// SPDX-License-Identifier: GPL-3.0-or-later

package groups

import (
	"bytes"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

// TestFromDecrypted reads the group_data vector of groups.json.
func TestFromDecrypted(t *testing.T) {
	var v struct {
		Encodings map[string]any `json:"proto_encodings"`
	}
	testvec.Load(t, "groups.json", &v)
	var raw string
	for name, val := range v.Encodings {
		if s, ok := val.(string); ok && len(name) > 15 && name[:15] == "DecryptedGroup " {
			raw = s
		}
	}
	var pb signalpb.DecryptedGroup
	if err := proto.Unmarshal(testvec.Hex(t, raw), &pb); err != nil {
		t.Fatal(err)
	}
	s, err := FromDecrypted(&pb)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(0x40 + i)
	}
	if s.Title != "T" || s.Revision != 3 || s.Timer != 0 || s.Announcements != Disabled || len(s.Members) != 1 ||
		s.Access == nil || s.Access.Attributes != AccessMember || s.Access.Members != AccessMember || s.Access.AddFromInviteLink != AccessUnsatisfiable {
		t.Fatalf("state %+v", s)
	}
	m := s.Members[0]
	if m.ACI != "10111213-1415-1617-1819-1a1b1c1d1e1f" || m.Role != RoleAdmin || !bytes.Equal(m.ProfileKey, key) || m.JoinedAt != 0 {
		t.Errorf("member %+v", m)
	}
}

func TestFromDecryptedServiceIDs(t *testing.T) {
	aci := bytes.Repeat([]byte{0x11}, 16)
	pni := append([]byte{1}, bytes.Repeat([]byte{0x22}, 16)...)
	pb := &signalpb.DecryptedGroup{
		Members:        []*signalpb.DecryptedMember{{AciBytes: aci, Role: signalpb.GroupMember_DEFAULT, PniBytes: bytes.Repeat([]byte{0x22}, 16)}},
		PendingMembers: []*signalpb.DecryptedPendingMember{{ServiceIdBytes: pni, AddedByAci: aci, ServiceIdCipherText: []byte{9}}, {AddedByAci: aci}},
		BannedMembers:  []*signalpb.DecryptedBannedMember{{ServiceIdBytes: pni, Timestamp: 5}},
	}
	s, err := FromDecrypted(pb)
	if err != nil {
		t.Fatal(err)
	}
	const a, p = "11111111-1111-1111-1111-111111111111", "PNI:22222222-2222-2222-2222-222222222222"
	if s.Members[0].PNI != p || s.Pending[0].ServiceID != p || s.Pending[0].AddedBy != a || s.Pending[1].ServiceID != UnknownACI ||
		s.Pending[0].Role != RoleDefault || s.Banned[0].ServiceID != p {
		t.Errorf("state %+v", s)
	}
	if _, err := FromDecrypted(&signalpb.DecryptedGroup{Members: []*signalpb.DecryptedMember{{AciBytes: pni}}}); err == nil {
		t.Error("a PNI as member ACI was accepted")
	}
}
