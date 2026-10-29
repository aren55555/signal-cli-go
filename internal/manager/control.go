// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

// Receipt kinds (content.md §5.1).
const (
	ReceiptDelivery = signalpb.AppReceipt_DELIVERY
	ReceiptRead     = signalpb.AppReceipt_READ
	ReceiptViewed   = signalpb.AppReceipt_VIEWED
)

// SendReceipt sends one ReceiptMessage to every device of dest: content
// hint IMPLICIT, not urgent, sealed when possible, no transcript
// (content.md §5.1, pipe.md §7.1).
func (s *Sender) SendReceipt(ctx context.Context, dest string, kind signalpb.AppReceipt_Kind, timestamps []uint64) error {
	_, err := s.SendReceiptAt(ctx, dest, kind, timestamps)
	return err
}

// SendReceiptAt is SendReceipt returning the receipt's timestamp.
func (s *Sender) SendReceiptAt(ctx context.Context, dest string, kind signalpb.AppReceipt_Kind, timestamps []uint64) (uint64, error) {
	content := &signalpb.AppContent{Body: &signalpb.AppContent_ReceiptMessage{
		ReceiptMessage: &signalpb.AppReceipt{Kind: kind.Enum(), Timestamps: timestamps}}}
	if err := s.addPNISignature(content, dest); err != nil {
		return 0, err
	}
	c, err := proto.Marshal(content)
	if err != nil {
		return 0, err
	}
	ts := s.timestamp()
	var devices []uint32
	if _, _, err = s.deliver(ctx, dest, c, ts, deliverOpts{hint: protocol.ContentHintImplicit, devices: &devices}); err != nil {
		return ts, err
	}
	s.logSend(c, ts, protocol.ContentHintImplicit, false, dest, devices)
	return ts, nil
}

// SendTyping sends a TypingMessage (content.md §5.2): online, not urgent,
// hint IMPLICIT, sealed when possible. ts 0 picks a new timestamp.
func (s *Sender) SendTyping(ctx context.Context, dest string, stop bool, ts uint64) (uint64, error) {
	if ts == 0 {
		ts = s.timestamp()
	}
	action := signalpb.AppTyping_STARTED
	if stop {
		action = signalpb.AppTyping_STOPPED
	}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_TypingMessage{
		TypingMessage: &signalpb.AppTyping{Timestamp: proto.Uint64(ts), Action: action.Enum()}}})
	if err != nil {
		return 0, err
	}
	_, _, err = s.sendOnline(ctx, dest, c, ts, false, true, protocol.ContentHintImplicit)
	return ts, err
}

// SendNullMessage sends a NullMessage whose padding is a serialized
// DataMessage with a base64 body of 1..140 random bytes (content.md §6).
func (s *Sender) SendNullMessage(ctx context.Context, dest string) error {
	nm, err := s.nullMessage()
	if err != nil {
		return err
	}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_NullMessage{NullMessage: nm}})
	if err != nil {
		return err
	}
	_, _, err = s.send(ctx, dest, c, s.timestamp(), false, protocol.ContentHintImplicit)
	return err
}

// SendRetryReceipt sends a DecryptionErrorMessage about a message from
// one of dest's devices, as PlaintextContent: 0xC0, the Content, 0x80, not
// encrypted (pipe.md §7.2 item 2). It is a normal 1:1 send to all of
// dest's devices (the message names the device); sealed, it rides in
// sealed sender with the plaintext content type, otherwise the envelope
// type is 8. groupID is the failed message's sealed-sender group ID, if
// any; the sealed retry receipt carries it.
func (s *Sender) SendRetryReceipt(ctx context.Context, dest string, dem *signalpb.AppDecryptionErrorMessage, groupID []byte) error {
	demBytes, err := proto.Marshal(dem)
	if err != nil {
		return err
	}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_DecryptionErrorMessage{DecryptionErrorMessage: demBytes}})
	if err != nil {
		return err
	}
	plain := append(append([]byte{0xC0}, c...), 0x80)
	_, _, err = s.deliver(ctx, dest, plain, s.timestamp(), deliverOpts{plaintextContent: true, hint: protocol.ContentHintImplicit, noDowngrade: true, groupID: groupID})
	return err
}
