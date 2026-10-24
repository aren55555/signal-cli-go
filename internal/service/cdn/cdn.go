// Package cdn talks to Signal's attachment CDNs: resumable uploads to CDN 2
// and 3 and plain downloads (spec/service/attachments.md §4-5).
package cdn

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/agent"
	"github.com/aren55555/signal-cli-go/internal/service/trust"
)

// StagingBases are the configured base URLs per CDN number (§4.3).
var StagingBases = map[uint32]string{
	0: "https://cdn-staging.signal.org",
	2: "https://cdn2-staging.signal.org",
	3: "https://cdn3-staging.signal.org",
}

// Caps (§2.5).
const (
	MaxAttachment = 150 << 20
	MaxSticker    = 1 << 20
	MaxAvatar     = 10 << 20
)

// Form is the upload form from GET /v4/attachments/form/upload (§4.2).
type Form struct {
	CDN                  uint32            `json:"cdn"`
	Key                  string            `json:"key"`
	Headers              map[string]string `json:"headers"`
	SignedUploadLocation string            `json:"signedUploadLocation"`
}

// Config configures a Client.
type Config struct {
	Bases             map[uint32]string // default StagingBases
	Roots             *x509.CertPool    // default trust.RESTRoots()
	Proxy             *url.URL
	UserAgentOverride string
}

// Client makes CDN requests.
type Client struct {
	bases map[uint32]string
	agent string
	hc    *http.Client
}

// New returns a client with the REST trust roots, 30 s per request.
func New(cfg Config) *Client {
	bases := cfg.Bases
	if bases == nil {
		bases = StagingBases
	}
	roots := cfg.Roots
	if roots == nil {
		roots = trust.RESTRoots()
	}
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
		MaxIdleConns:      5,
		IdleConnTimeout:   45 * time.Second,
	}
	if cfg.Proxy != nil {
		tr.Proxy = http.ProxyURL(cfg.Proxy)
	}
	return &Client{bases: bases, agent: agent.String(cfg.UserAgentOverride), hc: &http.Client{Transport: tr, Timeout: 30 * time.Second}}
}

// StatusError is an unexpected HTTP status.
type StatusError struct {
	Method string
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("Response code: %d", e.Status)
}

// configured grafts the path, query and fragment of a server-supplied URL
// onto the configured base for cdn (§4.3).
func (c *Client) configured(cdn uint32, server string) (string, error) {
	base, ok := c.bases[cdn]
	if !ok {
		return "", errors.New("Unknown CDN version")
	}
	u, err := url.Parse(server)
	if err != nil || !u.IsAbs() {
		return "", errors.New("Malformed URL!")
	}
	s := base + u.EscapedPath()
	if u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		s += "#" + u.EscapedFragment()
	}
	return s, nil
}

// header is one request header; order is kept.
type header struct{ name, value string }

func (c *Client) do(ctx context.Context, method, target string, hs []header, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.agent)
	for _, h := range hs {
		if strings.EqualFold(h.name, "Content-Length") {
			continue // from the body
		}
		if strings.EqualFold(h.name, "Host") {
			req.Host = h.value
			continue
		}
		req.Header.Set(h.name, h.value)
	}
	if body == nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		req.Body = http.NoBody
		req.ContentLength = 0
	}
	return c.hc.Do(req)
}

func ok(code int) bool { return code >= 200 && code < 300 }

func drain(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
}

// formHeaders are the form's headers in a stable order.
func formHeaders(f Form, skipHost bool) []header {
	var hs []header
	for k, v := range f.Headers {
		if skipHost && strings.EqualFold(k, "host") {
			continue
		}
		hs = append(hs, header{k, v})
	}
	sortHeaders(hs)
	return hs
}

func sortHeaders(hs []header) {
	for i := 1; i < len(hs); i++ {
		for j := i; j > 0 && strings.ToLower(hs[j].name) < strings.ToLower(hs[j-1].name); j-- {
			hs[j], hs[j-1] = hs[j-1], hs[j]
		}
	}
}

// Create opens the resumable session and returns its Location (§4.4).
func (c *Client) Create(ctx context.Context, f Form) (string, error) {
	target, err := c.configured(f.CDN, f.SignedUploadLocation)
	if err != nil {
		return "", err
	}
	hs := formHeaders(f, true)
	switch f.CDN {
	case 2:
		hs = append(hs, header{"Content-Type", "application/octet-stream"})
	case 3:
		hs = append(hs, header{"Upload-Defer-Length", "1"}, header{"Tus-Resumable", "1.0.0"})
	default:
		return "", errors.New("Unknown CDN version")
	}
	hs = append(hs, header{"X-Signal-Agent", c.agent})
	resp, err := c.do(ctx, "POST", target, hs, nil)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if !ok(resp.StatusCode) {
		return "", &StatusError{"POST", resp.StatusCode}
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("Missing Location header in resumable-upload response")
	}
	return loc, nil
}

// Upload sends blob to a session from Create (§4.5, §4.6).
func (c *Client) Upload(ctx context.Context, f Form, location string, blob []byte) error {
	target, err := c.configured(f.CDN, location)
	if err != nil {
		return err
	}
	n := len(blob)
	switch f.CDN {
	case 2:
		resp, err := c.do(ctx, "PUT", target, []header{{"Content-Range", fmt.Sprintf("bytes */%d", n)}}, nil)
		if err != nil {
			return err
		}
		drain(resp)
		offset := 0
		switch {
		case ok(resp.StatusCode):
			return nil
		case resp.StatusCode == 308:
			if r := resp.Header.Get("Range"); r != "" {
				_, last, found := strings.Cut(strings.TrimPrefix(r, "bytes="), "-")
				v, err := strconv.Atoi(last)
				if !found || err != nil {
					return fmt.Errorf("bad Range header %q", r)
				}
				offset = v + 1
			}
		case resp.StatusCode == 404:
			return errors.New("resume location invalid")
		default:
			return &StatusError{"PUT", resp.StatusCode}
		}
		resp, err = c.do(ctx, "PUT", target, []header{
			{"Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, n-1, n)},
			{"Content-Type", "application/octet-stream"},
		}, blob[offset:])
		if err != nil {
			return err
		}
		drain(resp)
		if !ok(resp.StatusCode) {
			return &StatusError{"PUT", resp.StatusCode}
		}
		return nil
	case 3:
		hs := append([]header{{"Tus-Resumable", "1.0.0"}}, formHeaders(f, false)...)
		resp, err := c.do(ctx, "HEAD", target, hs, nil)
		if err != nil {
			return err
		}
		drain(resp)
		if !ok(resp.StatusCode) {
			return errors.New("resume location invalid")
		}
		offset, err := strconv.Atoi(resp.Header.Get("Upload-Offset"))
		if err != nil || offset < 0 || offset > n {
			return errors.New("resume location invalid")
		}
		if offset == n {
			return nil
		}
		hs = append([]header{
			{"Upload-Offset", strconv.Itoa(offset)},
			{"Upload-Length", strconv.Itoa(n)},
			{"Tus-Resumable", "1.0.0"},
		}, formHeaders(f, false)...)
		hs = append(hs, header{"Content-Type", "application/offset+octet-stream"})
		resp, err = c.do(ctx, "PATCH", target, hs, blob[offset:])
		if err != nil {
			return err
		}
		drain(resp)
		if !ok(resp.StatusCode) {
			return &StatusError{"PATCH", resp.StatusCode}
		}
		return nil
	}
	return errors.New("Unknown CDN version")
}

// Download fetches <cdn base>/attachments/<id> with a size cap (§5.1-5.2).
// id is URL-encoded as Java's URLEncoder does.
func (c *Client) Download(ctx context.Context, cdn uint32, id string, limit int64) ([]byte, error) {
	base, okb := c.bases[cdn]
	if !okb {
		return nil, errors.New("Attempted to download from unsupported CDN number")
	}
	return c.get(ctx, base+"/attachments/"+formEncode(id), limit)
}

// Fetch gets a CDN 0 path (stickers, avatars) with a size cap.
func (c *Client) Fetch(ctx context.Context, path string, limit int64) ([]byte, error) {
	return c.get(ctx, c.bases[0]+"/"+strings.TrimPrefix(path, "/"), limit)
}

func (c *Client) get(ctx context.Context, target string, limit int64) ([]byte, error) {
	resp, err := c.do(ctx, "GET", target, nil, nil)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	if !ok(resp.StatusCode) {
		return nil, &StatusError{"GET", resp.StatusCode}
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("Response exceeds max size!")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("Response exceeds max size!")
	}
	return data, nil
}

// formEncode is application/x-www-form-urlencoded as Java's URLEncoder:
// A–Z a–z 0–9 . - * _ unchanged, space as +, the rest %XX upper-case.
func formEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '-', c == '*', c == '_':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
