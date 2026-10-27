package cdsi

import (
	"github.com/aren55555/signal-cli-go/internal/enclave"
)

// StagingMREnclave and StagingURL are libsignal's staging CDSI enclave
// (cdsi.md §2).
const (
	StagingMREnclave = "6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a"
	StagingURL       = "wss://cdsi.staging.signal.org/v1/" + StagingMREnclave + "/discovery"
)

// Channel is the attested connection a lookup runs on.
type Channel interface {
	Send([]byte) error
	Recv() ([]byte, error)
}

// Lookup runs the exchange of §4.3 on an established channel. onToken
// receives the new token before the acknowledgement is sent.
func Lookup(ch Channel, req *Request, onToken func([]byte)) (*Response, error) {
	b, err := req.Encode()
	if err != nil {
		return nil, err
	}
	if err := ch.Send(b); err != nil {
		return nil, err
	}
	first, err := ch.Recv()
	if err != nil {
		if cerr := closeErr(err); cerr != nil {
			return nil, cerr
		}
		return nil, ErrProtocol // closed before the token
	}
	tr, err := ParseResponse(first)
	if err != nil {
		return nil, err
	}
	if len(tr.Token) == 0 {
		return nil, errNoToken
	}
	if onToken != nil {
		onToken(tr.Token)
	}
	ack, _ := (&Request{TokenAck: true}).Encode()
	if err := ch.Send(ack); err != nil {
		return nil, err
	}
	resp := &Response{}
	for {
		m, err := ch.Recv()
		if err != nil {
			if cerr := closeErr(err); cerr != nil {
				return nil, cerr
			}
			return resp, nil
		}
		if err := resp.merge(m); err != nil {
			return nil, err
		}
	}
}

type protoError string

func (e protoError) Error() string { return string(e) }

var errNoToken = protoError("CDS protocol: no token found in response")

// closeErr maps the end of the stream: nil for a normal close.
func closeErr(err error) error {
	code, reason, ok := enclave.CloseCode(err)
	if !ok {
		return err
	}
	return ClassifyClose(code, reason)
}
