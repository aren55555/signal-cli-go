// SPDX-License-Identifier: GPL-3.0-or-later

// Command install-gitleaks downloads gitleaks into third_party/bin, for the
// secret scan over the repository history (`just secret-scan`). It is a
// build tool only, never a runtime dependency.
//
// The release tarball is checked against a pinned SHA-256 before anything
// is extracted, and only the gitleaks binary is extracted.
//
// Usage (from the repo root): go run ./tools/install-gitleaks
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	version = "8.30.1"
	tarName = "gitleaks_" + version + "_linux_x64.tar.gz"
	tarURL  = "https://github.com/gitleaks/gitleaks/releases/download/v" + version + "/" + tarName
	// Source: the release's gitleaks_8.30.1_checksums.txt and the asset
	// digest in the GitHub release API for v8.30.1, which agree; read
	// 2026-10-30.
	tarSHA256 = "551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb"
)

// Bin is the installed binary, relative to the repo root.
const Bin = "third_party/bin/gitleaks"

func main() {
	root := flag.String("root", ".", "repo root")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintln(os.Stderr, "install-gitleaks:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	dst := filepath.Join(root, Bin)
	if _, err := os.Stat(dst); err == nil {
		fmt.Println("already installed:", dst)
		return nil
	}
	dl := filepath.Join(root, "third_party", "downloads")
	if err := os.MkdirAll(dl, 0o755); err != nil {
		return err
	}
	tarPath := filepath.Join(dl, tarName)
	if err := download(tarURL, tarPath); err != nil {
		return err
	}
	sum, err := fileSHA256(tarPath)
	if err != nil {
		return err
	}
	if sum != tarSHA256 {
		os.Remove(tarPath)
		return fmt.Errorf("%s: sha256 %s, want %s", tarName, sum, tarSHA256)
	}
	fmt.Println("sha256 ok:", tarName)
	if err := extract(tarPath, dst); err != nil {
		os.Remove(dst)
		return err
	}
	fmt.Println("installed:", dst)
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

// extract writes the tarball's top-level regular file "gitleaks" to dst;
// every other entry is skipped.
func extract(tarPath, dst string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("tarball has no gitleaks binary")
		}
		if err != nil {
			return err
		}
		if h.Name != "gitleaks" {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("gitleaks is not a regular file")
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.LimitReader(tr, 256<<20)); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
}
