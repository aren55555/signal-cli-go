package manager

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// TestTrustSyncsVerified: trust -a on a multi-device account sends B an
// unsealed null message, then a verified sync with the same timestamp and
// null message bytes (sync.md §6.5).
func TestTrustSyncsVerified(t *testing.T) {
	fc, s := dataWorld(t)
	// A session with B first, so its identity is stored.
	if _, res := s.SendData(context.Background(), []string{dataBACI}, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("x")}}); res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	ident := s.Acct.Identities[dataBACI]
	ident.Trust = store.Untrusted
	s.Acct.Identities[dataBACI] = ident
	fc.delivered = nil

	if err := s.TrustAllKnownKeys(context.Background(), dataBACI); err != nil {
		t.Fatal(err)
	}
	if s.Acct.Identities[dataBACI].Trust != store.TrustedUnverified {
		t.Errorf("trust %v", s.Acct.Identities[dataBACI].Trust)
	}
	toB, toSelf := deliveredTo(t, fc, dataBACI), deliveredTo(t, fc, testACI)
	if len(toB) != 1 || toB[0].GetNullMessage() == nil || len(toSelf) != 1 {
		t.Fatalf("delivered %d to B, %d to self", len(toB), len(toSelf))
	}
	v := toSelf[0].GetSyncMessage().GetVerified()
	nm, _ := proto.Marshal(toB[0].GetNullMessage())
	bin, _ := uuidRaw(dataBACI)
	if v.GetState() != signalpb.AppVerified_DEFAULT || !bytes.Equal(v.GetIdentityKey(), ident.Key) || !bytes.Equal(v.GetDestinationAciBinary(), bin[:]) || !bytes.Equal(v.GetNullMessage(), nm) {
		t.Errorf("verified %v", v)
	}
	if fc.delivered[0].msg.Sealed {
		t.Error("null message was sealed")
	}

	// -v with the matching key: VERIFIED; with another key: an error.
	if err := s.TrustVerifiedKey(context.Background(), dataBACI, ident.Key); err != nil || s.Acct.Identities[dataBACI].Trust != store.TrustedVerified {
		t.Errorf("verify: %v", err)
	}
	if err := s.TrustVerifiedKey(context.Background(), dataBACI, make([]byte, 33)); !errors.Is(err, ErrTrustNotChanged) {
		t.Errorf("wrong key: %v", err)
	}
	if err := s.TrustAllKnownKeys(context.Background(), "c0c0c0c0-0000-4000-8000-000000000003"); !errors.Is(err, ErrTrustNotChanged) {
		t.Errorf("unknown recipient: %v", err)
	}
}
