package chat

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/service/h2"
)

// gRPC unary calls on the chat connection's HTTP/2 connection
// (spec/service/sync.md §8.1).

// unaryDoer sends one HTTP/2 request; *h2.Conn implements it.
type unaryDoer interface {
	Do(ctx context.Context, method, path string, headers []h2.HeaderField, body []byte) (*h2.Response, error)
}

// GRPCError is a non-zero grpc-status.
type GRPCError struct {
	Code    int
	Message string
}

func (e *GRPCError) Error() string {
	return fmt.Sprintf("gRPC status %d: %s", e.Code, e.Message)
}

// ErrNoGRPC: the socket is not on an HTTP/2 connection.
var ErrNoGRPC = errors.New("chat: no HTTP/2 connection for gRPC")

// GRPC calls method (e.g. "/org.signal.chat.device.Devices/GetDevices")
// with a serialized request message and returns the response message.
// The request carries the socket's upgrade headers except
// x-signal-receive-stories.
func (c *Conn) GRPC(ctx context.Context, method string, req []byte) ([]byte, error) {
	if c.rpc == nil {
		return nil, ErrNoGRPC
	}
	body := make([]byte, 5, 5+len(req))
	binary.BigEndian.PutUint32(body[1:], uint32(len(req)))
	body = append(body, req...)
	hdr := append([]h2.HeaderField{{Name: "content-type", Value: "application/grpc"}, {Name: "te", Value: "trailers"}}, c.rpcHeaders...)
	r, err := c.rpc.Do(ctx, "POST", method, hdr, body)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, &h2.StatusError{Status: r.Status, Header: r.Header}
	}
	status, message, found := grpcStatus(r.Trailer)
	if !found {
		status, message, found = grpcStatus(r.Header) // trailers-only
	}
	if !found {
		return nil, errors.New("chat: gRPC response without grpc-status")
	}
	if status != 0 {
		return nil, &GRPCError{Code: status, Message: message}
	}
	if len(r.Body) < 5 || r.Body[0] != 0 {
		return nil, errors.New("chat: malformed gRPC message")
	}
	n := binary.BigEndian.Uint32(r.Body[1:5])
	if uint64(len(r.Body)-5) < uint64(n) {
		return nil, errors.New("chat: truncated gRPC message")
	}
	return r.Body[5 : 5+n], nil
}

func grpcStatus(fields []h2.HeaderField) (code int, message string, found bool) {
	for _, f := range fields {
		switch f.Name {
		case "grpc-status":
			n, err := strconv.Atoi(f.Value)
			if err != nil {
				return 0, "", false
			}
			code, found = n, true
		case "grpc-message":
			message, _ = url.PathUnescape(f.Value)
		}
	}
	return code, message, found
}
