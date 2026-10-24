package manager

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// profileMaxAge is how long a stored profile is used (profile.md §3.1).
const profileMaxAge = 6 * time.Hour

// access is sealed-sender access for one recipient: the key for the
// Unidentified-Access-Key header and our sender certificate.
type access struct {
	key   []byte
	cert  []byte
	token []byte // a group send full token instead of the key (groups.md §12)
}

func (a *access) header() chat.Header {
	if a.token != nil {
		return chat.Header{Name: "group-send-token", Value: base64.StdEncoding.EncodeToString(a.token)}
	}
	return chat.Header{Name: "unidentified-access-key", Value: base64.StdEncoding.EncodeToString(a.key)}
}

// accessFor decides sealed or unsealed for a 1:1 send (send.md §3); nil
// means unsealed.
func (s *Sender) accessFor(ctx context.Context, dest string) *access {
	if s.Unauth == nil || dest == s.Acct.Data.ACI {
		return nil
	}
	rc := s.Acct.Recipients[dest]
	if rc.Profile == nil || s.now().Sub(time.UnixMilli(rc.Profile.LastUpdate)) > profileMaxAge {
		s.fetchProfile(ctx, dest)
		rc = s.Acct.Recipients[dest]
	}
	var key []byte
	switch {
	case rc.Profile.AccessMode == store.AccessUnrestricted:
		key = make([]byte, 16)
	case rc.Profile.AccessMode == store.AccessEnabled && len(rc.ProfileKey) == 32:
		key = api.UnidentifiedAccessKey(rc.ProfileKey)
	default:
		return nil
	}
	// The own key is only a gate and is always present (§3 item 2).
	cert := s.senderCertificate(ctx)
	if cert == nil {
		return nil
	}
	return &access{key: key, cert: cert}
}

// senderCertificate returns the E.164-less certificate, cached until a
// day before it expires (api.md §5.3); nil if it cannot be fetched.
func (s *Sender) senderCertificate(ctx context.Context) []byte {
	if s.cert != nil && time.UnixMilli(int64(s.certExpires)).Sub(s.now()) > 24*time.Hour {
		return s.cert
	}
	r, err := s.Chat.Request(ctx, "GET", "/v1/certificate/delivery?includeE164=false", nil, nil)
	if err != nil || r.Status != 200 {
		return nil
	}
	var resp struct {
		Certificate string `json:"certificate"`
	}
	if json.Unmarshal(r.Body, &resp) != nil {
		return nil
	}
	b, err := api.DecodeBase64(resp.Certificate)
	if err != nil {
		return nil
	}
	c, err := protocol.ParseSenderCertificate(b)
	if err != nil {
		return nil
	}
	s.cert, s.certExpires = b, c.Expires
	return b
}

// fetchProfile fetches dest's profile and stores the outcome; a failure
// is stored too, with mode unknown (profile.md §3.3).
// With a known profile key the versioned path is used (profile.md §3.2).
func (s *Sender) fetchProfile(ctx context.Context, dest string) {
	a := s.Acct
	rc := a.Recipients[dest]
	p := &store.Profile{LastUpdate: s.now().UnixMilli(), AccessMode: store.AccessUnknown}
	defer func() {
		rc.Profile = p
		a.Recipients[dest] = rc
		a.SaveRecipients()
	}()
	path := "/v1/profile/" + dest
	if len(rc.ProfileKey) == 32 {
		if aci, ok := uuidRaw(dest); ok {
			var k [32]byte
			copy(k[:], rc.ProfileKey)
			path += "/" + zkgroup.ProfileKeyVersion(k, aci)
		}
	}
	r, err := s.Chat.Request(ctx, "GET", path, []chat.Header{{Name: "accept-language", Value: "en;q=1"}}, nil)
	if err != nil {
		return
	}
	if r.Status == 404 {
		rc.Unregistered = true
		return
	}
	if r.Status != 200 {
		return
	}
	var resp struct {
		IdentityKey                    string          `json:"identityKey"`
		UnidentifiedAccess             string          `json:"unidentifiedAccess"`
		UnrestrictedUnidentifiedAccess bool            `json:"unrestrictedUnidentifiedAccess"`
		Capabilities                   map[string]bool `json:"capabilities"`
	}
	if json.Unmarshal(r.Body, &resp) != nil {
		return
	}
	rc.Unregistered = false
	if ik, err := api.DecodeBase64(resp.IdentityKey); err == nil {
		if _, err := curve25519.ParsePublicKey(ik); err == nil {
			saveIdentity(a, dest, ik, s.now())
		}
	}
	for name, on := range resp.Capabilities {
		if on {
			p.Capabilities = append(p.Capabilities, name)
		}
	}
	p.AccessMode = store.AccessDisabled
	switch {
	case resp.UnrestrictedUnidentifiedAccess:
		p.AccessMode = store.AccessUnrestricted
	case len(rc.ProfileKey) == 32:
		ver, err := api.DecodeBase64(resp.UnidentifiedAccess)
		m := hmac.New(sha256.New, api.UnidentifiedAccessKey(rc.ProfileKey))
		m.Write(make([]byte, 32))
		if err == nil && hmac.Equal(ver, m.Sum(nil)) {
			p.AccessMode = store.AccessEnabled
		}
	}
}

// saveIdentity records a peer identity key learned outside a message: a
// first key is trusted unverified, a changed one replaces the old one as
// untrusted.
func saveIdentity(a *store.Account, serviceID string, key []byte, now time.Time) {
	known, ok := a.Identities[serviceID]
	switch {
	case !ok:
		a.Identities[serviceID] = store.Identity{Key: key, Trust: store.TrustedUnverified, Added: now.UnixMilli()}
	case !bytes.Equal(known.Key, key):
		a.Identities[serviceID] = store.Identity{Key: key, Trust: store.Untrusted, Added: now.UnixMilli()}
	default:
		return
	}
	a.SaveIdentities()
}

// downgrade is the sticky downgrade after a success that was not
// unidentified (send.md §3.4).
func (s *Sender) downgrade(dest string) {
	rc, ok := s.Acct.Recipients[dest]
	if !ok || rc.Profile == nil {
		return
	}
	if m := rc.Profile.AccessMode; m == store.AccessEnabled || m == store.AccessUnrestricted {
		p := *rc.Profile
		p.AccessMode = store.AccessUnknown
		rc.Profile = &p
		s.Acct.Recipients[dest] = rc
		s.Acct.SaveRecipients()
	}
}

// ApplyContent applies a decrypted message's side effects on the store:
// for now, a sender's 32-byte profile key (content.md §4.4).
func ApplyContent(a *store.Account, m *Message, now time.Time) error {
	var c signalpb.AppContent
	if err := proto.Unmarshal(m.Content, &c); err != nil {
		return nil // validation drops it elsewhere
	}
	key := c.GetDataMessage().GetProfileKey()
	if sk := c.GetStoryMessage().GetProfileKey(); key == nil && sk != nil {
		key = sk // a story carries its sender's profile key (stories.md §5.2)
	}
	if len(key) != 32 {
		return nil
	}
	if m.Sender.ServiceID == a.Data.ACI {
		if bytes.Equal(a.Data.ProfileKey, key) {
			return nil
		}
		a.Data.ProfileKey = key
		return a.Save()
	}
	rc := a.Recipients[m.Sender.ServiceID]
	if bytes.Equal(rc.ProfileKey, key) {
		return nil
	}
	rc.ProfileKey = key
	// The cached profile was judged with the old key (or none); fetch it
	// again when next needed. The spec is silent here; without this a
	// stale "disabled" access mode would last the full 6 hours.
	rc.Profile = nil
	a.Recipients[m.Sender.ServiceID] = rc
	return a.SaveRecipients()
}

// uuidRaw parses a hyphenated UUID into its 16 bytes.
func uuidRaw(s string) ([16]byte, bool) {
	var u [16]byte
	if len(s) != 36 {
		return u, false
	}
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(b) != 16 {
		return u, false
	}
	copy(u[:], b)
	return u, true
}

// RefreshProfile fetches dest's profile now and returns what was stored.
func (s *Sender) RefreshProfile(ctx context.Context, dest string) *store.Profile {
	s.fetchProfile(ctx, dest)
	return s.Acct.Recipients[dest].Profile
}
