package cli

import (
	"encoding/base64"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// The SYNC block and the SyncMessage JSON object
// (spec/surface/cli/_received-message-output.md, sync.md §2.1).

// sentDestination is a transcript's destination service ID, binary form
// preferred.
func sentDestination(s *signalpb.AppSent) string {
	return aciOf(s.GetDestinationServiceIdBinary(), s.GetDestinationServiceId())
}

// mrrTypeName maps a received message request response type: wire values
// 1-4 by name, everything else UNKNOWN (sync.md §6.9).
func mrrTypeName(t signalpb.AppMessageRequestResponse_Type) string {
	switch t {
	case signalpb.AppMessageRequestResponse_ACCEPT, signalpb.AppMessageRequestResponse_DELETE,
		signalpb.AppMessageRequestResponse_BLOCK, signalpb.AppMessageRequestResponse_BLOCK_AND_DELETE:
		return t.String()
	}
	return "UNKNOWN"
}

// blockedIdentifiers are the received blocked recipients: numbers and
// string ACIs only (sync.md §6.4), each as number if known, else UUID.
func (c *RenderContext) blockedIdentifiers(b *signalpb.AppSyncMessage_Blocked) []string {
	var out []string
	out = append(out, b.GetNumbers()...)
	for _, aci := range b.GetAcis() {
		out = append(out, c.address(aci, 0).ident())
	}
	return out
}

func (c *RenderContext) plainSync(p *plain, sm *signalpb.AppSyncMessage) {
	p.line(0, "Received a sync message")
	switch manager.SyncMemberOf(sm) {
	case manager.SyncContacts:
		if sm.Contacts.GetComplete() {
			p.line(0, "Received complete sync contacts:")
		} else {
			p.line(0, "Received partial sync contacts:")
		}
	case manager.SyncGroups:
		p.line(0, "Received sync groups.")
	case manager.SyncRead, manager.SyncViewed:
		list, word := sm.Read, "read"
		if manager.SyncMemberOf(sm) == manager.SyncViewed {
			list, word = sm.Viewed, "viewed"
		}
		p.line(0, "Received sync %s messages list", word)
		for _, r := range list {
			p.line(0, "- From: %s Message timestamp: %s", c.contactOf(aciOf(r.GetSenderAciBinary(), r.GetSenderAci())), ts(r.GetTimestamp()))
		}
	case manager.SyncSent:
		s := sm.Sent
		p.line(0, "Received sync sent message")
		to := "<unknown>"
		if d := sentDestination(s); d != "" {
			to = c.address(d, 0).contact()
		} else if us := s.GetUnidentifiedStatus(); len(us) > 0 {
			var names []string
			for _, u := range us {
				names = append(names, c.contactOf(aciOf(u.GetDestinationServiceIdBinary(), u.GetDestinationServiceId())))
			}
			to = strings.Join(names, ", ")
		}
		p.line(1, "To: %s", to)
		p.line(1, "Timestamp: %s", ts(s.GetTimestamp()))
		if x := s.GetExpirationStartTimestamp(); x > 0 {
			p.line(1, "Expiration started at: %s", ts(x))
		}
		if s.GetMessage() != nil {
			c.plainData(p, 1, s.GetMessage())
		}
	case manager.SyncBlocked:
		p.line(0, "Received sync message with block list")
		p.line(0, "Blocked:")
		for _, id := range c.blockedIdentifiers(sm.Blocked) {
			p.line(0, "- %s", id)
		}
		for _, g := range sm.Blocked.GetGroupIds() {
			p.line(0, "- %s", base64.StdEncoding.EncodeToString(g))
		}
	case manager.SyncViewOnceOpen:
		v := sm.ViewOnceOpen
		p.line(0, "Received sync message with view once open message:")
		p.line(1, "Sender: %s", c.contactOf(aciOf(v.GetSenderAciBinary(), v.GetSenderAci())))
		p.line(1, "Timestamp: %s", ts(v.GetTimestamp()))
	case manager.SyncMessageRequestResponse:
		m := sm.MessageRequestResponse
		p.line(0, "Received message request response:")
		p.line(1, "Type: %s", mrrTypeName(m.GetType()))
		if m.GroupId != nil {
			p.line(0, "For group:")
			p.line(1, "Id: %s", base64.StdEncoding.EncodeToString(m.GetGroupId()))
			p.line(1, "Name: <Unknown group>") // no group store yet
		} else {
			p.line(1, "For Person: %s", c.contactOf(aciOf(m.GetThreadAciBinary(), m.GetThreadAci())))
		}
	}
}

func (c *RenderContext) jsonSync(sm *signalpb.AppSyncMessage) *jsonObject {
	so := &jsonObject{}
	switch manager.SyncMemberOf(sm) {
	case manager.SyncSent:
		s := sm.Sent
		sent := &jsonObject{}
		if d := sentDestination(s); d != "" {
			a := c.address(d, 0)
			sent.add("destination", a.ident())
			sent.add("destinationNumber", strOrNull(a.number))
			sent.add("destinationUuid", d)
		} else {
			sent.add("destination", nil)
			sent.add("destinationNumber", nil)
			sent.add("destinationUuid", nil)
		}
		if ed := s.GetEditMessage(); ed != nil {
			eo := &jsonObject{}
			eo.add("targetSentTimestamp", ed.GetTargetSentTimestamp())
			eo.add("dataMessage", c.jsonData(ed.GetDataMessage()))
			sent.add("editMessage", eo)
		}
		if s.GetMessage() != nil {
			sent.addAll(c.jsonData(s.GetMessage()))
		}
		so.add("sentMessage", sent)
	case manager.SyncBlocked:
		ids := c.blockedIdentifiers(sm.Blocked)
		if ids == nil {
			ids = []string{}
		}
		so.add("blockedNumbers", ids)
		groups := []string{}
		for _, g := range sm.Blocked.GetGroupIds() {
			groups = append(groups, base64.StdEncoding.EncodeToString(g))
		}
		so.add("blockedGroupIds", groups)
	case manager.SyncRead:
		var items []string
		for _, r := range sm.Read {
			ro := &jsonObject{}
			c.jsonAuthor(ro, "sender", aciOf(r.GetSenderAciBinary(), r.GetSenderAci()), "sender")
			ro.add("timestamp", r.GetTimestamp())
			items = append(items, ro.String())
		}
		so.add("readMessages", rawJSON("["+strings.Join(items, ",")+"]"))
	case manager.SyncContacts:
		so.add("type", "CONTACTS_SYNC")
	case manager.SyncGroups:
		so.add("type", "GROUPS_SYNC")
	}
	return so
}
