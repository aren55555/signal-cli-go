// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/svr2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

const regNumber = "+15555550102"

// regServer plays the chat server's REST registration endpoints
// (spec/service/registration.md §2-4).
type regServer struct {
	t           *testing.T
	needCaptcha bool
	captchaOK   bool
	verified    bool
	correctCode string
	log         []string
	regBody     map[string]any
	password    string
	lockToken   string // non-empty: registration is locked to this token
	posts       int
}

func (s *regServer) meta(w http.ResponseWriter, status int) {
	allowed := !s.needCaptcha || s.captchaOK
	req := []string{}
	if !allowed {
		req = append(req, "captcha")
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"id": "sess-1", "nextSms": 0, "nextCall": 0, "nextVerificationAttempt": nil,
		"allowedToRequestCode": allowed, "requestedInformation": req, "verified": s.verified})
}

func (s *regServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.log = append(s.log, r.Method+" "+r.URL.Path)
	user, pass, ok := r.BasicAuth()
	if !ok || user != regNumber || len(pass) != 24 || (s.password != "" && pass != s.password) {
		s.t.Errorf("%s %s: auth %q", r.Method, r.URL.Path, user)
	}
	s.password = pass
	if r.Header.Get("X-Signal-Agent") == "" {
		s.t.Errorf("%s: no X-Signal-Agent", r.URL.Path)
	}
	switch r.Method + " " + r.URL.Path {
	case "POST /v1/verification/session":
		if string(body) != `{"number":"`+regNumber+`","mcc":"","mnc":""}` {
			s.t.Errorf("create body %s", body)
		}
		s.meta(w, 200)
	case "GET /v1/verification/session/sess-1":
		s.meta(w, 200)
	case "PATCH /v1/verification/session/sess-1":
		if string(body) != `{"captcha":"signal-hcaptcha.TOKEN"}` {
			s.t.Errorf("captcha body %s", body)
		}
		s.captchaOK = true
		s.meta(w, 200)
	case "POST /v1/verification/session/sess-1/code":
		var b map[string]string
		json.Unmarshal(body, &b)
		if b["transport"] != "sms" || b["client"] != "android" || len(b) != 2 || r.Header.Get("Accept-Language") != "en-US" {
			s.t.Errorf("code request %s, accept-language %q", body, r.Header.Get("Accept-Language"))
		}
		if !s.captchaOK && s.needCaptcha {
			s.meta(w, 409)
			return
		}
		s.meta(w, 200)
	case "PUT /v1/verification/session/sess-1/code":
		if string(body) == `{"code":"`+s.correctCode+`"}` {
			s.verified = true
		}
		s.meta(w, 200) // a wrong code also answers 200
	case "POST /v1/registration":
		if !s.verified {
			w.WriteHeader(401)
			return
		}
		json.Unmarshal(body, &s.regBody)
		s.posts++
		if s.lockToken != "" {
			attrs, _ := s.regBody["accountAttributes"].(map[string]any)
			if tok, _ := attrs["registrationLock"].(string); tok != s.lockToken {
				w.WriteHeader(423)
				w.Write([]byte(`{"timeRemaining":604799000,"svr2Credentials":{"username":"0123456789abcdef0123456789abcdef","password":"1:ab"}}`))
				return
			}
		}
		w.Write([]byte(`{"uuid":"` + testACI + `","pni":"` + testPNI + `","number":"` + regNumber + `","storageCapable":true}`))
	default:
		w.WriteHeader(404)
	}
}

func newRegistrar(t *testing.T, s *regServer) (*Registrar, *store.Root) {
	srv := httptest.NewTLSServer(s)
	t.Cleanup(srv.Close)
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	root := store.Open(t.TempDir())
	return &Registrar{Root: root, REST: rest.New(rest.Config{BaseURL: srv.URL, Roots: pool}), Language: "en-US"}, root
}

// TestRegisterAndVerify runs register (captcha required) and verify
// against the fake server and checks requests and stored state.
func TestRegisterAndVerify(t *testing.T) {
	s := &regServer{t: t, needCaptcha: true, correctCode: "123456"}
	r, root := newRegistrar(t, s)
	ctx := context.Background()

	var ce *CaptchaRequiredError
	if err := r.Register(ctx, regNumber, "", false, false); !errors.As(err, &ce) || ce.Given {
		t.Fatalf("register without captcha: %v", err)
	}
	// The account exists locally, unregistered, with its session.
	a, err := root.Find(regNumber)
	if err != nil || a.Data.Registered || a.Data.SessionID != "sess-1" || a.Data.DeviceID != 1 || len(a.Data.ProfileKey) != 32 ||
		a.Data.ACIRegistrationID < 1 || a.Data.ACIRegistrationID > 16380 || a.Data.ACIKeys.ActiveSignedPreKeyID != -1 {
		t.Fatalf("local account %+v, %v", a, err)
	}
	password := a.Data.Password
	a.Close()

	s.log = nil
	if err := r.Register(ctx, regNumber, "signalcaptcha://signal-hcaptcha.TOKEN", false, false); err != nil {
		t.Fatalf("register with captcha: %v (%v)", err, s.log)
	}
	want := "GET /v1/verification/session/sess-1,PATCH /v1/verification/session/sess-1,POST /v1/verification/session/sess-1/code"
	if strings.Join(s.log, ",") != want {
		t.Errorf("requests %v", s.log)
	}

	// A wrong code: the code submit answers 200, registration 401.
	if _, err := r.Verify(ctx, regNumber, "999-999", ""); err == nil {
		t.Fatal("wrong code accepted")
	}
	acct, err := r.Verify(ctx, regNumber, "123-456", "")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	defer acct.Close()
	d := acct.Data
	if !d.Registered || d.ACI != testACI || d.PNI != testPNI || d.SessionID != "" || d.Password != password || d.DeviceID != 1 {
		t.Errorf("registered account %+v", d)
	}
	for name, ks := range map[string]store.PreKeyState{"aci": d.ACIKeys, "pni": d.PNIKeys} {
		if ks.ActiveSignedPreKeyID < 0 || ks.ActiveLastResortKyberID < 0 {
			t.Errorf("%s active keys %+v", name, ks)
		}
	}
	if id := acct.Identities[testACI]; id.Trust != store.TrustedVerified || string(id.Key) != string(d.ACIIdentity.Public) {
		t.Errorf("own identity %+v", id)
	}

	// The registration body (registration.md §4.2).
	b := s.regBody
	if b["sessionId"] != "sess-1" || b["skipDeviceTransfer"] != true || b["requireAtomic"] != true {
		t.Errorf("envelope %v", b)
	}
	for _, k := range []string{"recoveryPassword", "gcmToken"} {
		if _, ok := b[k]; ok {
			t.Errorf("%s present", k)
		}
	}
	attrs := b["accountAttributes"].(map[string]any)
	for _, k := range []string{"signalingKey", "registrationLock", "name", "recoveryPassword"} {
		if v, ok := attrs[k]; !ok || v != nil {
			t.Errorf("attribute %s = %v (present %v), want null", k, v, ok)
		}
	}
	caps := attrs["capabilities"].(map[string]any)
	if attrs["voice"] != false || attrs["video"] != false || attrs["fetchesMessages"] != true || caps["attachmentBackfill"] != false || caps["spqr"] != true {
		t.Errorf("attributes %v", attrs)
	}
	uak, _ := base64.StdEncoding.DecodeString(attrs["unidentifiedAccessKey"].(string))
	if string(uak) != string(api.UnidentifiedAccessKey(d.ProfileKey)) {
		t.Error("unidentified access key")
	}
	ik, _ := api.DecodeBase64(b["aciIdentityKey"].(string))
	if string(ik) != string(d.ACIIdentity.Public) || strings.HasSuffix(b["aciIdentityKey"].(string), "=") {
		t.Error("aciIdentityKey")
	}
	spk := b["pniSignedPreKey"].(map[string]any)
	pub, _ := api.DecodeBase64(spk["publicKey"].(string))
	sig, _ := api.DecodeBase64(spk["signature"].(string))
	pniKey, _ := curve25519.ParsePublicKey(d.PNIIdentity.Public)
	if !pniKey.Verify(pub, sig) {
		t.Error("PNI signed prekey signature")
	}
}

// TestRegisterAlreadyRegistered: a registered account is refused without
// --reregister, before any request (svr.md §4.4 step 2.1, §5.3).
func TestRegisterAlreadyRegistered(t *testing.T) {
	s := &regServer{t: t}
	r, root := newRegistrar(t, s)
	a, _ := root.Create(regNumber, store.EnvStaging)
	a.Data.Registered, a.Data.Password = true, strings.Repeat("p", 24)
	a.Save()
	a.Close()
	if err := r.Register(context.Background(), regNumber, "", false, false); !errors.Is(err, ErrAlreadyRegistered) || len(s.log) != 0 {
		t.Errorf("register on a registered account: %v, requests %v", err, s.log)
	}
}

type profileServer struct{ body []byte }

func (p *profileServer) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	if verb == "PUT" && path == "/v1/profile" {
		p.body = body
		return &chat.Response{Status: 200, Body: []byte(`""`)}, nil
	}
	return &chat.Response{Status: 404}, nil
}

func decryptField(t *testing.T, key []byte, s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	pt, err := gcm.Open(nil, b[:12], b[12:], nil)
	if err != nil {
		t.Fatal(err)
	}
	return pt
}

// TestInitialProfile: profile.md §2.2.
func TestInitialProfile(t *testing.T) {
	a, _ := testAccount(t)
	rand.Read(a.Data.ProfileKey)
	ps := &profileServer{}
	if err := UploadInitialProfile(context.Background(), a, ps); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Version, Name, About, AboutEmoji, PhoneNumberSharing, Commitment string
		PaymentAddress                                                   *string
		Avatar, SameAvatar                                               bool
		BadgeIds                                                         []string
	}
	if err := json.Unmarshal(ps.body, &body); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ps.body), `{"version":"`) || !strings.Contains(string(ps.body), `"paymentAddress":null,`) || !strings.HasSuffix(string(ps.body), `"badgeIds":[]}`) {
		t.Errorf("body %s", ps.body)
	}
	var k [32]byte
	copy(k[:], a.Data.ProfileKey)
	aci, _ := uuidRaw(a.Data.ACI)
	if body.Version != zkgroup.ProfileKeyVersion(k, aci) {
		t.Error("version")
	}
	if c, _ := base64.StdEncoding.DecodeString(body.Commitment); string(c) != string(zkgroup.ProfileKeyCommitment(k, aci)) {
		t.Error("commitment")
	}
	for name, f := range map[string]struct {
		v string
		n int
	}{"name": {body.Name, 53}, "about": {body.About, 128}, "aboutEmoji": {body.AboutEmoji, 32}, "phoneNumberSharing": {body.PhoneNumberSharing, 1}} {
		pt := decryptField(t, a.Data.ProfileKey, f.v)
		if len(pt) != f.n || strings.Trim(string(pt), "\x00") != "" {
			t.Errorf("%s: %d bytes %x", name, len(pt), pt)
		}
	}
	if !body.Avatar || !body.SameAvatar || body.PaymentAddress != nil || body.BadgeIds == nil {
		t.Errorf("profile %+v", body)
	}
}

// fakeSVR2 answers restore requests from one stored backup.
type fakeSVR2 struct {
	user   string
	access map[string][32]byte // mrenclave -> access key
	blob   map[string][]byte
	tries  uint32
	cur    string
	reply  []byte
}

func (f *fakeSVR2) Send(b []byte) error {
	r := RestoreRequestPin(b)
	acc := f.access[f.cur]
	var out []byte
	if bytes.Equal(r, acc[:]) {
		out = restoreResponse(1, f.blob[f.cur], f.tries)
	} else {
		f.tries--
		out = restoreResponse(3, nil, f.tries)
	}
	f.reply = out
	return nil
}

func (f *fakeSVR2) Recv() ([]byte, error) { return f.reply, nil }

func TestVerifyRegistrationLock(t *testing.T) {
	if testing.Short() {
		t.Skip("Argon2id")
	}
	master := [32]byte{7, 7, 7}
	s := &regServer{t: t, correctCode: "123456", lockToken: svr2.RegistrationLock(master)}
	r, _ := newRegistrar(t, s)
	ctx := context.Background()
	if err := r.Register(ctx, regNumber, "", false, false); err != nil {
		t.Fatal(err)
	}
	// No PIN: the locked message with hours remaining (604799000 ms → 167).
	_, err := r.Verify(ctx, regNumber, "123456", "")
	var le *LockedError
	if !errors.As(err, &le) || !strings.Contains(le.Error(), "Hours remaining until reset: 167") {
		t.Fatalf("no pin: %v", err)
	}
	user := "0123456789abcdef0123456789abcdef"
	fs := &fakeSVR2{user: user, access: map[string][32]byte{}, blob: map[string][]byte{}, tries: 10}
	norm, _ := svr2.NormalizePIN("1234")
	for _, e := range svr2.StagingEnclaves {
		enc, acc, _ := svr2.PINHash(norm, svr2.Salt(user, e.GroupID))
		fs.access[e.MREnclave], fs.blob[e.MREnclave] = acc, svr2.EncryptMasterKey(enc, master)
	}
	r.DialSVR2 = func(ctx context.Context, e svr2.Enclave, u, p string) (cdsi.Channel, map[string][]byte, func(), error) {
		fs.cur = e.MREnclave
		return fs, map[string][]byte{"config": e.RaftConfig()}, func() {}, nil
	}
	// A wrong PIN stops at once with the tries left.
	_, err = r.Verify(ctx, regNumber, "123456", "9999")
	var pm *PinMismatchError
	if !errors.As(err, &pm) || pm.Tries != 9 {
		t.Fatalf("wrong pin: %v", err)
	}
	// The right PIN restores the key and the retry carries its token.
	a, err := r.Verify(ctx, regNumber, "123456", "1234")
	if err != nil {
		t.Fatalf("right pin: %v", err)
	}
	defer a.Close()
	if !bytes.Equal(a.Data.PinMasterKey, master[:]) || a.Data.RegistrationLockPin != "1234" || a.Data.AccountEntropyPool != "" {
		t.Errorf("stored key %x pin %q aep %q", a.Data.PinMasterKey, a.Data.RegistrationLockPin, a.Data.AccountEntropyPool)
	}
}

// RestoreRequestPin extracts the access key from a restore request.
func RestoreRequestPin(b []byte) []byte {
	if len(b) < 4+32 {
		return nil
	}
	return b[4 : 4+32]
}

func restoreResponse(status int, data []byte, tries uint32) []byte {
	inner := []byte{0x08, byte(status)}
	if data != nil {
		inner = append(append(inner, 0x12, byte(len(data))), data...)
	}
	inner = append(inner, 0x18, byte(tries))
	return append([]byte{0x12, byte(len(inner))}, inner...)
}
