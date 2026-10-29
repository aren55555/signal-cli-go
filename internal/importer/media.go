// SPDX-License-Identifier: GPL-3.0-or-later

package importer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aren55555/signal-cli-go/internal/store"
)

// copyMedia copies the shared attachments, avatars and stickers
// directories (data-dir §6). signal-cli-go uses the same names and
// layout. Files already in the destination are kept.
func copyMedia(src, dst string) error {
	for _, dir := range []string{"attachments", "avatars", "stickers"} {
		from := filepath.Join(src, dir)
		err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, os.ErrNotExist) && path == from {
					return fs.SkipDir
				}
				return err
			}
			rel, err := filepath.Rel(from, path)
			if err != nil {
				return err
			}
			to := filepath.Join(dst, dir, rel)
			switch {
			case d.IsDir():
				return os.MkdirAll(to, 0o700)
			case !d.Type().IsRegular():
				return nil
			}
			err = copyFile(path, to)
			if errors.Is(err, os.ErrExist) {
				return nil
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	return nil
}

// cachedEnvelopeVersion is the msg-cache file format of signal-cli 0.14.7
// (spec/protocol/capture.md §3): uint32 version, uint64 server-delivered
// time, then the envelope.
const cachedEnvelopeVersion = 9

// copyMsgCache converts signal-cli's cached envelopes (data-dir §5) into
// our cache files: the 8-byte delivered time, then the envelope. Older
// file versions are not converted; their count is returned.
func copyMsgCache(from, to string) (old int, err error) {
	n := 0
	err = filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && path == from {
				return fs.SkipDir
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(b) < 12 || binary.BigEndian.Uint32(b) != cachedEnvelopeVersion {
			old++
			return nil
		}
		if err := os.MkdirAll(to, 0o700); err != nil {
			return err
		}
		n++
		delivered := binary.BigEndian.Uint64(b[4:])
		name := fmt.Sprintf("%020d-import%06d.env", delivered, n)
		return store.WriteFileAtomic(filepath.Join(to, name), b[4:], 0o600)
	})
	return old, err
}
