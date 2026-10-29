// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"os"
)

// StorageIdent is one manifest identifier: a raw storage ID and its
// record type (storage.md §4.2).
type StorageIdent struct {
	Raw  []byte `json:"raw"`
	Type int32  `json:"type"`
}

// StorageState is the local storage-service bookkeeping (storage.md
// §6.2): the last manifest (unencrypted) and the IDs of records we do
// not process.
type StorageState struct {
	Version    int            `json:"version"`
	Manifest   int64          `json:"manifestVersion"` // -1 when none
	IDs        []StorageIdent `json:"ids,omitempty"`
	RecordIkm  []byte         `json:"recordIkm,omitempty"`
	UnknownIDs []StorageIdent `json:"unknownIds,omitempty"`
}

// LoadStorage reads storage.json; a missing file is version -1.
func (a *Account) LoadStorage() (*StorageState, error) {
	s := &StorageState{Manifest: -1}
	if err := ReadJSON(a.file("storage.json"), 1, Migrations{}, s); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

// SaveStorage writes storage.json.
func (a *Account) SaveStorage(s *StorageState) error {
	s.Version = 1
	return WriteJSON(a.file("storage.json"), s)
}
