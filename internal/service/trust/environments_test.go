// SPDX-License-Identifier: GPL-3.0-or-later

package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// TestRootsMatchEnvironments: the embedded roots are the ones
// spec/service/environments.md §9 gives for both environments: the two
// Signal roots for libsignal-net hosts (chat, CDSI) and the RSA root for
// the OkHttp hosts (REST, storage, CDNs, SVR2), whose truststore's other
// entry expired in 2023 and is left out.
func TestRootsMatchEnvironments(t *testing.T) {
	type roots struct {
		SHA256 []string `json:"sha256_der"`
	}
	var raw struct {
		Staging, Production struct {
			TLS struct {
				LibsignalNet roots `json:"libsignal_net"`
				OkHTTP       roots `json:"okhttp_whisper_store"`
			} `json:"tls_roots"`
		}
	}
	testvec.Load(t, "environments.json", &raw)
	sum := func(der []byte) string { s := sha256.Sum256(der); return hex.EncodeToString(s[:]) }
	chat := []string{sum(rsaRoot), sum(ed25519Root)}
	for name, env := range map[string]struct {
		LibsignalNet, OkHTTP roots
	}{"staging": {raw.Staging.TLS.LibsignalNet, raw.Staging.TLS.OkHTTP}, "production": {raw.Production.TLS.LibsignalNet, raw.Production.TLS.OkHTTP}} {
		got, want := slices.Clone(chat), slices.Clone(env.LibsignalNet.SHA256)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: chat roots %v, spec %v", name, got, want)
		}
		if !slices.Contains(env.OkHTTP.SHA256, sum(rsaRoot)) {
			t.Errorf("%s: the REST root is not in the spec's OkHttp set %v", name, env.OkHTTP.SHA256)
		}
	}
}
