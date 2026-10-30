// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"sync"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/environment"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/svr2"
)

// accountEnv is the service environment an account belongs to
// (spec/service/environments.md). An account file names it; an unknown
// name is a broken store, not something to guess around.
func accountEnv(a *store.Account) *environment.Environment {
	e, err := environment.ByAccount(a.Data.Environment)
	if err != nil {
		panic(err)
	}
	return e
}

var paramsByEnv sync.Map // environment name -> *zkgroup.ServerPublicParams

// paramsFor returns the zkgroup server public params of a's environment.
func paramsFor(a *store.Account) *zkgroup.ServerPublicParams {
	e := accountEnv(a)
	if p, ok := paramsByEnv.Load(e.Name); ok {
		return p.(*zkgroup.ServerPublicParams)
	}
	p, err := zkgroup.ParseServerPublicParams(e.ZkgroupParams())
	if err != nil {
		panic("zkgroup: bad built-in server params for " + e.Name)
	}
	paramsByEnv.Store(e.Name, p)
	return p
}

// trustRootsFor returns the sealed-sender trust roots of a's environment.
func trustRootsFor(a *store.Account) []*curve25519.PublicKey {
	var out []*curve25519.PublicKey
	for _, b := range accountEnv(a).TrustRoots() {
		k, err := curve25519.ParsePublicKey(b)
		if err != nil {
			panic("environment: bad sealed-sender trust root")
		}
		out = append(out, k)
	}
	return out
}

// svr2Enclaves returns the SVR2 enclaves of the sender's environment.
func (s *Sender) svr2Enclaves() []svr2.Enclave { return svr2.Enclaves(s.env()) }

// env is the sender's environment: its account's, or before an account
// exists (registration lock recovery) the one it was given.
func (s *Sender) env() *environment.Environment {
	if s.Acct != nil {
		return accountEnv(s.Acct)
	}
	return envOrStaging(s.Env)
}

func envOrStaging(e *environment.Environment) *environment.Environment {
	if e == nil {
		return environment.Staging
	}
	return e
}
