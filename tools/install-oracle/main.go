// SPDX-License-Identifier: GPL-3.0-or-later

// Command install-oracle downloads the reference signal-cli release into
// third_party/ and verifies it before anything runs it.
//
// The reference binary is a test oracle for the differential tests
// (Layer B and C). It is never a runtime dependency of signal-cli-go.
//
// Verification, in order:
//  1. SHA-256 of the tarball against the pin below.
//  2. OpenPGP signature (.asc) against the signing key committed in
//     keys/, checked with gpg in a throwaway keyring. The signer's
//     fingerprint must equal signerFingerprint.
//  3. Archive extraction rejects absolute paths, ".." components, and
//     links that point outside the destination.
//
// Usage (from the repo root, inside the sandbox):
//
//	go run ./tools/install-oracle
package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	version     = "0.14.7"
	releaseBase = "https://github.com/AsamK/signal-cli/releases/download/v" + version + "/"
	tarball     = "signal-cli-" + version + ".tar.gz"

	// Source: GitHub release API asset digest for v0.14.7 (published
	// 2026-08-01), confirmed by downloading the asset.
	tarballSHA256 = "0e1eefdf4a2109edf7c899c9d1667167c54ac12c3ec824f27db7c1dac4fa7506"

	// AsamK <asamk@gmx.de>, rsa2048, created 2016-06-18. Key fetched from
	// https://github.com/AsamK.gpg (the release author's GitHub account).
	// signal-cli's README does not publish a fingerprint, so the GitHub
	// account is the trust anchor.
	signerFingerprint = "FA10826A74907F9EC6BBB7FC2BA2CD21B5B09570"
	signerKeyFile     = "tools/install-oracle/keys/asamk-" + signerFingerprint + ".asc"

	// signal-cli 0.14.7 requires Java 25; the sandbox image provides it.
	wantJavaMajor = "25"
)

func main() {
	root := flag.String("root", ".", "repo root")
	force := flag.Bool("force", false, "reinstall even if already present")
	flag.Parse()

	if err := run(*root, *force); err != nil {
		fmt.Fprintln(os.Stderr, "install-oracle:", err)
		os.Exit(1)
	}
}

func run(root string, force bool) error {
	if os.Getenv("SIGNAL_CLI_GO_SANDBOX") != "1" {
		return errors.New("refusing to run outside the sandbox (SIGNAL_CLI_GO_SANDBOX != 1)")
	}

	thirdParty := filepath.Join(root, "third_party")
	dest := filepath.Join(thirdParty, "signal-cli-"+version)
	bin := filepath.Join(dest, "bin", "signal-cli")
	if _, err := os.Stat(bin); err == nil && !force {
		fmt.Println("already installed:", bin)
		return checkJava()
	}

	// Downloads go in their own directory, separate from the extracted tree.
	dlDir := filepath.Join(thirdParty, "downloads")
	if err := os.MkdirAll(dlDir, 0o755); err != nil {
		return err
	}
	tarPath := filepath.Join(dlDir, tarball)
	ascPath := tarPath + ".asc"

	if err := download(releaseBase+tarball, tarPath); err != nil {
		return err
	}
	if err := download(releaseBase+tarball+".asc", ascPath); err != nil {
		return err
	}

	sum, err := fileSHA256(tarPath)
	if err != nil {
		return err
	}
	if sum != tarballSHA256 {
		return fmt.Errorf("%s: sha256 %s, want %s", tarball, sum, tarballSHA256)
	}
	fmt.Println("sha256 ok:", tarball)

	if err := verifySignature(filepath.Join(root, signerKeyFile), ascPath, tarPath); err != nil {
		return err
	}
	fmt.Println("signature ok: signed by", signerFingerprint)

	staging := dest + ".partial"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := extract(tarPath, staging); err != nil {
		return err
	}
	// The archive has a single top-level directory, signal-cli-<version>/.
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(staging, "signal-cli-"+version), dest); err != nil {
		return fmt.Errorf("unexpected archive layout: %w", err)
	}
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	fmt.Println("installed:", bin)
	return checkJava()
}

func download(url, path string) error {
	client := &http.Client{Timeout: 10 * time.Minute} // honors HTTPS_PROXY
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
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

// verifySignature checks a detached signature with gpg, using a temporary
// keyring that holds only the pinned key, and requires a VALIDSIG status
// line naming the pinned fingerprint.
func verifySignature(keyPath, sigPath, dataPath string) error {
	home, err := os.MkdirTemp(filepath.Dir(sigPath), "gnupg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	if err := os.Chmod(home, 0o700); err != nil {
		return err
	}

	gpg := func(args ...string) *exec.Cmd {
		return exec.Command("gpg", append([]string{"--homedir", home, "--batch", "--no-tty"}, args...)...)
	}
	if out, err := gpg("--import", keyPath).CombinedOutput(); err != nil {
		return fmt.Errorf("gpg import: %v: %s", err, out)
	}

	cmd := gpg("--status-fd", "1", "--verify", sigPath, dataPath)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gpg verify failed: %v", err)
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// [GNUPG:] VALIDSIG <sig-key-fpr> ... <primary-key-fpr>
		if len(f) >= 3 && f[1] == "VALIDSIG" && strings.EqualFold(f[len(f)-1], signerFingerprint) {
			return nil
		}
	}
	return fmt.Errorf("no valid signature from %s", signerFingerprint)
}

func extract(tarPath, dest string) error {
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
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	inside := func(p string) bool {
		rel, err := filepath.Rel(absDest, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if filepath.IsAbs(hdr.Name) {
			return fmt.Errorf("archive entry has absolute path: %q", hdr.Name)
		}
		target := filepath.Join(absDest, hdr.Name)
		if !inside(target) {
			return fmt.Errorf("archive entry escapes destination: %q", hdr.Name)
		}
		mode := os.FileMode(hdr.Mode).Perm() &^ 0o022
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			resolved := filepath.Join(filepath.Dir(target), hdr.Linkname)
			if filepath.IsAbs(hdr.Linkname) || !inside(resolved) {
				return fmt.Errorf("symlink escapes destination: %q -> %q", hdr.Name, hdr.Linkname)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type %q for %q", hdr.Typeflag, hdr.Name)
		}
	}
}

func checkJava() error {
	out, err := exec.Command("java", "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("java not runnable (the sandbox image provides JDK %s): %v", wantJavaMajor, err)
	}
	if !strings.Contains(string(out), `version "`+wantJavaMajor) {
		return fmt.Errorf("signal-cli %s needs Java %s, found: %s", version, wantJavaMajor, strings.SplitN(string(out), "\n", 2)[0])
	}
	return nil
}
