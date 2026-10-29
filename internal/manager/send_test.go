// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

type fakeDevice struct {
	acct   *store.Account
	bundle *protocol.PreKeyBundle
}

type delivery struct {
	to     string
	device uint32
	msg    *Message
}

// fakeChat plays the chat server: it serves bundles, enforces the
// device list of PUT /v1/messages with 409s, and hands each ciphertext to
// the recipient device's real Decryptor.
type fakeChat struct {
	t         *testing.T
	sender    string
	senderDev uint32
	devices   map[string]map[uint32]fakeDevice
	delivered []delivery
	requests  []string
	puts      []string
	putVia    []string
	// sealed sender
	profiles    map[string]string // aci -> unversioned profile JSON
	cert        []byte
	accessKeys  map[string][]byte // dest -> the access key the server accepts
	sealed401   bool              // answer sealed PUTs with 401
	mrPaths     []string          // multi_recipient request paths
	mrTokens    []string          // their group-send-token headers
	profilePuts []string          // PUT /v1/profile bodies
	forms       int               // upload forms handed out
	root        *curve25519.PrivateKey
}

func (f *fakeChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	return f.handle(false, verb, path, headers, body)
}

// unauthChat is the unauthenticated socket of the same fake server.
type unauthChat struct{ f *fakeChat }

func (u unauthChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	return u.f.handle(true, verb, path, headers, body)
}

func (f *fakeChat) handle(unauth bool, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	via := "auth"
	if unauth {
		via = "unauth"
	}
	f.requests = append(f.requests, via+" "+verb+" "+path)
	if verb == "PUT" && strings.HasPrefix(path, "/v1/messages/") {
		f.puts = append(f.puts, string(body))
		f.putVia = append(f.putVia, via)
	}
	if !unauth && verb == "PUT" && path == "/v1/profile" {
		f.profilePuts = append(f.profilePuts, string(body))
		if strings.Contains(string(body), `"sameAvatar":false`) {
			return &chat.Response{Status: 200, Body: []byte(`{"key":"profiles/NEW","credential":"c","acl":"private","algorithm":"AWS4-HMAC-SHA256","date":"d","policy":"p","signature":"s"}`)}, nil
		}
		return &chat.Response{Status: 200}, nil
	}
	if r := f.special(unauth, verb, path, headers); r != nil {
		return r, nil
	}
	if verb == "PUT" && strings.HasPrefix(path, "/v1/messages/multi_recipient") {
		return f.multiRecipient(unauth, path, headers, body), nil
	}
	switch {
	case verb == "GET" && strings.HasPrefix(path, "/v2/keys/"):
		parts := strings.Split(strings.TrimPrefix(path, "/v2/keys/"), "/")
		devs := f.devices[parts[0]]
		type entry struct {
			DeviceID       uint32                 `json:"deviceId"`
			RegistrationID uint32                 `json:"registrationId"`
			SignedPreKey   api.SignedPreKeyEntity `json:"signedPreKey"`
			PreKey         *api.PreKeyEntity      `json:"preKey,omitempty"`
			PqPreKey       api.SignedPreKeyEntity `json:"pqPreKey"`
		}
		var resp struct {
			IdentityKey string  `json:"identityKey"`
			Devices     []entry `json:"devices"`
		}
		for id, d := range devs {
			if parts[1] != "*" && parts[1] != fmt.Sprint(id) {
				continue
			}
			b := d.bundle
			resp.IdentityKey = base64.StdEncoding.EncodeToString(b.IdentityKey.Bytes())
			e := entry{DeviceID: id, RegistrationID: b.RegistrationID,
				SignedPreKey: api.SignedPreKeyEntity{KeyID: b.SignedPreKeyID, PublicKey: api.KeyBase64(b.SignedPreKey.Bytes()), Signature: api.KeyBase64(b.SignedPreKeySignature)},
				PqPreKey:     api.SignedPreKeyEntity{KeyID: b.KyberPreKeyID, PublicKey: api.KeyBase64(b.KyberPreKey.Bytes()), Signature: api.KeyBase64(b.KyberPreKeySignature)}}
			if b.PreKeyID != nil {
				e.PreKey = &api.PreKeyEntity{KeyID: *b.PreKeyID, PublicKey: api.KeyBase64(b.PreKey.Bytes())}
			}
			resp.Devices = append(resp.Devices, e)
		}
		if len(resp.Devices) == 0 {
			return &chat.Response{Status: 404}, nil
		}
		out, _ := json.Marshal(resp)
		return &chat.Response{Status: 200, Body: out}, nil

	case verb == "PUT" && strings.HasPrefix(path, "/v1/messages/"):
		dest, query, _ := strings.Cut(strings.TrimPrefix(path, "/v1/messages/"), "?")
		if query != "story=false" && query != "story=true" {
			f.t.Errorf("send query %q", query)
		}
		var req struct {
			Destination string `json:"destination"`
			Timestamp   uint64 `json:"timestamp"`
			Messages    []struct {
				Type                      uint32 `json:"type"`
				DestinationDeviceID       uint32 `json:"destinationDeviceId"`
				DestinationRegistrationID uint32 `json:"destinationRegistrationId"`
				Content                   string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Destination != dest {
			f.t.Errorf("bad send body %s", body)
			return &chat.Response{Status: 400}, nil
		}
		var want, got []uint32
		for id := range f.devices[dest] {
			if !(dest == f.sender && id == f.senderDev) {
				want = append(want, id)
			}
		}
		for _, m := range req.Messages {
			got = append(got, m.DestinationDeviceID)
		}
		var missing, extra []uint32
		for _, id := range want {
			if !slices.Contains(got, id) {
				missing = append(missing, id)
			}
		}
		for _, id := range got {
			if !slices.Contains(want, id) {
				extra = append(extra, id)
			}
		}
		if len(missing)+len(extra) > 0 {
			out, _ := json.Marshal(map[string][]uint32{"missingDevices": missing, "extraDevices": extra})
			return &chat.Response{Status: 409, Body: out}, nil
		}
		for _, m := range req.Messages {
			d := f.devices[dest][m.DestinationDeviceID]
			if m.DestinationRegistrationID != d.bundle.RegistrationID {
				f.t.Errorf("registration ID %d for device %d", m.DestinationRegistrationID, m.DestinationDeviceID)
			}
			wire, err := base64.StdEncoding.DecodeString(m.Content)
			if err != nil {
				f.t.Errorf("content not padded standard base64: %v", err)
			}
			env := &signalpb.ServiceEnvelope{Type: proto.Uint32(m.Type), SourceServiceIdBinary: uuidBytes(f.sender), SourceDeviceId: proto.Uint32(f.senderDev), ServerTimestamp: proto.Uint64(1),
				DestinationServiceIdBinary: uuidBytes(dest), Content: wire, ClientTimestamp: proto.Uint64(req.Timestamp)}
			if m.Type == 6 {
				env.SourceServiceIdBinary, env.SourceDeviceId = nil, nil
			}
			dec := &Decryptor{Acct: d.acct}
			if f.root != nil {
				dec.Roots = []*curve25519.PublicKey{f.root.PublicKey()}
			}
			msg, err := dec.Decrypt(env)
			if err != nil {
				f.t.Errorf("device %s.%d cannot decrypt: %v", dest, m.DestinationDeviceID, err)
				continue
			}
			f.receiveSKDM(d.acct, msg)
			f.delivered = append(f.delivered, delivery{dest, m.DestinationDeviceID, msg})
		}
		return &chat.Response{Status: 200, Body: []byte(`{"needsSync":true}`)}, nil
	}
	return &chat.Response{Status: 404}, nil
}

// TestSendText: A.2 sends to B (two devices) and the sent transcript goes
// to A.1 (spec/service/send.md, content.md §8-9).
func TestSendText(t *testing.T) {
	const bACI = "b0b0b0b0-0000-4000-8000-000000000002"
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a2, a2Bundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	a1, a1Bundles := testAccountFor(t, testNumber, testACI, testPNI, 1, aID)
	b1, b1Bundles := testAccountFor(t, "+15555550101", bACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	b2, b2Bundles := testAccountFor(t, "+15555550101", bACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 2, bID)
	fc := &fakeChat{t: t, sender: testACI, senderDev: 2, devices: map[string]map[uint32]fakeDevice{
		bACI:    {1: {b1, b1Bundles["aci"]}, 2: {b2, b2Bundles["aci"]}},
		testACI: {1: {a1, a1Bundles["aci"]}, 2: {a2, a2Bundles["aci"]}},
	}}
	s := &Sender{Acct: a2, Chat: fc}
	ts, err := s.SendText(context.Background(), bACI, "hello B")
	if err != nil {
		t.Fatalf("send: %v (requests %v)", err, fc.requests)
	}

	got := map[string]*signalpb.AppContent{}
	for _, d := range fc.delivered {
		var c signalpb.AppContent
		if err := proto.Unmarshal(d.msg.Content, &c); err != nil {
			t.Fatal(err)
		}
		got[fmt.Sprintf("%s.%d", d.to, d.device)] = &c
		if d.msg.ClientTimestamp != ts {
			t.Errorf("envelope timestamp %d, want %d", d.msg.ClientTimestamp, ts)
		}
	}
	for _, dev := range []string{bACI + ".1", bACI + ".2"} {
		dm := got[dev].GetDataMessage()
		if dm.GetBody() != "hello B" || dm.GetTimestamp() != ts || dm.GetExpireTimerVersion() != 1 || len(dm.GetProfileKey()) != 32 ||
			dm.RequiredProtocolVersion == nil || dm.GetRequiredProtocolVersion() != 0 || dm.ExpireTimer != nil {
			t.Errorf("%s got %v", dev, dm)
		}
	}
	sent := got[testACI+".1"].GetSyncMessage().GetSent()
	if formatUUIDBytes(sent.GetDestinationServiceIdBinary()) != bACI || sent.GetTimestamp() != ts || sent.GetMessage().GetBody() != "hello B" ||
		len(sent.GetUnidentifiedStatus()) != 1 || formatUUIDBytes(sent.GetUnidentifiedStatus()[0].GetDestinationServiceIdBinary()) != bACI {
		t.Errorf("sync transcript %v", sent)
	}
	if len(got[testACI+".1"].GetSyncMessage().GetPadding()) == 0 {
		t.Error("sync message without padding")
	}
	if _, ok := got[testACI+".2"]; ok {
		t.Error("sent a transcript to the sending device itself")
	}

	// The first PUT targeted device 1 only and drew a 409 for device 2
	// (send.md §4.1, §5); the body has signal-cli's member order.
	if len(fc.puts) < 3 || !strings.HasPrefix(fc.puts[0], `{"destination":"`+bACI+`","timestamp":`) ||
		!strings.Contains(fc.puts[0], `"messages":[{"type":3,"destinationDeviceId":1,"destinationRegistrationId":77,"content":"`) ||
		!strings.HasSuffix(fc.puts[0], `"online":false,"urgent":true}`) {
		t.Errorf("PUTs %d, first %.200s", len(fc.puts), fc.puts[0])
	}

	// A second send reuses the sessions: no bundle fetch, one PUT each.
	fc.requests, fc.puts, fc.delivered = nil, nil, nil
	if _, err := s.SendText(context.Background(), bACI, "again"); err != nil {
		t.Fatal(err)
	}
	for _, r := range fc.requests {
		if strings.Contains(r, "GET /v2/keys") {
			t.Errorf("second send fetched %s", r)
		}
	}
	if len(fc.puts) != 2 || len(fc.delivered) != 3 {
		t.Errorf("second send: %d PUTs, %d deliveries", len(fc.puts), len(fc.delivered))
	}
}

// special handles profiles, certificates and the unauthenticated
// socket's access check; nil means "not special".
func (f *fakeChat) special(unauth bool, verb, path string, headers []chat.Header) *chat.Response {
	switch {
	case !unauth && verb == "PUT" && path == "/v1/accounts/attributes":
		return &chat.Response{Status: 204}
	case !unauth && verb == "GET" && strings.HasPrefix(path, "/v4/attachments/form/upload"):
		f.forms++
		b, _ := json.Marshal(cdn.Form{CDN: 3, Key: fmt.Sprintf("form%d", f.forms), Headers: map[string]string{}, SignedUploadLocation: "https://x/attachments"})
		return &chat.Response{Status: 200, Body: b}
	case !unauth && verb == "GET" && path == "/v1/certificate/delivery?includeE164=false":
		if f.cert == nil {
			return &chat.Response{Status: 500}
		}
		b, _ := json.Marshal(map[string]string{"certificate": base64.StdEncoding.EncodeToString(f.cert)})
		return &chat.Response{Status: 200, Body: b}
	case verb == "GET" && strings.HasPrefix(path, "/v1/profile/"):
		if unauth {
			return &chat.Response{Status: 401}
		}
		p, ok := f.profiles[strings.SplitN(strings.TrimPrefix(path, "/v1/profile/"), "/", 2)[0]]
		if !ok {
			return &chat.Response{Status: 404}
		}
		return &chat.Response{Status: 200, Body: []byte(p)}
	case unauth && strings.HasPrefix(path, "/v1/messages/multi_recipient"):
		return nil // story authorization: no access check (stories.md §3.4)
	case unauth:
		var key string
		for _, h := range headers {
			if h.Name == "unidentified-access-key" {
				key = h.Value
			}
		}
		dest := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(path, "/v2/keys/"), "/v1/messages/"), "/", 2)[0]
		dest, query, _ := strings.Cut(dest, "?")
		if query == "story=true" && verb == "PUT" {
			return nil // access keys are not checked for story sends
		}
		if key == "" || key != base64.StdEncoding.EncodeToString(f.accessKeys[dest]) || (f.sealed401 && verb == "PUT") {
			return &chat.Response{Status: 401}
		}
	}
	return nil
}

// TestSendTyping: content.md §5.2 — online, not urgent, timestamp equal
// to the envelope's.
func TestSendTyping(t *testing.T) {
	const bACI = "b0b0b0b0-0000-4000-8000-000000000002"
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	bID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a2, a2Bundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	b1, b1Bundles := testAccountFor(t, "+15555550101", bACI, "b0b0b0b0-0000-4000-8000-0000000000f2", 1, bID)
	fc := &fakeChat{t: t, sender: testACI, senderDev: 2, devices: map[string]map[uint32]fakeDevice{
		bACI: {1: {b1, b1Bundles["aci"]}}, testACI: {2: {a2, a2Bundles["aci"]}}}}
	s := &Sender{Acct: a2, Chat: fc}
	ts, err := s.SendTyping(context.Background(), bACI, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fc.puts[0], `"online":true,"urgent":false}`) {
		t.Errorf("flags: %.300s", fc.puts[0])
	}
	var c signalpb.AppContent
	proto.Unmarshal(fc.delivered[0].msg.Content, &c)
	tm := c.GetTypingMessage()
	if tm == nil || tm.GetTimestamp() != ts || tm.GetAction() != signalpb.AppTyping_STOPPED || fc.delivered[0].msg.ClientTimestamp != ts {
		t.Errorf("typing %v at %d", tm, ts)
	}
	for _, p := range fc.puts[1:] {
		if strings.Contains(p, testACI) {
			t.Error("typing produced a sync transcript")
		}
	}
}
