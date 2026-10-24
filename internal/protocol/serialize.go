package protocol

import (
	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// Serialize returns the record in the stored form LoadSessionRecord reads
// (libsignal's layout, double-ratchet.md §10).
func (r *SessionRecord) Serialize() ([]byte, error) {
	var pb signalpb.StoredSessionRecord
	if r.current != nil {
		pb.Current = r.current.stored()
	}
	for _, s := range r.previous {
		b, err := proto.Marshal(s.stored())
		if err != nil {
			return nil, err
		}
		pb.Previous = append(pb.Previous, b)
	}
	return proto.Marshal(&pb)
}

func storedChainKey(c chain) *signalpb.StoredSession_Chain_ChainKey {
	return &signalpb.StoredSession_Chain_ChainKey{Index: c.index, Key: c.key}
}

func (s *SessionState) stored() *signalpb.StoredSession {
	ps := &signalpb.StoredSession{
		Version:              uint32(s.version),
		RootKey:              s.rootKey,
		PreviousCounter:      s.previousCounter,
		AliceBaseKey:         s.aliceBaseKey,
		LocalRegistrationId:  s.localRegistrationID,
		RemoteRegistrationId: s.remoteRegistrationID,
		PqRatchetState:       s.pqRaw,
	}
	if s.pq != nil {
		ps.PqRatchetState = s.pq.Export()
	}
	if s.localIdentity != nil {
		ps.LocalIdentity = s.localIdentity.Bytes()
	}
	if s.remoteIdentity != nil {
		ps.RemoteIdentity = s.remoteIdentity.Bytes()
	}
	if s.hasSenderChain {
		sc := &signalpb.StoredSession_Chain{ChainKey: storedChainKey(s.sendChain)}
		if s.senderRatchet != nil {
			sc.RatchetKey = s.senderRatchet.PublicKey().Bytes()
			sc.RatchetKeyPrivate = s.senderRatchet.Bytes()
		}
		ps.SenderChain = sc
	}
	for _, rc := range s.receivers {
		c := &signalpb.StoredSession_Chain{RatchetKey: rc.ratchetKey.Bytes(), ChainKey: storedChainKey(rc.chain)}
		// Stored newest first, as libsignal keeps them.
		for i := len(rc.skipped) - 1; i >= 0; i-- {
			sk := rc.skipped[i]
			mk := &signalpb.StoredSession_Chain_MessageKey{Index: sk.index, Seed: sk.seed}
			if sk.legacy != nil {
				mk.CipherKey, mk.MacKey, mk.Iv = sk.legacy.cipherKey, sk.legacy.macKey, sk.legacy.iv
			}
			c.MessageKeys = append(c.MessageKeys, mk)
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
	return ps
}

// Serialize stores a SenderKeyRecord in the form LoadSenderKeyRecord
// reads.
func (r *SenderKeyRecord) Serialize() ([]byte, error) {
	var pb signalpb.StoredSenderKeyRecord
	for _, s := range r.states {
		ps := &signalpb.StoredSenderKeyState{
			ChainId:        s.chainID,
			ChainKey:       &signalpb.StoredSenderKeyState_ChainKey{Iteration: s.iteration, Seed: s.chainKey},
			SigningKey:     &signalpb.StoredSenderKeyState_SigningKey{PublicKey: s.signingPub.Bytes()},
			MessageVersion: uint32(s.version),
		}
		if s.signingPriv != nil {
			ps.SigningKey.PrivateKey = s.signingPriv.Bytes()
		}
		for _, k := range s.skipped {
			ps.MessageKeys = append(ps.MessageKeys, &signalpb.StoredSenderKeyState_ChainKey{Iteration: k.index, Seed: k.seed})
		}
		pb.States = append(pb.States, ps)
	}
	return proto.Marshal(&pb)
}
