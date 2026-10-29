// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"google.golang.org/protobuf/proto"
)

// fakeCDN stores uploads in memory and serves them back.
type fakeCDN struct {
	log    []string
	blobs  map[string][]byte // cdn key -> blob
	failUp bool
	// lastFields and lastType: the latest PostCDN0File call.
	lastFields []cdn.FormField
	lastType   string
}

func (f *fakeCDN) Create(ctx context.Context, form cdn.Form) (string, error) {
	f.log = append(f.log, "create "+form.Key)
	return "https://cdn.example/session/" + form.Key, nil
}

func (f *fakeCDN) Upload(ctx context.Context, form cdn.Form, location string, blob []byte) error {
	f.log = append(f.log, "upload "+form.Key)
	if f.failUp {
		return errors.New("boom")
	}
	if f.blobs == nil {
		f.blobs = map[string][]byte{}
	}
	f.blobs[form.Key] = blob
	return nil
}

func (f *fakeCDN) Download(ctx context.Context, n uint32, id string, limit int64) ([]byte, error) {
	f.log = append(f.log, "download "+id)
	b, ok := f.blobs[id]
	if !ok {
		return nil, &cdn.StatusError{Method: "GET", Status: 404}
	}
	return b, nil
}

func (f *fakeCDN) Fetch(ctx context.Context, path string, limit int64) ([]byte, error) {
	f.log = append(f.log, "fetch "+path)
	b, ok := f.blobs[path]
	if !ok {
		return nil, &cdn.StatusError{Method: "GET", Status: 404}
	}
	return b, nil
}

// formChat answers upload-form requests with keys k1, k2, …
type formChat struct {
	n     int
	paths []string
	cdnNo uint32
}

func (c *formChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	c.paths = append(c.paths, verb+" "+path)
	if !strings.HasPrefix(path, "/v4/attachments/form/upload?uploadLength=") {
		return &chat.Response{Status: 404}, nil
	}
	c.n++
	b, _ := json.Marshal(cdn.Form{CDN: c.cdnNo, Key: "k" + string(rune('0'+c.n)), Headers: map[string]string{}, SignedUploadLocation: "https://x/attachments"})
	return &chat.Response{Status: 200, Body: b}, nil
}

func TestUploadAttachments(t *testing.T) {
	fc, cd := &formChat{cdnNo: 3}, &fakeCDN{}
	s := &Sender{Chat: fc, CDN: cd, Now: func() time.Time { return time.UnixMilli(1700000000000) }}
	srcs := []attachment.Source{{Data: []byte("hello"), ContentType: "text/plain", FileName: "a.txt"}, {Data: bytes.Repeat([]byte{1}, 1000), ContentType: "image/png", Width: 2, Height: 3}}
	ps, err := s.UploadAttachments(context.Background(), srcs, attachment.FlagVoiceMessage)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cd.log, ",") != "create k1,create k2,upload k1,upload k2" {
		t.Errorf("order %v", cd.log)
	}
	if fc.paths[0] != "GET /v4/attachments/form/upload?uploadLength=592" || fc.paths[1] != "GET /v4/attachments/form/upload?uploadLength=1072" {
		t.Errorf("forms %v", fc.paths)
	}
	p := ps[0]
	if p.GetCdnKey() != "k1" || p.GetCdnNumber() != 3 || p.GetSize() != 5 || p.GetFileName() != "a.txt" || p.GetFlags() != 1 ||
		p.GetUploadTimestamp() != 1700000000000 || len(p.GetClientUuid()) != 16 || p.GetClientUuid()[6]>>4 != 4 || p.Width != nil {
		t.Errorf("pointer %v", p)
	}
	if ps[1].GetWidth() != 2 || ps[1].GetHeight() != 3 || ps[1].FileName != nil {
		t.Errorf("pointer 2 %v", ps[1])
	}
	got, err := attachment.Decrypt(p.GetKey(), cd.blobs["k1"], p.GetDigest(), p.GetSize())
	if err != nil || string(got) != "hello" {
		t.Errorf("round trip: %v %q", err, got)
	}
	cd.failUp = true
	var ue *UploadError
	if _, err := s.UploadAttachments(context.Background(), srcs[:1], 0); !errors.As(err, &ue) {
		t.Errorf("failed upload: %v", err)
	}
}

// TestReceiveDownloads: attachments are stored under their ids, the long
// text is reported, a failed download leaves an empty file (§5.4, §6).
func TestReceiveDownloads(t *testing.T) {
	w := newReceiveWorld(t)
	cd := &fakeCDN{}
	up := &Sender{Chat: &formChat{cdnNo: 3}, CDN: cd}
	long := strings.Repeat("x", 2500)
	ps, err := up.UploadAttachments(context.Background(), []attachment.Source{
		{Data: []byte(long), ContentType: attachment.LongTextType},
		{Data: []byte("png"), ContentType: "image/png", FileName: "pic.PNG"},
		{Data: []byte("gone"), ContentType: "image/jpeg"},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	delete(cd.blobs, "k3")
	ps[1].Thumbnail = []byte("thumb")
	dir := t.TempDir()
	var logs []string
	w.r.Attachments = &AttachmentStore{Dir: dir, CDN: cd, Log: func(f string, a ...any) { logs = append(logs, f) }}
	c := dataContent(long[:2000], 1000)
	c.GetDataMessage().Attachments = ps
	w.push(t, "sealed", c, 1000)
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 1 || w.events[0].LongText[1000] != long {
		t.Fatalf("long text not reported: %+v", w.events)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		return string(b)
	}
	if read("k1.txt") != long || read("k2.PNG") != "png" || read("k2.PNG.preview") != "thumb" || read("k3.jpg") != "" {
		t.Error("stored files differ")
	}
	if len(logs) != 1 {
		t.Errorf("logs %v", logs)
	}

	// --ignore-attachments: nothing stored, long text still reported.
	w2 := newReceiveWorld(t)
	dir2 := t.TempDir()
	w2.r.Attachments = &AttachmentStore{Dir: dir2, CDN: cd, Ignore: true}
	w2.push(t, "sealed", c, 1000)
	w2.queueEmpty()
	w2.r.Run(context.Background(), w2.pipe, 50*time.Millisecond, 0)
	if files, _ := os.ReadDir(dir2); len(files) != 0 || w2.events[0].LongText[1000] != long {
		t.Errorf("ignore: %d files, long text %v", len(files), w2.events[0].LongText != nil)
	}
}

func TestAttachLongText(t *testing.T) {
	fc, cd := &formChat{cdnNo: 2}, &fakeCDN{}
	s := &Sender{Chat: fc, CDN: cd}
	text := strings.Repeat("é", 1001)
	dm := &signalpb.AppDataMessage{Body: proto.String(text)}
	if err := s.Attach(context.Background(), dm, []attachment.Source{{Data: []byte("a"), ContentType: "audio/aac"}}, true); err != nil {
		t.Fatal(err)
	}
	if len(dm.GetBody()) != 2000 || len(dm.Attachments) != 2 {
		t.Fatalf("body %d bytes, %d attachments", len(dm.GetBody()), len(dm.Attachments))
	}
	lt := dm.Attachments[0]
	if lt.GetContentType() != attachment.LongTextType || lt.FileName != nil || lt.GetSize() != uint32(len(text)) || lt.GetFlags() != 0 {
		t.Errorf("long text pointer %v", lt)
	}
	if dm.Attachments[1].GetFlags() != attachment.FlagVoiceMessage {
		t.Errorf("voice note flag %v", dm.Attachments[1])
	}
	got, _ := attachment.Decrypt(lt.GetKey(), cd.blobs[lt.GetCdnKey()], lt.GetDigest(), lt.GetSize())
	if string(got) != text {
		t.Error("long text content")
	}
	// Short text: nothing uploaded.
	cd.log = nil
	dm = &signalpb.AppDataMessage{Body: proto.String(strings.Repeat("a", 2000))}
	if err := s.Attach(context.Background(), dm, nil, false); err != nil || len(cd.log) != 0 || dm.Attachments != nil {
		t.Errorf("short text: %v %v", err, cd.log)
	}
}

func (f *fakeCDN) PostCDN0(ctx context.Context, fields []cdn.FormField, blob []byte) error {
	key := ""
	for _, fl := range fields {
		if fl.Name == "key" {
			key = fl.Value
		}
	}
	f.log = append(f.log, "post "+key)
	if f.blobs == nil {
		f.blobs = map[string][]byte{}
	}
	f.blobs[key] = blob
	return nil
}

func (f *fakeCDN) PostCDN0File(ctx context.Context, fields []cdn.FormField, fileType string, blob []byte) error {
	f.lastFields, f.lastType = fields, fileType
	return f.PostCDN0(ctx, fields, blob)
}
