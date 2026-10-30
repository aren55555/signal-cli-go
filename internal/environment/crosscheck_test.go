// SPDX-License-Identifier: GPL-3.0-or-later

package environment_test

import (
	"bytes"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/enclave"
	"github.com/aren55555/signal-cli-go/internal/environment"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/svr2"
)

// TestOtherCopiesAgree: values that other packages keep for their own
// purposes match the environment definitions.
func TestOtherCopiesAgree(t *testing.T) {
	for e, pe := range map[*environment.Environment]protocol.Environment{environment.Staging: protocol.Staging, environment.Production: protocol.Production} {
		want := protocol.TrustRoots(pe)
		got := e.TrustRoots()
		if len(got) != len(want) {
			t.Fatalf("%s: %d trust roots, protocol has %d", e.Name, len(got), len(want))
		}
		for i := range got {
			if !bytes.Equal(got[i], want[i].Bytes()) {
				t.Errorf("%s: trust root %d differs from protocol\x27s", e.Name, i)
			}
		}
		// Attestation must know every enclave of the environment.
		if !enclave.KnownEnclaves[e.CDSIMREnclave] {
			t.Errorf("%s: CDSI enclave unknown to attestation", e.Name)
		}
		for _, s := range svr2.Enclaves(e) {
			if !enclave.KnownEnclaves[s.MREnclave] {
				t.Errorf("%s: SVR2 enclave %s unknown to attestation", e.Name, s.MREnclave)
			}
			if s.SuperMajority != 0 && len(s.RaftConfig()) == 0 {
				t.Errorf("%s: empty Raft config", e.Name)
			}
		}
	}
}
