package manager

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// fakeStorage is the storage host with storage.md §5.3's write rules.
type fakeStorage struct {
	t        *testing.T
	manifest *signalpb.StorageManifest
	items    map[string][]byte
	writes   int
}

func (f *fakeStorage) Raw(ctx context.Context, method, path string, headers map[string]string, body []byte) (*rest.RawResponse, error) {
	if !strings.HasPrefix(headers["Authorization"], "Basic ") {
		f.t.Errorf("%s %s without auth", method, path)
	}
	resp := func(status int, m proto.Message) (*rest.RawResponse, error) {
		var b []byte
		if m != nil {
			b, _ = proto.Marshal(m)
		}
		return &rest.RawResponse{Status: status, Body: b}, nil
	}
	switch {
	case method == "GET" && strings.HasPrefix(path, "/v1/storage/manifest/version/"):
		v, _ := strconv.ParseInt(strings.TrimPrefix(path, "/v1/storage/manifest/version/"), 10, 64)
		if f.manifest == nil || int64(f.manifest.GetVersion()) == v {
			return resp(204, nil)
		}
		return resp(200, f.manifest)
	case method == "GET" && path == "/v1/storage/manifest":
		if f.manifest == nil {
			return resp(404, nil)
		}
		return resp(200, f.manifest)
	case method == "PUT" && path == "/v1/storage/read":
		var op signalpb.StorageReadOperation
		proto.Unmarshal(body, &op)
		out := &signalpb.StorageItems{}
		for _, k := range op.GetReadKey() {
			if v, ok := f.items[string(k)]; ok {
				out.Items = append(out.Items, &signalpb.StorageItem{Key: k, Value: v})
			}
		}
		return resp(200, out)
	case method == "PUT" && path == "/v1/storage":
		var op signalpb.StorageWriteOperation
		proto.Unmarshal(body, &op)
		if op.GetClearAll() {
			f.items = map[string][]byte{}
		}
		if f.manifest != nil && op.GetManifest().GetVersion() != f.manifest.GetVersion()+1 {
			return resp(409, f.manifest)
		}
		f.manifest = op.GetManifest()
		for _, it := range op.GetInsertItem() {
			f.items[string(it.GetKey())] = it.GetValue()
		}
		for _, k := range op.GetDeleteKey() {
			delete(f.items, string(k))
		}
		f.writes++
		return resp(200, nil)
	}
	f.t.Errorf("unexpected %s %s", method, path)
	return resp(500, nil)
}

const testAEP = "0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqr"

func storageDevice(t *testing.T, srv *fakeStorage, device int) *Storage {
	a, _ := testAccountFor(t, testNumber, testACI, testPNI, device, nil)
	a.Data.AccountEntropyPool = testAEP
	fc := &fakeChat{t: t, sender: testACI, senderDev: uint32(device)}
	s := &Sender{Acct: a, Chat: authChat{fc}}
	return &Storage{Sender: s, REST: srv}
}

// authChat adds the storage auth endpoint to the fake chat.
type authChat struct{ f *fakeChat }

func (c authChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	if path == "/v1/storage/auth" {
		return &chat.Response{Status: 200, Body: []byte(`{"username":"` + testACI + `","password":"p"}`)}, nil
	}
	return c.f.Request(ctx, verb, path, headers, body)
}

func TestStorageSyncBetweenDevices(t *testing.T) {
	srv := &fakeStorage{t: t, items: map[string][]byte{}}
	peer := "b0b0b0b0-0000-4000-8000-000000000002"
	d1 := storageDevice(t, srv, 1)
	given := "Bob"
	d1.Sender.Acct.Recipients[peer] = store.Recipient{ID: 9, GivenName: &given, ProfileSharing: true, ProfileKey: make([]byte, 32)}
	d1.Sender.Acct.Data.Username = "alice.01"
	ctx := context.Background()
	if err := d1.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if srv.manifest.GetVersion() != 1 || len(srv.items) != 2 {
		t.Fatalf("first write: version %d, %d items", srv.manifest.GetVersion(), len(srv.items))
	}
	// Nothing changed: no second write.
	if err := d1.Sync(ctx); err != nil || srv.writes != 1 {
		t.Fatalf("idle sync wrote: %v %d", err, srv.writes)
	}

	// A second device with the same entropy pool reads both records.
	d2 := storageDevice(t, srv, 2)
	if err := d2.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	rc := d2.Sender.Acct.Recipients[peer]
	if rc.GivenName == nil || *rc.GivenName != "Bob" || !rc.ProfileSharing || d2.Sender.Acct.Data.Username != "alice.01" {
		t.Fatalf("device 2 got %+v, username %q", rc, d2.Sender.Acct.Data.Username)
	}
	// Device 2 re-uploads the account record once: a remote account
	// record without Payments never merges as equal (storage.md §6.4.3).
	if srv.writes != 2 || srv.manifest.GetVersion() != 2 {
		t.Errorf("device 2: %d writes, version %d", srv.writes, srv.manifest.GetVersion())
	}
	if err := d2.Sync(ctx); err != nil || srv.writes != 2 {
		t.Errorf("device 2 idle sync wrote: %v", err)
	}

	// Device 2 blocks the contact; device 1 sees it after its next sync.
	rc.Blocked = true
	d2.Sender.Acct.Recipients[peer] = rc
	if err := d2.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if srv.manifest.GetVersion() != 3 {
		t.Fatalf("change not written: version %d", srv.manifest.GetVersion())
	}
	if err := d1.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if !d1.Sender.Acct.Recipients[peer].Blocked {
		t.Error("device 1 did not apply the block")
	}
	if len(srv.items) != 2 {
		t.Errorf("%d items after the update (old IDs not deleted?)", len(srv.items))
	}
}

func TestStorageUndecryptableManifestForcePush(t *testing.T) {
	srv := &fakeStorage{t: t, items: map[string][]byte{"old": []byte("x")}}
	srv.manifest = &signalpb.StorageManifest{Version: 7, Value: []byte("garbage-not-decryptable-123456789")}
	d1 := storageDevice(t, srv, 1)
	if err := d1.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if srv.manifest.GetVersion() != 8 || srv.items["old"] != nil {
		t.Errorf("force push: version %d, old item kept %v", srv.manifest.GetVersion(), srv.items["old"] != nil)
	}
	_ = base64.StdEncoding
}
