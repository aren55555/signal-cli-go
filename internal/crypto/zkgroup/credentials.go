// SPDX-License-Identifier: GPL-3.0-or-later

package zkgroup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
)

// Credentials, presentations and group send endorsements
// (spec/crypto/zkgroup-credentials.md).

const (
	labelZKCSystem      = "Signal_ZKCredential_ConstantSystemParams_generate_20230410"
	labelZKGCredSystem  = "Signal_ZKGroup_20200424_Constant_Credentials_SystemParams_Generate"
	labelTimestampM     = "Signal_ZKGroup_20220524_Timestamp_Calc_m"
	labelAuthZkc        = "20240222_Signal_AuthCredentialZkc"
	labelZKCPresent     = "Signal_ZKCredential_Presentation_20230410"
	labelProfileRequest = "Signal_ZKGroup_20200424_Random_ServerPublicParams_CreateProfileKeyCredentialRequestContext"
	labelProfilePresent = "Signal_ZKGroup_20220508_Random_ServerPublicParams_CreateExpiringProfileKeyCredentialPresentation"
	labelEndorseTag     = "20240215_Signal_GroupSendEndorsement"
	labelEndorseWeights = "Signal_ZKCredential_Endorsements_EndorsementResponse_ProofWeights_20240207"
)

// zkcGenerators are the zkcredential points (§4.1).
type zkcGenerators struct {
	w, wp, x0, x1, v, z *ristretto255.Element
	y                   [7]*ristretto255.Element
}

func (g *zkcGenerators) all() []*ristretto255.Element {
	return append([]*ristretto255.Element{g.w, g.wp, g.x0, g.x1, g.v, g.z}, g.y[:]...)
}

func shoSHA256Point(s *shoSHA256) *ristretto255.Element {
	e, _ := new(ristretto255.Element).SetUniformBytes(s.squeeze(64))
	return e
}

var gk = func() *zkcGenerators {
	s := newShoSHA256([]byte(labelZKCSystem))
	g := &zkcGenerators{}
	for _, p := range []**ristretto255.Element{&g.w, &g.wp, &g.x0, &g.x1, &g.v, &g.z} {
		*p = shoSHA256Point(s)
	}
	for i := range g.y {
		g.y[i] = shoSHA256Point(s)
	}
	return g
}()

// zkgGenerators are zkgroup's classic credential points (§4.2), indexed
// y[1..6], m[1..5].
type zkgGenerators struct {
	w, wp, x0, x1, v, z *ristretto255.Element
	y                   [7]*ristretto255.Element
	m                   [6]*ristretto255.Element
}

func (g *zkgGenerators) storageOrder() []*ristretto255.Element {
	out := []*ristretto255.Element{g.w, g.wp, g.x0, g.x1}
	out = append(out, g.y[1:]...)
	out = append(out, g.m[1:]...)
	return append(out, g.v, g.z)
}

var gz = func() *zkgGenerators {
	s := oneShot(labelZKGCredSystem, nil)
	g := &zkgGenerators{}
	order := []**ristretto255.Element{&g.w, &g.wp, &g.x0, &g.x1, &g.y[1], &g.y[2], &g.y[3], &g.y[4],
		&g.m[1], &g.m[2], &g.m[3], &g.m[4], &g.v, &g.z, &g.y[5], &g.y[6], &g.m[5]}
	for _, p := range order {
		*p = s.point()
	}
	return g
}()

func le64(n uint64) []byte { return binary.LittleEndian.AppendUint64(nil, n) }

func be64(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }

func mul(k *edwards25519.Scalar, p *ristretto255.Element) *ristretto255.Element {
	return new(ristretto255.Element).ScalarMult(k, p)
}

func add(ps ...*ristretto255.Element) *ristretto255.Element {
	sum := ristretto255.NewIdentityElement()
	for _, p := range ps {
		sum.Add(sum, p)
	}
	return sum
}

func sub(a, b *ristretto255.Element) *ristretto255.Element {
	return new(ristretto255.Element).Subtract(a, b)
}

func neg(a *ristretto255.Element) *ristretto255.Element { return new(ristretto255.Element).Negate(a) }

func negScalar(a *edwards25519.Scalar) *edwards25519.Scalar {
	return edwards25519.NewScalar().Negate(a)
}

func mulScalar(a, b *edwards25519.Scalar) *edwards25519.Scalar {
	return edwards25519.NewScalar().Multiply(a, b)
}

// reader walks a serialized value (§1.3).
type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || len(r.b) < n {
		r.err = ErrDeserialization
		return make([]byte, n)
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *reader) byteValue(want byte) {
	if r.take(1)[0] != want && r.err == nil {
		r.err = ErrDeserialization
	}
}

func (r *reader) point() *ristretto255.Element {
	p, err := new(ristretto255.Element).SetCanonicalBytes(r.take(32))
	if err != nil && r.err == nil {
		r.err = ErrDeserialization
	}
	if p == nil {
		p = ristretto255.NewIdentityElement()
	}
	return p
}

func (r *reader) scalar() *edwards25519.Scalar {
	k, err := edwards25519.NewScalar().SetCanonicalBytes(r.take(32))
	if err != nil {
		if r.err == nil {
			r.err = ErrDeserialization
		}
		return edwards25519.NewScalar()
	}
	return k
}

func (r *reader) u64() uint64 { return binary.LittleEndian.Uint64(r.take(8)) }

func (r *reader) bytesField() []byte {
	n := r.u64()
	if n > uint64(len(r.b)) {
		r.err = ErrDeserialization
		return nil
	}
	return r.take(int(n))
}

func (r *reader) done() error {
	if r.err == nil && len(r.b) != 0 {
		return ErrDeserialization
	}
	return r.err
}

// uidPoints are M1, M2 of a service ID (§5.1).
func uidPoints(id ServiceID) (*ristretto255.Element, *ristretto255.Element) {
	return calcM1(id), LizardEncode(id.UUID)
}

func m5(t uint64) *edwards25519.Scalar { return oneShot(labelTimestampM, be64(t)).scalar() }

// zkcredential (§6)

// issuanceStatement is §6.5 for n attribute points.
func issuanceStatement(n int) *statement {
	s := newStatement().eq("C_W", "w", "G_w", "wprime", "G_wprime")
	p := []string{"x0", "G_x0", "x1", "G_x1"}
	for i := 0; i < n; i++ {
		p = append(p, "y"+strconv.Itoa(i), "G_y"+strconv.Itoa(i))
	}
	s.eq("G_V-I", p...)
	p = []string{"w", "G_w", "x0", "U", "x1", "tU"}
	for i := 0; i < n; i++ {
		p = append(p, "y"+strconv.Itoa(i), "M"+strconv.Itoa(i))
	}
	return s.eq("V", p...)
}

// zkcCredential is (t, U, V) (§6.2).
type zkcCredential struct {
	t    *edwards25519.Scalar
	u, v *ristretto255.Element
}

func (c *zkcCredential) bytes() []byte {
	return append(append(append([]byte{}, c.t.Bytes()...), c.u.Bytes()...), c.v.Bytes()...)
}

// verifyIssuance checks a zkcredential issuance proof (§6.5).
func verifyIssuance(p *ServerPublicParams, c *zkcCredential, attrs []*ristretto255.Element, proof []byte) bool {
	n := len(attrs)
	pts := map[string]*ristretto255.Element{
		"C_W": p.GenericCredential[0], "G_w": gk.w, "G_wprime": gk.wp, "G_V-I": sub(gk.v, p.GenericCredential[n-1]),
		"G_x0": gk.x0, "G_x1": gk.x1, "V": c.v, "U": c.u, "tU": mul(c.t, c.u),
	}
	for i := 0; i < n; i++ {
		pts["G_y"+strconv.Itoa(i)] = gk.y[i]
		pts["M"+strconv.Itoa(i)] = attrs[i]
	}
	return issuanceStatement(n).verify(proof, pts, nil)
}

// authPresentationStatement is §7.4's 9-equation statement.
func authPresentationStatement() *statement {
	s := newStatement().
		eq("Z", "z", "I").
		eq("C_x1", "t", "C_x0", "z0", "G_x0", "z", "G_x1").
		eq("0", "z1", "I", "a1", "Z").
		eq("sumA", "a1", "G_a1", "a2", "G_a2")
	for _, f := range []int{1, 3} {
		F, F1 := strconv.Itoa(f), strconv.Itoa(f+1)
		s.eq("E_A"+F, "a1", "C_y"+F, "z1", "G_y"+F)
		s.eq("C_y"+F1+"-E_A"+F1, "z", "G_y"+F1, "a2", "-E_A"+F)
	}
	return s.eq("C_y0", "z", "G_y0")
}

// AuthCredentialWithPni is a received auth credential (§7.3).
type AuthCredentialWithPni struct {
	cred     zkcCredential
	aci, pni [16]byte
	aciM     [2]*ristretto255.Element
	pniM     [2]*ristretto255.Element
	t        uint64
}

// RedemptionTime is the credential's day (seconds).
func (a *AuthCredentialWithPni) RedemptionTime() uint64 { return a.t }

// ReceiveAuthCredentialWithPni verifies a 425-byte response for our ACI,
// PNI and the redemption time (§7.2-7.3).
func ReceiveAuthCredentialWithPni(p *ServerPublicParams, aci, pni [16]byte, redemption uint64, resp []byte) (*AuthCredentialWithPni, error) {
	r := &reader{b: resp}
	r.byteValue(0x03)
	c := zkcCredential{t: r.scalar(), u: r.point(), v: r.point()}
	proof := r.bytesField()
	if err := r.done(); err != nil {
		return nil, err
	}
	if redemption%86400 != 0 {
		return nil, ErrVerification
	}
	a := &AuthCredentialWithPni{cred: c, aci: aci, pni: pni, t: redemption}
	a.aciM[0], a.aciM[1] = uidPoints(ServiceID{UUID: aci})
	a.pniM[0], a.pniM[1] = uidPoints(ServiceID{PNI: true, UUID: pni})
	m0 := oneShot(labelAuthZkc, be64(redemption)).point()
	if !verifyIssuance(p, &c, []*ristretto255.Element{m0, a.aciM[0], a.aciM[1], a.pniM[0], a.pniM[1]}, proof) {
		return nil, ErrVerification
	}
	return a, nil
}

// Serialize is the 265-byte credential (§7.3).
func (a *AuthCredentialWithPni) Serialize() []byte {
	out := append([]byte{0x03}, a.cred.bytes()...)
	out = append(out, a.aci[:]...)
	out = append(out, a.aciM[0].Bytes()...)
	out = append(out, a.aciM[1].Bytes()...)
	out = append(out, a.pni[:]...)
	out = append(out, a.pniM[0].Bytes()...)
	out = append(out, a.pniM[1].Bytes()...)
	return append(out, le64(a.t)...)
}

// Present builds the 633-byte presentation for a group (§6.6, §7.4).
func (a *AuthCredentialWithPni) Present(p *ServerPublicParams, g *GroupSecretParams, randomness [32]byte) []byte {
	const n = 5
	sho := oneShot(labelZKCPresent, randomness[:])
	z := sho.scalar()
	m := []*ristretto255.Element{nil, a.aciM[0], a.aciM[1], a.pniM[0], a.pniM[1]}
	cy := make([]*ristretto255.Element, n)
	cy[0] = mul(z, gk.y[0])
	for i := 1; i < n; i++ {
		cy[i] = add(mul(z, gk.y[i]), m[i])
	}
	c := a.cred
	cx0 := add(mul(z, gk.x0), c.u)
	cx1 := add(mul(z, gk.x1), mul(c.t, c.u))
	cv := add(mul(z, gk.v), c.v)
	iN := p.GenericCredential[n-1]
	zP := mul(z, iN)
	z0 := negScalar(mulScalar(z, c.t))
	z1 := negScalar(mulScalar(z, g.a1))
	ea := make([]*ristretto255.Element, n)
	for _, f := range []int{1, 3} {
		ea[f] = mul(g.a1, m[f])
		ea[f+1] = add(mul(g.a2, ea[f]), m[f+1])
	}
	pts := map[string]*ristretto255.Element{
		"Z": zP, "I": iN, "C_x1": cx1, "C_x0": cx0, "G_x0": gk.x0, "G_x1": gk.x1, "0": ristretto255.NewIdentityElement(),
		"sumA": g.aUid, "G_a1": gA1, "G_a2": gA2, "C_y0": cy[0], "G_y0": gk.y[0],
	}
	for _, f := range []int{1, 3} {
		F, F1 := strconv.Itoa(f), strconv.Itoa(f+1)
		pts["E_A"+F] = ea[f]
		pts["C_y"+F] = cy[f]
		pts["G_y"+F] = gk.y[f]
		pts["C_y"+F1+"-E_A"+F1] = sub(cy[f+1], ea[f+1])
		pts["G_y"+F1] = gk.y[f+1]
		pts["-E_A"+F] = neg(ea[f])
	}
	scalars := map[string]*edwards25519.Scalar{"z": z, "t": c.t, "z0": z0, "z1": z1, "a1": g.a1, "a2": g.a2}
	proof := authPresentationStatement().prove(scalars, pts, nil, sho.Squeeze(32))
	out := []byte{0x03}
	for _, q := range []*ristretto255.Element{cx0, cx1, cv} {
		out = append(out, q.Bytes()...)
	}
	out = append(out, le64(n)...)
	for _, q := range cy {
		out = append(out, q.Bytes()...)
	}
	out = append(out, le64(uint64(len(proof)))...)
	out = append(out, proof...)
	for f := 1; f < n; f++ {
		out = append(out, ea[f].Bytes()...)
	}
	return append(out, le64(a.t)...)
}

// Expiring profile key credentials (§8)

func profileRequestStatement() *statement {
	return newStatement().
		eq("Y", "y", "G").
		eq("D1", "r1", "G").
		eq("E1", "r2", "G").
		eq("J3", "j3", "G_j3").
		eq("D2-J1", "r1", "Y", "j3", "-G_j1").
		eq("E2-J2", "r2", "Y", "j3", "-G_j2")
}

func profileIssuanceStatement() *statement {
	return newStatement().
		eq("C_W", "w", "G_w", "wprime", "G_wprime").
		eq("G_V-I", "x0", "G_x0", "x1", "G_x1", "y1", "G_y1", "y2", "G_y2", "y3", "G_y3", "y4", "G_y4", "y5", "G_y5").
		eq("S1", "y3", "D1", "y4", "E1", "rprime", "G").
		eq("S2", "y3", "D2", "y4", "E2", "rprime", "Y", "w", "G_w", "x0", "U", "x1", "tU", "y1", "M1", "y2", "M2", "y5", "M5")
}

func profilePresentationStatement() *statement {
	return newStatement().
		eq("Z", "z", "I").
		eq("C_x1", "t", "C_x0", "z0", "G_x0", "z", "G_x1").
		eq("A+B", "a1", "G_a1", "a2", "G_a2", "b1", "G_b1", "b2", "G_b2").
		eq("C_y2-E_A2", "z", "G_y2", "a2", "-E_A1").
		eq("E_A1", "a1", "C_y1", "z1", "G_y1").
		eq("C_y4-E_B2", "z", "G_y4", "b2", "-E_B1").
		eq("E_B1", "b1", "C_y3", "z2", "G_y3").
		eq("0", "z1", "I", "a1", "Z").
		eq("0", "z2", "I", "b1", "Z").
		eq("C_y5", "z", "G_y5")
}

// ProfileKeyCredentialRequestContext is the client side of a profile
// credential request (§8.1).
type ProfileKeyCredentialRequestContext struct {
	aci                  [16]byte
	key                  [32]byte
	y, r1, r2            *edwards25519.Scalar
	bigY, d1, d2, e1, e2 *ristretto255.Element
	proof                []byte
}

// NewProfileKeyCredentialRequestContext builds a request for (ACI,
// profile key) with 32 bytes of randomness.
func NewProfileKeyCredentialRequestContext(aci [16]byte, key [32]byte, randomness [32]byte) *ProfileKeyCredentialRequestContext {
	g := generators()
	j := j3(key, aci)
	m3p, m4p := m3(key, aci), m4(key)
	j1 := add(mul(j, g[0]), m3p)
	j2 := add(mul(j, g[1]), m4p)
	j3p := mul(j, g[2])
	sho := oneShot(labelProfileRequest, randomness[:])
	c := &ProfileKeyCredentialRequestContext{aci: aci, key: key, y: sho.scalar(), r1: sho.scalar(), r2: sho.scalar()}
	c.bigY = new(ristretto255.Element).ScalarBaseMult(c.y)
	c.d1 = new(ristretto255.Element).ScalarBaseMult(c.r1)
	c.e1 = new(ristretto255.Element).ScalarBaseMult(c.r2)
	c.d2 = add(mul(c.r1, c.bigY), m3p)
	c.e2 = add(mul(c.r2, c.bigY), m4p)
	pts := map[string]*ristretto255.Element{"Y": c.bigY, "D1": c.d1, "E1": c.e1, "J3": j3p, "G_j3": g[2],
		"D2-J1": sub(c.d2, j1), "-G_j1": neg(g[0]), "E2-J2": sub(c.e2, j2), "-G_j2": neg(g[1])}
	c.proof = profileRequestStatement().prove(map[string]*edwards25519.Scalar{"y": c.y, "r1": c.r1, "r2": c.r2, "j3": j},
		pts, nil, sho.Squeeze(32))
	return c
}

// Serialize is the 473-byte context.
func (c *ProfileKeyCredentialRequestContext) Serialize() []byte {
	out := append([]byte{0}, c.aci[:]...)
	out = append(out, c.key[:]...)
	out = append(out, c.y.Bytes()...)
	out = append(out, c.bigY.Bytes()...)
	out = append(out, c.r1.Bytes()...)
	out = append(out, c.r2.Bytes()...)
	for _, p := range []*ristretto255.Element{c.d1, c.d2, c.e1, c.e2} {
		out = append(out, p.Bytes()...)
	}
	out = append(out, le64(uint64(len(c.proof)))...)
	return append(out, c.proof...)
}

// ParseProfileKeyCredentialRequestContext reads a stored context.
func ParseProfileKeyCredentialRequestContext(b []byte) (*ProfileKeyCredentialRequestContext, error) {
	r := &reader{b: b}
	r.byteValue(0)
	c := &ProfileKeyCredentialRequestContext{}
	copy(c.aci[:], r.take(16))
	copy(c.key[:], r.take(32))
	c.y, c.bigY, c.r1, c.r2 = r.scalar(), r.point(), r.scalar(), r.scalar()
	c.d1, c.d2, c.e1, c.e2 = r.point(), r.point(), r.point(), r.point()
	c.proof = append([]byte{}, r.bytesField()...)
	return c, r.done()
}

// Request is the 329-byte request sent to the server (§8.2).
func (c *ProfileKeyCredentialRequestContext) Request() []byte {
	out := []byte{0}
	for _, p := range []*ristretto255.Element{c.bigY, c.d1, c.d2, c.e1, c.e2} {
		out = append(out, p.Bytes()...)
	}
	out = append(out, le64(uint64(len(c.proof)))...)
	return append(out, c.proof...)
}

// ExpiringProfileKeyCredential is a received profile key credential (§8.4).
type ExpiringProfileKeyCredential struct {
	cred       zkcCredential
	aci        [16]byte
	key        [32]byte
	Expiration uint64 // seconds
}

// Receive verifies the 497-byte response at time now (seconds) (§8.4).
func (c *ProfileKeyCredentialRequestContext) Receive(p *ServerPublicParams, resp []byte, now uint64) (*ExpiringProfileKeyCredential, error) {
	r := &reader{b: resp}
	r.byteValue(0)
	t, u, s1, s2 := r.scalar(), r.point(), r.point(), r.point()
	exp := r.u64()
	proof := r.bytesField()
	if err := r.done(); err != nil {
		return nil, err
	}
	m1, m2 := uidPoints(ServiceID{UUID: c.aci})
	pts := map[string]*ristretto255.Element{
		"C_W": p.ExpiringProfileKey[0], "G_w": gz.w, "G_wprime": gz.wp, "G_V-I": sub(gz.v, p.ExpiringProfileKey[1]),
		"G_x0": gz.x0, "G_x1": gz.x1, "G_y1": gz.y[1], "G_y2": gz.y[2], "G_y3": gz.y[3], "G_y4": gz.y[4], "G_y5": gz.y[5],
		"S1": s1, "D1": c.d1, "E1": c.e1, "S2": s2, "D2": c.d2, "E2": c.e2, "Y": c.bigY, "U": u, "tU": mul(t, u),
		"M1": m1, "M2": m2, "M5": mul(m5(exp), gz.m[5]),
	}
	if !profileIssuanceStatement().verify(proof, pts, nil) {
		return nil, ErrVerification
	}
	if exp%86400 != 0 {
		return nil, ErrVerification
	}
	var rem uint64
	if exp > now {
		rem = exp - now
	}
	if days := rem / 86400; days == 0 || days > 7 {
		return nil, ErrVerification
	}
	v := sub(s2, mul(c.y, s1))
	return &ExpiringProfileKeyCredential{cred: zkcCredential{t: t, u: u, v: v}, aci: c.aci, key: c.key, Expiration: exp}, nil
}

// Serialize is the 153-byte credential.
func (e *ExpiringProfileKeyCredential) Serialize() []byte {
	out := append([]byte{0}, e.cred.bytes()...)
	out = append(out, e.aci[:]...)
	out = append(out, e.key[:]...)
	return append(out, le64(e.Expiration)...)
}

// ParseExpiringProfileKeyCredential reads a stored credential.
func ParseExpiringProfileKeyCredential(b []byte) (*ExpiringProfileKeyCredential, error) {
	r := &reader{b: b}
	r.byteValue(0)
	e := &ExpiringProfileKeyCredential{cred: zkcCredential{t: r.scalar(), u: r.point(), v: r.point()}}
	copy(e.aci[:], r.take(16))
	copy(e.key[:], r.take(32))
	e.Expiration = r.u64()
	return e, r.done()
}

// Present builds the 721-byte presentation for a group (§8.5).
func (e *ExpiringProfileKeyCredential) Present(p *ServerPublicParams, g *GroupSecretParams, randomness [32]byte) []byte {
	sho := oneShot(labelProfilePresent, randomness[:])
	z := sho.scalar()
	c := e.cred
	m1, m2 := uidPoints(ServiceID{UUID: e.aci})
	m3p, m4p := m3(e.key, e.aci), m4(e.key)
	cy1 := add(mul(z, gz.y[1]), m1)
	cy2 := add(mul(z, gz.y[2]), m2)
	cy3 := add(mul(z, gz.y[3]), m3p)
	cy4 := add(mul(z, gz.y[4]), m4p)
	cy5 := mul(z, gz.y[5])
	cx0 := add(mul(z, gz.x0), c.u)
	cx1 := add(mul(z, gz.x1), mul(c.t, c.u))
	cv := add(mul(z, gz.v), c.v)
	iP := p.ExpiringProfileKey[1]
	zP := mul(z, iP)
	z0 := negScalar(mulScalar(z, c.t))
	z1 := negScalar(mulScalar(z, g.a1))
	z2 := negScalar(mulScalar(z, g.b1))
	ea1 := mul(g.a1, m1)
	ea2 := add(mul(g.a2, ea1), m2)
	eb1 := mul(g.b1, m3p)
	eb2 := add(mul(g.b2, eb1), m4p)
	pts := map[string]*ristretto255.Element{
		"Z": zP, "I": iP, "C_x1": cx1, "C_x0": cx0, "G_x0": gz.x0, "G_x1": gz.x1,
		"A+B": add(g.aUid, g.aPk), "G_a1": gA1, "G_a2": gA2, "G_b1": gB1, "G_b2": gB2,
		"C_y2-E_A2": sub(cy2, ea2), "G_y2": gz.y[2], "-E_A1": neg(ea1), "E_A1": ea1, "C_y1": cy1, "G_y1": gz.y[1],
		"C_y4-E_B2": sub(cy4, eb2), "G_y4": gz.y[4], "-E_B1": neg(eb1), "E_B1": eb1, "C_y3": cy3, "G_y3": gz.y[3],
		"0": ristretto255.NewIdentityElement(), "C_y5": cy5, "G_y5": gz.y[5],
	}
	scalars := map[string]*edwards25519.Scalar{"z": z, "t": c.t, "z0": z0, "a1": g.a1, "a2": g.a2, "b1": g.b1, "b2": g.b2, "z1": z1, "z2": z2}
	proof := profilePresentationStatement().prove(scalars, pts, nil, sho.Squeeze(32))
	out := []byte{0x03}
	for _, q := range []*ristretto255.Element{cx0, cx1, cy1, cy2, cy3, cy4, cy5, cv} {
		out = append(out, q.Bytes()...)
	}
	out = append(out, le64(uint64(len(proof)))...)
	out = append(out, proof...)
	for _, q := range []*ristretto255.Element{ea1, ea2, eb1, eb2} {
		out = append(out, q.Bytes()...)
	}
	return append(out, le64(e.Expiration)...)
}

// Group send endorsements (§9)

func endorsementStatement() *statement {
	return newStatement().eq("sumR", "skprime", "sumE").eq("G", "skprime", "PKprime")
}

// GroupSendEndorsement is one (or a combined) endorsement point.
type GroupSendEndorsement struct{ r *ristretto255.Element }

// GroupSendEndorsements is a received response.
type GroupSendEndorsements struct {
	Expiration   uint64                 // seconds
	Endorsements []GroupSendEndorsement // in member order
}

// ErrEndorsementExpiration: the response's expiration is out of range.
var ErrEndorsementExpiration = errors.New("zkgroup: endorsement expiration out of range")

// ReceiveGroupSendEndorsements verifies a response for the members (in
// group order) at time now (seconds) (§9.4).
func ReceiveGroupSendEndorsements(p *ServerPublicParams, g *GroupSecretParams, resp []byte, members []ServiceID, now uint64) (*GroupSendEndorsements, error) {
	r := &reader{b: resp}
	r.byteValue(0)
	n := r.u64()
	if n > uint64(len(r.b))/32 {
		return nil, ErrDeserialization
	}
	raw := make([][]byte, n)
	for i := range raw {
		raw[i] = append([]byte{}, r.take(32)...)
	}
	proof := r.bytesField()
	exp := r.u64()
	if err := r.done(); err != nil {
		return nil, err
	}
	if exp%86400 != 0 {
		return nil, ErrEndorsementExpiration
	}
	var rem uint64
	if exp > now {
		rem = exp - now
	}
	if rem < 7200 || rem > 7*86400 {
		return nil, ErrEndorsementExpiration
	}
	if int(n) != len(members) {
		return nil, ErrVerification
	}
	tau := oneShot(labelEndorseTag, be64(exp)).scalar()
	pkPrime := add(p.EndorsementPublicKey, new(ristretto255.Element).ScalarBaseMult(tau))
	es := make([]*ristretto255.Element, len(members))
	dbl := make([][]byte, len(members))
	order := make([]int, len(members))
	for i, m := range members {
		es[i] = mul(g.a1, calcM1(m))
		dbl[i] = add(es[i], es[i]).Bytes()
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return bytes.Compare(dbl[order[a]], dbl[order[b]]) < 0 })
	h := NewSho([]byte(labelEndorseWeights))
	buf := append([]byte{}, pkPrime.Bytes()...)
	for _, i := range order {
		buf = append(buf, dbl[i]...)
	}
	for _, b := range raw {
		buf = append(buf, b...)
	}
	h.Absorb(buf)
	weights := h.Squeeze(16 * (len(members) - 1))
	decoded := make([]*ristretto255.Element, n)
	for j, b := range raw {
		pt, err := new(ristretto255.Element).SetCanonicalBytes(b)
		if err != nil {
			pt = ristretto255.NewIdentityElement()
		}
		decoded[j] = pt
	}
	sumE := es[order[0]]
	sumR := decoded[0]
	for j := 1; j < len(members); j++ {
		var wb [32]byte
		copy(wb[:16], weights[16*(j-1):16*j])
		wb[15] &= 0x7f
		w, _ := edwards25519.NewScalar().SetCanonicalBytes(wb[:])
		sumE = add(sumE, mul(w, es[order[j]]))
		sumR = add(sumR, mul(w, decoded[j]))
	}
	pts := map[string]*ristretto255.Element{"sumR": sumR, "sumE": sumE, "PKprime": pkPrime}
	if !endorsementStatement().verify(proof, pts, nil) {
		return nil, ErrVerification
	}
	out := &GroupSendEndorsements{Expiration: exp, Endorsements: make([]GroupSendEndorsement, len(members))}
	for j, i := range order {
		out.Endorsements[i] = GroupSendEndorsement{r: decoded[j]}
	}
	return out, nil
}

// ParseGroupSendEndorsement reads a 33-byte stored endorsement.
func ParseGroupSendEndorsement(b []byte) (GroupSendEndorsement, error) {
	r := &reader{b: b}
	r.byteValue(0)
	p := r.point()
	return GroupSendEndorsement{r: p}, r.done()
}

// Serialize is the 33-byte form (§9.6).
func (e GroupSendEndorsement) Serialize() []byte { return append([]byte{0}, e.r.Bytes()...) }

// CombineEndorsements sums endorsements (§9.5).
func CombineEndorsements(es []GroupSendEndorsement) GroupSendEndorsement {
	sum := ristretto255.NewIdentityElement()
	for _, e := range es {
		sum.Add(sum, e.r)
	}
	return GroupSendEndorsement{r: sum}
}

// Remove subtracts another endorsement.
func (e GroupSendEndorsement) Remove(o GroupSendEndorsement) GroupSendEndorsement {
	return GroupSendEndorsement{r: sub(e.r, o.r)}
}

// Token is the 25-byte GroupSendToken (§9.7).
func (e GroupSendEndorsement) Token(g *GroupSecretParams) []byte {
	pt := mul(edwards25519.NewScalar().Invert(g.a1), e.r)
	h := sha256.Sum256(pt.Bytes())
	return append(append([]byte{0}, le64(16)...), h[:16]...)
}

// FullToken is the 33-byte GroupSendFullToken.
func (e GroupSendEndorsement) FullToken(g *GroupSecretParams, expiration uint64) []byte {
	return append(e.Token(g), le64(expiration)...)
}
