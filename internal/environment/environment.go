// SPDX-License-Identifier: GPL-3.0-or-later

// Package environment holds the service values of Signal's staging and
// production environments (spec/service/environments.md): hosts, enclave
// identities, zkgroup server parameters and sealed-sender trust roots. It
// is a leaf package of plain values; the service packages take what they
// need from it. TestValuesMatchSpec compares every value with
// testdata/vectors/environments.json.
package environment

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// SVR2Enclave is one SVR2 enclave and the Raft group claims its
// attestation must carry (spec/enclave/attestation.md §7.2).
type SVR2Enclave struct {
	MREnclave     string // hex
	GroupID       uint64
	Min, Max      uint32
	SuperMajority uint32
}

// Environment is one service environment.
type Environment struct {
	Name    string // "staging" or "production"
	CLI     string // the --service-environment value: "staging" or "live"
	Account string // the environment string of account files and accounts.json

	ChatRESTURL      string // REST base URL of the chat service
	ChatHost         string // chat WebSocket host (libsignal-net)
	ProvisioningHost string // provisioning (linking) WebSocket host
	StorageURL       string // storage and groups service base URL
	CDN              map[uint32]string

	CDSIURL       string // full WebSocket URL of the discovery endpoint
	CDSIMREnclave string // hex
	SVR2Host      string
	SVR2          []SVR2Enclave // signal-cli's order: current, then legacy

	ZkgroupServerPublicParams string   // base64
	SealedSenderTrustRoots    []string // hex, 33-byte serialized keys
}

// Staging is Signal's staging environment.
var Staging = &Environment{
	Name: "staging", CLI: "staging", Account: "STAGING",

	ChatRESTURL:      "https://chat.staging.signal.org",
	ChatHost:         "grpc.chat.staging.signal.org",
	ProvisioningHost: "chat.staging.signal.org",
	StorageURL:       "https://storage-staging.signal.org",
	CDN: map[uint32]string{
		0: "https://cdn-staging.signal.org",
		2: "https://cdn2-staging.signal.org",
		3: "https://cdn3-staging.signal.org",
	},

	CDSIURL:       "wss://cdsi.staging.signal.org/v1/6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a/discovery",
	CDSIMREnclave: "6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a",
	SVR2Host:      "svr2.staging.signal.org",
	SVR2: []SVR2Enclave{
		{MREnclave: "3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302", GroupID: 5138641357881452604, Min: 3, Max: 9},
		{MREnclave: "97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535", GroupID: 2330628069874851020, Min: 3, Max: 9},
	},

	ZkgroupServerPublicParams: stagingZkgroupParams,
	SealedSenderTrustRoots: []string{
		"05ba98d43ce8844e0d519a1517e2f5f2850facade420b9652c4261d949cf4ac131",
		"058854ead3e3a8fe3a286644cd1b3538be14dfd5797653c9fd7d3d85ae2b12b926",
	},
}

// Production is Signal's production environment ("LIVE"). The CLI still
// refuses it (errcode.LiveUnsupported) until the operator approves a live
// check; these values only make it selectable then.
var Production = &Environment{
	Name: "production", CLI: "live", Account: "LIVE",

	ChatRESTURL:      "https://chat.signal.org",
	ChatHost:         "grpc.chat.signal.org",
	ProvisioningHost: "chat.signal.org",
	StorageURL:       "https://storage.signal.org",
	CDN: map[uint32]string{
		0: "https://cdn.signal.org",
		2: "https://cdn2.signal.org",
		3: "https://cdn3.signal.org",
	},

	CDSIURL:       "wss://cdsi.signal.org/v1/15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b/discovery",
	CDSIMREnclave: "15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b",
	SVR2Host:      "svr2.signal.org",
	SVR2: []SVR2Enclave{
		{MREnclave: "ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4", GroupID: 11311619198250676136, Min: 4, Max: 13, SuperMajority: 2},
		{MREnclave: "1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708", GroupID: 2076725645304009823, Min: 4, Max: 13, SuperMajority: 2},
	},

	ZkgroupServerPublicParams: productionZkgroupParams,
	SealedSenderTrustRoots: []string{
		"057bba408295cf9300f20b2dcdf3a04501aac8ba8ec0d2872fa20d92fdc81d6305",
		"054918d08fbdfa83e00c29f8f8073a22ef35df2bea903aff81af03ccbc45c6e93a",
	},
}

// All lists the known environments.
var All = []*Environment{Staging, Production}

// ByCLI returns the environment for a --service-environment value.
func ByCLI(name string) (*Environment, error) {
	for _, e := range All {
		if e.CLI == name {
			return e, nil
		}
	}
	return nil, fmt.Errorf("environment: unknown service environment %q", name)
}

// ByAccount returns the environment of an account file's environment
// string. An account file without one counts as LIVE, as in signal-cli
// (spec/service/environments.md §1).
func ByAccount(name string) (*Environment, error) {
	if name == "" {
		return Production, nil
	}
	for _, e := range All {
		if e.Account == name {
			return e, nil
		}
	}
	return nil, fmt.Errorf("environment: unknown account environment %q", name)
}

// ZkgroupParams returns the decoded zkgroup server public params.
func (e *Environment) ZkgroupParams() []byte {
	b, err := base64.StdEncoding.DecodeString(e.ZkgroupServerPublicParams)
	if err != nil {
		panic("environment: bad zkgroup params for " + e.Name)
	}
	return b
}

// TrustRoots returns the decoded sealed-sender trust roots.
func (e *Environment) TrustRoots() [][]byte {
	var out [][]byte
	for _, h := range e.SealedSenderTrustRoots {
		b, err := hex.DecodeString(h)
		if err != nil {
			panic("environment: bad trust root for " + e.Name)
		}
		out = append(out, b)
	}
	return out
}

// SVR2URL is the WebSocket URL of one SVR2 enclave.
func (e *Environment) SVR2URL(enc SVR2Enclave) string {
	return "wss://" + e.SVR2Host + "/v1/" + enc.MREnclave
}

// AccountName maps a --service-environment value to the environment string
// of account files: "staging" -> "STAGING", "live" -> "LIVE" (both are
// defined by signal-cli, whichever environments are built in).
func AccountName(cli string) string {
	switch cli {
	case "staging":
		return "STAGING"
	case "live":
		return "LIVE"
	}
	return ""
}
