package manager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/username"
)

// Usernames (spec/service/usernames.md §3-§5, §7.2).

// UsernameStatusError is a non-2xx answer to a username request.
type UsernameStatusError struct{ Status int }

func (e *UsernameStatusError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Status, statusReason(e.Status))
}

func statusReason(code int) string {
	switch code {
	case 401:
		return "Unauthorized"
	case 409:
		return "Conflict"
	case 410:
		return "Gone"
	case 422:
		return "Unprocessable Entity"
	case 429:
		return "Too Many Requests"
	}
	return "Error"
}

// ErrUnexpectedUsername: the reserved hash is not one we sent (§3.3).
var ErrUnexpectedUsername = errors.New("Unexpected username response")

var b64url = base64.RawURLEncoding

func (s *Sender) usernameRequest(ctx context.Context, c Requester, verb, path string, body any, out any) error {
	var b []byte
	headers := jsonHeader
	if body != nil {
		var err error
		if b, err = api.Marshal(body); err != nil {
			return err
		}
	} else {
		headers = nil
	}
	r, err := c.Request(ctx, verb, path, headers, b)
	if err != nil {
		return err
	}
	if r.Status < 200 || r.Status > 299 {
		return &UsernameStatusError{Status: r.Status}
	}
	if out != nil && len(r.Body) > 0 {
		return json.Unmarshal(r.Body, out)
	}
	return nil
}

// SetUsername claims an exact username (with a '.') or a nickname
// (§3), stores it with its link and returns it.
func (s *Sender) SetUsername(ctx context.Context, value string) (string, error) {
	a := s.Acct
	var candidates []string
	if strings.Contains(value, ".") {
		if value == a.Data.Username {
			if err := s.RefreshUsername(ctx); err == nil {
				return a.Data.Username, nil
			}
		}
		if _, err := username.Parse(value); err != nil {
			return "", err
		}
		candidates = []string{value}
	} else {
		if cur := a.Data.Username; cur != "" && strings.SplitN(cur, ".", 2)[0] == value {
			if err := s.RefreshUsername(ctx); err == nil {
				return a.Data.Username, nil
			}
		}
		var err error
		if candidates, err = username.Candidates(value, 3, 32, s.rand()); err != nil {
			return "", err
		}
	}
	var hashes []string
	var parsed []username.Username
	for _, c := range candidates {
		u, err := username.Parse(c)
		if err != nil {
			return "", err
		}
		h, err := u.Hash()
		if err != nil {
			return "", err
		}
		parsed = append(parsed, u)
		hashes = append(hashes, b64url.EncodeToString(h))
	}
	var res struct {
		UsernameHash string `json:"usernameHash"`
	}
	if err := s.usernameRequest(ctx, s.Chat, "PUT", "/v1/accounts/username_hash/reserve", map[string]any{"usernameHashes": hashes}, &res); err != nil {
		return "", err
	}
	idx := -1
	for i, h := range hashes {
		if h == res.UsernameHash {
			idx = i
		}
	}
	if idx < 0 {
		return "", ErrUnexpectedUsername
	}
	var entropy [32]byte
	if _, err := io.ReadFull(s.rand(), entropy[:]); err != nil {
		return "", err
	}
	if err := s.confirmUsername(ctx, parsed[idx], candidates[idx], entropy); err != nil {
		return "", err
	}
	return a.Data.Username, nil
}

// confirmUsername sends the confirm with a fresh proof and a link blob
// under entropy, then stores the username and link.
func (s *Sender) confirmUsername(ctx context.Context, u username.Username, text string, entropy [32]byte) error {
	h, _ := u.Hash()
	proof, err := u.Proof(s.rand())
	if err != nil {
		return err
	}
	blob, err := username.NewLinkBlob(entropy, text)
	if err != nil {
		return err
	}
	var res struct {
		UsernameHash       string  `json:"usernameHash"`
		UsernameLinkHandle *string `json:"usernameLinkHandle"`
	}
	body := map[string]any{"usernameHash": b64url.EncodeToString(h), "zkProof": b64url.EncodeToString(proof), "encryptedUsername": b64url.EncodeToString(blob)}
	if err := s.usernameRequest(ctx, s.Chat, "PUT", "/v1/accounts/username_hash/confirm", body, &res); err != nil {
		return err
	}
	a := s.Acct
	a.Data.Username = text
	if res.UsernameLinkHandle != nil {
		a.Data.UsernameLinkEntropy, a.Data.UsernameLinkServerID = entropy[:], *res.UsernameLinkHandle
	}
	if err := a.Save(); err != nil {
		return err
	}
	k := a.ResolveHighTrust(store.Address{ACI: a.Data.ACI, Username: text})
	_ = k
	return a.SaveRecipients()
}

// RefreshUsername is §5: resynchronise the local username with the
// server; a status-code failure clears the local username.
func (s *Sender) RefreshUsername(ctx context.Context) error {
	a := s.Acct
	if a.Data.Username == "" {
		return nil
	}
	var who struct {
		UsernameHash *string `json:"usernameHash"`
	}
	if err := s.usernameRequest(ctx, s.Chat, "GET", "/v1/accounts/whoami", nil, &who); err != nil {
		return err
	}
	u, err := username.Parse(a.Data.Username)
	if err != nil {
		return err
	}
	h, _ := u.Hash()
	if who.UsernameHash != nil && *who.UsernameHash == b64url.EncodeToString(h) {
		return nil
	}
	if len(a.Data.UsernameLinkEntropy) == 32 && a.Data.UsernameLinkServerID != "" {
		err = s.confirmUsername(ctx, u, a.Data.Username, [32]byte(a.Data.UsernameLinkEntropy))
	} else {
		err = s.usernameRequest(ctx, s.Chat, "PUT", "/v1/accounts/username_hash/reserve", map[string]any{"usernameHashes": []string{b64url.EncodeToString(h)}}, nil)
		if err == nil {
			var entropy [32]byte
			io.ReadFull(s.rand(), entropy[:])
			err = s.confirmUsername(ctx, u, a.Data.Username, entropy)
		}
	}
	var se *UsernameStatusError
	if errors.As(err, &se) {
		a.Data.Username, a.Data.UsernameLinkEntropy, a.Data.UsernameLinkServerID = "", nil, ""
		a.Save()
	}
	return err
}

// DeleteUsername deletes the username hash (REST over the WebSocket, the
// documented alternative to the gRPC call, §4.3) and clears it locally.
func (s *Sender) DeleteUsername(ctx context.Context) error {
	if err := s.usernameRequest(ctx, s.Chat, "DELETE", "/v1/accounts/username_hash", nil, nil); err != nil {
		return err
	}
	a := s.Acct
	a.Data.Username, a.Data.UsernameLinkEntropy, a.Data.UsernameLinkServerID = "", nil, ""
	return a.Save()
}

// UsernameLinkURL is the stored link's URL, or "" without one.
func UsernameLinkURL(a *store.Account) string {
	if len(a.Data.UsernameLinkEntropy) != 32 || a.Data.UsernameLinkServerID == "" {
		return ""
	}
	return username.LinkURL([32]byte(a.Data.UsernameLinkEntropy), a.Data.UsernameLinkServerID)
}

// ResolveUsername resolves a username or signal.me link to a recipient
// row key (§7.2); force bypasses the local table. ok is false when the
// server knows no such username.
func (s *Sender) ResolveUsername(ctx context.Context, value string, force bool) (key string, ok bool, err error) {
	a := s.Acct
	var u username.Username
	var name string
	if ent, id, lerr := username.ParseLinkURL(value); lerr == nil {
		var res struct {
			Value string `json:"usernameLinkEncryptedValue"`
		}
		if err := s.usernameRequest(ctx, s.unauth(), "GET", "/v1/accounts/username_link/"+id, nil, &res); err != nil {
			return "", false, err
		}
		blob, derr := base64.RawURLEncoding.DecodeString(strings.TrimRight(res.Value, "="))
		if derr != nil {
			return "", false, derr
		}
		if name, err = username.DecryptLink(ent[:], blob); err != nil {
			return "", false, err
		}
		if u, err = username.Parse(name); err != nil {
			return "", false, username.ErrLinkDecryptedBad
		}
	} else {
		if u, err = username.Parse(value); err != nil {
			return "", false, err
		}
		name = value
	}
	if !force {
		if k, found := a.FindByUsername(name); found && a.AddressOf(k).ACI != "" {
			return k, true, nil
		}
	}
	h, _ := u.Hash()
	var res struct {
		UUID string `json:"uuid"`
	}
	err = s.usernameRequest(ctx, s.unauth(), "GET", "/v1/accounts/username_hash/"+b64url.EncodeToString(h), nil, &res)
	var se *UsernameStatusError
	if errors.As(err, &se) && se.Status == 404 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	k := a.ResolveHighTrust(store.Address{ACI: res.UUID, Username: name})
	a.SaveRecipients()
	return k, true, nil
}

func (s *Sender) unauth() Requester {
	if s.Unauth != nil {
		return s.Unauth
	}
	return s.Chat
}

// UpdateAccountAttributes uploads the account attributes (updateAccount
// step 1).
func (s *Sender) UpdateAccountAttributes(ctx context.Context) error {
	b, err := api.Marshal(accountAttributes(s.Acct))
	if err != nil {
		return err
	}
	r, err := s.Chat.Request(ctx, "PUT", "/v1/accounts/attributes", jsonHeader, b)
	if err != nil {
		return err
	}
	if r.Status < 200 || r.Status > 299 {
		return &SendStatusError{Status: r.Status}
	}
	return nil
}
