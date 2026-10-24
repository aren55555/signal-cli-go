package cli

import (
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

func featureEvent(dm *signalpb.AppDataMessage) manager.Event {
	dm.Timestamp = proto.Uint64(1700000000000)
	return manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1700000000000), ServerTimestamp: proto.Uint64(1700000000001)},
		ServerDelivered: 1700000000002,
		Message: &manager.Message{Sender: protocol.Address{ServiceID: peerACI, DeviceID: 1},
			Content: content(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: dm}})},
	}
}

func TestRenderFeatures(t *testing.T) {
	peer := uuidRaw(peerACI)
	dm := &signalpb.AppDataMessage{
		Body: proto.String("hi @x"),
		BodyRanges: []*signalpb.AppBodyRange{
			{Start: proto.Uint32(3), Length: proto.Uint32(2), AssociatedValue: &signalpb.AppBodyRange_MentionAciBinary{MentionAciBinary: peer}},
			{Start: proto.Uint32(0), Length: proto.Uint32(2), AssociatedValue: &signalpb.AppBodyRange_Style_{Style: signalpb.AppBodyRange_BOLD}},
		},
		Reaction: &signalpb.AppReaction{Emoji: proto.String("👍"), Remove: proto.Bool(false), TargetSentTimestamp: proto.Uint64(1699999999000), TargetAuthorAciBinary: peer},
		Quote:    &signalpb.AppQuote{Id: proto.Uint64(5), Text: proto.String("orig"), AuthorAciBinary: peer, Type: signalpb.AppQuote_NORMAL.Enum()},
		PollVote: &signalpb.AppPollVote{TargetAuthorAciBinary: peer, TargetSentTimestamp: proto.Uint64(1699999999000), OptionIndexes: []uint32{0, 2}, VoteCount: proto.Uint32(1)},
		PinMessage: &signalpb.AppPinMessage{TargetAuthorAciBinary: peer, TargetSentTimestamp: proto.Uint64(1699999999000),
			PinDuration: &signalpb.AppPinMessage_PinDurationForever{PinDurationForever: true}},
	}
	got := RenderPlain(ctx, featureEvent(dm))
	for _, want := range []string{
		"Body: hi @x\n",
		"Reaction:\n  Emoji: 👍\n  Target author: " + peerACI + "\n  Target timestamp: 1699999999000 (2023-11-14T22:13:19.000Z)\n  Is remove: false\n",
		"Quote:\n  Id: 5\n  Author: " + peerACI + "\n  Text: orig\n",
		"Mentions:\n- " + peerACI + ": 3 (length: 2)\n",
		"Text styles:\n- BOLD: 0 (length: 2)\n",
		"Poll Vote: \"" + peerACI + "\" (1699999999000 (2023-11-14T22:13:19.000Z)) selected 0,2 (vote #1)\n",
		"Pin Message:\n  Target author: " + peerACI + "\n  Target timestamp: 1699999999000 (2023-11-14T22:13:19.000Z)\n  Duration: forever\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plain output lacks %q:\n%s", want, got)
		}
	}
	// Order within the DATA block: reaction, quote, mentions, styles, poll vote, pin.
	order := []string{"Reaction:", "Quote:", "Mentions:", "Text styles:", "Poll Vote:", "Pin Message:"}
	last := -1
	for _, o := range order {
		i := strings.Index(got, o)
		if i < last {
			t.Errorf("%s out of order", o)
		}
		last = i
	}
	j := RenderJSON(ctx, featureEvent(dm))
	for _, want := range []string{
		`"reaction":{"emoji":"👍","targetAuthor":"` + peerACI + `","targetAuthorNumber":null,"targetAuthorUuid":"` + peerACI + `","targetSentTimestamp":1699999999000,"isRemove":false}`,
		`"quote":{"id":5,"author":"` + peerACI + `","authorNumber":null,"authorUuid":"` + peerACI + `","text":"orig","attachments":[]}`,
		`"mentions":[{"name":"` + peerACI + `","number":null,"uuid":"` + peerACI + `","start":3,"length":2}]`,
		`"textStyles":[{"style":"BOLD","start":0,"length":2}]`,
		`"pollVote":{"author":"` + peerACI + `","authorNumber":null,"authorUuid":"` + peerACI + `","targetSentTimestamp":1699999999000,"optionIndexes":[0,2],"voteCount":1}`,
		`"pinMessage":{"targetAuthor":"` + peerACI + `","targetAuthorNumber":null,"targetAuthorUuid":"` + peerACI + `","targetSentTimestamp":1699999999000,"pinDurationSeconds":-1}`,
	} {
		if !strings.Contains(j, want) {
			t.Errorf("JSON lacks %s\n%s", want, j)
		}
	}
	if strings.Index(j, `"reaction"`) > strings.Index(j, `"quote"`) || strings.Index(j, `"mentions"`) > strings.Index(j, `"pollVote"`) || strings.Index(j, `"pollVote"`) > strings.Index(j, `"textStyles"`) {
		t.Errorf("JSON key order: %s", j)
	}
}
