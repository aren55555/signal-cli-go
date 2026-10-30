// SPDX-License-Identifier: GPL-3.0-or-later

package environment

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strconv"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// vector is one environment of testdata/vectors/environments.json, the
// machine-readable copy of spec/service/environments.md.
type vector struct {
	AccountEnvironment string            `json:"account_environment"`
	CLIValue           string            `json:"cli_value"`
	ChatRESTURL        string            `json:"chat_rest_url"`
	ChatWSHost         string            `json:"chat_ws_host"`
	ProvisioningURL    string            `json:"provisioning_url"`
	StorageURL         string            `json:"storage_url"`
	GroupsURL          string            `json:"groups_url"`
	CDNURLs            map[string]string `json:"cdn_urls"`
	CDSIURL            string            `json:"cdsi_url"`
	CDSIMREnclave      string            `json:"cdsi_mrenclave"`
	SVR2URL            string            `json:"svr2_url"`
	SVR2Enclaves       []struct {
		MREnclave     string `json:"mrenclave"`
		URL           string `json:"url"`
		Min           uint32 `json:"min_voting_replicas"`
		Max           uint32 `json:"max_voting_replicas"`
		SuperMajority uint32 `json:"super_majority"`
		GroupID       string `json:"group_id"`
	} `json:"svr2_enclaves"`
	ZkgroupParams string   `json:"zkgroup_server_public_params"`
	TrustRoots    []string `json:"sealed_sender_trust_roots"`
}

func loadVectors(t *testing.T) map[string]vector {
	var v map[string]vector
	var raw struct {
		Staging    vector `json:"staging"`
		Production vector `json:"production"`
	}
	testvec.Load(t, "environments.json", &raw)
	v = map[string]vector{"staging": raw.Staging, "production": raw.Production}
	return v
}

func host(t *testing.T, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

// TestValuesMatchSpec compares every built-in value with the spec's
// vector file.
func TestValuesMatchSpec(t *testing.T) {
	vs := loadVectors(t)
	for _, e := range All {
		v, ok := vs[e.Name]
		if !ok {
			t.Fatalf("%s: not in environments.json", e.Name)
		}
		check := func(what, got, want string) {
			if got != want {
				t.Errorf("%s %s: %q, spec %q", e.Name, what, got, want)
			}
		}
		check("account environment", e.Account, v.AccountEnvironment)
		check("CLI value", e.CLI, v.CLIValue)
		check("chat REST URL", e.ChatRESTURL, v.ChatRESTURL)
		check("chat host", e.ChatHost, v.ChatWSHost)
		check("provisioning host", e.ProvisioningHost, host(t, v.ProvisioningURL))
		check("storage URL", e.StorageURL, v.StorageURL)
		check("groups URL (same service)", e.StorageURL, v.GroupsURL)
		if len(e.CDN) != len(v.CDNURLs) {
			t.Errorf("%s CDNs: %v, spec %v", e.Name, e.CDN, v.CDNURLs)
		}
		for n, u := range v.CDNURLs {
			num, _ := strconv.ParseUint(n, 10, 32)
			check("CDN "+n, e.CDN[uint32(num)], u)
		}
		check("CDSI URL", e.CDSIURL, v.CDSIURL)
		check("CDSI MRENCLAVE", e.CDSIMREnclave, v.CDSIMREnclave)
		check("SVR2 host", e.SVR2Host, host(t, v.SVR2URL))
		if len(e.SVR2) != len(v.SVR2Enclaves) {
			t.Fatalf("%s SVR2: %d enclaves, spec %d", e.Name, len(e.SVR2), len(v.SVR2Enclaves))
		}
		for i, w := range v.SVR2Enclaves {
			g := e.SVR2[i]
			check("SVR2 enclave", g.MREnclave, w.MREnclave)
			check("SVR2 group ID", strconv.FormatUint(g.GroupID, 10), w.GroupID)
			if g.Min != w.Min || g.Max != w.Max || g.SuperMajority != w.SuperMajority {
				t.Errorf("%s SVR2 %d Raft %d/%d/%d, spec %d/%d/%d", e.Name, i, g.Min, g.Max, g.SuperMajority, w.Min, w.Max, w.SuperMajority)
			}
			// The spec gives OkHttp's https:// form; on the wire it is wss://.
			check("SVR2 URL", e.SVR2URL(g), "wss://"+host(t, w.URL)+"/v1/"+w.MREnclave)
		}
		check("zkgroup server params", e.ZkgroupServerPublicParams, v.ZkgroupParams)
		if len(e.SealedSenderTrustRoots) != len(v.TrustRoots) {
			t.Fatalf("%s trust roots: %d, spec %d", e.Name, len(e.SealedSenderTrustRoots), len(v.TrustRoots))
		}
		for i, b64 := range v.TrustRoots {
			b, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatal(err)
			}
			check("sealed-sender trust root", e.SealedSenderTrustRoots[i], hex.EncodeToString(b))
		}
		if len(e.ZkgroupParams()) != 673 || len(e.TrustRoots()) != len(v.TrustRoots) {
			t.Errorf("%s: decoded params %d bytes, %d roots", e.Name, len(e.ZkgroupParams()), len(e.TrustRoots()))
		}
	}
	if len(vs) != len(All) {
		t.Errorf("environments.json has %d environments, built in %d", len(vs), len(All))
	}
}

func TestLookup(t *testing.T) {
	for cli, want := range map[string]*Environment{"staging": Staging, "live": Production} {
		if e, err := ByCLI(cli); err != nil || e != want {
			t.Errorf("ByCLI(%q) = %v, %v", cli, e, err)
		}
		if AccountName(cli) != want.Account {
			t.Errorf("AccountName(%q) = %q", cli, AccountName(cli))
		}
	}
	for acct, want := range map[string]*Environment{"STAGING": Staging, "LIVE": Production, "": Production} {
		if e, err := ByAccount(acct); err != nil || e != want {
			t.Errorf("ByAccount(%q) = %v, %v", acct, e, err)
		}
	}
	if _, err := ByAccount("TEST"); err == nil {
		t.Error("an unknown account environment was accepted")
	}
}
