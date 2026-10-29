// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"io"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// Initiate runs PQXDH against a bundle and installs the result as the
// current state, archiving the old one (libsignal's processing of a
// fetched bundle).
func (r *SessionRecord) Initiate(rand io.Reader, identity *curve25519.PrivateKey, localRegistrationID uint32, b *PreKeyBundle, now time.Time, local, remote Address) (*SessionRecord, error) {
	n, err := NewInitiatorRecord(rand, identity, localRegistrationID, b, now, local, remote)
	if err != nil {
		return nil, err
	}
	return r.withCurrent(n.current), nil
}

// ArchiveCurrent moves the current state to the archive, leaving no
// current state (send.md §5, extra and stale devices).
func (r *SessionRecord) ArchiveCurrent() *SessionRecord {
	if r.current == nil {
		return r
	}
	n := r.withCurrent(nil)
	return n
}

// HasCurrent reports whether the record has a current state.
func (r *SessionRecord) HasCurrent() bool { return r.current != nil }

// RemoteRegistrationID is the peer's registration ID in the current
// state, or 0.
func (r *SessionRecord) RemoteRegistrationID() uint32 {
	if r.current == nil {
		return 0
	}
	return r.current.remoteRegistrationID
}

// RemoteIdentity is the peer's identity key in the current state, or nil.
func (r *SessionRecord) RemoteIdentity() *curve25519.PublicKey {
	if r.current == nil {
		return nil
	}
	return r.current.remoteIdentity
}

// Unacknowledged reports whether the current state still sends PreKey
// messages (the peer has not replied on it yet).
func (r *SessionRecord) Unacknowledged() bool { return r.current != nil && r.current.pending != nil }

// SenderRatchetKey returns the 33-byte sender ratchet key of a
// SignalMessage (messageType 2) or of the SignalMessage inside a
// PreKeySignalMessage (3), for a retry receipt (pipe.md §7.2); nil if the
// message does not parse.
func SenderRatchetKey(wire []byte, messageType int) []byte {
	switch messageType {
	case MessageTypePreKey:
		pm, err := parsePreKeyMessage(wire)
		if err != nil || pm.inner == nil {
			return nil
		}
		return pm.inner.ratchetKey.Bytes()
	case MessageTypeWhisper:
		m, err := parseRatchetMessage(wire)
		if err != nil {
			return nil
		}
		return m.ratchetKey.Bytes()
	}
	return nil
}

// SenderRatchetKey returns the 33-byte public sender ratchet key of the
// current session, nil without one (to match a retry receipt's
// ratchet_key, send-log.md §4.3).
func (r *SessionRecord) SenderRatchetKey() []byte {
	if r.current == nil || r.current.senderRatchet == nil {
		return nil
	}
	return r.current.senderRatchet.PublicKey().Bytes()
}
