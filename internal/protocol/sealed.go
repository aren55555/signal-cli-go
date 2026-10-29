// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/gcmsiv"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// Sealed sender (spec/protocol/sealed-sender.md).

var (
	// ErrSealedSender covers every unsealing and certificate failure; like
	// signal-service-java's InvalidMetadataMessageException, it says
	// nothing about the sender.
	ErrSealedSender = errors.New("protocol: invalid sealed sender message")
	// ErrUnknownServerCertificate reports a sender certificate that names
	// a server certificate ID this build doesn't know.
	ErrUnknownServerCertificate = errors.New("protocol: unknown sealed sender server certificate id")
)

// Environment selects signal-cli's trust roots.
type Environment int

const (
	Production Environment = iota
	Staging
)

func mustKey(h string) *curve25519.PublicKey {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	k, err := curve25519.ParsePublicKey(b)
	if err != nil {
		panic(err)
	}
	return k
}

// The two trust roots signal-cli 0.14.7 configures per environment (§4.3).
var trustRoots = map[Environment][]*curve25519.PublicKey{
	Production: {
		mustKey("057bba408295cf9300f20b2dcdf3a04501aac8ba8ec0d2872fa20d92fdc81d6305"),
		mustKey("054918d08fbdfa83e00c29f8f8073a22ef35df2bea903aff81af03ccbc45c6e93a"),
	},
	Staging: {
		mustKey("05ba98d43ce8844e0d519a1517e2f5f2850facade420b9652c4261d949cf4ac131"),
		mustKey("058854ead3e3a8fe3a286644cd1b3538be14dfd5797653c9fd7d3d85ae2b12b926"),
	},
}

// TrustRoots returns the sealed-sender trust roots for env.
func TrustRoots(env Environment) []*curve25519.PublicKey { return trustRoots[env] }

// Built-in server certificates a sender certificate may name by ID
// (§4.2): staging (2), production (3) and libsignal's test certificate.
var knownServerCerts = map[uint32]string{
	2:          "0a25080212210539450d63ebd0752c0fd4038b9d07a916f5e174b756d409b5ca79f4c97400631e124064c5a38b1e927497d3d4786b101a623ab34a7da3954fae126b04dba9d7a3604ed88cdc8550950f0d4a9134ceb7e19b94139151d2c3d6e1c81e9d1128aafca806",
	3:          "0a250803122105bc9d1d290be964810dfa7e94856480a3f7060d004c9762c24c575a1522353a5a1240c11ec3c401eb0107ab38f8600e8720a63169e0e2eb8a3fae24f63099f85ea319c3c1c46d3454706ae2a679d1fee690a488adda98a2290b66c906bb60295ed781",
	0x7357c357: "0a2908d786df9a07122105847c0d2c375234f365e660955187a3735a0f7613d1609d3a6a4d8c53aeaa5a221240e0b9ebacdfc3aa2827f7924b697784d1c25e44ca05dd433e1a38dc6382eb2730d419ca9a250b1be9d5a9463e61efd6781777a91b83c97b844d014206e2829785",
}

// revokedServerKeyIDs is libsignal v0.99.1's revocation list (a test ID).
var revokedServerKeyIDs = map[uint32]bool{0xdeadc357: true}

// ServerCertificate is a server key signed by a trust root.
type ServerCertificate struct {
	KeyID uint32
	Key   *curve25519.PublicKey
	body  []byte
	sig   []byte
	raw   []byte
}

func knownServerCert(id uint32) (*ServerCertificate, error) {
	h, ok := knownServerCerts[id]
	if !ok {
		return nil, fmt.Errorf("%w %#x", ErrUnknownServerCertificate, id)
	}
	b, _ := hex.DecodeString(h)
	return ParseServerCertificate(b)
}

// ParseServerCertificate parses a serialized ServerCertificate.
func ParseServerCertificate(b []byte) (*ServerCertificate, error) {
	var pb signalpb.ServerCert
	if err := proto.Unmarshal(b, &pb); err != nil || pb.Body == nil || pb.Signature == nil {
		return nil, fmt.Errorf("%w: server certificate", ErrInvalidMessage)
	}
	var body signalpb.ServerCert_Body
	if err := proto.Unmarshal(pb.Body, &body); err != nil || body.KeyId == nil || body.PublicKey == nil {
		return nil, fmt.Errorf("%w: server certificate body", ErrInvalidMessage)
	}
	key, err := parseKey(body.PublicKey, "server key")
	if err != nil {
		return nil, err
	}
	return &ServerCertificate{KeyID: body.GetKeyId(), Key: key, body: pb.Body, sig: pb.Signature, raw: b}, nil
}

// validFor reports whether some trust root signed the certificate. Every
// root is checked, without stopping early (§4.1 step 2).
func (c *ServerCertificate) validFor(roots []*curve25519.PublicKey) bool {
	if revokedServerKeyIDs[c.KeyID] {
		return false
	}
	ok := 0
	for _, r := range roots {
		if r.Verify(c.body, c.sig) {
			ok |= 1
		}
	}
	return ok == 1
}

// SenderCertificate identifies the sender of a sealed message.
type SenderCertificate struct {
	SenderUUID  string // lowercase hyphenated ACI
	SenderE164  string // empty if absent
	DeviceID    uint32
	Expires     uint64 // milliseconds since the Unix epoch
	IdentityKey *curve25519.PublicKey
	Signer      *ServerCertificate
	body, sig   []byte
}

func formatUUID(b []byte) string {
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// ParseSenderCertificate parses a serialized SenderCertificate, resolving
// a signer given by ID from the built-in table.
func ParseSenderCertificate(b []byte) (*SenderCertificate, error) {
	var pb signalpb.SenderCert
	if err := proto.Unmarshal(b, &pb); err != nil || pb.Body == nil || pb.Signature == nil {
		return nil, fmt.Errorf("%w: sender certificate", ErrInvalidMessage)
	}
	var body signalpb.SenderCert_Body
	if err := proto.Unmarshal(pb.Body, &body); err != nil {
		return nil, fmt.Errorf("%w: sender certificate body: %v", ErrInvalidMessage, err)
	}
	if body.DeviceId == nil || body.GetDeviceId() < 1 || body.GetDeviceId() > 127 || body.Expires == nil || body.IdentityKey == nil {
		return nil, fmt.Errorf("%w: sender certificate fields", ErrInvalidMessage)
	}
	c := &SenderCertificate{
		SenderE164: body.GetPhoneNumber(),
		DeviceID:   body.GetDeviceId(),
		Expires:    body.GetExpires(),
		body:       pb.Body,
		sig:        pb.Signature,
	}
	var err error
	if c.IdentityKey, err = parseKey(body.IdentityKey, "sender identity"); err != nil {
		return nil, err
	}
	switch s := body.Signer.(type) {
	case *signalpb.SenderCert_Body_ServerCert:
		c.Signer, err = ParseServerCertificate(s.ServerCert)
	case *signalpb.SenderCert_Body_KnownServerCertId:
		c.Signer, err = knownServerCert(s.KnownServerCertId)
	default:
		err = fmt.Errorf("%w: sender certificate has no signer", ErrInvalidMessage)
	}
	if err != nil {
		return nil, err
	}
	switch u := body.SenderAci.(type) {
	case *signalpb.SenderCert_Body_AciString:
		c.SenderUUID = u.AciString
	case *signalpb.SenderCert_Body_AciBytes:
		if len(u.AciBytes) != 16 {
			return nil, fmt.Errorf("%w: sender UUID is %d bytes", ErrInvalidMessage, len(u.AciBytes))
		}
		c.SenderUUID = formatUUID(u.AciBytes)
	default:
		return nil, fmt.Errorf("%w: sender certificate has no sender", ErrInvalidMessage)
	}
	return c, nil
}

// Validate applies §4.1: the signer is valid for some root, it signed this
// certificate, and now (milliseconds) is not after the expiry.
func (c *SenderCertificate) Validate(roots []*curve25519.PublicKey, now uint64) bool {
	return c.Signer.validFor(roots) && c.Signer.Key.Verify(c.body, c.sig) && now <= c.Expires
}

// Sealed content kinds (SealedContent.kind) and content hints.
const (
	SealedContentPreKey    = 1
	SealedContentMessage   = 2
	SealedContentSenderKey = 7
	SealedContentPlaintext = 8

	ContentHintDefault    = 0
	ContentHintResendable = 1
	ContentHintImplicit   = 2
)

// SealedContent is the unsealed inner message (the USMC, §3.4).
type SealedContent struct {
	Kind        int
	Sender      *SenderCertificate
	Content     []byte
	ContentHint uint32
	GroupID     []byte
}

// EncodeSealedContent serializes a USMC. A zero hint and an empty group ID
// are omitted, as libsignal does.
func EncodeSealedContent(kind int, senderCert, content []byte, hint uint32, groupID []byte) []byte {
	k := signalpb.SealedContent_Kind(kind)
	pb := &signalpb.SealedContent{Kind: &k, SenderCert: senderCert, Content: content}
	if hint != 0 {
		pb.ContentHint = proto.Uint32(hint)
	}
	if len(groupID) > 0 {
		pb.GroupId = groupID
	}
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return b
}

func parseSealedContent(b []byte) (*SealedContent, error) {
	var pb signalpb.SealedContent
	// An unknown kind lands in the unknown fields, leaving Kind unset.
	if err := proto.Unmarshal(b, &pb); err != nil || pb.Kind == nil || pb.SenderCert == nil || pb.Content == nil {
		return nil, fmt.Errorf("%w: sealed content", ErrInvalidMessage)
	}
	cert, err := ParseSenderCertificate(pb.SenderCert)
	if err != nil {
		return nil, err
	}
	return &SealedContent{
		Kind:        int(pb.GetKind()),
		Sender:      cert,
		Content:     pb.Content,
		ContentHint: pb.GetContentHint(),
		GroupID:     pb.GroupId,
	}, nil
}

// aesCTR is AES-256-CTR from an all-zero counter block (§5.1).
func aesCTR(key, in []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	out := make([]byte, len(in))
	cipher.NewCTR(block, make([]byte, aes.BlockSize)).XORKeyStream(out, in)
	return out
}

const sealedMACSize = 10

func mac10(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)[:sealedMACSize]
}

func hkdf96(ikm, salt []byte) []byte {
	out, err := hkdf.Key(sha256.New, ikm, salt, "", 96)
	if err != nil {
		panic(err)
	}
	return out
}

// ctrMAC encrypts and appends the truncated MAC.
func ctrMAC(cipherKey, macKey, plaintext []byte) []byte {
	ct := aesCTR(cipherKey, plaintext)
	return append(ct, mac10(macKey, ct)...)
}

// openCtrMAC checks the MAC (constant time) and decrypts.
func openCtrMAC(cipherKey, macKey, in []byte) ([]byte, bool) {
	if len(in) < sealedMACSize {
		return nil, false
	}
	ct, mac := in[:len(in)-sealedMACSize], in[len(in)-sealedMACSize:]
	if subtle.ConstantTimeCompare(mac10(macKey, ct), mac) != 1 {
		return nil, false
	}
	return aesCTR(cipherKey, ct), true
}

const unidentifiedDelivery = "UnidentifiedDelivery"

// v1EphemeralKeys is §5.2 step 2: e_chain, e_cipher, e_mac.
func v1EphemeralKeys(dh []byte, recipient, ephemeral *curve25519.PublicKey) (chain, cipherKey, macKey []byte) {
	salt := append([]byte(unidentifiedDelivery), recipient.Bytes()...)
	salt = append(salt, ephemeral.Bytes()...)
	out := hkdf96(dh, salt)
	return out[:32], out[32:64], out[64:]
}

// v1StaticKeys is §5.2 step 4: the first 32 bytes are discarded.
func v1StaticKeys(dh, eChain, encStatic []byte) (cipherKey, macKey []byte) {
	out := hkdf96(dh, append(append([]byte(nil), eChain...), encStatic...))
	return out[32:64], out[64:]
}

// SealV1 seals a serialized USMC for one recipient identity (§5.2).
func SealV1(rand io.Reader, sender *curve25519.PrivateKey, recipient *curve25519.PublicKey, usmc []byte) ([]byte, error) {
	e, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	dh, err := e.Agree(recipient)
	if err != nil {
		return nil, err
	}
	eChain, eCipher, eMAC := v1EphemeralKeys(dh, recipient, e.PublicKey())
	encStatic := ctrMAC(eCipher, eMAC, sender.PublicKey().Bytes())
	sdh, err := sender.Agree(recipient)
	if err != nil {
		return nil, err
	}
	sCipher, sMAC := v1StaticKeys(sdh, eChain, encStatic)
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(&signalpb.SealedEnvelope{
		EphemeralPublic: e.PublicKey().Bytes(),
		EncryptedStatic: encStatic,
		EncryptedBody:   ctrMAC(sCipher, sMAC, usmc),
	})
	if err != nil {
		return nil, err
	}
	return append([]byte{0x11}, body...), nil
}

func unsealV1(recipient *curve25519.PrivateKey, b []byte) (*SealedContent, error) {
	var pb signalpb.SealedEnvelope
	if err := proto.Unmarshal(b, &pb); err != nil || pb.EphemeralPublic == nil || pb.EncryptedStatic == nil || pb.EncryptedBody == nil {
		return nil, errors.New("malformed v1 envelope")
	}
	eph, err := curve25519.ParsePublicKey(pb.EphemeralPublic)
	if err != nil {
		return nil, err
	}
	dh, err := recipient.Agree(eph)
	if err != nil {
		return nil, err
	}
	eChain, eCipher, eMAC := v1EphemeralKeys(dh, recipient.PublicKey(), eph)
	staticBytes, ok := openCtrMAC(eCipher, eMAC, pb.EncryptedStatic)
	if !ok {
		return nil, errors.New("failed to decrypt sealed sender v1 message key")
	}
	senderKey, err := curve25519.ParsePublicKey(staticBytes)
	if err != nil {
		return nil, err
	}
	sdh, err := recipient.Agree(senderKey)
	if err != nil {
		return nil, err
	}
	sCipher, sMAC := v1StaticKeys(sdh, eChain, pb.EncryptedStatic)
	usmc, ok := openCtrMAC(sCipher, sMAC, pb.EncryptedBody)
	if !ok {
		return nil, errors.New("failed to decrypt sealed sender v1 message contents")
	}
	content, err := parseSealedContent(usmc)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(staticBytes, content.Sender.IdentityKey.Bytes()) != 1 {
		return nil, errors.New("sender certificate key does not match message key")
	}
	return content, nil
}

// v2 labels (§6.1).
const (
	labelR        = "Sealed Sender v2: r (2023-08)"
	labelK        = "Sealed Sender v2: K"
	labelDH       = "Sealed Sender v2: DH"
	labelDHSender = "Sealed Sender v2: DH-sender"
)

func hkdfInfo(ikm []byte, info string, n int) []byte {
	out, err := hkdf.Key(sha256.New, ikm, nil, info, n)
	if err != nil {
		panic(err)
	}
	return out
}

// v2Ephemeral derives (E, K) from the message key M (§6.2 steps 2-3).
func v2Ephemeral(m []byte) (*curve25519.PrivateKey, []byte) {
	e, err := curve25519.NewPrivateKey(hkdfInfo(m, labelR, 32))
	if err != nil {
		panic(err)
	}
	return e, hkdfInfo(m, labelK, 32)
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// v2C is the per-recipient encrypted M: HKDF over DH || E || R, xor M.
func v2C(dh []byte, e, r *curve25519.PublicKey, m []byte) []byte {
	c := hkdfInfo(cat(dh, e.Bytes(), r.Bytes()), labelDH, 32)
	subtle.XORBytes(c, c, m)
	return c
}

// v2AT is the 16-byte authentication tag binding the sender identity S.
func v2AT(dh []byte, e *curve25519.PublicKey, c []byte, s, r *curve25519.PublicKey) []byte {
	return hkdfInfo(cat(dh, e.Bytes(), c, s.Bytes(), r.Bytes()), labelDHSender, 16)
}

var zeroNonce = make([]byte, 12)

// ServiceIDFixed is a service ID in fixed-width binary form: a kind byte
// (0x00 ACI, 0x01 PNI) and the 16 UUID bytes.
type ServiceIDFixed [17]byte

// RecipientDevice is one device of a v2 recipient.
type RecipientDevice struct {
	ID             uint8  // 1..127
	RegistrationID uint16 // at most 0x3fff
}

// MultiRecipient is one included recipient of a v2 message.
type MultiRecipient struct {
	ServiceID   ServiceIDFixed
	IdentityKey *curve25519.PublicKey
	Devices     []RecipientDevice
}

// SealV2 seals a USMC for several recipients and returns the SentMessage
// (version 0x23) for the server (§6.2-6.3).
func SealV2(rand io.Reader, sender *curve25519.PrivateKey, recipients []MultiRecipient, excluded []ServiceIDFixed, usmc []byte) ([]byte, error) {
	m := make([]byte, 32)
	if _, err := io.ReadFull(rand, m); err != nil {
		return nil, err
	}
	e, k := v2Ephemeral(m)
	out := []byte{0x23}
	out = protowire.AppendVarint(out, uint64(len(recipients)+len(excluded)))
	for _, r := range recipients {
		if len(r.Devices) == 0 {
			return nil, errors.New("protocol: recipient without devices")
		}
		out = append(out, r.ServiceID[:]...)
		for i, d := range r.Devices {
			if d.ID < 1 || d.ID > 127 {
				return nil, fmt.Errorf("protocol: invalid device ID %d", d.ID)
			}
			if d.RegistrationID > 0x3fff {
				return nil, fmt.Errorf("protocol: invalid registration ID %#x", d.RegistrationID)
			}
			word := d.RegistrationID
			if i < len(r.Devices)-1 {
				word |= 0x8000 // has more
			}
			out = append(out, d.ID, byte(word>>8), byte(word))
		}
		dh, err := e.Agree(r.IdentityKey)
		if err != nil {
			return nil, err
		}
		c := v2C(dh, e.PublicKey(), r.IdentityKey, m)
		sdh, err := sender.Agree(r.IdentityKey)
		if err != nil {
			return nil, err
		}
		out = append(out, c...)
		out = append(out, v2AT(sdh, e.PublicKey(), c, sender.PublicKey(), r.IdentityKey)...)
	}
	for _, x := range excluded {
		out = append(append(out, x[:]...), 0x00)
	}
	out = append(out, e.PublicKey().U()...)
	aead, err := gcmsiv.New(k)
	if err != nil {
		return nil, err
	}
	return aead.Seal(out, zeroNonce, usmc, nil), nil
}

// SentRecipient is one included recipient parsed from a SentMessage.
type SentRecipient struct {
	ServiceID ServiceIDFixed
	Devices   []RecipientDevice
	C, AT     []byte
}

// SentMessage is a parsed v2 SentMessage (server side; §6.4).
type SentMessage struct {
	Recipients []SentRecipient
	Excluded   []ServiceIDFixed
	tail       []byte // E.pub || ciphertext, shared by all recipients
}

var errSentMessage = fmt.Errorf("%w: sealed sender v2 sent message", ErrInvalidMessage)

// ParseSentMessage parses a SentMessage with version 0x22 (bare ACI UUIDs)
// or 0x23 (fixed-width service IDs).
func ParseSentMessage(b []byte) (*SentMessage, error) {
	if len(b) == 0 || (b[0] != 0x22 && b[0] != 0x23) {
		return nil, errSentMessage
	}
	bare := b[0] == 0x22
	b = b[1:]
	count, n := protowire.ConsumeVarint(b)
	if n < 0 {
		return nil, errSentMessage
	}
	b = b[n:]
	sm := new(SentMessage)
	included := map[ServiceIDFixed]int{}
	excluded := map[ServiceIDFixed]bool{}
	take := func(k int) ([]byte, bool) {
		if len(b) < k {
			return nil, false
		}
		v := b[:k]
		b = b[k:]
		return v, true
	}
	for range count {
		var id ServiceIDFixed
		if bare {
			u, ok := take(16)
			if !ok {
				return nil, errSentMessage
			}
			copy(id[1:], u)
		} else {
			u, ok := take(17)
			if !ok || u[0] > 0x01 {
				return nil, errSentMessage
			}
			copy(id[:], u)
		}
		if len(b) == 0 {
			return nil, errSentMessage
		}
		if b[0] == 0 { // excluded-recipient marker
			b = b[1:]
			if _, dup := included[id]; dup || excluded[id] {
				return nil, fmt.Errorf("%w: recipient redundantly encoded as empty", ErrInvalidMessage)
			}
			excluded[id] = true
			sm.Excluded = append(sm.Excluded, id)
			continue
		}
		if excluded[id] {
			return nil, fmt.Errorf("%w: recipient redundantly encoded as empty", ErrInvalidMessage)
		}
		var devs []RecipientDevice
		for {
			rec, ok := take(3)
			if !ok || rec[0] == 0 || rec[0] > 127 {
				return nil, errSentMessage
			}
			word := uint16(rec[1])<<8 | uint16(rec[2])
			devs = append(devs, RecipientDevice{ID: rec[0], RegistrationID: word & 0x3fff})
			if word&0x8000 == 0 {
				break
			}
		}
		cat, ok := take(32 + 16)
		if !ok {
			return nil, errSentMessage
		}
		if i, seen := included[id]; seen {
			// Devices of a repeated service ID join the first entry, whose
			// C and AT are kept.
			sm.Recipients[i].Devices = append(sm.Recipients[i].Devices, devs...)
			continue
		}
		included[id] = len(sm.Recipients)
		sm.Recipients = append(sm.Recipients, SentRecipient{ServiceID: id, Devices: devs, C: cat[:32], AT: cat[32:]})
	}
	if len(b) < 32 {
		return nil, errSentMessage
	}
	sm.tail = b
	return sm, nil
}

// ReceivedMessage is what the server delivers to every device of included
// recipient i: 0x22 || C || AT || E.pub || ciphertext.
func (sm *SentMessage) ReceivedMessage(i int) []byte {
	r := sm.Recipients[i]
	return cat([]byte{0x22}, r.C, r.AT, sm.tail)
}

func unsealV2(recipient *curve25519.PrivateKey, b []byte) (*SealedContent, error) {
	if len(b) < 32+16+32 {
		return nil, fmt.Errorf("%w: v2 message too short", ErrInvalidMessage)
	}
	c, at, epub, ct := b[:32], b[32:48], b[48:80], b[80:]
	e, err := curve25519.NewPublicKey(epub)
	if err != nil {
		return nil, err
	}
	dh, err := recipient.Agree(e)
	if err != nil {
		return nil, err
	}
	m := v2C(dh, e, recipient.PublicKey(), c) // xor with C recovers M
	derived, k := v2Ephemeral(m)
	if subtle.ConstantTimeCompare(derived.PublicKey().U(), epub) != 1 {
		return nil, errors.New("derived ephemeral key did not match key provided in message")
	}
	aead, err := gcmsiv.New(k)
	if err != nil {
		return nil, err
	}
	usmc, err := aead.Open(nil, zeroNonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt inner message: %w", err)
	}
	content, err := parseSealedContent(usmc)
	if err != nil {
		return nil, err
	}
	sdh, err := recipient.Agree(content.Sender.IdentityKey)
	if err != nil {
		return nil, err
	}
	want := v2AT(sdh, e, c, content.Sender.IdentityKey, recipient.PublicKey())
	if subtle.ConstantTimeCompare(want, at) != 1 {
		return nil, errors.New("sender certificate key does not match authentication tag")
	}
	return content, nil
}

// Unseal opens a sealed message (v1, or a v2 received message) with the
// recipient's identity key and validates the sender certificate against
// roots at serverTimestamp, the envelope's server timestamp in
// milliseconds (§4.4, §7 step 1). Every failure is ErrSealedSender.
func Unseal(recipient *curve25519.PrivateKey, roots []*curve25519.PublicKey, serverTimestamp uint64, b []byte) (*SealedContent, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: message was empty", ErrSealedSender)
	}
	var content *SealedContent
	var err error
	switch v := b[0] >> 4; v {
	case 0, 1:
		content, err = unsealV1(recipient, b[1:])
	case 2:
		content, err = unsealV2(recipient, b[1:])
	default:
		return nil, fmt.Errorf("%w: unknown sealed sender version %d", ErrSealedSender, v)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSealedSender, err)
	}
	if !content.Sender.Validate(roots, serverTimestamp) {
		return nil, fmt.Errorf("%w: trust root validation failed", ErrSealedSender)
	}
	return content, nil
}
