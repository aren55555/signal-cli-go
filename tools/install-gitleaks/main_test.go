// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func writeTar(t *testing.T, entries map[string]byte) string {
	p := filepath.Join(t.TempDir(), "x.tar.gz")
	f, _ := os.Create(p)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, typ := range entries {
		body := []byte("binary")
		tw.WriteHeader(&tar.Header{Name: name, Typeflag: typ, Mode: 0o755, Size: int64(len(body)), Linkname: "x"})
		if typ == tar.TypeReg {
			tw.Write(body)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
	return p
}

func TestExtractTakesOnlyGitleaks(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "bin", "gitleaks")
	if err := extract(writeTar(t, map[string]byte{"README.md": tar.TypeReg, "../evil": tar.TypeReg, "gitleaks": tar.TypeReg}), dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "binary" {
		t.Errorf("extracted %q", b)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "..", "evil")); err == nil {
		t.Error("an entry outside the binary was written")
	}
}

func TestExtractRejectsNonRegular(t *testing.T) {
	if err := extract(writeTar(t, map[string]byte{"gitleaks": tar.TypeSymlink}), filepath.Join(t.TempDir(), "g")); err == nil {
		t.Error("a symlink named gitleaks was accepted")
	}
	if err := extract(writeTar(t, map[string]byte{"other": tar.TypeReg}), filepath.Join(t.TempDir(), "g")); err == nil {
		t.Error("a tarball without gitleaks was accepted")
	}
}
