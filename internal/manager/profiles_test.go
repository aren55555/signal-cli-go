// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/profile"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// The peer of profile_cipher.json's decode cases.
const vecPeer = "00000000-0000-4000-8000-000000000002"

type profileWorld struct {
	fc   *fakeChat
	s    *Sender
	cdn  *fakeCDN
	k1   [32]byte
	img  []byte
	dir  string
	resp map[string]string // decode case -> response JSON
}

func newProfileWorld(t *testing.T) *profileWorld {
	var v struct {
		Keys   map[string]string
		Decode []struct {
			Case     string
			Response string `json:"response_json"`
		} `json:"decode_profile"`
	}
	testvec.Load(t, "profile_cipher.json", &v)
	w := &profileWorld{k1: [32]byte(testvec.Hex(t, v.Keys["k1"])), img: []byte("\x89PNG avatar"), resp: map[string]string{}}
	for _, d := range v.Decode {
		w.resp[d.Case] = d.Response
	}
	a, _ := testAccount(t)
	a.Recipients[vecPeer] = store.Recipient{ProfileKey: w.k1[:]}
	w.fc = &fakeChat{t: t, sender: testACI, senderDev: 2, profiles: map[string]string{vecPeer: w.resp["complete profile"]}}
	w.cdn = &fakeCDN{blobs: map[string][]byte{"profiles/AAECAwQFBgcICQoLDA0ODw==": profile.EncryptAvatar(w.k1, [12]byte{7}, w.img)}}
	w.dir = filepath.Join(t.TempDir(), "avatars")
	// The clock advances 1 ms per read: timestamps must strictly increase.
	now := time.UnixMilli(1_800_000_000_000)
	w.s = &Sender{Acct: a, Chat: w.fc, CDN: w.cdn, AvatarsDir: w.dir, Now: func() time.Time { now = now.Add(time.Millisecond); return now }}
	return w
}

func (w *profileWorld) profileGets() int {
	n := 0
	for _, r := range w.fc.requests {
		if strings.Contains(r, "GET /v1/profile/") {
			n++
		}
	}
	return n
}

func TestProfileFetchStoresRecord(t *testing.T) {
	w := newProfileWorld(t)
	p := w.s.RefreshProfile(context.Background(), vecPeer)
	if p == nil || *p.GivenName != "Alice" || *p.FamilyName != "Smith" || *p.About != "Hello" || p.AccessMode != store.AccessEnabled ||
		p.LastUpdate <= 1_800_000_000_000 || *p.PhoneNumberSharing != "EVERYBODY" || len(p.MobileCoinAddress) == 0 || *p.AvatarURLPath != "profiles/AAECAwQFBgcICQoLDA0ODw==" {
		t.Fatalf("profile %+v", p)
	}
	// Versioned path: the key is known.
	if !strings.Contains(w.fc.requests[len(w.fc.requests)-1], "/v1/profile/"+vecPeer+"/") {
		t.Errorf("requests %v", w.fc.requests)
	}
	if id, ok := w.s.Acct.Identities[vecPeer]; !ok || id.Trust != store.TrustedUnverified {
		t.Errorf("identity %+v", id)
	}
	got, err := os.ReadFile(filepath.Join(w.dir, "profile-"+vecPeer))
	if err != nil || !bytes.Equal(got, w.img) {
		t.Errorf("avatar %q %v", got, err)
	}
	// Lazy reads within 6 h make no request; the avatar is not fetched
	// again while its path is unchanged.
	n, fetches := w.profileGets(), len(w.cdn.log)
	w.s.Profile(context.Background(), vecPeer)
	if w.profileGets() != n {
		t.Error("lazy read fetched again")
	}
	w.s.RefreshProfile(context.Background(), vecPeer)
	if w.profileGets() != n+1 || len(w.cdn.log) != fetches {
		t.Errorf("forced fetch: %d gets, cdn %v", w.profileGets()-n, w.cdn.log)
	}
}

func TestProfileFetchFailure(t *testing.T) {
	w := newProfileWorld(t)
	w.s.RefreshProfile(context.Background(), vecPeer)
	// The server answers 404 now: unknown mode, timestamp now, names kept,
	// marked unregistered and split from its PNI.
	rc := w.s.Acct.Recipients[vecPeer]
	rc.PNI = "00000000-0000-4000-8000-0000000000f2"
	w.s.Acct.Recipients[vecPeer] = rc
	delete(w.fc.profiles, vecPeer)
	var logged []string
	w.s.Log = func(f string, args ...any) { logged = append(logged, f) }
	p := w.s.RefreshProfile(context.Background(), vecPeer)
	if p.AccessMode != store.AccessUnknown || len(p.Capabilities) != 0 || *p.GivenName != "Alice" || p.PhoneNumberSharing != nil {
		t.Errorf("profile %+v", p)
	}
	if rc := w.s.Acct.Recipients[vecPeer]; !rc.Unregistered || rc.PNI != "" {
		t.Errorf("recipient %+v", rc)
	}
	if _, ok := w.s.Acct.Recipients["PNI:00000000-0000-4000-8000-0000000000f2"]; !ok {
		t.Error("no split row")
	}
	if len(logged) != 1 || !strings.HasPrefix(logged[0], "Failed to retrieve profile for") {
		t.Errorf("log %v", logged)
	}
}

func TestProfileUnsealedFallback(t *testing.T) {
	w := newProfileWorld(t)
	w.s.RefreshProfile(context.Background(), vecPeer)
	// With ENABLED access stored and a certificate, the next fetch tries
	// the unauthenticated socket first; the fake answers 401, so the same
	// request is repeated authenticated.
	w.s.Unauth = unauthChat{w.fc}
	w.s.cert, w.s.certExpires = []byte{1}, uint64(time.UnixMilli(1_900_000_000_000).UnixMilli())
	w.fc.requests = nil
	w.s.RefreshProfile(context.Background(), vecPeer)
	if len(w.fc.requests) != 2 || !strings.HasPrefix(w.fc.requests[0], "unauth GET /v1/profile/") || !strings.HasPrefix(w.fc.requests[1], "auth GET /v1/profile/") {
		t.Errorf("requests %v", w.fc.requests)
	}
}

func TestProfileDecodeFailureKeepsOldRecord(t *testing.T) {
	w := newProfileWorld(t)
	w.s.RefreshProfile(context.Background(), vecPeer)
	w.fc.profiles[vecPeer] = w.resp["name ciphertext tampered: whole decode fails"]
	p := w.s.RefreshProfile(context.Background(), vecPeer)
	// Without a usable decode: old names stay, mode without a key.
	if p.AccessMode != store.AccessDisabled || *p.GivenName != "Alice" || p.PhoneNumberSharing != nil {
		t.Errorf("profile %+v", p)
	}
}

// selfWorld: the account's own profile is the decode vectors' complete
// profile (key k1), so writes start from stored values.
func selfWorld(t *testing.T) *profileWorld {
	w := newProfileWorld(t)
	w.s.Acct.Data.ProfileKey = w.k1[:]
	w.s.Acct.Data.DeviceID = 1 // single-device primary: no syncs to own devices
	w.fc.profiles[testACI] = w.resp["complete profile"]
	return w
}

func lastBody(t *testing.T, w *profileWorld) map[string]json.RawMessage {
	t.Helper()
	if len(w.fc.profilePuts) == 0 {
		t.Fatal("no profile PUT")
	}
	raw := w.fc.profilePuts[len(w.fc.profilePuts)-1]
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	order := []string{"version", "name", "about", "aboutEmoji", "paymentAddress", "phoneNumberSharing", "avatar", "sameAvatar", "commitment", "badgeIds"}
	last := -1
	for _, k := range order {
		i := strings.Index(raw, `"`+k+`":`)
		if i <= last {
			t.Errorf("member %s out of order in %s", k, raw)
		}
		last = i
	}
	return m
}

func decField(t *testing.T, w *profileWorld, m map[string]json.RawMessage, k string) string {
	var b []byte
	json.Unmarshal(m[k], &b)
	s, err := profile.DecryptString(w.k1, b)
	if err != nil {
		t.Fatalf("%s: %v", k, err)
	}
	return s
}

func TestWriteProfileKeepsStoredFields(t *testing.T) {
	w := selfWorld(t)
	about := "New about"
	if err := w.s.WriteProfile(context.Background(), ProfileUpdate{About: &about}); err != nil {
		t.Fatal(err)
	}
	m := lastBody(t, w)
	if decField(t, w, m, "name") != "Alice\x00Smith" || decField(t, w, m, "about") != about || decField(t, w, m, "aboutEmoji") != "🙂" {
		t.Errorf("body %v", m)
	}
	if string(m["avatar"]) != "true" || string(m["sameAvatar"]) != "true" || string(m["badgeIds"]) != "[]" || string(m["paymentAddress"]) == "null" {
		t.Errorf("flags %s %s %s %s", m["avatar"], m["sameAvatar"], m["badgeIds"], m["paymentAddress"])
	}
	p := w.s.Acct.Recipients[testACI].Profile
	if *p.About != about || p.PhoneNumberSharing != nil {
		t.Errorf("stored %+v", p)
	}
}

func TestWriteProfileAvatar(t *testing.T) {
	w := selfWorld(t)
	img := []byte("\x89PNG\r\n\x1a\nnew avatar")
	before := w.s.Profile(context.Background(), testACI).LastUpdate
	err := w.s.WriteProfile(context.Background(), ProfileUpdate{Avatar: AvatarNew, AvatarSource: &attachment.Source{Data: img, ContentType: "image/png"}})
	if err != nil {
		t.Fatal(err)
	}
	m := lastBody(t, w)
	if string(m["avatar"]) != "true" || string(m["sameAvatar"]) != "false" {
		t.Errorf("flags %s %s", m["avatar"], m["sameAvatar"])
	}
	var names []string
	for _, f := range w.cdn.lastFields {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "acl,key,policy,Content-Type,x-amz-algorithm,x-amz-credential,x-amz-date,x-amz-signature" || w.cdn.lastType != "image/png" {
		t.Errorf("form %v %s", names, w.cdn.lastType)
	}
	if got, err := profile.DecryptAvatar(w.k1, w.cdn.blobs["profiles/NEW"]); err != nil || !bytes.Equal(got, img) {
		t.Errorf("uploaded avatar %v", err)
	}
	p := w.s.Acct.Recipients[testACI].Profile
	if *p.AvatarURLPath != "profiles/NEW" || p.LastUpdate != before {
		t.Errorf("stored %+v", p)
	}
	if got, _ := os.ReadFile(filepath.Join(w.dir, "profile-"+testNumber)); !bytes.Equal(got, img) {
		t.Error("local avatar not written")
	}
	if err := w.s.WriteProfile(context.Background(), ProfileUpdate{Avatar: AvatarRemove}); err != nil {
		t.Fatal(err)
	}
	m = lastBody(t, w)
	if string(m["avatar"]) != "false" || string(m["sameAvatar"]) != "true" {
		t.Errorf("remove flags %s %s", m["avatar"], m["sameAvatar"])
	}
	if _, err := os.Stat(filepath.Join(w.dir, "profile-"+testNumber)); !os.IsNotExist(err) {
		t.Error("local avatar not removed")
	}
}

func TestWriteProfileTooLong(t *testing.T) {
	w := selfWorld(t)
	long := strings.Repeat("x", 258)
	if err := w.s.WriteProfile(context.Background(), ProfileUpdate{GivenName: &long}); !errors.Is(err, ErrProfileInputTooLong) {
		t.Errorf("err %v", err)
	}
}
