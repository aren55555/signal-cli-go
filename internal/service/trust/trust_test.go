// SPDX-License-Identifier: GPL-3.0-or-later

package trust

import (
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"testing"
	"time"
)

// Facts from spec/service/transport.md §3.3 (the two roots compiled into
// libsignal) and §5 (signal-cli's REST truststore).
var rootFacts = []struct {
	der              []byte
	sha256, spki     string
	serial           string
	notBefore, until string
	ed25519          bool
}{
	{rsaRoot, "ddb0f92bb95c8d6fd202ea6e8cc5ccd182b544f8cd696f47d580659ddc9df65a",
		"c7ff606d9e4cdeb5eb5ad8e49ce4b499299b3dff2fe6c3e7dbd36a6f6a0aefb9",
		"c1f3e20eb47080c1a4faf583267f24369a3e97", "2022-01-26T00:45:51Z", "2032-01-24T00:45:50Z", false},
	{ed25519Root, "2642f1f90b389fed2558d7324ca26d70e750b06f87c595c5de955e2560821657",
		"e5851ab98ad5a2fd564c38eeff3419cc682a3d86ff7dae092e08976a138dd508",
		"67adec3009f0933b73c7985bc835cc79", "2026-06-16T21:29:14Z", "2036-06-16T21:34:14Z", true},
}

func TestRootFacts(t *testing.T) {
	for _, f := range rootFacts {
		sum := sha256.Sum256(f.der)
		if hex.EncodeToString(sum[:]) != f.sha256 {
			t.Errorf("certificate SHA-256 %x, want %s", sum, f.sha256)
			continue
		}
		c, err := x509.ParseCertificate(f.der)
		if err != nil {
			t.Fatal(err)
		}
		spki := sha256.Sum256(c.RawSubjectPublicKeyInfo)
		if hex.EncodeToString(spki[:]) != f.spki {
			t.Errorf("SPKI SHA-256 %x, want %s", spki, f.spki)
		}
		if c.SerialNumber.Text(16) != f.serial {
			t.Errorf("serial %s, want %s", c.SerialNumber.Text(16), f.serial)
		}
		nb, _ := time.Parse(time.RFC3339, f.notBefore)
		na, _ := time.Parse(time.RFC3339, f.until)
		if !c.NotBefore.Equal(nb) || !c.NotAfter.Equal(na) {
			t.Errorf("validity %v to %v", c.NotBefore, c.NotAfter)
		}
		if !c.IsCA || c.Subject.String() != c.Issuer.String() ||
			c.Subject.String() != "CN=Signal Messenger,O=Signal Messenger\\, LLC,L=Mountain View,ST=California,C=US" {
			t.Errorf("subject %s, issuer %s, CA %v", c.Subject, c.Issuer, c.IsCA)
		}
		switch k := c.PublicKey.(type) {
		case ed25519.PublicKey:
			if !f.ed25519 {
				t.Error("unexpected Ed25519 key")
			}
		case *rsa.PublicKey:
			if f.ed25519 || k.N.BitLen() != 4096 {
				t.Errorf("RSA key of %d bits", k.N.BitLen())
			}
		}
		if err := c.CheckSignatureFrom(c); err != nil {
			t.Errorf("self-signature: %v", err)
		}
	}
}

// TestPools: the chat host trusts both roots, REST only the RSA root, and
// neither consults the system pool.
func TestPools(t *testing.T) {
	rsaCert, _ := x509.ParseCertificate(rsaRoot)
	edCert, _ := x509.ParseCertificate(ed25519Root)
	verify := func(pool *x509.CertPool, c *x509.Certificate) bool {
		_, err := c.Verify(x509.VerifyOptions{Roots: pool, CurrentTime: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)})
		return err == nil
	}
	if !verify(ChatRoots(), rsaCert) || !verify(ChatRoots(), edCert) {
		t.Error("chat pool lacks a root")
	}
	if !verify(RESTRoots(), rsaCert) || verify(RESTRoots(), edCert) {
		t.Error("REST pool should hold exactly the RSA root")
	}
	if ChatRoots() == ChatRoots() {
		t.Error("pools must be fresh copies, not shared mutable state")
	}
}
