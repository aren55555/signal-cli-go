package manager

import (
	"strings"
	"unicode/utf16"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Receive-side rules for data-message features
// (spec/service/content-features.md §4.3-4.4, §7).

// validACI: a binary ACI of 16 non-zero bytes, or a string UUID.
func validACI(bin []byte, str string) bool {
	if bin != nil {
		if len(bin) != 16 {
			return false
		}
		for _, b := range bin {
			if b != 0 {
				return true
			}
		}
		return false
	}
	_, ok := uuidRaw(str)
	return ok && strings.Trim(strings.ReplaceAll(str, "-", ""), "0") != ""
}

func utf16Len(s string) uint32 { return uint32(len(utf16.Encode([]rune(s)))) }

// validRanges checks body ranges against their text (§7.1).
func validRanges(rs []*signalpb.AppBodyRange, text string, allowOutOfBounds bool) bool {
	n := utf16Len(text)
	for _, r := range rs {
		switch v := r.AssociatedValue.(type) {
		case *signalpb.AppBodyRange_MentionAciBinary:
			if !validACI(v.MentionAciBinary, "") {
				return false
			}
		case *signalpb.AppBodyRange_MentionAci:
			if !validACI(nil, v.MentionAci) {
				return false
			}
		case *signalpb.AppBodyRange_Style_:
			if r.Start == nil || r.Length == nil {
				return false
			}
		}
		if !allowOutOfBounds && uint64(r.GetStart())+uint64(r.GetLength()) > uint64(n) {
			return false
		}
	}
	return true
}

func hasLongText(dm *signalpb.AppDataMessage) bool {
	// Attachment pointers are not decoded yet; any attachment may be the
	// long-text one, so out-of-bounds ranges are allowed with attachments.
	return len(dm.GetAttachments()) > 0
}

// validData is §7.1's stage V for one DataMessage.
func validData(dm *signalpb.AppDataMessage) bool {
	if dm.GetRequiredProtocolVersion() > currentProtocolVersion {
		return false
	}
	if q := dm.GetQuote(); q != nil {
		if !validACI(q.GetAuthorAciBinary(), q.GetAuthorAci()) || !validRanges(q.GetBodyRanges(), q.GetText(), false) {
			return false
		}
	}
	if !validRanges(dm.GetBodyRanges(), dm.GetBody(), hasLongText(dm)) {
		return false
	}
	if r := dm.GetReaction(); r != nil && (r.TargetSentTimestamp == nil || !validACI(r.GetTargetAuthorAciBinary(), r.GetTargetAuthorAci())) {
		return false
	}
	if d := dm.GetDelete(); d != nil && d.TargetSentTimestamp == nil {
		return false
	}
	if sc := dm.GetStoryContext(); sc != nil && (sc.SentTimestamp == nil || !validACI(sc.GetAuthorAciBinary(), sc.GetAuthorAci())) {
		return false
	}
	if g := dm.GetGiftBadge(); g != nil && len(g.GetReceiptCredentialPresentation()) == 0 {
		return false
	}
	if pc := dm.GetPollCreate(); pc != nil {
		q := strings.TrimSpace(pc.GetQuestion())
		if pc.Question == nil || q == "" || utf16Len(pc.GetQuestion()) > 200 || len(pc.GetOptions()) < 2 || pc.AllowMultiple == nil {
			return false
		}
		for _, o := range pc.GetOptions() {
			if utf16Len(o) > 100 {
				return false
			}
		}
	}
	if pt := dm.GetPollTerminate(); pt != nil && pt.TargetSentTimestamp == nil {
		return false
	}
	if pv := dm.GetPollVote(); pv != nil && (!validACI(pv.GetTargetAuthorAciBinary(), "") || pv.TargetSentTimestamp == nil || pv.VoteCount == nil || len(pv.GetOptionIndexes()) > 10) {
		return false
	}
	if pm := dm.GetPinMessage(); pm != nil {
		_, secs := pm.PinDuration.(*signalpb.AppPinMessage_PinDurationSeconds)
		if !validACI(pm.GetTargetAuthorAciBinary(), "") || pm.TargetSentTimestamp == nil || !(secs || pm.GetPinDurationForever()) {
			return false
		}
	}
	if um := dm.GetUnpinMessage(); um != nil && (!validACI(um.GetTargetAuthorAciBinary(), "") || um.TargetSentTimestamp == nil) {
		return false
	}
	if ad := dm.GetAdminDelete(); ad != nil && (!validACI(ad.GetTargetAuthorAciBinary(), "") || ad.TargetSentTimestamp == nil) {
		return false
	}
	return true
}

// valid applies the receive-side checks that make signal-cli drop content
// silently (pipe.md §7 step 4, content.md §7, content-features.md §7.1).
func valid(c *signalpb.AppContent, env *signalpb.ServiceEnvelope) bool {
	if c.Body == nil && c.SenderKeyDistributionMessage == nil && c.PniSignatureMessage == nil {
		return false
	}
	if sig := c.GetPniSignatureMessage(); sig != nil {
		p := sig.GetPni()
		if !(len(p) == 16 || (len(p) == 17 && p[0] == 0x01)) || sig.Signature == nil {
			return false
		}
	}
	if dm := c.GetDataMessage(); dm != nil {
		if dm.Timestamp == nil || dm.GetTimestamp() != env.GetClientTimestamp() || !validData(dm) {
			return false
		}
	}
	if ed := c.GetEditMessage(); ed != nil {
		inner := ed.GetDataMessage()
		if inner == nil || ed.TargetSentTimestamp == nil || inner.Timestamp == nil || inner.GetTimestamp() != env.GetClientTimestamp() || !validData(inner) {
			return false
		}
	}
	return true
}

// applyPNI updates the needs-PNI-signature flag from the envelope's
// destination and verifies an incoming PNI signature (§4.3-4.4).
func applyPNI(a *store.Account, m *Message, c *signalpb.AppContent) {
	sender := m.Sender.ServiceID
	if sender == "" || strings.HasPrefix(sender, "PNI:") {
		return
	}
	if c.GetNullMessage() != nil && c.GetDataMessage() == nil {
		return // a bare null message changes nothing (§5)
	}
	rc := a.Recipients[sender]
	changed := rc.NeedsPNISignature != m.ToPNI
	rc.NeedsPNISignature = m.ToPNI
	if sig := c.GetPniSignatureMessage(); sig != nil {
		raw := sig.GetPni()
		if len(raw) == 17 {
			raw = raw[1:]
		}
		pni := formatUUIDBytes(raw)
		aciKey, ok1 := a.Identities[sender]
		pniKey, ok2 := a.Identities["PNI:"+pni]
		if ok1 && ok2 && pni != sender && verifyPNISignature(aciKey.Key, pniKey.Key, sig.GetSignature()) {
			rc.PNI, rc.PNIVerified = pni, true
			changed = true
		}
	}
	if changed {
		a.Recipients[sender] = rc
		a.SaveRecipients()
	}
}

func formatUUIDBytes(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	const hexd = "0123456789abcdef"
	var out []byte
	for i, x := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hexd[x>>4], hexd[x&15])
	}
	return string(out)
}

// applyTimer is §7.2's disappearing-timer rule for a 1:1 partner.
func applyTimer(a *store.Account, partner string, dm *signalpb.AppDataMessage) {
	if dm.GetFlags()&2 == 0 && dm.GetBody() == "" {
		return
	}
	timer := dm.GetExpireTimer()
	version := dm.GetExpireTimerVersion()
	if dm.ExpireTimerVersion == nil {
		version = 1
	}
	rc := a.Recipients[partner]
	if rc.TimerVersion != 0 && (rc.Timer == timer || rc.TimerVersion >= version) {
		return
	}
	rc.Timer, rc.TimerVersion = timer, version
	a.Recipients[partner] = rc
	a.SaveRecipients()
}
