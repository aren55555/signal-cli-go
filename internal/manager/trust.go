package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Trusting identity keys (spec/surface/cli/trust.md) and the verified
// sync that follows on a multi-device account (sync.md §6.5).

// ErrTrustNotChanged: no stored identity could be updated.
var ErrTrustNotChanged = errors.New("trust not changed")

// identityIDs are the identity store keys of a recipient: its ACI and, if
// known, its PNI.
func identityIDs(a *store.Account, id string) []string {
	ids := []string{id}
	if pni := a.Recipients[id].PNI; pni != "" {
		ids = append(ids, "PNI:"+pni)
	}
	return ids
}

// TrustAllKnownKeys marks every known identity key of a recipient trusted
// (unverified) and syncs the changes.
func (s *Sender) TrustAllKnownKeys(ctx context.Context, id string) error {
	changed := false
	for _, k := range identityIDs(s.Acct, id) {
		ident, ok := s.Acct.Identities[k]
		if !ok {
			continue
		}
		changed = true
		if ident.Trust == store.TrustedUnverified {
			continue
		}
		ident.Trust = store.TrustedUnverified
		s.Acct.Identities[k] = ident
		if err := s.Acct.SaveIdentities(); err != nil {
			return err
		}
		s.syncVerified(ctx, k, ident)
	}
	if !changed {
		return ErrTrustNotChanged
	}
	return nil
}

// TrustVerifiedKey marks the recipient's key verified if it equals key
// (the 33-byte legacy fingerprint).
func (s *Sender) TrustVerifiedKey(ctx context.Context, id string, key []byte) error {
	for _, k := range identityIDs(s.Acct, id) {
		ident, ok := s.Acct.Identities[k]
		if !ok || !bytes.Equal(ident.Key, key) {
			continue
		}
		if ident.Trust != store.TrustedVerified {
			ident.Trust = store.TrustedVerified
			s.Acct.Identities[k] = ident
			if err := s.Acct.SaveIdentities(); err != nil {
				return err
			}
			s.syncVerified(ctx, k, ident)
		}
		return nil
	}
	return ErrTrustNotChanged
}

// syncVerified is §6.5: a null message to the contact (unsealed, now),
// then, if it went out and needs sync, the verified sync with the same
// timestamp and null message. Failures are only logged by callers'
// absence of output; nothing is reported.
func (s *Sender) syncVerified(ctx context.Context, id string, ident store.Identity) {
	if !s.Acct.IsMultiDevice() {
		return
	}
	nm, err := s.nullMessage()
	if err != nil {
		return
	}
	nmBytes, _ := proto.Marshal(nm)
	c, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_NullMessage{NullMessage: nm}})
	ts := uint64(s.now().UnixMilli())
	dest := id
	needsSync, err := s.sendUnsealed(ctx, dest, c, ts)
	if err != nil || !(needsSync || s.Acct.IsMultiDevice()) {
		return
	}
	bin, _ := protocol.ServiceIDToBinary(id)
	state := signalpb.AppVerified_DEFAULT
	switch ident.Trust {
	case store.TrustedVerified:
		state = signalpb.AppVerified_VERIFIED
	case store.Untrusted:
		state = signalpb.AppVerified_UNVERIFIED
	}
	s.sendSync(ctx, &signalpb.AppSyncMessage{Verified: &signalpb.AppVerified{IdentityKey: ident.Key, DestinationAciBinary: bin,
		State: state.Enum(), NullMessage: nmBytes}}, ts, false)
}

// nullMessage is content.md §6's NullMessage: padding is a serialized
// DataMessage with a base64 body of 1..140 random bytes.
func (s *Sender) nullMessage() (*signalpb.AppNullMessage, error) {
	var n [1]byte
	if _, err := io.ReadFull(s.rand(), n[:]); err != nil {
		return nil, err
	}
	filler := make([]byte, 1+int(n[0])%140)
	if _, err := io.ReadFull(s.rand(), filler); err != nil {
		return nil, err
	}
	padding, err := proto.Marshal(&signalpb.AppDataMessage{Body: proto.String(base64.StdEncoding.EncodeToString(filler))})
	if err != nil {
		return nil, err
	}
	return &signalpb.AppNullMessage{Padding: padding}, nil
}
