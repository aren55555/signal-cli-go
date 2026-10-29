// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

// TestSendLog: send-log.md — a 1:1 data message is logged with its
// devices; a retry receipt with our current ratchet key archives the
// session and resends the stored Content at the same timestamp; a
// delivery receipt clears the entry; --disable-send-log writes nothing.
func TestSendLog(t *testing.T) {
	ctx := context.Background()
	fc, s := dataWorld(t)
	ts, res := s.SendData(ctx, []string{dataBACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("hi")}, Urgent: true})
	if res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	entries, _ := s.Acct.SendLogLookup(dataBACI, 1, ts, false)
	if len(entries) != 1 || entries[0].Hint != protocol.ContentHintResendable || !entries[0].Urgent || entries[0].GroupID != nil {
		t.Fatalf("entries %+v", entries)
	}
	if e, _ := s.Acct.SendLogLookup(dataBACI, 1, ts, true); len(e) != 0 {
		t.Error("a 1:1 entry matched a sender-key retry")
	}

	rec, _ := protocol.LoadSessionRecord(s.Acct.Sessions[sessionKey(false, protocol.Address{ServiceID: dataBACI, DeviceID: 1})])
	r := &Receiver{Acct: s.Acct, Sender: s, Decryptor: &Decryptor{Acct: s.Acct}}
	r.handleRetryReceipt(protocol.Address{ServiceID: dataBACI, DeviceID: 1}, false,
		&signalpb.AppDecryptionErrorMessage{Timestamp: proto.Uint64(ts), DeviceId: proto.Uint32(2), RatchetKey: rec.SenderRatchetKey()})
	if got, _ := protocol.LoadSessionRecord(s.Acct.Sessions[sessionKey(false, protocol.Address{ServiceID: dataBACI, DeviceID: 1})]); got.HasCurrent() {
		t.Error("session not archived before the resend")
	}
	if len(r.pending.resends) != 1 || len(r.pending.renew) != 0 {
		t.Fatalf("pending %+v", r.pending)
	}
	// The fake server would hand out the consumed one-time prekey again;
	// a real one serves another or none.
	fd := fc.devices[dataBACI][1]
	fd.bundle.PreKeyID, fd.bundle.PreKey = nil, nil
	before := len(fc.delivered)
	r.flush(ctx)
	got := deliveredTo(t, fc, dataBACI)
	if len(fc.delivered) == before || got[len(got)-1].GetDataMessage().GetBody() != "hi" || fc.delivered[len(fc.delivered)-1].msg.ClientTimestamp != ts {
		t.Fatalf("resend not delivered at the original timestamp")
	}

	s.Acct.SendLogDelivered(dataBACI, 1, []uint64{ts})
	if e, _ := s.Acct.SendLogLookup(dataBACI, 1, ts, false); len(e) != 0 {
		t.Error("delivery receipt did not clear the entry")
	}

	s.DisableSendLog = true
	ts2, _ := s.SendData(ctx, []string{dataBACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("off")}})
	if e, _ := s.Acct.SendLogLookup(dataBACI, 1, ts2, false); len(e) != 0 {
		t.Error("logged with --disable-send-log")
	}
}

// A retry receipt whose ratchet key is not our current one, with nothing
// logged, changes no session; with our key it queues a renewal.
func TestRetryReceiptWithoutEntries(t *testing.T) {
	ctx := context.Background()
	_, s := dataWorld(t)
	s.DisableSendLog = true
	ts, _ := s.SendData(ctx, []string{dataBACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("x")}})
	r := &Receiver{Acct: s.Acct, Sender: s}
	peer := protocol.Address{ServiceID: dataBACI, DeviceID: 1}
	r.handleRetryReceipt(peer, false, &signalpb.AppDecryptionErrorMessage{Timestamp: proto.Uint64(ts), DeviceId: proto.Uint32(2), RatchetKey: make([]byte, 33)})
	if len(r.pending.renew) != 0 {
		t.Error("renewal for a foreign ratchet key")
	}
	rec, _ := protocol.LoadSessionRecord(s.Acct.Sessions[sessionKey(false, peer)])
	r.handleRetryReceipt(peer, false, &signalpb.AppDecryptionErrorMessage{Timestamp: proto.Uint64(ts), DeviceId: proto.Uint32(2), RatchetKey: rec.SenderRatchetKey()})
	if len(r.pending.renew) != 1 {
		t.Error("no renewal for our ratchet key")
	}
}
