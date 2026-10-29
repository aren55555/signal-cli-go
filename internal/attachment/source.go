// SPDX-License-Identifier: GPL-3.0-or-later

package attachment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif"  // dimensions (§7.3)
	_ "image/jpeg" // dimensions
	_ "image/png"  // dimensions
	"os"
	"path/filepath"
	"strings"
)

// Turning `send -a` arguments into sources (§7).

// ErrDataDirectory refuses files under <config>/data (§7.2).
var ErrDataDirectory = errors.New("Attaching files from the signal-cli data directory is not allowed")

// byExtension is the JDK's probe as observed with no MIME databases
// (vectors mime_observations), plus a few common aliases.
var byExtension = map[string]string{
	"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "jpe": "image/jpeg", "gif": "image/gif",
	"webp": "image/webp", "mp4": "video/mp4", "mov": "video/quicktime", "m4a": "audio/mp4", "mp3": "audio/mpeg",
	"ogg": "audio/ogg", "aac": "audio/aac", "wav": "audio/x-wav", "txt": "text/plain", "html": "text/html",
	"htm": "text/html", "pdf": "application/pdf", "json": "application/json", "zip": "application/zip",
	"bin": "application/octet-stream", "svg": "image/svg+xml", "tif": "image/tiff", "tiff": "image/tiff",
	"bmp": "image/bmp", "csv": "text/csv", "doc": "application/msword", "gz": "application/gzip",
}

// sniff is the JDK's content guess for the types it recognises.
func sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("GIF8")):
		return "image/gif"
	case bytes.HasPrefix(b, []byte("RIFF")):
		return "audio/x-wav"
	case bytes.HasPrefix(b, []byte("<?xml")):
		return "application/xml"
	}
	head := strings.ToLower(string(b[:min(len(b), 16)]))
	for _, p := range []string{"<!doctype html", "<html", "<head", "<body"} {
		if strings.HasPrefix(head, p) {
			return "text/html"
		}
	}
	return ""
}

// ContentType detects a file's MIME type: extension, then content, then
// application/octet-stream (§7.2).
func ContentType(name string, data []byte) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		if t, ok := byExtension[strings.ToLower(name[i+1:])]; ok {
			return t
		}
	}
	if t := sniff(data); t != "" {
		return t
	}
	return "application/octet-stream"
}

// dimensions decodes an image's size for image/* up to 20 MiB (§7.3).
func dimensions(contentType string, data []byte) (uint32, uint32) {
	if !strings.HasPrefix(contentType, "image/") || len(data) == 0 || len(data) > 20<<20 {
		return 0, 0
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return uint32(cfg.Width), uint32(cfg.Height)
	}
	// BMP: BITMAPINFOHEADER width and height at offsets 18 and 22.
	if len(data) >= 26 && data[0] == 'B' && data[1] == 'M' {
		w, h := int32(binary.LittleEndian.Uint32(data[18:])), int32(binary.LittleEndian.Uint32(data[22:]))
		if h < 0 {
			h = -h
		}
		if w > 0 && h > 0 {
			return uint32(w), uint32(h)
		}
	}
	return 0, 0
}

// FromArgument turns an -a value into a source: a data URI, else a file
// path outside dataDir (§7).
func FromArgument(arg, dataDir string) (Source, error) {
	d, err := ParseDataURI(arg)
	if err == nil {
		s := Source{Data: d.Data, ContentType: d.MediaType, FileName: d.FileName()}
		s.Width, s.Height = dimensions(s.ContentType, s.Data)
		return s, nil
	}
	// Any data-URI failure, bad base64 included, falls back to a path
	// (observed with signal-cli 0.14.7; the spec says otherwise).
	return FromFile(arg, dataDir)
}

// FromFile reads a file to attach (§7.2).
func FromFile(path, dataDir string) (Source, error) {
	canon, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Source{}, err
	}
	if canon, err = filepath.Abs(canon); err != nil {
		return Source{}, err
	}
	if dataDir != "" {
		dd, err := filepath.Abs(dataDir)
		if err != nil {
			return Source{}, err
		}
		if r, err := filepath.EvalSymlinks(dd); err == nil {
			dd = r
		}
		if rel, err := filepath.Rel(dd, canon); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return Source{}, ErrDataDirectory
		}
	}
	data, err := os.ReadFile(canon)
	if err != nil {
		return Source{}, err
	}
	s := Source{Data: data, FileName: filepath.Base(path)}
	s.ContentType = ContentType(s.FileName, data)
	s.Width, s.Height = dimensions(s.ContentType, data)
	return s, nil
}
