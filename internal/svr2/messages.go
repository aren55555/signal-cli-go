// SPDX-License-Identifier: GPL-3.0-or-later

package svr2

import (
	"errors"

	"google.golang.org/protobuf/encoding/protowire"
)

// Enclave is one SVR2 enclave with its Raft group (svr.md §3.1).
type Enclave struct {
	MREnclave string
	GroupID   uint64
	// Config is the expected serialized RaftGroupConfig claim.
	Min, Max, SuperMajority uint32
}

// StagingEnclaves in signal-cli's order: current, then legacy.
var StagingEnclaves = []Enclave{
	{MREnclave: "3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302", GroupID: 5138641357881452604, Min: 3, Max: 9},
	{MREnclave: "97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535", GroupID: 2330628069874851020, Min: 3, Max: 9},
}

// URL is the enclave's WebSocket URL on staging.
func (e Enclave) URL() string { return "wss://svr2.staging.signal.org/v1/" + e.MREnclave }

// RaftConfig is the expected `config` claim (attestation.md §7.2): all
// seven fields, db_version 2, timeout 604800, not simulated.
func (e Enclave) RaftConfig() []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.Fixed64Type)
	b = protowire.AppendFixed64(b, e.GroupID)
	b = protowire.AppendTag(b, 2, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(e.Min))
	b = protowire.AppendTag(b, 3, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(e.Max))
	if e.SuperMajority != 0 {
		b = protowire.AppendTag(b, 4, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(e.SuperMajority))
	}
	b = protowire.AppendTag(b, 5, protowire.VarintType)
	b = protowire.AppendVarint(b, 2)
	b = protowire.AppendTag(b, 6, protowire.VarintType)
	return protowire.AppendVarint(b, 604800)
}

func sub(n protowire.Number, body []byte) []byte {
	var b []byte
	b = protowire.AppendTag(b, n, protowire.BytesType)
	return protowire.AppendBytes(b, body)
}

func bytesField(n protowire.Number, v []byte) []byte {
	var b []byte
	b = protowire.AppendTag(b, n, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}

// BackupRequest is Request{backup{data, pin, max_tries}}.
func BackupRequest(data, pin []byte, maxTries uint32) []byte {
	body := append(bytesField(1, data), bytesField(2, pin)...)
	body = protowire.AppendTag(body, 3, protowire.VarintType)
	body = protowire.AppendVarint(body, uint64(maxTries))
	return sub(2, body)
}

// ExposeRequest is Request{expose{data}}.
func ExposeRequest(data []byte) []byte { return sub(5, bytesField(1, data)) }

// RestoreRequest is Request{restore{pin}}.
func RestoreRequest(pin []byte) []byte { return sub(3, bytesField(1, pin)) }

// DeleteRequest is Request{delete{}}.
func DeleteRequest() []byte { return sub(4, nil) }

// Response is a decoded SVR2 response; Kind is which part was present.
type Response struct {
	Kind   int // 1 backup, 2 restore, 3 delete, 4 expose, 0 none
	Status int
	Data   []byte
	Tries  uint32
}

// Status values (svr.md §3.3).
const (
	StatusOK           = 1
	RestoreMissing     = 2
	RestorePinMismatch = 3
	ExposeError        = 2
)

// ParseResponse decodes an SVR2 Response.
func ParseResponse(b []byte) (*Response, error) {
	r := &Response{}
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return nil, errors.New("svr2: bad response")
		}
		b = b[l:]
		vl := protowire.ConsumeFieldValue(n, typ, b)
		if vl < 0 {
			return nil, errors.New("svr2: bad response")
		}
		if typ == protowire.BytesType && n >= 1 && n <= 4 {
			r.Kind = int(n)
			inner, _ := protowire.ConsumeBytes(b)
			for len(inner) > 0 {
				fn, ft, fl := protowire.ConsumeTag(inner)
				if fl < 0 {
					return nil, errors.New("svr2: bad response")
				}
				inner = inner[fl:]
				fvl := protowire.ConsumeFieldValue(fn, ft, inner)
				if fvl < 0 {
					return nil, errors.New("svr2: bad response")
				}
				switch {
				case fn == 1 && ft == protowire.VarintType:
					v, _ := protowire.ConsumeVarint(inner)
					r.Status = int(v)
				case fn == 2 && ft == protowire.BytesType && r.Kind == 2:
					v, _ := protowire.ConsumeBytes(inner)
					r.Data = append([]byte{}, v...)
				case fn == 3 && ft == protowire.VarintType && r.Kind == 2:
					v, _ := protowire.ConsumeVarint(inner)
					r.Tries = uint32(v)
				}
				inner = inner[fvl:]
			}
		}
		b = b[vl:]
	}
	return r, nil
}
