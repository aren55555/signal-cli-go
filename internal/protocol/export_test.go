package protocol

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// Test-only writers for libsignal's storage format, the inverse of
// load.go, so tests can check that loading preserves live state.

func exportSession(s *SessionState) *signalpb.StoredSession {
	ps := &signalpb.StoredSession{
		Version:              uint32(s.version),
		RootKey:              s.rootKey,
		PreviousCounter:      s.previousCounter,
		AliceBaseKey:         s.aliceBaseKey,
		LocalRegistrationId:  s.localRegistrationID,
		RemoteRegistrationId: s.remoteRegistrationID,
	}
	if s.localIdentity != nil {
		ps.LocalIdentity = s.localIdentity.Bytes()
	}
	if s.remoteIdentity != nil {
		ps.RemoteIdentity = s.remoteIdentity.Bytes()
	}
	if s.hasSenderChain {
		sc := &signalpb.StoredSession_Chain{ChainKey: &signalpb.StoredSession_Chain_ChainKey{Index: s.sendChain.index, Key: s.sendChain.key}}
		if s.senderRatchet != nil {
			sc.RatchetKey = s.senderRatchet.PublicKey().Bytes()
			sc.RatchetKeyPrivate = s.senderRatchet.Bytes()
		}
		ps.SenderChain = sc
	}
	for _, rc := range s.receivers {
		c := &signalpb.StoredSession_Chain{
			RatchetKey: rc.ratchetKey.Bytes(),
			ChainKey:   &signalpb.StoredSession_Chain_ChainKey{Index: rc.chain.index, Key: rc.chain.key},
		}
		for i := len(rc.skipped) - 1; i >= 0; i-- { // newest first
			c.MessageKeys = append(c.MessageKeys, &signalpb.StoredSession_Chain_MessageKey{Index: rc.skipped[i].index, Seed: rc.skipped[i].seed})
		}
		ps.ReceiverChains = append(ps.ReceiverChains, c)
	}
	if p := s.pending; p != nil {
		ps.PendingPreKey = &signalpb.StoredSession_PendingPreKey{
			OneTimePreKeyId: p.oneTimePreKeyID,
			BaseKey:         p.baseKey.Bytes(),
			SignedPreKeyId:  int32(p.signedPreKeyID),
			Timestamp:       uint64(p.created.Unix()),
		}
	}
	if k := s.pendingKyber; k != nil {
		ps.PendingKyberPreKey = &signalpb.StoredSession_PendingKyberPreKey{KyberPreKeyId: k.kyberPreKeyID, Ciphertext: k.ciphertext}
	}
	if s.pq != nil {
		ps.PqRatchetState = s.pq.Export()
	}
	return ps
}

func exportRecordForTest(t *testing.T, r *SessionRecord) []byte {
	t.Helper()
	pb := new(signalpb.StoredSessionRecord)
	if r.current != nil {
		pb.Current = exportSession(r.current)
	}
	for _, s := range r.previous {
		b, err := proto.Marshal(exportSession(s))
		if err != nil {
			t.Fatal(err)
		}
		pb.Previous = append(pb.Previous, b)
	}
	b, err := proto.Marshal(pb)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func exportSenderKeyForTest(t *testing.T, r *SenderKeyRecord) []byte {
	t.Helper()
	pb := new(signalpb.StoredSenderKeyRecord)
	for _, s := range r.states {
		ps := &signalpb.StoredSenderKeyState{
			MessageVersion: uint32(s.version),
			ChainId:        s.chainID,
			ChainKey:       &signalpb.StoredSenderKeyState_ChainKey{Iteration: s.iteration, Seed: s.chainKey},
			SigningKey:     &signalpb.StoredSenderKeyState_SigningKey{PublicKey: s.signingPub.Bytes()},
		}
		if s.signingPriv != nil {
			ps.SigningKey.PrivateKey = s.signingPriv.Bytes()
		}
		for _, sk := range s.skipped {
			ps.MessageKeys = append(ps.MessageKeys, &signalpb.StoredSenderKeyState_ChainKey{Iteration: sk.index, Seed: sk.seed})
		}
		pb.States = append(pb.States, ps)
	}
	b, err := proto.Marshal(pb)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
