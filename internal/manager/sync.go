// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Receiving sync messages (spec/service/sync.md §2, §6).

// SyncMember is the one SyncMessage member a receiver acts on.
type SyncMember int

// Members in signal-service-java's priority order (§2.1).
const (
	SyncEmpty SyncMember = iota
	SyncSent
	SyncRequest
	SyncRead
	SyncViewed
	SyncViewOnceOpen
	SyncVerified
	SyncStickerPackOperation
	SyncBlocked
	SyncConfiguration
	SyncFetchLatest
	SyncMessageRequestResponse
	SyncGroups
	SyncOutgoingPayment
	SyncKeys
	SyncContacts
	SyncPniChangeNumber
	SyncDeviceNameChange
	SyncCallEvent
)

// SyncMemberOf picks the member a receiver sees: the first present in
// priority order; anything else is an empty sync message (§2.1).
func SyncMemberOf(sm *signalpb.AppSyncMessage) SyncMember {
	switch {
	case sm.Sent != nil:
		return SyncSent
	case sm.Request != nil:
		return SyncRequest
	case len(sm.Read) > 0:
		return SyncRead
	case len(sm.Viewed) > 0:
		return SyncViewed
	case sm.ViewOnceOpen != nil:
		return SyncViewOnceOpen
	case sm.Verified != nil:
		return SyncVerified
	case len(sm.StickerPackOperation) > 0:
		return SyncStickerPackOperation
	case sm.Blocked != nil:
		return SyncBlocked
	case sm.Configuration != nil:
		return SyncConfiguration
	case sm.FetchLatest != nil && sm.FetchLatest.Type != nil:
		return SyncFetchLatest
	case sm.MessageRequestResponse != nil && sm.MessageRequestResponse.Type != nil:
		return SyncMessageRequestResponse
	case sm.Groups != nil:
		return SyncGroups
	case sm.OutgoingPayment != nil:
		return SyncOutgoingPayment
	case sm.Keys != nil:
		return SyncKeys
	case sm.Contacts != nil:
		return SyncContacts
	case sm.PniChangeNumber != nil:
		return SyncPniChangeNumber
	case sm.DeviceNameChange != nil:
		return SyncDeviceNameChange
	case sm.CallEvent != nil:
		return SyncCallEvent
	}
	return SyncEmpty
}

// validServiceID: a binary or string service ID that parses (binary
// preferred).
func validServiceID(bin []byte, str string) bool {
	if len(bin) > 0 {
		if _, err := protocol.ServiceIDFromBinary(bin); err == nil {
			return true
		}
	}
	_, ok := protocol.ServiceIDToBinary(str)
	return ok
}

// validSync is §2.1's validation: failing content is dropped silently.
func validSync(sm *signalpb.AppSyncMessage, env *signalpb.ServiceEnvelope, sender, ownACI string) bool {
	if sender != ownACI {
		return false
	}
	if s := sm.Sent; s != nil {
		if s.Timestamp == nil {
			return false
		}
		dm, edit := s.GetMessage(), s.GetEditMessage().GetDataMessage()
		manifest := s.GetStoryMessageRecipients()
		if dm == nil && s.EditMessage == nil && s.StoryMessage == nil && len(manifest) == 0 {
			return false
		}
		hasDest := validServiceID(s.GetDestinationServiceIdBinary(), s.GetDestinationServiceId())
		if !hasDest && dm.GetGroupV2() == nil && edit.GetGroupV2() == nil && s.GetStoryMessage().GetGroup() == nil &&
			dm.GetDelete() == nil && len(manifest) == 0 {
			return false
		}
		// Story manifests (stories.md §5.3).
		for _, r := range manifest {
			hasID := len(r.GetDestinationServiceIdBinary()) > 0 || r.GetDestinationServiceId() != ""
			if hasID && (!validServiceID(r.GetDestinationServiceIdBinary(), r.GetDestinationServiceId()) || r.IsAllowedToReply == nil) {
				return false
			}
		}
		if len(manifest) > 0 && s.StoryMessage == nil && !s.GetIsRecipientUpdate() {
			return false
		}
		if s.StoryMessage != nil && !validStory(s.StoryMessage) {
			return false
		}
		for _, u := range s.GetUnidentifiedStatus() {
			if !validServiceID(u.GetDestinationServiceIdBinary(), u.GetDestinationServiceId()) {
				return false
			}
		}
		for _, d := range []*signalpb.AppDataMessage{dm, edit} {
			if d != nil && (d.Timestamp == nil || d.GetTimestamp() != env.GetClientTimestamp() || !validData(d)) {
				return false
			}
		}
	}
	for _, r := range sm.Read {
		if !validACI(r.GetSenderAciBinary(), r.GetSenderAci()) || r.Timestamp == nil {
			return false
		}
	}
	for _, r := range sm.Viewed {
		if !validACI(r.GetSenderAciBinary(), r.GetSenderAci()) {
			return false
		}
	}
	if v := sm.ViewOnceOpen; v != nil && !validACI(v.GetSenderAciBinary(), v.GetSenderAci()) {
		return false
	}
	if v := sm.Verified; v != nil && !validACI(v.GetDestinationAciBinary(), v.GetDestinationAci()) {
		return false
	}
	for _, op := range sm.StickerPackOperation {
		if op.PackId == nil {
			return false
		}
	}
	if b := sm.Blocked; b != nil {
		for _, a := range b.GetAcis() {
			if !validACI(nil, a) {
				return false
			}
		}
		for _, a := range b.GetAcisBinary() {
			if !validACI(a, "") {
				return false
			}
		}
	}
	if m := sm.MessageRequestResponse; m != nil && m.GroupId == nil && !validACI(m.GetThreadAciBinary(), m.GetThreadAci()) {
		return false
	}
	return true
}

// recipientByNumber finds the ACI we know for a phone number.
func recipientByNumber(a *store.Account, number string) (string, bool) {
	for aci, rc := range a.Recipients {
		if rc.Number == number {
			return aci, true
		}
	}
	return "", false
}

func aciOfBinary(bin []byte, str string) string {
	if len(bin) == 16 {
		if s, err := protocol.ServiceIDFromBinary(bin); err == nil {
			return s
		}
	}
	return str
}

// applySync carries out a received SyncMessage's effects (§1, §6). Every
// sync message turns the multi-device flag on.
func (r *Receiver) applySync(ctx context.Context, sm *signalpb.AppSyncMessage) {
	a := r.Acct
	if !a.Data.MultiDevice {
		a.Data.MultiDevice = true
		if err := a.Save(); err != nil {
			r.logf("Storing the multi-device flag failed: %v", err)
		}
	}
	switch SyncMemberOf(sm) {
	case SyncSent:
		r.applySent(sm.Sent)
	case SyncRequest:
		if a.Data.DeviceID == 1 {
			if r.pending.syncAnswers == nil {
				r.pending.syncAnswers = map[signalpb.AppSyncMessage_Request_Type]bool{}
			}
			r.pending.syncAnswers[sm.Request.GetType()] = true
			r.pending.storage = true
		}
	case SyncBlocked:
		// Only numbers and string ACIs count; blocks are only ever added.
		changed := false
		block := func(aci string) {
			if rc := a.Recipients[aci]; !rc.Blocked {
				rc.Blocked = true
				a.Recipients[aci] = rc
				changed = true
			}
		}
		for _, n := range sm.Blocked.GetNumbers() {
			if aci, ok := recipientByNumber(a, n); ok {
				block(aci)
			}
		}
		for _, aci := range sm.Blocked.GetAcis() {
			block(aci)
		}
		if changed {
			a.SaveRecipients()
		}
	case SyncVerified:
		v := sm.Verified
		aci := aciOfBinary(v.GetDestinationAciBinary(), v.GetDestinationAci())
		id, ok := a.Identities[aci]
		if !ok || !bytes.Equal(id.Key, v.GetIdentityKey()) {
			return
		}
		switch v.GetState() {
		case signalpb.AppVerified_VERIFIED:
			id.Trust = store.TrustedVerified
		case signalpb.AppVerified_DEFAULT:
			id.Trust = store.TrustedUnverified
		case signalpb.AppVerified_UNVERIFIED:
			id.Trust = store.Untrusted
		default:
			return
		}
		a.Identities[aci] = id
		a.SaveIdentities()
	case SyncConfiguration:
		c, cfg := sm.Configuration, &a.Data.Configuration
		if c.ReadReceipts != nil {
			cfg.ReadReceipts = proto.Bool(c.GetReadReceipts())
		}
		if c.UnidentifiedDeliveryIndicators != nil {
			cfg.UnidentifiedDeliveryIndicators = proto.Bool(c.GetUnidentifiedDeliveryIndicators())
		}
		if c.TypingIndicators != nil {
			cfg.TypingIndicators = proto.Bool(c.GetTypingIndicators())
		}
		if c.LinkPreviews != nil {
			cfg.LinkPreviews = proto.Bool(c.GetLinkPreviews())
		}
		a.Save()
	case SyncKeys:
		if k := sm.Keys; k.AccountEntropyPool != nil || k.MediaRootBackupKey != nil {
			if k.AccountEntropyPool != nil {
				a.Data.AccountEntropyPool = k.GetAccountEntropyPool()
			}
			if k.MediaRootBackupKey != nil {
				a.Data.MediaRootBackupKey = k.GetMediaRootBackupKey()
			}
			a.Save()
			r.pending.storage = true
		}
	case SyncFetchLatest:
		if sm.FetchLatest.GetType() == signalpb.AppSyncMessage_FetchLatest_LOCAL_PROFILE {
			r.pending.ownProfile = true
		}
		if sm.FetchLatest.GetType() == signalpb.AppSyncMessage_FetchLatest_STORAGE_MANIFEST {
			r.pending.storage = true
		}
	case SyncContacts:
		r.importContacts(ctx, sm.Contacts)
	case SyncStickerPackOperation:
		r.applyStickerOperations(sm.StickerPackOperation)
	}
}

// applySent handles a transcript's DataMessage as our own outgoing
// message (§6.1); downloads happen with the incoming ones.
func (r *Receiver) applySent(s *signalpb.AppSent) {
	if pk := s.GetStoryMessage().GetProfileKey(); len(pk) == 32 && !bytes.Equal(pk, r.Acct.Data.ProfileKey) {
		r.Acct.Data.ProfileKey = pk // a story transcript's key is our own (stories.md §5.3)
		r.Acct.Save()
	}
	dm := s.GetMessage()
	if dm == nil {
		return
	}
	a := r.Acct
	if dest := aciOfBinary(s.GetDestinationServiceIdBinary(), s.GetDestinationServiceId()); dest != "" && dm.GroupV2 == nil &&
		(dm.Body != nil || dm.GetFlags()&2 != 0) {
		rc := a.Recipients[dest]
		rc.Timer = dm.GetExpireTimer()
		if v := dm.GetExpireTimerVersion(); v > 0 {
			rc.TimerVersion = v
		}
		a.Recipients[dest] = rc
		a.SaveRecipients()
	}
	if pk := dm.GetProfileKey(); len(pk) == 32 && !bytes.Equal(pk, a.Data.ProfileKey) {
		a.Data.ProfileKey = pk
		a.Save()
	}
}

// importContacts downloads and applies a contacts sync (§5.1).
func (r *Receiver) importContacts(ctx context.Context, c *signalpb.AppSyncMessage_Contacts) {
	st := r.Attachments
	if st == nil || st.CDN == nil || c.GetBlob() == nil {
		return
	}
	data, err := st.download(ctx, c.GetBlob())
	if err != nil {
		r.logf("Failed to handle received sync contacts, ignoring: %v", err)
		return
	}
	records, err := DecodeContacts(data)
	if err != nil {
		r.logf("Failed to handle received sync contacts, ignoring: %v", err)
	}
	a := r.Acct
	for _, rec := range records {
		d := rec.Details
		aci := aciOfBinary(d.GetAciBinary(), d.GetAci())
		if aci == "" && d.Number != nil {
			aci, _ = recipientByNumber(a, d.GetNumber())
		}
		if aci == "" {
			continue // no ACI to file it under
		}
		rc, existed := a.Recipients[aci]
		if d.Number != nil {
			rc.Number = d.GetNumber()
		}
		if rc.GivenName == nil && rc.FamilyName == nil && d.Name != nil {
			name := d.GetName()
			rc.GivenName = &name
		}
		if t, v := d.GetExpireTimer(), d.GetExpireTimerVersion(); t > 0 && v > 0 && (!existed || v > rc.TimerVersion) {
			rc.Timer, rc.TimerVersion = t, v
		}
		a.Recipients[aci] = rc
		if rec.Avatar != nil && !st.IgnoreAvatars && st.AvatarsDir != "" {
			id := rc.Number
			if id == "" {
				id = aci
			}
			if err := os.MkdirAll(st.AvatarsDir, 0o700); err == nil {
				os.WriteFile(filepath.Join(st.AvatarsDir, "contact-"+id), rec.Avatar, 0o644)
			}
		}
	}
	a.SaveRecipients()
}

// Sending sync messages (§6).

// SendSyncRequests sends the five requests of §6.2 from a linked device:
// GROUPS, CONTACTS, BLOCKED, CONFIGURATION, KEYS, each its own message,
// urgent only for CONTACTS and KEYS. A primary sends nothing.
func (s *Sender) SendSyncRequests(ctx context.Context) error {
	if s.Acct.Data.DeviceID == 1 {
		return nil
	}
	var first error
	for _, t := range []signalpb.AppSyncMessage_Request_Type{
		signalpb.AppSyncMessage_Request_GROUPS, signalpb.AppSyncMessage_Request_CONTACTS, signalpb.AppSyncMessage_Request_BLOCKED,
		signalpb.AppSyncMessage_Request_CONFIGURATION, signalpb.AppSyncMessage_Request_KEYS,
	} {
		urgent := t == signalpb.AppSyncMessage_Request_CONTACTS || t == signalpb.AppSyncMessage_Request_KEYS
		if err := s.SendSync(ctx, &signalpb.AppSyncMessage{Request: &signalpb.AppSyncMessage_Request{Type: t.Enum()}}, urgent); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// isContact is §5.1's contact rule: a contact (contacts.md §1) that is not
// hidden.
func isContact(rc store.Recipient) bool { return rc.IsContact() && !rc.Hidden }

func contactName(rc store.Recipient) string { return rc.ContactName() }

// SendContacts builds and sends the contact stream (§5.1, §8.6). The
// upload form is requested even on a single-device account; nothing is
// uploaded or sent there.
func (s *Sender) SendContacts(ctx context.Context) error {
	a := s.Acct
	acis := make([]string, 0, len(a.Recipients))
	for aci := range a.Recipients {
		acis = append(acis, aci)
	}
	sort.Strings(acis)
	var records []ContactRecord
	for _, aci := range acis {
		rc := a.Recipients[aci]
		if aci == a.Data.ACI || !isContact(rc) {
			continue
		}
		d := &signalpb.AppContactDetails{Name: proto.String(contactName(rc)), ExpireTimer: proto.Uint32(rc.Timer)}
		if rc.Number != "" {
			d.Number = proto.String(rc.Number)
		}
		if bin, ok := uuidRaw(aci); ok {
			d.AciBinary = bin[:]
		}
		records = append(records, ContactRecord{Details: d})
	}
	if a.Data.ProfileKey != nil {
		self := &signalpb.AppContactDetails{Number: proto.String(a.Data.Number)}
		if bin, ok := uuidRaw(a.Data.ACI); ok {
			self.AciBinary = bin[:]
		}
		if rc, ok := a.Recipients[a.Data.ACI]; ok && isContact(rc) {
			self.Name, self.ExpireTimer = proto.String(contactName(rc)), proto.Uint32(rc.Timer)
		}
		records = append(records, ContactRecord{Details: self})
	}
	data, err := EncodeContacts(records)
	if err != nil || len(data) == 0 {
		return err
	}
	if !a.IsMultiDevice() {
		_, blob, _, err := attachment.Encrypt(data)
		if err != nil {
			return err
		}
		_, err = s.uploadForm(ctx, len(blob))
		return err
	}
	ps, err := s.UploadAttachments(ctx, []attachment.Source{{Data: data, ContentType: "application/octet-stream"}}, 0)
	if err != nil {
		return err
	}
	return s.SendSync(ctx, &signalpb.AppSyncMessage{Contacts: &signalpb.AppSyncMessage_Contacts{Blob: ps[0], Complete: proto.Bool(true)}}, false)
}

// SendBlocked sends the blocked list (§6.4): numbers and binary ACIs of
// blocked contacts, filled independently.
func (s *Sender) SendBlocked(ctx context.Context) error {
	b := &signalpb.AppSyncMessage_Blocked{}
	acis := make([]string, 0)
	for aci, rc := range s.Acct.Recipients {
		if rc.Blocked {
			acis = append(acis, aci)
		}
	}
	sort.Strings(acis)
	for _, aci := range acis {
		if n := s.Acct.Recipients[aci].Number; n != "" {
			b.Numbers = append(b.Numbers, n)
		}
		if bin, ok := uuidRaw(aci); ok {
			b.AcisBinary = append(b.AcisBinary, bin[:])
		}
	}
	return s.SendSync(ctx, &signalpb.AppSyncMessage{Blocked: b}, false)
}

// SendConfiguration sends the settings that have a value (§6.6).
func (s *Sender) SendConfiguration(ctx context.Context) error {
	cfg := s.Acct.Data.Configuration
	return s.SendSync(ctx, &signalpb.AppSyncMessage{Configuration: &signalpb.AppSyncMessage_Configuration{
		ReadReceipts: cfg.ReadReceipts, UnidentifiedDeliveryIndicators: cfg.UnidentifiedDeliveryIndicators,
		TypingIndicators: cfg.TypingIndicators, LinkPreviews: cfg.LinkPreviews,
	}}, false)
}

// aepAlphabet: the account entropy pool is 64 characters; signal-cli-go
// draws them from lower-case letters and digits.
const aepAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// SendKeys sends the account entropy pool and media root backup key,
// creating and storing them first if absent (§6.7).
func (s *Sender) SendKeys(ctx context.Context) error {
	if err := s.ensureKeys(); err != nil {
		return err
	}
	a := s.Acct
	return s.SendSync(ctx, &signalpb.AppSyncMessage{Keys: &signalpb.AppSyncMessage_Keys{
		AccountEntropyPool: proto.String(a.Data.AccountEntropyPool), MediaRootBackupKey: a.Data.MediaRootBackupKey}}, false)
}

// ensureKeys creates and stores the account entropy pool and media root
// backup key if absent.
func (s *Sender) ensureKeys() error {
	a := s.Acct
	changed := false
	if a.Data.AccountEntropyPool == "" {
		pool := make([]byte, 0, 64)
		var b [1]byte
		for len(pool) < 64 {
			if _, err := io.ReadFull(s.rand(), b[:]); err != nil {
				return err
			}
			if int(b[0]) < 252 { // 252 = 7*36: no modulo bias
				pool = append(pool, aepAlphabet[int(b[0])%36])
			}
		}
		a.Data.AccountEntropyPool, changed = string(pool), true
	}
	if a.Data.MediaRootBackupKey == nil {
		k := make([]byte, 32)
		if _, err := io.ReadFull(s.rand(), k); err != nil {
			return err
		}
		a.Data.MediaRootBackupKey, changed = k, true
	}
	if changed {
		return a.Save()
	}
	return nil
}

// SendReadSync sends a read or viewed sync for messages from sender
// (§6.8): read is urgent, viewed is not.
func (s *Sender) SendReadSync(ctx context.Context, sender string, viewed bool, timestamps []uint64) error {
	bin, ok := uuidRaw(sender)
	if !ok {
		return nil
	}
	var entries []*signalpb.AppSyncMessage_Read
	for _, ts := range timestamps {
		entries = append(entries, &signalpb.AppSyncMessage_Read{Timestamp: proto.Uint64(ts), SenderAciBinary: bin[:]})
	}
	if viewed {
		return s.SendSync(ctx, &signalpb.AppSyncMessage{Viewed: entries}, false)
	}
	return s.SendSync(ctx, &signalpb.AppSyncMessage{Read: entries}, true)
}

// answerSyncRequests is §6.2.1, run after the queue is drained.
func (r *Receiver) answerSyncRequests(ctx context.Context, types map[signalpb.AppSyncMessage_Request_Type]bool) {
	for _, t := range []signalpb.AppSyncMessage_Request_Type{
		signalpb.AppSyncMessage_Request_CONTACTS, signalpb.AppSyncMessage_Request_BLOCKED,
		signalpb.AppSyncMessage_Request_CONFIGURATION, signalpb.AppSyncMessage_Request_KEYS,
	} {
		if !types[t] {
			continue
		}
		var err error
		switch t {
		case signalpb.AppSyncMessage_Request_CONTACTS:
			err = r.Sender.SendContacts(ctx)
		case signalpb.AppSyncMessage_Request_BLOCKED:
			err = r.Sender.SendBlocked(ctx)
		case signalpb.AppSyncMessage_Request_CONFIGURATION:
			err = r.Sender.SendConfiguration(ctx)
		case signalpb.AppSyncMessage_Request_KEYS:
			err = r.Sender.SendKeys(ctx)
		}
		if err != nil {
			r.logf("Message action failed. %v", err)
		}
	}
}
