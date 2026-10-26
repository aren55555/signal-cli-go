package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

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
	s.Profile(ctx, dest)
	rc := s.Acct.Recipients[dest]
	var key []byte
	switch {
	case rc.Profile.AccessMode == store.AccessUnrestricted:
		key = make([]byte, 16)
	case rc.Profile.AccessMode == store.AccessEnabled && len(rc.ProfileKey) == 32:
		key = accessKey(rc.ProfileKey)
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
	if !StoreProfileKey(a, m.Sender.ServiceID, key) {
		return nil
	}
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

// accessKey is the unidentified-access key of a profile key.
func accessKey(pk []byte) []byte { return api.UnidentifiedAccessKey(pk) }
