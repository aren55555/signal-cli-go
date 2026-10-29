// SPDX-License-Identifier: GPL-3.0-or-later

package zkgroup

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/zkgroup_credentials.json
// (spec/crypto/zkgroup-credentials.md §14).

type credVectors struct {
	ShoSHA256 []struct{ Output string } `json:"sho_sha256"`
	Poksho    struct {
		Statements []struct{ Statement, Description string } `json:"statement_encodings"`
		Signature  struct {
			PrivateKey string `json:"private_key"`
			PublicKey  string `json:"public_key"`
			Message    string
			Randomness string
			Signature  string
		}
		Complex struct {
			A, B, H, I, Message, Randomness, Proof string
			Sa                                     string `json:"a"`
			Sb                                     string `json:"b"`
			Sc                                     string `json:"c"`
			Sd                                     string `json:"d"`
		} `json:"complex_statement"`
	}
	SystemParams struct {
		ZKCredential string `json:"zkcredential_G_w_G_wprime_G_x0_G_x1_G_V_G_z_G_y0_to_G_y6"`
		ZKGroup      string `json:"zkgroup_credentials_G_w_G_wprime_G_x0_G_x1_G_y1_to_G_y6_G_m1_to_G_m5_G_V_G_z"`
	} `json:"system_params"`
	Descriptions map[string]string `json:"statement_descriptions"`
	Server       struct {
		PublicParams string `json:"public_params"`
	}
	Groups map[string]struct {
		MasterKey string `json:"master_key"`
	}
	Notary struct {
		Message, Signature string
		Verifies           bool
	} `json:"notary_signature"`
	Auth []struct {
		Name              string
		Group             string
		ACI, PNI          string
		RedemptionTime    uint64 `json:"redemption_time"`
		PresentRandomness string `json:"present_randomness"`
		Response          string
		Credential        string
		Presentation      string
		Negative          map[string]string `json:"receive_negative"`
	} `json:"auth_credential_with_pni"`
	Profile []struct {
		Name                     string
		Group                    string
		ACI                      string
		ProfileKey               string `json:"profile_key"`
		RequestContextRandomness string `json:"request_context_randomness"`
		Expiration               uint64
		ReceiveTime              uint64 `json:"receive_time"`
		PresentRandomness        string `json:"present_randomness"`
		RequestContext           string `json:"request_context"`
		Request                  string
		Response                 string
		Credential               string
		Presentation             string
		ReceiveAt                map[string]string `json:"receive_at"`
	} `json:"expiring_profile_key_credential"`
	Endorsements []struct {
		Name                    string
		Group                   string
		Members                 []string `json:"members_aci"`
		Local                   int      `json:"local_user_index"`
		Expiration              uint64
		ReceiveTime             uint64 `json:"receive_time"`
		Response                string
		Endorsements            []string
		Tokens                  []string
		FullTokens              []string          `json:"full_tokens"`
		CombinedWithoutLocal    string            `json:"combined_without_local_user"`
		CombinedWithoutLocalTok string            `json:"combined_without_local_user_full_token"`
		CombinedAll             string            `json:"combined_all"`
		ReceiveAt               map[string]string `json:"receive_at"`
	} `json:"group_send_endorsements"`
}

func loadCreds(t *testing.T) credVectors {
	var v credVectors
	testvec.Load(t, "zkgroup_credentials.json", &v)
	return v
}

func scalarHex(t *testing.T, s string) *edwards25519.Scalar {
	k, err := edwards25519.NewScalar().SetCanonicalBytes(testvec.Hex(t, s))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func pointHex(t *testing.T, s string) *ristretto255.Element {
	p, err := new(ristretto255.Element).SetCanonicalBytes(testvec.Hex(t, s))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestShoSHA256(t *testing.T) {
	v := loadCreds(t)
	s := newShoSHA256([]byte("asd"))
	s.absorbRatchet([]byte("asdasd"))
	if got := hex.EncodeToString(s.squeeze(64)); got != v.ShoSHA256[0].Output {
		t.Errorf("64: %s", got)
	}
	s = newShoSHA256([]byte("asd"))
	s.absorbRatchet([]byte("asdasd"))
	if got := hex.EncodeToString(s.squeeze(65)); got != v.ShoSHA256[1].Output {
		t.Errorf("65: %s", got)
	}
	s = newShoSHA256(nil)
	s.absorbRatchet([]byte("abc"))
	for _, n := range []int{63, 64, 65, 127, 128, 129} {
		s.absorbRatchet(make([]byte, n))
	}
	for _, n := range []int{63, 64, 65, 127, 128, 129} {
		s.squeeze(n)
	}
	s.absorbRatchet([]byte("def"))
	if got := hex.EncodeToString(s.squeeze(63)); got != v.ShoSHA256[2].Output {
		t.Errorf("long: %s", got)
	}
}

func TestCredentialSystemParams(t *testing.T) {
	v := loadCreds(t)
	var got []byte
	for _, p := range gk.all() {
		got = append(got, p.Bytes()...)
	}
	if hex.EncodeToString(got) != v.SystemParams.ZKCredential {
		t.Errorf("zkcredential generators differ")
	}
	got = nil
	for _, p := range gz.storageOrder() {
		got = append(got, p.Bytes()...)
	}
	if hex.EncodeToString(got) != v.SystemParams.ZKGroup {
		t.Errorf("zkgroup credential generators differ")
	}
}

func TestPokshoStatements(t *testing.T) {
	v := loadCreds(t)
	want := map[string]*statement{
		"A = a*G":                            newStatement().eq("A", "a", "G"),
		"A = a*G; B = a*H":                   newStatement().eq("A", "a", "G").eq("B", "a", "H"),
		"A = a*G + b*H":                      newStatement().eq("A", "a", "G", "b", "H"),
		"A = a*G + b*H + c*I; B = c*H + d*I": newStatement().eq("A", "a", "G", "b", "H", "c", "I").eq("B", "c", "H", "d", "I"),
	}
	for _, c := range v.Poksho.Statements {
		st := want[c.Statement]
		if st == nil {
			t.Fatalf("unknown statement %q", c.Statement)
		}
		if hex.EncodeToString(st.description()) != c.Description {
			t.Errorf("%s: %x", c.Statement, st.description())
		}
	}
	// The upstream multi-equation proof.
	cx := v.Poksho.Complex
	st := want["A = a*G + b*H + c*I; B = c*H + d*I"]
	scalars := map[string]*edwards25519.Scalar{"a": scalarHex(t, cx.Sa), "b": scalarHex(t, cx.Sb), "c": scalarHex(t, cx.Sc), "d": scalarHex(t, cx.Sd)}
	points := map[string]*ristretto255.Element{"A": pointHex(t, cx.A), "B": pointHex(t, cx.B), "H": pointHex(t, cx.H), "I": pointHex(t, cx.I)}
	proof := st.prove(scalars, points, testvec.Hex(t, cx.Message), testvec.Hex(t, cx.Randomness))
	if hex.EncodeToString(proof) != cx.Proof {
		t.Errorf("complex proof %x", proof)
	}
	if !st.verify(proof, points, testvec.Hex(t, cx.Message)) {
		t.Error("complex proof does not verify")
	}
	points["A"] = pointHex(t, cx.B)
	if st.verify(proof, points, testvec.Hex(t, cx.Message)) {
		t.Error("proof verifies for a wrong point")
	}
	// The upstream signature.
	sg := v.Poksho.Signature
	sigSt := newStatement().eq("public_key", "private_key", "G")
	sig := sigSt.prove(map[string]*edwards25519.Scalar{"private_key": scalarHex(t, sg.PrivateKey)},
		map[string]*ristretto255.Element{"public_key": pointHex(t, sg.PublicKey)}, testvec.Hex(t, sg.Message), testvec.Hex(t, sg.Randomness))
	if hex.EncodeToString(sig) != sg.Signature {
		t.Errorf("signature %x", sig)
	}
}

func TestStatementDescriptions(t *testing.T) {
	v := loadCreds(t)
	cases := map[string]*statement{
		"zkcredential_issuance_5_attribute_points":     issuanceStatement(5),
		"auth_credential_with_pni_presentation":        authPresentationStatement(),
		"profile_key_credential_request":               profileRequestStatement(),
		"expiring_profile_key_credential_issuance":     profileIssuanceStatement(),
		"expiring_profile_key_credential_presentation": profilePresentationStatement(),
		"endorsement_response":                         endorsementStatement(),
		"notary_signature":                             newStatement().eq("public_key", "private_key", "G"),
	}
	for name, st := range cases {
		if got := hex.EncodeToString(st.description()); got != v.Descriptions[name] {
			t.Errorf("%s: %s\nwant %s", name, got, v.Descriptions[name])
		}
	}
}

func credServer(t *testing.T, v credVectors) *ServerPublicParams {
	p, err := ParseServerPublicParams(testvec.Hex(t, v.Server.PublicParams))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func credGroup(t *testing.T, v credVectors, name string) *GroupSecretParams {
	return DeriveGroupSecretParams(key32(t, v.Groups[name].MasterKey))
}

func TestNotaryVectorServer(t *testing.T) {
	v := loadCreds(t)
	p := credServer(t, v)
	if got := VerifyNotarySignature(p.SigPublicKey, testvec.Hex(t, v.Notary.Message), testvec.Hex(t, v.Notary.Signature)); got != v.Notary.Verifies {
		t.Errorf("notary %v", got)
	}
}

func TestAuthCredentialWithPni(t *testing.T) {
	v := loadCreds(t)
	p := credServer(t, v)
	for _, c := range v.Auth {
		g := credGroup(t, v, c.Group)
		aci, pni := uuid16(t, c.ACI), uuid16(t, c.PNI)
		cred, err := ReceiveAuthCredentialWithPni(p, aci, pni, c.RedemptionTime, testvec.Hex(t, c.Response))
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if hex.EncodeToString(cred.Serialize()) != c.Credential {
			t.Errorf("%s: credential %x", c.Name, cred.Serialize())
		}
		pres := cred.Present(p, g, key32(t, c.PresentRandomness))
		if hex.EncodeToString(pres) != c.Presentation {
			t.Errorf("%s: presentation %x", c.Name, pres)
		}
		for neg := range c.Negative {
			a, n, T := aci, pni, c.RedemptionTime
			switch neg {
			case "aci_00000000-0000-4000-8000-000000000001":
				a = uuid16(t, "00000000-0000-4000-8000-000000000001")
			case "aci_and_pni_swapped":
				a, n = pni, aci
			case "redemption_time_plus_1":
				T++
			case "redemption_time_plus_86400":
				T += 86400
			}
			if _, err := ReceiveAuthCredentialWithPni(p, a, n, T, testvec.Hex(t, c.Response)); err == nil {
				t.Errorf("%s: %s accepted", c.Name, neg)
			}
		}
	}
}

func TestExpiringProfileKeyCredential(t *testing.T) {
	v := loadCreds(t)
	p := credServer(t, v)
	for _, c := range v.Profile {
		g := credGroup(t, v, c.Group)
		aci, key := uuid16(t, c.ACI), key32(t, c.ProfileKey)
		ctx := NewProfileKeyCredentialRequestContext(aci, key, key32(t, c.RequestContextRandomness))
		if hex.EncodeToString(ctx.Serialize()) != c.RequestContext {
			t.Errorf("%s: context %x", c.Name, ctx.Serialize())
		}
		if hex.EncodeToString(ctx.Request()) != c.Request {
			t.Errorf("%s: request %x", c.Name, ctx.Request())
		}
		ctx2, err := ParseProfileKeyCredentialRequestContext(ctx.Serialize())
		if err != nil || !bytes.Equal(ctx2.Serialize(), ctx.Serialize()) {
			t.Errorf("%s: context round trip %v", c.Name, err)
		}
		cred, err := ctx.Receive(p, testvec.Hex(t, c.Response), c.ReceiveTime)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if hex.EncodeToString(cred.Serialize()) != c.Credential {
			t.Errorf("%s: credential %x", c.Name, cred.Serialize())
		}
		if cred2, err := ParseExpiringProfileKeyCredential(cred.Serialize()); err != nil || !bytes.Equal(cred2.Serialize(), cred.Serialize()) {
			t.Errorf("%s: credential round trip %v", c.Name, err)
		}
		pres := cred.Present(p, g, key32(t, c.PresentRandomness))
		if hex.EncodeToString(pres) != c.Presentation {
			t.Errorf("%s: presentation %x", c.Name, pres)
		}
		for at, want := range c.ReceiveAt {
			now, _ := strconv.ParseUint(at, 10, 64)
			_, err := ctx.Receive(p, testvec.Hex(t, c.Response), now)
			if (err == nil) != (want == "accepted") {
				t.Errorf("%s: receive at %d: %v, want %s", c.Name, now, err, want)
			}
		}
	}
}

func TestGroupSendEndorsements(t *testing.T) {
	v := loadCreds(t)
	p := credServer(t, v)
	for _, c := range v.Endorsements {
		g := credGroup(t, v, c.Group)
		var members []ServiceID
		for _, m := range c.Members {
			members = append(members, ServiceID{UUID: uuid16(t, m)})
		}
		res, err := ReceiveGroupSendEndorsements(p, g, testvec.Hex(t, c.Response), members, c.ReceiveTime)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if res.Expiration != c.Expiration || len(res.Endorsements) != len(c.Members) {
			t.Fatalf("%s: expiration %d, %d endorsements", c.Name, res.Expiration, len(res.Endorsements))
		}
		for i := range c.Members {
			e := res.Endorsements[i]
			if hex.EncodeToString(e.Serialize()) != c.Endorsements[i] {
				t.Errorf("%s: endorsement %d %x", c.Name, i, e.Serialize())
			}
			if hex.EncodeToString(e.Token(g)) != c.Tokens[i] {
				t.Errorf("%s: token %d %x", c.Name, i, e.Token(g))
			}
			if hex.EncodeToString(e.FullToken(g, res.Expiration)) != c.FullTokens[i] {
				t.Errorf("%s: full token %d", c.Name, i)
			}
		}
		var others []GroupSendEndorsement
		for i, e := range res.Endorsements {
			if i != c.Local {
				others = append(others, e)
			}
		}
		comb := CombineEndorsements(others)
		if hex.EncodeToString(comb.Serialize()) != c.CombinedWithoutLocal {
			t.Errorf("%s: combined %x", c.Name, comb.Serialize())
		}
		if hex.EncodeToString(comb.FullToken(g, res.Expiration)) != c.CombinedWithoutLocalTok {
			t.Errorf("%s: combined full token", c.Name)
		}
		all := CombineEndorsements(res.Endorsements)
		if hex.EncodeToString(all.Serialize()) != c.CombinedAll {
			t.Errorf("%s: combined all", c.Name)
		}
		if hex.EncodeToString(all.Remove(res.Endorsements[c.Local]).Serialize()) != c.CombinedWithoutLocal {
			t.Errorf("%s: remove", c.Name)
		}
		for at, want := range c.ReceiveAt {
			now, _ := strconv.ParseUint(at, 10, 64)
			_, err := ReceiveGroupSendEndorsements(p, g, testvec.Hex(t, c.Response), members, now)
			if (err == nil) != (want == "accepted") {
				t.Errorf("%s: receive at %d: %v, want %s", c.Name, now, err, want)
			}
		}
		if len(members) > 1 {
			swapped := append([]ServiceID{}, members...)
			swapped[0].UUID[0] ^= 1
			if _, err := ReceiveGroupSendEndorsements(p, g, testvec.Hex(t, c.Response), swapped, c.ReceiveTime); err == nil {
				t.Errorf("%s: wrong member accepted", c.Name)
			}
		}
	}
}
