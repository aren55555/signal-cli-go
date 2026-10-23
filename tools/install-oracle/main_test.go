package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTarGz(t *testing.T, entries []tar.Header) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, h := range entries {
		h.Mode = 0o644
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(make([]byte, h.Size)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
	return path
}

func TestExtractRejectsEscapes(t *testing.T) {
	cases := map[string]tar.Header{
		"dotdot":       {Name: "../evil", Typeflag: tar.TypeReg, Size: 1},
		"nested":       {Name: "a/../../evil", Typeflag: tar.TypeReg, Size: 1},
		"absolute":     {Name: "/etc/evil", Typeflag: tar.TypeReg, Size: 1},
		"symlink out":  {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"},
		"symlink abs":  {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		"device entry": {Name: "dev", Typeflag: tar.TypeChar},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "out")
			if err := extract(writeTarGz(t, []tar.Header{h}), dest); err == nil {
				t.Fatalf("extract accepted %+v", h)
			}
		})
	}
}

func TestExtractAcceptsNormalLayout(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	arch := writeTarGz(t, []tar.Header{
		{Name: "signal-cli-0.14.7/", Typeflag: tar.TypeDir},
		{Name: "signal-cli-0.14.7/bin/signal-cli", Typeflag: tar.TypeReg, Size: 3},
		{Name: "signal-cli-0.14.7/lib/x.jar", Typeflag: tar.TypeReg, Size: 3},
		{Name: "signal-cli-0.14.7/lib/link", Typeflag: tar.TypeSymlink, Linkname: "x.jar"},
	})
	if err := extract(arch, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "signal-cli-0.14.7", "bin", "signal-cli")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(dest, "signal-cli-0.14.7", "lib", "link")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink not created: %v", err)
	}
}

func TestPinsLookSane(t *testing.T) {
	if len(tarballSHA256) != 64 || strings.ToLower(tarballSHA256) != tarballSHA256 {
		t.Errorf("tarballSHA256 malformed")
	}
	if len(signerFingerprint) != 40 {
		t.Errorf("signerFingerprint malformed")
	}
	if !strings.Contains(signerKeyFile, signerFingerprint) {
		t.Errorf("key file name must carry the fingerprint")
	}
}
