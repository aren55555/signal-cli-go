// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/svr2"
	"maps"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// ErrNotRegistered reports an account the server no longer accepts; it
// has been marked unregistered locally (api.md §4.3).
var ErrNotRegistered = errors.New("account is not registered")

// Refresh rules (keys.md §3.2).
const (
	minOneTimeKeys = 10
	keyBatch       = 100
	rotationAge    = 2 * 24 * time.Hour
)

// accountAttributes is signal-cli's attributes object for this account.
func accountAttributes(a *store.Account) api.AccountAttributes {
	attrs := api.AccountAttributes{
		RegistrationID:                 a.Data.ACIRegistrationID,
		FetchesMessages:                true,
		UnidentifiedAccessKey:          api.UnidentifiedAccessKey(a.Data.ProfileKey),
		UnrestrictedUnidentifiedAccess: a.Data.UnrestrictedUnidentifiedAccess,
		DiscoverableByPhoneNumber:      !a.Data.Unlisted,
		Capabilities:                   api.PrimaryDeviceCapabilities,
		PNIRegistrationID:              a.Data.PNIRegistrationID,
	}
	if m, ok := pinBackedMasterKey(a); ok {
		lock, rec := svr2.RegistrationLock(m), svr2.RecoveryPassword(m)
		attrs.RegistrationLock, attrs.RecoveryPassword = &lock, &rec
	}
	if a.Data.DeviceID != 1 {
		attrs.Capabilities = api.LinkedDeviceCapabilities
		if a.Data.DeviceName != "" {
			name := a.Data.DeviceName
			attrs.Name = &name
		}
	}
	return attrs
}

// CheckAccount is the start-up account check (api.md §4.3): account
// attributes, then a prekey refresh for ACI and then PNI. A 401 or 403
// marks the account unregistered.
func CheckAccount(ctx context.Context, a *store.Account, c Requester, now time.Time) error {
	b, err := api.Marshal(accountAttributes(a))
	if err != nil {
		return err
	}
	r, err := c.Request(ctx, "PUT", "/v1/accounts/attributes", jsonHeader, b)
	if err != nil {
		return err
	}
	switch {
	case r.Status == 401 || r.Status == 403:
		a.Data.Registered = false
		if err := a.Save(); err != nil {
			return err
		}
		return ErrNotRegistered
	case r.Status == 499:
		return errors.New("signal-cli version is too old for the Signal-Server, please update")
	case r.Status < 200 || r.Status > 299:
		return &SendStatusError{Status: r.Status}
	}
	return RefreshPreKeys(ctx, a, c, now)
}

// RefreshPreKeys runs one refresh pass for ACI, then PNI (keys.md §3).
func RefreshPreKeys(ctx context.Context, a *store.Account, c Requester, now time.Time) error {
	if err := refreshIdentity(ctx, a, c, now, "aci", a.Data.ACIIdentity, &a.Data.ACIKeys, &a.Keys.ACI); err != nil {
		return err
	}
	return refreshIdentity(ctx, a, c, now, "pni", a.Data.PNIIdentity, &a.Data.PNIKeys, &a.Keys.PNI)
}

// needsRotation: no active key, unloadable, or older than two days.
func needsRotation(active int64, records map[uint32]store.StoredKey, kyber bool, now time.Time) bool {
	if active < 0 {
		return true
	}
	sk, ok := records[uint32(active)]
	if !ok {
		return true
	}
	var ts uint64
	if kyber {
		r, err := protocol.LoadKyberPreKeyRecord(sk.Record)
		if err != nil {
			return true
		}
		ts = r.Timestamp
	} else {
		r, err := protocol.LoadSignedPreKeyRecord(sk.Record)
		if err != nil {
			return true
		}
		ts = r.Timestamp
	}
	return now.Sub(time.UnixMilli(int64(ts))) > rotationAge
}

func refreshIdentity(ctx context.Context, a *store.Account, c Requester, now time.Time, which string, pair store.KeyPair,
	state *store.PreKeyState, keys *store.IdentityKeys) error {
	identity, err := curve25519.NewPrivateKey(pair.Private)
	if err != nil {
		return fmt.Errorf("%s identity: %w", which, err)
	}
	var counts api.PreKeyCounts
	r, err := c.Request(ctx, "GET", "/v2/keys?identity="+which, nil, nil)
	if err != nil {
		return err
	}
	switch {
	case r.Status == 200:
		if err := json.Unmarshal(r.Body, &counts); err != nil {
			return fmt.Errorf("prekey counts: %w", err)
		}
	case r.Status == 401 || r.Status == 403:
		// Taken as zero (§3.1).
	default:
		return &SendStatusError{Status: r.Status}
	}

	next := *state
	var up api.PreKeyUpload
	var newEC []*protocol.PreKeyRecord
	var newKyber []*protocol.KyberPreKeyRecord
	var newSigned *protocol.SignedPreKeyRecord
	var newLastResort *protocol.KyberPreKeyRecord
	if counts.Count < minOneTimeKeys {
		for i := range uint32(keyBatch) {
			k, err := protocol.GeneratePreKey(rand.Reader, (state.NextPreKeyID+i)%idSpace)
			if err != nil {
				return err
			}
			newEC = append(newEC, k)
			up.PreKeys = append(up.PreKeys, api.PreKey(k))
		}
		next.NextPreKeyID = (state.NextPreKeyID + keyBatch) % idSpace
	}
	if needsRotation(state.ActiveSignedPreKeyID, keys.SignedPreKeys, false, now) {
		if newSigned, err = protocol.GenerateSignedPreKey(rand.Reader, identity, state.NextSignedPreKeyID, now); err != nil {
			return err
		}
		up.SignedPreKey = api.SignedPreKey(newSigned)
		next.ActiveSignedPreKeyID = int64(newSigned.ID)
		next.NextSignedPreKeyID = (newSigned.ID + 1) % idSpace
	}
	if counts.PqCount < minOneTimeKeys {
		for i := range uint32(keyBatch) {
			k, err := protocol.GenerateKyberPreKey(rand.Reader, identity, (state.NextKyberPreKeyID+i)%idSpace, now)
			if err != nil {
				return err
			}
			newKyber = append(newKyber, k)
			up.PqPreKeys = append(up.PqPreKeys, *api.KyberPreKey(k))
		}
		next.NextKyberPreKeyID = (state.NextKyberPreKeyID + keyBatch) % idSpace
	}
	if needsRotation(state.ActiveLastResortKyberID, keys.KyberPreKeys, true, now) {
		// Right after this pass's one-time keys; not reduced (§3.2 item 4).
		id := state.NextKyberPreKeyID + uint32(len(newKyber))
		if newLastResort, err = protocol.GenerateKyberPreKey(rand.Reader, identity, id, now); err != nil {
			return err
		}
		up.PqLastResortPreKey = api.KyberPreKey(newLastResort)
		next.ActiveLastResortKyberID = int64(newLastResort.ID)
		next.NextKyberPreKeyID = (newLastResort.ID + 1) % idSpace
	}
	if newEC == nil && newKyber == nil && newSigned == nil && newLastResort == nil {
		return nil
	}

	b, err := api.Marshal(up)
	if err != nil {
		return err
	}
	if r, err = c.Request(ctx, "PUT", "/v2/keys?identity="+which, jsonHeader, b); err != nil {
		return err
	}
	switch {
	case r.Status >= 200 && r.Status <= 299:
	case r.Status == 400 || r.Status == 401:
		return nil // "Failed to updated pre keys": logged and ignored
	case r.Status == 422 && which == "pni":
		return nil // "Account needs to be reregistered to fix this"
	case r.Status == 429:
		return fmt.Errorf("rate limited while uploading prekeys")
	default:
		return &SendStatusError{Status: r.Status}
	}

	// Stored only after the server accepted them (§2.3, §3.2).
	nowMs := now.UnixMilli()
	markStale := func(m map[uint32]store.StoredKey, skipLastResort bool) map[uint32]store.StoredKey {
		out := maps.Clone(m)
		if out == nil {
			out = map[uint32]store.StoredKey{}
		}
		for id, k := range out {
			if k.StaleAt == 0 && !(skipLastResort && k.LastResort) {
				k.StaleAt = nowMs
				out[id] = k
			}
		}
		return out
	}
	if newEC != nil {
		keys.PreKeys = markStale(keys.PreKeys, false)
		for _, k := range newEC {
			keys.PreKeys[k.ID] = store.StoredKey{Record: k.Serialize()}
		}
	}
	if newKyber != nil {
		keys.KyberPreKeys = markStale(keys.KyberPreKeys, true)
		for _, k := range newKyber {
			keys.KyberPreKeys[k.ID] = store.StoredKey{Record: k.Serialize()}
		}
	}
	if newSigned != nil {
		keys.SignedPreKeys = maps.Clone(keys.SignedPreKeys)
		if keys.SignedPreKeys == nil {
			keys.SignedPreKeys = map[uint32]store.StoredKey{}
		}
		keys.SignedPreKeys[newSigned.ID] = store.StoredKey{Record: newSigned.Serialize()}
	}
	if newLastResort != nil {
		if keys.KyberPreKeys == nil {
			keys.KyberPreKeys = map[uint32]store.StoredKey{}
		} else if newKyber == nil {
			keys.KyberPreKeys = maps.Clone(keys.KyberPreKeys)
		}
		keys.KyberPreKeys[newLastResort.ID] = store.StoredKey{Record: newLastResort.Serialize(), LastResort: true}
	}
	*state = next
	if err := a.SaveKeys(); err != nil {
		return err
	}
	return a.Save()
}
