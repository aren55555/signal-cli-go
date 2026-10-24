package manager

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

func TestStickerURL(t *testing.T) {
	var v struct {
		Sticker struct {
			PackID   string `json:"pack_id"`
			PackKey  string `json:"pack_key"`
			ShareURL string `json:"share_url"`
		} `json:"sticker"`
	}
	testvec.Load(t, "attachments.json", &v)
	id, key := testvec.Hex(t, v.Sticker.PackID), testvec.Hex(t, v.Sticker.PackKey)
	if got := StickerURL(id, key); got != v.Sticker.ShareURL {
		t.Errorf("URL %s", got)
	}
	gid, gkey, err := ParseStickerURL("https://signal.art/addstickers/#pack_id=" + "A0A1A2A3A4A5A6A7A8A9AAABACADAEAF" + "&pack_key=" + v.Sticker.PackKey)
	if err != nil || hex.EncodeToString(gid) != v.Sticker.PackID || hex.EncodeToString(gkey) != v.Sticker.PackKey {
		t.Errorf("parse: %v", err)
	}
	for _, bad := range []string{"https://signal.art/addstickers/", "https://signal.art/addstickers/#pack_id=&pack_key=00", "https://x/#pack_id=zz&pack_key=00"} {
		if _, _, err := ParseStickerURL(bad); !errors.Is(err, ErrInvalidStickerLink) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	var ue *StickerURLError
	if _, _, err := ParseStickerURL("https://signal.art/%zz#x"); !errors.As(err, &ue) {
		t.Errorf("syntax error: %v", err)
	}
}

// stickerWorld uploads a pack through UploadStickerPack and serves it back
// at the CDN 0 paths a download uses.
func stickerWorld(t *testing.T) (*Sender, *StickerStore, *fakeCDN, string, []byte) {
	_, s := dataWorld(t)
	cd := &fakeCDN{}
	s.CDN = cd
	st := &StickerStore{Dir: t.TempDir(), CDN: cd}
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.webp"), []byte("RIFF....WEBPsticker a"), 0o600)
	os.WriteFile(filepath.Join(src, "b.webp"), []byte("RIFF....WEBPsticker b"), 0o600)
	os.WriteFile(filepath.Join(src, "cover.webp"), []byte("RIFF....WEBPcover"), 0o600)
	os.WriteFile(filepath.Join(src, "manifest.json"), []byte(`{"title":"T","author":"A","cover":{"file":"cover.webp","emoji":"★"},
		"stickers":[{"file":"a.webp","emoji":"😀"},{"file":"b.webp"}]}`), 0o600)
	getForm := func(ctx context.Context, count int) (*StickerPackForm, error) {
		if count != 3 {
			t.Errorf("form for %d blobs", count)
		}
		f := &StickerPackForm{PackID: "a0a1a2a3a4a5a6a7a8a9aaabacadaeaf", Manifest: StickerFormEntry{ID: -1, Key: "stickers/a0a1a2a3a4a5a6a7a8a9aaabacadaeaf/manifest.proto"}}
		for i := 0; i < count; i++ {
			f.Stickers = append(f.Stickers, StickerFormEntry{ID: i, Key: fmt.Sprintf("stickers/a0a1a2a3a4a5a6a7a8a9aaabacadaeaf/full/%d", i)})
		}
		return f, nil
	}
	url, err := s.UploadStickerPack(context.Background(), st, filepath.Join(src, "manifest.json"), getForm)
	if err != nil {
		t.Fatal(err)
	}
	id, key, err := ParseStickerURL(url)
	if err != nil {
		t.Fatal(err)
	}
	return s, st, cd, hex.EncodeToString(id), key
}

func TestStickerUploadAndDownload(t *testing.T) {
	s, st, _, hexID, key := stickerWorld(t)
	if i := s.Acct.StickerPackByID(hexID); i < 0 || s.Acct.Stickers[i].Installed || len(key) != 32 {
		t.Fatalf("pack store %+v", s.Acct.Stickers)
	}
	id, _ := hex.DecodeString(hexID)
	if err := st.Download(context.Background(), id, key); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(st.Dir, hexID, "manifest.json"))
	want := `{"title":"T","author":"A","cover":{"id":2,"emoji":"★","file":"2","contentType":null},"stickers":[{"id":0,"emoji":"😀","file":"0","contentType":null},{"id":1,"emoji":"","file":"1","contentType":null}]}`
	if string(b) != want {
		t.Errorf("manifest.json\n%s\nwant\n%s", b, want)
	}
	for n, wantData := range map[uint32]string{0: "RIFF....WEBPsticker a", 1: "RIFF....WEBPsticker b", 2: "RIFF....WEBPcover"} {
		if got, _ := os.ReadFile(st.StickerFile(hexID, n)); string(got) != wantData {
			t.Errorf("sticker %d: %q", n, got)
		}
	}

	sticker, err := s.StickerMessage(context.Background(), st, hexID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sticker.GetEmoji() != "😀" || sticker.GetStickerId() != 0 || hex.EncodeToString(sticker.GetPackId()) != hexID ||
		sticker.GetData().GetContentType() != "audio/x-wav" || sticker.GetData().FileName != nil {
		t.Errorf("sticker %v", sticker)
	}
	if _, err := s.StickerMessage(context.Background(), st, hexID, 2); !errors.Is(err, ErrStickerIDUnknown) {
		t.Errorf("index past the list: %v", err)
	}
	if _, err := s.StickerMessage(context.Background(), st, "00ff", 0); !errors.Is(err, ErrStickerPackUnknown) {
		t.Errorf("unknown pack: %v", err)
	}
}

func TestStickerPackValidation(t *testing.T) {
	_, s := dataWorld(t)
	st := &StickerStore{Dir: t.TempDir(), CDN: &fakeCDN{}}
	dir := t.TempDir()
	cases := []struct{ manifest, want string }{
		{`{"title":"x"}`, "Invalid sticker pack: Must set a 'stickers' field."},
		{`{"stickers":[]}`, "Invalid sticker pack: Must include stickers."},
		{`{"stickers":[{"emoji":"x"}]}`, "Invalid sticker pack: Must set a 'file' field on each sticker."},
		{`{"stickers":[{"file":"missing.webp"}]}`, "Invalid sticker pack: Could not find find missing.webp"},
	}
	for _, c := range cases {
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(c.manifest), 0o600)
		_, err := s.UploadStickerPack(context.Background(), st, filepath.Join(dir, "manifest.json"), nil)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: %v", c.manifest, err)
		}
	}
	if _, err := s.UploadStickerPack(context.Background(), st, filepath.Join(dir, "pack.json"), nil); err == nil || err.Error() != "Invalid sticker pack: Could not find manifest.json" {
		t.Errorf("wrong name: %v", err)
	}
}

// TestReceivedStickerPack: an incoming sticker records its pack (not
// installed) and the pack is fetched after the batch; a sync INSTALL marks
// it installed (attachments.md §6.6, sync.md §6.11).
func TestReceivedStickerPack(t *testing.T) {
	_, _, cd, hexID, key := stickerWorld(t)
	id, _ := hex.DecodeString(hexID)
	w := newReceiveWorld(t)
	w.r.Stickers = &StickerStore{Dir: t.TempDir(), CDN: cd}
	c := dataContent("", 1000)
	c.GetDataMessage().Body = nil
	c.GetDataMessage().Sticker = &signalpb.AppSticker{PackId: id, PackKey: key, StickerId: proto.Uint32(1), Data: keyed(located("image/webp"))}
	w.push(t, "sealed", c, 1000)
	w.queueEmpty()
	if err := w.r.Run(context.Background(), w.pipe, 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	i := w.a.StickerPackByID(hexID)
	if i < 0 || w.a.Stickers[i].Installed {
		t.Fatalf("pack store %+v, events %+v", w.a.Stickers, w.events)
	}
	if _, err := os.Stat(filepath.Join(w.r.Stickers.Dir, hexID, "manifest.json")); err != nil {
		t.Errorf("pack not downloaded: %v", err)
	}
	w.r.applyStickerOperations([]*signalpb.AppSyncMessage_StickerPackOperation{{PackId: id}})
	if !w.a.Stickers[w.a.StickerPackByID(hexID)].Installed {
		t.Error("sync INSTALL did not install")
	}
	w.r.applyStickerOperations([]*signalpb.AppSyncMessage_StickerPackOperation{{PackId: id, Type: signalpb.AppSyncMessage_StickerPackOperation_REMOVE.Enum()}})
	if w.a.Stickers[w.a.StickerPackByID(hexID)].Installed {
		t.Error("sync REMOVE did not uninstall")
	}
	b, _ := os.ReadFile(filepath.Join(w.a.Dir(), "stickers.json"))
	var doc struct{ Packs []store.StickerPack }
	json.Unmarshal(b, &doc)
	if len(doc.Packs) != 1 || doc.Packs[0].ID != hexID {
		t.Errorf("stickers.json %s", b)
	}
}

func keyed(p *signalpb.AppAttachmentPointer) *signalpb.AppAttachmentPointer {
	p.Key = make([]byte, 64)
	return p
}
