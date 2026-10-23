// Package trust holds the TLS roots for Signal's servers
// (spec/service/transport.md §3.3, §5). The system trust store is never
// consulted.
//
// Both certificates were extracted from the reference distribution's
// libsignal_jni_amd64.so with tools/extract-certs, selected by the spec's
// SHA-256 fingerprints; trust_test.go checks every fact the spec records.
package trust

import (
	"crypto/x509"
	_ "embed"
)

var (
	// rsaRoot is "Signal Messenger" RSA 4096 (2022 to 2032). libsignal
	// trusts it for chat and it is the only live entry in signal-cli's
	// REST truststore.
	//go:embed signal-rsa.cer
	rsaRoot []byte
	// ed25519Root is "Signal Messenger" Ed25519 (2026 to 2036), trusted
	// by libsignal for chat only.
	//go:embed signal-ed25519.cer
	ed25519Root []byte
)

func pool(ders ...[]byte) *x509.CertPool {
	p := x509.NewCertPool()
	for _, der := range ders {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			panic("trust: embedded root: " + err.Error())
		}
		p.AddCert(c)
	}
	return p
}

// ChatRoots returns a new pool with the roots for the libsignal-net chat
// host (grpc.chat.staging.signal.org): both Signal roots.
func ChatRoots() *x509.CertPool { return pool(rsaRoot, ed25519Root) }

// RESTRoots returns a new pool with the roots for REST, CDN, storage and
// SVR2 hosts: the Signal RSA root (the truststore's other entry expired
// in 2023).
func RESTRoots() *x509.CertPool { return pool(rsaRoot) }
