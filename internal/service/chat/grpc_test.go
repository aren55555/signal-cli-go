package chat

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/service/h2"
)

type fakeDoer struct {
	method, path string
	headers      []h2.HeaderField
	body         []byte
	resp         *h2.Response
}

func (f *fakeDoer) Do(ctx context.Context, method, path string, headers []h2.HeaderField, body []byte) (*h2.Response, error) {
	f.method, f.path, f.headers, f.body = method, path, headers, body
	return f.resp, nil
}

func TestGRPC(t *testing.T) {
	ok := &fakeDoer{resp: &h2.Response{Status: 200, Body: []byte{0, 0, 0, 0, 2, 8, 1},
		Trailer: []h2.HeaderField{{Name: "grpc-status", Value: "0"}}}}
	c := &Conn{rpc: ok, rpcHeaders: []h2.HeaderField{{Name: "authorization", Value: "Basic x"}}}
	got, err := c.GRPC(context.Background(), "/org.signal.chat.device.Devices/RemoveDevice", []byte{8, 2})
	if err != nil || !bytes.Equal(got, []byte{8, 1}) {
		t.Fatalf("%v %x", err, got)
	}
	if ok.method != "POST" || ok.path != "/org.signal.chat.device.Devices/RemoveDevice" || !bytes.Equal(ok.body, []byte{0, 0, 0, 0, 2, 8, 2}) {
		t.Errorf("request %s %s %x", ok.method, ok.path, ok.body)
	}
	want := []h2.HeaderField{{Name: "content-type", Value: "application/grpc"}, {Name: "te", Value: "trailers"}, {Name: "authorization", Value: "Basic x"}}
	for i, h := range want {
		if i >= len(ok.headers) || ok.headers[i].Name != h.Name || ok.headers[i].Value != h.Value {
			t.Fatalf("headers %v", ok.headers)
		}
	}

	failing := &fakeDoer{resp: &h2.Response{Status: 200, Header: []h2.HeaderField{{Name: "grpc-status", Value: "3"}, {Name: "grpc-message", Value: "bad%20id"}}}}
	c.rpc = failing
	var ge *GRPCError
	if _, err := c.GRPC(context.Background(), "/x", nil); !errors.As(err, &ge) || ge.Code != 3 || ge.Message != "bad id" {
		t.Errorf("trailers-only: %v", err)
	}
	c.rpc = &fakeDoer{resp: &h2.Response{Status: 200, Body: []byte{0, 0, 0, 0, 9, 1}, Trailer: []h2.HeaderField{{Name: "grpc-status", Value: "0"}}}}
	if _, err := c.GRPC(context.Background(), "/x", nil); err == nil {
		t.Error("truncated message accepted")
	}
	c.rpc = &fakeDoer{resp: &h2.Response{Status: 200}}
	if _, err := c.GRPC(context.Background(), "/x", nil); err == nil {
		t.Error("missing grpc-status accepted")
	}
	if _, err := (&Conn{}).GRPC(context.Background(), "/x", nil); !errors.Is(err, ErrNoGRPC) {
		t.Errorf("no h2: %v", err)
	}
}
