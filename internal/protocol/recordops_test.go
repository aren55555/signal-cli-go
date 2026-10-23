package protocol

import "testing"

func TestArchiveAndInitiate(t *testing.T) {
	d := newDuo(t, 90)
	typ, wire := d.aliceSend(t, "old session", t0)
	if d.alice.RemoteRegistrationID() != d.bobRegID || !d.alice.HasCurrent() {
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
