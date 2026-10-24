package manager

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"google.golang.org/protobuf/proto"
)

type fakeGRPC struct {
	calls []string
	resp  map[string]proto.Message
}

func (f *fakeGRPC) GRPC(ctx context.Context, method string, req []byte) ([]byte, error) {
	f.calls = append(f.calls, method)
	if m, ok := f.resp[method]; ok {
		return proto.Marshal(m)
	}
	return nil, nil
}

// sealCreatedAt is the server side of §8.2's created_at_ciphertext.
func sealCreatedAt(t *testing.T, aciPublic []byte, id, regID uint32, ms int64) []byte {
	k, err := ecdh.X25519().NewPublicKey(aciPublic[1:])
	if err != nil {
		t.Fatal(err)
	}
	pk, err := hpke.DHKEM(ecdh.X25519()).NewPublicKey(k.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	enc, s, err := hpke.NewSender(pk, hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("deviceCreatedAt"))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := s.Seal(binary.BigEndian.AppendUint32([]byte{byte(id)}, regID), binary.BigEndian.AppendUint64(nil, uint64(ms)))
	if err != nil {
		t.Fatal(err)
	}
	return append(append([]byte{0x01}, enc...), ct...)
}

func TestListDevices(t *testing.T) {
	a, _ := testAccount(t) // device 2
	pub, _ := curve25519.ParsePublicKey(a.Data.ACIIdentity.Public)
	name, err := protocol.EncryptDeviceName(rand.Reader, pub, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGRPC{resp: map[string]proto.Message{devicesService + "GetDevices": &signalpb.GetDevicesResponse{Devices: []*signalpb.GetDevicesResponse_LinkedDevice{
		{Id: 1, LastSeen: 1700000000000, RegistrationId: 77, CreatedAtCiphertext: sealCreatedAt(t, a.Data.ACIIdentity.Public, 1, 77, 1690000000000)},
		{Id: 2, Name: name, LastSeen: 1700000000000, RegistrationId: 99, CreatedAtCiphertext: sealCreatedAt(t, a.Data.ACIIdentity.Public, 2, 98, 1)}, // wrong aad
		{Id: 3, Name: []byte("plain name")},
	}}}}
	a.Data.MultiDevice = false
	var warnings []string
	devs, err := ListDevices(context.Background(), a, f, func(s string) { warnings = append(warnings, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 3 || devs[0].Name != nil || devs[0].Created != 1690000000000 || devs[0].This {
		t.Errorf("device 1: %+v", devs[0])
	}
	if *devs[1].Name != "laptop" || devs[1].Created != 0 || !devs[1].This || *devs[2].Name != "plain name" {
		t.Errorf("devices %+v %+v", devs[1], devs[2])
	}
	if len(warnings) != 2 || !a.Data.MultiDevice {
		t.Errorf("warnings %v, multi %v", warnings, a.Data.MultiDevice)
	}

	// After removal only one device is left: single-device.
	f.resp[devicesService+"GetDevices"] = &signalpb.GetDevicesResponse{Devices: []*signalpb.GetDevicesResponse_LinkedDevice{{Id: 1}}}
	if err := RemoveDevice(context.Background(), a, f, 2); err != nil || a.Data.MultiDevice {
		t.Errorf("remove: %v multi %v", err, a.Data.MultiDevice)
	}
	if f.calls[len(f.calls)-2] != devicesService+"RemoveDevice" {
		t.Errorf("calls %v", f.calls)
	}
}

// provChat answers the verification-code request and records the PUT.
type provChat struct {
	path string
	body []byte
}

func (p *provChat) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	switch {
	case verb == "GET" && path == "/v1/devices/provisioning/code":
		return &chat.Response{Status: 200, Body: []byte(`{"verificationCode":"123456","tokenIdentifier":"t"}`)}, nil
	case verb == "PUT":
		p.path, p.body = path, body
		return &chat.Response{Status: 204}, nil
	}
	return &chat.Response{Status: 404}, nil
}

func TestAddDevice(t *testing.T) {
	a, _ := testAccount(t)
	a.Data.DeviceID = 1
	a.Data.AccountEntropyPool, a.Data.MediaRootBackupKey = "", nil
	a.Data.MultiDevice = false
	secondary, _ := curve25519.GeneratePrivateKey(rand.Reader)
	link := provision.LinkURL("addr-_x==", secondary.PublicKey())
	pc := &provChat{}
	s := &Sender{Acct: a, Chat: pc}
	if err := s.AddDevice(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	if pc.path != "/v1/provisioning/addr-_x%3D%3D" || !a.Data.MultiDevice || len(a.Data.AccountEntropyPool) != 64 {
		t.Fatalf("path %s multi %v", pc.path, a.Data.MultiDevice)
	}
	var req struct{ Body string }
	json.Unmarshal(pc.body, &req)
	envBytes, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	var env signalpb.ProvisionEnvelope
	proto.Unmarshal(envBytes, &env)
	plain, err := protocol.DecryptProvision(secondary, env.GetPublicKey(), env.GetBody())
	if err != nil {
		t.Fatal(err)
	}
	var pm signalpb.ProvisionMessage
	proto.Unmarshal(plain, &pm)
	aci, _ := uuidRaw(a.Data.ACI)
	if pm.GetProvisioningCode() != "123456" || pm.GetAci() != a.Data.ACI || pm.GetPni() != a.Data.PNI || pm.GetNumber() != a.Data.Number ||
		!bytes.Equal(pm.GetAciIdentityKeyPrivate(), a.Data.ACIIdentity.Private) || !bytes.Equal(pm.GetAciBinary(), aci[:]) ||
		pm.GetProvisioningVersion() != 1 || pm.GetAccountEntropyPool() != a.Data.AccountEntropyPool || pm.UserAgent != nil || pm.ReadReceipts != nil {
		t.Errorf("provision message %v", &pm)
	}

	a.Data.DeviceID = 2
	if err := s.AddDevice(context.Background(), link); !errors.Is(err, ErrNotPrimary) {
		t.Errorf("linked device: %v", err)
	}
}
