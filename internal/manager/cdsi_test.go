// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/websocket"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/encoding/protowire"
)

// cdsiServer answers lookups from a fixed directory.
type cdsiServer struct {
	dir      map[uint64][2][16]byte // number -> (pni, aci)
	closeAt  int                    // close with code instead of replying on this Recv (0 = never)
	code     int
	recvs    int
	requests [][]byte
	pending  [][]byte
}

func (c *cdsiServer) Send(b []byte) error {
	c.requests = append(c.requests, b)
	if len(c.requests) == 1 {
		var tok []byte
		tok = protowire.AppendTag(tok, 3, protowire.BytesType)
		tok = protowire.AppendBytes(tok, append([]byte{1}, make([]byte, 64)...))
		c.pending = append(c.pending, tok)
		return nil
	}
	// The ack: answer every requested number of the first request.
	var triples []byte
	first := c.requests[0]
	for len(first) > 0 {
		n, typ, l := protowire.ConsumeTag(first)
		first = first[l:]
		vl := protowire.ConsumeFieldValue(n, typ, first)
		if n == 2 || n == 3 {
			v, _ := protowire.ConsumeBytes(first)
			for ; len(v) >= 8; v = v[8:] {
				num := binary.BigEndian.Uint64(v)
				rec := make([]byte, 40)
				if e, ok := c.dir[num]; ok {
					binary.BigEndian.PutUint64(rec, num)
					copy(rec[8:], e[0][:])
					copy(rec[24:], e[1][:])
				}
				triples = append(triples, rec...)
			}
		}
		first = first[vl:]
	}
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.BytesType)
	out = protowire.AppendBytes(out, triples)
	c.pending = append(c.pending, out)
	return nil
}

func (c *cdsiServer) Recv() ([]byte, error) {
	c.recvs++
	if c.closeAt == c.recvs {
		return nil, &websocket.CloseError{Code: c.code}
	}
	if len(c.pending) == 0 {
		return nil, &websocket.CloseError{Code: 1000}
	}
	m := c.pending[0]
	c.pending = c.pending[1:]
	return m, nil
}

type cdsiAuthChat struct{ f *fakeChat }

func (c cdsiAuthChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	if path == "/v2/directory/auth" {
		return &chat.Response{Status: 200, Body: []byte(`{"username":"u","password":"p"}`)}, nil
	}
	return c.f.Request(ctx, verb, path, headers, body)
}

func TestCDSILookups(t *testing.T) {
	a, _ := testAccountFor(t, testNumber, testACI, testPNI, 1, nil)
	pni := [16]byte{0xb1, 1}
	aci := [16]byte{0xa1, 2}
	srv := &cdsiServer{dir: map[uint64][2][16]byte{12025550199: {pni, aci}, 12025550188: {{0xb2, 3}, {}}}}
	s := &Sender{Acct: a, Chat: cdsiAuthChat{&fakeChat{t: t}}}
	s.DialCDSI = func(ctx context.Context, u, p string) (cdsi.Channel, func(), error) {
		srv.requests, srv.pending, srv.recvs = nil, nil, 0
		return srv, func() {}, nil
	}
	ctx := context.Background()

	// Partial: found with ACI; the token is not kept.
	k, ok := s.ResolveNumber(ctx, "+12025550199")
	if !ok || k != uuidString(aci[:]) {
		t.Fatalf("resolve %q %v", k, ok)
	}
	if ad := a.AddressOf(k); ad.Number != "+12025550199" || ad.PNI != uuidString(pni[:]) || a.Data.CDSIToken != nil {
		t.Errorf("address %+v token %x", ad, a.Data.CDSIToken)
	}
	// PNI only (ACI withheld) resolves to the PNI form.
	if k, ok := s.ResolveNumber(ctx, "+12025550188"); !ok || k != "PNI:"+uuidString([]byte{0xb2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("pni-only %q %v", k, ok)
	}
	// Not found: a number-only recipient becomes unregistered.
	a.ResolveLowTrust(storeAddrNumber("+12025550177"))
	if _, ok := s.ResolveNumber(ctx, "+12025550177"); ok {
		t.Error("absent number resolved")
	}
	if kk, _ := a.FindByNumber("+12025550177"); !a.Recipients[kk].Unregistered {
		t.Error("absent number not marked unregistered")
	}

	// Full: token and table persisted; the next full lookup sends prev+token.
	if _, err := s.LookupNumbers(ctx, []string{"+12025550199"}, true); err != nil {
		t.Fatal(err)
	}
	if len(a.Data.CDSIToken) != 65 || len(a.Data.CDSINumbers) != 1 || a.Data.LastRecipientsRefresh == 0 {
		t.Fatalf("persisted %x %v", a.Data.CDSIToken, a.Data.CDSINumbers)
	}
	s.LookupNumbers(ctx, []string{"+12025550188"}, true)
	r, _ := protowireFields(srv.requests[0])
	if len(r[2]) != 8 || len(r[3]) != 8 || len(r[6]) != 65 {
		t.Errorf("second full request fields %v", r)
	}
	// 4101 clears token and table.
	srv.closeAt, srv.code = 1, 4101
	_, err := s.LookupNumbers(ctx, []string{"+12025550199"}, true)
	var ce *cdsi.CloseError
	if !errors.As(err, &ce) || a.Data.CDSIToken != nil || len(a.Data.CDSINumbers) != 0 {
		t.Errorf("4101: %v token %x numbers %v", err, a.Data.CDSIToken, a.Data.CDSINumbers)
	}
}

func protowireFields(b []byte) (map[int][]byte, error) {
	out := map[int][]byte{}
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		b = b[l:]
		vl := protowire.ConsumeFieldValue(n, typ, b)
		if typ == protowire.BytesType {
			v, _ := protowire.ConsumeBytes(b)
			out[int(n)] = v
		}
		b = b[vl:]
	}
	return out, nil
}

func storeAddrNumber(n string) store.Address { return store.Address{Number: n} }
