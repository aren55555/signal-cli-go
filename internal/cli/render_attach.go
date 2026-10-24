package cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// ATTACHMENT blocks and Attachment JSON objects
// (spec/surface/cli/_received-message-output.md, attachments.md §3.3).

// attachmentID is the stored file name of a pointer, or "" without a
// remote id.
func attachmentID(p *signalpb.AppAttachmentPointer) string {
	remote, err := attachment.RemoteID(p)
	if err != nil {
		return ""
	}
	return attachment.StoredName(remote, p.GetContentType(), p.GetFileName())
}

func flagNames(flags uint32) string {
	var out []string
	if flags&attachment.FlagVoiceMessage != 0 {
		out = append(out, "voice note")
	}
	if flags&attachment.FlagBorderless != 0 {
		out = append(out, "borderless")
	}
	if flags&attachment.FlagGIF != 0 {
		out = append(out, "video gif")
	}
	return strings.Join(out, ", ")
}

// plainAttachment is the ATTACHMENT block for a received pointer.
func (c *RenderContext) plainAttachment(p *plain, l int, a *signalpb.AppAttachmentPointer) {
	p.line(l, "Content-Type: %s", a.GetContentType())
	id := attachmentID(a)
	if id != "" {
		p.line(l, "Type: Pointer")
		p.line(l, "Id: %s", id)
	} else {
		p.line(l, "Type: Stream")
	}
	if a.GetUploadTimestamp() > 0 {
		p.line(l, "Upload timestamp: %s", ts(a.GetUploadTimestamp()))
	}
	if a.GetCaption() != "" {
		p.line(l, "Caption: %s", a.GetCaption())
	}
	if a.FileName != nil {
		p.line(l, "Filename: %s", a.GetFileName())
	}
	if a.Size != nil || len(a.GetThumbnail()) > 0 {
		size := "<unavailable>"
		if a.Size != nil {
			size = itoa(uint64(a.GetSize())) + " bytes"
		}
		if n := len(a.GetThumbnail()); n > 0 {
			size += " (Preview is available: " + itoa(uint64(n)) + " bytes)"
		}
		p.line(l, "Size: %s", size)
	}
	if f := flagNames(a.GetFlags()); f != "" {
		p.line(l, "Flags: %s", f)
	}
	if a.GetWidth() > 0 || a.GetHeight() > 0 {
		p.line(l, "Dimensions: %dx%d", a.GetWidth(), a.GetHeight())
	}
	if id != "" && c.AttachmentsDir != "" {
		// The path as configured, not made absolute (observed).
		path := strings.TrimRight(c.AttachmentsDir, "/") + "/" + id
		if _, err := os.Stat(path); err == nil {
			p.line(l, "Stored plaintext in: %s", path)
		}
	}
}

// plainQuotedAttachment is the ATTACHMENT block of a quoted attachment: a
// stream with an optional thumbnail pointer.
func (c *RenderContext) plainQuotedAttachment(p *plain, l int, q *signalpb.AppQuotedAttachment) {
	p.line(l, "Content-Type: %s", q.GetContentType())
	p.line(l, "Type: Stream")
	if q.FileName != nil {
		p.line(l, "Filename: %s", q.GetFileName())
	}
	if q.Thumbnail != nil {
		p.line(l, "Thumbnail:")
		c.plainAttachment(p, l+1, q.Thumbnail)
	}
}

// jsonAttachment is the Attachment object: all eight keys, in order.
func jsonAttachment(a *signalpb.AppAttachmentPointer) *jsonObject {
	o := &jsonObject{}
	o.add("contentType", a.GetContentType())
	o.add("filename", nilIfAbsent(a.FileName))
	o.add("id", strOrNull(attachmentID(a)))
	if a.Size != nil {
		o.add("size", a.GetSize())
	} else {
		o.add("size", nil)
	}
	o.add("width", zeroNull(uint64(a.GetWidth())))
	o.add("height", zeroNull(uint64(a.GetHeight())))
	o.add("caption", strOrNull(a.GetCaption()))
	o.add("uploadTimestamp", zeroNull(a.GetUploadTimestamp()))
	return o
}

func jsonAttachments(as []*signalpb.AppAttachmentPointer) rawJSON {
	items := make([]string, len(as))
	for i, a := range as {
		items[i] = jsonAttachment(a).String()
	}
	return rawJSON("[" + strings.Join(items, ",") + "]")
}

func nilIfAbsent(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func zeroNull(v uint64) any {
	if v == 0 {
		return nil
	}
	return v
}

func itoa(v uint64) string { return strconv.FormatUint(v, 10) }

// withLongText puts the long-text attachment body in place of the body
// (attachments.md §6.5).
func (c *RenderContext) withLongText(dm *signalpb.AppDataMessage) *signalpb.AppDataMessage {
	text, ok := c.longText[dm.GetTimestamp()]
	if !ok || dm == nil {
		return dm
	}
	cp := proto.Clone(dm).(*signalpb.AppDataMessage)
	cp.Body = &text
	return cp
}
