package manager

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
)

// Attachment upload (spec/service/attachments.md §4).

// CDN is the attachment CDN client.
type CDN interface {
	Create(ctx context.Context, f cdn.Form) (string, error)
	Upload(ctx context.Context, f cdn.Form, location string, blob []byte) error
	Download(ctx context.Context, cdnNumber uint32, id string, limit int64) ([]byte, error)
	Fetch(ctx context.Context, path string, limit int64) ([]byte, error)
}

// UploadError is a failed attachment upload; the send is aborted.
type UploadError struct{ Err error }

func (e *UploadError) Error() string { return "Failed to upload attachment: " + e.Err.Error() }
func (e *UploadError) Unwrap() error { return e.Err }

// ErrUploadTooLarge is the form's 413.
var ErrUploadTooLarge = fmt.Errorf("attachment too large")

type pendingUpload struct {
	src      attachment.Source
	key      []byte
	blob     []byte
	digest   []byte
	form     cdn.Form
	location string
	uploadTS uint64
}

// uploadForm is GET /v4/attachments/form/upload on the authenticated
// socket (§4.2).
func (s *Sender) uploadForm(ctx context.Context, length int) (cdn.Form, error) {
	var f cdn.Form
	r, err := s.Chat.Request(ctx, "GET", fmt.Sprintf("/v4/attachments/form/upload?uploadLength=%d", length), nil, nil)
	if err != nil {
		return f, err
	}
	switch r.Status {
	case 200:
	case 413:
		return f, ErrUploadTooLarge
	default:
		return f, fmt.Errorf("upload form: status %d", r.Status)
	}
	if err := json.Unmarshal(r.Body, &f); err != nil {
		return f, fmt.Errorf("upload form: %w", err)
	}
	return f, nil
}

// UploadAttachments encrypts and uploads srcs: all forms and sessions
// first, then the data, in order (§4.1); flags go on every pointer.
func (s *Sender) UploadAttachments(ctx context.Context, srcs []attachment.Source, flags uint32) ([]*signalpb.AppAttachmentPointer, error) {
	if s.CDN == nil {
		return nil, &UploadError{fmt.Errorf("no CDN client")}
	}
	var ups []*pendingUpload
	for _, src := range srcs {
		key, blob, digest, err := attachment.Encrypt(src.Data)
		if err != nil {
			return nil, err
		}
		u := &pendingUpload{src: src, key: key, blob: blob, digest: digest, uploadTS: uint64(s.now().UnixMilli())}
		if u.form, err = s.uploadForm(ctx, len(blob)); err != nil {
			return nil, &UploadError{err}
		}
		if u.location, err = s.CDN.Create(ctx, u.form); err != nil {
			return nil, &UploadError{err}
		}
		ups = append(ups, u)
	}
	var out []*signalpb.AppAttachmentPointer
	for _, u := range ups {
		if err := s.CDN.Upload(ctx, u.form, u.location, u.blob); err != nil {
			return nil, &UploadError{err}
		}
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return nil, err
		}
		id[6] = id[6]&0x0f | 0x40
		id[8] = id[8]&0x3f | 0x80
		out = append(out, attachment.NewPointer(attachment.Uploaded{CDN: u.form.CDN, CDNKey: u.form.Key, Key: u.key, Digest: u.digest, Size: uint32(len(u.src.Data))},
			u.src, flags, u.uploadTS, id))
	}
	return out, nil
}

// Attach uploads a message's attachments before it is sent (§4.1, §6.5):
// a body over 2000 UTF-8 bytes is cut and the full text goes first as a
// long-text attachment, then srcs in order, each with the voice-note flag
// if voiceNote.
func (s *Sender) Attach(ctx context.Context, dm *signalpb.AppDataMessage, srcs []attachment.Source, voiceNote bool) error {
	var ps []*signalpb.AppAttachmentPointer
	if dm.Body != nil {
		if body, split := attachment.SplitLongText(dm.GetBody()); split {
			long, err := s.UploadAttachments(ctx, []attachment.Source{{Data: []byte(dm.GetBody()), ContentType: attachment.LongTextType}}, 0)
			if err != nil {
				return err
			}
			dm.Body = &body
			ps = append(ps, long...)
		}
	}
	if len(srcs) > 0 {
		var flags uint32
		if voiceNote {
			flags = attachment.FlagVoiceMessage
		}
		up, err := s.UploadAttachments(ctx, srcs, flags)
		if err != nil {
			return err
		}
		ps = append(ps, up...)
	}
	dm.Attachments = append(ps, dm.Attachments...)
	return nil
}
