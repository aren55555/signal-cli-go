// SPDX-License-Identifier: GPL-3.0-or-later

// Package manager runs signal-cli's account operations on top of the
// protocol, service and store layers.
package manager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// ErrUserExists reports a link for a number that already has a local
// account.
var ErrUserExists = errors.New("user already exists")

// idSpace is the prekey ID range [0, 0xFFFFFF) (keys.md §1).
const idSpace = 0xFFFFFF

// Linker links this client as a secondary device (linking.md §7).
type Linker struct {
	Root         *store.Root
	REST         *rest.Client
	OpenSocket   func(context.Context) (*provision.Session, error)
	OnURL        func(string) // shows the sgnl://linkdevice URL
	DeviceName   string       // default "cli"
	SkipPostLink bool         // tests: no prekey refresh after linking
	PostLink     func(context.Context, *store.Account) error
	Now          func() time.Time
	Rand         io.Reader
}

func (l *Linker) rand() io.Reader {
	if l.Rand != nil {
		return l.Rand
	}
	return rand.Reader
}

func (l *Linker) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func randUint32(r io.Reader, n uint32) (uint32, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return uint32(binary.BigEndian.Uint64(b[:]) % uint64(n)), nil
}

// newPassword is 18 random bytes in padded standard base64 (api.md §2.1).
func newPassword(r io.Reader) (string, error) {
	b := make([]byte, 18)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// uuidFrom takes a UUID from its string field or, failing that, its
// 16-byte binary field (linking.md §7 step 4).
func uuidFrom(s string, bin []byte) (string, error) {
	if s != "" {
		raw, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
		if err != nil || len(raw) != 16 || len(s) != 36 {
			return "", fmt.Errorf("invalid UUID %q", s)
		}
		return strings.ToLower(s), nil
	}
	if len(bin) != 16 {
		return "", errors.New("missing UUID")
	}
	h := hex.EncodeToString(bin)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func keyPair(pub, priv []byte) (store.KeyPair, *curve25519.PrivateKey, error) {
	k, err := curve25519.NewPrivateKey(priv)
	if err != nil {
		return store.KeyPair{}, nil, err
	}
	if !bytes.Equal(k.PublicKey().Bytes(), pub) {
		return store.KeyPair{}, nil, errors.New("public key does not match private key")
	}
	return store.KeyPair{Public: pub, Private: priv}, k, nil
}

// Link runs a whole link: provisioning socket, URL, provisioning message,
// device registration, local account.
func (l *Linker) Link(ctx context.Context) (*store.Account, error) {
	p, url, err := l.Start(ctx)
	if err != nil {
		return nil, err
	}
	if l.OnURL != nil {
		l.OnURL(url)
	}
	return p.Finish(ctx, l.DeviceName)
}

// PendingLink is a link whose URL was shown and whose provisioning
// message has not arrived yet (JSON-RPC startLink, then finishLink).
type PendingLink struct {
	l        *Linker
	sess     *provision.Session
	temp     *curve25519.PrivateKey
	password string
}

// Start opens the provisioning socket and returns the link URL.
func (l *Linker) Start(ctx context.Context) (*PendingLink, string, error) {
	r := l.rand()
	temp, err := curve25519.GeneratePrivateKey(r)
	if err != nil {
		return nil, "", err
	}
	password, err := newPassword(r)
	if err != nil {
		return nil, "", err
	}
	sess, err := l.OpenSocket(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("opening the provisioning socket: %w", err)
	}
	addr, err := sess.Address(ctx)
	if err != nil {
		sess.Close()
		return nil, "", err
	}
	return &PendingLink{l: l, sess: sess, temp: temp, password: password}, provision.LinkURL(addr, temp.PublicKey()), nil
}

// Close abandons the link.
func (p *PendingLink) Close() { p.sess.Close() }

// Finish waits for the provisioning message and completes the link with
// the device name name ("" means "cli").
func (p *PendingLink) Finish(ctx context.Context, name string) (*store.Account, error) {
	l, sess, temp, password := p.l, p.sess, p.temp, p.password
	defer sess.Close()
	if name != "" {
		l.DeviceName = name
	}
	env, err := sess.Envelope(ctx)
	if err != nil {
		return nil, err
	}
	sess.Close()
	plain, err := protocol.DecryptProvision(temp, env.GetPublicKey(), env.GetBody())
	if err != nil {
		return nil, err
	}
	var pm signalpb.ProvisionMessage
	if err := proto.Unmarshal(plain, &pm); err != nil {
		return nil, fmt.Errorf("%w: %v", protocol.ErrProvisionDecrypt, err)
	}
	return l.finish(ctx, &pm, password)
}

func (l *Linker) finish(ctx context.Context, pm *signalpb.ProvisionMessage, password string) (*store.Account, error) {
	r := l.rand()
	number := pm.GetNumber()
	if number == "" {
		return nil, errors.New("provisioning message without a number")
	}
	aci, err := uuidFrom(pm.GetAci(), pm.GetAciBinary())
	if err != nil {
		return nil, fmt.Errorf("ACI: %w", err)
	}
	pni, err := uuidFrom(strings.TrimPrefix(pm.GetPni(), "PNI:"), pm.GetPniBinary())
	if err != nil {
		return nil, fmt.Errorf("PNI: %w", err)
	}
	aciPair, aciKey, err := keyPair(pm.GetAciIdentityKeyPublic(), pm.GetAciIdentityKeyPrivate())
	if err != nil {
		return nil, fmt.Errorf("invalid key material: %w", err)
	}
	pniPair, pniKey, err := keyPair(pm.GetPniIdentityKeyPublic(), pm.GetPniIdentityKeyPrivate())
	if err != nil {
		return nil, fmt.Errorf("invalid key material: %w", err)
	}
	if a, err := l.Root.Find(number); err == nil {
		a.Close()
		return nil, ErrUserExists
	} else if !errors.Is(err, store.ErrNoAccount) {
		return nil, err
	}

	profileKey := pm.GetProfileKey()
	if len(profileKey) != 32 {
		profileKey = make([]byte, 32)
		if _, err := io.ReadFull(r, profileKey); err != nil {
			return nil, err
		}
	}
	name := l.DeviceName
	if name == "" {
		name = "cli"
	}
	encName, err := protocol.EncryptDeviceName(r, aciKey.PublicKey(), name)
	if err != nil {
		return nil, err
	}

	d := store.AccountData{
		Environment: store.EnvStaging, Number: number, ACI: aci, PNI: pni, Password: password,
		ACIIdentity: aciPair, PNIIdentity: pniPair, ProfileKey: profileKey,
		AccountEntropyPool: pm.GetAccountEntropyPool(), MediaRootBackupKey: pm.GetMediaRootBackupKey(),
	}
	for _, id := range []*uint32{&d.ACIRegistrationID, &d.PNIRegistrationID} {
		v, err := randUint32(r, 16380)
		if err != nil {
			return nil, err
		}
		*id = v + 1 // 1..16380 (registration.md §1)
	}
	var keys store.KeysData
	now := l.now()
	var signed [2]*protocol.SignedPreKeyRecord
	var kyber [2]*protocol.KyberPreKeyRecord
	for i, side := range []struct {
		state *store.PreKeyState
		keys  *store.IdentityKeys
		id    *curve25519.PrivateKey
	}{{&d.ACIKeys, &keys.ACI, aciKey}, {&d.PNIKeys, &keys.PNI, pniKey}} {
		s := side.state
		for _, c := range []*uint32{&s.NextPreKeyID, &s.NextSignedPreKeyID, &s.NextKyberPreKeyID} {
			if *c, err = randUint32(r, idSpace); err != nil {
				return nil, err
			}
		}
		if signed[i], err = protocol.GenerateSignedPreKey(r, side.id, s.NextSignedPreKeyID, now); err != nil {
			return nil, err
		}
		if kyber[i], err = protocol.GenerateKyberPreKey(r, side.id, s.NextKyberPreKeyID, now); err != nil {
			return nil, err
		}
		side.keys.SignedPreKeys = map[uint32]store.StoredKey{signed[i].ID: {Record: signed[i].Serialize()}}
		side.keys.KyberPreKeys = map[uint32]store.StoredKey{kyber[i].ID: {Record: kyber[i].Serialize(), LastResort: true}}
		s.ActiveSignedPreKeyID, s.ActiveLastResortKyberID = int64(signed[i].ID), int64(kyber[i].ID)
		s.NextSignedPreKeyID = (signed[i].ID + 1) % idSpace
		s.NextKyberPreKeyID = (kyber[i].ID + 1) % idSpace
	}

	req := api.LinkRequest{
		VerificationCode: pm.GetProvisioningCode(),
		AccountAttributes: api.LinkAttributes{
			FetchesMessages: true, RegistrationID: d.ACIRegistrationID, PNIRegistrationID: d.PNIRegistrationID,
			Name: base64.RawStdEncoding.EncodeToString(encName), Capabilities: api.LinkedDeviceCapabilities,
		},
		ACISignedPreKey: api.SignedPreKey(signed[0]), PNISignedPreKey: api.SignedPreKey(signed[1]),
		ACIPqLastResortPreKey: api.KyberPreKey(kyber[0]), PNIPqLastResortPreKey: api.KyberPreKey(kyber[1]),
	}
	var resp api.LinkResponse
	if err := l.REST.Do(ctx, "PUT", "/v1/devices/link", &rest.Basic{User: number, Password: password}, req, &resp); err != nil {
		return nil, fmt.Errorf("registering the device: %w", err)
	}
	devID, err := resp.DeviceID.Int64()
	if err != nil || devID < 2 {
		return nil, fmt.Errorf("server assigned device ID %q", resp.DeviceID)
	}
	d.DeviceID = int(devID)
	d.DeviceName = req.AccountAttributes.Name
	d.Registered = true

	acct, err := l.Root.Create(number, store.EnvStaging)
	if err != nil {
		return nil, err
	}
	acct.Data, acct.Keys = d, keys
	acct.EnsureSelf() // the self recipient is the first row (contacts.md §4)
	if err := acct.SaveRecipients(); err != nil {
		l.Root.Delete(acct)
		return nil, err
	}
	if err := acct.Save(); err != nil {
		l.Root.Delete(acct)
		return nil, err
	}
	if err := acct.SaveKeys(); err != nil {
		l.Root.Delete(acct)
		return nil, err
	}
	if !l.SkipPostLink && l.PostLink != nil {
		if err := l.PostLink(ctx, acct); err != nil {
			return acct, fmt.Errorf("linked, but the follow-up failed: %w", err)
		}
	}
	return acct, nil
}
