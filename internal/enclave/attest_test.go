// SPDX-License-Identifier: GPL-3.0-or-later

package enclave

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/enclave_attestation.json (attestation.md §10).
type attestCase struct {
	Name           string
	MREnclave      string
	TimestampUnix  int64                   `json:"timestamp_unix"`
	HandshakeStart struct{ Base64 string } `json:"handshake_start"`
	Observed       struct {
		Range []int64 `json:"accepted_given_time_range_unix"`
	} `json:"observed_with_reference_jar"`
	RaftHex string `json:"expected_raft_group_config_protobuf_hex"`
}

func loadCases(t *testing.T) []attestCase {
	var v struct{ Cases []attestCase }
	testvec.Load(t, "enclave_attestation.json", &v)
	return v.Cases[:2] // the two staging captures
}

func TestAttestationStagingCaptures(t *testing.T) {
	for _, c := range loadCases(t) {
		raw, _ := base64.StdEncoding.DecodeString(c.HandshakeStart.Base64)
		ev, en, err := ParseHandshakeStart(raw)
		if err != nil {
			t.Fatal(err)
		}
		mr, _ := hex.DecodeString(c.MREnclave)
		claims, err := Verify(ev, en, mr, time.Unix(c.TimestampUnix, 0))
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if len(claims["pk"]) != 32 {
			t.Errorf("%s: pk %x", c.Name, claims["pk"])
		}
		if c.RaftHex != "" && hex.EncodeToString(claims["config"]) != c.RaftHex {
			t.Errorf("%s: config %x", c.Name, claims["config"])
		}
		// The jar's acceptance window, inclusive at both ends.
		lo, hi := c.Observed.Range[0], c.Observed.Range[1]
		for _, at := range []struct {
			ts int64
			ok bool
		}{{lo, true}, {hi, true}, {lo - 1, false}, {hi + 1, false}} {
			_, err := Verify(ev, en, mr, time.Unix(at.ts, 0))
			if (err == nil) != at.ok {
				t.Errorf("%s at %d: %v (want ok=%v)", c.Name, at.ts, err, at.ok)
			}
		}
		if _, err := Verify(ev, en, mr, time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "(endorsements) attestation is not valid") {
			t.Errorf("%s today: %v", c.Name, err)
		}
		bad := append([]byte{}, mr...)
		bad[0] ^= 1
		if _, err := Verify(ev, en, bad, time.Unix(c.TimestampUnix, 0)); err == nil || !strings.Contains(err.Error(), "unaccepted advisory ids") {
			t.Errorf("%s wrong mrenclave: %v", c.Name, err)
		}
		var de *DataError
		if _, err := Verify(nil, en, mr, time.Unix(c.TimestampUnix, 0)); !errors.As(err, &de) {
			t.Errorf("%s empty evidence: %v", c.Name, err)
		}
		if _, err := Verify(ev, en, mr[:31], time.Unix(c.TimestampUnix, 0)); !errors.As(err, &de) {
			t.Errorf("%s 31-byte mrenclave: %v", c.Name, err)
		}
		// Any flipped byte in the signed quote fails.
		tampered := append([]byte{}, ev...)
		tampered[200] ^= 1
		if _, err := Verify(tampered, en, mr, time.Unix(c.TimestampUnix, 0)); err == nil {
			t.Errorf("%s: tampered quote accepted", c.Name)
		}
	}
}
