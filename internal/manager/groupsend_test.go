// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

// groupWorld: a three-member v2 group (self, B, C) built from the
// endorsement vector gse1, so the stored endorsements are real ones.
type groupWorld struct {
	fc      *fakeChat
	m       *Groups
	rec     *store.GroupRecord
	g       *zkgroup.GroupSecretParams
	ends    [][]byte
	exp     uint64
	b, c    string
	cKey    []byte
	accSelf *store.Account
}

func newGroupWorld(t *testing.T) *groupWorld {
	var v struct {
		Groups map[string]struct {
			MasterKey string `json:"master_key"`
		}
		Endorsements []struct {
			Group        string
			Members      []string `json:"members_aci"`
			Expiration   uint64
			ReceiveTime  int64 `json:"receive_time"`
			Endorsements []string
		} `json:"group_send_endorsements"`
	}
	testvec.Load(t, "zkgroup_credentials.json", &v)
	gse := v.Endorsements[0]
	self, b, c := gse.Members[0], gse.Members[1], gse.Members[2]

	selfID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	cID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a, _ := testAccountFor(t, testNumber, self, testPNI, 1, selfID)
	b1, b1Bundles := testAccountFor(t, "+15555550101", b, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	c1, c1Bundles := testAccountFor(t, "+15555550102", c, "c0c0c0c0-0000-4000-8000-0000000000f3", 1, cID)
	root, _ := curve25519.GeneratePrivateKey(rand.Reader)
	fc := &fakeChat{t: t, sender: self, senderDev: 1, root: root,
		devices: map[string]map[uint32]fakeDevice{b: {1: {b1, b1Bundles["aci"]}}, c: {1: {c1, c1Bundles["aci"]}}},
		cert:    mintCert(t, root, self, selfID.PublicKey(), 1),
		profiles: map[string]string{b: profileJSON(bID.PublicKey().Bytes(), nil, true),
			c: profileJSON(cID.PublicKey().Bytes(), nil, true)},
		accessKeys: map[string][]byte{b: make([]byte, 16), c: make([]byte, 16)},
	}
	now := func() time.Time { return time.Unix(gse.ReceiveTime, 0) }
	s := &Sender{Acct: a, Chat: fc, Unauth: unauthChat{fc}, CDN: &fakeCDN{}, Now: now}
	m := &Groups{Acct: a, Svc: &GroupsService{Acct: a, Chat: fc, Now: now}, Sender: s}

	var mk [32]byte
	copy(mk[:], testvec.Hex(t, v.Groups[gse.Group].MasterKey))
	g := zkgroup.DeriveGroupSecretParams(mk)
	rec := &store.GroupRecord{GroupID: g.GroupID[:], MasterKey: mk[:], DistributionID: newDistributionID(),
		EndorsementExpiration: int64(gse.Expiration) * 1000, Endorsements: map[string][]byte{}}
	var ends [][]byte
	for i, aci := range gse.Members[:3] {
		e := testvec.Hex(t, gse.Endorsements[i])
		ends = append(ends, e)
		rec.Endorsements[aci] = e
	}
	setState(rec, &groups.State{Revision: 4, Members: []groups.Member{{ACI: self, Role: groups.RoleAdmin}, {ACI: b}, {ACI: c}}})
	a.Groups = append(a.Groups, *rec)
	return &groupWorld{fc: fc, m: m, rec: a.Group(rec.GroupID), g: g, ends: ends, exp: gse.Expiration, b: b, c: c, cKey: cID.PublicKey().Bytes(), accSelf: a}
}

func (w *groupWorld) token(t *testing.T, idx ...int) string {
	var es []zkgroup.GroupSendEndorsement
	for _, i := range idx {
		e, err := zkgroup.ParseGroupSendEndorsement(w.ends[i])
		if err != nil {
			t.Fatal(err)
		}
		es = append(es, e)
	}
	return base64.StdEncoding.EncodeToString(zkgroup.CombineEndorsements(es).FullToken(w.g, w.exp))
}

// TestGroupSendSenderKey: two qualifying members get one multi-recipient
// request carrying the combined group send token (groups.md §12.2-12.3).
func TestGroupSendSenderKey(t *testing.T) {
	w := newGroupWorld(t)
	ts, results, err := w.m.SendData(context.Background(), w.rec.GroupID, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("hi group")}, Urgent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("results %+v (requests %v)", results, w.fc.requests)
	}
	if len(w.fc.mrPaths) != 1 || strings.Contains(w.fc.mrPaths[0], "story=true") {
		t.Fatalf("multi-recipient paths %v", w.fc.mrPaths)
	}
	if len(w.fc.mrTokens) != 1 || w.fc.mrTokens[0] != w.token(t, 1, 2) {
		t.Errorf("group-send-token %v, want the combined token of B and C", w.fc.mrTokens)
	}
	for _, dest := range []string{w.b, w.c} {
		var got *signalpb.AppDataMessage
		for _, c := range deliveredTo(t, w.fc, dest) {
			if dm := c.GetDataMessage(); dm != nil {
				got = dm
			}
		}
		if got == nil || got.GetBody() != "hi group" || got.GetTimestamp() != ts ||
			!slices.Equal(got.GetGroupV2().GetMasterKey(), w.rec.MasterKey) || got.GetGroupV2().GetRevision() != 4 || got.ProfileKey != nil {
			t.Errorf("%s got %v", dest, got)
		}
	}
	if !w.accSelf.Group(w.rec.GroupID).ProfileSharing {
		t.Error("profile sharing not switched on")
	}
	// One send-log entry with the group id and a row per member device
	// (send-log.md §1.3).
	for _, dest := range []string{w.b, w.c} {
		entries, _ := w.accSelf.SendLogLookup(dest, 1, ts, true)
		if len(entries) != 1 || !slices.Equal(entries[0].GroupID, w.rec.GroupID) {
			t.Errorf("send log for %s: %+v", dest, entries)
		}
	}
}

// TestGroupSendFanOut: with one qualifying member there are no sender-key
// targets; both members get 1:1 sends.
func TestGroupSendFanOut(t *testing.T) {
	w := newGroupWorld(t)
	rec := w.accSelf.Group(w.rec.GroupID)
	delete(rec.Endorsements, w.c)
	w.accSelf.SaveGroups()
	_, results, err := w.m.SendData(context.Background(), w.rec.GroupID, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("x")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || len(w.fc.mrPaths) != 0 {
		t.Fatalf("results %+v, multi-recipient %v", results, w.fc.mrPaths)
	}
	if len(deliveredTo(t, w.fc, w.b)) != 1 || len(deliveredTo(t, w.fc, w.c)) != 1 {
		t.Errorf("1:1 deliveries: requests %v", w.fc.requests)
	}
}

// TestGroupStory: the story goes by sender key with story authorization
// and the master key as the group ID; with too few targets nothing is
// sent at all (groups.md §13).
func TestGroupStory(t *testing.T) {
	w := newGroupWorld(t)
	src := attachment.Source{Data: []byte("img"), ContentType: "image/png"}
	_, results, err := w.m.SendStory(context.Background(), w.rec.GroupID, src, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || len(w.fc.mrPaths) != 1 || !strings.Contains(w.fc.mrPaths[0], "story=true") || len(w.fc.mrTokens) != 0 {
		t.Fatalf("results %+v, paths %v, tokens %v", results, w.fc.mrPaths, w.fc.mrTokens)
	}
	for _, c := range deliveredTo(t, w.fc, w.b) {
		if st := c.GetStoryMessage(); st != nil && !slices.Equal(st.GetGroup().GetMasterKey(), w.rec.MasterKey) {
			t.Errorf("story group %v", st.GetGroup())
		}
	}

	w = newGroupWorld(t)
	rec := w.accSelf.Group(w.rec.GroupID)
	delete(rec.Endorsements, w.c)
	w.accSelf.SaveGroups()
	_, results, err = w.m.SendStory(context.Background(), w.rec.GroupID, src, true)
	if err != nil || len(results) != 0 {
		t.Fatalf("results %+v, err %v", results, err)
	}
	for _, r := range w.fc.requests {
		if strings.Contains(r, "/v1/messages") {
			t.Errorf("sent %s", r)
		}
	}
}

// TestGroupSendUntrusted: a member whose stored identity is untrusted is
// no sender-key target, which leaves too few targets. (Without the
// target check the sender-key attempt fails on the identity before any
// request and falls back, so this pins the outcome, not the check.)
func TestGroupSendUntrusted(t *testing.T) {
	w := newGroupWorld(t)
	w.accSelf.Identities[w.c] = store.Identity{Key: w.cKey, Trust: store.Untrusted}
	w.m.SendData(context.Background(), w.rec.GroupID, DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("x")}})
	if len(w.fc.mrPaths) != 0 {
		t.Errorf("sender-key send with an untrusted member: %v", w.fc.mrPaths)
	}
}

// TestGroupSendPathLog: every group send names its path in a debug line,
// which the live interop runs assert (spec/testing/groups-interop.md §3).
func TestGroupSendPathLog(t *testing.T) {
	capture := func(w *groupWorld) *[]string {
		var lines []string
		w.m.Sender.Debug = func(f string, args ...any) { lines = append(lines, fmt.Sprintf(f, args...)) }
		return &lines
	}
	has := func(lines []string, s string) bool {
		return slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, s) })
	}
	msg := DataSend{Message: &signalpb.AppDataMessage{Body: proto.String("x")}}
	src := attachment.Source{Data: []byte("img"), ContentType: "image/png"}

	w := newGroupWorld(t)
	lines := capture(w)
	w.m.SendData(context.Background(), w.rec.GroupID, msg)
	if !has(*lines, "multi-recipient endpoint with group-send token, sender key, sealed sender v2, 2 recipient(s), 2 device(s)") ||
		!has(*lines, "multi-recipient endpoint accepted") || !has(*lines, "sender key distribution to 2 recipient(s)") || has(*lines, "fallback") {
		t.Errorf("sender-key send logged %q", *lines)
	}

	w = newGroupWorld(t)
	delete(w.accSelf.Group(w.rec.GroupID).Endorsements, w.c)
	lines = capture(w)
	w.m.SendData(context.Background(), w.rec.GroupID, msg)
	if !has(*lines, "Group send path: 1:1 fallback for 2 of 2 recipient(s): only 1 member(s) qualify for sender key") || has(*lines, "multi-recipient") {
		t.Errorf("fan-out logged %q", *lines)
	}

	w = newGroupWorld(t)
	lines = capture(w)
	w.m.SendStory(context.Background(), w.rec.GroupID, src, true)
	if !has(*lines, "multi-recipient endpoint with story, sender key") || has(*lines, "not sent") {
		t.Errorf("story logged %q", *lines)
	}

	w = newGroupWorld(t)
	delete(w.accSelf.Group(w.rec.GroupID).Endorsements, w.c)
	lines = capture(w)
	w.m.SendStory(context.Background(), w.rec.GroupID, src, true)
	if !has(*lines, "Group send path: story not sent to 2 of 2 recipient(s)") {
		t.Errorf("story without targets logged %q", *lines)
	}
}

// TestEnvelopePathLog: the receive side classifies envelopes by
// sealed-sender version and inner message type.
func TestEnvelopePathLog(t *testing.T) {
	var got []string
	d := &Decryptor{Debug: func(f string, args ...any) { got = append(got, fmt.Sprintf(f, args...)) }}
	from := protocol.Address{ServiceID: "s", DeviceID: 2}
	d.logPath(2, envSenderKey, from, true, 7)
	d.logPath(1, envCiphertext, from, true, 7)
	d.logPath(2, envPreKeyBundle, from, false, 7)
	d.logPath(-1, envPreKeyBundle, from, false, 7)
	want := []string{
		"Envelope path: multi-recipient (sealed sender v2, sender key message, group id true) from s.2, timestamp 7",
		"Envelope path: 1:1 (sealed sender v1, whisper message, group id true) from s.2, timestamp 7",
		"Envelope path: 1:1 (sealed sender v2, prekey message, group id false) from s.2, timestamp 7",
		"Envelope path: 1:1 (unsealed, prekey message, group id false) from s.2, timestamp 7",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}
