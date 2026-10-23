package signalpb

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// These tests check the hand-written .proto files against the serialized
// vectors the spec agents extracted from libsignal's tests.

func h(t *testing.T, s string) []byte { return testvec.Hex(t, s) }

func marshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPreKeyRecordVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind, Name, Record, Expected string
			RecordType                   string         `json:"record_type"`
			Parsed                       map[string]any `json:"expected_parsed"`
		}
	}
	testvec.Load(t, "ratchet_records.json", &f)
	n := 0
	for _, v := range f.Vectors {
		if v.Kind != "prekey_record_parse" {
			continue
		}
		n++
		var m proto.Message = &StoredSignedPreKey{}
		if v.RecordType == "PreKeyRecord" {
			m = &StoredPreKey{}
		}
		err := proto.Unmarshal(h(t, v.Record), m)
		if strings.HasPrefix(v.Expected, "parse fails") {
			if err == nil {
				t.Errorf("%s: parsed", v.Name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		switch r := m.(type) {
		case *StoredPreKey:
			if r.Id != 42 || len(r.PublicKey) != 0 || len(r.PrivateKey) != 0 {
				t.Errorf("%s: %v", v.Name, r)
			}
		case *StoredSignedPreKey:
			if r.Id != 42 || len(r.PublicKey) != 0 || len(r.Signature) != 0 || r.Timestamp != 0 {
				t.Errorf("%s: %v", v.Name, r)
			}
		}
		// proto3 writes no zero values, but these records spell out empty
		// fields, so the input is preserved only semantically.
	}
	if n != 6 {
		t.Errorf("ran %d prekey record vectors, want 6", n)
	}
}

func TestSessionRecordVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind, Name, Record string
			Parsed             map[string]any `json:"expected_parsed"`
		}
	}
	testvec.Load(t, "ratchet_records.json", &f)
	n := 0
	for _, v := range f.Vectors {
		if v.Kind != "session_usability" {
			continue
		}
		n++
		var rec StoredSessionRecord
		if err := proto.Unmarshal(h(t, v.Record), &rec); err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		cur := rec.GetCurrent()
		if got := float64(cur.GetVersion()); got != v.Parsed["current_session.session_version"].(float64) {
			t.Errorf("%s: version %v", v.Name, got)
		}
		if cur.SenderChain == nil {
			t.Errorf("%s: sender chain missing", v.Name)
		}
		if hex.EncodeToString(cur.GetAliceBaseKey()) != v.Parsed["current_session.alice_base_key"] {
			t.Errorf("%s: alice base key %x", v.Name, cur.GetAliceBaseKey())
		}
		if hex.EncodeToString(cur.GetPqRatchetState()) != v.Parsed["current_session.pq_ratchet_state"] {
			t.Errorf("%s: SPQR state %x", v.Name, cur.GetPqRatchetState())
		}
		if len(rec.GetPrevious()) != 0 {
			t.Errorf("%s: %d previous sessions", v.Name, len(rec.GetPrevious()))
		}
		// These records are in field order with no zero values, so they
		// re-encode byte for byte.
		if got := marshal(t, &rec); !bytes.Equal(got, h(t, v.Record)) {
			t.Errorf("%s: re-encodes as %x", v.Name, got)
		}
	}
	if n != 2 {
		t.Errorf("ran %d session records, want 2", n)
	}
}

func TestDecryptionFailureReportVector(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind      string
			Timestamp uint64 `json:"original_timestamp_ms"`
			Device    uint32 `json:"original_sender_device_id"`
			Report    string `json:"expected_decryption_error_message"`
			Body      string `json:"expected_plaintext_content_body"`
		}
	}
	testvec.Load(t, "ratchet_messages.json", &f)
	for _, v := range f.Vectors {
		if v.Kind != "decryption_error_message_in_plaintext_content" {
			continue
		}
		report := marshal(t, &DecryptionFailureReport{Timestamp: proto.Uint64(v.Timestamp), DeviceId: proto.Uint32(v.Device)})
		if hex.EncodeToString(report) != v.Report {
			t.Errorf("report %x, want %s", report, v.Report)
		}
		// The body is followed by the single 0x80 padding-boundary byte.
		body := marshal(t, &PlaintextContentBody{DecryptionFailureReport: report})
		if hex.EncodeToString(append(body, 0x80)) != v.Body {
			t.Errorf("body %x, want %s (minus the trailing 80)", body, v.Body)
		}
		return
	}
	t.Fatal("no vector")
}

func TestSenderCertVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind, Name, Certificate, Signature, Serialized string
			SerializedSender                               string         `json:"serialized_sender_certificate"`
			Fields                                         map[string]any `json:"expected_fields"`
			ID                                             uint32
		}
	}
	testvec.Load(t, "sealed_sender_certificates.json", &f)
	senders, known := 0, 0
	for _, v := range f.Vectors {
		switch v.Kind {
		case "sender_certificate":
			senders++
			raw := h(t, v.SerializedSender)
			var c SenderCert
			if err := proto.Unmarshal(raw, &c); err != nil {
				t.Fatalf("%s: %v", v.Name, err)
			}
			if hex.EncodeToString(c.GetBody()) != v.Certificate || hex.EncodeToString(c.GetSignature()) != v.Signature {
				t.Errorf("%s: body or signature differs", v.Name)
			}
			var b SenderCert_Body
			if err := proto.Unmarshal(c.GetBody(), &b); err != nil {
				t.Fatalf("%s: body: %v", v.Name, err)
			}
			fl := v.Fields
			if float64(b.GetDeviceId()) != fl["sender_device"].(float64) || float64(b.GetExpires()) != fl["expires"].(float64) {
				t.Errorf("%s: device %d expires %d", v.Name, b.GetDeviceId(), b.GetExpires())
			}
			if hex.EncodeToString(b.GetIdentityKey()) != fl["identity_key"] {
				t.Errorf("%s: identity key %x", v.Name, b.GetIdentityKey())
			}
			if b.PhoneNumber != nil {
				t.Errorf("%s: unexpected phone number", v.Name)
			}
			enc := fl["sender_uuid_encoding"].(string)
			switch {
			case strings.Contains(enc, "field 6"):
				if b.GetAciString() != fl["sender_uuid"] {
					t.Errorf("%s: ACI %q", v.Name, b.GetAciString())
				}
			case strings.Contains(enc, "field 7"):
				if len(b.GetAciBytes()) != 16 {
					t.Errorf("%s: ACI bytes %x", v.Name, b.GetAciBytes())
				}
			}
			signer := fl["signer"].(string)
			if strings.Contains(signer, "field 5") {
				if hex.EncodeToString(b.GetServerCert()) != fl["signer_server_certificate"] {
					t.Errorf("%s: embedded server cert differs", v.Name)
				}
			} else if b.GetKnownServerCertId() != 0x7357c357 {
				t.Errorf("%s: known cert id %#x", v.Name, b.GetKnownServerCertId())
			}
			// Lossless: unknown fields (one vector carries field 999) survive.
			if got := marshal(t, &c); !bytes.Equal(got, raw) {
				t.Errorf("%s: outer cert re-encodes differently", v.Name)
			}
			if got := marshal(t, &b); !bytes.Equal(got, c.GetBody()) {
				t.Errorf("%s: body re-encodes differently", v.Name)
			}
		case "known_server_certificate":
			known++
			raw := h(t, v.Serialized)
			var c ServerCert
			if err := proto.Unmarshal(raw, &c); err != nil {
				t.Fatalf("%s: %v", v.Name, err)
			}
			var b ServerCert_Body
			if err := proto.Unmarshal(c.GetBody(), &b); err != nil {
				t.Fatal(err)
			}
			if b.GetKeyId() != v.ID || len(b.GetPublicKey()) != 33 || len(c.GetSignature()) != 64 {
				t.Errorf("%s: id %d, key %d bytes", v.Name, b.GetKeyId(), len(b.GetPublicKey()))
			}
			if got := marshal(t, &c); !bytes.Equal(got, raw) {
				t.Errorf("%s: re-encodes differently", v.Name)
			}
		}
	}
	if senders != 3 || known != 3 {
		t.Errorf("ran %d sender and %d known-server vectors, want 3 and 3", senders, known)
	}
}
