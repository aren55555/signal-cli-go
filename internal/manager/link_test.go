// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// fakeProvisioning is the server end of a provisioning socket.
type fakeProvisioning struct {
	toClient chan []byte
	once     sync.Once
	closed   chan struct{}
}

func (f *fakeProvisioning) Read() ([]byte, error) {
	select {
	case b := <-f.toClient:
		return b, nil
	case <-f.closed:
		return nil, io.EOF
	}
}
func (f *fakeProvisioning) Write([]byte) error { return nil }
func (f *fakeProvisioning) Close() error       { f.once.Do(func() { close(f.closed) }); return nil }

func (f *fakeProvisioning) push(path string, body []byte) {
	b, _ := proto.Marshal(&signalpb.ChatWebSocketMessage{
		Type:    signalpb.ChatWebSocketMessage_REQUEST.Enum(),
		Request: &signalpb.ChatWebSocketRequest{Id: proto.Uint64(1), Verb: proto.String("PUT"), Path: proto.String(path), Body: body},
	})
	f.toClient <- b
}

const (
	testNumber = "+15555550100"
	testACI    = "0b6ab9b6-0000-4000-8000-00000000c5d3"
	testPNI    = "1c7bcac7-0000-4000-8000-00000000d6e4"
)

// TestLink plays the server and the primary for a full link
// (spec/service/linking.md §7) and checks the request and what is stored.
func TestLink(t *testing.T) {
	aciID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	pniID, _ := curve25519.GeneratePrivateKey(rand.Reader)
	profileKey := make([]byte, 32)
	rand.Read(profileKey)

	var linkErr string
	var gotPassword string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check := func(ok bool, what string) bool {
			if !ok && linkErr == "" {
				linkErr = what
			}
			return ok
		}
		user, pass, ok := r.BasicAuth()
		gotPassword = pass
		body, _ := io.ReadAll(r.Body)
		var req struct {
			VerificationCode  string `json:"verificationCode"`
			AccountAttributes struct {
				FetchesMessages   bool   `json:"fetchesMessages"`
				RegistrationID    uint32 `json:"registrationId"`
				PNIRegistrationID uint32 `json:"pniRegistrationId"`
				Name              string `json:"name"`
			} `json:"accountAttributes"`
			ACISigned, PNISigned, ACIKyber, PNIKyber api.SignedPreKeyEntity
		}
		json.Unmarshal([]byte(strings.NewReplacer(`"aciSignedPreKey"`, `"ACISigned"`, `"pniSignedPreKey"`, `"PNISigned"`,
			`"aciPqLastResortPreKey"`, `"ACIKyber"`, `"pniPqLastResortPreKey"`, `"PNIKyber"`).Replace(string(body))), &req)
		check(r.Method == "PUT" && r.URL.Path == "/v1/devices/link", "path")
		check(ok && user == testNumber && len(pass) == 24, "auth")
		check(req.VerificationCode == "123456", "code")
		a := req.AccountAttributes
		check(a.FetchesMessages && a.RegistrationID >= 1 && a.RegistrationID <= 16380 && a.PNIRegistrationID >= 1 && a.PNIRegistrationID <= 16380, "attributes")
		nameBytes, err := base64.RawStdEncoding.DecodeString(a.Name)
		name, err2 := protocol.DecryptDeviceName(aciID, nameBytes)
		check(err == nil && err2 == nil && name == "cli", "device name")
		for _, k := range []struct {
			e  api.SignedPreKeyEntity
			id *curve25519.PrivateKey
			n  int
		}{{req.ACISigned, aciID, 33}, {req.PNISigned, pniID, 33}, {req.ACIKyber, aciID, 1569}, {req.PNIKyber, pniID, 1569}} {
			pub, _ := api.DecodeBase64(k.e.PublicKey)
			sig, _ := api.DecodeBase64(k.e.Signature)
			check(len(pub) == k.n && k.id.PublicKey().Verify(pub, sig), "prekey signature")
		}
		w.Write([]byte(`{"uuid":"` + testACI + `","pni":"` + testPNI + `","deviceId":3}`))
	}))
	defer srv.Close()
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs

	fp := &fakeProvisioning{toClient: make(chan []byte, 4), closed: make(chan struct{})}
	addr, _ := proto.Marshal(&signalpb.ProvisioningAddress{Address: proto.String("q83vEjRWeJCrze8SNFZ4kA==")})
	fp.push("/v1/address", addr)

	root := store.Open(t.TempDir())
	l := &Linker{
		Root:         root,
		REST:         rest.New(rest.Config{BaseURL: srv.URL, Roots: pool}),
		OpenSocket:   func(context.Context) (*provision.Session, error) { return provision.NewSession(fp), nil },
		SkipPostLink: true,
		Now:          func() time.Time { return time.UnixMilli(1700000000000) },
		Rand:         rand.Reader,
		DeviceName:   "cli",
	}
	l.OnURL = func(u string) {
		_, pub, err := provision.ParseLinkURL(u)
		if err != nil {
			t.Error(err)
			return
		}
		msg, _ := proto.Marshal(&signalpb.ProvisionMessage{
			AciIdentityKeyPublic: aciID.PublicKey().Bytes(), AciIdentityKeyPrivate: aciID.Bytes(),
			PniIdentityKeyPublic: pniID.PublicKey().Bytes(), PniIdentityKeyPrivate: pniID.Bytes(),
			Number: proto.String(testNumber), ProvisioningCode: proto.String("123456"), ProfileKey: profileKey,
			Aci: proto.String(testACI), Pni: proto.String(testPNI), ProvisioningVersion: proto.Uint32(1),
			AccountEntropyPool: proto.String(strings.Repeat("a", 64)),
		})
		epub, body, _ := protocol.EncryptProvision(rand.Reader, pub, msg)
		env, _ := proto.Marshal(&signalpb.ProvisionEnvelope{PublicKey: epub, Body: body})
		fp.push("/v1/message", env)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	acct, err := l.Link(ctx)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	defer acct.Close()
	if linkErr != "" {
		t.Fatalf("link request: bad %s", linkErr)
	}
	d := acct.Data
	if d.Number != testNumber || d.ACI != testACI || d.PNI != testPNI || d.DeviceID != 3 || !d.Registered || d.Password != gotPassword ||
		d.Environment != store.EnvStaging || string(d.ProfileKey) != string(profileKey) || string(d.ACIIdentity.Private) != string(aciID.Bytes()) {
		t.Errorf("stored account %+v", d)
	}
	for name, ks := range map[string]struct {
		state store.PreKeyState
		keys  store.IdentityKeys
	}{"aci": {d.ACIKeys, acct.Keys.ACI}, "pni": {d.PNIKeys, acct.Keys.PNI}} {
		s := ks.state
		spk, ok1 := ks.keys.SignedPreKeys[uint32(s.ActiveSignedPreKeyID)]
		kpk, ok2 := ks.keys.KyberPreKeys[uint32(s.ActiveLastResortKyberID)]
		if s.ActiveSignedPreKeyID < 0 || s.ActiveLastResortKyberID < 0 || !ok1 || !ok2 || !kpk.LastResort ||
			s.NextSignedPreKeyID != (uint32(s.ActiveSignedPreKeyID)+1)%0xFFFFFF || s.NextKyberPreKeyID != (uint32(s.ActiveLastResortKyberID)+1)%0xFFFFFF {
			t.Errorf("%s keys: state %+v", name, s)
		}
		if _, err := protocol.LoadSignedPreKeyRecord(spk.Record); err != nil {
			t.Errorf("%s signed prekey: %v", name, err)
		}
	}

	// Linking the same number again is refused.
	fp2 := &fakeProvisioning{toClient: make(chan []byte, 4), closed: make(chan struct{})}
	fp2.push("/v1/address", addr)
	l.OpenSocket = func(context.Context) (*provision.Session, error) { return provision.NewSession(fp2), nil }
	acct.Close()
	fp = fp2
	if _, err := l.Link(ctx); !errors.Is(err, ErrUserExists) {
		t.Errorf("second link: %v", err)
	}
}
