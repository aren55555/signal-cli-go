// SPDX-License-Identifier: GPL-3.0-or-later

// Package store is signal-cli-go's own on-disk storage: JSON documents
// written crash-safely (temp file, fsync, rename, directory fsync), each
// carrying a version and migrated forward on load, under a per-account
// advisory lock. Reading signal-cli's own data directory is a separate,
// later import tool (PROMPT.md §1.4).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Errors.
var (
	ErrNewerVersion = errors.New("store: written by a newer signal-cli-go")
	ErrLocked       = errors.New("store: account is in use by another process")
)

// WriteFileAtomic replaces path with data so that a crash leaves either
// the old or the new content, never a mix.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Migrations maps a target version n to the step that turns a version n-1
// document (decoded generically) into version n.
type Migrations map[int]func(map[string]any) error

// ReadJSON decodes the document at path into v, first migrating it from
// its stored version up to current.
func ReadJSON(path string, current int, migrations Migrations, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var hdr struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &hdr); err != nil {
		return fmt.Errorf("store: %s: %w", path, err)
	}
	switch {
	case hdr.Version > current:
		return fmt.Errorf("%w: %s has version %d, this build reads up to %d", ErrNewerVersion, path, hdr.Version, current)
	case hdr.Version < current:
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("store: %s: %w", path, err)
		}
		for n := hdr.Version + 1; n <= current; n++ {
			step := migrations[n]
			if step == nil {
				return fmt.Errorf("store: %s: no migration to version %d", path, n)
			}
			if err := step(m); err != nil {
				return fmt.Errorf("store: %s: migrating to version %d: %w", path, n, err)
			}
			m["version"] = n
		}
		if b, err = json.Marshal(m); err != nil {
			return err
		}
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("store: %s: %w", path, err)
	}
	return nil
}

// WriteJSON writes v atomically with owner-only permissions.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(b, '\n'), 0o600)
}

// FileLock is an exclusive advisory lock on a directory.
type FileLock struct{ f *os.File }

// Lock takes the lock file in dir without waiting.
func Lock(dir string) (*FileLock, error) {
	f, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return &FileLock{f}, nil
}

// Unlock releases the lock.
func (l *FileLock) Unlock() error { return l.f.Close() }
