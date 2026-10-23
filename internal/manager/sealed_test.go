package manager

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

const sealedBACI = "b0b0b0b0-0000-4000-8000-000000000002"

func profileJSON(identity []byte, verifierKey []byte, unrestricted bool) string {
	ver := "AAAA"
	if verifierKey != nil {
		m := hmac.New(sha256.New, api.UnidentifiedAccessKey(verifierKey))
		m.Write(make([]byte, 32))
		ver = base64.StdEncoding.EncodeToString(m.Sum(nil))
	}
	return fmt.Sprintf(`{"identityKey":%q,"unidentifiedAccess":%q,"unrestrictedUnidentifiedAccess":%v,"capabilities":{"spqr":true}}`,
		base64.StdEncoding.EncodeToString(identity), ver, unrestricted)
}

// sealedWorld: A.2 sending to single-device B, with a test trust root
// and a sender certificate for A.2.
func sealedWorld(t *testing.T) (*fakeChat, *Sender, *store.Account, []byte) {
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a2, a2Bundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	b1, b1Bundles := testAccountFor(t, "+15555550101", sealedBACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	bKey := make([]byte, 32)
	rand.Read(bKey)
	root, _ := curve25519.GeneratePrivateKey(rand.Reader)
	fc := &fakeChat{t: t, sender: testACI, senderDev: 2, root: root,
		devices: map[string]map[uint32]fakeDevice{
			sealedBACI: {1: {b1, b1Bundles["aci"]}},
			testACI:    {2: {a2, a2Bundles["aci"]}},
		},
		cert:       mintCert(t, root, testACI, aID.PublicKey(), 2),
		profiles:   map[string]string{sealedBACI: profileJSON(bID.PublicKey().Bytes(), bKey, false)},
		accessKeys: map[string][]byte{sealedBACI: api.UnidentifiedAccessKey(bKey)},
	}
	s := &Sender{Acct: a2, Chat: fc, Unauth: unauthChat{fc}}
	return fc, s, b1, bKey
}

func lastDelivery(t *testing.T, fc *fakeChat) (*Message, string) {
	t.Helper()
	if len(fc.delivered) == 0 {
		t.Fatal("nothing delivered")
	}
	d := fc.delivered[0]
	var c signalpb.AppContent
	proto.Unmarshal(d.msg.Content, &c)
	return d.msg, c.GetDataMessage().GetBody()
}

// TestSealedSend: with the recipient's profile key and a matching
// verifier, the bundle fetch and the send go over the unauthenticated
// socket with the access key, as sealed sender v1 (send.md §3, §3.1).
func TestSealedSend(t *testing.T) {
	fc, s, _, bKey := sealedWorld(t)
	s.Acct.Recipients[sealedBACI] = store.Recipient{ProfileKey: bKey}
	if _, err := s.SendText(context.Background(), sealedBACI, "sealed hi"); err != nil {
		t.Fatalf("send: %v\n%v", err, fc.requests)
	}
	m, body := lastDelivery(t, fc)
	if !m.Sealed || body != "sealed hi" || m.Sender.ServiceID != testACI || m.Sender.DeviceID != 2 {
		t.Errorf("delivered %+v %q", m, body)
	}
	var k [32]byte
	copy(k[:], bKey)
	var u [16]byte
	copy(u[:], uuidBytes(sealedBACI))
	want := []string{"auth GET /v1/profile/" + sealedBACI + "/" + zkgroup.ProfileKeyVersion(k, u), "auth GET /v1/certificate/delivery?includeE164=false",
		"unauth GET /v2/keys/" + sealedBACI + "/*", "unauth PUT /v1/messages/" + sealedBACI + "?story=false"}
	if got := strings.Join(fc.requests[:4], ","); got != strings.Join(want, ",") {
		t.Errorf("requests\n%v\nwant\n%v", fc.requests, want)
	}
	if !strings.Contains(fc.puts[0], `"type":6,`) {
		t.Errorf("sealed entry type: %.120s", fc.puts[0])
	}
	if p := s.Acct.Recipients[sealedBACI].Profile; p == nil || p.AccessMode != store.AccessEnabled {
		t.Errorf("stored profile %+v", p)
	}

	// Within 6 hours the profile and the certificate are reused.
	fc.requests, fc.delivered, fc.puts = nil, nil, nil
	if _, err := s.SendText(context.Background(), sealedBACI, "again"); err != nil {
		t.Fatal(err)
	}
	var toB []string
	for _, r := range fc.requests {
		if strings.Contains(r, sealedBACI) || strings.Contains(r, "certificate") {
			toB = append(toB, r)
		}
	}
	if len(toB) != 1 || toB[0] != "unauth PUT /v1/messages/"+sealedBACI+"?story=false" {
		t.Errorf("second send requests %v", fc.requests)
	}
}

// TestUnsealedWithoutKey: no profile key and a restricted profile means an
// unsealed send; an unrestricted profile seals with the zero key.
func TestUnsealedWithoutKey(t *testing.T) {
	fc, s, _, _ := sealedWorld(t)
	if _, err := s.SendText(context.Background(), sealedBACI, "plain"); err != nil {
		t.Fatal(err)
	}
	if m, _ := lastDelivery(t, fc); m.Sealed || fc.putVia[0] != "auth" {
		t.Errorf("sealed without a key: via %v", fc.putVia)
	}
	if p := s.Acct.Recipients[sealedBACI].Profile; p.AccessMode != store.AccessDisabled {
		t.Errorf("mode %s", p.AccessMode)
	}

	fc, s, _, _ = sealedWorld(t)
	bID := fc.devices[sealedBACI][1].bundle.IdentityKey.Bytes()
	fc.profiles[sealedBACI] = profileJSON(bID, nil, true)
	fc.accessKeys[sealedBACI] = make([]byte, 16)
	if _, err := s.SendText(context.Background(), sealedBACI, "unrestricted"); err != nil {
		t.Fatal(err)
	}
	if m, _ := lastDelivery(t, fc); !m.Sealed {
		t.Error("unrestricted recipient not sealed")
	}
}

// TestSealed401: a 401 on the sealed PUT repeats it unchanged on the
// authenticated socket; the success is not unidentified, so the stored
// mode drops to unknown (send.md §3.3, §3.4).
func TestSealed401(t *testing.T) {
	fc, s, _, bKey := sealedWorld(t)
	s.Acct.Recipients[sealedBACI] = store.Recipient{ProfileKey: bKey}
	fc.sealed401 = true
	if _, err := s.SendText(context.Background(), sealedBACI, "fallback"); err != nil {
		t.Fatalf("send: %v\n%v", err, fc.requests)
	}
	if len(fc.puts) < 2 || fc.puts[0] != fc.puts[1] || fc.putVia[0] != "unauth" || fc.putVia[1] != "auth" {
		t.Errorf("PUTs via %v, identical %v", fc.putVia, len(fc.puts) >= 2 && fc.puts[0] == fc.puts[1])
	}
	if m, _ := lastDelivery(t, fc); !m.Sealed {
		t.Error("the repeated body should still be the sealed ciphertext")
	}
	if p := s.Acct.Recipients[sealedBACI].Profile; p.AccessMode != store.AccessUnknown {
		t.Errorf("mode after an unsealed success %s", p.AccessMode)
	}
	// The downgrade sticks: the next send is unsealed, without refetching.
	fc.sealed401 = false
	fc.requests, fc.delivered, fc.putVia = nil, nil, nil
	s.SendText(context.Background(), sealedBACI, "next")
	if m, _ := lastDelivery(t, fc); m.Sealed || fc.putVia[0] != "auth" {
		t.Errorf("after downgrade: sealed %v via %v", m.Sealed, fc.putVia)
	}
}

// TestProfileKeyFromMessage: a peer's 32-byte profile key in a data
// message is stored; other lengths are ignored (content.md §4.4).
func TestProfileKeyFromMessage(t *testing.T) {
	a, _ := testAccount(t)
	key := make([]byte, 32)
	key[0] = 9
	for _, k := range [][]byte{key, make([]byte, 31)} {
		c, _ := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_DataMessage{DataMessage: &signalpb.AppDataMessage{ProfileKey: k}}})
		if err := ApplyContent(a, &Message{Sender: bobAddrFor(), Content: c}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got := a.Recipients[bobACI].ProfileKey; len(got) != 32 || got[0] != 9 {
		t.Errorf("stored key %x", got)
	}
}
