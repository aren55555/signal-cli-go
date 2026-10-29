// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// CDSI lookups (spec/service/cdsi.md §6).

// CDSIDialer opens an attested CDSI channel with the given credentials.
type CDSIDialer func(ctx context.Context, username, password string) (cdsi.Channel, func(), error)

// MaxOneOffLookup is the largest partial lookup (§6.1).
const MaxOneOffLookup = 3

const recipientsRefreshAge = 24 * time.Hour

func (s *Sender) cdsiCredentials(ctx context.Context) (string, string, error) {
	r, err := s.Chat.Request(ctx, "GET", "/v2/directory/auth", nil, nil)
	if err != nil {
		return "", "", err
	}
	switch {
	case r.Status == 401 || r.Status == 403:
		return "", "", fmt.Errorf("[%d] Authorization failed!", r.Status)
	case r.Status == 413 || r.Status == 429:
		return "", "", errors.New("[429] Rate Limited")
	case r.Status != 200:
		return "", "", fmt.Errorf("StatusCode: %d", r.Status)
	}
	var c struct{ Username, Password string }
	if err := json.Unmarshal(r.Body, &c); err != nil {
		return "", "", err
	}
	return c.Username, c.Password, nil
}

// cdsiPairs is one ACI/access-key pair per recipient with both, the
// account included (§6.2).
func (s *Sender) cdsiPairs() []cdsi.Pair {
	a := s.Acct
	var out []cdsi.Pair
	add := func(aci string, pk []byte) {
		u, ok := uuidRaw(aci)
		if !ok || len(pk) != 32 {
			return
		}
		out = append(out, cdsi.Pair{ACI: u, AccessKey: [16]byte(api.UnidentifiedAccessKey(pk))})
	}
	add(a.Data.ACI, a.Data.ProfileKey)
	for _, k := range a.SortedRecipientKeys() {
		if k == a.Data.ACI {
			continue
		}
		add(a.AddressOf(k).ACI, a.Recipients[k].ProfileKey)
	}
	return out
}

// storedNumbers is every stored recipient number but the own (§6.1).
func (s *Sender) StoredNumbers() []string {
	var out []string
	for _, k := range s.Acct.SortedRecipientKeys() {
		n := s.Acct.AddressOf(k).Number
		if n != "" && n != s.Acct.Data.Number {
			if _, err := strconv.ParseInt(n, 10, 64); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// LookupNumbers runs one CDSI lookup for requested, partial or full
// (§6.2-§6.4), and applies the results to the recipient store.
func (s *Sender) LookupNumbers(ctx context.Context, requested []string, full bool) (map[string]cdsi.Entry, error) {
	a := s.Acct
	if s.DialCDSI == nil {
		return nil, errors.New("CDSI is not available")
	}
	req := &cdsi.Request{Pairs: s.cdsiPairs()}
	var prev []string
	if full {
		seen := map[string]bool{}
		for n := range a.Data.CDSINumbers {
			prev = append(prev, n)
			seen[n] = true
		}
		for _, n := range requested {
			if !seen[n] {
				req.New = append(req.New, n)
				seen[n] = true
			}
		}
		if len(prev) > 0 && len(a.Data.CDSIToken) > 0 {
			req.Prev, req.Token = cdsi.Sorted(prev), a.Data.CDSIToken
		}
		if len(req.New) == 0 && len(prev) == 0 {
			return map[string]cdsi.Entry{}, nil
		}
	} else {
		req.New = requested
	}
	req.New = cdsi.Sorted(req.New)
	user, pass, err := s.cdsiCredentials(ctx)
	if err != nil {
		return nil, err
	}
	ch, closeFn, err := s.DialCDSI(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	now := s.now().UnixMilli()
	resp, err := cdsi.Lookup(ch, req, func(token []byte) {
		if a.Data.CDSINumbers == nil {
			a.Data.CDSINumbers = map[string]int64{}
		}
		if !full {
			for _, n := range requested {
				if _, ok := a.Data.CDSINumbers[n]; ok {
					a.Data.CDSINumbers[n] = now
				}
			}
			a.Save()
			return
		}
		keep := map[string]bool{}
		for _, n := range append(append([]string{}, prev...), req.New...) {
			keep[n] = true
		}
		for n := range a.Data.CDSINumbers {
			if !keep[n] {
				delete(a.Data.CDSINumbers, n)
			}
		}
		for n := range keep {
			if _, ok := a.Data.CDSINumbers[n]; !ok {
				a.Data.CDSINumbers[n] = now
			}
		}
		for _, n := range append(append([]string{}, requested...), req.New...) {
			a.Data.CDSINumbers[n] = now
		}
		a.Data.CDSIToken, a.Data.LastRecipientsRefresh = token, now
		a.Save()
	})
	var ce *cdsi.CloseError
	if errors.Is(err, cdsi.ErrInvalidNumber) || (errors.As(err, &ce) && (ce.IsInvalidToken() || ce.IsInvalidArgument())) {
		a.Data.CDSIToken, a.Data.CDSINumbers = nil, nil
		a.Save()
	}
	if err != nil {
		return nil, err
	}
	results, err := resp.Results()
	if err != nil {
		return nil, err
	}
	s.applyCDSI(requested, results)
	return results, nil
}

// applyCDSI is §6.4.
func (s *Sender) applyCDSI(requested []string, results map[string]cdsi.Entry) {
	a := s.Acct
	yes := true
	for n, e := range results {
		k := a.ResolveHighTrust(store.Address{ACI: e.ACI, PNI: e.PNI, Number: n})
		rc := a.Recipients[k]
		rc.Discoverable = &yes
		a.Recipients[k] = rc
	}
	for _, n := range requested {
		if _, ok := results[n]; ok {
			continue
		}
		k, ok := a.FindByNumber(n)
		if !ok {
			continue
		}
		rc := a.Recipients[k]
		no := false
		rc.Discoverable = &no
		a.Recipients[k] = rc
		if a.AddressOf(k).ACI == "" || rc.Unregistered {
			a.MarkUnregistered(k, s.now().UnixMilli())
		}
	}
	a.SaveRecipients()
}

// ResolveNumber is §6.6: a stored recipient with that number and a
// service ID, else a partial lookup. Any lookup failure is "not found".
func (s *Sender) ResolveNumber(ctx context.Context, number string) (string, bool) {
	a := s.Acct
	if k, ok := a.FindByNumber(number); ok {
		if ad := a.AddressOf(k); ad.ACI != "" || ad.PNI != "" {
			return k, true
		}
	}
	res, err := s.LookupNumbers(ctx, []string{number}, false)
	if err != nil {
		s.logf("Failed to get uuid for e164 number: %s", number)
		return "", false
	}
	e, ok := res[number]
	if !ok {
		return "", false
	}
	if e.ACI != "" {
		return e.ACI, true
	}
	return "PNI:" + e.PNI, true
}

// RefreshRecipients is the scheduled full refresh at account open when
// the last one is older than 24 h (§6.1); failures are logged.
func (s *Sender) RefreshRecipients(ctx context.Context) {
	a := s.Acct
	if a.Data.LastRecipientsRefresh != 0 && s.now().Sub(time.UnixMilli(a.Data.LastRecipientsRefresh)) < recipientsRefreshAge {
		return
	}
	if _, err := s.LookupNumbers(ctx, s.StoredNumbers(), true); err != nil {
		s.logf("Full CDSI recipients refresh failed, ignoring: %v", err)
	}
}
