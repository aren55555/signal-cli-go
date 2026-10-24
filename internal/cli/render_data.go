package cli

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
)

// The DATA block and the DataMessage JSON object
// (spec/surface/cli/_received-message-output.md).

// aciOf reads an author from its binary or string form; the binary form
// wins (content-features.md §1.2).
func aciOf(bin []byte, str string) string {
	if len(bin) == 16 {
		if s, err := protocol.ServiceIDFromBinary(bin); err == nil {
			return s
		}
	}
	return str
}

func (c *RenderContext) contactOf(aci string) string { return c.address(aci, 0).contact() }

func (c *RenderContext) plainData(p *plain, l int, dm *signalpb.AppDataMessage) {
	dm = c.withLongText(dm)
	p.line(l, "Message timestamp: %s", ts(dm.GetTimestamp()))
	if dm.GetIsViewOnce() {
		p.line(l, "=VIEW ONCE=")
	}
	if dm.Body != nil {
		p.line(l, "Body: %s", dm.GetBody())
	}
	if sc := dm.GetStoryContext(); sc != nil {
		p.line(l, "Story reply:")
		p.line(l+1, "Sender: %s", c.contactOf(aciOf(sc.GetAuthorAciBinary(), sc.GetAuthorAci())))
		p.line(l+1, "Sent timestamp: %d", sc.GetSentTimestamp())
	}
	if g := dm.GetGroupCallUpdate(); g != nil {
		p.line(l, "Group call update:")
		p.line(l+1, "Era id: %s", g.GetEraId())
	}
	if len(dm.GetPreview()) > 0 {
		p.line(l, "Previews:")
		for _, pv := range dm.GetPreview() {
			p.line(l, "- Preview")
			c.plainPreview(p, l+1, pv)
		}
	}
	if len(dm.GetContact()) > 0 {
		p.line(l, "Contacts:")
		for _, sc := range dm.GetContact() {
			p.line(l, "- Contact:")
			c.plainContact(p, l+1, sc)
		}
	}
	if st := dm.GetSticker(); st != nil {
		p.line(l, "Sticker:")
		p.line(l+1, "Pack id: %s", hex.EncodeToString(st.GetPackId()))
		p.line(l+1, "Sticker id: %d", st.GetStickerId())
	}
	if dm.GetFlags()&flagExpirationUpdate != 0 {
		p.line(l, "Is Expiration update: true")
	}
	if dm.GetExpireTimer() > 0 {
		p.line(l, "Expires in: %d seconds", dm.GetExpireTimer())
	}
	if dm.GetFlags()&flagProfileKeyUpdate != 0 {
		p.line(l, "Profile key update")
	}
	if len(dm.GetProfileKey()) > 0 {
		p.line(l, "With profile key")
	}
	if r := dm.GetReaction(); r != nil {
		p.line(l, "Reaction:")
		p.line(l+1, "Emoji: %s", r.GetEmoji())
		p.line(l+1, "Target author: %s", c.contactOf(aciOf(r.GetTargetAuthorAciBinary(), r.GetTargetAuthorAci())))
		p.line(l+1, "Target timestamp: %s", ts(r.GetTargetSentTimestamp()))
		p.line(l+1, "Is remove: %t", r.GetRemove())
	}
	if q := dm.GetQuote(); q != nil {
		p.line(l, "Quote:")
		p.line(l+1, "Id: %d", q.GetId())
		p.line(l+1, "Author: %s", c.contactOf(aciOf(q.GetAuthorAciBinary(), q.GetAuthorAci())))
		if q.Text != nil {
			p.line(l+1, "Text: %s", q.GetText())
		}
		if ms := mentions(q.GetBodyRanges()); len(ms) > 0 && q.Text != nil {
			p.line(l+1, "Mentions:")
			for _, m := range ms {
				c.plainMention(p, l+1, m)
			}
		}
		if len(q.GetAttachments()) > 0 {
			p.line(l+1, "Attachments:")
			for _, a := range q.GetAttachments() {
				p.line(l+1, "- Attachment:")
				c.plainQuotedAttachment(p, l+2, a)
			}
		}
	}
	if d := dm.GetDelete(); d != nil {
		p.line(l, "Remote delete message: timestamp = %d", d.GetTargetSentTimestamp())
	}
	if dm.Body != nil {
		if ms := mentions(dm.GetBodyRanges()); len(ms) > 0 {
			p.line(l, "Mentions:")
			for _, m := range ms {
				c.plainMention(p, l, m)
			}
		}
		if ss := styles(dm.GetBodyRanges()); len(ss) > 0 {
			p.line(l, "Text styles:")
			for _, s := range ss {
				p.line(l, "- %s: %d (length: %d)", s.GetStyle().String(), s.GetStart(), s.GetLength())
			}
		}
	}
	if len(dm.GetAttachments()) > 0 {
		p.line(l, "Attachments:")
		for _, a := range dm.GetAttachments() {
			p.line(l, "- Attachment:")
			c.plainAttachment(p, l+1, a)
		}
	}
	if pc := dm.GetPollCreate(); pc != nil {
		kind := "single"
		if pc.GetAllowMultiple() {
			kind = "multi"
		}
		p.line(l, "Poll Create: %q (%s)", pc.GetQuestion(), kind)
		for _, o := range pc.GetOptions() {
			p.line(l, "- %s", o)
		}
	}
	if pv := dm.GetPollVote(); pv != nil {
		idx := make([]string, len(pv.GetOptionIndexes()))
		for i, x := range pv.GetOptionIndexes() {
			idx[i] = strconv.Itoa(int(x))
		}
		p.line(l, "Poll Vote: \"%s\" (%s) selected %s (vote #%d)", c.contactOf(aciOf(pv.GetTargetAuthorAciBinary(), "")),
			ts(pv.GetTargetSentTimestamp()), strings.Join(idx, ","), pv.GetVoteCount())
	}
	if pt := dm.GetPollTerminate(); pt != nil {
		p.line(l, "Poll Terminate: %s", ts(pt.GetTargetSentTimestamp()))
	}
	if pm := dm.GetPinMessage(); pm != nil {
		p.line(l, "Pin Message:")
		p.line(l+1, "Target author: %s", c.contactOf(aciOf(pm.GetTargetAuthorAciBinary(), "")))
		p.line(l+1, "Target timestamp: %s", ts(pm.GetTargetSentTimestamp()))
		switch d := pinDuration(pm); d {
		case -1:
			p.line(l+1, "Duration: forever")
		case 0:
			p.line(l+1, "Duration: unspecified")
		default:
			p.line(l+1, "Duration: %d seconds", d)
		}
	}
	if um := dm.GetUnpinMessage(); um != nil {
		p.line(l, "Unpin Message:")
		p.line(l+1, "Target author: %s", c.contactOf(aciOf(um.GetTargetAuthorAciBinary(), "")))
		p.line(l+1, "Target timestamp: %s", ts(um.GetTargetSentTimestamp()))
	}
	if ad := dm.GetAdminDelete(); ad != nil {
		p.line(l, "Admin Delete:")
		p.line(l+1, "Target author: %s", c.contactOf(aciOf(ad.GetTargetAuthorAciBinary(), "")))
		p.line(l+1, "Target timestamp: %s", ts(ad.GetTargetSentTimestamp()))
	}
}

// pinDuration is the reported duration: -1 forever, 0 unspecified.
func pinDuration(pm *signalpb.AppPinMessage) int64 {
	switch {
	case pm.GetPinDurationForever():
		return -1
	case pm.PinDuration != nil:
		return int64(pm.GetPinDurationSeconds())
	}
	return 0
}

func mentions(rs []*signalpb.AppBodyRange) []*signalpb.AppBodyRange {
	var out []*signalpb.AppBodyRange
	for _, r := range rs {
		if aciOf(r.GetMentionAciBinary(), r.GetMentionAci()) != "" {
			out = append(out, r)
		}
	}
	return out
}

func styles(rs []*signalpb.AppBodyRange) []*signalpb.AppBodyRange {
	var out []*signalpb.AppBodyRange
	for _, r := range rs {
		if _, ok := r.AssociatedValue.(*signalpb.AppBodyRange_Style_); ok {
			out = append(out, r)
		}
	}
	return out
}

func (c *RenderContext) plainMention(p *plain, l int, m *signalpb.AppBodyRange) {
	p.line(l, "- %s: %d (length: %d)", c.contactOf(aciOf(m.GetMentionAciBinary(), m.GetMentionAci())), m.GetStart(), m.GetLength())
}

func (c *RenderContext) plainContact(p *plain, l int, sc *signalpb.AppSharedContact) {
	p.line(l, "Name:")
	n := sc.GetName()
	for _, f := range [][2]string{{"First name", n.GetGivenName()}, {"Middle name", n.GetMiddleName()}, {"Family name", n.GetFamilyName()},
		{"Prefix name", n.GetPrefix()}, {"Suffix name", n.GetSuffix()}, {"Display name", n.GetNickname()}} {
		if strings.TrimSpace(f[1]) != "" {
			p.line(l+1, "%s: %s", f[0], f[1])
		}
	}
	if av := sc.GetAvatar(); av != nil {
		p.line(l, "Avatar: (profile: %t)", av.GetIsProfile())
		if av.Avatar != nil {
			c.plainAttachment(p, l+1, av.Avatar)
		}
	}
	if sc.Organization != nil {
		p.line(l, "Organisation: %s", sc.GetOrganization())
	}
	phoneType := func(t signalpb.AppSharedContact_Phone_Type) string {
		if t == 0 {
			t = signalpb.AppSharedContact_Phone_HOME
		}
		return t.String()
	}
	if len(sc.GetNumber()) > 0 {
		p.line(l, "Phone details:")
		for _, ph := range sc.GetNumber() {
			p.line(l, "- Phone:")
			p.line(l+1, "Number: %s", ph.GetValue())
			p.line(l+1, "Type: %s", phoneType(ph.GetType()))
			if strings.TrimSpace(ph.GetLabel()) != "" {
				p.line(l+1, "Label: %s", ph.GetLabel())
			}
		}
	}
	if len(sc.GetEmail()) > 0 {
		p.line(l, "Email details:")
		for _, em := range sc.GetEmail() {
			p.line(l, "- Email:")
			p.line(l+1, "Address: %s", em.GetValue())
			p.line(l+1, "Type: %s", phoneType(em.GetType()))
			if strings.TrimSpace(em.GetLabel()) != "" {
				p.line(l+1, "Label: %s", em.GetLabel())
			}
		}
	}
	if len(sc.GetAddress()) > 0 {
		p.line(l, "Address details:")
		for _, ad := range sc.GetAddress() {
			p.line(l, "- Address:")
			t := ad.GetType()
			if t == 0 {
				t = signalpb.AppSharedContact_PostalAddress_HOME
			}
			p.line(l+1, "Type: %s", t.String())
			for _, f := range [][2]string{{"Label", ad.GetLabel()}, {"Street", ad.GetStreet()}, {"Pobox", ad.GetPobox()}, {"Neighbourhood", ad.GetNeighborhood()},
				{"City", ad.GetCity()}, {"Region", ad.GetRegion()}, {"Postcode", ad.GetPostcode()}, {"Country", ad.GetCountry()}} {
				if strings.TrimSpace(f[1]) != "" {
					p.line(l+1, "%s: %s", f[0], f[1])
				}
			}
		}
	}
}

func (c *RenderContext) jsonAuthor(o *jsonObject, prefix, aci string, deprecated string) {
	a := c.address(aci, 0)
	if deprecated != "" {
		o.add(deprecated, a.ident())
	}
	o.add(prefix+"Number", strOrNull(a.number))
	o.add(prefix+"Uuid", strOrNull(aci))
}

func (c *RenderContext) jsonMentions(rs []*signalpb.AppBodyRange) rawJSON {
	var items []string
	for _, m := range mentions(rs) {
		aci := aciOf(m.GetMentionAciBinary(), m.GetMentionAci())
		a := c.address(aci, 0)
		o := &jsonObject{}
		o.add("name", a.ident())
		o.add("number", strOrNull(a.number))
		o.add("uuid", aci)
		o.add("start", m.GetStart())
		o.add("length", m.GetLength())
		items = append(items, o.String())
	}
	return rawJSON("[" + strings.Join(items, ",") + "]")
}

func jsonStyles(rs []*signalpb.AppBodyRange) rawJSON {
	var items []string
	for _, s := range styles(rs) {
		o := &jsonObject{}
		o.add("style", s.GetStyle().String())
		o.add("start", s.GetStart())
		o.add("length", s.GetLength())
		items = append(items, o.String())
	}
	return rawJSON("[" + strings.Join(items, ",") + "]")
}

func blankNull(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (c *RenderContext) jsonData(dm *signalpb.AppDataMessage) *jsonObject {
	dm = c.withLongText(dm)
	o := &jsonObject{}
	o.add("timestamp", dm.GetTimestamp())
	if dm.Body != nil {
		o.add("message", dm.GetBody())
	} else {
		o.add("message", nil)
	}
	o.add("expiresInSeconds", dm.GetExpireTimer())
	o.add("isExpirationUpdate", dm.GetFlags()&flagExpirationUpdate != 0)
	o.add("viewOnce", dm.GetIsViewOnce())
	if r := dm.GetReaction(); r != nil {
		ro := &jsonObject{}
		ro.add("emoji", r.GetEmoji())
		c.jsonAuthor(ro, "targetAuthor", aciOf(r.GetTargetAuthorAciBinary(), r.GetTargetAuthorAci()), "targetAuthor")
		ro.add("targetSentTimestamp", r.GetTargetSentTimestamp())
		ro.add("isRemove", r.GetRemove())
		o.add("reaction", ro)
	}
	if q := dm.GetQuote(); q != nil {
		qo := &jsonObject{}
		qo.add("id", q.GetId())
		c.jsonAuthor(qo, "author", aciOf(q.GetAuthorAciBinary(), q.GetAuthorAci()), "author")
		if q.Text != nil {
			qo.add("text", q.GetText())
		} else {
			qo.add("text", nil)
		}
		if len(mentions(q.GetBodyRanges())) > 0 {
			qo.add("mentions", c.jsonMentions(q.GetBodyRanges()))
		}
		var atts []string
		for _, a := range q.GetAttachments() {
			ao := &jsonObject{}
			ao.add("contentType", a.GetContentType())
			ao.add("filename", strOrNull(a.GetFileName()))
			if a.Thumbnail != nil {
				ao.add("thumbnail", jsonAttachment(a.Thumbnail))
			}
			atts = append(atts, ao.String())
		}
		qo.add("attachments", rawJSON("["+strings.Join(atts, ",")+"]"))
		if len(styles(q.GetBodyRanges())) > 0 {
			qo.add("textStyles", jsonStyles(q.GetBodyRanges()))
		}
		o.add("quote", qo)
	}
	if pay := dm.GetPayment().GetNotification(); pay != nil {
		po := &jsonObject{}
		po.add("note", pay.GetNote())
		po.add("receipt", pay.GetMobileCoin().GetReceipt())
		o.add("payment", po)
	}
	if dm.Body != nil && len(mentions(dm.GetBodyRanges())) > 0 {
		o.add("mentions", c.jsonMentions(dm.GetBodyRanges()))
	}
	if len(dm.GetPreview()) > 0 {
		var items []string
		for _, pv := range dm.GetPreview() {
			items = append(items, jsonPreview(pv).String())
		}
		o.add("previews", rawJSON("["+strings.Join(items, ",")+"]"))
	}
	if len(dm.GetAttachments()) > 0 {
		o.add("attachments", jsonAttachments(dm.GetAttachments()))
	}
	if st := dm.GetSticker(); st != nil {
		so := &jsonObject{}
		so.add("packId", hex.EncodeToString(st.GetPackId()))
		so.add("stickerId", st.GetStickerId())
		o.add("sticker", so)
	}
	if d := dm.GetDelete(); d != nil {
		do := &jsonObject{}
		do.add("timestamp", d.GetTargetSentTimestamp())
		o.add("remoteDelete", do)
	}
	if len(dm.GetContact()) > 0 {
		var items []string
		for _, sc := range dm.GetContact() {
			items = append(items, jsonContact(sc).String())
		}
		o.add("contacts", rawJSON("["+strings.Join(items, ",")+"]"))
	}
	if pc := dm.GetPollCreate(); pc != nil {
		po := &jsonObject{}
		po.add("question", pc.GetQuestion())
		po.add("allowMultiple", pc.GetAllowMultiple())
		opts := pc.GetOptions()
		if opts == nil {
			opts = []string{}
		}
		po.add("options", opts)
		o.add("pollCreate", po)
	}
	if pv := dm.GetPollVote(); pv != nil {
		po := &jsonObject{}
		c.jsonAuthor(po, "author", aciOf(pv.GetTargetAuthorAciBinary(), ""), "author")
		po.add("targetSentTimestamp", pv.GetTargetSentTimestamp())
		idx := pv.GetOptionIndexes()
		if idx == nil {
			idx = []uint32{}
		}
		po.add("optionIndexes", idx)
		po.add("voteCount", pv.GetVoteCount())
		o.add("pollVote", po)
	}
	if pt := dm.GetPollTerminate(); pt != nil {
		po := &jsonObject{}
		po.add("targetSentTimestamp", pt.GetTargetSentTimestamp())
		o.add("pollTerminate", po)
	}
	if dm.Body != nil && len(styles(dm.GetBodyRanges())) > 0 {
		o.add("textStyles", jsonStyles(dm.GetBodyRanges()))
	}
	if sc := dm.GetStoryContext(); sc != nil {
		so := &jsonObject{}
		c.jsonAuthor(so, "author", aciOf(sc.GetAuthorAciBinary(), sc.GetAuthorAci()), "")
		so.add("sentTimestamp", sc.GetSentTimestamp())
		o.add("storyContext", so)
	}
	target := func(key string, bin []byte, t uint64, extra func(*jsonObject)) {
		to := &jsonObject{}
		c.jsonAuthor(to, "targetAuthor", aciOf(bin, ""), "targetAuthor")
		to.add("targetSentTimestamp", t)
		if extra != nil {
			extra(to)
		}
		o.add(key, to)
	}
	if pm := dm.GetPinMessage(); pm != nil {
		target("pinMessage", pm.GetTargetAuthorAciBinary(), pm.GetTargetSentTimestamp(), func(to *jsonObject) { to.add("pinDurationSeconds", pinDuration(pm)) })
	}
	if um := dm.GetUnpinMessage(); um != nil {
		target("unpinMessage", um.GetTargetAuthorAciBinary(), um.GetTargetSentTimestamp(), nil)
	}
	if ad := dm.GetAdminDelete(); ad != nil {
		target("adminDelete", ad.GetTargetAuthorAciBinary(), ad.GetTargetSentTimestamp(), nil)
	}
	return o
}

func jsonContact(sc *signalpb.AppSharedContact) *jsonObject {
	o := &jsonObject{}
	n := sc.GetName()
	no := &jsonObject{}
	no.add("nickname", blankNull(n.GetNickname()))
	no.add("given", blankNull(n.GetGivenName()))
	no.add("family", blankNull(n.GetFamilyName()))
	no.add("prefix", blankNull(n.GetPrefix()))
	no.add("suffix", blankNull(n.GetSuffix()))
	no.add("middle", blankNull(n.GetMiddleName()))
	o.add("name", no)
	if av := sc.GetAvatar(); av != nil && av.Avatar != nil {
		ao := &jsonObject{}
		ao.add("attachment", jsonAttachment(av.Avatar))
		ao.add("isProfile", av.GetIsProfile())
		o.add("avatar", ao)
	}
	phones := func(ps []*signalpb.AppSharedContact_Phone) rawJSON {
		var items []string
		for _, p := range ps {
			t := p.GetType()
			if t == 0 {
				t = signalpb.AppSharedContact_Phone_HOME
			}
			po := &jsonObject{}
			po.add("value", p.GetValue())
			po.add("type", t.String())
			po.add("label", blankNull(p.GetLabel()))
			items = append(items, po.String())
		}
		return rawJSON("[" + strings.Join(items, ",") + "]")
	}
	if len(sc.GetNumber()) > 0 {
		o.add("phone", phones(sc.GetNumber()))
	}
	if len(sc.GetEmail()) > 0 {
		o.add("email", phones(sc.GetEmail()))
	}
	if len(sc.GetAddress()) > 0 {
		var items []string
		for _, ad := range sc.GetAddress() {
			t := ad.GetType()
			if t == 0 {
				t = signalpb.AppSharedContact_PostalAddress_HOME
			}
			ao := &jsonObject{}
			ao.add("type", t.String())
			for _, f := range [][2]string{{"label", ad.GetLabel()}, {"street", ad.GetStreet()}, {"pobox", ad.GetPobox()}, {"neighborhood", ad.GetNeighborhood()},
				{"city", ad.GetCity()}, {"region", ad.GetRegion()}, {"postcode", ad.GetPostcode()}, {"country", ad.GetCountry()}} {
				ao.add(f[0], blankNull(f[1]))
			}
			items = append(items, ao.String())
		}
		o.add("address", rawJSON("["+strings.Join(items, ",")+"]"))
	}
	if sc.Organization != nil {
		o.add("organization", sc.GetOrganization())
	} else {
		o.add("organization", nil)
	}
	return o
}
