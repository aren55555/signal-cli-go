// Command install-protoc downloads protoc into third_party/, for
// regenerating the protobuf code in internal/proto (see `just proto-gen`).
// It is a build tool only, never a runtime dependency.
//
// The release zip is checked against a pinned SHA-256 before anything is
// extracted, and only bin/protoc and include/google/protobuf/*.proto are
// extracted, with paths checked.
//
// Usage (from the repo root): go run ./tools/install-protoc
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	version = "36.2"
	zipName = "protoc-" + version + "-linux-x86_64.zip"
	zipURL  = "https://github.com/protocolbuffers/protobuf/releases/download/v" + version + "/" + zipName
	// Source: the asset digest on the GitHub release page for v36.2
	// (github.com/protocolbuffers/protobuf/releases/tag/v36.2), read
	// 2026-10-23. protoc releases carry no signature.
	zipSHA256 = "121f6c7afe1d4d0e3ea6aab9432038599250134cbf4474cb1167d2c7decd4278"
)

// Dir is where protoc is installed, relative to the repo root.
const Dir = "third_party/protoc-" + version

func main() {
	root := flag.String("root", ".", "repo root")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintln(os.Stderr, "install-protoc:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	dest := filepath.Join(root, Dir)
	if _, err := os.Stat(filepath.Join(dest, "bin", "protoc")); err == nil {
		fmt.Println("already installed:", dest)
		return nil
	}
	dl := filepath.Join(root, "third_party", "downloads")
	if err := os.MkdirAll(dl, 0o755); err != nil {
		return err
	}
	zipPath := filepath.Join(dl, zipName)
	if err := download(zipURL, zipPath); err != nil {
		return err
	}
	sum, err := fileSHA256(zipPath)
	if err != nil {
		return err
	}
	if sum != zipSHA256 {
		os.Remove(zipPath)
		return fmt.Errorf("%s: sha256 %s, want %s", zipName, sum, zipSHA256)
	}
	fmt.Println("sha256 ok:", zipName)
	if err := extract(zipPath, dest); err != nil {
		os.RemoveAll(dest)
		return err
	}
	fmt.Println("installed:", filepath.Join(dest, "bin", "protoc"))
	return nil
}

func download(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// wanted reports whether a zip entry is one we extract, and its mode.
func wanted(name string) (bool, os.FileMode) {
	switch {
	case name == "bin/protoc":
		return true, 0o755
	case strings.HasPrefix(name, "include/google/protobuf/") && strings.HasSuffix(name, ".proto"):
		return true, 0o644
	}
	return false, 0
}

// extract writes the wanted entries of the zip under dest. Names must be
// clean, relative, and regular files; anything else is an error.
func extract(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	found := false
	for _, f := range r.File {
		name := f.Name
		if path.IsAbs(name) || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || name == ".." || strings.Contains(name, "\\") {
			return fmt.Errorf("unsafe path %q in zip", name)
		}
		ok, mode := wanted(name)
		if !ok {
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", name)
		}
		if err := writeEntry(f, filepath.Join(dest, filepath.FromSlash(name)), mode); err != nil {
			return err
		}
		if name == "bin/protoc" {
			found = true
		}
	}
	if !found {
		return errors.New("zip has no bin/protoc")
	}
	return nil
}

func writeEntry(f *zip.File, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(rc, 64<<20)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
