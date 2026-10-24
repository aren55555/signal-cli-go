package manager

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
)

// Attachment downloads during receive (spec/service/attachments.md §5-6).

// AttachmentStore downloads received attachments into the shared
// <config>/attachments directory.
type AttachmentStore struct {
	Dir    string
	CDN    CDN
	Ignore bool // --ignore-attachments: only long text, in memory

	// AvatarsDir is <config>/avatars, for contact avatars from a sync.
	AvatarsDir    string
	IgnoreAvatars bool
	Log           func(format string, args ...any)
}

func (s *AttachmentStore) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

// Path is the stored file of a pointer and its id (§6.3).
func (s *AttachmentStore) Path(p *signalpb.AppAttachmentPointer) (path, id string) {
	remote, err := attachment.RemoteID(p)
	if err != nil {
		return "", ""
	}
	id = attachment.StoredName(remote, p.GetContentType(), p.GetFileName())
	return filepath.Join(s.Dir, id), id
}

// download fetches and verifies one pointer (§5).
func (s *AttachmentStore) download(ctx context.Context, p *signalpb.AppAttachmentPointer) ([]byte, error) {
	if len(p.GetDigest()) == 0 {
		return attachment.Decrypt(p.GetKey(), nil, nil, 0) // "Attachment pointer has no digest."
	}
	remote, err := attachment.RemoteID(p)
	if err != nil {
		return nil, err
	}
	blob, err := s.CDN.Download(ctx, p.GetCdnNumber(), remote, cdn.MaxAttachment)
	if err != nil {
		return nil, err
	}
	return attachment.Decrypt(p.GetKey(), blob, p.GetDigest(), p.GetSize())
}

// store downloads p into its file, writing the inline thumbnail first and
// leaving an empty file on failure (§5.4, §6.3). It returns the plaintext.
func (s *AttachmentStore) store(ctx context.Context, p *signalpb.AppAttachmentPointer) ([]byte, bool) {
	path, id := s.Path(p)
	if path == "" {
		return nil, false
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		s.logf("Failed to download attachment (%s), ignoring: %v", id, err)
		return nil, false
	}
	if p.Thumbnail != nil {
		os.WriteFile(path+".preview", p.GetThumbnail(), 0o644)
	}
	f, err := os.Create(path)
	if err != nil {
		s.logf("Failed to download attachment (%s), ignoring: %v", id, err)
		return nil, false
	}
	defer f.Close()
	data, err := s.download(ctx, p)
	if err != nil {
		s.logf("Failed to download attachment (%s), ignoring: %v", id, err)
		return nil, false
	}
	if _, err := f.Write(data); err != nil {
		s.logf("Failed to download attachment (%s), ignoring: %v", id, err)
		return nil, false
	}
	return data, true
}

// FetchData downloads what a DataMessage refers to, in signal-cli's
// order (§6.2), and returns the long text if there is one.
func (s *AttachmentStore) FetchData(ctx context.Context, dm *signalpb.AppDataMessage) (longText string, ok bool) {
	if s == nil || dm == nil {
		return "", false
	}
	for _, p := range dm.GetAttachments() {
		long := p.GetContentType() == attachment.LongTextType
		var data []byte
		var got bool
		switch {
		case !s.Ignore:
			data, got = s.store(ctx, p)
		case long:
			var err error
			data, err = s.download(ctx, p)
			got = err == nil
			if err != nil {
				s.logf("Failed to download attachment, ignoring: %v", err)
			}
		}
		if long && got {
			longText, ok = string(data), true
		}
	}
	if s.Ignore {
		return longText, ok
	}
	for _, sc := range dm.GetContact() {
		if av := sc.GetAvatar().GetAvatar(); av != nil {
			s.store(ctx, av)
		}
	}
	for _, pv := range dm.GetPreview() {
		if pv.Image != nil {
			s.store(ctx, pv.Image)
		}
	}
	for _, qa := range dm.GetQuote().GetAttachments() {
		if qa.Thumbnail != nil {
			s.store(ctx, qa.Thumbnail)
		}
	}
	return longText, ok
}
