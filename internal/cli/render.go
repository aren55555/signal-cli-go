package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"google.golang.org/protobuf/proto"
)

// RenderContext is what rendering needs to know about the account
// (spec/surface/cli/_received-message-output.md).
type RenderContext struct {
	Self    string // the account's phone number
	SelfACI string
	// AttachmentsDir is <config>/attachments, for stored-file lines.
	AttachmentsDir string

	longText map[uint64]string // the event being rendered
	// Lookup returns a peer's phone number and contact or profile name,
	// either possibly empty.
	Lookup func(aci string) (number, name string)
	// Group returns a stored group's title; null when it has no state,
	// ok false when it is not stored.
	Group func(id []byte) (title string, null, ok bool)
}

// DataMessage flags (content.md §4.2).
const (
	flagEndSession       = 1
	flagExpirationUpdate = 2
	flagProfileKeyUpdate = 4
)

// ts renders "TS(x)": millis and the UTC instant with milliseconds.
func ts(ms uint64) string {
	return fmt.Sprintf("%d (%s)", ms, time.UnixMilli(int64(ms)).UTC().Format("2006-01-02T15:04:05.000Z"))
}

// address is a sender as rendering sees it.
type address struct {
	aci, number, name string
	device            uint32
	known             bool
}

func (c *RenderContext) address(aci string, device uint32) address {
	a := address{aci: aci, device: device, known: aci != ""}
	if aci == c.SelfACI && aci != "" {
		a.number = c.Self
	} else if c.Lookup != nil && aci != "" {
		a.number, a.name = c.Lookup(aci)
	}
	return a
}

// ident is the identifier: the number if known, else the ACI.
func (a address) ident() string {
	if a.number != "" {
		return a.number
	}
	return a.aci
}

// contact is CONTACT rendering: “NAME” IDENT or IDENT.
func (a address) contact() string {
	if a.name != "" {
		return "“" + a.name + "” " + a.ident()
	}
	return a.ident()
}

// source works out an event's sender: the unsealed sender, the
// envelope's source, or the sender named by the error.
func (c *RenderContext) source(e manager.Event) address {
	if m := e.Message; m != nil && m.Sender.ServiceID != "" {
		return c.address(m.Sender.ServiceID, m.Sender.DeviceID)
	}
	var de *manager.DecryptError
	if errors.As(e.Err, &de) && de.Sender.ServiceID != "" {
		return c.address(de.Sender.ServiceID, de.Sender.DeviceID)
	}
	src := e.Envelope.GetSourceServiceId()
	if b := e.Envelope.GetSourceServiceIdBinary(); src == "" && len(b) > 0 {
		src, _ = protocol.ServiceIDFromBinary(b)
	}
	return c.address(src, e.Envelope.GetSourceDeviceId())
}

func parseContent(e manager.Event) *signalpb.AppContent {
	if e.Message == nil || e.Message.Content == nil {
		return nil
	}
	var c signalpb.AppContent
	if proto.Unmarshal(e.Message.Content, &c) != nil {
		return nil
	}
	return &c
}

// receipt is a ReceiptMessage, or the receipt a server delivery receipt
// stands for.
type receipt struct {
	when       uint64
	kind       signalpb.AppReceipt_Kind
	timestamps []uint64
}

func eventReceipt(e manager.Event, c *signalpb.AppContent) *receipt {
	if e.Message != nil && e.Message.ServerReceipt {
		return &receipt{when: e.Envelope.GetServerTimestamp(), kind: signalpb.AppReceipt_DELIVERY, timestamps: []uint64{e.Envelope.GetClientTimestamp()}}
	}
	if r := c.GetReceiptMessage(); r != nil {
		return &receipt{when: e.Envelope.GetClientTimestamp(), kind: r.GetKind(), timestamps: r.GetTimestamps()}
	}
	return nil
}

// javaException names an error the way signal-cli reports it.
func javaException(err error) (msg, typ string) {
	var de *manager.DecryptError
	switch {
	case errors.Is(err, protocol.ErrUntrustedIdentity):
		return "Untrusted identity key!", "UntrustedIdentityException"
	case errors.Is(err, protocol.ErrDuplicateMessage):
		return err.Error(), "ProtocolDuplicateMessageException"
	case errors.As(err, &de):
		return err.Error(), "ProtocolInvalidMessageException"
	}
	return err.Error(), "InvalidMessageStructureException"
}

type plain struct{ b strings.Builder }

func (p *plain) line(level int, format string, args ...any) {
	p.b.WriteString(strings.Repeat("  ", level))
	fmt.Fprintf(&p.b, format, args...)
	p.b.WriteByte('\n')
}

// RenderPlain renders one event in signal-cli's plain-text form, ending
// with the blank line that terminates every envelope.
func RenderPlain(c *RenderContext, e manager.Event) string {
	c = c.forEvent(e)
	var p plain
	src := c.source(e)
	from := "unknown source"
	if src.known {
		from = src.contact()
	}
	p.line(0, "Envelope from: %s (device: %d) to %s", from, src.device, c.Self)
	p.line(0, "Timestamp: %s", ts(e.Envelope.GetClientTimestamp()))
	p.line(0, "Server timestamps: received: %s delivered: %s", ts(e.Envelope.GetServerTimestamp()), ts(e.ServerDelivered))
	if e.Message != nil && e.Message.Sealed {
		p.line(0, "Sent by unidentified/sealed sender")
	}
	if e.Err != nil {
		if errors.Is(e.Err, protocol.ErrUntrustedIdentity) {
			id := src.ident()
			p.line(0, "The user’s key is untrusted, either the user has reinstalled Signal or a third party sent this message.")
			p.line(0, "Use 'signal-cli -a %s listIdentities -n %s', verify the key and run 'signal-cli -a %s trust -v \"FINGER_PRINT\" %s' to mark it as trusted", c.Self, id, c.Self, id)
			p.line(0, "If you don't care about security, use 'signal-cli -a %s trust -a %s' to trust it without verification", c.Self, id)
		} else {
			msg, typ := javaException(e.Err)
			p.line(0, "Exception: %s (%s)", msg, typ)
		}
	}
	ct := parseContent(e)
	if dm := ct.GetDataMessage(); dm != nil {
		c.plainData(&p, 0, dm)
	}
	if ed := ct.GetEditMessage(); ed != nil {
		p.line(0, "Edit: Target message timestamp: %s", ts(ed.GetTargetSentTimestamp()))
		if ed.GetDataMessage() != nil {
			c.plainData(&p, 1, ed.GetDataMessage())
		}
	}
	if st := ct.GetStoryMessage(); st != nil {
		c.plainStory(&p, 1, st)
	}
	if sm := ct.GetSyncMessage(); sm != nil {
		c.plainSync(&p, sm)
	}
	if r := eventReceipt(e, ct); r != nil {
		p.line(0, "Received a receipt message")
		p.line(1, "When: %s", ts(r.when))
		switch r.kind {
		case signalpb.AppReceipt_DELIVERY:
			p.line(1, "Is delivery receipt")
		case signalpb.AppReceipt_READ:
			p.line(1, "Is read receipt")
		case signalpb.AppReceipt_VIEWED:
			p.line(1, "Is viewed receipt")
		}
		p.line(1, "Timestamps:")
		for _, t := range r.timestamps {
			p.line(1, "- %s", ts(t))
		}
	}
	if tm := ct.GetTypingMessage(); tm != nil {
		p.line(0, "Received a typing message")
		p.line(1, "Action: %s", tm.GetAction().String())
		p.line(1, "Timestamp: %s", ts(tm.GetTimestamp()))
		if g := tm.GetGroupId(); g != nil {
			p.line(1, "Group Info:")
			c.plainGroupInfo(&p, 2, g)
		}
	}
	p.b.WriteByte('\n')
	return p.b.String()
}

// jsonObject builds a JSON object with keys in insertion order.
type jsonObject struct{ parts []string }

func (o *jsonObject) add(key string, v any) {
	var val string
	switch x := v.(type) {
	case rawJSON:
		val = string(x)
	case *jsonObject:
		val = x.String()
	default:
		val = marshalJSON(x)
	}
	o.parts = append(o.parts, strconv.Quote(key)+":"+val)
}

// addAll appends another object's members (flattening).
func (o *jsonObject) addAll(other *jsonObject) { o.parts = append(o.parts, other.parts...) }

func (o *jsonObject) String() string { return "{" + strings.Join(o.parts, ",") + "}" }

type rawJSON string

func strOrNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RenderJSON renders one event as signal-cli's single-line JSON object.
func RenderJSON(c *RenderContext, e manager.Event) string {
	c = c.forEvent(e)
	top := &jsonObject{}
	if e.Err != nil {
		msg, typ := javaException(e.Err)
		ex := &jsonObject{}
		ex.add("message", msg)
		ex.add("type", typ)
		top.add("exception", ex)
	}
	src := c.source(e)
	env := &jsonObject{}
	if src.known {
		name := src.name
		env.add("source", src.ident())
		env.add("sourceNumber", strOrNull(src.number))
		env.add("sourceUuid", strOrNull(src.aci))
		env.add("sourceName", name)
		env.add("sourceDevice", src.device)
	} else {
		for _, k := range []string{"source", "sourceNumber", "sourceUuid", "sourceName", "sourceDevice"} {
			env.add(k, nil)
		}
	}
	env.add("timestamp", e.Envelope.GetClientTimestamp())
	env.add("serverReceivedTimestamp", e.Envelope.GetServerTimestamp())
	env.add("serverDeliveredTimestamp", e.ServerDelivered)
	ct := parseContent(e)
	if dm := ct.GetDataMessage(); dm != nil {
		env.add("dataMessage", c.jsonData(dm))
	}
	if ed := ct.GetEditMessage(); ed != nil {
		eo := &jsonObject{}
		eo.add("targetSentTimestamp", ed.GetTargetSentTimestamp())
		eo.add("dataMessage", c.jsonData(ed.GetDataMessage()))
		env.add("editMessage", eo)
	}
	if st := ct.GetStoryMessage(); st != nil {
		env.add("storyMessage", c.jsonStory(st))
	}
	if sm := ct.GetSyncMessage(); sm != nil {
		env.add("syncMessage", c.jsonSync(sm))
	}
	if r := eventReceipt(e, ct); r != nil {
		ro := &jsonObject{}
		ro.add("when", r.when)
		ro.add("isDelivery", r.kind == signalpb.AppReceipt_DELIVERY)
		ro.add("isRead", r.kind == signalpb.AppReceipt_READ)
		ro.add("isViewed", r.kind == signalpb.AppReceipt_VIEWED)
		tsList := r.timestamps
		if tsList == nil {
			tsList = []uint64{}
		}
		ro.add("timestamps", tsList)
		env.add("receiptMessage", ro)
	}
	if tm := ct.GetTypingMessage(); tm != nil {
		to := &jsonObject{}
		to.add("action", tm.GetAction().String())
		to.add("timestamp", tm.GetTimestamp())
		if g := tm.GetGroupId(); g != nil {
			to.add("groupId", g)
		}
		env.add("typingMessage", to)
	}
	top.add("envelope", env)
	top.add("account", c.Self)
	return top.String()
}

// uuidRaw is the 16 bytes of a hyphenated UUID (tests and lookups).
func uuidRaw(s string) []byte {
	b, _ := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return b
}

// forEvent is the context with the event's long texts.
func (c *RenderContext) forEvent(e manager.Event) *RenderContext {
	cp := *c
	cp.longText = e.LongText
	return &cp
}

// marshalJSON encodes v as Jackson does by default: no HTML escaping of
// & < >, and U+2028/U+2029 written raw.
func marshalJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "null"
	}
	s := strings.TrimSuffix(buf.String(), "\n")
	return strings.NewReplacer(`\u2028`, "\u2028", `\u2029`, "\u2029").Replace(s)
}
