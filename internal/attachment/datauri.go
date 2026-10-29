// SPDX-License-Identifier: GPL-3.0-or-later

package attachment

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// DataURI is a parsed data: URI (§7.1).
type DataURI struct {
	MediaType string
	Params    map[string]string
	Data      []byte
}

// ErrNotDataURI: the string is not a data URI (so it is a file path).
var ErrNotDataURI = errors.New("The given string is not a valid data URI.")

// The grammar of §7.1, including the greedy parameter key that makes a
// second parameter part of the first one's name.
var dataURIPattern = regexp.MustCompile(`(?s)^(?i:data:)(.+?/.+?)?(;.+=.+?)?(?i:(;base64))?,(.+)$`)

// ParseDataURI parses s; ErrNotDataURI means it is not one. Bad base64 is
// a different error.
func ParseDataURI(s string) (DataURI, error) {
	m := dataURIPattern.FindStringSubmatch(s)
	if m == nil {
		return DataURI{}, ErrNotDataURI
	}
	d := DataURI{MediaType: m[1], Params: map[string]string{}}
	if d.MediaType == "" {
		d.MediaType = "text/plain"
	}
	if p := m[2]; p != "" {
		i := strings.LastIndexByte(p, '=')
		v, err := url.QueryUnescape(p[i+1:])
		if err != nil {
			return DataURI{}, err
		}
		d.Params[p[1:i]] = v
	}
	if m[3] != "" {
		data, err := decodeBase64(m[4])
		if err != nil {
			return DataURI{}, err
		}
		d.Data = data
	} else {
		v, err := url.QueryUnescape(m[4])
		if err != nil {
			return DataURI{}, err
		}
		d.Data = []byte(v)
	}
	return d, nil
}

// decodeBase64 is the standard alphabet with optional padding; errors
// name the first bad character as Java does.
func decodeBase64(s string) ([]byte, error) {
	trimmed := strings.TrimRight(s, "=")
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return nil, fmt.Errorf("Illegal base64 character %x", c)
		}
	}
	b, err := base64.RawStdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, errors.New("Last unit does not have enough valid bits")
	}
	return b, nil
}

// FileName is the pointer file name a data URI gives.
func (d DataURI) FileName() string { return d.Params["filename"] }
