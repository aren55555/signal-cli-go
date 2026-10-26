package manager

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/store"
)

func (w *profileWorld) requested(prefix string) bool {
	return slices.ContainsFunc(w.fc.requests, func(r string) bool { return strings.HasPrefix(r, prefix) })
}

// TestBlockRotatesProfileKey: blocking a contact that shares no group
// turns its profile sharing off and rotates the own key (attributes,
// profile write); the blocked-list sync is sent even without changes.
func TestBlockRotatesProfileKey(t *testing.T) {
	w := selfWorld(t)
	rc := w.s.Acct.Recipients[vecPeer]
	rc.ProfileSharing = true
	w.s.Acct.Recipients[vecPeer] = rc
	oldKey := bytes.Clone(w.s.Acct.Data.ProfileKey)
	if err := w.s.SetBlocked(context.Background(), []string{vecPeer}, true, nil); err != nil {
		t.Fatal(err)
	}
	rc = w.s.Acct.Recipients[vecPeer]
	if !rc.Blocked || rc.ProfileSharing {
		t.Errorf("recipient %+v", rc)
	}
	if bytes.Equal(w.s.Acct.Data.ProfileKey, oldKey) || !w.requested("auth PUT /v1/accounts/attributes") || len(w.fc.profilePuts) != 1 {
		t.Errorf("no rotation: requests %v", w.fc.requests)
	}
	// Unchanged state: no second rotation, nothing fails.
	w.fc.profilePuts = nil
	if err := w.s.SetBlocked(context.Background(), []string{vecPeer}, true, nil); err != nil || len(w.fc.profilePuts) != 0 {
		t.Errorf("second block: %v, %d profile writes", err, len(w.fc.profilePuts))
	}
	// Unblocking turns profile sharing on (UNBLOCK_AND_ACCEPT).
	if err := w.s.SetBlocked(context.Background(), []string{vecPeer}, false, nil); err != nil {
		t.Fatal(err)
	}
	if rc := w.s.Acct.Recipients[vecPeer]; rc.Blocked || !rc.ProfileSharing {
		t.Errorf("after unblock %+v", rc)
	}
}

func TestUpdateContactTimerVersions(t *testing.T) {
	w := selfWorld(t)
	given := "Ann"
	exp := uint32(60)
	// Not yet a contact: version 1, names stored, unhidden.
	rc := w.s.Acct.Recipients[vecPeer]
	rc.Hidden = true
	w.s.Acct.Recipients[vecPeer] = rc
	if err := w.s.UpdateContact(context.Background(), vecPeer, ContactUpdate{GivenName: &given, Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	rc = w.s.Acct.Recipients[vecPeer]
	if rc.Timer != 60 || rc.TimerVersion != 1 || *rc.GivenName != "Ann" || rc.Hidden {
		t.Errorf("first %+v", rc)
	}
	exp = 120
	w.s.UpdateContact(context.Background(), vecPeer, ContactUpdate{Expiration: &exp})
	if rc := w.s.Acct.Recipients[vecPeer]; rc.Timer != 120 || rc.TimerVersion != 2 {
		t.Errorf("second %+v", rc)
	}
	// Same timer: version unchanged.
	w.s.UpdateContact(context.Background(), vecPeer, ContactUpdate{Expiration: &exp})
	if rc := w.s.Acct.Recipients[vecPeer]; rc.TimerVersion != 2 {
		t.Errorf("same timer bumped the version: %+v", rc)
	}
}

func TestRemoveContactModes(t *testing.T) {
	w := selfWorld(t)
	given := "Ann"
	a := w.s.Acct
	rc := a.Recipients[vecPeer]
	rc.GivenName, rc.Timer, rc.TimerVersion, rc.ProfileSharing = &given, 30, 3, true
	a.Recipients[vecPeer] = rc
	a.Sessions["aci:"+vecPeer+".1"] = []byte{1}
	a.Identities[vecPeer] = store.Identity{Key: []byte{5}}
	if err := w.s.RemoveContact(vecPeer, true, false); err != nil || !a.Recipients[vecPeer].Hidden || a.Recipients[vecPeer].GivenName == nil {
		t.Errorf("hide: %v %+v", err, a.Recipients[vecPeer])
	}
	w.s.RemoveContact(vecPeer, false, false)
	if rc := a.Recipients[vecPeer]; rc.IsContact() || rc.TimerVersion != 1 || rc.Hidden || rc.ProfileKey == nil {
		t.Errorf("delete: %+v", rc)
	}
	w.s.RemoveContact(vecPeer, false, true)
	if _, ok := a.Recipients[vecPeer]; ok || len(a.Sessions) != 0 || a.Identities[vecPeer].Key != nil {
		t.Errorf("forget left data: %v %v", a.Sessions, a.Identities[vecPeer])
	}
	if err := w.s.RemoveContact(testACI, false, true); err != ErrDeleteSelf {
		t.Errorf("forget self: %v", err)
	}
}
