// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Data messages through signal-cli's common send path
// (spec/service/content-features.md §1.3).

// pniSignaturePrefix precedes the ACI identity key in the signed message
// (§4.1): 32 bytes of 0xFF, then "Signal_PNI_Signature".
func pniSignedMessage(aciIdentity []byte) []byte {
	msg := make([]byte, 32, 32+20+33)
	for i := range msg {
		msg[i] = 0xFF
	}
	msg = append(msg, "Signal_PNI_Signature"...)
	return append(msg, aciIdentity...)
}

// pniSignature builds our PniSignatureMessage (§4.1, §2.14).
func pniSignature(a *store.Account) (*signalpb.AppPniSignature, error) {
	pniKey, err := curve25519.NewPrivateKey(a.Data.PNIIdentity.Private)
	if err != nil {
		return nil, err
	}
	sig, err := pniKey.Sign(rand.Reader, pniSignedMessage(a.Data.ACIIdentity.Public))
	if err != nil {
		return nil, err
	}
	pni, ok := uuidRaw(a.Data.PNI)
	if !ok {
		return nil, errors.New("account has no PNI")
	}
	return &signalpb.AppPniSignature{Pni: pni[:], Signature: sig}, nil
}

// verifyPNISignature checks a peer's PNI signature: the PNI identity key
// over the ACI identity key (§4.4).
func verifyPNISignature(aciIdentity, pniIdentity, sig []byte) bool {
	k, err := curve25519.ParsePublicKey(pniIdentity)
	if err != nil {
		return false
	}
	return k.Verify(pniSignedMessage(aciIdentity), sig)
}

// requiredVersion is §1.4's requiredProtocolVersion.
func requiredVersion(dm *signalpb.AppDataMessage) uint32 {
	v := uint32(0)
	raise := func(x uint32) {
		if x > v {
			v = x
		}
	}
	hasMention := func(rs []*signalpb.AppBodyRange) bool {
		for _, r := range rs {
			if r.GetMentionAciBinary() != nil || r.GetMentionAci() != "" {
				return true
			}
		}
		return false
	}
	if hasMention(dm.GetBodyRanges()) || hasMention(dm.GetQuote().GetBodyRanges()) {
		raise(6)
	}
	if dm.GetIsViewOnce() {
		raise(3)
	}
	if dm.Reaction != nil {
		raise(4)
	}
	if dm.Payment != nil {
		raise(7)
	}
	if dm.PollCreate != nil {
		raise(8)
	}
	return v
}

// DataSend describes one data-message command.
type DataSend struct {
	// Message is the feature part of the DataMessage (body, quote, …);
	// SendData adds timestamp, timer, profile key and protocol version.
	Message    *signalpb.AppDataMessage
	EditTarget uint64 // non-zero: send as an edit of that message
	NotifySelf bool   // send to ourselves as a normal message
	Urgent     bool
	Timestamp  uint64 // 0: a new one
	// Prepare runs on the open session before anything is sent (uploads);
	// an error aborts the send.
	Prepare func(ctx context.Context, s *Sender) error
}

// SendResult is one recipient's outcome.
type SendResult struct {
	Dest string
	Err  error
}

// SendData sends d to each destination ACI with one shared timestamp and
// returns that timestamp and the per-recipient results.
func (s *Sender) SendData(ctx context.Context, dests []string, d DataSend) (uint64, []SendResult) {
	ts := d.Timestamp
	if ts == 0 {
		ts = s.timestamp()
	}
	var results []SendResult
	for _, dest := range dests {
		results = append(results, SendResult{Dest: dest, Err: s.sendDataTo(ctx, dest, d, ts)})
	}
	return ts, results
}

// enrich is §1.3 step 3 for a 1:1 recipient (or the own transcript).
func (s *Sender) enrich(dest string, base *signalpb.AppDataMessage, ts uint64, self bool) *signalpb.AppDataMessage {
	dm := proto.Clone(base).(*signalpb.AppDataMessage)
	dm.Timestamp = proto.Uint64(ts)
	rc := s.Acct.Recipients[dest]
	if dm.GetFlags()&2 == 0 { // a timer update carries its own timer
		if rc.Timer > 0 {
			dm.ExpireTimer = proto.Uint32(rc.Timer)
		}
		v := rc.TimerVersion
		if v == 0 {
			v = 1
		}
		dm.ExpireTimerVersion = proto.Uint32(v)
	}
	if !self && !rc.Blocked {
		dm.ProfileKey = s.Acct.Data.ProfileKey
	}
	dm.RequiredProtocolVersion = proto.Uint32(requiredVersion(dm))
	return dm
}

func (s *Sender) sendDataTo(ctx context.Context, dest string, d DataSend, ts uint64) error {
	self := dest == s.Acct.Data.ACI
	if self && (!d.NotifySelf || d.EditTarget != 0) {
		// Only a sent transcript to our other devices (§1.3).
		dm := s.enrich(dest, d.Message, ts, true)
		if !s.Acct.IsMultiDevice() {
			return nil // nothing to sync to (sync.md §4.2)
		}
		return s.transcript(ctx, dest, dm, d.EditTarget, ts)
	}
	dm := s.enrich(dest, d.Message, ts, false)
	content := &signalpb.AppContent{}
	if d.EditTarget != 0 {
		content.Body = &signalpb.AppContent_EditMessage{EditMessage: &signalpb.AppEdit{TargetSentTimestamp: proto.Uint64(d.EditTarget), DataMessage: dm}}
	} else {
		content.Body = &signalpb.AppContent_DataMessage{DataMessage: dm}
		if err := s.addPNISignature(content, dest); err != nil {
			return err
		}
	}
	// The recipient becomes a profile-sharing contact (§1.3 step 3).
	if !self {
		rc := s.Acct.Recipients[dest]
		if !rc.ProfileSharing || rc.Hidden || !rc.IsContact() {
			rc.ProfileSharing, rc.Hidden = true, false
			s.Acct.Recipients[dest] = rc
			s.Acct.SaveRecipients()
		}
	}
	c, err := proto.Marshal(content)
	if err != nil {
		return err
	}
	needsSync, _, err := s.send(ctx, dest, c, ts, d.Urgent, protocol.ContentHintResendable)
	if err != nil {
		return err
	}
	if self {
		return nil
	}
	if needsSync || s.Acct.IsMultiDevice() {
		if err := s.transcript(ctx, dest, dm, d.EditTarget, ts); err != nil {
			return errTranscript{err}
		}
	}
	return nil
}

// errTranscript: the message went out but the transcript did not; the
// send still counts as a success.
type errTranscript struct{ err error }

func (e errTranscript) Error() string {
	return "sent, but the sync transcript failed: " + e.err.Error()
}

// addPNISignature adds Content field 10 when the recipient needs it
// (§4.2).
func (s *Sender) addPNISignature(content *signalpb.AppContent, dest string) error {
	if !s.Acct.Recipients[dest].NeedsPNISignature {
		return nil
	}
	sig, err := pniSignature(s.Acct)
	if err != nil {
		return err
	}
	content.PniSignatureMessage = sig
	return nil
}

// transcript sends the SyncMessage.Sent for dm (or its edit) to our other
// devices (sync.md §4.1, §4.2).
func (s *Sender) transcript(ctx context.Context, dest string, dm *signalpb.AppDataMessage, editTarget, ts uint64) error {
	bin, _ := protocol.ServiceIDToBinary(dest)
	status := &signalpb.AppUnidentifiedStatus{DestinationServiceIdBinary: bin, Unidentified: proto.Bool(false)}
	if strings.HasPrefix(dest, "PNI:") {
		if id, ok := s.Acct.Identities[dest]; ok {
			status.DestinationPniIdentityKey = id.Key
		}
	}
	sent := &signalpb.AppSent{
		Timestamp:                  proto.Uint64(ts),
		UnidentifiedStatus:         []*signalpb.AppUnidentifiedStatus{status},
		IsRecipientUpdate:          proto.Bool(false),
		DestinationServiceIdBinary: bin,
	}
	number := s.Acct.Recipients[dest].Number
	if dest == s.Acct.Data.ACI {
		number = s.Acct.Data.Number
	}
	if number != "" {
		sent.DestinationE164 = proto.String(number)
	}
	if dm.GetExpireTimer() > 0 {
		sent.ExpirationStartTimestamp = proto.Uint64(uint64(s.now().UnixMilli()))
	}
	if editTarget != 0 {
		sent.EditMessage = &signalpb.AppEdit{TargetSentTimestamp: proto.Uint64(editTarget), DataMessage: dm}
	} else {
		if dm.GetIsViewOnce() && len(dm.Attachments) > 0 {
			dm = proto.Clone(dm).(*signalpb.AppDataMessage)
			dm.Attachments = nil
		}
		sent.Message = dm
	}
	return s.sendSync(ctx, &signalpb.AppSyncMessage{Sent: sent}, ts, false)
}
