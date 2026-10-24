package manager

import (
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Linked-device management (spec/service/sync.md §8).

// GRPCer makes gRPC unary calls on the chat connection.
type GRPCer interface {
	GRPC(ctx context.Context, method string, req []byte) ([]byte, error)
}

const devicesService = "/org.signal.chat.device.Devices/"

// Device is one entry of listDevices.
type Device struct {
	ID       uint32
	Name     *string // nil: no name
	Created  int64   // ms; 0 when it cannot be decrypted
	LastSeen int64
	This     bool
}

// ListDevices fetches the account's devices and updates the multi-device
// flag (§8.2). warn reports a creation time that could not be decrypted.
func ListDevices(ctx context.Context, a *store.Account, rpc GRPCer, warn func(string)) ([]Device, error) {
	resp, err := getDevices(ctx, a, rpc)
	if err != nil {
		return nil, err
	}
	priv, err := curve25519.NewPrivateKey(a.Data.ACIIdentity.Private)
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, d := range resp.GetDevices() {
		dev := Device{ID: d.GetId(), LastSeen: int64(d.GetLastSeen()), This: d.GetId() == uint32(a.Data.DeviceID)}
		if raw := d.GetName(); len(raw) > 0 {
			name, err := protocol.DecryptDeviceName(priv, raw)
			if err != nil {
				name = string(raw)
			}
			dev.Name = &name
		}
		created, err := decryptCreatedAt(a.Data.ACIIdentity.Private, d.GetId(), d.GetRegistrationId(), d.GetCreatedAtCiphertext())
		if err != nil && warn != nil {
			warn("Failed to decrypt device created timestamp: " + err.Error())
		}
		dev.Created = created
		out = append(out, dev)
	}
	return out, nil
}

// getDevices calls Devices/GetDevices and sets the multi-device flag from
// the count.
func getDevices(ctx context.Context, a *store.Account, rpc GRPCer) (*signalpb.GetDevicesResponse, error) {
	b, err := rpc.GRPC(ctx, devicesService+"GetDevices", nil)
	if err != nil {
		return nil, err
	}
	var resp signalpb.GetDevicesResponse
	if err := proto.Unmarshal(b, &resp); err != nil {
		return nil, err
	}
	if multi := len(resp.GetDevices()) > 1; multi != a.Data.MultiDevice {
		a.Data.MultiDevice = multi
		if err := a.Save(); err != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// decryptCreatedAt opens the HPKE-sealed creation time (§8.2): 0x01, the
// 32-byte encapsulated key, AES-256-GCM ciphertext; info
// "deviceCreatedAt", aad = device ID byte ‖ registration ID (4 bytes BE).
func decryptCreatedAt(aciPrivate []byte, id, regID uint32, ct []byte) (int64, error) {
	if len(ct) < 1+32+16 || ct[0] != 0x01 {
		return 0, errors.New("malformed ciphertext")
	}
	k, err := ecdh.X25519().NewPrivateKey(aciPrivate)
	if err != nil {
		return 0, err
	}
	hk, err := hpke.DHKEM(ecdh.X25519()).NewPrivateKey(k.Bytes())
	if err != nil {
		return 0, err
	}
	r, err := hpke.NewRecipient(ct[1:33], hk, hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("deviceCreatedAt"))
	if err != nil {
		return 0, err
	}
	aad := binary.BigEndian.AppendUint32([]byte{byte(id)}, regID)
	pt, err := r.Open(aad, ct[33:])
	if err != nil {
		return 0, err
	}
	if len(pt) != 8 {
		return 0, errors.New("bad plaintext length")
	}
	return int64(binary.BigEndian.Uint64(pt)), nil
}

// RemoveDevice removes another device and refreshes the multi-device
// flag (§8.3); primaries only.
func RemoveDevice(ctx context.Context, a *store.Account, rpc GRPCer, id uint32) error {
	req, _ := proto.Marshal(&signalpb.RemoveDeviceRequest{Id: id})
	if _, err := rpc.GRPC(ctx, devicesService+"RemoveDevice", req); err != nil {
		return err
	}
	_, err := getDevices(ctx, a, rpc)
	return err
}

// ClearPushToken is unregister's server call (§8.7).
func ClearPushToken(ctx context.Context, rpc GRPCer) error {
	_, err := rpc.GRPC(ctx, devicesService+"ClearPushToken", nil)
	return err
}

// Device-link errors (linking.md §6).
var (
	ErrNotPrimary     = errors.New("This command doesn't work on linked devices.")
	ErrTooManyDevices = errors.New("Account has too many linked devices already")
)

// AddDevice links a new device from its sgnl://linkdevice URL
// (linking.md §6): fetch a verification code, send the encrypted
// ProvisionMessage to the device's provisioning address, and turn the
// multi-device flag on.
func (s *Sender) AddDevice(ctx context.Context, link string) error {
	a := s.Acct
	if a.Data.DeviceID != 1 {
		return ErrNotPrimary
	}
	address, pub, err := provision.ParseLinkURL(link)
	if err != nil {
		return err
	}
	r, err := s.Chat.Request(ctx, "GET", "/v1/devices/provisioning/code", nil, nil)
	if err != nil {
		return err
	}
	switch r.Status {
	case 200:
	case 411:
		return ErrTooManyDevices
	default:
		return &SendStatusError{Status: r.Status}
	}
	var code struct {
		VerificationCode string `json:"verificationCode"`
	}
	if err := json.Unmarshal(r.Body, &code); err != nil {
		return err
	}
	if err := s.ensureKeys(); err != nil {
		return err
	}
	aci, _ := uuidRaw(a.Data.ACI)
	pni, _ := uuidRaw(a.Data.PNI)
	pm := &signalpb.ProvisionMessage{
		AciIdentityKeyPublic: a.Data.ACIIdentity.Public, AciIdentityKeyPrivate: a.Data.ACIIdentity.Private,
		PniIdentityKeyPublic: a.Data.PNIIdentity.Public, PniIdentityKeyPrivate: a.Data.PNIIdentity.Private,
		Aci: proto.String(a.Data.ACI), Pni: proto.String(a.Data.PNI), Number: proto.String(a.Data.Number),
		ProvisioningCode: proto.String(code.VerificationCode), ProfileKey: a.Data.ProfileKey, ProvisioningVersion: proto.Uint32(1),
		AccountEntropyPool: proto.String(a.Data.AccountEntropyPool), MediaRootBackupKey: a.Data.MediaRootBackupKey,
		AciBinary: aci[:], PniBinary: pni[:],
	}
	plain, err := proto.Marshal(pm)
	if err != nil {
		return err
	}
	ephemeral, body, err := protocol.EncryptProvision(s.rand(), pub, plain)
	if err != nil {
		return err
	}
	env, err := proto.Marshal(&signalpb.ProvisionEnvelope{PublicKey: ephemeral, Body: body})
	if err != nil {
		return err
	}
	reqBody, _ := json.Marshal(map[string]string{"body": base64.StdEncoding.EncodeToString(env)})
	r, err = s.Chat.Request(ctx, "PUT", "/v1/provisioning/"+url.QueryEscape(address), jsonHeader, reqBody)
	if err != nil {
		return err
	}
	switch r.Status {
	case 200, 204:
	case 411:
		return ErrTooManyDevices
	default:
		return &SendStatusError{Status: r.Status}
	}
	a.Data.MultiDevice = true
	return a.Save()
}

// UpdateDeviceName renames a device (sync.md §8.4). The local device's
// name is only stored (it reaches the server with the next account
// attributes); another device needs the primary and a request, then a
// deviceNameChange sync.
func (s *Sender) UpdateDeviceName(ctx context.Context, id uint32, name string) error {
	a := s.Acct
	pub, err := curve25519.ParsePublicKey(a.Data.ACIIdentity.Public)
	if err != nil {
		return err
	}
	if id != uint32(a.Data.DeviceID) && a.Data.DeviceID != 1 {
		return ErrNotPrimary
	}
	enc, err := protocol.EncryptDeviceName(s.rand(), pub, name)
	if err != nil {
		return err
	}
	b64 := base64.RawStdEncoding.EncodeToString(enc)
	if id == uint32(a.Data.DeviceID) {
		a.Data.DeviceName = b64
		return a.Save()
	}
	body, _ := json.Marshal(map[string]string{"deviceName": b64})
	r, err := s.Chat.Request(ctx, "PUT", "/v1/accounts/name?deviceId="+strconv.Itoa(int(id)), jsonHeader, body)
	if err != nil {
		return err
	}
	if r.Status != 200 && r.Status != 204 {
		return &SendStatusError{Status: r.Status}
	}
	return s.SendSync(ctx, &signalpb.AppSyncMessage{DeviceNameChange: &signalpb.AppSyncMessage_DeviceNameChange{DeviceId: proto.Uint32(id)}}, false)
}
