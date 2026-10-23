package spqr

import (
	"encoding/binary"
	"sort"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// Export serializes the state in libsignal's PqRatchetState format
// (spec/protocol/spqr/state.md), the inverse of Import. A V0 state with
// nothing else exports as empty bytes.
func (s *State) Export() []byte {
	if s.neg == nil && s.chain == nil && s.scka == nil {
		return nil
	}
	pb := new(signalpb.SpqrState)
	if n := s.neg; n != nil {
		pb.Negotiation = &signalpb.SpqrState_Negotiation{
			AuthKey:     n.authKey,
			Direction:   direction(n.a2b),
			MinVersion:  signalpb.SpqrVersion(n.minVersion),
			ChainParams: exportParams(n.params.MaxJump, n.params.MaxOOO),
		}
	}
	if c := s.chain; c != nil {
		pc := &signalpb.SpqrChain{
			Direction:    direction(c.a2b),
			CurrentEpoch: c.currentEpoch,
			NextRoot:     c.nextRoot,
			SendEpoch:    c.sendEpoch,
			Params:       exportParams(c.maxJump, c.maxOOO),
		}
		for _, l := range c.links {
			pc.Links = append(pc.Links, &signalpb.SpqrChain_Link{Send: exportHalf(&l.send), Recv: exportHalf(&l.recv)})
		}
		pb.Chain = pc
	}
	if s.scka != nil {
		pb.Inner = &signalpb.SpqrState_V1{V1: exportSCKA(s.scka)}
	}
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return b
}

func direction(a2b bool) signalpb.SpqrDirection {
	if a2b {
		return signalpb.SpqrDirection_SPQR_A2B
	}
	return signalpb.SpqrDirection_SPQR_B2A
}

// exportParams writes the library defaults as 0, as libsignal does.
func exportParams(maxJump, maxOOO uint32) *signalpb.SpqrChainParams {
	p := &signalpb.SpqrChainParams{MaxJump: maxJump, MaxOooKeys: maxOOO}
	if p.MaxJump == defaultMaxJump {
		p.MaxJump = 0
	}
	if p.MaxOooKeys == defaultMaxOOO {
		p.MaxOooKeys = 0
	}
	return p
}

func exportHalf(h *half) *signalpb.SpqrChain_Link_Half {
	out := &signalpb.SpqrChain_Link_Half{Counter: h.counter, Next: h.next}
	for _, s := range h.store {
		out.Skipped = binary.BigEndian.AppendUint32(out.Skipped, s.index)
		out.Skipped = append(out.Skipped, s.key...)
	}
	return out
}

// exportEncoder writes points until the encoder has produced a parity
// chunk, coefficients after.
func exportEncoder(s *streamEnc) *signalpb.SpqrEncoder {
	out := &signalpb.SpqrEncoder{Next: s.next}
	if s.enc.coeffs == nil && uint64(s.next) <= uint64(len(s.enc.pts[0])) {
		for _, pts := range s.enc.pts {
			var b []byte
			for _, p := range pts {
				b = binary.BigEndian.AppendUint16(b, p.y)
			}
			out.Points = append(out.Points, b)
		}
		return out
	}
	for j := range polys {
		cs := []uint16(nil)
		if s.enc.coeffs != nil {
			cs = s.enc.coeffs[j]
		} else {
			cs = toCoefficients(s.enc.pts[j])
		}
		var b []byte
		for _, c := range cs {
			b = binary.BigEndian.AppendUint16(b, c)
		}
		out.Coefficients = append(out.Coefficients, b)
	}
	return out
}

func exportDecoder(d *decoder) *signalpb.SpqrDecoder {
	out := &signalpb.SpqrDecoder{PointsNeeded: uint32(d.length / 2), Polynomials: polys}
	for _, pts := range d.pts {
		sorted := append([]point(nil), pts...)
		sort.Slice(sorted, func(a, b int) bool { return sorted[a].x < sorted[b].x })
		var b []byte
		for _, p := range sorted {
			b = binary.BigEndian.AppendUint16(b, p.x)
			b = binary.BigEndian.AppendUint16(b, p.y)
		}
		out.Points = append(out.Points, b)
	}
	return out
}

func marshal(m proto.Message) []byte {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

func exportSCKA(s *scka) *signalpb.SpqrV1 {
	c := &signalpb.SpqrCore{Epoch: s.epoch, Auth: &signalpb.SpqrAuthenticator{RootKey: s.auth.rootKey, MacKey: s.auth.macKey}}
	ed := &signalpb.SpqrChunkedEncDec{Core: c}
	d := &signalpb.SpqrChunkedDec{Core: c}
	v := new(signalpb.SpqrV1)
	switch s.kind {
	case keysUnsampled:
		v.State = &signalpb.SpqrV1_KeysUnsampled{KeysUnsampled: marshal(ed)}
	case keysSampled:
		c.F3, c.F4 = s.ekPayload, s.dk
		ed.Encoder = exportEncoder(s.hdrEnc)
		v.State = &signalpb.SpqrV1_KeysSampled{KeysSampled: marshal(ed)}
	case headerSent:
		c.F3 = s.dk
		ed.Encoder, ed.Decoder = exportEncoder(s.ekEnc), exportDecoder(s.ct1Dec)
		v.State = &signalpb.SpqrV1_HeaderSent{HeaderSent: marshal(ed)}
	case ct1Received:
		c.F3, c.F4 = s.dk, s.ct1
		ed.Encoder = exportEncoder(s.ekEnc)
		v.State = &signalpb.SpqrV1_Ct1Received{Ct1Received: marshal(ed)}
	case ekSentCt1Received:
		c.F3, c.F4 = s.dk, s.ct1
		ed.Decoder = exportDecoder(s.ct2Dec)
		v.State = &signalpb.SpqrV1_EkSentCt1Received{EkSentCt1Received: marshal(ed)}
	case noHeaderReceived:
		d.Decoder = exportDecoder(s.hdrDec)
		v.State = &signalpb.SpqrV1_NoHeaderReceived{NoHeaderReceived: marshal(d)}
	case headerReceived:
		c.F3 = s.hdr
		d.Decoder = exportDecoder(s.ekDec)
		v.State = &signalpb.SpqrV1_HeaderReceived{HeaderReceived: marshal(d)}
	case ct1Sampled:
		c.F3, c.F4, c.F5 = s.hdr, s.enc.State(), s.ct1
		ed.Encoder, ed.Decoder = exportEncoder(s.ct1Enc), exportDecoder(s.ekDec)
		v.State = &signalpb.SpqrV1_Ct1Sampled{Ct1Sampled: marshal(ed)}
	case ekReceivedCt1Sampled:
		c.F3, c.F4, c.F5 = s.enc.State(), s.ekPayload, s.ct1
		ed.Encoder = exportEncoder(s.ct1Enc)
		v.State = &signalpb.SpqrV1_EkReceivedCt1Sampled{EkReceivedCt1Sampled: marshal(ed)}
	case ct1Acknowledged:
		c.F3, c.F4, c.F5 = s.hdr, s.enc.State(), s.ct1
		d.Decoder = exportDecoder(s.ekDec)
		v.State = &signalpb.SpqrV1_Ct1Acknowledged{Ct1Acknowledged: marshal(d)}
	case ct2Sampled:
		ed.Encoder = exportEncoder(s.ct2Enc)
		v.State = &signalpb.SpqrV1_Ct2Sampled{Ct2Sampled: marshal(ed)}
	}
	return v
}
