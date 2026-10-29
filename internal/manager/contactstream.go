// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// The contact stream (spec/service/sync.md §5.1): varint-length-prefixed
// ContactDetails records, each followed by its avatar bytes.

// ContactRecord is one record with its avatar data.
type ContactRecord struct {
	Details *signalpb.AppContactDetails
	Avatar  []byte
}

// EncodeContacts writes the stream.
func EncodeContacts(records []ContactRecord) ([]byte, error) {
	var out []byte
	for _, r := range records {
		b, err := proto.Marshal(r.Details)
		if err != nil {
			return nil, err
		}
		out = binary.AppendUvarint(out, uint64(len(b)))
		out = append(out, b...)
		if av := r.Details.GetAvatar(); av != nil && av.Length != nil {
			out = append(out, r.Avatar...)
		}
	}
	return out, nil
}

// errMissingAddress is the record-level rejection of §5.1's quirk.
var errMissingAddress = errors.New("Missing contact address!")

// DecodeContacts reads the stream. Records rejected for a missing address
// are skipped; any other error stops the import, keeping what was read.
func DecodeContacts(data []byte) ([]ContactRecord, error) {
	var out []ContactRecord
	for len(data) > 0 {
		n, k := binary.Uvarint(data)
		if k <= 0 || k > 5 {
			return out, errors.New("malformed length")
		}
		data = data[k:]
		if uint64(len(data)) < n {
			return out, io.ErrUnexpectedEOF
		}
		var d signalpb.AppContactDetails
		if err := proto.Unmarshal(data[:n], &d); err != nil {
			return out, err
		}
		data = data[n:]
		var avatar []byte
		if av := d.GetAvatar(); av != nil && av.Length != nil {
			if uint64(len(data)) < uint64(av.GetLength()) {
				return out, io.ErrUnexpectedEOF
			}
			avatar, data = data[:av.GetLength()], data[av.GetLength():]
		}
		// The string aci and the number are required, even with aciBinary.
		if d.Aci == nil && d.Number == nil {
			continue
		}
		out = append(out, ContactRecord{Details: &d, Avatar: avatar})
	}
	return out, nil
}
