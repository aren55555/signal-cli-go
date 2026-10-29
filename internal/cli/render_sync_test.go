// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

func syncEvent(sm *signalpb.AppSyncMessage) manager.Event {
	return manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1700000000000), ServerTimestamp: proto.Uint64(1700000000001)},
		ServerDelivered: 1700000000002,
		Message: &manager.Message{Sender: protocol.Address{ServiceID: "self-aci", DeviceID: 1},
			Content: content(&signalpb.AppContent{Body: &signalpb.AppContent_SyncMessage{SyncMessage: sm}})},
	}
}

func TestRenderSync(t *testing.T) {
	peer := uuidRaw(peerACI)
	c := &RenderContext{Self: selfNumber, SelfACI: "self-aci", Lookup: func(aci string) (string, string) {
		if aci == peerACI {
			return peerNumber, ""
		}
		return "", ""
	}}
	cases := []struct {
		sm          *signalpb.AppSyncMessage
		plain, json string
	}{
		{&signalpb.AppSyncMessage{Sent: &signalpb.AppSent{Timestamp: proto.Uint64(1700000000000), DestinationServiceIdBinary: peer,
			Message: &signalpb.AppDataMessage{Timestamp: proto.Uint64(1700000000000), Body: proto.String("hi")}}},
			"Received a sync message\nReceived sync sent message\n  To: " + peerNumber + "\n  Timestamp: 1700000000000 (2023-11-14T22:13:20.000Z)\n  Message timestamp:",
			`"syncMessage":{"sentMessage":{"destination":"` + peerNumber + `","destinationNumber":"` + peerNumber + `","destinationUuid":"` + peerACI + `","timestamp":1700000000000,"message":"hi"`},
		{&signalpb.AppSyncMessage{Read: []*signalpb.AppSyncMessage_Read{{SenderAciBinary: peer, Timestamp: proto.Uint64(1699999999000)}}},
			"Received a sync message\nReceived sync read messages list\n- From: " + peerNumber + " Message timestamp: 1699999999000 (2023-11-14T22:13:19.000Z)\n",
			`"syncMessage":{"readMessages":[{"sender":"` + peerNumber + `","senderNumber":"` + peerNumber + `","senderUuid":"` + peerACI + `","timestamp":1699999999000}]}`},
		{&signalpb.AppSyncMessage{Viewed: []*signalpb.AppSyncMessage_Read{{SenderAciBinary: peer, Timestamp: proto.Uint64(1699999999000)}}},
			"Received sync viewed messages list\n- From: " + peerNumber, `"syncMessage":{}`},
		{&signalpb.AppSyncMessage{Blocked: &signalpb.AppSyncMessage_Blocked{Numbers: []string{"+15555550188"}, AcisBinary: [][]byte{peer}, GroupIds: [][]byte{{1, 2}}}},
			"Received sync message with block list\nBlocked:\n- +15555550188\n- AQI=\n",
			`"syncMessage":{"blockedNumbers":["+15555550188"],"blockedGroupIds":["AQI="]}`},
		{&signalpb.AppSyncMessage{MessageRequestResponse: &signalpb.AppMessageRequestResponse{Type: signalpb.AppMessageRequestResponse_SPAM.Enum(), ThreadAciBinary: peer}},
			"Received message request response:\n  Type: UNKNOWN\n  For Person: " + peerNumber + "\n", `"syncMessage":{}`},
		{&signalpb.AppSyncMessage{Contacts: &signalpb.AppSyncMessage_Contacts{Complete: proto.Bool(true)}},
			"Received complete sync contacts:\n", `"syncMessage":{"type":"CONTACTS_SYNC"}`},
		{&signalpb.AppSyncMessage{Padding: []byte{1}}, "Received a sync message\n\n", `"syncMessage":{}`},
	}
	for i, tc := range cases {
		e := syncEvent(tc.sm)
		if got := RenderPlain(c, e); !strings.Contains(got, tc.plain) {
			t.Errorf("case %d plain lacks %q:\n%s", i, tc.plain, got)
		}
		if got := RenderJSON(c, e); !strings.Contains(got, tc.json) {
			t.Errorf("case %d JSON lacks %s:\n%s", i, tc.json, got)
		}
	}
}

func TestRenderStory(t *testing.T) {
	img := &signalpb.AppAttachmentPointer{ContentType: proto.String("image/jpeg"), CdnNumber: proto.Uint32(3), Key: make([]byte, 64),
		AttachmentIdentifier: &signalpb.AppAttachmentPointer_CdnKey{CdnKey: "K"}, Size: proto.Uint32(4)}
	file := &signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: &signalpb.AppStoryMessage{
		AllowsReplies: proto.Bool(true), Attachment: &signalpb.AppStoryMessage_FileAttachment{FileAttachment: img}}}}
	e := manager.Event{Envelope: &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1700000000000), ServerTimestamp: proto.Uint64(1700000000001)},
		Message: &manager.Message{Sender: protocol.Address{ServiceID: peerACI, DeviceID: 1}, Content: content(file)}}
	got := RenderPlain(ctx, e)
	want := "  Story: with replies: true\n  Attachments:\n    Content-Type: image/jpeg\n    Type: Pointer\n    Id: K.jpg\n    Size: 4 bytes\n\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("plain:\n%s\nwant suffix:\n%s", got, want)
	}
	if j := RenderJSON(ctx, e); !strings.Contains(j, `"storyMessage":{"allowsReplies":true,"fileAttachment":{"contentType":"image/jpeg","filename":null,"id":"K.jpg"`) {
		t.Errorf("JSON: %s", j)
	}

	text := &signalpb.AppContent{Body: &signalpb.AppContent_StoryMessage{StoryMessage: &signalpb.AppStoryMessage{
		Attachment: &signalpb.AppStoryMessage_TextAttachment{TextAttachment: &signalpb.AppTextAttachment{Text: proto.String("hello"),
			TextStyle: signalpb.AppTextAttachment_BOLD.Enum(), TextForegroundColor: proto.Uint32(0xff102030),
			Background: &signalpb.AppTextAttachment_Gradient_{Gradient: &signalpb.AppTextAttachment_Gradient{Colors: []uint32{0xff000000, 0xffffffff}, Positions: []float32{0, 1}, Angle: proto.Uint32(90)}}}}}}}
	e.Message.Content = content(text)
	if got := RenderPlain(ctx, e); !strings.HasSuffix(got, "  Story: with replies: false\n  Body: hello\n\n") {
		t.Errorf("text plain:\n%s", got)
	}
	want = `"textAttachment":{"text":"hello","style":"BOLD","textForegroundColor":"#ff102030","backgroundGradient":{"startColor":"#ff000000","endColor":"#ffffffff","colors":["#ff000000","#ffffffff"],"positions":[0.0,1.0],"angle":90}}`
	if j := RenderJSON(ctx, e); !strings.Contains(j, want) {
		t.Errorf("text JSON: %s", j)
	}
}
