// SPDX-License-Identifier: GPL-3.0-or-later

package zkgroup

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519/field"
	"github.com/aren55555/signal-cli-go/internal/crypto/gcmsiv"
	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
)

// Group keys, member ciphertexts, blobs, notary signatures and server
// params (spec/crypto/zkgroup-groups.md).

// Error kinds (§2).
var (
	ErrDeserialization = errors.New("zkgroup: deserialization failure")
	ErrVerification    = errors.New("zkgroup: verification failure")
)

// Sho labels (§9).
const (
	labelGenerate     = "Signal_ZKGroup_20200424_Random_GroupSecretParams_Generate"
	labelDerive       = "Signal_ZKGroup_20200424_GroupMasterKey_GroupSecretParams_DeriveFromMasterKey"
	labelUidGens      = "Signal_ZKGroup_20200424_Constant_UidEncryption_SystemParams_Generate"
	labelProfileGens  = "Signal_ZKGroup_20200424_Constant_ProfileKeyEncryption_SystemParams_Generate"
	labelCalcM1       = "Signal_ZKGroup_20200424_UID_CalcM1"
	labelEncryptBlob  = "Signal_ZKGroup_20200424_Random_GroupSecretParams_EncryptBlob"
	labelPokshoSHA256 = "POKSHO_Ristretto_SHOHMACSHA256"
)

// stagingParamsB64 is signal-cli 0.14.7's staging zkGroupServerPublicParams
// (§8.1; interop data).
const stagingParamsB64 = "ABSY21VckQcbSXVNCGRYJcfWHiAMZmpTtTELcDmxgdFbtp/bWsSxZdMKzfCp8rvIs8ocCU3B37fT3r4Mi5qAemeGeR2X+/YmOGR5ofui7tD5mDQfstAI9i+4WpMtIe8KC3wU5w3Inq3uNWVmoGtpKndsNfwJrCg0Hd9zmObhypUnSkfYn2ooMOOnBpfdanRtrvetZUayDMSC5iSRcXKpdlukrpzzsCIvEwjwQlJYVPOQPj4V0F4UXXBdHSLK05uoPBCQG8G9rYIGedYsClJXnbrgGYG3eMTG5hnx4X4ntARBgELuMWWUEEfSK0mjXg+/2lPmWcTZWR9nkqgQQP0tbzuiPm74H2wMO4u1Wafe+UwyIlIT9L7KLS19Aw8r4sPrXZSSsOZ6s7M1+rTJN0bI5CKY2PX29y5Ok3jSWufIKcgKOnWoP67d5b2du2ZVJjpjfibNIHbT/cegy/sBLoFwtHogVYUewANUAXIaMPyCLRArsKhfJ5wBtTminG/PAvuBdJ70Z/bXVPf8TVsR292zQ65xwvWTejROW6AZX6aqucUjlENAErBme1YHmOSpU6tr6doJ66dPzVAWIanmO/5mgjNEDeK7DDqQdB1xd03HT2Qs2TxY3kCK8aAb/0iM0HQiXjxZ9HIgYhbtvGEnDKW5ILSUydqH/KBhW4Pb0jZWnqN/YgbWDKeJxnDbYcUob5ZY5Lt5ZCMKuaGUvCJRrCtuugSMaqjowCGRempsDdJEt+cMaalhZ6gczklJB/IbdwENW9KeVFPoFNFzhxWUIS5ML9riVYhAtE6JE5jX0xiHNVIIPthb458cfA8daR0nYfYAUKogQArm0iBezOO+mPk5vCNWI+wwkyFCqNDXz/qxl1gAntuCJtSfq9OC3NkdhQlgYQ=="

// StagingServerPublicParams returns the staging server public params.
func StagingServerPublicParams() []byte {
	b, _ := base64.StdEncoding.DecodeString(stagingParamsB64)
	return b
}

// systemPair squeezes two points from a constant-generator Sho (§3.4).
func systemPair(label string) (*ristretto255.Element, *ristretto255.Element) {
	s := oneShot(label, nil)
	return s.point(), s.point()
}

var (
	gA1, gA2 = systemPair(labelUidGens)
	gB1, gB2 = systemPair(labelProfileGens)
)

// GenerateMasterKey derives a new master key from 32 random bytes (§3.1).
func GenerateMasterKey(randomness [32]byte) [32]byte {
	var k [32]byte
	copy(k[:], oneShot(labelGenerate, randomness[:]).Squeeze(32))
	return k
}

// GroupSecretParams are everything derived from a group master key (§3.2).
type GroupSecretParams struct {
	MasterKey, GroupID, BlobKey [32]byte
	a1, a2, b1, b2              *edwards25519.Scalar
	aUid, aPk                   *ristretto255.Element
}

// DeriveGroupSecretParams derives the group params from a master key.
func DeriveGroupSecretParams(master [32]byte) *GroupSecretParams {
	s := oneShot(labelDerive, master[:])
	g := &GroupSecretParams{MasterKey: master}
	copy(g.GroupID[:], s.Squeeze(32))
	copy(g.BlobKey[:], s.Squeeze(32))
	g.a1, g.a2, g.b1, g.b2 = s.scalar(), s.scalar(), s.scalar(), s.scalar()
	g.aUid = lincomb(g.a1, gA1, g.a2, gA2)
	g.aPk = lincomb(g.b1, gB1, g.b2, gB2)
	return g
}

func lincomb(x *edwards25519.Scalar, p *ristretto255.Element, y *edwards25519.Scalar, q *ristretto255.Element) *ristretto255.Element {
	a := new(ristretto255.Element).ScalarMult(x, p)
	return a.Add(a, new(ristretto255.Element).ScalarMult(y, q))
}

// Serialize is the 289-byte GroupSecretParams (§3.3).
func (g *GroupSecretParams) Serialize() []byte {
	out := []byte{0}
	out = append(out, g.MasterKey[:]...)
	out = append(out, g.GroupID[:]...)
	out = append(out, g.BlobKey[:]...)
	out = append(out, g.a1.Bytes()...)
	out = append(out, g.a2.Bytes()...)
	out = append(out, g.aUid.Bytes()...)
	out = append(out, g.b1.Bytes()...)
	out = append(out, g.b2.Bytes()...)
	return append(out, g.aPk.Bytes()...)
}

// PublicParams is the 97-byte GroupPublicParams (§3.3).
func (g *GroupSecretParams) PublicParams() []byte {
	out := append([]byte{0}, g.GroupID[:]...)
	out = append(out, g.aUid.Bytes()...)
	return append(out, g.aPk.Bytes()...)
}

// ServiceID is an ACI or a PNI.
type ServiceID struct {
	PNI  bool
	UUID [16]byte
}

// Binary is the hashed form: 16 bytes for an ACI, 0x01 ‖ UUID for a PNI.
func (id ServiceID) Binary() []byte {
	if id.PNI {
		return append([]byte{1}, id.UUID[:]...)
	}
	return id.UUID[:]
}

// FixedWidth is the 17-byte form: kind byte then UUID.
func (id ServiceID) FixedWidth() []byte {
	k := byte(0)
	if id.PNI {
		k = 1
	}
	return append([]byte{k}, id.UUID[:]...)
}

func calcM1(id ServiceID) *ristretto255.Element { return oneShot(labelCalcM1, id.Binary()).point() }

// lizardBytes is steps 1-3 of Lizard encoding (§4.3).
func lizardBytes(u []byte) [32]byte {
	h := sha256.Sum256(u)
	var s [32]byte
	copy(s[:8], h[:8])
	copy(s[8:24], u)
	copy(s[24:], h[24:])
	s[0] &= 0xFE
	s[31] &= 0x3F
	return s
}

// LizardEncode maps 16 bytes to a point (§4.3).
func LizardEncode(u [16]byte) *ristretto255.Element {
	s := lizardBytes(u[:])
	e, _ := new(ristretto255.Element).SetElligatorBytes(s[:])
	return e
}

// LizardDecode recovers the 16 bytes if exactly one preimage slot passes.
func LizardDecode(p *ristretto255.Element) ([16]byte, bool) {
	var out [16]byte
	n := 0
	for _, c := range preimageSlots(p) {
		if c == nil {
			continue
		}
		cb := c.Bytes()
		if want := lizardBytes(cb[8:24]); string(want[:]) == string(cb) {
			copy(out[:], cb[8:24])
			n++
		}
	}
	return out, n == 1
}

func fe() *field.Element { return new(field.Element) }

func feInt(n int64) *field.Element {
	var b [32]byte
	binary.LittleEndian.PutUint64(b[:], uint64(n))
	e, _ := fe().SetBytes(b[:])
	return e
}

var (
	sqrtM1 = ristretto255.SqrtM1()
	feOne  = feInt(1)
	// SQRT_ID, K1, K2, K3 of §4.4.
	sqrtID = func() *field.Element {
		r, _ := fe().SqrtRatio(fe().Multiply(sqrtM1, ristretto255.D()), feOne)
		return r
	}()
	k1 = func() *field.Element {
		d := ristretto255.D()
		return fe().Multiply(fe().Add(d, feOne), fe().Invert(fe().Subtract(d, feOne)))
	}()
	k2 = fe().Negate(fe().Add(ristretto255.InvSqrtAMinusD(), ristretto255.InvSqrtAMinusD()))
	k3 = func() *field.Element {
		r, _ := fe().SqrtRatio(feOne, fe().Add(feOne, ristretto255.D()))
		return fe().Negate(r)
	}()
)

// preimageSlots returns the eight slots of §4.4: nil (undefined) or a
// non-negative field element r with MAP(r) = p.
func preimageSlots(p *ristretto255.Element) [8]*field.Element {
	x, y := p.Affine()
	zero := fe().Zero()
	type jp struct{ s, t *field.Element }
	var pts [4]jp
	if x.Equal(zero) == 1 || y.Equal(zero) == 1 {
		pts = [4]jp{{fe().Zero(), feInt(1)}, {fe().Zero(), feInt(1)}, {feInt(1), fe().Multiply(k2, sqrtM1)}, {fe().Negate(feOne), fe().Multiply(k2, sqrtM1)}}
	} else {
		y2 := fe().Square(y)
		oneMinusY2 := fe().Subtract(feOne, y2)
		g, _ := fe().SqrtRatio(feOne, fe().Multiply(fe().Multiply(fe().Square(y2), fe().Square(x)), oneMinusY2))
		e := fe().Multiply(g, y2)
		e2 := fe().Multiply(fe().Multiply(oneMinusY2, k3), g)
		oneMinusY := fe().Subtract(feOne, y)
		onePlusY := fe().Add(feOne, y)
		iMinusX := fe().Subtract(sqrtM1, x)
		iPlusX := fe().Add(sqrtM1, x)
		pts = [4]jp{
			{fe().Multiply(fe().Multiply(e, oneMinusY), x), fe().Multiply(fe().Multiply(k2, e), oneMinusY)},
			{fe().Negate(fe().Multiply(fe().Multiply(e, onePlusY), x)), fe().Multiply(fe().Multiply(k2, e), onePlusY)},
			{fe().Multiply(fe().Multiply(e2, iMinusX), y), fe().Multiply(fe().Multiply(fe().Multiply(k2, sqrtM1), e2), iMinusX)},
			{fe().Negate(fe().Multiply(fe().Multiply(e2, iPlusX), y)), fe().Multiply(fe().Multiply(fe().Multiply(k2, sqrtM1), e2), iPlusX)},
		}
	}
	var out [8]*field.Element
	for i, q := range pts {
		out[2*i] = slotValue(q.s, q.t)
		out[2*i+1] = slotValue(fe().Negate(q.s), fe().Negate(q.t))
	}
	identity := p.Equal(ristretto255.NewIdentityElement()) == 1
	seen := map[string]bool{}
	for i, c := range out {
		if c == nil {
			continue
		}
		// Keep only true preimages; outside the identity, each once (§4.4).
		if new(ristretto255.Element).SetMap(c).Equal(p) != 1 || (!identity && seen[string(c.Bytes())]) {
			out[i] = nil
			continue
		}
		seen[string(c.Bytes())] = true
	}
	return out
}

// slotValue is step 4 of the §4.4 method.
func slotValue(s, t *field.Element) *field.Element {
	if s.Equal(fe().Zero()) == 1 {
		if t.Equal(feOne) == 1 {
			return fe().Set(sqrtID)
		}
		return fe().Zero()
	}
	a := fe().Multiply(fe().Add(t, feOne), k1)
	s2 := fe().Square(s)
	w, ok := fe().SqrtRatio(feOne, fe().Multiply(sqrtM1, fe().Subtract(fe().Square(s2), fe().Square(a))))
	if ok != 1 {
		return nil
	}
	var v *field.Element
	if s.IsNegative() == 0 {
		v = fe().Multiply(fe().Add(a, s2), w)
	} else {
		v = fe().Multiply(fe().Subtract(a, s2), w)
	}
	return v.Absolute(v)
}

// parseCiphertext reads a 65-byte UuidCiphertext or ProfileKeyCiphertext.
func parseCiphertext(b []byte) (e1, e2 *ristretto255.Element, err error) {
	if len(b) != 65 || b[0] != 0 {
		return nil, nil, ErrDeserialization
	}
	if e1, err = new(ristretto255.Element).SetCanonicalBytes(b[1:33]); err != nil {
		return nil, nil, ErrDeserialization
	}
	if e2, err = new(ristretto255.Element).SetCanonicalBytes(b[33:65]); err != nil {
		return nil, nil, ErrDeserialization
	}
	return e1, e2, nil
}

func encodeCiphertext(e1, e2 *ristretto255.Element) []byte {
	return append(append([]byte{0}, e1.Bytes()...), e2.Bytes()...)
}

// EncryptServiceID is the 65-byte UuidCiphertext (§4.2).
func (g *GroupSecretParams) EncryptServiceID(id ServiceID) []byte {
	e1 := new(ristretto255.Element).ScalarMult(g.a1, calcM1(id))
	m2 := LizardEncode(id.UUID)
	e2 := new(ristretto255.Element).ScalarMult(g.a2, e1)
	return encodeCiphertext(e1, e2.Add(e2, m2))
}

// DecryptServiceID is §4.5.
func (g *GroupSecretParams) DecryptServiceID(ct []byte) (ServiceID, error) {
	e1, e2, err := parseCiphertext(ct)
	if err != nil {
		return ServiceID{}, err
	}
	if e1.Equal(ristretto255.NewGeneratorElement()) == 1 {
		return ServiceID{}, ErrVerification
	}
	m2 := new(ristretto255.Element).ScalarMult(g.a2, e1)
	m2.Subtract(e2, m2)
	u, ok := LizardDecode(m2)
	if !ok {
		return ServiceID{}, ErrVerification
	}
	m1 := new(ristretto255.Element).ScalarMult(edwards25519.NewScalar().Invert(g.a1), e1)
	for _, id := range []ServiceID{{UUID: u}, {PNI: true, UUID: u}} {
		if calcM1(id).Equal(m1) == 1 {
			return id, nil
		}
	}
	return ServiceID{}, ErrVerification
}

// EncryptProfileKey is the 65-byte ProfileKeyCiphertext (§5.2).
func (g *GroupSecretParams) EncryptProfileKey(key [32]byte, aci [16]byte) []byte {
	e1 := new(ristretto255.Element).ScalarMult(g.b1, m3(key, aci))
	e2 := new(ristretto255.Element).ScalarMult(g.b2, e1)
	return encodeCiphertext(e1, e2.Add(e2, m4(key)))
}

// DecryptProfileKey is §5.3: exactly one (slot, variant) must match.
func (g *GroupSecretParams) DecryptProfileKey(ct []byte, aci [16]byte) ([32]byte, error) {
	var out [32]byte
	e1, e2, err := parseCiphertext(ct)
	if err != nil {
		return out, err
	}
	if e1.Equal(ristretto255.NewGeneratorElement()) == 1 {
		return out, ErrVerification
	}
	m4p := new(ristretto255.Element).ScalarMult(g.b2, e1)
	m4p.Subtract(e2, m4p)
	target := new(ristretto255.Element).ScalarMult(edwards25519.NewScalar().Invert(g.b1), e1)
	matches := 0
	for _, c := range preimageSlots(m4p) {
		if c == nil {
			continue
		}
		cb := c.Bytes()
		for v := 0; v < 8; v++ {
			var k [32]byte
			copy(k[:], cb)
			if v&1 != 0 {
				k[0] |= 0x01
			}
			if v&2 != 0 {
				k[31] |= 0x80
			}
			if v&4 != 0 {
				k[31] |= 0x40
			}
			if m3(k, aci).Equal(target) == 1 {
				matches++
				out = k
			}
		}
	}
	if matches != 1 {
		return [32]byte{}, ErrVerification
	}
	return out, nil
}

// encryptBlobUnpadded is §6.1.
func (g *GroupSecretParams) encryptBlobUnpadded(randomness [32]byte, plain []byte) []byte {
	nonce := oneShot(labelEncryptBlob, randomness[:]).Squeeze(12)
	a, _ := gcmsiv.New(g.BlobKey[:])
	out := a.Seal(nil, nonce, plain, nil)
	out = append(out, nonce...)
	return append(out, 0)
}

func (g *GroupSecretParams) decryptBlobUnpadded(b []byte) ([]byte, error) {
	if len(b) < 13 {
		return nil, ErrVerification
	}
	b = b[:len(b)-1] // the reserved byte, whatever its value
	nonce, c := b[len(b)-12:], b[:len(b)-12]
	if len(c) < 16 {
		return nil, ErrVerification
	}
	a, _ := gcmsiv.New(g.BlobKey[:])
	p, err := a.Open(nil, nonce, c, nil)
	if err != nil {
		return nil, ErrVerification
	}
	return p, nil
}

// EncryptBlob is the padded blob of §6.2 (clients use padding 0).
func (g *GroupSecretParams) EncryptBlob(randomness [32]byte, plain []byte, padding uint32) []byte {
	xp := binary.BigEndian.AppendUint32(nil, padding)
	xp = append(xp, plain...)
	xp = append(xp, make([]byte, padding)...)
	return g.encryptBlobUnpadded(randomness, xp)
}

// DecryptBlob is §6.2's decryption.
func (g *GroupSecretParams) DecryptBlob(b []byte) ([]byte, error) {
	xp, err := g.decryptBlobUnpadded(b)
	if err != nil {
		return nil, err
	}
	if len(xp) < 4 {
		return nil, ErrVerification
	}
	n := uint64(binary.BigEndian.Uint32(xp))
	if uint64(len(xp)-4) < n {
		return nil, ErrVerification
	}
	return xp[4 : uint64(len(xp))-n], nil
}

// VerifyNotarySignature checks a server signature over msg (§7.3).
func VerifyNotarySignature(pub *ristretto255.Element, msg, sig []byte) bool {
	if len(sig) != 64 {
		return false
	}
	c, err1 := edwards25519.NewScalar().SetCanonicalBytes(sig[:32])
	s, err2 := edwards25519.NewScalar().SetCanonicalBytes(sig[32:])
	if err1 != nil || err2 != nil {
		return false
	}
	r := new(ristretto255.Element).ScalarBaseMult(s)
	r.Subtract(r, new(ristretto255.Element).ScalarMult(c, pub))
	sho := NewSho([]byte(labelPokshoSHA256))
	stmt := append([]byte{1, 1, 1, 0, 0}, ristretto255.NewGeneratorElement().Bytes()...)
	sho.Absorb(append(stmt, pub.Bytes()...))
	sho.Absorb(append(r.Bytes(), msg...))
	return sho.scalar().Equal(c) == 1
}

// ServerPublicParams are the parts of the 673-byte server params a client
// uses (§8.2).
type ServerPublicParams struct {
	SigPublicKey         *ristretto255.Element
	ExpiringProfileKey   [2]*ristretto255.Element // credential public key at offset 289
	GenericCredential    [7]*ristretto255.Element // offset 417
	EndorsementPublicKey *ristretto255.Element    // offset 641
	ReceiptCredentialKey [2]*ristretto255.Element // offset 161
}

// ParseServerPublicParams parses the server params; every point must
// decode. Trailing bytes are ignored, as upstream does.
func ParseServerPublicParams(b []byte) (*ServerPublicParams, error) {
	if len(b) < 673 || b[0] != 0 {
		return nil, ErrDeserialization
	}
	pts := make([]*ristretto255.Element, 21)
	for i := range pts {
		p, err := new(ristretto255.Element).SetCanonicalBytes(b[1+32*i : 33+32*i])
		if err != nil {
			return nil, ErrDeserialization
		}
		pts[i] = p
	}
	p := &ServerPublicParams{SigPublicKey: pts[4], ReceiptCredentialKey: [2]*ristretto255.Element{pts[5], pts[6]},
		ExpiringProfileKey: [2]*ristretto255.Element{pts[9], pts[10]}, EndorsementPublicKey: pts[20]}
	copy(p.GenericCredential[:], pts[13:20])
	return p, nil
}
