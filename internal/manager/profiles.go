package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/profile"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Fetching and storing profiles (spec/service/profiles.md §5, §6).

const (
	profileAvatarLimit = 10 << 20
	zeroUUID           = "00000000-0000-0000-0000-000000000000"
)

func (s *Sender) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

// profileKeyOf is the profile key of the row under key; for self, the
// account's own key.
func (s *Sender) profileKeyOf(key string) []byte {
	if key == s.Acct.Data.ACI {
		return s.Acct.Data.ProfileKey
	}
	return s.Acct.Recipients[key].ProfileKey
}

// Profile returns key's profile, fetching it first when missing or 6 h
// old (§5.1, lazy).
func (s *Sender) Profile(ctx context.Context, key string) *store.Profile {
	rc := s.Acct.Recipients[key]
	if rc.Profile == nil || s.now().Sub(time.UnixMilli(rc.Profile.LastUpdate)) >= profileMaxAge {
		s.retrieveProfile(ctx, key, false)
	}
	return s.Acct.Recipients[key].Profile
}

// RefreshProfile fetches key's profile now (§5.1, forced) and returns
// what was stored.
func (s *Sender) RefreshProfile(ctx context.Context, key string) *store.Profile {
	s.retrieveProfile(ctx, key, false)
	return s.Acct.Recipients[key].Profile
}

// ProfileKeyCredential returns a valid expiring profile-key credential for
// key, fetching one when none is stored (§5.6).
func (s *Sender) ProfileKeyCredential(ctx context.Context, key string) (*zkgroup.ExpiringProfileKeyCredential, error) {
	if c := s.validCredential(key); c != nil {
		return c, nil
	}
	if len(s.profileKeyOf(key)) != 32 {
		return nil, errors.New("no profile key")
	}
	if err := s.retrieveProfile(ctx, key, true); err != nil {
		return nil, err
	}
	if c := s.validCredential(key); c != nil {
		return c, nil
	}
	return nil, errors.New("no profile key credential")
}

func (s *Sender) validCredential(key string) *zkgroup.ExpiringProfileKeyCredential {
	b := s.Acct.Recipients[key].ProfileKeyCredential
	if len(b) == 0 {
		return nil
	}
	c, err := zkgroup.ParseExpiringProfileKeyCredential(b)
	if err != nil || uint64(s.now().Unix()) >= c.Expiration {
		return nil
	}
	return c
}

// identifier is how logs name a recipient: ACI, else number, else PNI.
func (s *Sender) identifier(key string) string {
	ad := s.Acct.AddressOf(key)
	switch {
	case ad.ACI != "":
		return ad.ACI
	case ad.Number != "":
		return ad.Number
	}
	return "PNI:" + ad.PNI
}

// AvatarID is the file-name id of a recipient's avatars (§1.3): number,
// else ACI, else PNI:<uuid>.
func AvatarID(a *store.Account, key string) string {
	ad := a.AddressOf(key)
	switch {
	case key == a.Data.ACI && a.Data.Number != "":
		return a.Data.Number
	case ad.Number != "":
		return ad.Number
	case ad.ACI != "":
		return ad.ACI
	}
	return "PNI:" + ad.PNI
}

// storedAccess is sealed-sender access computable without a fetch
// (§5.2 socket choice).
func (s *Sender) storedAccess(ctx context.Context, key string) *access {
	if s.Unauth == nil || key == s.Acct.Data.ACI {
		return nil
	}
	rc := s.Acct.Recipients[key]
	if rc.Profile == nil {
		return nil
	}
	var k []byte
	switch {
	case rc.Profile.AccessMode == store.AccessUnrestricted:
		k = make([]byte, 16)
	case rc.Profile.AccessMode == store.AccessEnabled && len(rc.ProfileKey) == 32:
		k = accessKey(rc.ProfileKey)
	default:
		return nil
	}
	cert := s.senderCertificate(ctx)
	if cert == nil {
		return nil
	}
	return &access{key: k, cert: cert}
}

// retrieveProfile is one fetch with all of §5.4/§5.5's bookkeeping.
func (s *Sender) retrieveProfile(ctx context.Context, key string, credential bool) error {
	a := s.Acct
	ad := a.AddressOf(key)
	pk := s.profileKeyOf(key)
	var path string
	var rctx *zkgroup.ProfileKeyCredentialRequestContext
	switch {
	case len(pk) == 32 && ad.ACI == "":
		err := errors.New("ServiceId must be an ACI if a profile key is available!")
		s.profileFailed(key, err, false)
		return err
	case len(pk) == 32:
		u, _ := uuidRaw(ad.ACI)
		k := [32]byte(pk)
		path = "/v1/profile/" + ad.ACI + "/" + zkgroup.ProfileKeyVersion(k, u)
		if credential {
			var r [32]byte
			s.rand().Read(r[:])
			rctx = zkgroup.NewProfileKeyCredentialRequestContext(u, k, r)
			path += "/" + hex.EncodeToString(rctx.Request()) + "?credentialType=expiringProfileKey"
		}
	case ad.ACI != "":
		path = "/v1/profile/" + ad.ACI
	case ad.PNI != "":
		path = "/v1/profile/PNI:" + ad.PNI
	default:
		path = "/v1/profile/" + zeroUUID // number-only: CDSI did not resolve it
	}
	headers := []chat.Header{{Name: "accept-language", Value: "en;q=1"}}
	var r *chat.Response
	var err error
	if acc := s.storedAccess(ctx, key); acc != nil {
		r, err = s.Unauth.Request(ctx, "GET", path, append(headers, acc.header()), nil)
		if err == nil && r.Status == 401 {
			r, err = s.Chat.Request(ctx, "GET", path, headers, nil)
		}
	} else {
		r, err = s.Chat.Request(ctx, "GET", path, headers, nil)
	}
	if err != nil {
		s.profileFailed(key, err, false)
		return err
	}
	if r.Status != 200 {
		err := fmt.Errorf("StatusCode: %d", r.Status)
		s.profileFailed(key, err, r.Status == 404)
		return err
	}
	resp, err := profile.ParseResponse(r.Body)
	if err == nil && resp.Capabilities == nil {
		err = errors.New("profile response without capabilities")
	}
	if err != nil {
		s.profileFailed(key, err, false)
		return err
	}
	// 1. Credential.
	var newCred []byte
	if rctx != nil && resp.Credential != nil {
		if b, derr := base64.StdEncoding.DecodeString(*resp.Credential); derr == nil {
			c, verr := rctx.Receive(ServerParams(), b, uint64(s.now().Unix()))
			if verr != nil {
				s.profileFailed(key, verr, false)
				return verr
			}
			newCred = c.Serialize()
		}
	}
	rc := a.Recipients[key]
	if rctx != nil || s.validCredential(key) == nil {
		rc.ProfileKeyCredential = newCred
	}
	old := rc.Profile
	now := s.now().UnixMilli()
	var p *store.Profile
	if len(pk) == 32 {
		s.avatarStep(ctx, key, &rc, [32]byte(pk), resp.Avatar)
		if d, derr := profile.Decode([32]byte(pk), resp); derr == nil {
			p = &store.Profile{LastUpdate: now, AccessMode: d.AccessMode, Capabilities: d.Capabilities,
				GivenName: d.GivenName, FamilyName: d.FamilyName, About: d.About, AboutEmoji: d.AboutEmoji,
				AvatarURLPath: d.AvatarURLPath, MobileCoinAddress: d.MobileCoinAddress, PhoneNumberSharing: d.PhoneNumberSharing}
		}
	}
	if p == nil {
		p = copyProfile(old)
		p.LastUpdate, p.AccessMode, p.Capabilities = now, resp.ModeWithoutKey(), resp.CapabilityList()
	}
	if key == a.Data.ACI && resp.Unrestricted != a.Data.UnrestrictedUnidentifiedAccess {
		a.Data.UnrestrictedUnidentifiedAccess = resp.Unrestricted
		a.Save()
	}
	if ik := resp.IdentityPublicKey(); ik != nil {
		id := resp.UUID
		if id == "" {
			id = ad.ACI
		}
		saveProfileIdentity(a, id, ik.Bytes(), s.TrustNew, s.now())
	} else {
		s.logf("Got invalid identity key in profile for %s", s.identifier(key))
	}
	rc.Profile = p
	rc.Unregistered, rc.UnregisteredAt = false, 0
	a.Recipients[key] = rc
	a.SaveRecipients()
	return nil
}

// copyProfile copies a stored record without its phone-number sharing
// mode, as the reference does (§3.1 step 2, §5.4 step 4).
func copyProfile(p *store.Profile) *store.Profile {
	if p == nil {
		return &store.Profile{}
	}
	c := *p
	c.PhoneNumberSharing = nil
	return &c
}

// profileFailed stores the failure outcome of §5.5.
func (s *Sender) profileFailed(key string, err error, notFound bool) {
	s.logf("Failed to retrieve profile for %s, ignoring: %v", s.identifier(key), err)
	a := s.Acct
	rc := a.Recipients[key]
	p := copyProfile(rc.Profile)
	p.LastUpdate, p.AccessMode, p.Capabilities = s.now().UnixMilli(), store.AccessUnknown, nil
	rc.Profile = p
	a.Recipients[key] = rc
	if notFound {
		a.MarkUnregistered(key, s.now().UnixMilli())
	}
	a.SaveRecipients()
}

// avatarStep downloads or deletes the profile avatar when the path
// changed (§6.1); the stored path follows even if the download fails.
func (s *Sender) avatarStep(ctx context.Context, key string, rc *store.Recipient, pk [32]byte, avatar *string) {
	if rc.Profile != nil && strEq(rc.Profile.AvatarURLPath, avatar) {
		return
	}
	file := filepath.Join(s.AvatarsDir, "profile-"+AvatarID(s.Acct, key))
	switch {
	case s.AvatarsDir == "":
	case avatar == nil || *avatar == "":
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logf("Failed to delete profile avatar, ignoring: %v", err)
		}
	case s.CDN == nil:
	default:
		if err := s.downloadAvatar(ctx, file, pk, *avatar); err != nil {
			s.logf("Failed to download profile avatar, ignoring: %v", err)
		}
	}
	if rc.Profile == nil {
		rc.Profile = &store.Profile{}
	}
	rc.Profile.AvatarURLPath = avatar
}

func (s *Sender) downloadAvatar(ctx context.Context, file string, pk [32]byte, path string) error {
	data, err := s.CDN.Fetch(ctx, path, profileAvatarLimit)
	if err != nil {
		return err
	}
	plain, err := profile.DecryptAvatar(pk, data)
	if err != nil {
		return errors.New("authentication of decrypted data failed")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	return os.WriteFile(file, plain, 0o600)
}

func strEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// saveProfileIdentity stores an identity key from a profile (§5.4 step
// 6): a new key is trusted unless trust is "never"; a changed one only
// under "always".
func saveProfileIdentity(a *store.Account, id string, key []byte, trust string, now time.Time) {
	known, ok := a.Identities[id]
	switch {
	case ok && bytes.Equal(known.Key, key):
		return
	case !ok && trust != "never", ok && trust == "always":
		a.Identities[id] = store.Identity{Key: key, Trust: store.TrustedUnverified, Added: now.UnixMilli()}
	default:
		a.Identities[id] = store.Identity{Key: key, Trust: store.Untrusted, Added: now.UnixMilli()}
	}
	a.SaveIdentities()
}

// StoreProfileKey records a peer's profile key (§2.1): a new key, or the
// same key while the stored access mode is not ENABLED/UNRESTRICTED,
// clears the credential and invalidates the stored profile.
func StoreProfileKey(a *store.Account, key string, pk []byte) bool {
	rc := a.Recipients[key]
	if bytes.Equal(rc.ProfileKey, pk) && (rc.Profile == nil || rc.Profile.AccessMode == store.AccessEnabled || rc.Profile.AccessMode == store.AccessUnrestricted) {
		return false
	}
	rc.ProfileKey = pk
	rc.ProfileKeyCredential = nil
	if rc.Profile != nil {
		p := *rc.Profile
		p.LastUpdate = 0
		rc.Profile = &p
	}
	a.Recipients[key] = rc
	return true
}
