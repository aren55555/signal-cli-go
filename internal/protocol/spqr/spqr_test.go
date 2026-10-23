package spqr

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"
)

type side struct {
	name string
	st   *State
	rng  *rand.ChaCha8
}

type wireMsg struct {
	from string
	b    []byte
	key  []byte
}

func newSides(t *testing.T) (a, b *side) {
	t.Helper()
	return &side{"A", New(authKey, true, Params{}), rand.NewChaCha8([32]byte{1})},
		&side{"B", New(authKey, false, Params{}), rand.NewChaCha8([32]byte{2})}
}

func (s *side) send(t *testing.T) wireMsg {
	t.Helper()
	next, msg, key, err := s.st.Send(s.rng)
	if err != nil {
		t.Fatalf("%s send: %v", s.name, err)
	}
	if len(key) != 32 {
		t.Fatalf("%s send: key of %d bytes", s.name, len(key))
	}
	s.st = next
	return wireMsg{s.name, msg, key}
}

func (s *side) recv(t *testing.T, m wireMsg) {
	t.Helper()
	next, key, err := s.st.Receive(m.b)
	if err != nil {
		t.Fatalf("%s receive: %v", s.name, err)
	}
	if !bytes.Equal(key, m.key) {
		t.Fatalf("%s: key differs from the sender's", s.name)
	}
	s.st = next
}

func (s *side) epoch() uint64 { return s.st.scka.epoch }

// TestLockstep: one message each way per round. An epoch takes about 40
// messages each way (3 Hdr, 36 Ek, 30 Ct1, 5 Ct2 chunks, overlapping).
func TestLockstep(t *testing.T) {
	a, b := newSides(t)
	for range 300 {
		b.recv(t, a.send(t))
		a.recv(t, b.send(t))
	}
	if a.epoch() < 4 || b.epoch() < 4 {
		t.Errorf("epochs after 300 rounds: A %d, B %d; want at least 4", a.epoch(), b.epoch())
	}
	// Roles alternate: the initiator is send_ek in odd epochs.
	if (a.epoch()%2 == 1) != a.st.scka.sendEK() {
		t.Errorf("A is %v at epoch %d", a.st.scka.kind, a.epoch())
	}
}

// TestLossyReordered drops and reorders messages; every delivered message
// still yields the sender's key, and epochs still advance.
func TestLossyReordered(t *testing.T) {
	a, b := newSides(t)
	r := rand.New(rand.NewPCG(9, 9))
	var toA, toB []wireMsg
	deliver := func(q *[]wireMsg, to *side) {
		// Deliver a random queued message, or none.
		if len(*q) == 0 || r.IntN(3) == 0 {
			return
		}
		i := r.IntN(len(*q))
		m := (*q)[i]
		*q = append((*q)[:i], (*q)[i+1:]...)
		to.recv(t, m)
	}
	for range 3000 {
		if m := a.send(t); r.IntN(10) >= 3 { // 30% loss
			toB = append(toB, m)
		}
		if m := b.send(t); r.IntN(10) >= 3 {
			toA = append(toA, m)
		}
		deliver(&toB, b)
		deliver(&toA, a)
		deliver(&toB, b)
		deliver(&toA, a)
	}
	if a.epoch() < 3 || b.epoch() < 3 {
		t.Errorf("epochs under loss: A %d, B %d; want at least 3", a.epoch(), b.epoch())
	}
}

func TestFirstKeyIndex(t *testing.T) {
	a, _ := newSides(t)
	m := a.send(t)
	pm, err := parseMessage(m.b)
	if err != nil {
		t.Fatal(err)
	}
	if pm.epoch != 1 || pm.keyIndex != 1 || pm.payload != payloadHdr || pm.chunkIndex != 0 {
		t.Errorf("first message %+v", pm)
	}
}

// TestFailedReceiveKeepsState: Receive returns a new state; on error the
// old one is still usable (the caller doesn't store the failure).
func TestFailedReceiveKeepsState(t *testing.T) {
	a, b := newSides(t)
	m := a.send(t)
	bad := bytes.Clone(m.b)
	bad[len(bad)-1] ^= 0xff // chunk data: only detected when the Hdr completes
	before := b.st
	var err error
	cur := b.st
	cur, _, err = cur.Receive(bad)
	if err != nil {
		t.Fatal(err) // one bad chunk can't complete the header
	}
	_ = cur
	b.st = before // drop the result, as a failed outer MAC would
	b.recv(t, m)
	// Replaying the same message: its key was already used.
	if _, _, err := b.st.Receive(m.b); !errors.Is(err, ErrKeyAlreadyRequested) {
		t.Errorf("replay: %v", err)
	}
}

func TestHeaderMACFailure(t *testing.T) {
	a, b := newSides(t)
	var msgs []wireMsg
	for range 3 {
		msgs = append(msgs, a.send(t))
	}
	bad := bytes.Clone(msgs[2].b)
	bad[len(bad)-1] ^= 0x01
	b.recv(t, msgs[0])
	b.recv(t, msgs[1])
	if _, _, err := b.st.Receive(bad); !errors.Is(err, ErrMacVerifyFailed) {
		t.Errorf("corrupted header chunk: %v, want ErrMacVerifyFailed", err)
	}
}

func TestVersionHandling(t *testing.T) {
	a, b := newSides(t)
	if _, _, err := b.st.Receive(nil); !errors.Is(err, ErrMinimumVersion) {
		t.Errorf("empty (V0) message: %v, want ErrMinimumVersion", err)
	}
	if _, _, err := b.st.Receive([]byte{0x00, 0x01}); !errors.Is(err, ErrMinimumVersion) {
		t.Errorf("V0 message: %v", err)
	}
	next, key, err := b.st.Receive([]byte{0x02, 0xaa})
	if err != nil || key != nil || next != b.st {
		t.Errorf("unknown version: %v %x (state replaced: %v)", err, key, next != b.st)
	}
	// After a successful receive the negotiation record is gone, so a V0
	// message is a version mismatch.
	b.recv(t, a.send(t))
	if _, _, err := b.st.Receive(nil); !errors.Is(err, ErrVersionMismatch) {
		t.Errorf("V0 after negotiation: %v, want ErrVersionMismatch", err)
	}
	if _, _, err := b.st.Receive([]byte{0x01, 0x00}); !errors.Is(err, ErrMsgDecode) {
		t.Errorf("malformed V1: %v", err)
	}
}

func TestFutureEpochRejected(t *testing.T) {
	_, b := newSides(t)
	m := message{epoch: 5, keyIndex: 1, payload: payloadNone}
	if _, _, err := b.st.Receive(m.encode()); !errors.Is(err, ErrEpochOutOfRange) {
		t.Errorf("epoch 5 at epoch 1: %v", err)
	}
}

// TestTransitions drives both Ek-completion paths of protocol.md §7.2,
// which lockstep delivery never reaches: completion on an EkCt1Ack chunk
// goes straight to Ct2Sampled; completion on plain Ek chunks goes to
// EkReceivedCt1Sampled, then Ct2Sampled on the next acknowledgement.
func TestTransitions(t *testing.T) {
	for _, withAck := range []bool{true, false} {
		a, b := newSides(t)
		// Run until B has sent Ct1 #0 and A has seen it (A is in HeaderSent).
		for a.st.scka.kind != headerSent {
			b.recv(t, a.send(t))
			a.recv(t, b.send(t))
		}
		if b.st.scka.kind != ct1Sampled {
			t.Fatalf("B is %v, want Ct1Sampled", b.st.scka.kind)
		}
		// A sends plain Ek chunks; B gets 35 (36 without the ack case).
		// B's Ct1 chunks are held back from A meanwhile.
		plain := 35
		if !withAck {
			plain = 36
		}
		var held []wireMsg
		for range plain {
			m := a.send(t)
			if pm, _ := parseMessage(m.b); pm.payload != payloadEk {
				t.Fatalf("A sent %v, want Ek", pm.payload)
			}
			b.recv(t, m)
			held = append(held, b.send(t))
		}
		if withAck && b.st.scka.kind != ct1Sampled {
			t.Fatalf("after 35 Ek chunks B is %v", b.st.scka.kind)
		}
		if !withAck && b.st.scka.kind != ekReceivedCt1Sampled {
			t.Fatalf("after 36 plain Ek chunks B is %v, want EkReceivedCt1Sampled", b.st.scka.kind)
		}
		// Now A gets all of ct1 and starts acknowledging.
		for _, m := range held {
			a.recv(t, m)
		}
		for a.st.scka.kind != ct1Received {
			a.recv(t, b.send(t))
		}
		m := a.send(t)
		if pm, _ := parseMessage(m.b); pm.payload != payloadEkCt1Ack {
			t.Fatalf("A sent %v, want EkCt1Ack", pm.payload)
		}
		b.recv(t, m)
		if b.st.scka.kind != ct2Sampled {
			t.Fatalf("withAck=%v: after the acknowledgement B is %v, want Ct2Sampled", withAck, b.st.scka.kind)
		}
	}
}
