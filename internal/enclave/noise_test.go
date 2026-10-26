package enclave

import (
	"encoding/hex"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/noise.json (spec/enclave/noise.md §7).
type transcript struct {
	ProtocolName string `json:"protocol_name"`
	HInitial     string `json:"h_initial"`
	Handshake    string `json:"handshake_hash"`
	Ephemeral    string `json:"initiator_ephemeral_private"`
	ResponderPub string `json:"responder_static_public"`
	KEMSeed      string `json:"kem_seed_d_z"`
	Msg1, Msg2   string
	C2S          string `json:"initiator_to_responder_key"`
	S2C          string `json:"responder_to_initiator_key"`
	Transport    []struct {
		Ciphertext, Direction, Plaintext string
		Nonce                            uint64
	}
}

func TestNoiseTranscripts(t *testing.T) {
	var v struct{ NK, NKhfs transcript }
	testvec.Load(t, "noise.json", &v)
	for _, tr := range []transcript{v.NK, v.NKhfs} {
		if newSymmetric(tr.ProtocolName).h != [32]byte(testvec.Hex(t, tr.HInitial)) {
			t.Errorf("%s: initial h", tr.ProtocolName)
		}
		o := Options{Ephemeral: testvec.Hex(t, tr.Ephemeral)}
		if tr.KEMSeed != "" {
			o.KEMSeed = testvec.Hex(t, tr.KEMSeed)
		}
		in, m1, err := NewInitiator(tr.ProtocolName, testvec.Hex(t, tr.ResponderPub), o)
		if err != nil || hex.EncodeToString(m1) != tr.Msg1 {
			t.Fatalf("%s: message 1 %v (%d bytes)", tr.ProtocolName, err, len(m1))
		}
		tp, err := in.Finish(testvec.Hex(t, tr.Msg2))
		if err != nil {
			t.Fatalf("%s: message 2: %v", tr.ProtocolName, err)
		}
		if hex.EncodeToString(tp.send.k[:]) != tr.C2S || hex.EncodeToString(tp.recv.k[:]) != tr.S2C {
			t.Errorf("%s: split keys", tr.ProtocolName)
		}
		if h := tp.HandshakeHash(); hex.EncodeToString(h[:]) != tr.Handshake {
			t.Errorf("%s: handshake hash %x", tr.ProtocolName, h)
		}
		for _, m := range tr.Transport {
			switch m.Direction {
			case "initiator_to_responder":
				tp.send.n = m.Nonce
				if got := tp.send.seal(nil, testvec.Hex(t, m.Plaintext)); hex.EncodeToString(got) != m.Ciphertext {
					t.Errorf("%s: c2s nonce %d: %x", tr.ProtocolName, m.Nonce, got)
				}
			default:
				tp.recv.n = m.Nonce
				p, err := tp.recv.open(nil, testvec.Hex(t, m.Ciphertext))
				if err != nil || hex.EncodeToString(p) != m.Plaintext {
					t.Errorf("%s: s2c nonce %d: %x %v", tr.ProtocolName, m.Nonce, p, err)
				}
			}
		}
		// A flipped byte in message 2 fails the handshake.
		in2, _, _ := NewInitiator(tr.ProtocolName, testvec.Hex(t, tr.ResponderPub), o)
		bad := testvec.Hex(t, tr.Msg2)
		bad[40] ^= 1
		if _, err := in2.Finish(bad); err == nil {
			t.Errorf("%s: tampered message 2 accepted", tr.ProtocolName)
		}
	}
}

// TestTransportChunking: frames split at 65519-byte plaintexts and open
// in 65535-byte pieces (noise.md §5 table).
func TestTransportChunking(t *testing.T) {
	var a, b Transport
	a.send = cipherState{k: [32]byte{1}, has: true}
	b.recv = a.send
	for _, c := range []struct{ plain, frame int }{{0, 0}, {3, 19}, {65519, 65535}, {65520, 65552}, {200000, 200064}} {
		p := make([]byte, c.plain)
		for i := range p {
			p[i] = byte(i)
		}
		f := a.Seal(p)
		if len(f) != c.frame {
			t.Errorf("%d bytes: frame %d", c.plain, len(f))
		}
		got, err := b.Open(f)
		if err != nil || len(got) != c.plain {
			t.Errorf("%d bytes: open %d %v", c.plain, len(got), err)
		}
	}
}
