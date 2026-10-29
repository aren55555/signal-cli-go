// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/aren55555/signal-cli-go/internal/cdsi"
	"github.com/aren55555/signal-cli-go/internal/enclave"
	"github.com/aren55555/signal-cli-go/internal/storage"
	"github.com/aren55555/signal-cli-go/internal/store"
	"github.com/aren55555/signal-cli-go/internal/svr2"
)

// PIN, SVR2 and registration lock (spec/service/svr.md, pin.md §4).

const aepAlphabetPin = "0123456789abcdefghijklmnopqrstuvwxyz"

// currentMasterKey is pin.md §4.2's current master key.
func currentMasterKey(a *store.Account) ([32]byte, bool) {
	if len(a.Data.PinMasterKey) == 32 {
		return [32]byte(a.Data.PinMasterKey), true
	}
	if a.Data.AccountEntropyPool != "" {
		return storage.MasterKey(a.Data.AccountEntropyPool), true
	}
	return [32]byte{}, false
}

// getOrCreateMasterKey generates and stores an AEP when there is none.
func getOrCreateMasterKey(a *store.Account) ([32]byte, error) {
	if m, ok := currentMasterKey(a); ok {
		return m, nil
	}
	pool := make([]byte, 0, 64)
	var b [1]byte
	for len(pool) < 64 {
		if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
			return [32]byte{}, err
		}
		if b[0] < 252 {
			pool = append(pool, aepAlphabetPin[int(b[0])%36])
		}
	}
	a.Data.AccountEntropyPool = string(pool)
	if err := a.Save(); err != nil {
		return [32]byte{}, err
	}
	return storage.MasterKey(a.Data.AccountEntropyPool), nil
}

// pinBackedMasterKey is the key whose lock token and recovery password
// account attributes carry (pin.md §4.2).
func pinBackedMasterKey(a *store.Account) ([32]byte, bool) {
	if a.Data.RegistrationLockPin == "" {
		return [32]byte{}, false
	}
	if a.Data.DeviceID != 1 {
		return currentMasterKey(a)
	}
	m, err := getOrCreateMasterKey(a)
	return m, err == nil
}

// SVR2Dialer opens an attested SVR2 channel and returns its claims.
type SVR2Dialer func(ctx context.Context, e svr2.Enclave, username, password string) (cdsi.Channel, map[string][]byte, func(), error)

// SVR2Error is an SVR2 operation failure; Message is signal-cli's text.
type SVR2Error struct{ Message string }

func (e *SVR2Error) Error() string { return e.Message }

// errEnclaveNotFound is a 404 on auth or upgrade.
var errEnclaveNotFound = errors.New("EnclaveNotFound")

func (s *Sender) svrCredentials(ctx context.Context) (string, string, error) {
	r, err := s.Chat.Request(ctx, "GET", "/v2/svr/auth", nil, nil)
	if err != nil {
		return "", "", err
	}
	if r.Status == 404 {
		return "", "", errEnclaveNotFound
	}
	if r.Status != 200 {
		return "", "", fmt.Errorf("StatusCode: %d", r.Status)
	}
	var c struct{ Username, Password string }
	if err := json.Unmarshal(r.Body, &c); err != nil {
		return "", "", err
	}
	return c.Username, c.Password, nil
}

// svrRequest runs one request on a fresh connection (svr.md §3.2).
func (s *Sender) svrRequest(ctx context.Context, e svr2.Enclave, user, pass string, req []byte) (*svr2.Response, error) {
	if s.DialSVR2 == nil {
		return nil, errors.New("SVR2 is not available")
	}
	ch, claims, closeFn, err := s.DialSVR2(ctx, e, user, pass)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	if !bytes.Equal(claims["config"], e.RaftConfig()) {
		return nil, errors.New("Claims must contain a raft group config matching the enclave")
	}
	if err := ch.Send(req); err != nil {
		return nil, err
	}
	b, err := ch.Recv()
	if err != nil {
		if code, _, ok := enclave.CloseCode(err); ok {
			if code == 1000 || code == 0 {
				return nil, errors.New("Websocket was closed with code 1000")
			}
			return nil, &SVR2Error{fmt.Sprintf("org.signal.network.exceptions.NonSuccessfulResponseCodeException: StatusCode: %d", code)}
		}
		return nil, err
	}
	return svr2.ParseResponse(b)
}

// SetPin is svr.md §4.1.
func (s *Sender) SetPin(ctx context.Context, pin string) error {
	a := s.Acct
	norm, err := svr2.NormalizePIN(pin)
	if err != nil {
		return err
	}
	m, err := getOrCreateMasterKey(a)
	if err != nil {
		return err
	}
	var lastErr error
	succeeded := false
	for _, e := range svr2.StagingEnclaves {
		user, pass, err := s.svrCredentials(ctx)
		if errors.Is(err, errEnclaveNotFound) {
			s.logf("Backup svr failed: EnclaveNotFound")
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		enc, acc, err := svr2.PINHash(norm, svr2.Salt(user, e.GroupID))
		if err != nil {
			return err
		}
		blob := svr2.EncryptMasterKey(enc, m)
		r, err := s.svrRequest(ctx, e, user, pass, svr2.BackupRequest(blob, acc[:], 10))
		if err == nil && (r.Kind != 1 || r.Status != svr2.StatusOK) {
			err = &SVR2Error{"java.lang.IllegalStateException: Backup response not set!"}
			if r.Kind == 1 {
				err = &SVR2Error{fmt.Sprintf("java.lang.IllegalStateException: Unknown status: %d", r.Status)}
			}
		}
		if err == nil {
			r, err = s.svrRequest(ctx, e, user, pass, svr2.ExposeRequest(blob))
			if err == nil && r.Kind == 4 && r.Status == svr2.ExposeError {
				s.logf("Backup svr failed: ExposeFailure")
				continue
			}
			if err == nil && (r.Kind != 4 || r.Status != svr2.StatusOK) {
				err = &SVR2Error{"java.lang.IllegalStateException: Backup response not set!"}
			}
		}
		if err != nil {
			lastErr = err
			continue
		}
		succeeded = true
	}
	if !succeeded && lastErr != nil {
		return lastErr
	}
	body, _ := json.Marshal(map[string]string{"registrationLock": svr2.RegistrationLock(m)})
	r, err := s.Chat.Request(ctx, "PUT", "/v1/accounts/registration_lock", jsonHeader, body)
	if err != nil {
		return err
	}
	if r.Status < 200 || r.Status > 299 {
		return fmt.Errorf("StatusCode: %d", r.Status)
	}
	a.Data.RegistrationLockPin = pin
	if err := a.Save(); err != nil {
		return err
	}
	return s.UpdateAccountAttributes(ctx)
}

// RemovePin is svr.md §4.2.
func (s *Sender) RemovePin(ctx context.Context) error {
	a := s.Acct
	var lastErr error
	succeeded := false
	for _, e := range svr2.StagingEnclaves {
		user, pass, err := s.svrCredentials(ctx)
		if errors.Is(err, errEnclaveNotFound) {
			s.logf("Delete svr2 failed: EnclaveNotFound")
			continue
		}
		if err == nil {
			_, err = s.svrRequest(ctx, e, user, pass, svr2.DeleteRequest())
		}
		if err != nil {
			lastErr = err
			continue
		}
		succeeded = true
	}
	if !succeeded && lastErr != nil {
		return lastErr
	}
	r, err := s.Chat.Request(ctx, "DELETE", "/v1/accounts/registration_lock", nil, nil)
	if err != nil {
		return err
	}
	if r.Status < 200 || r.Status > 299 {
		return fmt.Errorf("StatusCode: %d", r.Status)
	}
	a.Data.RegistrationLockPin = ""
	return a.Save()
}

// PinMismatchError is a wrong PIN with the tries left (svr.md §4.3).
type PinMismatchError struct{ Tries uint32 }

func (e *PinMismatchError) Error() string {
	return fmt.Sprintf("Verification failed! Invalid pin, tries remaining: %d", e.Tries)
}

// ErrPinDataDeleted: locked, but nothing to restore.
var ErrPinDataDeleted = errors.New("Account is pin locked, but pin data has been deleted on the server.")

// RestoreMasterKey restores the master key with the 423 body's
// credentials (svr.md §4.3 step 4).
func (s *Sender) RestoreMasterKey(ctx context.Context, user, pass, pin string) ([32]byte, error) {
	norm, err := svr2.NormalizePIN(pin)
	if err != nil {
		return [32]byte{}, err
	}
	var lastErr error
	for _, e := range svr2.StagingEnclaves {
		enc, acc, err := svr2.PINHash(norm, svr2.Salt(user, e.GroupID))
		if err != nil {
			return [32]byte{}, err
		}
		r, err := s.svrRequest(ctx, e, user, pass, svr2.RestoreRequest(acc[:]))
		if err != nil {
			lastErr = err
			continue
		}
		switch {
		case r.Kind == 2 && r.Status == svr2.StatusOK:
			m, err := svr2.DecryptMasterKey(enc, r.Data)
			if err != nil {
				lastErr = &SVR2Error{"org.whispersystems.signalservice.api.crypto.InvalidCiphertextException"}
				continue
			}
			return m, nil
		case r.Kind == 2 && r.Status == svr2.RestoreMissing:
			continue
		case r.Kind == 2 && r.Status == svr2.RestorePinMismatch:
			return [32]byte{}, &PinMismatchError{Tries: r.Tries}
		default:
			lastErr = &SVR2Error{fmt.Sprintf("java.lang.IllegalStateException: Unknown status: %d", r.Status)}
		}
	}
	if lastErr != nil {
		return [32]byte{}, lastErr
	}
	return [32]byte{}, ErrPinDataDeleted
}
