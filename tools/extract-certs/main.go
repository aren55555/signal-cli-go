// Command extract-certs finds DER X.509 certificates embedded in binary
// files (a jar's native library, a keystore) and writes those whose SHA-256
// fingerprint is listed. Certificates are public data; the fingerprints
// come from spec/service/transport.md, so nothing is taken on trust from
// the scanned file.
//
//	go run ./tools/extract-certs -want ddb0f9…,2642f1… -out dir file...
package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	want := flag.String("want", "", "comma-separated SHA-256 fingerprints (hex, colons allowed)")
	out := flag.String("out", ".", "output directory; files are named <fingerprint prefix>.cer")
	flag.Parse()
	wanted := map[string]bool{}
	for _, w := range strings.Split(*want, ",") {
		w = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(w), ":", ""))
		if len(w) != 64 {
			log.Fatalf("bad fingerprint %q", w)
		}
		wanted[w] = true
	}
	found := map[string]bool{}
	for _, path := range flag.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		for _, der := range scan(data) {
			sum := sha256.Sum256(der)
			fp := hex.EncodeToString(sum[:])
			if !wanted[fp] || found[fp] {
				continue
			}
			found[fp] = true
			name := filepath.Join(*out, fp[:16]+".cer")
			if err := os.WriteFile(name, der, 0o644); err != nil {
				log.Fatal(err)
			}
			cert, _ := x509.ParseCertificate(der)
			fmt.Printf("%s  %s  %s\n", name, fp, cert.Subject)
		}
	}
	for fp := range wanted {
		if !found[fp] {
			log.Fatalf("not found: %s", fp)
		}
	}
}

// scan returns every byte range that starts with a DER SEQUENCE with a
// two-byte length and parses as a certificate.
func scan(data []byte) [][]byte {
	var out [][]byte
	for i := 0; i+4 <= len(data); i++ {
		if data[i] != 0x30 || data[i+1] != 0x82 {
			continue
		}
		n := int(data[i+2])<<8 | int(data[i+3])
		end := i + 4 + n
		if end > len(data) {
			continue
		}
		if _, err := x509.ParseCertificate(data[i:end]); err == nil {
			out = append(out, data[i:end])
		}
	}
	return out
}
