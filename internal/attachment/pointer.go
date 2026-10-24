package attachment

import (
	"errors"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// Pointer flags (§3.1).
const (
	FlagVoiceMessage = 1
	FlagBorderless   = 2
	FlagGIF          = 8
)

// Source describes the data being attached (§7).
type Source struct {
	Data          []byte
	ContentType   string
	FileName      string // empty: no file name in the pointer
	Width, Height uint32
}

// Uploaded is the result of an upload: the form's CDN and key, and the
// attachment key, digest and plaintext size.
type Uploaded struct {
	CDN    uint32
	CDNKey string
	Key    []byte
	Digest []byte
	Size   uint32
}

// NewPointer builds the pointer signal-cli sends (§3.2): flags always
// present, file name and dimensions only when known.
func NewPointer(u Uploaded, s Source, flags uint32, uploadTS uint64, clientUUID []byte) *signalpb.AppAttachmentPointer {
	p := &signalpb.AppAttachmentPointer{
		AttachmentIdentifier: &signalpb.AppAttachmentPointer_CdnKey{CdnKey: u.CDNKey},
		ClientUuid:           clientUUID,
		ContentType:          proto.String(s.ContentType),
		Key:                  u.Key,
		Size:                 proto.Uint32(u.Size),
		Digest:               u.Digest,
		Flags:                proto.Uint32(flags),
		UploadTimestamp:      proto.Uint64(uploadTS),
		CdnNumber:            proto.Uint32(u.CDN),
	}
	if s.FileName != "" {
		p.FileName = proto.String(s.FileName)
	}
	if s.Width > 0 {
		p.Width = proto.Uint32(s.Width)
	}
	if s.Height > 0 {
		p.Height = proto.Uint32(s.Height)
	}
	return p
}

// RemoteID is the pointer's remote id: the cdnKey if present (even
// empty), else the decimal cdnId if positive (§3.3).
func RemoteID(p *signalpb.AppAttachmentPointer) (string, error) {
	switch id := p.GetAttachmentIdentifier().(type) {
	case *signalpb.AppAttachmentPointer_CdnKey:
		return id.CdnKey, nil
	case *signalpb.AppAttachmentPointer_CdnId:
		if int64(id.CdnId) > 0 {
			return strconv.FormatInt(int64(id.CdnId), 10), nil
		}
	}
	return "", errors.New("CDN location not set")
}

// Check is §3.3 step 2's structure check.
func Check(p *signalpb.AppAttachmentPointer) error {
	if p.CdnNumber == nil {
		return errors.New("AttachmentPointer CDN number not set")
	}
	if p.Key == nil {
		return errors.New("AttachmentPointer key not set")
	}
	_, err := RemoteID(p)
	return err
}

// HasLocation is §3.3 step 1: a pointer needs cdnId or cdnKey.
func HasLocation(p *signalpb.AppAttachmentPointer) bool {
	return p.GetAttachmentIdentifier() != nil
}
