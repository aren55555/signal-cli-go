package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, body := range entries {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		e.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func TestExtractKeepsOnlyWantedFiles(t *testing.T) {
	z := writeZip(t, map[string]string{
		"bin/protoc":                        "binary",
		"include/google/protobuf/any.proto": "syntax",
		"readme.txt":                        "skip",
		"include/google/protobuf/notes.txt": "skip",
	})
	dest := t.TempDir()
	if err := extract(z, dest); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dest, "bin", "protoc"))
	if err != nil || st.Mode().Perm() != 0o755 {
		t.Fatalf("bin/protoc: %v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "include", "google", "protobuf", "any.proto")); err != nil {
		t.Error(err)
	}
	for _, skipped := range []string{"readme.txt", "include/google/protobuf/notes.txt"} {
		if _, err := os.Stat(filepath.Join(dest, skipped)); err == nil {
			t.Errorf("%s extracted", skipped)
		}
	}
}

func TestExtractRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../bin/protoc", "/bin/protoc", "bin/../../protoc", "bin\\protoc"} {
		z := writeZip(t, map[string]string{name: "x", "bin/protoc": "x"})
		if err := extract(z, t.TempDir()); err == nil {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestExtractRequiresProtoc(t *testing.T) {
	z := writeZip(t, map[string]string{"include/google/protobuf/any.proto": "x"})
	if err := extract(z, t.TempDir()); err == nil {
		t.Error("zip without bin/protoc accepted")
	}
}
