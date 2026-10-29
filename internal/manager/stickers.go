// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Sticker packs (spec/service/attachments.md §6.6).

// Sticker errors.
var (
	ErrInvalidStickerLink = errors.New("Invalid sticker pack link")
	ErrStickerPackUnknown = errors.New("Sticker pack not found")
	ErrStickerIDUnknown   = errors.New("Sticker id not part of this pack")
	ErrStickerFileMissing = errors.New("Missing local sticker file")
)

// StickerURLError is a URI that does not parse at all.
type StickerURLError struct{ Detail string }

func (e *StickerURLError) Error() string { return "Sticker pack uri has invalid format: " + e.Detail }

// StickerURL is a pack's share URL.
func StickerURL(id, key []byte) string {
	return "https://signal.art/addstickers/#pack_id=" + hex.EncodeToString(id) + "&pack_key=" + hex.EncodeToString(key)
}

// ParseStickerURL reads pack ID and key from a share URL's fragment.
func ParseStickerURL(uri string) (id, key []byte, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, nil, &StickerURLError{Detail: err.Error()}
	}
	frag := u.EscapedFragment()
	if frag == "" {
		return nil, nil, ErrInvalidStickerLink
	}
	params := map[string]string{}
	for _, part := range strings.Split(frag, "&") {
		k, v, _ := strings.Cut(part, "=")
		dk, err1 := url.QueryUnescape(k)
		dv, err2 := url.QueryUnescape(v)
		if err1 != nil || err2 != nil {
			return nil, nil, ErrInvalidStickerLink
		}
		params[dk] = dv
	}
	if params["pack_id"] == "" || params["pack_key"] == "" {
		return nil, nil, ErrInvalidStickerLink
	}
	if id, err = hex.DecodeString(params["pack_id"]); err != nil {
		return nil, nil, ErrInvalidStickerLink
	}
	if key, err = hex.DecodeString(params["pack_key"]); err != nil {
		return nil, nil, ErrInvalidStickerLink
	}
	return id, key, nil
}

// StickerManifest is the local manifest.json (§6.6), keys in this order.
type StickerManifest struct {
	Title    *string                `json:"title"`
	Author   *string                `json:"author"`
	Cover    *StickerManifestEntry  `json:"cover"`
	Stickers []StickerManifestEntry `json:"stickers"`
}

// StickerManifestEntry is one sticker of a local manifest.
type StickerManifestEntry struct {
	ID          uint32  `json:"id"`
	Emoji       *string `json:"emoji"`
	File        string  `json:"file"`
	ContentType *string `json:"contentType"`
}

// StickerStore is the shared <config>/stickers directory.
type StickerStore struct {
	Dir    string
	CDN    CDN
	Ignore bool // receive --ignore-stickers
	Log    func(format string, args ...any)
}

func (st *StickerStore) logf(format string, args ...any) {
	if st.Log != nil {
		st.Log(format, args...)
	}
}

func (st *StickerStore) packDir(id string) string { return filepath.Join(st.Dir, id) }

// StickerFile is the local file of sticker n.
func (st *StickerStore) StickerFile(id string, n uint32) string {
	return filepath.Join(st.packDir(id), strconv.Itoa(int(n)))
}

// Manifest reads a downloaded pack's manifest.json; nil if not cached.
func (st *StickerStore) Manifest(id string) (*StickerManifest, error) {
	b, err := os.ReadFile(filepath.Join(st.packDir(id), "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m StickerManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func strPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

// Download fetches and decrypts a pack unless manifest.json exists:
// manifest, each distinct sticker, manifest.json last (§6.6).
func (st *StickerStore) Download(ctx context.Context, id, key []byte) error {
	hexID := hex.EncodeToString(id)
	dir := st.packDir(hexID)
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
		return nil
	}
	blobKey := attachment.StickerKey(key)
	blob, err := st.CDN.Fetch(ctx, "stickers/"+hexID+"/manifest.proto", cdn.MaxSticker)
	if err != nil {
		return err
	}
	plain, err := attachment.DecryptSticker(blobKey, blob)
	if err != nil {
		return err
	}
	var pack signalpb.AppStickerPack
	if err := proto.Unmarshal(plain, &pack); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entry := func(s *signalpb.AppStickerPack_Sticker) StickerManifestEntry {
		return StickerManifestEntry{ID: s.GetId(), Emoji: strPtr(s.Emoji), File: strconv.Itoa(int(s.GetId())), ContentType: strPtr(s.ContentType)}
	}
	m := StickerManifest{Title: strPtr(pack.Title), Author: strPtr(pack.Author), Stickers: []StickerManifestEntry{}}
	all := pack.GetStickers()
	if pack.Cover != nil {
		c := entry(pack.Cover)
		m.Cover = &c
		all = append([]*signalpb.AppStickerPack_Sticker{pack.Cover}, all...)
	}
	for _, s := range pack.GetStickers() {
		m.Stickers = append(m.Stickers, entry(s))
	}
	done := map[uint32]bool{}
	for _, s := range all {
		if done[s.GetId()] {
			continue
		}
		done[s.GetId()] = true
		blob, err := st.CDN.Fetch(ctx, "stickers/"+hexID+"/full/"+strconv.Itoa(int(s.GetId())), cdn.MaxSticker)
		if err != nil {
			return err
		}
		data, err := attachment.DecryptSticker(blobKey, blob)
		if err != nil {
			return err
		}
		if err := store.WriteFileAtomic(st.StickerFile(hexID, s.GetId()), data, 0o644); err != nil {
			return err
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(filepath.Join(dir, "manifest.json"), b, 0o644)
}

// rememberPack records a pack in the account's store; installed is set
// only when given (an incoming sticker records a pack as not installed).
func rememberPack(a *store.Account, id, key []byte, installed *bool) error {
	hexID := hex.EncodeToString(id)
	i := a.StickerPackByID(hexID)
	if i < 0 {
		a.Stickers = append(a.Stickers, store.StickerPack{ID: hexID, Key: key})
		i = len(a.Stickers) - 1
	}
	if key != nil {
		a.Stickers[i].Key = key
	}
	if installed != nil {
		a.Stickers[i].Installed = *installed
	}
	return a.SaveStickers()
}

// syncStickerInstall sends a stickerPackOperation INSTALL (sync.md §6.11).
func (s *Sender) syncStickerInstall(ctx context.Context, id, key []byte) error {
	return s.SendSync(ctx, &signalpb.AppSyncMessage{StickerPackOperation: []*signalpb.AppSyncMessage_StickerPackOperation{
		{PackId: id, PackKey: key, Type: signalpb.AppSyncMessage_StickerPackOperation_INSTALL.Enum()}}}, false)
}

// AddStickerPack installs a pack from its URL: download now (a failure
// is an error), record it installed, sync the install.
func (s *Sender) AddStickerPack(ctx context.Context, st *StickerStore, uri string) error {
	id, key, err := ParseStickerURL(uri)
	if err != nil {
		return err
	}
	if err := st.Download(ctx, id, key); err != nil {
		return err
	}
	installed := true
	if err := rememberPack(s.Acct, id, key, &installed); err != nil {
		return err
	}
	return s.syncStickerInstall(ctx, id, key)
}

// StickerMessage builds DataMessage.sticker for --sticker PACKID:N
// (content-features.md §3.6, attachments.md §6.6): the emoji from the
// manifest entry at list index N, the local file re-uploaded.
func (s *Sender) StickerMessage(ctx context.Context, st *StickerStore, hexID string, n uint32) (*signalpb.AppSticker, error) {
	i := s.Acct.StickerPackByID(hexID)
	if i < 0 {
		return nil, ErrStickerPackUnknown
	}
	pack := s.Acct.Stickers[i]
	id, _ := hex.DecodeString(hexID)
	m, err := st.Manifest(hexID)
	if err == nil && m == nil {
		if err = st.Download(ctx, id, pack.Key); err == nil {
			m, err = st.Manifest(hexID)
		}
	}
	if err != nil {
		return nil, err
	}
	if m == nil || int(n) >= len(m.Stickers) {
		return nil, ErrStickerIDUnknown
	}
	data, err := os.ReadFile(st.StickerFile(hexID, n))
	if err != nil {
		return nil, ErrStickerFileMissing
	}
	ps, err := s.UploadAttachments(ctx, []attachment.Source{{Data: data, ContentType: attachment.ContentType("", data)}}, 0)
	if err != nil {
		return nil, err
	}
	sticker := &signalpb.AppSticker{PackId: id, PackKey: pack.Key, StickerId: proto.Uint32(n), Data: ps[0]}
	if e := m.Stickers[n].Emoji; e != nil {
		sticker.Emoji = proto.String(*e)
	}
	return sticker, nil
}

// StickerPackForm is the CDN 0 upload form (attachments.md §4.9).
type StickerPackForm struct {
	PackID   string             `json:"packId"`
	Manifest StickerFormEntry   `json:"manifest"`
	Stickers []StickerFormEntry `json:"stickers"`
}

// StickerFormEntry is one upload slot.
type StickerFormEntry struct {
	ID         int    `json:"id"`
	Key        string `json:"key"`
	Credential string `json:"credential"`
	ACL        string `json:"acl"`
	Algorithm  string `json:"algorithm"`
	Date       string `json:"date"`
	Policy     string `json:"policy"`
	Signature  string `json:"signature"`
}

func (f StickerFormEntry) fields() []cdn.FormField {
	return []cdn.FormField{{Name: "acl", Value: f.ACL}, {Name: "key", Value: f.Key}, {Name: "policy", Value: f.Policy},
		{Name: "Content-Type", Value: "application/octet-stream"}, {Name: "x-amz-algorithm", Value: f.Algorithm},
		{Name: "x-amz-credential", Value: f.Credential}, {Name: "x-amz-date", Value: f.Date}, {Name: "x-amz-signature", Value: f.Signature}}
}

// InvalidStickerPackError is a local validation failure.
type InvalidStickerPackError struct{ Reason string }

func (e *InvalidStickerPackError) Error() string { return "Invalid sticker pack: " + e.Reason }

type uploadManifest struct {
	Title    *string               `json:"title"`
	Author   *string               `json:"author"`
	Cover    *uploadManifestEntry  `json:"cover"`
	Stickers []uploadManifestEntry `json:"stickers"`
}

type uploadManifestEntry struct {
	File        *string `json:"file"`
	Emoji       *string `json:"emoji"`
	ContentType *string `json:"contentType"`
	ID          *int    `json:"id"`
}

// readStickerPack reads a manifest.json or a zip with one, and the files
// it names, validating as uploadStickerPack.md lists.
func readStickerPack(path string) (*uploadManifest, map[string][]byte, error) {
	var open func(name string) ([]byte, error)
	var manifest []byte
	switch {
	case strings.HasSuffix(filepath.Base(path), ".zip"):
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, nil, err
		}
		defer zr.Close()
		files := map[string][]byte{}
		for _, f := range zr.File {
			rc, err := f.Open()
			if err != nil {
				return nil, nil, err
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, nil, err
			}
			files[f.Name] = b
		}
		var ok bool
		if manifest, ok = files["manifest.json"]; !ok {
			return nil, nil, errors.New("manifest.json missing from the zip")
		}
		open = func(name string) ([]byte, error) {
			b, ok := files[name]
			if !ok {
				return nil, os.ErrNotExist
			}
			return b, nil
		}
	case filepath.Base(path) == "manifest.json":
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		manifest = b
		dir := filepath.Dir(path)
		open = func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, name)) }
	default:
		return nil, nil, &InvalidStickerPackError{"Could not find manifest.json"}
	}
	dec := json.NewDecoder(strings.NewReader(string(manifest)))
	dec.DisallowUnknownFields()
	var m uploadManifest
	if err := dec.Decode(&m); err != nil {
		return nil, nil, err
	}
	if m.Stickers == nil {
		return nil, nil, &InvalidStickerPackError{"Must set a 'stickers' field."}
	}
	if len(m.Stickers) == 0 {
		return nil, nil, &InvalidStickerPackError{"Must include stickers."}
	}
	files := map[string][]byte{}
	read := func(e uploadManifestEntry) error {
		b, err := open(*e.File)
		if err != nil {
			return &InvalidStickerPackError{"Could not find find " + *e.File}
		}
		files[*e.File] = b
		return nil
	}
	for _, e := range m.Stickers {
		if e.File == nil {
			return nil, nil, &InvalidStickerPackError{"Must set a 'file' field on each sticker."}
		}
		if err := read(e); err != nil {
			return nil, nil, err
		}
	}
	if m.Cover != nil {
		if m.Cover.File == nil {
			return nil, nil, &InvalidStickerPackError{"Must set a 'file' field on the cover."}
		}
		if err := read(*m.Cover); err != nil {
			return nil, nil, err
		}
	}
	return &m, files, nil
}

// UploadStickerPack uploads a pack (attachments.md §6.6) with a fresh
// key: stickers get IDs 0..k-1, the cover k (or sticker 0's entry is
// used as the cover). getForm fetches the CDN 0 form for count blobs.
// The pack is remembered as not installed and its install synced.
func (s *Sender) UploadStickerPack(ctx context.Context, st *StickerStore, path string, getForm func(ctx context.Context, count int) (*StickerPackForm, error)) (string, error) {
	m, files, err := readStickerPack(path)
	if err != nil {
		return "", err
	}
	emoji := func(e uploadManifestEntry) *string {
		if e.Emoji == nil {
			return proto.String("")
		}
		return proto.String(*e.Emoji)
	}
	pack := &signalpb.AppStickerPack{Title: m.Title, Author: m.Author}
	for i, e := range m.Stickers {
		pack.Stickers = append(pack.Stickers, &signalpb.AppStickerPack_Sticker{Id: proto.Uint32(uint32(i)), Emoji: emoji(e)})
	}
	count := len(m.Stickers)
	if m.Cover != nil {
		pack.Cover = &signalpb.AppStickerPack_Sticker{Id: proto.Uint32(uint32(count)), Emoji: emoji(*m.Cover)}
		count++
	} else {
		pack.Cover = proto.Clone(pack.Stickers[0]).(*signalpb.AppStickerPack_Sticker)
	}
	form, err := getForm(ctx, count)
	if err != nil {
		return "", err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	blobKey := attachment.StickerKey(key)
	upload := func(slot StickerFormEntry, plain []byte) error {
		blob, err := attachment.EncryptSticker(blobKey, plain)
		if err != nil {
			return err
		}
		return st.CDN.PostCDN0(ctx, slot.fields(), blob)
	}
	manifestBytes, err := proto.Marshal(pack)
	if err != nil {
		return "", err
	}
	if err := upload(form.Manifest, manifestBytes); err != nil {
		return "", err
	}
	slot := func(id int) (StickerFormEntry, error) {
		for _, f := range form.Stickers {
			if f.ID == id {
				return f, nil
			}
		}
		return StickerFormEntry{}, fmt.Errorf("no upload slot for sticker %d", id)
	}
	entries := m.Stickers
	if m.Cover != nil {
		entries = append(append([]uploadManifestEntry{}, entries...), *m.Cover)
	}
	for i, e := range entries {
		f, err := slot(i)
		if err != nil {
			return "", err
		}
		if err := upload(f, files[*e.File]); err != nil {
			return "", err
		}
	}
	id, err := hex.DecodeString(form.PackID)
	if err != nil {
		return "", fmt.Errorf("bad pack id from the server: %v", err)
	}
	notInstalled := false
	if err := rememberPack(s.Acct, id, key, &notInstalled); err != nil {
		return "", err
	}
	if err := s.syncStickerInstall(ctx, id, key); err != nil {
		st.logf("Failed to sync the sticker pack: %v", err)
	}
	return StickerURL(id, key), nil
}

// Receiving (attachments.md §6.6, sync.md §6.11).

// noteSticker records an incoming sticker's pack and returns it for a
// download, unless already known.
func (r *Receiver) noteSticker(st *signalpb.AppSticker) {
	if st == nil || len(st.GetPackId()) == 0 || len(st.GetPackKey()) == 0 {
		return
	}
	if r.Acct.StickerPackByID(hex.EncodeToString(st.GetPackId())) >= 0 {
		return
	}
	if err := rememberPack(r.Acct, st.GetPackId(), st.GetPackKey(), nil); err != nil {
		r.logf("Storing sticker pack failed: %v", err)
	}
	r.queueStickerDownload(st.GetPackId(), st.GetPackKey())
}

func (r *Receiver) queueStickerDownload(id, key []byte) {
	if r.Stickers == nil || r.Stickers.Ignore {
		return
	}
	r.pending.stickers = append(r.pending.stickers, [2][]byte{id, key})
}

// applyStickerOperations is sync.md §6.11.
func (r *Receiver) applyStickerOperations(ops []*signalpb.AppSyncMessage_StickerPackOperation) {
	for _, op := range ops {
		if op.PackId == nil {
			continue
		}
		installed := op.GetType() != signalpb.AppSyncMessage_StickerPackOperation_REMOVE
		if err := rememberPack(r.Acct, op.GetPackId(), op.PackKey, &installed); err != nil {
			r.logf("Storing sticker pack failed: %v", err)
			continue
		}
		if installed {
			key := op.GetPackKey()
			if key == nil {
				if i := r.Acct.StickerPackByID(hex.EncodeToString(op.GetPackId())); i >= 0 {
					key = r.Acct.Stickers[i].Key
				}
			}
			if key != nil {
				r.queueStickerDownload(op.GetPackId(), key)
			}
		}
	}
}

// downloadStickers runs the queued pack downloads; failures are logged.
func (r *Receiver) downloadStickers(ctx context.Context, packs [][2][]byte) {
	for _, p := range packs {
		if err := r.Stickers.Download(ctx, p[0], p[1]); err != nil {
			r.logf("Failed to download sticker pack: %v", err)
		}
	}
}
