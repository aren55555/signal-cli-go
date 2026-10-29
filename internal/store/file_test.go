// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.json")
	if err := WriteFileAtomic(p, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Errorf("content %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
	// A crash mid-write leaves a temp file and the old document intact;
	// readers never see the temp file.
	os.WriteFile(filepath.Join(dir, ".doc.json.tmp-123"), []byte("half"), 0o600)
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Errorf("content after a crashed write %q", b)
	}
	if err := WriteFileAtomic(filepath.Join(dir, "missing", "x"), nil, 0o600); err == nil {
		t.Error("write into a missing directory succeeded")
	}
}

type docV2 struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	Count   int    `json:"count"`
}

// TestVersionedJSON: documents carry a version; older ones are migrated
// step by step on load, newer ones are refused.
func TestVersionedJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.json")
	migrations := Migrations{
		1: func(m map[string]any) error { m["name"] = m["title"]; delete(m, "title"); return nil },
		2: func(m map[string]any) error { m["count"] = 1; return nil },
	}
	os.WriteFile(p, []byte(`{"version":0,"title":"old"}`), 0o600)
	var d docV2
	if err := ReadJSON(p, 2, migrations, &d); err != nil {
		t.Fatal(err)
	}
	if d.Version != 2 || d.Name != "old" || d.Count != 1 {
		t.Errorf("migrated %+v", d)
	}

	if err := WriteJSON(p, &docV2{Version: 2, Name: "new"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	var raw map[string]any
	if json.Unmarshal(b, &raw) != nil || raw["name"] != "new" {
		t.Errorf("written %s", b)
	}

	os.WriteFile(p, []byte(`{"version":3}`), 0o600)
	if err := ReadJSON(p, 2, migrations, &d); !errors.Is(err, ErrNewerVersion) {
		t.Errorf("newer version: %v", err)
	}
	os.WriteFile(p, []byte(`{"version":0}`), 0o600)
	if err := ReadJSON(p, 2, Migrations{2: migrations[2]}, &d); err == nil || !strings.Contains(err.Error(), "no migration") {
		t.Errorf("missing migration: %v", err)
	}
	if err := ReadJSON(filepath.Join(dir, "absent.json"), 2, nil, &d); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent file: %v", err)
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	l, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(dir); !errors.Is(err, ErrLocked) {
		t.Errorf("second lock: %v", err)
	}
	l.Unlock()
	l2, err := Lock(dir)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	l2.Unlock()
}
