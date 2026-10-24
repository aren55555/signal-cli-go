package groups

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

// TestProtoEncodings: our groups protos decode and re-encode every vector
// byte for byte (spec/service/groups.md §2).
func TestProtoEncodings(t *testing.T) {
	var v struct {
		Encodings map[string]any `json:"proto_encodings"`
	}
	testvec.Load(t, "groups.json", &v)
	types := map[string]func() proto.Message{
		"AccessControl":          func() proto.Message { return &signalpb.GroupAccessControl{} },
		"Actions":                func() proto.Message { return &signalpb.GroupChange_Actions{} },
		"AvatarUploadAttributes": func() proto.Message { return &signalpb.AvatarUploadAttributes{} },
		"GroupAttributeBlob":     func() proto.Message { return &signalpb.GroupAttributeBlob{} },
		"GroupChange":            func() proto.Message { return &signalpb.GroupChange{} },
		"GroupChangeResponse":    func() proto.Message { return &signalpb.GroupChangeResponse{} },
		"GroupChanges":           func() proto.Message { return &signalpb.GroupChanges{} },
		"GroupContextV2":         func() proto.Message { return &signalpb.AppGroupContextV2{} },
		"GroupJoinInfo":          func() proto.Message { return &signalpb.GroupJoinInfo{} },
		"GroupResponse":          func() proto.Message { return &signalpb.GroupResponse{} },
		"Member":                 func() proto.Message { return &signalpb.GroupMember{} },
	}
	n := 0
	for name, val := range v.Encodings {
		s, ok := val.(string)
		if !ok || name == "note" {
			continue
		}
		mk := types[strings.SplitN(name, " ", 2)[0]]
		if mk == nil {
			continue // DecryptedGroup, ExternalGroupCredential: not used
		}
		b := testvec.Hex(t, s)
		m := mk()
		if err := proto.Unmarshal(b, m); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(m.ProtoReflect().GetUnknown()) > 0 {
			t.Errorf("%s: unknown fields %x", name, m.ProtoReflect().GetUnknown())
		}
		out, _ := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		if hex.EncodeToString(out) != s {
			t.Errorf("%s: re-encoded %x", name, out)
		}
		n++
	}
	if n < 20 {
		t.Errorf("only %d encodings checked", n)
	}
}
