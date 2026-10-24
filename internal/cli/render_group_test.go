package cli

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

func TestRenderGroupInfo(t *testing.T) {
	mk := bytes.Repeat([]byte{9}, 32)
	id := zkgroup.DeriveGroupSecretParams([32]byte(mk)).GroupID
	gid := base64.StdEncoding.EncodeToString(id[:])
	title := "Team"
	c := &RenderContext{Self: selfNumber, SelfACI: "self-aci", Group: func(g []byte) (string, bool, bool) {
		if !bytes.Equal(g, id[:]) {
			return "", false, false
		}
		if title == "" {
			return "", true, true
		}
		return title, false, true
	}}
	ev := func(ct *signalpb.AppContent) manager.Event {
		return manager.Event{Envelope: &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1700000000000), ServerTimestamp: proto.Uint64(1700000000001)},
			Message: &manager.Message{Sender: protocol.Address{ServiceID: peerACI, DeviceID: 1}, Content: content(ct)}}
	}
	gc := &signalpb.AppGroupContextV2{MasterKey: mk, Revision: proto.Uint32(3)}
	data := ev(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{
		Timestamp: proto.Uint64(1700000000000), Body: proto.String("hi"), GroupV2: gc}}})
	want := "Body: hi\nGroup info:\n  Id: " + gid + "\n  Name: Team\n  Revision: 3\n  Type: DELIVER\n"
	if got := RenderPlain(c, data); !strings.Contains(got, want) {
		t.Errorf("plain:\n%s\nwant:\n%s", got, want)
	}
	if got := RenderJSON(c, data); !strings.Contains(got, `"groupInfo":{"groupId":"`+gid+`","groupName":"Team","revision":3,"type":"DELIVER"}`) {
		t.Errorf("JSON: %s", got)
	}
	// A context with a change is an UPDATE; no state renders a null name.
	gc.GroupChange = []byte{1}
	data = ev(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{
		Timestamp: proto.Uint64(1700000000000), Body: proto.String("hi"), GroupV2: gc}}})
	title = ""
	if got := RenderPlain(c, data); !strings.Contains(got, "  Name: null\n  Revision: 3\n  Type: UPDATE\n") {
		t.Errorf("update plain:\n%s", got)
	}
	if got := RenderJSON(c, data); !strings.Contains(got, `"groupName":null,"revision":3,"type":"UPDATE"`) {
		t.Errorf("update JSON: %s", got)
	}

	typing := ev(&signalpb.AppContent{Body: &signalpb.AppContent_TypingMessage{TypingMessage: &signalpb.AppTyping{
		Timestamp: proto.Uint64(1700000000000), Action: signalpb.AppTyping_STARTED.Enum(), GroupId: []byte{1, 2}}}})
	if got := RenderPlain(c, typing); !strings.Contains(got, "  Group Info:\n    Id: AQI=\n    Name: <Unknown group>\n") {
		t.Errorf("typing plain:\n%s", got)
	}

	title = "Team"
	story := ev(&signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: &signalpb.AppStoryMessage{
		AllowsReplies: proto.Bool(true), Group: &signalpb.AppGroupContextV2{MasterKey: mk},
		Attachment: &signalpb.AppStoryMessage_TextAttachment{TextAttachment: &signalpb.AppTextAttachment{Text: proto.String("s")}}}}})
	if got := RenderPlain(c, story); !strings.Contains(got, "  Story: with replies: true\n  Group info:\n    Id: "+gid+"\n    Name: Team\n  Body: s\n") {
		t.Errorf("story plain:\n%s", got)
	}
	if got := RenderJSON(c, story); !strings.Contains(got, `"storyMessage":{"allowsReplies":true,"groupId":"`+gid+`",`) {
		t.Errorf("story JSON: %s", got)
	}
}
