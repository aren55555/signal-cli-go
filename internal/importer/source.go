// SPDX-License-Identifier: GPL-3.0-or-later

// Package importer copies accounts from a signal-cli 0.14.7 data
// directory into signal-cli-go's store (spec/storage/signal-cli-data-dir.md,
// spec/storage/signal-cli-account-db.md). The source is only ever opened
// for reading: the account database is copied aside before SQLite sees it.
package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// indexEntry is one accounts.json entry (data-dir §2.1).
type indexEntry struct {
	Path        string  `json:"path"`
	Environment *string `json:"environment"`
	Number      *string `json:"number"`
	UUID        *string `json:"uuid"`
}

// readIndex reads <src>/data/accounts.json. Version 1 (or absent) is
// accepted only when every entry names its environment, which is what
// the version-2 upgrade would fill in (data-dir §2.4).
func readIndex(src string) ([]indexEntry, error) {
	b, err := os.ReadFile(filepath.Join(src, "data", "accounts.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s is not a signal-cli data directory (no data/accounts.json)", src)
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Version  *int         `json:"version"`
		Accounts []indexEntry `json:"accounts"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("accounts.json: %w", err)
	}
	v := 1
	if doc.Version != nil {
		v = *doc.Version
	}
	if v < 1 || v > 2 {
		return nil, fmt.Errorf("accounts.json has version %d; signal-cli 0.14.7 writes version 2", v)
	}
	return doc.Accounts, nil
}

// identityData is aciAccountData / pniAccountData (data-dir §3.3).
type identityData struct {
	ServiceID                     *string `json:"serviceId"`
	RegistrationID                uint32  `json:"registrationId"`
	IdentityPrivateKey            []byte  `json:"identityPrivateKey"`
	IdentityPublicKey             []byte  `json:"identityPublicKey"`
	NextPreKeyID                  uint32  `json:"nextPreKeyId"`
	NextSignedPreKeyID            uint32  `json:"nextSignedPreKeyId"`
	ActiveSignedPreKeyID          int64   `json:"activeSignedPreKeyId"`
	NextKyberPreKeyID             uint32  `json:"nextKyberPreKeyId"`
	ActiveLastResortKyberPreKeyID int64   `json:"activeLastResortKyberPreKeyId"`
}

// accountFile is the account file, version 10 (data-dir §3.2).
type accountFile struct {
	Version              *int         `json:"version"`
	ServiceEnvironment   string       `json:"serviceEnvironment"`
	Registered           bool         `json:"registered"`
	Number               *string      `json:"number"`
	Username             *string      `json:"username"`
	EncryptedDeviceName  *string      `json:"encryptedDeviceName"`
	DeviceID             int          `json:"deviceId"`
	IsMultiDevice        bool         `json:"isMultiDevice"`
	Password             string       `json:"password"`
	ACI                  identityData `json:"aciAccountData"`
	PNI                  identityData `json:"pniAccountData"`
	RegistrationLockPin  *string      `json:"registrationLockPin"`
	PinMasterKey         []byte       `json:"pinMasterKey"`
	StorageKey           []byte       `json:"storageKey"`
	AccountEntropyPool   *string      `json:"accountEntropyPool"`
	MediaRootBackupKey   []byte       `json:"mediaRootBackupKey"`
	ProfileKey           []byte       `json:"profileKey"`
	UsernameLinkEntropy  []byte       `json:"usernameLinkEntropy"`
	UsernameLinkServerID *string      `json:"usernameLinkServerId"`
}

// accountFileVersion is the only layout imported. Older files are
// migrated by signal-cli itself on its next start (data-dir §3.6).
const accountFileVersion = 10

func readAccountFile(path string) (*accountFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errNotRegistered
	}
	var f accountFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("account file: %w", err)
	}
	switch {
	case f.Version == nil || *f.Version < accountFileVersion:
		v := "no version"
		if f.Version != nil {
			v = fmt.Sprintf("version %d", *f.Version)
		}
		return nil, fmt.Errorf("account file has %s; run any account command once with signal-cli 0.14.7 to migrate it to version %d, then import again", v, accountFileVersion)
	case *f.Version > accountFileVersion:
		return nil, fmt.Errorf("account file has version %d, newer than signal-cli 0.14.7 writes (%d)", *f.Version, accountFileVersion)
	}
	if f.ServiceEnvironment != "LIVE" && f.ServiceEnvironment != "STAGING" {
		return nil, fmt.Errorf("account file has unknown service environment %q", f.ServiceEnvironment)
	}
	return &f, nil
}

var errNotRegistered = errors.New("account is not registered")

// inUse reports whether another process holds signal-cli's account lock:
// a POSIX write lock over the whole account file (data-dir §7). F_GETLK
// only asks; it takes nothing.
func inUse(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk); err != nil {
		return false, err
	}
	return lk.Type != syscall.F_UNLCK, nil
}

// bareUUID strips the "PNI:" prefix of a service ID string.
func bareUUID(s string) string { return strings.TrimPrefix(s, "PNI:") }
