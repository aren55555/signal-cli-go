package spqr

import (
	"encoding/binary"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/kyber"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// ErrStateDecode reports stored SPQR state that cannot be read
// (protocol.md §9). libsignal treats it as an invalid session state.
var ErrStateDecode = errors.New("spqr: cannot decode stored state")

func stateErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrStateDecode}, args...)...)
}

// Import reads the PqRatchetState libsignal stores in a session (StoredSession
// field 15), per spec/protocol/spqr/state.md. Empty input is V0.
func Import(b []byte) (*State, error) {
	if len(b) == 0 {
		return &State{}, nil
	}
	var pb signalpb.SpqrState
	if err := proto.Unmarshal(b, &pb); err != nil {
		return nil, stateErr("%v", err)
	}
	s := new(State)
	if n := pb.Negotiation; n != nil {
		if len(n.AuthKey) != 32 {
			return nil, stateErr("auth key is %d bytes", len(n.AuthKey))
		}
		s.neg = &negotiation{
			authKey:    n.AuthKey,
			a2b:        n.Direction == signalpb.SpqrDirection_SPQR_A2B,
			minVersion: int(n.MinVersion),
			params:     importParams(n.ChainParams),
		}
	}
	if pb.Chain != nil {
		c, err := importChain(pb.Chain)
		if err != nil {
			return nil, err
		}
		s.chain = c
	}
	if v1 := pb.GetV1(); v1 != nil {
		sc, err := importSCKA(v1)
		if err != nil {
			return nil, err
		}
		s.scka = sc
	}
	return s, nil
}

func importParams(p *signalpb.SpqrChainParams) Params {
	if p == nil {
		return Params{}
	}
	return Params{MaxJump: p.MaxJump, MaxOOO: p.MaxOooKeys}
}

func importChain(pc *signalpb.SpqrChain) (*chain, error) {
	if pc.Params == nil {
		return nil, stateErr("chain without parameters")
	}
	if len(pc.NextRoot) != 32 {
		return nil, stateErr("next_root is %d bytes", len(pc.NextRoot))
	}
	if uint64(len(pc.Links)) > pc.CurrentEpoch+1 || len(pc.Links) == 0 {
		return nil, stateErr("%d links at epoch %d", len(pc.Links), pc.CurrentEpoch)
	}
	p := importParams(pc.Params)
	c := newChainParams(pc.Direction == signalpb.SpqrDirection_SPQR_A2B, p.MaxJump, p.MaxOOO)
	c.currentEpoch, c.sendEpoch, c.nextRoot = pc.CurrentEpoch, pc.SendEpoch, pc.NextRoot
	// Links are oldest first and end at the current epoch.
	first := pc.CurrentEpoch + 1 - uint64(len(pc.Links))
	for i, l := range pc.Links {
		if l.Send == nil || l.Recv == nil {
			return nil, stateErr("link without both halves")
		}
		send, err := importHalf(l.Send)
		if err != nil {
			return nil, err
		}
		recv, err := importHalf(l.Recv)
		if err != nil {
			return nil, err
		}
		c.links = append(c.links, &link{epoch: first + uint64(i), send: send, recv: recv})
	}
	return c, nil
}

func importHalf(h *signalpb.SpqrChain_Link_Half) (half, error) {
	out := half{counter: h.Counter}
	switch len(h.Next) {
	case 0:
	case 32:
		out.next = h.Next
	default:
		return half{}, stateErr("chain next is %d bytes", len(h.Next))
	}
	if len(h.Skipped)%36 != 0 {
		return half{}, stateErr("skipped-key store is %d bytes", len(h.Skipped))
	}
	for i := 0; i < len(h.Skipped); i += 36 {
		out.store = append(out.store, skipped{binary.BigEndian.Uint32(h.Skipped[i:]), h.Skipped[i+4 : i+36]})
	}
	return out, nil
}

func importEncoder(pe *signalpb.SpqrEncoder, length int) (*streamEnc, error) {
	if pe == nil {
		return nil, stateErr("missing encoder")
	}
	e := new(encoder)
	switch {
	case len(pe.Points) == polys && len(pe.Coefficients) == 0:
		need := pointCounts(length / 2)
		for j, raw := range pe.Points {
			if len(raw) != 2*need[j] {
				return nil, stateErr("encoder polynomial %d has %d bytes", j, len(raw))
			}
			for x := 0; x < need[j]; x++ {
				e.pts[j] = append(e.pts[j], point{uint16(x), binary.BigEndian.Uint16(raw[2*x:])})
			}
		}
	case len(pe.Coefficients) == polys && len(pe.Points) == 0:
		e.coeffs = new([polys][]uint16)
		for j, raw := range pe.Coefficients {
			if len(raw)%2 != 0 {
				return nil, stateErr("odd coefficient bytes")
			}
			for k := 0; k < len(raw); k += 2 {
				e.coeffs[j] = append(e.coeffs[j], binary.BigEndian.Uint16(raw[k:]))
			}
		}
	default:
		return nil, stateErr("encoder needs 16 point lists or 16 coefficient lists")
	}
	return &streamEnc{enc: e, next: pe.Next}, nil
}

func importDecoder(pd *signalpb.SpqrDecoder, length int) (*decoder, error) {
	if pd == nil {
		return nil, stateErr("missing decoder")
	}
	if int(pd.PointsNeeded) != length/2 {
		// libsignal reports this one as a message decode error.
		return nil, fmt.Errorf("%w: decoder for %d words, want %d", ErrMsgDecode, pd.PointsNeeded, length/2)
	}
	if pd.Complete || len(pd.Points) != polys {
		return nil, stateErr("unusable decoder")
	}
	d := newDec(length)
	for j, raw := range pd.Points {
		if len(raw)%4 != 0 {
			return nil, stateErr("decoder points are %d bytes", len(raw))
		}
		for k := 0; k < len(raw); k += 4 {
			d.pts[j] = append(d.pts[j], point{binary.BigEndian.Uint16(raw[k:]), binary.BigEndian.Uint16(raw[k+2:])})
		}
	}
	return d, nil
}

// core reads the epoch, the authenticator and fields 3-5 with their
// required lengths (0 means the field is unused by that state).
func core(c *signalpb.SpqrCore, len3, len4, len5 int) (*scka, [3][]byte, error) {
	var f [3][]byte
	if c == nil || c.Auth == nil || len(c.Auth.RootKey) != 32 || len(c.Auth.MacKey) != 32 {
		return nil, f, stateErr("state without core data or authenticator")
	}
	for i, want := range []struct {
		b []byte
		n int
	}{{c.F3, len3}, {c.F4, len4}, {c.F5, len5}} {
		if want.n > 0 && len(want.b) != want.n {
			return nil, f, stateErr("core field %d is %d bytes, want %d", i+3, len(want.b), want.n)
		}
		f[i] = want.b
	}
	s := &scka{epoch: c.Epoch, auth: &authenticator{rootKey: c.Auth.RootKey, macKey: c.Auth.MacKey}}
	return s, f, nil
}

const (
	ekLen  = kyber.EKPayloadSizeMLKEM768
	dkLen  = kyber.DecapsulationKeySizeMLKEM768
	hdrLen = kyber.HeaderSizeMLKEM768
	ct1Len = kyber.Ct1SizeMLKEM768
	esLen  = kyber.EncapsulationStateSizeMLKEM768
)

func importSCKA(v1 *signalpb.SpqrV1) (*scka, error) {
	var ed signalpb.SpqrChunkedEncDec
	var d signalpb.SpqrChunkedDec
	encdec := func(b []byte) error { return proto.Unmarshal(b, &ed) }
	dec := func(b []byte) error { return proto.Unmarshal(b, &d) }
	var err error
	var s *scka
	var f [3][]byte

	switch st := v1.State.(type) {
	case *signalpb.SpqrV1_KeysUnsampled:
		if err = encdec(st.KeysUnsampled); err == nil {
			s, _, err = core(ed.Core, 0, 0, 0)
		}
		if err == nil {
			s.kind = keysUnsampled
		}
	case *signalpb.SpqrV1_KeysSampled:
		if err = encdec(st.KeysSampled); err == nil {
			if s, f, err = core(ed.Core, ekLen, dkLen, 0); err == nil {
				s.kind, s.ekPayload, s.dk = keysSampled, f[0], f[1]
				s.hdrEnc, err = importEncoder(ed.Encoder, hdrStreamLen)
			}
		}
	case *signalpb.SpqrV1_HeaderSent:
		if err = encdec(st.HeaderSent); err == nil {
			if s, f, err = core(ed.Core, dkLen, 0, 0); err == nil {
				s.kind, s.dk = headerSent, f[0]
				if s.ekEnc, err = importEncoder(ed.Encoder, ekStreamLen); err == nil {
					s.ct1Dec, err = importDecoder(ed.Decoder, ct1StreamLen)
				}
			}
		}
	case *signalpb.SpqrV1_Ct1Received:
		if err = encdec(st.Ct1Received); err == nil {
			if s, f, err = core(ed.Core, dkLen, ct1Len, 0); err == nil {
				s.kind, s.dk, s.ct1 = ct1Received, f[0], f[1]
				s.ekEnc, err = importEncoder(ed.Encoder, ekStreamLen)
			}
		}
	case *signalpb.SpqrV1_EkSentCt1Received:
		if err = encdec(st.EkSentCt1Received); err == nil {
			if s, f, err = core(ed.Core, dkLen, ct1Len, 0); err == nil {
				s.kind, s.dk, s.ct1 = ekSentCt1Received, f[0], f[1]
				s.ct2Dec, err = importDecoder(ed.Decoder, ct2StreamLen)
			}
		}
	case *signalpb.SpqrV1_NoHeaderReceived:
		if err = dec(st.NoHeaderReceived); err == nil {
			if s, _, err = core(d.Core, 0, 0, 0); err == nil {
				s.kind = noHeaderReceived
				s.hdrDec, err = importDecoder(d.Decoder, hdrStreamLen)
			}
		}
	case *signalpb.SpqrV1_HeaderReceived:
		if err = dec(st.HeaderReceived); err == nil {
			if s, f, err = core(d.Core, hdrLen, 0, 0); err == nil {
				s.kind, s.hdr = headerReceived, f[0]
				s.ekDec, err = importDecoder(d.Decoder, ekStreamLen)
			}
		}
	case *signalpb.SpqrV1_Ct1Sampled:
		if err = encdec(st.Ct1Sampled); err == nil {
			if s, f, err = core(ed.Core, hdrLen, esLen, ct1Len); err == nil {
				s.kind, s.hdr, s.ct1 = ct1Sampled, f[0], f[2]
				if s.enc, err = kyber.ResumeEncapsulationMLKEM768(f[1], f[0], f[2]); err == nil {
					if s.ct1Enc, err = importEncoder(ed.Encoder, ct1StreamLen); err == nil {
						s.ekDec, err = importDecoder(ed.Decoder, ekStreamLen)
					}
				}
			}
		}
	case *signalpb.SpqrV1_EkReceivedCt1Sampled:
		if err = encdec(st.EkReceivedCt1Sampled); err == nil {
			if s, f, err = core(ed.Core, esLen, ekLen, ct1Len); err == nil {
				s.kind, s.ct1 = ekReceivedCt1Sampled, f[2]
				if s.enc, err = kyber.ResumeEncapsulationMLKEM768(f[0], nil, f[2]); err == nil {
					// This implementation computes c2 when the ek payload
					// completes, so do it now from the stored payload.
					if s.ct2, err = s.enc.Finish(f[1]); err == nil {
						s.ct1Enc, err = importEncoder(ed.Encoder, ct1StreamLen)
					}
				}
			}
		}
	case *signalpb.SpqrV1_Ct1Acknowledged:
		if err = dec(st.Ct1Acknowledged); err == nil {
			if s, f, err = core(d.Core, hdrLen, esLen, ct1Len); err == nil {
				s.kind, s.hdr, s.ct1 = ct1Acknowledged, f[0], f[2]
				if s.enc, err = kyber.ResumeEncapsulationMLKEM768(f[1], f[0], f[2]); err == nil {
					s.ekDec, err = importDecoder(d.Decoder, ekStreamLen)
				}
			}
		}
	case *signalpb.SpqrV1_Ct2Sampled:
		if err = encdec(st.Ct2Sampled); err == nil {
			if s, _, err = core(ed.Core, 0, 0, 0); err == nil {
				s.kind = ct2Sampled
				s.ct2Enc, err = importEncoder(ed.Encoder, ct2StreamLen)
			}
		}
	default:
		return nil, stateErr("V1 state without an SCKA state")
	}
	if err != nil {
		if errors.Is(err, ErrStateDecode) || errors.Is(err, ErrMsgDecode) {
			return nil, err
		}
		return nil, stateErr("%v", err)
	}
	return s, nil
}
