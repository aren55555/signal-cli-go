package protocol

import "testing"

func TestArchiveAndInitiate(t *testing.T) {
	d := newDuo(t, 90)
	typ, wire := d.aliceSend(t, "old session", t0)
	if d.alice.RemoteRegistrationID() != d.bobRegID || !d.alice.HasCurrent() || !d.alice.Unacknowledged() {
		t.Fatalf("registration ID %d", d.alice.RemoteRegistrationID())
	}
	archived := d.alice.ArchiveCurrent()
	if archived.HasCurrent() || archived.RemoteRegistrationID() != 0 || len(archived.previous) != 1 {
		t.Fatalf("archived record: current %v, %d previous", archived.HasCurrent(), len(archived.previous))
	}
	if _, _, _, err := archived.Encrypt(d.rngA, []byte("x"), t0, aliceAddr, bobAddr); err == nil {
		t.Error("encrypting without a current state succeeded")
	}
	fresh, err := archived.Initiate(d.rngA, d.aliceID, d.aliceRegID, d.b.bundle, t0, aliceAddr, bobAddr)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.HasCurrent() || len(fresh.previous) != 1 {
		t.Errorf("after Initiate: %d previous", len(fresh.previous))
	}
	// Bob still decrypts the message from the archived session.
	if got, err := d.bobReceive(t, typ, wire); err != nil || got != "old session" {
		t.Errorf("bob: %q, %v", got, err)
	}
}

func TestUnacknowledgedClearsOnReply(t *testing.T) {
	d := newDuo(t, 91)
	typ, wire := d.aliceSend(t, "hi", t0)
	if _, err := d.bobReceive(t, typ, wire); err != nil {
		t.Fatal(err)
	}
	d.aliceReceive(t, d.bobSend(t, "reply"))
	if d.alice.Unacknowledged() {
		t.Error("still unacknowledged after a reply")
	}
}

func TestSenderRatchetKey(t *testing.T) {
	d := newDuo(t, 92)
	typ, wire := d.aliceSend(t, "hi", t0)
	k := SenderRatchetKey(wire, typ)
	if len(k) != 33 || k[0] != 0x05 {
		t.Fatalf("ratchet key %x", k)
	}
	if string(k) != string(d.alice.current.senderRatchet.PublicKey().Bytes()) {
		t.Error("not the sender ratchet key")
	}
	if SenderRatchetKey([]byte{1, 2}, typ) != nil {
		t.Error("garbage parsed")
	}
}
