// SPDX-License-Identifier: GPL-3.0-or-later

// Package fingerprint computes safety numbers: the numeric fingerprint
// and the scannable form, and parses what users paste into `trust -v`
// (spec/crypto/fingerprint.md).
package fingerprint

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"

	"google.golang.org/protobuf/encoding/protowire"
)

// Iterations is the hash iteration count signal-cli uses (§2.1).
const Iterations = 5200

// Digest is the per-side digest of a stable identifier and a 33-byte
// identity key (§2.1).
func Digest(id, key []byte) [64]byte {
	h := sha512.New()
	h.Write([]byte{0, 0})
	h.Write(key)
	h.Write(id)
	h.Write(key)
	var d [64]byte
	h.Sum(d[:0])
	for range Iterations - 1 {
		h.Reset()
		h.Write(d[:])
		h.Write(key)
		h.Sum(d[:0])
	}
	return d
}

// Digits are the 30 per-side digits (§2.2).
func Digits(d [64]byte) string {
	var b strings.Builder
	for i := 0; i < 30; i += 5 {
		var chunk [8]byte
		copy(chunk[3:], d[i:i+5])
		fmt.Fprintf(&b, "%05d", binary.BigEndian.Uint64(chunk[:])%100000)
	}
	return b.String()
}

// Fingerprint is both forms for one pair of parties.
type Fingerprint struct {
	Display   string // 60 digits, no separators
	Scannable []byte // the encoding of §4.2, from the local party's view
}

// New computes the fingerprint for a local and a remote party.
func New(version uint32, localID, localKey, remoteID, remoteKey []byte) Fingerprint {
	l, r := Digest(localID, localKey), Digest(remoteID, remoteKey)
	ld, rd := Digits(l), Digits(r)
	display := ld + rd
	if rd < ld {
		display = rd + ld
	}
	return Fingerprint{Display: display, Scannable: encodeScannable(version, l[:32], r[:32])}
}

func encodeScannable(version uint32, local, remote []byte) []byte {
	logical := func(content []byte) []byte {
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		return protowire.AppendBytes(b, content)
	}
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(version))
	b = protowire.AppendTag(b, 2, protowire.BytesType)
	b = protowire.AppendBytes(b, logical(local))
	b = protowire.AppendTag(b, 3, protowire.BytesType)
	return protowire.AppendBytes(b, logical(remote))
}

// Comparison errors (§4.3).
var (
	ErrVersionMismatch = errors.New("fingerprint version mismatch")
	ErrParse           = errors.New("fingerprint parsing error")
)

type scannable struct {
	version       uint64
	local, remote []byte
	hasL, hasR    bool
}

func decodeScannable(b []byte) (*scannable, error) {
	s := &scannable{}
	content := func(m []byte) ([]byte, bool, error) {
		var out []byte
		found := false
		for len(m) > 0 {
			n, typ, l := protowire.ConsumeTag(m)
			if l < 0 {
				return nil, false, ErrParse
			}
			m = m[l:]
			vl := protowire.ConsumeFieldValue(n, typ, m)
			if vl < 0 {
				return nil, false, ErrParse
			}
			if n == 1 && typ == protowire.BytesType {
				out, _ = protowire.ConsumeBytes(m)
				found = true
			}
			m = m[vl:]
		}
		return out, found, nil
	}
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return nil, ErrParse
		}
		b = b[l:]
		vl := protowire.ConsumeFieldValue(n, typ, b)
		if vl < 0 {
			return nil, ErrParse
		}
		switch {
		case n == 1 && typ == protowire.VarintType:
			s.version, _ = protowire.ConsumeVarint(b)
		case (n == 2 || n == 3) && typ == protowire.BytesType:
			m, _ := protowire.ConsumeBytes(b)
			c, ok, err := content(m)
			if err != nil {
				return nil, err
			}
			if n == 2 {
				s.local, s.hasL = c, ok
			} else {
				s.remote, s.hasR = c, ok
			}
		}
		b = b[vl:]
	}
	return s, nil
}

// Compare checks a code scanned from the other party against ours (§4.3).
func Compare(ours, theirs []byte) (bool, error) {
	o, err := decodeScannable(ours)
	if err != nil {
		return false, err
	}
	t, err := decodeScannable(theirs)
	if err != nil {
		return false, err
	}
	if t.version != o.version {
		return false, ErrVersionMismatch
	}
	if !t.hasL || !t.hasR {
		return false, ErrParse
	}
	return subtle.ConstantTimeCompare(t.local, o.remote) == 1 && subtle.ConstantTimeCompare(t.remote, o.local) == 1, nil
}

// FormatSafetyNumber cuts the digits into 12 equal groups (§5.2).
func FormatSafetyNumber(s string) string {
	n := len(s) / 12
	parts := make([]string, 12)
	for i := range parts {
		parts[i] = s[i*n : (i+1)*n]
	}
	return strings.Join(parts, " ")
}

// FormatKey is the spaced lowercase hex of an identity key (§5.2).
func FormatKey(key []byte) string {
	parts := make([]string, len(key))
	for i, b := range key {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, " ")
}

// Kind is the type of a pasted verification code (§6.1).
type Kind int

const (
	KindHex Kind = iota
	KindNumeric
	KindScannable
)

// Code is a parsed `trust -v` value.
type Code struct {
	Kind    Kind
	Numeric string
	Bytes   []byte
}

// ErrInvalidFormat is the only parse failure (§6.1 step 3).
var ErrInvalidFormat = errors.New("Safety number has invalid format, either specify the old hex fingerprint or the new safety number")

// ParseVerificationCode classifies and decodes a `trust -v` value.
func ParseVerificationCode(input string) (Code, error) {
	s := strings.ReplaceAll(input, " ", "")
	switch len(utf16.Encode([]rune(s))) {
	case 66:
		return Code{Kind: KindHex, Bytes: javaHex(strings.ToLower(s))}, nil
	case 60:
		return Code{Kind: KindNumeric, Numeric: s}, nil
	}
	b, err := javaBase64(s)
	if err != nil {
		return Code{}, ErrInvalidFormat
	}
	return Code{Kind: KindScannable, Bytes: b}, nil
}

// digit is Java's Character.digit(c, 16) for the ASCII range: -1 when c
// is not a hex digit.
func digit(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// javaHex decodes pairs as (hi << 4 | lo) with invalid digits as -1,
// never failing (§6.1).
func javaHex(s string) []byte {
	rs := []rune(s)
	out := make([]byte, len(rs)/2)
	for i := range out {
		out[i] = byte(digit(rs[2*i])<<4 | digit(rs[2*i+1]))
	}
	return out
}

// javaBase64 is Java's basic decoder: standard alphabet, optional but
// correct padding, no single-character final group.
func javaBase64(s string) ([]byte, error) {
	body := strings.TrimRight(s, "=")
	pad := len(s) - len(body)
	if pad > 2 || (pad > 0 && len(s)%4 != 0) {
		return nil, ErrInvalidFormat
	}
	if len(body)%4 == 1 {
		return nil, ErrInvalidFormat
	}
	b, err := base64.RawStdEncoding.DecodeString(body)
	if err != nil {
		return nil, ErrInvalidFormat
	}
	return b, nil
}
