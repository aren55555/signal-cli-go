// SPDX-License-Identifier: GPL-3.0-or-later

package importer

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/store"
)

// TestReferenceDirs imports copies of data directories written by the
// reference signal-cli 0.14.7 and checks the result against their
// databases row by row. Opt-in, since the directories hold live staging
// accounts and are never committed:
//
//	IMPORTER_TEST_DIRS=/path/one:/path/two go test ./internal/importer -run TestReferenceDirs
//
// Each directory must be a copy; the test also checks that the import
// left it byte for byte unchanged.
func TestReferenceDirs(t *testing.T) {
	dirs := os.Getenv("IMPORTER_TEST_DIRS")
	if dirs == "" {
		t.Skip("IMPORTER_TEST_DIRS not set")
	}
	for _, src := range filepath.SplitList(dirs) {
		t.Run(filepath.Base(src), func(t *testing.T) {
			before := treeHash(t, src)
			dst := t.TempDir()
			res, err := Import(Options{Source: src, Dest: dst, Open: openTest})
			if err != nil {
				t.Fatal(err)
			}
			if after := treeHash(t, src); after != before {
				t.Fatal("the import changed the source directory")
			}
			for _, r := range res {
				if r.Err != nil {
					t.Logf("account %s not imported: %v", r.Path, r.Err)
					continue
				}
				t.Logf("account %s imported; skipped %q", r.Path, r.Skipped)
				compareAccount(t, src, dst, r.Path)
			}
		})
	}
}

func treeHash(t *testing.T, dir string) string {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fi, _ := d.Info()
		sum := sha256.Sum256(b)
		h.Write([]byte(path + " " + fi.Mode().String() + " " + fi.ModTime().String() + " " + hex.EncodeToString(sum[:]) + "\n"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func compareAccount(t *testing.T, src, dst, path string) {
	tmp := t.TempDir()
	if err := copyDatabase(filepath.Join(src, "data", path+".d"), tmp); err != nil {
		t.Fatal(err)
	}
	db, err := openTest(filepath.Join(tmp, "account.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := store.Open(filepath.Join(dst, "data")).Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	count := func(q string) int {
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	check := func(what string, db, ours int) {
		if db != ours {
			t.Errorf("%s: %d in account.db, %d imported", what, db, ours)
		}
	}
	for scope, keys := range []store.IdentityKeys{a.Keys.ACI, a.Keys.PNI} {
		s := strconv.Itoa(scope)
		check("pre_key "+s, count("SELECT count(*) FROM pre_key WHERE account_id_type = "+s), len(keys.PreKeys))
		check("signed_pre_key "+s, count("SELECT count(*) FROM signed_pre_key WHERE account_id_type = "+s), len(keys.SignedPreKeys))
		check("kyber_pre_key "+s, count("SELECT count(*) FROM kyber_pre_key WHERE account_id_type = "+s), len(keys.KyberPreKeys))
	}
	check("session", count("SELECT count(*) FROM session"), len(a.Sessions))
	check("identity", count("SELECT count(*) FROM identity"), len(a.Identities))
	check("sender_key", count("SELECT count(*) FROM sender_key"), len(a.SenderKeys.Records))
	shared := 0
	for _, m := range a.SenderKeys.Shared {
		shared += len(m)
	}
	check("sender_key_shared", count("SELECT count(*) FROM sender_key_shared"), shared)
	check("recipient", count("SELECT count(*) FROM recipient WHERE aci IS NOT NULL OR pni IS NOT NULL OR number IS NOT NULL"), len(a.Recipients))
	check("group_v2", count("SELECT count(*) FROM group_v2"), len(a.Groups))
	check("sticker", count("SELECT count(*) FROM sticker"), len(a.Stickers))

	// Identity listing order and recipient row numbers.
	var order []string
	each(db, "SELECT address FROM identity ORDER BY _id", func(r *sql.Rows) error {
		var s string
		r.Scan(&s)
		order = append(order, s)
		return nil
	})
	if got := a.IdentityOrder(); !slices.Equal(got, order) {
		t.Errorf("identity order %q, account.db %q", got, order)
	}
	var ids []int64
	each(db, "SELECT _id FROM recipient WHERE aci IS NOT NULL OR pni IS NOT NULL OR number IS NOT NULL ORDER BY _id", func(r *sql.Rows) error {
		var id int64
		r.Scan(&id)
		ids = append(ids, id)
		return nil
	})
	var ours []int64
	for _, k := range a.SortedRecipientKeys() {
		ours = append(ours, a.Recipients[k].ID)
	}
	if !slices.Equal(ids, ours) {
		t.Errorf("recipient rows %v, account.db %v", ours, ids)
	}

	// Every session and sender key byte for byte.
	each(db, "SELECT account_id_type, address, device_id, record FROM session", func(r *sql.Rows) error {
		var scope int
		var addr string
		var dev int64
		var rec []byte
		r.Scan(&scope, &addr, &dev, &rec)
		k := []string{"aci", "pni"}[scope] + ":" + addr + "." + strconv.FormatInt(dev, 10)
		if string(a.Sessions[k]) != string(rec) {
			t.Errorf("session %s differs", strings.Split(k, ".")[0])
		}
		return nil
	})
}
