// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"encoding/base64"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
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
	_, _, err = s.send(ctx, dest, c, ts, false, protocol.ContentHintImplicit)
	return ts, err
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

// SendRetryReceipt sends a DecryptionErrorMessage to the device whose
// message failed, as PlaintextContent: 0xC0, the Content, 0x80, not
// encrypted (pipe.md §7.2 item 2). Sealed, it rides in sealed sender with
// the plaintext content type; otherwise the envelope type is 8.
func (s *Sender) SendRetryReceipt(ctx context.Context, dest string, device uint32, dem *signalpb.AppDecryptionErrorMessage) error {
	demBytes, err := proto.Marshal(dem)
	if err != nil {
		return err
	}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_DecryptionErrorMessage{DecryptionErrorMessage: demBytes}})
	if err != nil {
		return err
	}
	plain := append(append([]byte{0xC0}, c...), 0x80)
	entryType, wire := envPlaintext, plain
	acc := s.accessFor(ctx, dest)
	if acc != nil {
		identity, err := curve25519.NewPrivateKey(s.Acct.Data.ACIIdentity.Private)
		if err != nil {
			return err
		}
		ik, ok := s.Acct.Identities[dest]
		recipientKey, perr := curve25519.ParsePublicKey(ik.Key)
		if !ok || perr != nil {
			acc = nil
		} else {
			usmc := protocol.EncodeSealedContent(protocol.SealedContentPlaintext, acc.cert, plain, protocol.ContentHintImplicit, nil)
			if wire, err = protocol.SealV1(s.rand(), identity, recipientKey, usmc); err != nil {
				return err
			}
			entryType = envUnidentified
		}
	}
	// The registration ID comes from our session with that device, if any.
	var regID uint32
	if recs, err := s.sessions(dest); err == nil && recs[device] != nil {
		regID = recs[device].RemoteRegistrationID()
	}
	body := outgoingMessages{Destination: dest, Timestamp: s.timestamp(), Online: false, Urgent: false,
		Messages: []outgoingMessage{{Type: entryType, DestinationDeviceID: device, DestinationRegistrationID: regID, Content: base64.StdEncoding.EncodeToString(wire)}}}
	b, err := api.Marshal(body)
	if err != nil {
		return err
	}
	path := "/v1/messages/" + dest + "?story=false"
	var r *chat.Response
	if acc != nil {
		r, err = s.Unauth.Request(ctx, "PUT", path, append(append([]chat.Header{}, jsonHeader...), acc.header()), b)
		if err == nil && r.Status == 401 {
			r, err = s.Chat.Request(ctx, "PUT", path, jsonHeader, b)
		}
	} else {
		r, err = s.Chat.Request(ctx, "PUT", path, jsonHeader, b)
	}
	if err != nil {
		return err
	}
	if r.Status != 200 {
		return &SendStatusError{Status: r.Status}
	}
	return nil
}
