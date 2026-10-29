// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

// Vectors: testdata/vectors/storage.json (spec/service/storage.md §10).
type vectors struct {
	Keys struct {
		Chain struct {
			AEP        string `json:"account_entropy_pool_ascii"`
			Master     string `json:"master_key"`
			StorageKey string `json:"storage_key"`
		} `json:"aep_chain"`
		KAT struct {
			Master     string `json:"master_key"`
			StorageKey string `json:"storage_key"`
		} `json:"kat_svrkey_2a"`
		Manifest map[string]string `json:"manifest_keys"`
		Legacy   struct {
			StorageKey string `json:"storage_key"`
			Cases      []struct {
				Raw string `json:"raw_id"`
				Key string `json:"item_key"`
			}
		} `json:"item_keys_legacy"`
		Ikm struct {
			RecordIkm string `json:"record_ikm"`
			Cases     []struct {
				Raw string `json:"raw_id"`
				Key string `json:"item_key"`
			}
		} `json:"item_keys_record_ikm"`
	}
	Encryption struct {
		IV       string
		Items    map[string]json3 `json:"-"`
		Contact  json3            `json:"item_contact_legacy"`
		Ikm      json3            `json:"item_contact_record_ikm"`
		Account  json3            `json:"item_account_record_ikm"`
		Manifest struct {
			Key            string
			ManifestRecord string
			Encrypted      string `json:"encrypted_value"`
		} `json:"manifest_v2"`
	}
	Records map[string]struct {
		StorageRecord string
	}
}

type json3 struct{ Key, Plaintext, Ciphertext string }

func TestKeys(t *testing.T) {
	var v vectors
	testvec.Load(t, "storage.json", &v)
	c := v.Keys.Chain
	m := MasterKey(c.AEP)
	sk := StorageKey(m)
	if hex.EncodeToString(m[:]) != c.Master || hex.EncodeToString(sk[:]) != c.StorageKey {
		t.Errorf("chain %x %x", m, sk)
	}
	if k := StorageKey([32]byte(testvec.Hex(t, v.Keys.KAT.Master))); hex.EncodeToString(k[:]) != v.Keys.KAT.StorageKey {
		t.Errorf("libsignal KAT %x", k)
	}
	msk := [32]byte(testvec.Hex(t, v.Keys.Manifest["storage_key"]))
	for _, ver := range []uint64{0, 1, 2, 12345} {
		want := v.Keys.Manifest["v"+map[uint64]string{0: "0", 1: "1", 2: "2", 12345: "12345"}[ver]]
		if k := ManifestKey(msk, ver); hex.EncodeToString(k[:]) != want {
			t.Errorf("manifest key v%d %x", ver, k)
		}
	}
	lsk := [32]byte(testvec.Hex(t, v.Keys.Legacy.StorageKey))
	for _, cs := range v.Keys.Legacy.Cases {
		if k := ItemKey(lsk, nil, testvec.Hex(t, cs.Raw)); hex.EncodeToString(k[:]) != cs.Key {
			t.Errorf("legacy item key %s: %x", cs.Raw, k)
		}
	}
	for _, cs := range v.Keys.Ikm.Cases {
		if k := ItemKey(lsk, testvec.Hex(t, v.Keys.Ikm.RecordIkm), testvec.Hex(t, cs.Raw)); hex.EncodeToString(k[:]) != cs.Key {
			t.Errorf("ikm item key %s: %x", cs.Raw, k)
		}
	}
}

func TestEncryption(t *testing.T) {
	var v vectors
	testvec.Load(t, "storage.json", &v)
	iv := testvec.Hex(t, v.Encryption.IV)
	for name, c := range map[string]json3{"contact": v.Encryption.Contact, "ikm": v.Encryption.Ikm, "account": v.Encryption.Account} {
		k := [32]byte(testvec.Hex(t, c.Key))
		ct := Seal(k, iv, testvec.Hex(t, c.Plaintext))
		if hex.EncodeToString(ct) != c.Ciphertext {
			t.Errorf("%s: %x", name, ct)
		}
		p, err := Open(k, ct)
		if err != nil || !bytes.Equal(p, testvec.Hex(t, c.Plaintext)) {
			t.Errorf("%s: open %v", name, err)
		}
		var r signalpb.StorageRecord
		if err := proto.Unmarshal(p, &r); err != nil || r.Record == nil {
			t.Errorf("%s: record %v", name, err)
		}
		ct[len(ct)-1] ^= 1
		if _, err := Open(k, ct); err != ErrDecrypt {
			t.Errorf("%s: tampered tag accepted", name)
		}
	}
	mf := v.Encryption.Manifest
	p, err := Open([32]byte(testvec.Hex(t, mf.Key)), testvec.Hex(t, mf.Encrypted))
	if err != nil || hex.EncodeToString(p) != mf.ManifestRecord {
		t.Fatalf("manifest open %v", err)
	}
	var m signalpb.ManifestRecord
	if err := proto.Unmarshal(p, &m); err != nil || m.GetVersion() != 2 || len(m.GetIdentifiers()) != 5 || m.GetIdentifiers()[4].GetType() != 42 {
		t.Errorf("manifest %v %v", &m, err)
	}
	// Unknown type 42 round-trips.
	b, _ := proto.Marshal(&m)
	var back signalpb.ManifestRecord
	proto.Unmarshal(b, &back)
	if !proto.Equal(&m, &back) {
		t.Error("manifest does not round-trip")
	}
}

func TestRecords(t *testing.T) {
	var v vectors
	testvec.Load(t, "storage.json", &v)
	for name, r := range v.Records {
		var sr signalpb.StorageRecord
		if err := proto.Unmarshal(testvec.Hex(t, r.StorageRecord), &sr); err != nil || sr.Record == nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
