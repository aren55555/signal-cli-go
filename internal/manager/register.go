package manager

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Registration errors (spec/service/registration.md §2.5, §4, §6).
var (
	ErrNoSession       = errors.New("No registration verification session active")
	ErrCodeExpired     = errors.New("Verification code expired, please request a new one by registering again.")
	ErrCaptchaRejected = errors.New("Captcha not accepted")
	ErrCaptchaInvalid  = errors.New("Captcha has invalid format")
)

// CaptchaRequiredError: the server wants a captcha. Given tells whether
// one was supplied (and rejected); NextAt is when the next may be given.
type CaptchaRequiredError struct {
	Given  bool
	NextAt time.Time
}

func (e *CaptchaRequiredError) Error() string { return "captcha required" }

// MethodUnavailableError: the requested transport cannot be used now.
type MethodUnavailableError struct{ Voice bool }

func (e *MethodUnavailableError) Error() string { return "verification method not available" }

// sessionMeta is the session metadata (§2.4).
type sessionMeta struct {
	ID                      string   `json:"id"`
	NextSms                 *int64   `json:"nextSms"`
	NextCall                *int64   `json:"nextCall"`
	NextVerificationAttempt *int64   `json:"nextVerificationAttempt"`
	AllowedToRequestCode    bool     `json:"allowedToRequestCode"`
	RequestedInformation    []string `json:"requestedInformation"`
	Verified                bool     `json:"verified"`
}

func (m *sessionMeta) challenged() bool {
	return slices.Contains(m.RequestedInformation, "captcha") || slices.Contains(m.RequestedInformation, "pushChallenge")
}

// Registrar runs register and verify for a primary device.
type Registrar struct {
	Root         *store.Root
	REST         *rest.Client
	Language     string // Accept-Language for the code request, e.g. "en-US"
	Rand         io.Reader
	Now          func() time.Time
	PostRegister func(context.Context, *store.Account) error // prekey refresh, profile
}

func (r *Registrar) rand() io.Reader {
	if r.Rand != nil {
		return r.Rand
	}
	return rand.Reader
}

func (r *Registrar) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// newAccount creates the local material of §1.
func (r *Registrar) newAccount(number string) (*store.Account, error) {
	a, err := r.Root.Create(number, store.EnvStaging)
	if err != nil {
		return nil, err
	}
	rd := r.rand()
	d := &a.Data
	d.DeviceID = 1
	if d.Password, err = newPassword(rd); err != nil {
		return nil, err
	}
	d.ProfileKey = make([]byte, 32)
	if _, err := io.ReadFull(rd, d.ProfileKey); err != nil {
		return nil, err
	}
	for _, pair := range []*store.KeyPair{&d.ACIIdentity, &d.PNIIdentity} {
		k, err := curve25519.GeneratePrivateKey(rd)
		if err != nil {
			return nil, err
		}
		*pair = store.KeyPair{Public: k.PublicKey().Bytes(), Private: k.Bytes()}
	}
	for _, id := range []*uint32{&d.ACIRegistrationID, &d.PNIRegistrationID} {
		v, err := randUint32(rd, 16380)
		if err != nil {
			return nil, err
		}
		*id = v + 1
	}
	for _, s := range []*store.PreKeyState{&d.ACIKeys, &d.PNIKeys} {
		for _, c := range []*uint32{&s.NextPreKeyID, &s.NextSignedPreKeyID, &s.NextKyberPreKeyID} {
			if *c, err = randUint32(rd, idSpace); err != nil {
				return nil, err
			}
		}
		s.ActiveSignedPreKeyID, s.ActiveLastResortKyberID = -1, -1
	}
	return a, a.Save()
}

func (r *Registrar) auth(a *store.Account) *rest.Basic {
	return &rest.Basic{User: a.Data.Number, Password: a.Data.Password}
}

// call makes a session call and decodes the metadata, also from a 409 or
// 429 body.
func (r *Registrar) call(ctx context.Context, a *store.Account, method, path string, headers map[string]string, body any) (*sessionMeta, int, error) {
	var m sessionMeta
	err := r.REST.DoHeaders(ctx, method, path, r.auth(a), headers, body, &m)
	var se *rest.StatusError
	var rl *rest.RateLimitError
	switch {
	case err == nil:
		return &m, 200, nil
	case errors.As(err, &se):
		if json.Unmarshal(se.Body, &m) == nil && m.ID != "" {
			return &m, se.Status, err
		}
		return nil, se.Status, err
	case errors.As(err, &rl):
		json.Unmarshal(rl.Body, &m)
		return &m, rl.Status, err
	}
	return nil, 0, err
}

// Register runs `register` (§2-3): the local account, the verification
// session, an optional captcha and the code request.
func (r *Registrar) Register(ctx context.Context, number, captcha string, voice, reregister bool) error {
	a, err := r.Root.Find(number)
	switch {
	case errors.Is(err, store.ErrNoAccount):
		if a, err = r.newAccount(number); err != nil {
			return err
		}
	case err != nil:
		return err
	}
	defer a.Close()
	if a.Data.Registered && !reregister {
		return nil
	}
	captcha = strings.ReplaceAll(captcha, "signalcaptcha://", "")

	submitCaptcha := func(m *sessionMeta) (*sessionMeta, error) {
		if captcha == "" {
			return nil, &CaptchaRequiredError{}
		}
		nm, status, err := r.call(ctx, a, "PATCH", "/v1/verification/session/"+m.ID, nil, map[string]string{"captcha": captcha})
		switch {
		case status == 403 || (status == 409 && nm != nil && nm.challenged()):
			return nil, ErrCaptchaRejected
		case status == 400:
			return nil, ErrCaptchaInvalid
		case err != nil:
			return nil, err
		}
		return nm, nil
	}

	// 1. Get a session.
	var m *sessionMeta
	var status int
	if a.Data.SessionID != "" {
		m, status, err = r.call(ctx, a, "GET", "/v1/verification/session/"+a.Data.SessionID, nil, nil)
	}
	if a.Data.SessionID == "" || status == 404 {
		m, status, err = r.call(ctx, a, "POST", "/v1/verification/session", nil,
			struct {
				Number string `json:"number"`
				MCC    string `json:"mcc"`
				MNC    string `json:"mnc"`
			}{number, "", ""})
	}
	if status == 409 && m != nil && m.challenged() {
		if m, err = submitCaptcha(m); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// 2. Store it.
	a.Data.SessionID = m.ID
	if err := a.Save(); err != nil {
		return err
	}
	// 3-6.
	if !m.Verified && !m.AllowedToRequestCode {
		next := m.NextSms
		if voice {
			next = m.NextCall
		}
		switch {
		case next == nil:
			return &MethodUnavailableError{Voice: voice}
		case *next > 0:
			return &rest.RateLimitError{Status: 429, After: time.Duration(*next) * time.Second}
		}
		if v := m.NextVerificationAttempt; v != nil && *v > 0 {
			return &CaptchaRequiredError{Given: captcha != "", NextAt: r.now().Add(time.Duration(*v) * time.Second)}
		}
		if slices.Contains(m.RequestedInformation, "captcha") {
			if m, err = submitCaptcha(m); err != nil {
				var ce *CaptchaRequiredError
				if errors.As(err, &ce) {
					ce.Given = captcha != ""
				}
				return err
			}
			if !m.AllowedToRequestCode {
				return &CaptchaRequiredError{Given: captcha != ""}
			}
		}
	}
	// §3: request the code.
	transport := "sms"
	if voice {
		transport = "voice"
	}
	lang := r.Language
	if lang == "" {
		lang = "en-US"
	}
	m, status, err = r.call(ctx, a, "POST", "/v1/verification/session/"+a.Data.SessionID+"/code",
		map[string]string{"Accept-Language": lang}, map[string]string{"transport": transport, "client": "android"})
	switch {
	case status == 409 && m != nil && m.challenged():
		return &CaptchaRequiredError{Given: captcha != ""}
	case err != nil:
		return err
	}
	a.Data.Registered = false
	return a.Save()
}

// registrationRequest is POST /v1/registration's body (§4.2): envelope
// nulls omitted, account-attribute nulls emitted.
type registrationRequest struct {
	SessionID             string                  `json:"sessionId,omitempty"`
	AccountAttributes     api.AccountAttributes   `json:"accountAttributes"`
	ACIIdentityKey        string                  `json:"aciIdentityKey"`
	PNIIdentityKey        string                  `json:"pniIdentityKey"`
	ACISignedPreKey       *api.SignedPreKeyEntity `json:"aciSignedPreKey"`
	PNISignedPreKey       *api.SignedPreKeyEntity `json:"pniSignedPreKey"`
	ACIPqLastResortPreKey *api.SignedPreKeyEntity `json:"aciPqLastResortPreKey"`
	PNIPqLastResortPreKey *api.SignedPreKeyEntity `json:"pniPqLastResortPreKey"`
	SkipDeviceTransfer    bool                    `json:"skipDeviceTransfer"`
	RequireAtomic         bool                    `json:"requireAtomic"`
}

// Verify runs `verify CODE` (§4, §7): submit the code, register, store.
func (r *Registrar) Verify(ctx context.Context, number, code, pin string) (*store.Account, error) {
	a, err := r.Root.Find(number)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			a.Close()
		}
	}()
	if a.Data.SessionID == "" {
		return nil, ErrNoSession
	}
	code = strings.ReplaceAll(code, "-", "")
	d := &a.Data
	rd := r.rand()
	now := r.now()
	if d.PNIIdentity.Private == nil {
		k, err := curve25519.GeneratePrivateKey(rd)
		if err != nil {
			return nil, err
		}
		d.PNIIdentity = store.KeyPair{Public: k.PublicKey().Bytes(), Private: k.Bytes()}
	}
	type sideKeys struct {
		signed *protocol.SignedPreKeyRecord
		kyber  *protocol.KyberPreKeyRecord
	}
	var sides [2]sideKeys
	for i, side := range []struct {
		pair  store.KeyPair
		state *store.PreKeyState
	}{{d.ACIIdentity, &d.ACIKeys}, {d.PNIIdentity, &d.PNIKeys}} {
		id, err := curve25519.NewPrivateKey(side.pair.Private)
		if err != nil {
			return nil, err
		}
		if sides[i].signed, err = protocol.GenerateSignedPreKey(rd, id, side.state.NextSignedPreKeyID, now); err != nil {
			return nil, err
		}
		if sides[i].kyber, err = protocol.GenerateKyberPreKey(rd, id, side.state.NextKyberPreKeyID, now); err != nil {
			return nil, err
		}
	}

	m, status, err := r.call(ctx, a, "PUT", "/v1/verification/session/"+d.SessionID+"/code", nil, map[string]string{"code": code})
	switch {
	case status == 409 && m != nil && m.Verified:
	case status == 409 && m != nil && m.NextVerificationAttempt == nil:
		return nil, ErrCodeExpired
	case err != nil:
		return nil, err
	}

	attrs := accountAttributes(a)
	attrs.Capabilities = api.PrimaryDeviceCapabilities
	attrs.Name = nil
	req := registrationRequest{
		SessionID: d.SessionID, AccountAttributes: attrs,
		ACIIdentityKey: api.KeyBase64(d.ACIIdentity.Public), PNIIdentityKey: api.KeyBase64(d.PNIIdentity.Public),
		ACISignedPreKey: api.SignedPreKey(sides[0].signed), PNISignedPreKey: api.SignedPreKey(sides[1].signed),
		ACIPqLastResortPreKey: api.KyberPreKey(sides[0].kyber), PNIPqLastResortPreKey: api.KyberPreKey(sides[1].kyber),
		SkipDeviceTransfer: true, RequireAtomic: true,
	}
	var resp struct {
		UUID           string `json:"uuid"`
		PNI            string `json:"pni"`
		Number         string `json:"number"`
		StorageCapable bool   `json:"storageCapable"`
	}
	if err := r.REST.Do(ctx, "POST", "/v1/registration", r.auth(a), req, &resp); err != nil {
		return nil, err
	}
	if _, ok := uuidRaw(resp.UUID); !ok {
		return nil, fmt.Errorf("registration response without a valid uuid")
	}
	if _, ok := uuidRaw(resp.PNI); !ok {
		return nil, fmt.Errorf("registration response without a valid pni")
	}

	// §7 step 1: local state.
	d.ACI, d.PNI, d.Registered, d.DeviceID, d.DeviceName, d.SessionID = resp.UUID, resp.PNI, true, 1, "", ""
	for i, side := range []struct {
		state *store.PreKeyState
		keys  *store.IdentityKeys
	}{{&d.ACIKeys, &a.Keys.ACI}, {&d.PNIKeys, &a.Keys.PNI}} {
		sk, kk := sides[i].signed, sides[i].kyber
		if side.keys.SignedPreKeys == nil {
			side.keys.SignedPreKeys = map[uint32]store.StoredKey{}
		}
		if side.keys.KyberPreKeys == nil {
			side.keys.KyberPreKeys = map[uint32]store.StoredKey{}
		}
		side.keys.SignedPreKeys[sk.ID] = store.StoredKey{Record: sk.Serialize()}
		side.keys.KyberPreKeys[kk.ID] = store.StoredKey{Record: kk.Serialize(), LastResort: true}
		side.state.ActiveSignedPreKeyID, side.state.ActiveLastResortKyberID = int64(sk.ID), int64(kk.ID)
		side.state.NextSignedPreKeyID = (sk.ID + 1) % idSpace
		side.state.NextKyberPreKeyID = (kk.ID + 1) % idSpace
	}
	for k, b := range a.Sessions {
		if rec, err := protocol.LoadSessionRecord(b); err == nil {
			if nb, err := rec.ArchiveCurrent().Serialize(); err == nil {
				a.Sessions[k] = nb
			}
		}
	}
	a.Identities[d.ACI] = store.Identity{Key: d.ACIIdentity.Public, Trust: store.TrustedVerified, Added: now.UnixMilli()}
	a.Identities["PNI:"+d.PNI] = store.Identity{Key: d.PNIIdentity.Public, Trust: store.TrustedVerified, Added: now.UnixMilli()}
	for _, save := range []func() error{a.Save, a.SaveKeys, a.SaveSessions, a.SaveIdentities} {
		if err := save(); err != nil {
			return nil, err
		}
	}
	ok = true
	if r.PostRegister != nil {
		if err := r.PostRegister(ctx, a); err != nil {
			return a, fmt.Errorf("registered, but the follow-up failed: %w", err)
		}
	}
	return a, nil
}

// encryptProfileField pads plaintext with zeros to n bytes and encrypts
// it with AES-256-GCM under the profile key: nonce || ciphertext || tag
// (profile.md §2.2).
func encryptProfileField(key, plaintext []byte, n int) (string, error) {
	pt := make([]byte, n)
	copy(pt, plaintext)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, pt, nil)), nil
}

// UploadInitialProfile sends the empty versioned profile of profile.md
// §2.2.
func UploadInitialProfile(ctx context.Context, a *store.Account, c Requester) error {
	var key [32]byte
	copy(key[:], a.Data.ProfileKey)
	aci, ok := uuidRaw(a.Data.ACI)
	if !ok {
		return fmt.Errorf("account has no ACI")
	}
	var body struct {
		Version            string   `json:"version"`
		Name               string   `json:"name"`
		About              string   `json:"about"`
		AboutEmoji         string   `json:"aboutEmoji"`
		PaymentAddress     *string  `json:"paymentAddress"`
		PhoneNumberSharing string   `json:"phoneNumberSharing"`
		Avatar             bool     `json:"avatar"`
		SameAvatar         bool     `json:"sameAvatar"`
		Commitment         []byte   `json:"commitment"`
		BadgeIDs           []string `json:"badgeIds"`
	}
	body.Version = zkgroup.ProfileKeyVersion(key, aci)
	var err error
	for _, f := range []struct {
		dst *string
		pt  []byte
		n   int
	}{{&body.Name, nil, 53}, {&body.About, nil, 128}, {&body.AboutEmoji, nil, 32}, {&body.PhoneNumberSharing, []byte{0}, 1}} {
		if *f.dst, err = encryptProfileField(key[:], f.pt, f.n); err != nil {
			return err
		}
	}
	body.Avatar, body.SameAvatar = true, true
	body.Commitment = zkgroup.ProfileKeyCommitment(key, aci)
	body.BadgeIDs = []string{}
	b, err := api.Marshal(body)
	if err != nil {
		return err
	}
	r, err := c.Request(ctx, "PUT", "/v1/profile", jsonHeader, b)
	if err != nil {
		return err
	}
	if r.Status < 200 || r.Status > 299 {
		return &SendStatusError{Status: r.Status}
	}
	return nil
}
