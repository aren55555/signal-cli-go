// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"testing"
	"time"
)

type fakeRetrier struct{ called chan struct{} }

func (f *fakeRetrier) RetryCache(context.Context) error {
	f.called <- struct{}{}
	return nil
}

// TestRetryFailedAfterTrust: trust changed while the daemon receives
// schedules a retry of the cached envelopes on the running receive loop
// (trust.md), once the command holding the account has released it.
// Found live: a message held for an untrusted identity never arrived after
// `trust` until the daemon happened to reconnect.
func TestRetryFailedAfterTrust(t *testing.T) {
	d := &daemonState{ctx: context.Background()}
	r := &fakeRetrier{called: make(chan struct{}, 1)}
	la := &liveAccount{receiver: r}

	la.mu.Lock() // the trust command still holds the account
	d.retryFailed(la)
	select {
	case <-r.called:
		t.Fatal("retried while the command still held the account")
	case <-time.After(50 * time.Millisecond):
	}
	la.mu.Unlock()
	select {
	case <-r.called:
	case <-time.After(2 * time.Second):
		t.Fatal("no retry after trust")
	}

	// Without a running receive loop there is nothing to retry.
	d.retryFailed(&liveAccount{})
}
