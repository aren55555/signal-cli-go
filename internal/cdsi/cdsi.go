// SPDX-License-Identifier: GPL-3.0-or-later

// Package cdsi encodes CDSI requests and parses responses
// (spec/service/cdsi.md §4, §5) and runs the lookup over an attested
// enclave connection.
package cdsi

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

// ErrInvalidNumber is a number string libsignal rejects (§5.2).
var ErrInvalidNumber = errors.New("cdsi: invalid number")

// ParseE164 converts a number string to its 64-bit value (§5.2).
func ParseE164(s string) (uint64, error) {
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "+") // Rust's parser accepts one more '+'
	if s == "" {
		return 0, ErrInvalidNumber
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v == 0 {
		return 0, ErrInvalidNumber
	}
	return v, nil
}

// Pair is one ACI with the unidentified access key of its profile key.
type Pair struct {
	ACI       [16]byte
	AccessKey [16]byte
}

// Request is a ClientRequest (§4.1).
type Request struct {
	Pairs    []Pair
	Prev     []string
	New      []string
	Token    []byte
	TokenAck bool
}

func e164s(list []string) ([]byte, error) {
	var out []byte
	for _, s := range list {
		v, err := ParseE164(s)
		if err != nil {
			return nil, err
		}
		out = binary.BigEndian.AppendUint64(out, v)
	}
	return out, nil
}

// Encode serializes the request with empty fields omitted.
func (r *Request) Encode() ([]byte, error) {
	var b []byte
	bytesField := func(n protowire.Number, v []byte) {
		if len(v) > 0 {
			b = protowire.AppendTag(b, n, protowire.BytesType)
			b = protowire.AppendBytes(b, v)
		}
	}
	var pairs []byte
	for _, p := range r.Pairs {
		pairs = append(append(pairs, p.ACI[:]...), p.AccessKey[:]...)
	}
	prev, err := e164s(r.Prev)
	if err != nil {
		return nil, err
	}
	nw, err := e164s(r.New)
	if err != nil {
		return nil, err
	}
	bytesField(1, pairs)
	bytesField(2, prev)
	bytesField(3, nw)
	bytesField(6, r.Token)
	if r.TokenAck {
		b = protowire.AppendTag(b, 7, protowire.VarintType)
		b = protowire.AppendVarint(b, 1)
	}
	return b, nil
}

// Response is a ClientResponse (§4.2).
type Response struct {
	Triples     []byte
	Token       []byte
	PermitsUsed int32
}

// ParseResponse decodes a ClientResponse.
func ParseResponse(b []byte) (*Response, error) {
	r := &Response{}
	if err := r.merge(b); err != nil {
		return nil, err
	}
	return r, nil
}

// merge applies protobuf merge semantics (later non-empty bytes win).
func (r *Response) merge(b []byte) error {
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return errors.New("cdsi: bad response")
		}
		b = b[l:]
		vl := protowire.ConsumeFieldValue(n, typ, b)
		if vl < 0 {
			return errors.New("cdsi: bad response")
		}
		switch {
		case n == 1 && typ == protowire.BytesType:
			v, _ := protowire.ConsumeBytes(b)
			r.Triples = append([]byte{}, v...)
		case n == 3 && typ == protowire.BytesType:
			v, _ := protowire.ConsumeBytes(b)
			r.Token = append([]byte{}, v...)
		case n == 4 && typ == protowire.VarintType:
			v, _ := protowire.ConsumeVarint(b)
			r.PermitsUsed = int32(v)
		}
		b = b[vl:]
	}
	return nil
}

// Entry is one result: either service ID may be empty.
type Entry struct{ ACI, PNI string }

func uuidOf(b []byte) string {
	for _, c := range b {
		if c != 0 {
			h := hex.EncodeToString(b)
			return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
		}
	}
	return ""
}

// Results parses the triples into number → entry (§4.4).
func (r *Response) Results() (map[string]Entry, error) {
	if len(r.Triples)%40 != 0 {
		return nil, fmt.Errorf("CDS protocol: could not parse response triples (%d bytes)", len(r.Triples))
	}
	out := map[string]Entry{}
	for t := r.Triples; len(t) > 0; t = t[40:] {
		num := binary.BigEndian.Uint64(t)
		if num == 0 {
			continue
		}
		out["+"+strconv.FormatUint(num, 10)] = Entry{PNI: uuidOf(t[8:24]), ACI: uuidOf(t[24:40])}
	}
	return out, nil
}

// CloseError is a CDSI close other than 1000 (§4.5).
type CloseError struct {
	Code       int
	Reason     string
	RetryAfter uint32 // 4008 only
}

func (e *CloseError) Error() string {
	switch e.Code {
	case 4013:
		return "Server error: ServerInternalError"
	case 4014:
		return "Server error: ServerUnavailable"
	}
	return fmt.Sprintf("StatusCode: %d", e.Code)
}

// IsRateLimit, IsInvalidToken and IsInvalidArgument classify errors.
func (e *CloseError) IsRateLimit() bool       { return e.Code == 4008 }
func (e *CloseError) IsInvalidToken() bool    { return e.Code == 4101 }
func (e *CloseError) IsInvalidArgument() bool { return e.Code == 4003 }

// ErrProtocol is an unexpected close or malformed exchange.
var ErrProtocol = errors.New("Protocol error after establishing a connection")

// ClassifyClose maps a close code and reason (§4.5); nil for 1000.
func ClassifyClose(code int, reason string) error {
	switch code {
	case 1000, 0:
		return nil
	case 4003, 4101, 4013, 4014:
		return &CloseError{Code: code, Reason: reason}
	case 4008:
		var r struct {
			RetryAfter *uint32 `json:"retry_after"`
		}
		if json.Unmarshal([]byte(reason), &r) != nil || r.RetryAfter == nil {
			return ErrProtocol
		}
		return &CloseError{Code: 4008, Reason: reason, RetryAfter: *r.RetryAfter}
	}
	return ErrProtocol
}

// Sorted returns a sorted copy (request entry order carries no meaning;
// sorting keeps requests deterministic).
func Sorted(list []string) []string {
	out := append([]string{}, list...)
	sort.Strings(out)
	return out
}
