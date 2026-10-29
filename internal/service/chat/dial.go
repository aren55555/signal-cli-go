// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/agent"
	"github.com/aren55555/signal-cli-go/internal/service/h2"
	"github.com/aren55555/signal-cli-go/internal/service/trust"
	"github.com/aren55555/signal-cli-go/internal/service/websocket"
)

// StagingHost is the libsignal-net chat host for staging
// (spec/service/transport.md §3).
const StagingHost = "grpc.chat.staging.signal.org"

// Upgrade failures (pipe.md §2, transport.md §3.4).
var (
	// ErrDeregistered is a 403 on the authenticated upgrade: the device
	// is no longer registered.
	ErrDeregistered = errors.New("chat: device deregistered (403)")
	// ErrExpired is a 499: this client version is no longer accepted.
	ErrExpired = errors.New("chat: client version expired (499)")
)

// RetryLaterError is an upgrade answered with Retry-After, which takes
// precedence over the status.
type RetryLaterError struct {
	Status int
	After  time.Duration
}

func (e *RetryLaterError) Error() string {
	return fmt.Sprintf("chat: server asked to retry after %v (status %d)", e.After, e.Status)
}

// Credentials authenticate a device on the chat socket.
type Credentials struct {
	ACI      string // lower-case hyphenated UUID, no prefix
	DeviceID int
	Password string
}

// Endpoint describes a chat socket to open.
type Endpoint struct {
	Host              string         // default StagingHost
	Credentials       *Credentials   // nil for the unauthenticated socket
	ReceiveStories    bool           // x-signal-receive-stories, authenticated socket only
	UserAgentOverride string         // SIGNAL_CLI_USER_AGENT
	Proxy             *url.URL       // HTTP CONNECT proxy
	Roots             *x509.CertPool // default trust.ChatRoots()
	KeepAlive         Config
}

// dialSocket opens the WebSocket; tests replace it.
var dialSocket = func(ctx context.Context, rawURL string, opts websocket.Options) (Socket, http.Header, error) {
	c, err := websocket.DialH2(ctx, rawURL, opts)
	if err != nil {
		return nil, nil, err
	}
	return c, c.ResponseHeader(), nil
}

// Dial opens a chat socket (transport.md §3.4) and starts the protocol.
func Dial(ctx context.Context, ep Endpoint) (*Conn, error) {
	host := ep.Host
	if host == "" {
		host = StagingHost
	}
	roots := ep.Roots
	if roots == nil {
		roots = trust.ChatRoots()
	}
	h := http.Header{}
	if cr := ep.Credentials; cr != nil {
		user := cr.ACI + "." + strconv.Itoa(cr.DeviceID)
		h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+cr.Password)))
		h.Set("X-Signal-Receive-Stories", strconv.FormatBool(ep.ReceiveStories))
	}
	h.Set("User-Agent", agent.Chat(ep.UserAgentOverride))
	s, resp, err := dialSocket(ctx, "wss://"+host+"/v1/websocket/", websocket.Options{
		Header:    h,
		Proxy:     ep.Proxy,
		TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13},
	})
	if err != nil {
		return nil, classify(err)
	}
	c := New(s, ep.KeepAlive)
	if hs, ok := s.(interface{ H2() *h2.Conn }); ok && hs.H2() != nil {
		rh := h.Clone()
		rh.Del("X-Signal-Receive-Stories")
		c.rpc, c.rpcHeaders = hs.H2(), websocket.UpgradeHeaders(rh)
	}
	if ms, err := strconv.ParseInt(strings.TrimSpace(resp.Get("X-Signal-Timestamp")), 10, 64); err == nil {
		c.serverTime = time.UnixMilli(ms)
	}
	return c, nil
}

// classify maps an upgrade failure to the errors above.
func classify(err error) error {
	var se *h2.StatusError
	if !errors.As(err, &se) {
		return err
	}
	for _, f := range se.Header {
		if f.Name != "retry-after" {
			continue
		}
		// Integer seconds only; the HTTP-date form is not supported.
		if n, perr := strconv.ParseUint(strings.TrimSpace(f.Value), 10, 32); perr == nil {
			return &RetryLaterError{Status: se.Status, After: time.Duration(n) * time.Second}
		}
	}
	switch se.Status {
	case 403:
		return fmt.Errorf("%w: %w", ErrDeregistered, err)
	case 499:
		return fmt.Errorf("%w: %w", ErrExpired, err)
	}
	return err
}

// ServerTime returns the server's clock at connection time
// (x-signal-timestamp), or the zero time if it was not sent.
func (c *Conn) ServerTime() time.Time { return c.serverTime }
