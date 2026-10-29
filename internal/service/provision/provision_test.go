// SPDX-License-Identifier: GPL-3.0-or-later

package provision

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

type fakeSocket struct {
	toClient, fromClient chan []byte
	once                 sync.Once
	closed               chan struct{}
}

func newFake() *fakeSocket {
	return &fakeSocket{toClient: make(chan []byte, 8), fromClient: make(chan []byte, 8), closed: make(chan struct{})}
}

var errFakeClosed = errors.New("fake closed")

func (f *fakeSocket) Read() ([]byte, error) {
	select {
	case b := <-f.toClient:
		return b, nil
	case <-f.closed:
		return nil, errFakeClosed
	}
}
func (f *fakeSocket) Write(b []byte) error {
	select {
	case <-f.closed:
		return errFakeClosed
	default:
	}
	f.fromClient <- append([]byte(nil), b...)
	return nil
}
func (f *fakeSocket) Close() error { f.once.Do(func() { close(f.closed) }); return nil }

func (f *fakeSocket) recv(t *testing.T) *signalpb.ChatWebSocketMessage {
	t.Helper()
	select {
	case b := <-f.fromClient:
		m := &signalpb.ChatWebSocketMessage{}
		if err := proto.Unmarshal(b, m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("nothing from the client")
		return nil
	}
}

func (f *fakeSocket) request(t *testing.T, id uint64, verb, path string, body []byte) {
	t.Helper()
	b, _ := proto.Marshal(&signalpb.ChatWebSocketMessage{
		Type: signalpb.ChatWebSocketMessage_REQUEST.Enum(),
		Request: &signalpb.ChatWebSocketRequest{Id: proto.Uint64(id), Verb: proto.String(verb), Path: proto.String(path), Body: body,
			Headers: []string{"X-Signal-Timestamp: 1"}},
	})
	f.toClient <- b
}

func (f *fakeSocket) expectAck(t *testing.T, id uint64) {
	t.Helper()
	m := f.recv(t)
	r := m.GetResponse()
	if m.GetType() != signalpb.ChatWebSocketMessage_RESPONSE || r.GetId() != id || r.GetStatus() != 200 || r.GetMessage() != "OK" || r.Body != nil || r.Headers != nil {
		t.Fatalf("ack for %d: %v", id, m)
	}
}

const testAddress = "q83vEjRWeJCrze8SNFZ4kA=="

// TestLinkFlow: every request is acked before it is interpreted, the
// address gives the link URL, and the envelope comes back as sent
// (spec/service/linking.md §2, §5).
func TestLinkFlow(t *testing.T) {
	fs := newFake()
	s := newSession(fs, timing{keepAlive: time.Hour, address: time.Second, message: 2 * time.Second})
	defer s.Close()
	ctx := context.Background()

	fs.request(t, 5, "GET", "/v1/address", nil) // wrong verb: acked, ignored
	fs.expectAck(t, 5)
	fs.request(t, 6, "PUT", "/v1/elsewhere", []byte{1}) // unknown path: acked, ignored
	fs.expectAck(t, 6)
	addr, _ := proto.Marshal(&signalpb.ProvisioningAddress{Address: proto.String(testAddress)})
	fs.request(t, 7, "PUT", "/v1/address", addr)
	fs.expectAck(t, 7)
	got, err := s.Address(ctx)
	if err != nil || got != testAddress {
		t.Fatalf("address %q, %v", got, err)
	}
	env, _ := proto.Marshal(&signalpb.ProvisionEnvelope{PublicKey: []byte{5, 1}, Body: []byte{1, 2, 3}})
	fs.request(t, 8, "PUT", "/v1/message", env)
	fs.expectAck(t, 8)
	e, err := s.Envelope(ctx)
	if err != nil || string(e.GetBody()) != "\x01\x02\x03" || string(e.GetPublicKey()) != "\x05\x01" {
		t.Fatalf("envelope %v, %v", e, err)
	}
}

// TestTimeouts: no address within its deadline, or no message within the
// socket's lifespan, fails the link (§2.4).
func TestTimeouts(t *testing.T) {
	fs := newFake()
	s := newSession(fs, timing{keepAlive: time.Hour, address: 30 * time.Millisecond, message: time.Second})
	defer s.Close()
	if _, err := s.Address(context.Background()); !errors.Is(err, ErrNoAddress) {
		t.Errorf("address: %v", err)
	}

	fs = newFake()
	s = newSession(fs, timing{keepAlive: time.Hour, address: time.Second, message: 50 * time.Millisecond})
	defer s.Close()
	addr, _ := proto.Marshal(&signalpb.ProvisioningAddress{Address: proto.String(testAddress)})
	fs.request(t, 1, "PUT", "/v1/address", addr)
	fs.expectAck(t, 1)
	if _, err := s.Envelope(context.Background()); !errors.Is(err, ErrNoMessage) {
		t.Errorf("message: %v", err)
	}
}

// TestKeepAlive: GET /v1/keepalive with the wall-clock time in ms as id,
// no body, no headers; the matching response is swallowed (§2.4).
func TestKeepAlive(t *testing.T) {
	fs := newFake()
	before := uint64(time.Now().UnixMilli())
	s := newSession(fs, timing{keepAlive: 20 * time.Millisecond, address: time.Second, message: time.Second})
	defer s.Close()
	m := fs.recv(t)
	q := m.GetRequest()
	if q.GetVerb() != "GET" || q.GetPath() != "/v1/keepalive" || q.Body != nil || q.Headers != nil ||
		q.GetId() < before || q.GetId() > uint64(time.Now().UnixMilli()) {
		t.Fatalf("keep-alive %v", m)
	}
	b, _ := proto.Marshal(&signalpb.ChatWebSocketMessage{Type: signalpb.ChatWebSocketMessage_RESPONSE.Enum(),
		Response: &signalpb.ChatWebSocketResponse{Id: proto.Uint64(q.GetId()), Status: proto.Uint32(200)}})
	fs.toClient <- b
	fs.recv(t) // the next keep-alive; the session is still up
}

func TestLinkURL(t *testing.T) {
	k, _ := curve25519.GeneratePrivateKey(rand.Reader)
	pub := k.PublicKey()
	u := LinkURL(testAddress, pub)
	if !strings.HasPrefix(u, "sgnl://linkdevice?uuid=q83vEjRWeJCrze8SNFZ4kA%3D%3D&pub_key=") {
		t.Errorf("URL %s", u)
	}
	enc := strings.SplitN(u, "&pub_key=", 2)[1]
	if strings.ContainsAny(enc, "+/=") {
		t.Errorf("pub_key not form-encoded or padded: %s", enc)
	}
	addr, key, err := ParseLinkURL(u)
	if err != nil || addr != testAddress || string(key.Bytes()) != string(pub.Bytes()) {
		t.Errorf("parse: %q, %v", addr, err)
	}
	// The primary accepts extra parameters and padded keys, and checks
	// nothing about the scheme.
	padded := "x://y?pub_key=" + url.QueryEscape(base64Std(pub.Bytes())) + "&uuid=abc&capabilities=backup5"
	if addr, _, err := ParseLinkURL(padded); err != nil || addr != "abc" {
		t.Errorf("parse padded: %q, %v", addr, err)
	}
	for _, bad := range []string{"sgnl://linkdevice", "sgnl://linkdevice?uuid=a", "sgnl://linkdevice?uuid=a&pub_key=AAAA"} {
		if _, _, err := ParseLinkURL(bad); !errors.Is(err, ErrInvalidLink) {
			t.Errorf("%s: %v", bad, err)
		}
	}
}
