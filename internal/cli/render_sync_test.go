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
