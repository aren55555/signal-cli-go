// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"errors"
	"os"
	"slices"
)

// The message send log (spec/service/send-log.md §1): what was sent to
// which devices, kept for a day to answer retry receipts. It lives in
// sendlog.json and is read on first use.

const sendLogVersion = 1

// SendLogRecipient is one device an entry was encrypted for.
type SendLogRecipient struct {
	Address string `json:"address"` // ACI, or "PNI:" + UUID
	Device  uint32 `json:"device"`
}

// SendLogEntry is one logged Content.
type SendLogEntry struct {
	ID         int64              `json:"id"`
	Timestamp  uint64             `json:"timestamp"`
	Content    []byte             `json:"content"`
	Hint       uint32             `json:"contentHint"`
	Urgent     bool               `json:"urgent"`
	GroupID    []byte             `json:"groupId,omitempty"`
	Recipients []SendLogRecipient `json:"recipients"`
}

type sendLogDoc struct {
	Version int            `json:"version"`
	NextID  int64          `json:"nextId"`
	Entries []SendLogEntry `json:"entries"`
}

func (a *Account) sendLogDoc() (*sendLogDoc, error) {
	if a.sendLog != nil {
		return a.sendLog, nil
	}
	d := &sendLogDoc{}
	if err := ReadJSON(a.file("sendlog.json"), sendLogVersion, Migrations{}, d); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	a.sendLog = d
	return d, nil
}

func (a *Account) saveSendLog() error {
	a.sendLog.Version = sendLogVersion
	return WriteJSON(a.file("sendlog.json"), a.sendLog)
}

// SendLogAdd stores a new entry and returns its id.
func (a *Account) SendLogAdd(e SendLogEntry) (int64, error) {
	d, err := a.sendLogDoc()
	if err != nil {
		return 0, err
	}
	d.NextID++
	e.ID = d.NextID
	d.Entries = append(d.Entries, e)
	return e.ID, a.saveSendLog()
}

// SendLogAppend adds recipient rows to entry id (a group send's later
// results, §1.3).
func (a *Account) SendLogAppend(id int64, rs []SendLogRecipient) error {
	d, err := a.sendLogDoc()
	if err != nil {
		return err
	}
	for i := range d.Entries {
		if d.Entries[i].ID == id {
			d.Entries[i].Recipients = append(d.Entries[i].Recipients, rs...)
			return a.saveSendLog()
		}
	}
	return nil
}

// SendLogPrune deletes entries whose timestamp is older than cutoff (ms,
// §3.3); it returns how many went.
func (a *Account) SendLogPrune(cutoff uint64) (int, error) {
	d, err := a.sendLogDoc()
	if err != nil {
		return 0, err
	}
	n := len(d.Entries)
	d.Entries = slices.DeleteFunc(d.Entries, func(e SendLogEntry) bool { return e.Timestamp < cutoff })
	if removed := n - len(d.Entries); removed > 0 {
		return removed, a.saveSendLog()
	}
	return 0, nil
}

// SendLogDelivered deletes the rows of (address, device) for entries
// with these timestamps, then every entry left without rows (§3.1).
func (a *Account) SendLogDelivered(address string, device uint32, timestamps []uint64) error {
	d, err := a.sendLogDoc()
	if err != nil {
		return err
	}
	changed := false
	for i := range d.Entries {
		e := &d.Entries[i]
		if !slices.Contains(timestamps, e.Timestamp) {
			continue
		}
		n := len(e.Recipients)
		e.Recipients = slices.DeleteFunc(e.Recipients, func(r SendLogRecipient) bool { return r.Address == address && r.Device == device })
		changed = changed || len(e.Recipients) != n
	}
	if !changed {
		return nil
	}
	d.Entries = slices.DeleteFunc(d.Entries, func(e SendLogEntry) bool { return len(e.Recipients) == 0 })
	return a.saveSendLog()
}

// SendLogRemoteDelete removes the entries a remote delete targets
// (§3.2): with groupID, those of that group at timestamp; else 1:1
// entries at timestamp with a row for address (then empty entries).
func (a *Account) SendLogRemoteDelete(timestamp uint64, address string, groupID []byte) error {
	d, err := a.sendLogDoc()
	if err != nil {
		return err
	}
	n := len(d.Entries)
	d.Entries = slices.DeleteFunc(d.Entries, func(e SendLogEntry) bool {
		if e.Timestamp != timestamp {
			return false
		}
		if groupID != nil {
			return bytes.Equal(e.GroupID, groupID)
		}
		return e.GroupID == nil && slices.ContainsFunc(e.Recipients, func(r SendLogRecipient) bool { return r.Address == address })
	})
	if groupID == nil {
		d.Entries = slices.DeleteFunc(d.Entries, func(e SendLogEntry) bool { return len(e.Recipients) == 0 })
	}
	if len(d.Entries) == n {
		return nil
	}
	return a.saveSendLog()
}

// SendLogLookup returns the entries sent to (address, device) at
// timestamp; groupsOnly keeps only entries with a group id (§4.1).
func (a *Account) SendLogLookup(address string, device uint32, timestamp uint64, groupsOnly bool) ([]SendLogEntry, error) {
	d, err := a.sendLogDoc()
	if err != nil {
		return nil, err
	}
	var out []SendLogEntry
	for _, e := range d.Entries {
		if e.Timestamp != timestamp || (groupsOnly && e.GroupID == nil) {
			continue
		}
		if slices.ContainsFunc(e.Recipients, func(r SendLogRecipient) bool { return r.Address == address && r.Device == device }) {
			out = append(out, e)
		}
	}
	return out, nil
}
