// SPDX-License-Identifier: GPL-3.0-or-later

// Package groups is the client side of Signal groups v2: identifiers,
// invite links, decrypted group state and changes
// (spec/service/groups.md).
package groups

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// GroupIDError is an unparsable group ID (§1); commands prefix it with
// "Invalid group id: ".
type GroupIDError struct {
	Input, Cause string
}

func (e *GroupIDError) Error() string {
	return fmt.Sprintf("Failed to decode groupId (must be base64) %q: %s", e.Input, e.Cause)
}

// decodeBasic is Java's basic base64 decoder: standard alphabet, padding
// optional, nothing else allowed.
func decodeBasic(s string) ([]byte, error) {
	trimmed := strings.TrimRight(s, "=")
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return nil, fmt.Errorf("Illegal base64 character %x", c)
		}
	}
	return base64.RawStdEncoding.DecodeString(trimmed)
}

// ParseGroupID parses a CLI group ID: 16 bytes (v1) or 32 bytes (v2).
func ParseGroupID(s string) ([]byte, error) {
	b, err := decodeBasic(s)
	if err != nil {
		return nil, &GroupIDError{Input: s, Cause: err.Error()}
	}
	if len(b) != 16 && len(b) != 32 {
		return nil, &GroupIDError{Input: s, Cause: fmt.Sprintf("Invalid group id of size %d", len(b))}
	}
	return b, nil
}

// FormatGroupID is the text form: standard base64 with padding.
func FormatGroupID(id []byte) string { return base64.StdEncoding.EncodeToString(id) }

// Invite-link errors (§9.1).
var ErrNotALink = errors.New("Link is not a signal group invitation link")

// InviteLinkError is an invalid link; UnknownVersion marks an
// incompatible one.
type InviteLinkError struct {
	Message        string
	UnknownVersion bool
}

func (e *InviteLinkError) Error() string { return e.Message }

// InviteLinkURL is https://signal.group/# plus the encoded link (§5).
func InviteLinkURL(masterKey, password []byte) string {
	b, _ := proto.Marshal(&signalpb.GroupInviteLink{Contents: &signalpb.GroupInviteLink_ContentsV1{
		ContentsV1: &signalpb.GroupInviteLink_V1{GroupMasterKey: masterKey, InviteLinkPassword: password}}})
	return "https://signal.group/#" + base64.RawURLEncoding.EncodeToString(b)
}

// decodeLenient is Signal's lenient decoder: URL-safe if the input has
// - or _, padding added if missing.
func decodeLenient(s string) ([]byte, error) {
	if len(s)%4 == 1 {
		return nil, fmt.Errorf("Base64-encoded string must have at least four characters, but length specified was %d", len(s))
	}
	enc := base64.StdEncoding
	if strings.ContainsAny(s, "-_") {
		enc = base64.URLEncoding
	}
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '='
		if enc == base64.StdEncoding {
			ok = ok || c == '+' || c == '/'
		} else {
			ok = ok || c == '-' || c == '_'
		}
		if !ok {
			return nil, fmt.Errorf("java.io.IOException: Bad Base64 input character '%c' in array position %d", c, i)
		}
	}
	return enc.DecodeString(s)
}

// ErrUncaught marks a link input that signal-cli fails on with an
// uncaught exception (a too-short fragment).
type ErrUncaught struct{ Message string }

func (e *ErrUncaught) Error() string { return e.Message }

// ParseInviteLink reads master key and password from an invite link
// (§9.1).
func ParseInviteLink(link string) (masterKey [32]byte, password []byte, err error) {
	u, perr := url.Parse(link)
	if perr != nil || !(strings.EqualFold(u.Scheme, "https") || strings.EqualFold(u.Scheme, "sgnl")) || !strings.EqualFold(u.Hostname(), "signal.group") {
		return masterKey, nil, ErrNotALink
	}
	if u.Path != "" && u.Path != "/" {
		return masterKey, nil, &InviteLinkError{Message: "No path was expected in uri"}
	}
	frag := u.EscapedFragment()
	if frag == "" {
		return masterKey, nil, &InviteLinkError{Message: "No reference was in the uri"}
	}
	b, derr := decodeLenient(frag)
	if derr != nil {
		if len(frag) < 4 {
			return masterKey, nil, &ErrUncaught{Message: "java.lang.IllegalArgumentException: " + derr.Error()}
		}
		return masterKey, nil, &InviteLinkError{Message: derr.Error()}
	}
	var l signalpb.GroupInviteLink
	if proto.Unmarshal(b, &l) != nil || l.GetContentsV1() == nil {
		return masterKey, nil, &InviteLinkError{Message: "Url contains no known group link content", UnknownVersion: true}
	}
	mk := l.GetContentsV1().GetGroupMasterKey()
	if len(mk) != 32 {
		return masterKey, nil, &InviteLinkError{Message: fmt.Sprintf("org.signal.libsignal.zkgroup.InvalidInputException: Length of array supplied was %d expected 32", len(mk))}
	}
	copy(masterKey[:], mk)
	return masterKey, l.GetContentsV1().GetInviteLinkPassword(), nil
}

var contentRangeRe = regexp.MustCompile(`^versions (\d+)-(\d+)/(\d+)$`)

// ParseContentRange reads a 206 logs response's Content-Range (§3.5.4)
// and returns the next fromRevision.
func ParseContentRange(h string) (next uint32, ok bool) {
	m := contentRangeRe.FindStringSubmatch(h)
	if m == nil {
		return 0, false
	}
	end, err := strconv.ParseUint(m[2], 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(end) + 1, true
}
