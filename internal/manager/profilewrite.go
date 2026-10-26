package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/profile"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
)

// Writing the own profile (spec/service/profiles.md §3, §4).

// AvatarMode is what a profile write does with the avatar (§3.2).
type AvatarMode int

const (
	AvatarKeep AvatarMode = iota
	AvatarNew
	AvatarRemove
	AvatarRotate
)

// ProfileUpdate holds the values a caller supplies; nil keeps the stored
// value.
type ProfileUpdate struct {
	GivenName, FamilyName, About, AboutEmoji *string
	MobileCoinAddress                        []byte // nil keeps; non-nil (even empty) replaces
	Avatar                                   AvatarMode
	AvatarSource                             *attachment.Source // for AvatarNew
}

// ProfileError wraps a failure of a profile write (`Update profile
// error: <msg>`, §4.4).
type ProfileError struct{ Err error }

func (e *ProfileError) Error() string { return e.Err.Error() }
func (e *ProfileError) Unwrap() error { return e.Err }

// ErrProfileInputTooLong is a name, about or emoji over its largest
// padding target; signal-cli fails uncaught there (§4.4).
var ErrProfileInputTooLong = errors.New("Input is too long")

type avatarForm struct {
	Key        string `json:"key"`
	Credential string `json:"credential"`
	ACL        string `json:"acl"`
	Algorithm  string `json:"algorithm"`
	Date       string `json:"date"`
	Policy     string `json:"policy"`
	Signature  string `json:"signature"`
}

// WriteProfile writes the own profile with u (§3) and stores the result.
func (s *Sender) WriteProfile(ctx context.Context, u ProfileUpdate) error {
	a := s.Acct
	self := a.Data.ACI
	s.Profile(ctx, self)
	p := copyProfile(a.Recipients[self].Profile)
	for _, f := range []struct{ dst, src **string }{{&p.GivenName, &u.GivenName}, {&p.FamilyName, &u.FamilyName}, {&p.About, &u.About}, {&p.AboutEmoji, &u.AboutEmoji}} {
		if *f.src != nil {
			v := **f.src
			*f.dst = &v
		}
	}
	if u.MobileCoinAddress != nil {
		p.MobileCoinAddress = u.MobileCoinAddress
	}

	key := [32]byte(a.Data.ProfileKey)
	aci, ok := uuidRaw(self)
	if !ok {
		return &ProfileError{errors.New("account has no ACI")}
	}
	enc := func(f profile.Field, v *string) ([]byte, error) {
		val := ""
		if v != nil {
			val = *v
		}
		b, err := profile.EncryptString(key, profile.NewNonce(s.rand()), f, val)
		if err != nil {
			return nil, ErrProfileInputTooLong
		}
		return b, nil
	}
	name := profile.JoinName(p.GivenName, p.FamilyName)
	nameCT, err := enc(profile.FieldName, &name)
	if err != nil {
		return err
	}
	aboutCT, err := enc(profile.FieldAbout, p.About)
	if err != nil {
		return err
	}
	emojiCT, err := enc(profile.FieldAboutEmoji, p.AboutEmoji)
	if err != nil {
		return err
	}
	var payment []byte
	if p.MobileCoinAddress != nil {
		identity, err := curve25519.NewPrivateKey(a.Data.ACIIdentity.Private)
		if err != nil {
			return &ProfileError{err}
		}
		if payment, err = profile.EncryptPaymentAddress(key, profile.NewNonce(s.rand()), identity, p.MobileCoinAddress, s.rand()); err != nil {
			return &ProfileError{err}
		}
	}
	sharing := profile.EncryptBool(key, profile.NewNonce(s.rand()), a.Data.PhoneNumberSharingMode == "EVERYBODY")

	var image []byte
	contentType := ""
	mode := u.Avatar
	if mode == AvatarRotate {
		mode = AvatarKeep
		if src, err := attachment.FromArgument(filepath.Join(s.AvatarsDir, "profile-"+AvatarID(s.Acct, self)), ""); err == nil {
			mode, image, contentType = AvatarNew, src.Data, src.ContentType
		}
	} else if mode == AvatarNew {
		image, contentType = u.AvatarSource.Data, u.AvatarSource.ContentType
	}
	body := struct {
		Version            string   `json:"version"`
		Name               []byte   `json:"name"`
		About              []byte   `json:"about"`
		AboutEmoji         []byte   `json:"aboutEmoji"`
		PaymentAddress     []byte   `json:"paymentAddress"`
		PhoneNumberSharing []byte   `json:"phoneNumberSharing"`
		Avatar             bool     `json:"avatar"`
		SameAvatar         bool     `json:"sameAvatar"`
		Commitment         []byte   `json:"commitment"`
		BadgeIDs           []string `json:"badgeIds"`
	}{
		Version: zkgroup.ProfileKeyVersion(key, aci), Name: nameCT, About: aboutCT, AboutEmoji: emojiCT,
		PaymentAddress: payment, PhoneNumberSharing: sharing,
		Avatar: mode != AvatarRemove, SameAvatar: mode != AvatarNew,
		Commitment: zkgroup.ProfileKeyCommitment(key, aci), BadgeIDs: []string{},
	}
	b, err := api.Marshal(body)
	if err != nil {
		return &ProfileError{err}
	}
	r, err := s.Chat.Request(ctx, "PUT", "/v1/profile", jsonHeader, b)
	if err != nil {
		return &ProfileError{err}
	}
	if r.Status < 200 || r.Status > 299 {
		return &ProfileError{profileStatusError(r.Status)}
	}
	if mode == AvatarNew {
		var form avatarForm
		if err := json.Unmarshal(r.Body, &form); err != nil {
			return &ProfileError{err}
		}
		fields := []cdn.FormField{{Name: "acl", Value: form.ACL}, {Name: "key", Value: form.Key}, {Name: "policy", Value: form.Policy},
			{Name: "Content-Type", Value: contentType}, {Name: "x-amz-algorithm", Value: form.Algorithm},
			{Name: "x-amz-credential", Value: form.Credential}, {Name: "x-amz-date", Value: form.Date}, {Name: "x-amz-signature", Value: form.Signature}}
		if s.CDN == nil {
			return &ProfileError{errors.New("no CDN")}
		}
		if err := s.CDN.PostCDN0File(ctx, fields, contentType, profile.EncryptAvatar(key, profile.NewNonce(s.rand()), image)); err != nil {
			return &ProfileError{err}
		}
		p.AvatarURLPath = &form.Key
	}
	// §3.6: local avatar file, then the stored record (timestamp kept).
	file := filepath.Join(s.AvatarsDir, "profile-"+AvatarID(s.Acct, self))
	switch {
	case s.AvatarsDir == "":
	case mode == AvatarNew:
		if err := os.MkdirAll(s.AvatarsDir, 0o700); err == nil {
			os.WriteFile(file, image, 0o600)
		}
	case u.Avatar == AvatarRemove:
		os.Remove(file)
	}
	rc := a.Recipients[self]
	rc.Profile = p
	a.Recipients[self] = rc
	return a.SaveRecipients()
}

// profileStatusError is the message of a rejected profile PUT (§4.4).
func profileStatusError(status int) error {
	switch status {
	case 401:
		return errors.New("Authorization failed!")
	case 413, 429:
		return errors.New("Rate Limited")
	}
	return fmt.Errorf("StatusCode: %d", status)
}

// SendFetchLatest sends a fetchLatest sync of the given type to the own
// devices of a multi-device account (§3.6 step 3).
func (s *Sender) SendFetchLatest(ctx context.Context, t signalpb.AppSyncMessage_FetchLatest_Type) error {
	if !s.Acct.IsMultiDevice() {
		return nil
	}
	return s.SendSync(ctx, &signalpb.AppSyncMessage{FetchLatest: &signalpb.AppSyncMessage_FetchLatest{Type: t.Enum()}}, false)
}
