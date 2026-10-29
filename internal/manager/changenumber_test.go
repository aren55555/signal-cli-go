// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"google.golang.org/protobuf/proto"
)

const (
	newNumber = "+15555550177"
	newPNI    = "a0a0a0a0-0000-4000-8000-0000000000ee"
)

// numberServer answers PUT /v2/accounts/number and keeps the body;
// other requests get an empty 200.
type numberServer struct {
	status int
	body   []byte
	paths  []string
}

func (n *numberServer) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	n.paths = append(n.paths, verb+" "+path)
	switch {
	case verb == "PUT" && path == "/v2/accounts/number":
		n.body = body
		if n.status != 200 {
			return &chat.Response{Status: n.status}, nil
		}
		return &chat.Response{Status: 200, Body: []byte(`{"uuid":"` + testACI + `","number":"` + newNumber + `","pni":"` + newPNI + `"}`)}, nil
	case strings.HasPrefix(path, "/v2/keys"):
		return &chat.Response{Status: 200, Body: []byte(`{"count":100,"pqCount":100}`)}, nil
	}
	return &chat.Response{Status: 200, Body: []byte(`{}`)}, nil
}

func codeServer(t *testing.T, status int) *rest.Client {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.Path != "/v1/verification/session/sess-1/code" || string(b) != `{"code":"123456"}` {
			t.Errorf("code request %s %s %s", r.Method, r.URL.Path, b)
		}
		w.WriteHeader(status)
		io.WriteString(w, `{"id":"sess-1","verified":true}`)
	}))
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return rest.New(rest.Config{BaseURL: srv.URL, Roots: srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs})
}

// changeNumberWorld: primary A.1 with a session to its linked A.2.
func changeNumberWorld(t *testing.T) (*Sender, *numberServer, *Receiver) {
	aID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	a1, _ := testAccountFor(t, testNumber, testACI, testPNI, 1, aID)
	a2, a2Bundles := testAccountFor(t, testNumber, testACI, testPNI, 2, aID)
	rec, err := new(protocol.SessionRecord).Initiate(rand.Reader, aID, a1.Data.ACIRegistrationID, a2Bundles["aci"], time.Now(),
		protocol.Address{ServiceID: testACI, DeviceID: 1}, protocol.Address{ServiceID: testACI, DeviceID: 2})
	if err != nil {
		t.Fatal(err)
	}
	ns := &numberServer{status: 200}
	s := &Sender{Acct: a1, Chat: ns}
	if err := s.saveSession(testACI, 2, rec); err != nil {
		t.Fatal(err)
	}
	a1.Data.SessionID, a1.Data.SessionNumber = "sess-1", newNumber
	return s, ns, &Receiver{Acct: a2, Decryptor: &Decryptor{Acct: a2}}
}

// TestFinishChangeNumber: change-number.md §3.2-3.5 and §4, and the
// linked device applying its pniChangeNumber (sync.md §6.13).
func TestFinishChangeNumber(t *testing.T) {
	s, ns, linked := changeNumberWorld(t)
	if err := s.FinishChangeNumber(context.Background(), codeServer(t, 200), newNumber, "123-456"); err != nil {
		t.Fatal(err)
	}
	// Member order and nulls.
	dec := json.NewDecoder(bytes.NewReader(ns.body))
	dec.Token()
	var keys []string
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var skip json.RawMessage
		dec.Decode(&skip)
	}
	if got := strings.Join(keys, ","); got != "sessionId,recoveryPassword,number,pniIdentityKey,deviceMessages,devicePniSignedPrekeys,pniRegistrationIds,reglock,devicePniPqLastResortPrekeys" {
		t.Errorf("member order %s", got)
	}
	var req struct {
		SessionID        string  `json:"sessionId"`
		RecoveryPassword *string `json:"recoveryPassword"`
		PNIIdentityKey   string  `json:"pniIdentityKey"`
		DeviceMessages   []outgoingMessage
		Signed           map[string]struct{ KeyID uint32 } `json:"devicePniSignedPrekeys"`
		RegIDs           map[string]uint32                 `json:"pniRegistrationIds"`
		Kyber            map[string]struct{ KeyID uint32 } `json:"devicePniPqLastResortPrekeys"`
	}
	if err := json.Unmarshal(ns.body, &req); err != nil {
		t.Fatal(err)
	}
	if req.SessionID != "sess-1" || len(req.PNIIdentityKey) != 44 || len(req.DeviceMessages) != 1 || req.DeviceMessages[0].DestinationDeviceID != 2 ||
		len(req.Signed) != 2 || len(req.RegIDs) != 2 || len(req.Kyber) != 2 || req.RegIDs["1"] == req.RegIDs["2"] {
		t.Fatalf("request %s", ns.body)
	}
	// Local state on the primary.
	a := s.Acct
	if a.Data.Number != newNumber || a.Data.PNI != newPNI || a.Data.SessionID != "" || a.Data.PNIRegistrationID != req.RegIDs["1"] ||
		a.Data.PNIKeys.ActiveSignedPreKeyID != int64(req.Signed["1"].KeyID) {
		t.Errorf("primary state %+v", a.Data)
	}
	if entries, _ := a.Root().Accounts(); entries[0].Number != newNumber {
		t.Errorf("index %+v", entries)
	}
	// The linked device decrypts and applies its entry.
	m := req.DeviceMessages[0]
	ct, _ := base64.StdEncoding.DecodeString(m.Content)
	env := &signalpb.ServiceEnvelope{Type: proto.Uint32(uint32(m.Type)), SourceServiceId: proto.String(testACI), SourceDeviceId: proto.Uint32(1),
		DestinationServiceId: proto.String(testACI), Content: ct, ClientTimestamp: proto.Uint64(1), UpdatedPni: proto.String(newPNI)}
	msg, err := linked.Decryptor.Decrypt(env)
	if err != nil {
		t.Fatal(err)
	}
	var c signalpb.AppContent
	if err := proto.Unmarshal(msg.Content, &c); err != nil {
		t.Fatal(err)
	}
	p := c.GetSyncMessage().GetPniChangeNumber()
	if p.GetNewE164() != newNumber || p.GetRegistrationId() != req.RegIDs["2"] {
		t.Fatalf("sync %v", p)
	}
	if !linked.applyPniChangeSync(env, 1, 5000, p) {
		t.Fatal("not applied")
	}
	if l := linked.Acct.Data; l.Number != newNumber || l.PNI != newPNI || !bytes.Equal(l.PNIIdentity.Public, a.Data.PNIIdentity.Public) {
		t.Errorf("linked state %+v", l)
	}
	if linked.applyPniChangeSync(env, 1, 5000, p) {
		t.Error("a replay was applied")
	}
}

// TestFinishChangeNumberErrors: §3.1 step 3 and §3.8.
func TestFinishChangeNumberErrors(t *testing.T) {
	s, ns, _ := changeNumberWorld(t)
	s.Acct.Data.SessionNumber = "+15555550178"
	err := s.FinishChangeNumber(context.Background(), codeServer(t, 200), newNumber, "123456")
	if err == nil || err.Error() != "No change number verification session active (IOException)" {
		t.Errorf("no session: %v", err)
	}
	for status, want := range map[int]string{401: "[401] Authorization failed! (AuthorizationFailedException)", 429: "[429] Rate Limited (RateLimitException)", 409: "[409]  (NonSuccessfulResponseCodeException)"} {
		s, ns, _ = changeNumberWorld(t)
		ns.status = status
		err := s.FinishChangeNumber(context.Background(), codeServer(t, 200), newNumber, "123456")
		if err == nil || err.Error() != want {
			t.Errorf("%d: %v", status, err)
		}
		if s.Acct.Data.Number != testNumber || s.Acct.Data.SessionID != "sess-1" {
			t.Errorf("%d: state changed", status)
		}
	}
}
