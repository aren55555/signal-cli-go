// Package api holds the chat service's JSON bodies and their encoding
// conventions (spec/service/api.md §3), shared by the REST and WebSocket
// transports.
package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/protocol"
)

// Marshal encodes like signal-service-java's Jackson mapper: members in
// struct order, no HTML escaping.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// KeyBase64 is standard base64 without padding, the form for key material.
func KeyBase64(b []byte) string { return base64.RawStdEncoding.EncodeToString(b) }

// DecodeBase64 decodes leniently: padded or unpadded, and the URL-safe
// alphabet if the input contains '-' or '_'.
func DecodeBase64(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	if strings.ContainsAny(s, "-_") {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// PreKeyEntity is a one-time EC prekey (keys.md §2.1).
type PreKeyEntity struct {
	KeyID     uint32 `json:"keyId"`
	PublicKey string `json:"publicKey"`
}

// SignedPreKeyEntity is a signed EC or Kyber prekey.
type SignedPreKeyEntity struct {
	KeyID     uint32 `json:"keyId"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}

// PreKey converts a one-time prekey.
func PreKey(r *protocol.PreKeyRecord) PreKeyEntity {
	return PreKeyEntity{KeyID: r.ID, PublicKey: KeyBase64(r.PublicKey())}
}

// SignedPreKey converts a signed EC prekey.
func SignedPreKey(r *protocol.SignedPreKeyRecord) *SignedPreKeyEntity {
	return &SignedPreKeyEntity{KeyID: r.ID, PublicKey: KeyBase64(r.PublicKey()), Signature: KeyBase64(r.Signature)}
}

// KyberPreKey converts a Kyber prekey.
func KyberPreKey(r *protocol.KyberPreKeyRecord) *SignedPreKeyEntity {
	return &SignedPreKeyEntity{KeyID: r.ID, PublicKey: KeyBase64(r.PublicKey()), Signature: KeyBase64(r.Signature)}
}

// Capabilities are the account capabilities a device declares.
type Capabilities struct {
	Storage                   bool `json:"storage"`
	VersionedExpirationTimer  bool `json:"versionedExpirationTimer"`
	AttachmentBackfill        bool `json:"attachmentBackfill"`
	SPQR                      bool `json:"spqr"`
	UsernameChangeSyncMessage bool `json:"usernameChangeSyncMessage"`
}

// LinkedDeviceCapabilities is signal-cli's set for a linked device
// (linking.md §7 step 6).
var LinkedDeviceCapabilities = Capabilities{Storage: true, VersionedExpirationTimer: true, AttachmentBackfill: true, SPQR: true, UsernameChangeSyncMessage: true}

// LinkAttributes is the accountAttributes object of a link request:
// exactly these five members.
type LinkAttributes struct {
	FetchesMessages   bool         `json:"fetchesMessages"`
	RegistrationID    uint32       `json:"registrationId"`
	PNIRegistrationID uint32       `json:"pniRegistrationId"`
	Name              string       `json:"name"`
	Capabilities      Capabilities `json:"capabilities"`
}

// LinkRequest is the body of PUT /v1/devices/link.
type LinkRequest struct {
	VerificationCode      string              `json:"verificationCode"`
	AccountAttributes     LinkAttributes      `json:"accountAttributes"`
	ACISignedPreKey       *SignedPreKeyEntity `json:"aciSignedPreKey"`
	PNISignedPreKey       *SignedPreKeyEntity `json:"pniSignedPreKey"`
	ACIPqLastResortPreKey *SignedPreKeyEntity `json:"aciPqLastResortPreKey"`
	PNIPqLastResortPreKey *SignedPreKeyEntity `json:"pniPqLastResortPreKey"`
	GCMToken              *string             `json:"gcmToken"` // always null
}

// LinkResponse is the answer to a link request.
type LinkResponse struct {
	UUID     string      `json:"uuid"`
	PNI      string      `json:"pni"`
	DeviceID json.Number `json:"deviceId"` // a number; tolerated as a string
}
