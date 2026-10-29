// SPDX-License-Identifier: GPL-3.0-or-later

package attachment

import "strings"

// extensions maps content types to stored-file extensions (§6.3).
var extensions = map[string]string{
	"application/vnd.android.package-archive": "apk",
	"application/json":                        "json",
	"image/png":                               "png",
	"image/jpeg":                              "jpg",
	"image/heic":                              "heic",
	"image/heif":                              "heif",
	"image/webp":                              "webp",
	"image/gif":                               "gif",
	"audio/aac":                               "aac",
	"video/mp4":                               "mp4",
	"text/x-vcard":                            "vcf",
	"text/plain":                              "txt",
	"text/x-signal-plain":                     "txt",
	"application/octet-stream":                "bin",
}

// Sanitize replaces every character outside A–Z a–z 0–9 _ . - with '_'.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			return r
		}
		return '_'
	}, s)
}

// StoredName is the file name (and attachment id) of a received pointer
// (§6.3).
func StoredName(remoteID, contentType, fileName string) string {
	name := Sanitize(remoteID)
	if i := strings.LastIndexByte(fileName, '.'); i >= 0 {
		return name + fileName[i:]
	}
	if ext, ok := extensions[contentType]; ok {
		return name + "." + ext
	}
	return name
}
