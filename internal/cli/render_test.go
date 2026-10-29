// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

const (
	selfNumber = "+15555550100"
	peerACI    = "b0b0b0b0-0000-4000-8000-000000000002"
	peerNumber = "+15555550101"
)

func content(c *signalpb.AppContent) []byte {
	b, _ := proto.Marshal(c)
	return b
}

func dataEvent(body string, sealed bool) manager.Event {
	return manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1792796479741), ServerTimestamp: proto.Uint64(1792796479879), SourceDeviceId: proto.Uint32(2)},
		ServerDelivered: 1792796483544,
		Message: &manager.Message{Sender: protocol.Address{ServiceID: peerACI, DeviceID: 2}, Sealed: sealed,
			Content: content(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{
				Body: proto.String(body), Timestamp: proto.Uint64(1792796479741), ProfileKey: make([]byte, 32), ExpireTimerVersion: proto.Uint32(1)}}})},
	}
}

var ctx = &RenderContext{Self: selfNumber, Lookup: func(aci string) (string, string) {
	if aci == peerACI+"-known" {
		return peerNumber, ""
	}
	return "", ""
}}

// TestPlainData is the reference's own output for a message from our
// client (Layer C transcript, anonymized).
func TestPlainData(t *testing.T) {
	got := RenderPlain(ctx, dataEvent("layer-c 2: from signal-cli-go, ünïcødé ✓", false))
	want := "Envelope from: " + peerACI + " (device: 2) to " + selfNumber + "\n" +
		"Timestamp: 1792796479741 (2026-10-23T23:01:19.741Z)\n" +
		"Server timestamps: received: 1792796479879 (2026-10-23T23:01:19.879Z) delivered: 1792796483544 (2026-10-23T23:01:23.544Z)\n" +
		"Message timestamp: 1792796479741 (2026-10-23T23:01:19.741Z)\n" +
		"Body: layer-c 2: from signal-cli-go, ünïcødé ✓\n" +
		"With profile key\n\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := RenderPlain(ctx, dataEvent("x", true)); !strings.Contains(got, "\nSent by unidentified/sealed sender\nMessage timestamp:") {
		t.Errorf("sealed:\n%s", got)
	}
}

// TestJSONData is the reference's JSON line for the same kind of message.
func TestJSONData(t *testing.T) {
	got := RenderJSON(ctx, dataEvent("layer-c 5: sealed reply from signal-cli-go", true))
	want := `{"envelope":{"source":"` + peerACI + `","sourceNumber":null,"sourceUuid":"` + peerACI + `","sourceName":"","sourceDevice":2,` +
		`"timestamp":1792796479741,"serverReceivedTimestamp":1792796479879,"serverDeliveredTimestamp":1792796483544,` +
		`"dataMessage":{"timestamp":1792796479741,"message":"layer-c 5: sealed reply from signal-cli-go","expiresInSeconds":0,"isExpirationUpdate":false,"viewOnce":false}},` +
		`"account":"` + selfNumber + `"}`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestReceiptAndTyping(t *testing.T) {
	e := manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1700000000500), ServerTimestamp: proto.Uint64(1700000000600)},
		ServerDelivered: 1700000000700,
		Message: &manager.Message{Sender: protocol.Address{ServiceID: peerACI, DeviceID: 1}, Content: content(&signalpb.AppContent{
			Body: &signalpb.AppContent_ReceiptMessage{ReceiptMessage: &signalpb.AppReceipt{Kind: signalpb.AppReceipt_DELIVERY.Enum(), Timestamps: []uint64{1700000000000, 1700000000001}}}})},
	}
	plain := RenderPlain(ctx, e)
	if !strings.HasSuffix(plain, "Received a receipt message\n  When: 1700000000500 (2023-11-14T22:13:20.500Z)\n  Is delivery receipt\n  Timestamps:\n  - 1700000000000 (2023-11-14T22:13:20.000Z)\n  - 1700000000001 (2023-11-14T22:13:20.001Z)\n\n") {
		t.Errorf("receipt plain:\n%s", plain)
	}
	if j := RenderJSON(ctx, e); !strings.Contains(j, `"receiptMessage":{"when":1700000000500,"isDelivery":true,"isRead":false,"isViewed":false,"timestamps":[1700000000000,1700000000001]}}`) {
		t.Errorf("receipt json %s", j)
	}
	e.Message.Content = content(&signalpb.AppContent{Body: &signalpb.AppContent_TypingMessage{TypingMessage: &signalpb.AppTyping{
		Action: signalpb.AppTyping_STOPPED.Enum(), Timestamp: proto.Uint64(1700000000000)}}})
	if p := RenderPlain(ctx, e); !strings.HasSuffix(p, "Received a typing message\n  Action: STOPPED\n  Timestamp: 1700000000000 (2023-11-14T22:13:20.000Z)\n\n") {
		t.Errorf("typing plain:\n%s", p)
	}
	if j := RenderJSON(ctx, e); !strings.Contains(j, `"typingMessage":{"action":"STOPPED","timestamp":1700000000000}}`) {
		t.Errorf("typing json %s", j)
	}
}

// TestSyncSent: the transcript block as the reference printed our own
// transcript on A's primary (Layer C transcript, anonymized).
func TestSyncSent(t *testing.T) {
	e := manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1792796371867), ServerTimestamp: proto.Uint64(1792796372312)},
		ServerDelivered: 1792796504569,
		Message: &manager.Message{Sender: protocol.Address{ServiceID: "self-aci", DeviceID: 2}, Content: content(&signalpb.AppContent{
			Body: &signalpb.AppContent_SyncMessage{SyncMessage: &signalpb.AppSyncMessage{Sent: &signalpb.AppSent{
				DestinationServiceId: proto.String(peerACI), Timestamp: proto.Uint64(1792796371867),
				Message: &signalpb.AppDataMessage{Body: proto.String("hello"), Timestamp: proto.Uint64(1792796371867), ProfileKey: make([]byte, 32)}}}}})},
	}
	c := &RenderContext{Self: selfNumber, SelfACI: "self-aci", Lookup: func(string) (string, string) { return "", "" }}
	got := RenderPlain(c, e)
	want := "Envelope from: " + selfNumber + " (device: 2) to " + selfNumber + "\n" +
		"Timestamp: 1792796371867 (2026-10-23T22:59:31.867Z)\n" +
		"Server timestamps: received: 1792796372312 (2026-10-23T22:59:32.312Z) delivered: 1792796504569 (2026-10-23T23:01:44.569Z)\n" +
		"Received a sync message\n" +
		"Received sync sent message\n" +
		"  To: " + peerACI + "\n" +
		"  Timestamp: 1792796371867 (2026-10-23T22:59:31.867Z)\n" +
		"  Message timestamp: 1792796371867 (2026-10-23T22:59:31.867Z)\n" +
		"  Body: hello\n" +
		"  With profile key\n\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	j := RenderJSON(c, e)
	if !strings.Contains(j, `"syncMessage":{"sentMessage":{"destination":"`+peerACI+`","destinationNumber":null,"destinationUuid":"`+peerACI+`","timestamp":1792796371867,"message":"hello","expiresInSeconds":0,"isExpirationUpdate":false,"viewOnce":false}}`) {
		t.Errorf("sync json %s", j)
	}
}

// TestUntrusted: the three lines the reference printed for the refused
// message, and the JSON exception key first.
func TestUntrusted(t *testing.T) {
	e := manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1792796371867), ServerTimestamp: proto.Uint64(1792796372233), SourceServiceIdBinary: uuidRaw(peerACI), SourceDeviceId: proto.Uint32(2)},
		ServerDelivered: 1792796380808,
		Err:             protocol.ErrUntrustedIdentity,
	}
	got := RenderPlain(ctx, e)
	want := "Envelope from: " + peerACI + " (device: 2) to " + selfNumber + "\n" +
		"Timestamp: 1792796371867 (2026-10-23T22:59:31.867Z)\n" +
		"Server timestamps: received: 1792796372233 (2026-10-23T22:59:32.233Z) delivered: 1792796380808 (2026-10-23T22:59:40.808Z)\n" +
		"The user’s key is untrusted, either the user has reinstalled Signal or a third party sent this message.\n" +
		"Use 'signal-cli -a " + selfNumber + " listIdentities -n " + peerACI + "', verify the key and run 'signal-cli -a " + selfNumber + " trust -v \"FINGER_PRINT\" " + peerACI + "' to mark it as trusted\n" +
		"If you don't care about security, use 'signal-cli -a " + selfNumber + " trust -a " + peerACI + "' to trust it without verification\n\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if j := RenderJSON(ctx, e); !strings.HasPrefix(j, `{"exception":{"message":`) || !strings.Contains(j, `"type":"UntrustedIdentityException"}`) {
		t.Errorf("json %s", j)
	}
	other := e
	other.Err = &manager.DecryptError{Err: errors.New("x")}
	if p := RenderPlain(ctx, other); !strings.Contains(p, "\nException: ") {
		t.Errorf("other error:\n%s", p)
	}
}

// TestPlainNoSenderKey is the reference's rendering of a sealed
// sender-key message it holds no key for (live, 2026-10-29): sealed-sender
// line, then libsignal's NoSessionException.
func TestPlainNoSenderKey(t *testing.T) {
	ev := manager.Event{
		Envelope:        &signalpb.ServiceEnvelope{ClientTimestamp: proto.Uint64(1793317314301), ServerTimestamp: proto.Uint64(1793317314572), SourceDeviceId: proto.Uint32(1)},
		ServerDelivered: 1793317314600,
		Err: &manager.DecryptError{Sender: protocol.Address{ServiceID: peerACI, DeviceID: 1}, Sealed: true,
			Err: &manager.NoSenderKeyError{Distribution: "244db90c-c537-4b7d-8024-76fd4ca4e3d9"}},
	}
	got := RenderPlain(ctx, ev)
	want := "Sent by unidentified/sealed sender\n" +
		"Exception: org.signal.libsignal.protocol.NoSessionException: missing sender key state for distribution ID 244db90c-c537-4b7d-8024-76fd4ca4e3d9 (ProtocolNoSessionException)\n"
	if !strings.Contains(got, want) {
		t.Errorf("got\n%s", got)
	}
}
