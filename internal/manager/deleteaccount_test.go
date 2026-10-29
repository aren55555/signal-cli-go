// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"context"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/service/chat"
)

// accountServer records requests and answers DELETE /v1/accounts/me
// with status.
type accountServer struct {
	status int
	paths  []string
}

func (f *accountServer) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	f.paths = append(f.paths, verb+" "+path)
	if verb == "DELETE" && path == "/v1/accounts/me" {
		return &chat.Response{Status: f.status}, nil
	}
	return &chat.Response{Status: 500}, nil
}

// TestDeleteAccount: svr.md §4.5 — a failing PIN removal does not stop
// the deletion, the local PIN is cleared, and the server status maps to
// an error. No account is deleted anywhere: the server is a fake.
func TestDeleteAccount(t *testing.T) {
	a, _ := testAccountFor(t, testNumber, testACI, testPNI, 1, nil)
	a.Data.RegistrationLockPin, a.Data.PinMasterKey = "1234", make([]byte, 32)
	srv := &accountServer{status: 204}
	s := &Sender{Acct: a, Chat: srv}
	pinErr, err := s.DeleteAccount(context.Background())
	if err != nil || pinErr == nil {
		t.Fatalf("err %v, pinErr %v (want a logged PIN failure and success)", err, pinErr)
	}
	if a.Data.RegistrationLockPin != "" || a.Data.PinMasterKey != nil {
		t.Error("local PIN not cleared")
	}
	if last := srv.paths[len(srv.paths)-1]; last != "DELETE /v1/accounts/me" {
		t.Errorf("last request %s", last)
	}
	srv.status = 401
	if _, err := s.DeleteAccount(context.Background()); err == nil || err.Error() != "[401] " {
		t.Errorf("401: %v", err)
	}
}
