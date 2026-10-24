package cli

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Sticker commands (spec/surface/cli/{addStickerPack,getSticker,
// listStickerPacks,uploadStickerPack}.md, attachments.md §6.6).

func (n *network) stickerStore(g *globals) *manager.StickerStore {
	return &manager.StickerStore{Dir: filepath.Join(g.dataDir, "stickers"), CDN: n.cdn()}
}

func runAddStickerPack(ctx context.Context, e *env, g *globals, uris []string) error {
	return withAccount(g, func(a *store.Account) error {
		for _, uri := range uris {
			if _, _, err := manager.ParseStickerURL(uri); err != nil {
				var ue *manager.StickerURLError
				if errors.As(err, &ue) {
					return userError(ue.Error())
				}
				fmt.Fprintln(e.stderr, "ERROR AddStickerPackCommand - Invalid sticker pack link")
				return userError("Invalid sticker pack link")
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		n, _ := e.network(g)
		for _, uri := range uris {
			if err := s.sender.AddStickerPack(ctx, n.stickerStore(g), uri); err != nil {
				fmt.Fprintf(e.stderr, "ERROR AddStickerPackCommand - Install sticker pack failed: %v\n", err)
				return ioError("Install sticker pack failed")
			}
		}
		return nil
	})
}

func runGetSticker(e *env, g *globals, packID string, stickerID int) error {
	return withAccount(g, func(a *store.Account) error {
		if len(packID)%2 != 0 {
			return &cliError{msg: "Exception in thread \"main\" java.lang.IllegalArgumentException: odd-length pack id", code: 2}
		}
		id := strings.ToLower(packID)
		st := &manager.StickerStore{Dir: filepath.Join(g.dataDir, "stickers")}
		data, err := os.ReadFile(st.StickerFile(id, uint32(stickerID)))
		notFound := fmt.Sprintf("Could not find sticker with ID: %d in pack StickerPackId{%s}", stickerID, id)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// A known pack is fetched, but this call still fails (§6.6).
			if i := a.StickerPackByID(id); i >= 0 {
				if n, nerr := e.network(g); nerr == nil {
					raw, _ := hex.DecodeString(id)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					n.stickerStore(g).Download(ctx, raw, a.Stickers[i].Key)
				}
			}
			return userError(notFound)
		case err != nil:
			return &cliError{msg: fmt.Sprintf("An error occurred reading sticker with ID: %d in pack StickerPackId{%s}", stickerID, id), code: 2}
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		if g.json() {
			o := &jsonObject{}
			o.add("data", b64)
			fmt.Fprintln(e.stdout, o.String())
		} else {
			fmt.Fprintln(e.stdout, b64)
		}
		return nil
	})
}

func runListStickerPacks(e *env, g *globals) error {
	return withAccount(g, func(a *store.Account) error {
		st := &manager.StickerStore{Dir: filepath.Join(g.dataDir, "stickers")}
		var items []string
		for _, p := range a.Stickers {
			id, _ := hex.DecodeString(p.ID)
			url := manager.StickerURL(id, p.Key)
			m, _ := st.Manifest(p.ID)
			if g.json() {
				o := &jsonObject{}
				o.add("packId", p.ID)
				o.add("url", url)
				o.add("installed", p.Installed)
				entry := func(s manager.StickerManifestEntry) *jsonObject {
					so := &jsonObject{}
					so.add("id", s.ID)
					so.add("emoji", s.Emoji)
					so.add("contentType", s.ContentType)
					return so
				}
				if m == nil {
					o.add("title", "")
					o.add("author", "")
					o.add("cover", nil)
					o.add("stickers", rawJSON("[]"))
				} else {
					o.add("title", m.Title)
					o.add("author", m.Author)
					if m.Cover != nil {
						o.add("cover", entry(*m.Cover))
					} else {
						o.add("cover", nil)
					}
					var ss []string
					for _, s := range m.Stickers {
						ss = append(ss, entry(s).String())
					}
					o.add("stickers", rawJSON("["+strings.Join(ss, ",")+"]"))
				}
				items = append(items, o.String())
				continue
			}
			title, author, count := "", "", 0
			if m != nil {
				title, author, count = deref(m.Title), deref(m.Author), len(m.Stickers)
			}
			fmt.Fprintf(e.stdout, "Pack %s: “%s” by “%s” has %d stickers. %s\n", p.ID, title, author, count, url)
		}
		if g.json() {
			fmt.Fprintln(e.stdout, "["+strings.Join(items, ",")+"]")
		}
		return nil
	})
}

func deref(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}

func runUploadStickerPack(ctx context.Context, e *env, g *globals, path string) error {
	return withAccount(g, func(a *store.Account) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		n, _ := e.network(g)
		rc := rest.New(rest.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent})
		user := a.Data.ACI
		if a.Data.DeviceID != 1 {
			user += "." + strconv.Itoa(a.Data.DeviceID)
		}
		getForm := func(ctx context.Context, count int) (*manager.StickerPackForm, error) {
			var f manager.StickerPackForm
			err := rc.Do(ctx, "GET", "/v1/sticker/pack/form/"+strconv.Itoa(count), &rest.Basic{User: user, Password: a.Data.Password}, nil, &f)
			return &f, err
		}
		url, err := s.sender.UploadStickerPack(ctx, n.stickerStore(g), path, getForm)
		var ie *manager.InvalidStickerPackError
		switch {
		case errors.As(err, &ie):
			return userError(ie.Error())
		case err != nil:
			return ioError("Upload error (maybe image size too large):%v", err)
		}
		if g.json() {
			o := &jsonObject{}
			o.add("url", url)
			fmt.Fprintln(e.stdout, o.String())
		} else {
			fmt.Fprintln(e.stdout, url)
		}
		return nil
	})
}

// runUpdateConfiguration stores the given settings and syncs them
// (sync.md §6.6).
func runUpdateConfiguration(ctx context.Context, e *env, g *globals, set map[string]*bool) error {
	return withAccount(g, func(a *store.Account) error {
		if a.Data.DeviceID != 1 {
			return userError(manager.ErrNotPrimary.Error())
		}
		cfg := &a.Data.Configuration
		for name, v := range set {
			if v == nil {
				continue
			}
			switch name {
			case "read-receipts":
				cfg.ReadReceipts = v
			case "unidentified-delivery-indicators":
				cfg.UnidentifiedDeliveryIndicators = v
			case "typing-indicators":
				cfg.TypingIndicators = v
			case "link-previews":
				cfg.LinkPreviews = v
			}
		}
		if err := a.Save(); err != nil {
			return ioError("UpdateAccount error: %v", err)
		}
		if !a.IsMultiDevice() {
			return nil
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if err := s.sender.SendConfiguration(ctx); err != nil {
			fmt.Fprintf(e.stderr, "WARN  SyncHelper - %v\n", err)
		}
		return nil
	})
}
