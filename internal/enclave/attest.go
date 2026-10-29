// SPDX-License-Identifier: GPL-3.0-or-later

package enclave

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// SGX DCAP attestation as libsignal verifies it
// (spec/enclave/attestation.md §3-§8).

// DataError is an attestation-data error (malformed input); other
// failures are *FailedError (§6 "Error classes").
type DataError struct{ Msg string }

func (e *DataError) Error() string { return e.Msg }

// FailedError is a failed attestation.
type FailedError struct{ Msg string }

func (e *FailedError) Error() string { return e.Msg }

func failf(f string, args ...any) error { return &FailedError{fmt.Sprintf(f, args...)} }

// PinnedRootKey is Intel's SGX root public key (§8).
var PinnedRootKey, _ = hex.DecodeString("040ba9c4c0c0c86193a3fe23d6b02cda10a8bbd4e88e48b4458561a36e705525f567918e2edc88e40d860bd0cc4ee26aacc988e505a953558c453f6b0904ae7394")

var intelQEVendor, _ = hex.DecodeString("939a7233f79c4ca9940a0db3957f0607")

// AcceptedAdvisories is the advisory list of every known enclave (§7.1).
var AcceptedAdvisories = []string{"INTEL-SA-00615", "INTEL-SA-00657"}

// Known MRENCLAVEs (§7.1); unknown ones get no advisories.
var KnownEnclaves = map[string]bool{
	"6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a": true, // CDSI staging
	"15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b": true, // CDSI prod
	"3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302": true, // SVR2 2026Q2 staging
	"97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535": true, // SVR2 2026Q1 staging
	"ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4": true,
	"1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708": true,
	"29cd63c87bea751e3bfd0fbd401279192e2e5c99948b4ee9437eafc4968355fb": true,
}

const minTCBEvaluationDataNumber = 21

// ParseHandshakeStart reads evidence (field 2) and endorsements (field 3)
// from the server's first frame (§3).
func ParseHandshakeStart(b []byte) (evidence, endorsements []byte, err error) {
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return nil, nil, &DataError{"Protobuf decode failed"}
		}
		b = b[l:]
		vl := protowire.ConsumeFieldValue(n, typ, b)
		if vl < 0 {
			return nil, nil, &DataError{"Protobuf decode failed"}
		}
		if typ == protowire.BytesType && (n == 2 || n == 3) {
			v, _ := protowire.ConsumeBytes(b)
			if n == 2 {
				evidence = v
			} else {
				endorsements = v
			}
		}
		b = b[vl:]
	}
	return evidence, endorsements, nil
}

// --- evidence ------------------------------------------------------------

type reportBody struct {
	raw        []byte // 384 bytes
	miscselect uint32
	attributes []byte // 16
	mrenclave  []byte
	mrsigner   []byte
	isvprodid  uint16
	isvsvn     uint16
	reportData []byte // 64
}

func parseReport(b []byte) reportBody {
	return reportBody{raw: b, miscselect: binary.LittleEndian.Uint32(b[16:]), attributes: b[48:64], mrenclave: b[64:96],
		mrsigner: b[128:160], isvprodid: binary.LittleEndian.Uint16(b[256:]), isvsvn: binary.LittleEndian.Uint16(b[258:]), reportData: b[320:384]}
}

type evidence struct {
	signed     []byte // first 432 bytes
	isv        reportBody
	isvSig     []byte
	attestKey  []byte // x‖y
	qe         reportBody
	qeSig      []byte
	qeAuth     []byte
	pckChain   []*x509.Certificate
	claimsBlob []byte
	claims     map[string][]byte
}

func parseEvidence(b []byte) (*evidence, error) {
	if len(b) == 0 {
		return nil, &DataError{"Evidence does not fit expected format"}
	}
	if len(b) < 432 {
		return nil, failf("incorrect buffer size")
	}
	if v := binary.LittleEndian.Uint16(b); v != 3 {
		return nil, failf("unsupported quote version %d", v)
	}
	if binary.LittleEndian.Uint16(b[2:]) != 2 {
		return nil, failf("unsupported attestation key type")
	}
	e := &evidence{signed: b[:432], isv: parseReport(b[48:432])}
	if !bytes.Equal(b[12:28], intelQEVendor) {
		return nil, failf("QE vendor ID is not Intel's")
	}
	rest := b[432:]
	if len(rest) < 4 {
		return nil, failf("buffer underflow")
	}
	l := binary.LittleEndian.Uint32(rest)
	rest = rest[4:]
	if uint64(len(rest)) < uint64(l) || len(rest) < 578 {
		return nil, failf("buffer underflow")
	}
	e.isvSig, e.attestKey = rest[0:64], rest[64:128]
	e.qe, e.qeSig = parseReport(rest[128:512]), rest[512:576]
	a := int(binary.LittleEndian.Uint16(rest[576:]))
	rest = rest[578:]
	if len(rest) < a+6 {
		return nil, failf("buffer underflow")
	}
	e.qeAuth = rest[:a]
	rest = rest[a:]
	if binary.LittleEndian.Uint16(rest) != 5 {
		return nil, failf("unsupported certification key type")
	}
	c := int(binary.LittleEndian.Uint32(rest[2:]))
	rest = rest[6:]
	if len(rest) < c {
		return nil, failf("buffer underflow")
	}
	chain, err := parseChain(rest[:c])
	if err != nil {
		return nil, err
	}
	e.pckChain = chain
	e.claimsBlob = rest[c:]
	if e.claims, err = parseClaims(e.claimsBlob); err != nil {
		return nil, err
	}
	return e, nil
}

func parseClaims(b []byte) (map[string][]byte, error) {
	if len(b) < 16 || binary.LittleEndian.Uint64(b) != 1 {
		return nil, failf("unsupported custom claims version")
	}
	count := binary.LittleEndian.Uint64(b[8:])
	if count > 256 {
		return nil, failf("too many custom claims")
	}
	b = b[16:]
	out := map[string][]byte{}
	for range count {
		if len(b) < 16 {
			return nil, failf("underflow")
		}
		ns, vs := binary.LittleEndian.Uint64(b), binary.LittleEndian.Uint64(b[8:])
		b = b[16:]
		if ns > 1024 || vs > 1<<20 || ns+vs > uint64(len(b)) {
			return nil, failf("underflow")
		}
		name := b[:ns]
		val := b[ns : ns+vs]
		b = b[ns+vs:]
		name = bytes.TrimSuffix(name, []byte{0})
		if !utf8.Valid(name) {
			return nil, failf("claim name is not UTF-8")
		}
		out[string(name)] = val
	}
	if len(b) != 0 {
		return nil, failf("unexpected data after custom claims")
	}
	return out, nil
}

// parseChain reads every PEM certificate and orders the chain leaf →
// root (§5.1 "PEM chains").
func parseChain(b []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for {
		var blk *pem.Block
		blk, b = pem.Decode(b)
		if blk == nil {
			break
		}
		if blk.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			return nil, failf("invalid certificate: %v", err)
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, failf("Invalid certificate chain")
	}
	root := -1
	for i, c := range certs {
		if bytes.Equal(c.RawIssuer, c.RawSubject) && c.CheckSignatureFrom(c) == nil {
			root = i
		}
	}
	if root < 0 {
		return nil, failf("Invalid certificate chain")
	}
	chain := []*x509.Certificate{certs[root]}
	used := map[int]bool{root: true}
	for len(chain) < len(certs) {
		parent := chain[0]
		next := -1
		for i, c := range certs {
			if !used[i] && bytes.Equal(c.RawIssuer, parent.RawSubject) && c.CheckSignatureFrom(parent) == nil {
				if next >= 0 {
					return nil, failf("Invalid certificate chain")
				}
				next = i
			}
		}
		if next < 0 {
			return nil, failf("Invalid certificate chain")
		}
		used[next] = true
		chain = append([]*x509.Certificate{certs[next]}, chain...)
	}
	return chain, nil
}

// --- endorsements --------------------------------------------------------

type tcbLevel struct {
	comps    [16]uint8
	pcesvn   uint16
	status   string
	advisory []string
}

type tcbInfo struct {
	nextUpdate time.Time
	fmspc      []byte
	pceID      []byte
	evalNumber int
	levels     []tcbLevel
}

type qeIdentity struct {
	id                         string
	nextUpdate                 time.Time
	miscselect, miscselectMask uint32
	attributes, attributesMask []byte
	mrsigner                   []byte
	isvprodid                  uint16
	levels                     []struct {
		isvsvn uint16
		status string
	}
}

type endorsements struct {
	tcb             *tcbInfo
	tcbChain        []*x509.Certificate
	pckCRL, rootCRL *x509.RevocationList
	crlChain        []*x509.Certificate
	qe              *qeIdentity
	qeChain         []*x509.Certificate
}

func hexBytes(s string, n int) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil || (n > 0 && len(b) != n) {
		return nil, failf("bad hex field")
	}
	return b, nil
}

// verifySignedJSON checks the signature over the exact bytes of field
// and returns those bytes (§5.2).
func verifySignedJSON(doc []byte, field string, key *x509.Certificate) (json.RawMessage, error) {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSuffix(doc, []byte{0}), &outer); err != nil {
		return nil, failf("invalid %s JSON", field)
	}
	body := outer[field]
	var sigHex string
	if json.Unmarshal(outer["signature"], &sigHex) != nil {
		return nil, failf("missing signature")
	}
	sig, err := hexBytes(sigHex, 64)
	if err != nil {
		return nil, err
	}
	if !verifyRaw(key.PublicKey, body, sig) {
		return nil, failf("%s signature is invalid", field)
	}
	return body, nil
}

func verifyRaw(pub any, msg, sig []byte) bool {
	k, ok := pub.(*ecdsa.PublicKey)
	if !ok || len(sig) != 64 {
		return false
	}
	h := sha256.Sum256(msg)
	return ecdsa.Verify(k, h[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]))
}

func parseCRL(b []byte) (*x509.RevocationList, error) {
	crl, err := x509.ParseRevocationList(bytes.TrimSuffix(b, []byte{0}))
	if err != nil {
		return nil, failf("invalid CRL: %v", err)
	}
	if len(crl.AuthorityKeyId) == 0 || crl.Number == nil {
		return nil, failf("CRL missing required extension")
	}
	return crl, nil
}

func parseEndorsements(b []byte) (*endorsements, error) {
	if len(b) == 0 {
		return nil, &DataError{"Endorsements do not fit expected format"}
	}
	if len(b) < 16 || binary.LittleEndian.Uint32(b) != 1 || binary.LittleEndian.Uint32(b[4:]) != 2 {
		return nil, failf("unsupported endorsements")
	}
	n := int(binary.LittleEndian.Uint32(b[12:]))
	if n < 9 || len(b) < 16+4*n {
		return nil, failf("endorsements underflow")
	}
	data := b[16+4*n:]
	offs := make([]int, n)
	for i := range offs {
		offs[i] = int(binary.LittleEndian.Uint32(b[16+4*i:]))
		if (i > 0 && offs[i] <= offs[i-1]) || offs[i] >= len(data) {
			return nil, failf("invalid endorsement offsets")
		}
	}
	field := func(i int) []byte {
		end := len(data)
		if i+1 < n {
			end = offs[i+1]
		}
		return data[offs[i]:end]
	}
	if f := field(0); len(f) != 4 || binary.LittleEndian.Uint32(f) != 1 {
		return nil, failf("unsupported endorsements version")
	}
	e := &endorsements{}
	var err error
	if e.tcbChain, err = parseChain(field(2)); err != nil {
		return nil, err
	}
	if e.pckCRL, err = parseCRL(field(3)); err != nil {
		return nil, err
	}
	if e.rootCRL, err = parseCRL(field(4)); err != nil {
		return nil, err
	}
	if e.crlChain, err = parseChain(field(5)); err != nil {
		return nil, err
	}
	if e.qeChain, err = parseChain(field(7)); err != nil {
		return nil, err
	}
	tcbRaw, err := verifySignedJSON(field(1), "tcbInfo", e.tcbChain[0])
	if err != nil {
		return nil, err
	}
	if e.tcb, err = parseTCBInfo(tcbRaw); err != nil {
		return nil, err
	}
	qeRaw, err := verifySignedJSON(field(6), "enclaveIdentity", e.qeChain[0])
	if err != nil {
		return nil, err
	}
	if e.qe, err = parseQEIdentity(qeRaw); err != nil {
		return nil, err
	}
	if !utf8.Valid(bytes.TrimSuffix(field(8), []byte{0})) {
		return nil, failf("creation datetime is not UTF-8")
	}
	return e, nil
}

var tcbStatuses = map[string]bool{"UpToDate": true, "OutOfDate": true, "ConfigurationNeeded": true, "SWHardeningNeeded": true,
	"ConfigurationAndSWHardeningNeeded": true, "OutOfDateConfigurationNeeded": true, "Revoked": true}

func parseTCBInfo(raw []byte) (*tcbInfo, error) {
	var j struct {
		Version    int    `json:"version"`
		NextUpdate string `json:"nextUpdate"`
		FMSPC      string `json:"fmspc"`
		PCEID      string `json:"pceId"`
		TCBType    int    `json:"tcbType"`
		Eval       int    `json:"tcbEvaluationDataNumber"`
		Levels     []struct {
			TCB        map[string]json.RawMessage `json:"tcb"`
			Status     string                     `json:"tcbStatus"`
			Advisories []string                   `json:"advisoryIDs"`
		} `json:"tcbLevels"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, failf("invalid tcbInfo: %v", err)
	}
	if j.Version != 2 && j.Version != 3 {
		return nil, failf("unsupported tcbInfo version %d", j.Version)
	}
	if j.TCBType != 0 {
		return nil, failf("unsupported tcb type")
	}
	t := &tcbInfo{evalNumber: j.Eval}
	var err error
	if t.nextUpdate, err = time.Parse(time.RFC3339, j.NextUpdate); err != nil {
		return nil, failf("invalid nextUpdate")
	}
	if t.fmspc, err = hexBytes(j.FMSPC, 6); err != nil {
		return nil, err
	}
	if t.pceID, err = hexBytes(j.PCEID, 2); err != nil {
		return nil, err
	}
	for _, l := range j.Levels {
		if !tcbStatuses[l.Status] {
			return nil, failf("unknown tcb status %q", l.Status)
		}
		lv := tcbLevel{status: l.Status, advisory: l.Advisories}
		if err := json.Unmarshal(l.TCB["pcesvn"], &lv.pcesvn); err != nil {
			return nil, failf("missing pcesvn")
		}
		if j.Version == 3 {
			var comps []struct {
				SVN uint8 `json:"svn"`
			}
			if raw, ok := l.TCB["sgxtcbcomponents"]; !ok || json.Unmarshal(raw, &comps) != nil || len(comps) != 16 {
				return nil, failf("mismatched tcb info versions")
			}
			for i, c := range comps {
				lv.comps[i] = c.SVN
			}
		} else {
			for i := range 16 {
				raw, ok := l.TCB[fmt.Sprintf("sgxtcbcomp%02dsvn", i+1)]
				if !ok || json.Unmarshal(raw, &lv.comps[i]) != nil {
					return nil, failf("mismatched tcb info versions")
				}
			}
		}
		t.levels = append(t.levels, lv)
	}
	return t, nil
}

func parseQEIdentity(raw []byte) (*qeIdentity, error) {
	var j struct {
		ID             string `json:"id"`
		Version        int    `json:"version"`
		NextUpdate     string `json:"nextUpdate"`
		Eval           *int   `json:"tcbEvaluationDataNumber"`
		Misc           string `json:"miscselect"`
		MiscMask       string `json:"miscselectMask"`
		Attributes     string `json:"attributes"`
		AttributesMask string `json:"attributesMask"`
		MRSigner       string `json:"mrsigner"`
		ISVProdID      uint16 `json:"isvprodid"`
		Levels         []struct {
			TCB struct {
				ISVSVN uint16 `json:"isvsvn"`
			} `json:"tcb"`
			Status string `json:"tcbStatus"`
		} `json:"tcbLevels"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, failf("invalid enclaveIdentity: %v", err)
	}
	if j.Version != 2 || j.Eval == nil {
		return nil, failf("unsupported enclaveIdentity")
	}
	q := &qeIdentity{id: j.ID, isvprodid: j.ISVProdID}
	var err error
	if q.nextUpdate, err = time.Parse(time.RFC3339, j.NextUpdate); err != nil {
		return nil, failf("invalid nextUpdate")
	}
	u32 := func(s string) (uint32, error) {
		b, err := hexBytes(s, 4)
		if err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint32(b), nil // libsignal's byte order (§5.3)
	}
	if q.miscselect, err = u32(j.Misc); err != nil {
		return nil, err
	}
	if q.miscselectMask, err = u32(j.MiscMask); err != nil {
		return nil, err
	}
	if q.attributes, err = hexBytes(j.Attributes, 16); err != nil {
		return nil, err
	}
	if q.attributesMask, err = hexBytes(j.AttributesMask, 16); err != nil {
		return nil, err
	}
	if q.mrsigner, err = hexBytes(j.MRSigner, 32); err != nil {
		return nil, err
	}
	for _, l := range j.Levels {
		switch l.Status {
		case "UpToDate", "OutOfDate", "Revoked":
		default:
			return nil, failf("unknown QE tcb status %q", l.Status)
		}
		q.levels = append(q.levels, struct {
			isvsvn uint16
			status string
		}{l.TCB.ISVSVN, l.Status})
	}
	return q, nil
}

// --- PCK extension ---------------------------------------------------------

var sgxExtOID = asn1.ObjectIdentifier{1, 2, 840, 113741, 1, 13, 1}

type pckExtension struct {
	comps  [16]uint8
	pcesvn uint16
	pceID  []byte
	fmspc  []byte
}

type extEntry struct {
	ID    asn1.ObjectIdentifier
	Value asn1.RawValue
}

func parsePCKExtension(c *x509.Certificate) (*pckExtension, error) {
	var raw []byte
	for _, e := range c.Extensions {
		if e.Id.Equal(sgxExtOID) {
			raw = e.Value
		}
	}
	if raw == nil {
		return nil, failf("PCK certificate is missing SGX extension")
	}
	var top []extEntry
	if _, err := asn1.Unmarshal(raw, &top); err != nil {
		return nil, failf("invalid SGX extension: %v", err)
	}
	p := &pckExtension{}
	seen := map[int]bool{}
	for _, e := range top {
		if len(e.ID) != len(sgxExtOID)+1 || !e.ID[:len(sgxExtOID)].Equal(sgxExtOID) {
			return nil, failf("unexpected extension")
		}
		k := e.ID[len(sgxExtOID)]
		if seen[k] || k < 1 || k > 7 {
			return nil, failf("unexpected extension")
		}
		seen[k] = true
		switch k {
		case 2:
			var tcb []extEntry
			if _, err := asn1.Unmarshal(e.Value.FullBytes, &tcb); err != nil || len(tcb) != 18 {
				return nil, failf("invalid TCB extension")
			}
			for _, t := range tcb {
				idx := t.ID[len(t.ID)-1]
				var v int64
				switch {
				case idx >= 1 && idx <= 16:
					if _, err := asn1.Unmarshal(t.Value.FullBytes, &v); err != nil || v < 0 || v > 255 {
						return nil, failf("invalid COMPSVN")
					}
					p.comps[idx-1] = uint8(v)
				case idx == 17:
					if _, err := asn1.Unmarshal(t.Value.FullBytes, &v); err != nil || v < 0 || v > 65535 {
						return nil, failf("invalid PCESVN")
					}
					p.pcesvn = uint16(v)
				}
			}
		case 3:
			if _, err := asn1.Unmarshal(e.Value.FullBytes, &p.pceID); err != nil || len(p.pceID) != 2 {
				return nil, failf("invalid PCE-ID")
			}
		case 4:
			if _, err := asn1.Unmarshal(e.Value.FullBytes, &p.fmspc); err != nil || len(p.fmspc) != 6 {
				return nil, failf("invalid FMSPC")
			}
		}
	}
	if len(seen) != 7 {
		return nil, failf("missing SGX extension field")
	}
	return p, nil
}

// --- verification ------------------------------------------------------------

func validAt(chain []*x509.Certificate, t time.Time) bool {
	for _, c := range chain {
		if t.Before(c.NotBefore) || t.After(c.NotAfter) {
			return false
		}
	}
	return true
}

func revoked(c *x509.Certificate, crl *x509.RevocationList) bool {
	for _, r := range crl.RevokedCertificateEntries {
		if r.SerialNumber.Cmp(c.SerialNumber) == 0 {
			return true
		}
	}
	return false
}

// checkChain validates chain (leaf → root) to the given root at t, with
// each non-root certificate covered by its issuer's CRL (§6 step 3).
func checkChain(chain []*x509.Certificate, root *x509.Certificate, crls []*x509.RevocationList, t time.Time) error {
	roots := x509.NewCertPool()
	roots.AddCert(root)
	inter := x509.NewCertPool()
	for _, c := range chain[1:] {
		if !c.Equal(root) {
			inter.AddCert(c)
		}
	}
	if _, err := chain[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: t, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return failf("certificate chain does not verify: %v", err)
	}
	for i, c := range chain[:len(chain)-1] {
		issuer := chain[i+1]
		var covering *x509.RevocationList
		for _, crl := range crls {
			if bytes.Equal(crl.RawIssuer, issuer.RawSubject) && crl.CheckSignatureFrom(issuer) == nil {
				covering = crl
			}
		}
		if covering == nil || t.Before(covering.ThisUpdate) || !t.Before(covering.NextUpdate) {
			return failf("no valid CRL for %s", c.Subject.CommonName)
		}
		if revoked(c, covering) {
			return failf("certificate %s is revoked", c.Subject.CommonName)
		}
	}
	return nil
}

func pinnedKey() *ecdsa.PublicKey {
	x, y := elliptic.Unmarshal(elliptic.P256(), PinnedRootKey)
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
}

// Verify checks evidence and endorsements for the expected MRENCLAVE at
// now and returns the claims (§6).
func Verify(evidenceBytes, endorsementBytes, mrenclave []byte, now time.Time) (map[string][]byte, error) {
	if len(mrenclave) != 32 {
		return nil, &DataError{"MREnclave value does not fit expected format"}
	}
	ev, err := parseEvidence(evidenceBytes)
	if err != nil {
		return nil, err
	}
	en, err := parseEndorsements(endorsementBytes)
	if err != nil {
		return nil, err
	}
	t := now.Add(24 * time.Hour)
	// 2. Validity windows.
	if !validAt(ev.pckChain, t) {
		return nil, failf("(evidence) attestation is not valid for %d", t.Unix())
	}
	if !validAt(en.tcbChain, t) || !validAt(en.crlChain, t) || !validAt(en.qeChain, t) ||
		t.After(en.tcb.nextUpdate) || en.tcb.evalNumber < minTCBEvaluationDataNumber || t.After(en.qe.nextUpdate) ||
		!t.Before(en.pckCRL.NextUpdate) || !t.Before(en.rootCRL.NextUpdate) {
		return nil, failf("(endorsements) attestation is not valid for %d", t.Unix())
	}
	// 3. Certificates and revocation.
	root := en.tcbChain[len(en.tcbChain)-1]
	pk, ok := root.PublicKey.(*ecdsa.PublicKey)
	if !ok || !pk.Equal(pinnedKey()) || !bytes.Equal(root.RawIssuer, root.RawSubject) || root.CheckSignatureFrom(root) != nil {
		return nil, failf("root certificate does not match the pinned Intel key")
	}
	if en.rootCRL.CheckSignatureFrom(root) != nil {
		return nil, failf("root CRL signature is invalid")
	}
	crlChain := append(append([]*x509.Certificate{}, en.crlChain[:len(en.crlChain)-1]...), root)
	if err := checkChain(crlChain, root, []*x509.RevocationList{en.rootCRL, en.pckCRL}, t); err != nil {
		return nil, err
	}
	if !bytes.Equal(en.pckCRL.RawIssuer, en.crlChain[0].RawSubject) || en.pckCRL.CheckSignatureFrom(en.crlChain[0]) != nil {
		return nil, failf("PCK CRL is not issued by the PCK CRL issuer")
	}
	crls := []*x509.RevocationList{en.rootCRL, en.pckCRL}
	for _, ch := range [][]*x509.Certificate{en.tcbChain, ev.pckChain, en.qeChain} {
		full := append(append([]*x509.Certificate{}, ch[:len(ch)-1]...), root)
		if err := checkChain(full, root, crls, t); err != nil {
			return nil, err
		}
	}
	// 4. Quoting enclave identity.
	q := en.qe
	if !bytes.Equal(ev.qe.mrsigner, q.mrsigner) || ev.qe.isvprodid != q.isvprodid || ev.qe.miscselect&q.miscselectMask != q.miscselect || q.id != "QE" {
		return nil, failf("quoting enclave does not match the QE identity")
	}
	for i := range 16 {
		if ev.qe.attributes[i]&q.attributesMask[i] != q.attributes[i] {
			return nil, failf("quoting enclave attributes do not match")
		}
	}
	qeStatus := "Revoked"
	for _, l := range q.levels {
		if l.isvsvn <= ev.qe.isvsvn {
			qeStatus = l.status
			break
		}
	}
	if qeStatus != "UpToDate" {
		return nil, failf("Enclave version tcb not up to date")
	}
	// 5. Signatures.
	if !verifyRaw(ev.pckChain[0].PublicKey, ev.qe.raw, ev.qeSig) {
		return nil, failf("QE report signature is invalid")
	}
	h := sha256.Sum256(append(append([]byte{}, ev.attestKey...), ev.qeAuth...))
	if !bytes.Equal(ev.qe.reportData[:32], h[:]) || !allZero(ev.qe.reportData[32:]) {
		return nil, failf("QE report data does not bind the attestation key")
	}
	ak := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(ev.attestKey[:32]), Y: new(big.Int).SetBytes(ev.attestKey[32:])}
	if !verifyRaw(ak, ev.signed, ev.isvSig) {
		return nil, failf("quote signature is invalid")
	}
	// 6. Platform TCB level.
	pck, err := parsePCKExtension(ev.pckChain[0])
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(pck.fmspc, en.tcb.fmspc) || !bytes.Equal(pck.pceID, en.tcb.pceID) {
		return nil, failf("PCK FMSPC/PCE-ID do not match the TCB info")
	}
	var level *tcbLevel
	for i := range en.tcb.levels {
		l := &en.tcb.levels[i]
		ok := pck.pcesvn >= l.pcesvn
		for j := range 16 {
			ok = ok && pck.comps[j] >= l.comps[j]
		}
		if ok {
			level = l
			break
		}
	}
	if level == nil {
		return nil, failf("Unsupported TCB in pck extension")
	}
	var advisories []string
	switch level.status {
	case "UpToDate":
	case "SWHardeningNeeded":
		advisories = level.advisory
	default:
		return nil, failf("invalid tcb status")
	}
	// 8. Claims binding.
	rd := ev.isv.reportData
	if !allZero(rd[32:]) {
		return nil, failf("report data hash had unexpected data")
	}
	ch := sha256.Sum256(ev.claimsBlob)
	if !bytes.Equal(rd[:32], ch[:]) || allZero(ch[:]) {
		return nil, failf("report data does not bind the custom claims")
	}
	// 9. Debug.
	if binary.LittleEndian.Uint64(ev.isv.attributes)&0x02 != 0 {
		return nil, failf("Application enclave in debug mode")
	}
	// 10. Advisories, then 11. MRENCLAVE.
	var accepted []string
	if KnownEnclaves[hex.EncodeToString(mrenclave)] {
		accepted = AcceptedAdvisories
	}
	var bad []string
	for _, a := range advisories {
		if !contains(accepted, a) {
			bad = append(bad, `"`+a+`"`)
		}
	}
	if len(bad) > 0 {
		return nil, failf("TCB contains unmitigated unaccepted advisory ids: [%s]", strings.Join(bad, ", "))
	}
	if !bytes.Equal(ev.isv.mrenclave, mrenclave) {
		return nil, failf("expected mrenclave %x, was %x", mrenclave, ev.isv.mrenclave)
	}
	return ev.claims, nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
