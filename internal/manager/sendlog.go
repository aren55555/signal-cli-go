// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// The message send log (spec/service/send-log.md).

const sendLogRetention = 24 * time.Hour

// logGroupID derives an entry's group id from its Content (§1.1): only a
// DataMessage's v2 group context counts, so edits get none.
func logGroupID(content []byte) []byte {
	var c signalpb.AppContent
	if proto.Unmarshal(content, &c) != nil {
		return nil
	}
	mk := c.GetDataMessage().GetGroupV2().GetMasterKey()
	if len(mk) != 32 {
		return nil
	}
	g := zkgroup.DeriveGroupSecretParams([32]byte(mk))
	return append([]byte(nil), g.GroupID[:]...)
}

func logRecipients(address string, devices []uint32) []store.SendLogRecipient {
	out := make([]store.SendLogRecipient, len(devices))
	for i, d := range devices {
		out[i] = store.SendLogRecipient{Address: address, Device: d}
	}
	return out
}

// logSend records a successful 1:1 send (§2.1) unless the send log is
// disabled.
func (s *Sender) logSend(content []byte, ts uint64, hint uint32, urgent bool, dest string, devices []uint32) {
	if s.DisableSendLog {
		return
	}
	pruneSendLog(s.Acct, s.now(), nil)
	s.Acct.SendLogAdd(store.SendLogEntry{Timestamp: ts, Content: content, Hint: hint, Urgent: urgent,
		GroupID: logGroupID(content), Recipients: logRecipients(dest, devices)})
}

// groupLog collects one group send's successful results into one entry
// (§1.3).
type groupLog struct {
	s       *Sender
	content []byte
	ts      uint64
	hint    uint32
	urgent  bool
	id      int64
}

func (g *groupLog) add(results []SendResult) {
	if g == nil || g.s.DisableSendLog {
		return
	}
	var rows []store.SendLogRecipient
	for _, r := range results {
		if r.Err == nil {
			rows = append(rows, logRecipients(r.Dest, r.Devices)...)
		}
	}
	if len(rows) == 0 {
		return
	}
	if g.id == 0 {
		g.id, _ = g.s.Acct.SendLogAdd(store.SendLogEntry{Timestamp: g.ts, Content: g.content, Hint: g.hint, Urgent: g.urgent,
			GroupID: logGroupID(g.content), Recipients: rows})
		return
	}
	g.s.Acct.SendLogAppend(g.id, rows)
}

// pruneSendLog drops entries older than a day (§3.3).
func pruneSendLog(a *store.Account, now time.Time, logf func(string, ...any)) {
	if n, _ := a.SendLogPrune(uint64(now.Add(-sendLogRetention).UnixMilli())); n > 0 && logf != nil {
		logf("Removed %d outdated entries from the message send log", n)
	}
}

// resendAction is one queued resend (§4.4); equal actions run once.
type resendAction struct {
	peer  string
	entry store.SendLogEntry
}

func (a resendAction) key() string {
	return fmt.Sprintf("%s|%d|%d|%t|%x|%x", a.peer, a.entry.Timestamp, a.entry.Hint, a.entry.Urgent, a.entry.GroupID, a.entry.Content)
}

// handleRetryReceipt is §4.1 and §4.3 for a DecryptionErrorMessage
// addressed to this device: it queues the resends and archives or
// renews sessions.
func (r *Receiver) handleRetryReceipt(sender protocol.Address, toPNI bool, d *signalpb.AppDecryptionErrorMessage) {
	a := r.Acct
	pruneSendLog(a, r.now(), nil)
	groupsOnly := len(d.GetRatchetKey()) == 0
	entries, _ := a.SendLogLookup(sender.ServiceID, sender.DeviceID, d.GetTimestamp(), groupsOnly)
	for _, e := range entries {
		r.pending.addResend(resendAction{peer: sender.ServiceID, entry: e})
	}
	if groupsOnly {
		// A sender-key failure: forget who has our key (§4.3).
		reset := false
		for _, e := range entries {
			if rec := a.Group(e.GroupID); rec != nil && rec.DistributionID != "" {
				dist, _ := uuidRaw(rec.DistributionID)
				delete(a.SenderKeys.Shared[protocol.DistributionID(dist).String()], device{sender.ServiceID, sender.DeviceID}.name())
				reset = true
			}
		}
		if !reset {
			for _, shared := range a.SenderKeys.Shared {
				for name := range shared {
					if bytes.HasPrefix([]byte(name), []byte(sender.ServiceID+".")) {
						delete(shared, name)
					}
				}
			}
		}
		a.SaveSenderKeys()
		return
	}
	// A 1:1 failure: only when the ratchet key is our current session's.
	b, ok := a.Sessions[sessionKey(toPNI, sender)]
	if !ok {
		return
	}
	rec, err := protocol.LoadSessionRecord(b)
	if err != nil || !rec.HasCurrent() || !bytes.Equal(rec.SenderRatchetKey(), d.GetRatchetKey()) {
		return
	}
	if len(entries) == 0 {
		r.queueRenewal(toPNI, sender.ServiceID)
		return
	}
	r.archiveAll(toPNI, sender.ServiceID)
}

func (p *actions) addResend(a resendAction) {
	if p.resends == nil {
		p.resends = map[string]resendAction{}
	}
	p.resends[a.key()] = a
}

// resend sends a logged entry again to all of the peer's devices with
// its timestamp, hint and urgency (§4.2). Group entries get a fresh
// sender-key distribution message.
func (s *Sender) resend(ctx context.Context, gm *Groups, a resendAction) error {
	e := a.entry
	content := e.Content
	opts := deliverOpts{urgent: e.Urgent, hint: e.Hint, unsealed: a.peer == s.Acct.Data.ACI, noDowngrade: true}
	if e.GroupID != nil {
		rec := s.Acct.Group(e.GroupID)
		if rec == nil {
			s.logf("Could not find a matching group for the groupId %s! Skipping message send.", base64.StdEncoding.EncodeToString(e.GroupID))
			return nil
		}
		st := StateOf(rec)
		if st == nil || st.Member(a.peer) == nil {
			s.logf("The target user is no longer in the group %s! Skipping message send.", base64.StdEncoding.EncodeToString(e.GroupID))
			return nil
		}
		dist, _ := uuidRaw(rec.DistributionID)
		skdm, err := s.distributionMessage(protocol.DistributionID(dist))
		if err != nil {
			return err
		}
		var c signalpb.AppContent
		if err := proto.Unmarshal(content, &c); err != nil {
			return err
		}
		c.SenderKeyDistributionMessage = skdm
		if content, err = proto.Marshal(&c); err != nil {
			return err
		}
		s.debugf("Group send path: resend after a retry receipt to %s, 1:1 with a fresh sender key distribution, timestamp %d", a.peer, e.Timestamp)
		opts.groupID = rec.GroupID
		var devices []uint32
		opts.devices = &devices
		if _, _, err := s.deliver(ctx, a.peer, content, e.Timestamp, opts); err != nil {
			return err
		}
		shared := s.Acct.SenderKeys.Shared[protocol.DistributionID(dist).String()]
		if shared == nil {
			shared = map[string]int64{}
			s.Acct.SenderKeys.Shared[protocol.DistributionID(dist).String()] = shared
		}
		for _, d := range devices {
			shared[device{a.peer, d}.name()] = s.now().UnixMilli()
		}
		return s.Acct.SaveSenderKeys()
	}
	s.debugf("Group send path: resend after a retry receipt to %s, 1:1, timestamp %d", a.peer, e.Timestamp)
	_, _, err := s.deliver(ctx, a.peer, content, e.Timestamp, opts)
	return err
}

// distributionMessage is the current SKDM for dist, creating our sender
// key if there is none.
func (s *Sender) distributionMessage(dist protocol.DistributionID) ([]byte, error) {
	name, rec, err := s.ownSenderKey(dist)
	if err != nil {
		return nil, err
	}
	rec, skdm, err := rec.NewDistribution(s.rand(), dist)
	if err != nil {
		return nil, err
	}
	return skdm, s.saveSenderKey(name, rec)
}
