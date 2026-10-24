// Package rest is the HTTPS side of the chat service (spec/service/api.md
// §1-4, transport.md §5): registration, device linking and the send
// fallback.
package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/agent"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/trust"
)

// StagingURL is the REST base URL for staging.
const StagingURL = "https://chat.staging.signal.org"

// Errors (api.md §4.1).
var ErrAuthorization = errors.New("rest: authorization failed")

// RateLimitError is a 413 or 429; After is the integer-seconds
// Retry-After, or 0.
type RateLimitError struct {
	Status int
	After  time.Duration
	Body   []byte
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rest: rate limited (%d), retry after %v", e.Status, e.After)
}

// StatusError is any other unsuccessful status, with the response body
// for call-specific handling.
type StatusError struct {
	Status int
	Header http.Header
	Body   []byte
}

func (e *StatusError) Error() string { return fmt.Sprintf("rest: status %d", e.Status) }

// Basic is an HTTP Basic credential.
type Basic struct{ User, Password string }

// Config configures a Client.
type Config struct {
	BaseURL           string         // default StagingURL
	Roots             *x509.CertPool // default trust.RESTRoots()
	Proxy             *url.URL
	UserAgentOverride string
	Timeout           time.Duration // default 30 s
}

// Client makes REST calls.
type Client struct {
	base  string
	agent string
	hc    *http.Client
}

// New returns a client: TLS 1.2 or 1.3 with the REST roots, offering h2
// and http/1.1.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = StagingURL
	}
	roots := cfg.Roots
	if roots == nil {
		roots = trust.RESTRoots()
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
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
	return &Client{base: strings.TrimRight(base, "/"), agent: agent.String(cfg.UserAgentOverride), hc: &http.Client{Transport: tr, Timeout: timeout}}
}

// Do sends a request with an optional JSON body and decodes a JSON
// response into out (if non-nil and the body is non-empty).
func (c *Client) Do(ctx context.Context, method, path string, auth *Basic, body, out any) error {
	return c.DoHeaders(ctx, method, path, auth, nil, body, out)
}

// DoHeaders is Do with extra request headers.
func (c *Client) DoHeaders(ctx context.Context, method, path string, auth *Basic, headers map[string]string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := api.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("X-Signal-Agent", c.agent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if auth != nil {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth.User+":"+auth.Password)))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case 200, 202, 204, 207:
		if out != nil && len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("rest: decoding %s %s response: %w", method, path, err)
			}
		}
		return nil
	case 413, 429:
		e := &RateLimitError{Status: resp.StatusCode, Body: data}
		if n, err := strconv.ParseUint(strings.TrimSpace(resp.Header.Get("Retry-After")), 10, 32); err == nil {
			e.After = time.Duration(n) * time.Second
		}
		return e
	}
	return &StatusError{Status: resp.StatusCode, Header: resp.Header, Body: data}
}

// Is lets a 401 or 403 match ErrAuthorization (api.md §4.1).
func (e *StatusError) Is(target error) bool {
	return target == ErrAuthorization && (e.Status == 401 || e.Status == 403)
}
