package cli

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/enclave"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/trust"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/svr2"
)

// cdsiDialer opens attested CDSI channels on staging (cdsi.md §2).
func (n *network) cdsiDialer() manager.CDSIDialer {
	return func(ctx context.Context, user, pass string) (cdsi.Channel, func(), error) {
		mr, _ := hex.DecodeString(cdsi.StagingMREnclave)
		c, err := enclave.Dial(ctx, cdsi.StagingURL, mr, enclave.DialOptions{Username: user, Password: pass,
			Roots: trust.ChatRoots(), Proxy: n.proxy, UserAgent: n.userAgent})
		if err != nil {
			return nil, nil, err
		}
		return c, func() { c.Close() }, nil
	}
}

// numberResolver resolves a phone-number recipient through CDSI
// (cdsi.md §6.6); set per run by main, nil in offline tests.
var numberResolver func(a *store.Account, number string) (string, bool)

// cdsiResolver opens its own authenticated connection for one lookup.
func cdsiResolver(e *env, g *globals) func(a *store.Account, number string) (string, bool) {
	return func(a *store.Account, number string) (string, bool) {
		n, err := e.network(g)
		if err != nil {
			return "", false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		c, err := n.dialChat(ctx, a, false)
		if err != nil {
			return "", false
		}
		defer c.Close()
		s := g.withProfiles(e, &manager.Sender{Acct: a, Chat: c, DialCDSI: n.cdsiDialer()})
		return s.ResolveNumber(ctx, number)
	}
}

// svr2Dialer opens attested SVR2 channels on staging (svr.md §3.1).
func (n *network) svr2Dialer() manager.SVR2Dialer {
	return func(ctx context.Context, en svr2.Enclave, user, pass string) (cdsi.Channel, map[string][]byte, func(), error) {
		mr, _ := hex.DecodeString(en.MREnclave)
		c, err := enclave.Dial(ctx, en.URL(), mr, enclave.DialOptions{Username: user, Password: pass,
			Roots: trust.RESTRoots(), Proxy: n.proxy, UserAgent: n.userAgent})
		if err != nil {
			return nil, nil, nil, err
		}
		return c, c.Claims, func() { c.Close() }, nil
	}
}
