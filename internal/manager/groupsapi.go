package manager

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// The groups service (spec/service/groups.md §3).

// Raw is a REST client for the groups host.
type Raw interface {
	Raw(ctx context.Context, method, path string, headers map[string]string, body []byte) (*rest.RawResponse, error)
}

// GroupError is a failed groups request, with signal-cli's message and
// exception class (§3.5.1).
type GroupError struct {
	Status int
	Msg    string
	Class  string
	// Reason is X-Signal-Forbidden-Reason on a join-info 403.
	Reason string
}

func (e *GroupError) Error() string { return e.Msg }

// Endpoint kinds for the endpoint-specific error rules.
const (
	epState = iota
	epPatch
	epCreate
	epJoinInfo
	epAvatarForm
)

var serverParams = sync.OnceValue(func() *zkgroup.ServerPublicParams {
	p, err := zkgroup.ParseServerPublicParams(zkgroup.StagingServerPublicParams())
	if err != nil {
		panic("zkgroup: bad built-in server params")
	}
	return p
})

// ServerParams returns the staging zkgroup server public params.
func ServerParams() *zkgroup.ServerPublicParams { return serverParams() }

// GroupsService talks to the groups host for one account.
type GroupsService struct {
	Acct *store.Account
	Chat Requester // authenticated socket: auth credentials, profiles
	REST Raw       // https://storage-staging.signal.org
	CDN  CDN
	Rand io.Reader
	Now  func() time.Time

	creds map[uint64][]byte // redemption time -> auth credential response
}

func (s *GroupsService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *GroupsService) random32() [32]byte {
	var b [32]byte
	r := s.Rand
	if r == nil {
		r = rand.Reader
	}
	io.ReadFull(r, b[:])
	return b
}

func uuid16(s string) [16]byte {
	b, _ := uuidRaw(s)
	return b
}

// fetchAuthCredentials is §3.2.
func (s *GroupsService) fetchAuthCredentials(ctx context.Context, today uint64) error {
	path := fmt.Sprintf("/v1/certificate/auth/group?redemptionStartSeconds=%d&redemptionEndSeconds=%d", today, today+7*86400)
	r, err := s.Chat.Request(ctx, "GET", path, nil, nil)
	if err != nil {
		return err
	}
	if r.Status != 200 {
		return &GroupError{Status: r.Status, Msg: fmt.Sprintf("StatusCode: %d", r.Status), Class: "NonSuccessfulResponseCodeException"}
	}
	var body struct {
		Credentials []struct {
			Credential     string `json:"credential"`
			RedemptionTime uint64 `json:"redemptionTime"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return err
	}
	s.creds = map[uint64][]byte{}
	for _, c := range body.Credentials {
		b, err := base64.StdEncoding.DecodeString(c.Credential)
		if err != nil {
			return err
		}
		s.creds[c.RedemptionTime] = b
	}
	return nil
}

// authCredential returns today's received credential (§3.2: fetch when
// missing; one re-fetch on a verification failure).
func (s *GroupsService) authCredential(ctx context.Context) (*zkgroup.AuthCredentialWithPni, error) {
	today := uint64(s.now().UTC().Unix()) / 86400 * 86400
	for attempt := 0; attempt < 2; attempt++ {
		if _, ok := s.creds[today]; !ok || attempt == 1 {
			if err := s.fetchAuthCredentials(ctx, today); err != nil {
				return nil, err
			}
		}
		resp, ok := s.creds[today]
		if !ok {
			continue
		}
		cred, err := zkgroup.ReceiveAuthCredentialWithPni(ServerParams(), uuid16(s.Acct.Data.ACI), uuid16(s.Acct.Data.PNI), today, resp)
		if err == nil {
			return cred, nil
		}
		if attempt == 1 {
			return nil, fmt.Errorf("auth credential: %w", err)
		}
	}
	return nil, errors.New("no auth credential for today")
}

// authHeader is §3.3: Basic hex(public params):hex(presentation).
func (s *GroupsService) authHeader(ctx context.Context, g *zkgroup.GroupSecretParams) (string, error) {
	cred, err := s.authCredential(ctx)
	if err != nil {
		return "", err
	}
	pres := cred.Present(ServerParams(), g, s.random32())
	user := hex.EncodeToString(g.PublicParams()) + ":" + hex.EncodeToString(pres)
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user)), nil
}

// request sends one groups request and maps failures (§3.5.1).
func (s *GroupsService) request(ctx context.Context, g *zkgroup.GroupSecretParams, ep int, method, path string, extra map[string]string, body []byte) (*rest.RawResponse, error) {
	auth, err := s.authHeader(ctx, g)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Authorization": auth}
	if body != nil {
		headers["Content-Type"] = "application/x-protobuf"
	}
	for k, v := range extra {
		headers[k] = v
	}
	r, err := s.REST.Raw(ctx, method, path, headers, body)
	if err != nil {
		return nil, err
	}
	if r.Status >= 200 && r.Status < 300 && r.Status != 204 {
		return r, nil
	}
	return nil, groupError(ep, r)
}

func groupError(ep int, r *rest.RawResponse) error {
	if ep != epAvatarForm && r.Header.Get("X-Signal-Timestamp") == "" {
		return &GroupError{Status: 500, Msg: "[500] Missing timestamp header", Class: "NonSuccessfulResponseCodeException"}
	}
	code := func(class string) error {
		return &GroupError{Status: r.Status, Msg: fmt.Sprintf("StatusCode: %d", r.Status), Class: class}
	}
	switch {
	case ep == epState && r.Status == 403:
		return code("NotInGroupException")
	case ep == epState && r.Status == 404:
		return code("GroupNotFoundException")
	case ep == epPatch && r.Status == 400:
		var body struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(r.Body, &body) == nil && body.Message != "" {
			return &GroupError{Status: 400, Msg: "[400] " + body.Message, Class: "GroupPatchNotAcceptedException"}
		}
		return code("GroupPatchNotAcceptedException")
	case (ep == epPatch || ep == epJoinInfo) && r.Status == 423:
		return code("GroupTerminatedException")
	case ep == epCreate && r.Status == 409:
		return code("GroupExistsException")
	case ep == epJoinInfo && r.Status == 403:
		e := code("ForbiddenException").(*GroupError)
		e.Reason = r.Header.Get("X-Signal-Forbidden-Reason")
		return e
	}
	switch r.Status {
	case 204:
		return &GroupError{Status: 204, Msg: "StatusCode: 204", Class: "NoContentException"}
	case 401, 403:
		return &GroupError{Status: r.Status, Msg: fmt.Sprintf("[%d] Authorization failed!", r.Status), Class: "AuthorizationFailedException"}
	case 404:
		return &GroupError{Status: 404, Msg: "[404] Not found", Class: "NotFoundException"}
	case 409:
		return &GroupError{Status: 409, Msg: "[409] Conflict", Class: "ContactManifestMismatchException"}
	case 429:
		return &GroupError{Status: 429, Msg: "[429] Rate limit exceeded: 429", Class: "RateLimitException"}
	case 499:
		return &GroupError{Status: 499, Msg: "[499] Deprecated version", Class: "DeprecatedVersionException"}
	}
	return &GroupError{Status: r.Status, Msg: fmt.Sprintf("[%d] Response: %s", r.Status, http.StatusText(r.Status)), Class: "NonSuccessfulResponseCodeException"}
}

// IsConflict reports a 409.
func IsConflict(err error) bool {
	var ge *GroupError
	return errors.As(err, &ge) && ge.Status == 409
}

// IsNotInGroup reports a 403 on state or logs.
func IsNotInGroup(err error) bool {
	var ge *GroupError
	return errors.As(err, &ge) && ge.Class == "NotInGroupException"
}

// GetGroup is GET /v2/groups/ (§3.5.2).
func (s *GroupsService) GetGroup(ctx context.Context, g *zkgroup.GroupSecretParams) (*signalpb.GroupResponse, error) {
	r, err := s.request(ctx, g, epState, "GET", "/v2/groups/", nil, nil)
	if err != nil {
		return nil, err
	}
	var resp signalpb.GroupResponse
	if err := proto.Unmarshal(r.Body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Patch is PATCH /v2/groups/ (§3.5.3).
func (s *GroupsService) Patch(ctx context.Context, g *zkgroup.GroupSecretParams, a *signalpb.GroupChange_Actions, password []byte) (*signalpb.GroupChangeResponse, error) {
	body, err := proto.Marshal(a)
	if err != nil {
		return nil, err
	}
	path := "/v2/groups/"
	if password != nil {
		path += "?inviteLinkPassword=" + base64.RawURLEncoding.EncodeToString(password)
	}
	r, err := s.request(ctx, g, epPatch, "PATCH", path, nil, body)
	if err != nil {
		return nil, err
	}
	var resp signalpb.GroupChangeResponse
	if err := proto.Unmarshal(r.Body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Create is PUT /v2/groups/ (§3.5.8).
func (s *GroupsService) Create(ctx context.Context, g *zkgroup.GroupSecretParams, group *signalpb.Group) error {
	body, err := proto.Marshal(group)
	if err != nil {
		return err
	}
	_, err = s.request(ctx, g, epCreate, "PUT", "/v2/groups/", nil, body)
	return err
}

// Logs pages through GET /v2/groups/logs/<from> (§3.5.4).
func (s *GroupsService) Logs(ctx context.Context, g *zkgroup.GroupSecretParams, from uint32, cachedExpirationMs int64) ([]*signalpb.GroupChanges_GroupChangeState, error) {
	var out []*signalpb.GroupChanges_GroupChangeState
	for {
		path := fmt.Sprintf("/v2/groups/logs/%d?maxSupportedChangeEpoch=%d&includeFirstState=false&includeLastState=false", from, groups.HighestKnownEpoch)
		r, err := s.request(ctx, g, epState, "GET", path, map[string]string{"Cached-Send-Endorsements": strconv.FormatInt(cachedExpirationMs/1000, 10)}, nil)
		if err != nil {
			return out, err
		}
		var page signalpb.GroupChanges
		if err := proto.Unmarshal(r.Body, &page); err != nil {
			return out, err
		}
		out = append(out, page.GetGroupChanges()...)
		if r.Status != 206 {
			return out, nil
		}
		next, ok := groups.ParseContentRange(r.Header.Get("Content-Range"))
		if !ok {
			return out, errors.New("Unable to parse content range header on 206")
		}
		from = next
	}
}

// JoinInfo is GET /v2/groups/join/<pw> (§3.5.5).
func (s *GroupsService) JoinInfo(ctx context.Context, g *zkgroup.GroupSecretParams, password []byte) (*signalpb.GroupJoinInfo, error) {
	r, err := s.request(ctx, g, epJoinInfo, "GET", "/v2/groups/join/"+base64.RawURLEncoding.EncodeToString(password), nil, nil)
	if err != nil {
		return nil, err
	}
	var info signalpb.GroupJoinInfo
	if err := proto.Unmarshal(r.Body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// UploadAvatar uploads an avatar and returns its CDN key (§3.5.6).
func (s *GroupsService) UploadAvatar(ctx context.Context, g *zkgroup.GroupSecretParams, image []byte) (string, error) {
	r, err := s.request(ctx, g, epAvatarForm, "GET", "/v2/groups/avatar/form", nil, nil)
	if err != nil {
		return "", err
	}
	var form signalpb.AvatarUploadAttributes
	if err := proto.Unmarshal(r.Body, &form); err != nil {
		return "", err
	}
	plain, _ := proto.Marshal(&signalpb.GroupAttributeBlob{Content: &signalpb.GroupAttributeBlob_Avatar{Avatar: image}})
	blob := g.EncryptBlob(s.random32(), plain, 0)
	fields := []cdn.FormField{{Name: "acl", Value: form.GetAcl()}, {Name: "key", Value: form.GetKey()}, {Name: "policy", Value: form.GetPolicy()},
		{Name: "Content-Type", Value: "application/octet-stream"}, {Name: "x-amz-algorithm", Value: form.GetAlgorithm()},
		{Name: "x-amz-credential", Value: form.GetCredential()}, {Name: "x-amz-date", Value: form.GetDate()}, {Name: "x-amz-signature", Value: form.GetSignature()}}
	if err := s.CDN.PostCDN0(ctx, fields, blob); err != nil {
		return "", err
	}
	return form.GetKey(), nil
}

// DownloadAvatar fetches and decrypts a group avatar (§3.5.7).
func (s *GroupsService) DownloadAvatar(ctx context.Context, g *zkgroup.GroupSecretParams, key string) ([]byte, error) {
	b, err := s.CDN.Fetch(ctx, key, cdn.MaxAvatar)
	if err != nil {
		return nil, err
	}
	plain, err := g.DecryptBlob(b)
	if err != nil {
		return nil, err
	}
	var blob signalpb.GroupAttributeBlob
	if err := proto.Unmarshal(plain, &blob); err != nil {
		return nil, err
	}
	return blob.GetAvatar(), nil
}
