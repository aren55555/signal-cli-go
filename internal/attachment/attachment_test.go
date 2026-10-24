package attachment

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/testvec"
	"google.golang.org/protobuf/proto"
)

type vectors struct {
	Padding struct {
		Buckets []int64 `json:"buckets_from_541_to_2^31"`
		Samples []struct {
			Plain  int64 `json:"plaintext_size"`
			Padded int64 `json:"padded_size"`
			Cipher int64 `json:"ciphertext_size"`
		} `json:"samples"`
	} `json:"padding"`
	Cipher struct {
		Key   string `json:"attachment_key"`
		IV    string `json:"iv"`
		Cases []struct {
			Name       string `json:"name"`
			PlainHex   string `json:"plaintext_hex"`
			PlainRule  string `json:"plaintext_rule"`
			PlainSize  int    `json:"plaintext_size"`
			Padded     bool   `json:"padded"`
			CipherHex  string `json:"ciphertext_hex"`
			CipherSize int    `json:"ciphertext_size"`
			Digest     string `json:"digest"`
			MAC        string `json:"mac"`
		} `json:"cases"`
	} `json:"attachment_cipher"`
	Pointer struct {
		Cases []struct {
			Name    string `json:"name"`
			Fields  map[string]any
			Encoded string `json:"encoded_hex"`
		} `json:"cases"`
	} `json:"attachment_pointer_encoding"`
	DataURI []struct {
		Input     string            `json:"input"`
		Result    string            `json:"result"`
		MediaType string            `json:"media_type"`
		Params    map[string]string `json:"parameters"`
		DataHex   string            `json:"data_hex"`
		Error     string            `json:"error"`
	} `json:"data_uri"`
	StoredNames struct {
		Cases []struct {
			RemoteID    string  `json:"remote_id"`
			ContentType *string `json:"content_type"`
			FileName    *string `json:"file_name"`
			Stored      string  `json:"stored_name"`
		} `json:"cases"`
	} `json:"stored_file_names"`
	Sticker struct {
		PackKey  string `json:"pack_key"`
		Expanded string `json:"expanded_key"`
		Blob     struct {
			Plain  string `json:"plaintext_hex"`
			IV     string `json:"iv"`
			Cipher string `json:"ciphertext_hex"`
		} `json:"blob"`
		Manifest struct {
			Pack   string `json:"pack_proto_hex"`
			Cipher string `json:"ciphertext_hex"`
		} `json:"manifest"`
	} `json:"sticker"`
}

func load(t *testing.T) vectors {
	var v vectors
	testvec.Load(t, "attachments.json", &v)
	return v
}

func TestPaddingBuckets(t *testing.T) {
	v := load(t)
	if len(buckets) != len(v.Padding.Buckets) {
		t.Fatalf("%d buckets, want %d", len(buckets), len(v.Padding.Buckets))
	}
	for i, b := range v.Padding.Buckets {
		if buckets[i] != b {
			t.Fatalf("bucket %d = %d, want %d", i, buckets[i], b)
		}
	}
	for _, s := range v.Padding.Samples {
		if p := PaddedSize(s.Plain); p != s.Padded {
			t.Errorf("PaddedSize(%d) = %d, want %d", s.Plain, p, s.Padded)
		}
		if c := CiphertextSize(s.Padded); c != s.Cipher {
			t.Errorf("CiphertextSize(%d) = %d, want %d", s.Padded, c, s.Cipher)
		}
	}
	// Around each boundary.
	for i := 1; i < len(buckets); i++ {
		if PaddedSize(buckets[i-1]+1) != buckets[i] || PaddedSize(buckets[i]) != buckets[i] {
			t.Fatalf("boundary at bucket %d", i)
		}
	}
}

func plaintext(t *testing.T, hexs, rule string, n int) []byte {
	if hexs != "" || n == 0 {
		return testvec.Hex(t, hexs)
	}
	if !strings.Contains(rule, "mod 251") {
		t.Fatalf("unknown rule %q", rule)
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestCipherVectors(t *testing.T) {
	v := load(t)
	key, iv := testvec.Hex(t, v.Cipher.Key), testvec.Hex(t, v.Cipher.IV)
	for _, c := range v.Cipher.Cases {
		t.Run(c.Name, func(t *testing.T) {
			plain := plaintext(t, c.PlainHex, c.PlainRule, c.PlainSize)
			if len(plain) != c.PlainSize && c.PlainHex == "" {
				t.Fatalf("plaintext size %d", len(plain))
			}
			blob, digest := encrypt(key, iv, plain, c.Padded)
			if len(blob) != c.CipherSize {
				t.Fatalf("ciphertext %d bytes, want %d", len(blob), c.CipherSize)
			}
			if c.CipherHex != "" && hex.EncodeToString(blob) != c.CipherHex {
				t.Errorf("ciphertext mismatch")
			}
			if hex.EncodeToString(digest) != c.Digest {
				t.Errorf("digest %x", digest)
			}
			if hex.EncodeToString(blob[len(blob)-32:]) != c.MAC {
				t.Errorf("mac mismatch")
			}
			got, err := Decrypt(key, blob, digest, uint32(len(plain)))
			if err != nil || !bytes.Equal(got, plain) {
				t.Fatalf("round trip: %v", err)
			}
		})
	}
}

func TestDecryptChecks(t *testing.T) {
	key := make([]byte, 64)
	for i := range key {
		key[i] = byte(i)
	}
	blob, digest := encrypt(key, make([]byte, 16), []byte("hello"), true)
	cases := []struct {
		name   string
		key    []byte
		blob   []byte
		digest []byte
		want   string
	}{
		{"short key", key[:63], blob, digest, "Invalid combined key material size"},
		{"short", key, blob[:48], digest, "Message shorter than crypto overhead!"},
		{"mac", key, flip(blob, 20), digest, "MAC doesn't match!"},
		{"digest", key, blob, flip(digest, 0), "Digest doesn't match!"},
		{"no digest", key, blob, nil, "Attachment pointer has no digest."},
	}
	for _, c := range cases {
		if _, err := Decrypt(c.key, c.blob, c.digest, 5); err == nil || err.Error() != c.want {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	// size larger than the plaintext: the 0x00 padding, ending at the
	// padded end; size 0 gives nothing.
	if got, _ := Decrypt(key, blob, digest, 600); len(got) != 541 || got[5] != 0 {
		t.Errorf("oversize: %d bytes", len(got))
	}
	if got, _ := Decrypt(key, blob, digest, 0); len(got) != 0 {
		t.Errorf("size 0: %d bytes", len(got))
	}
}

func flip(b []byte, i int) []byte {
	c := bytes.Clone(b)
	c[i] ^= 1
	return c
}

func TestEncryptRandom(t *testing.T) {
	key, blob, digest, err := Encrypt([]byte("abc"))
	if err != nil || len(key) != 64 || len(blob) != 592 {
		t.Fatalf("%v %d %d", err, len(key), len(blob))
	}
	if d := sha256.Sum256(blob); !bytes.Equal(d[:], digest) {
		t.Fatal("digest")
	}
	got, err := Decrypt(key, blob, digest, 3)
	if err != nil || string(got) != "abc" {
		t.Fatal(err)
	}
}

func TestPointerEncoding(t *testing.T) {
	v := load(t)
	for _, c := range v.Pointer.Cases {
		var p signalpb.AppAttachmentPointer
		if err := proto.Unmarshal(testvec.Hex(t, c.Encoded), &p); err != nil {
			t.Fatal(err)
		}
		f := c.Fields
		if p.GetCdnKey() != f["cdnKey"] || float64(p.GetCdnNumber()) != f["cdnNumber"] ||
			p.GetContentType() != f["contentType"] || float64(p.GetSize()) != f["size"] ||
			float64(p.GetFlags()) != f["flags"] || float64(p.GetUploadTimestamp()) != f["uploadTimestamp"] ||
			len(p.GetKey()) != 64 || len(p.GetDigest()) != 32 || p.Flags == nil {
			t.Errorf("%s: decoded %v", c.Name, &p)
		}
		if fn, ok := f["fileName"]; ok != (p.FileName != nil) || (ok && fn != p.GetFileName()) {
			t.Errorf("%s: fileName", c.Name)
		}
		if uuidString(p.GetClientUuid()) != f["clientUuid"] {
			t.Errorf("%s: clientUuid %x", c.Name, p.GetClientUuid())
		}
		// Our own pointer for the same values encodes to the same fields.
		mine := NewPointer(Uploaded{CDN: p.GetCdnNumber(), CDNKey: p.GetCdnKey(), Key: p.GetKey(), Digest: p.GetDigest(), Size: p.GetSize()},
			Source{ContentType: p.GetContentType(), FileName: p.GetFileName(), Width: p.GetWidth(), Height: p.GetHeight()},
			p.GetFlags(), p.GetUploadTimestamp(), p.GetClientUuid())
		if !proto.Equal(mine, &p) {
			t.Errorf("%s: NewPointer = %v\nwant %v", c.Name, mine, &p)
		}
	}
}

func uuidString(b []byte) string {
	h := hex.EncodeToString(b)
	if len(h) != 32 {
		return h
	}
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func TestDataURI(t *testing.T) {
	v := load(t)
	for _, c := range v.DataURI {
		d, err := ParseDataURI(c.Input)
		if c.Result != "ok" {
			if err == nil {
				t.Errorf("%q: no error (want %s)", c.Input, c.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.Input, err)
			continue
		}
		if d.MediaType != c.MediaType || hex.EncodeToString(d.Data) != c.DataHex {
			t.Errorf("%q: got %q %x", c.Input, d.MediaType, d.Data)
		}
		if len(d.Params) != len(c.Params) {
			t.Errorf("%q: params %v, want %v", c.Input, d.Params, c.Params)
		}
		for k, want := range c.Params {
			if d.Params[k] != want {
				t.Errorf("%q: param %q = %q, want %q", c.Input, k, d.Params[k], want)
			}
		}
	}
}

func TestDataURINotURI(t *testing.T) {
	if _, err := ParseDataURI("/tmp/file.png"); !errors.Is(err, ErrNotDataURI) {
		t.Errorf("path: %v", err)
	}
}

func TestStoredNames(t *testing.T) {
	v := load(t)
	for _, c := range v.StoredNames.Cases {
		ct, fn := "", ""
		if c.ContentType != nil {
			ct = *c.ContentType
		}
		if c.FileName != nil {
			fn = *c.FileName
		}
		if got := StoredName(c.RemoteID, ct, fn); got != c.Stored {
			t.Errorf("StoredName(%q, %q, %q) = %q, want %q", c.RemoteID, ct, fn, got, c.Stored)
		}
	}
}

func TestSplitLongText(t *testing.T) {
	cases := []struct {
		in        string
		split     bool
		bodyBytes int
	}{
		{strings.Repeat("a", 2000), false, 2000},
		{strings.Repeat("a", 2001), true, 2000},
		{strings.Repeat("a", 1999) + "é", true, 1999},
		{strings.Repeat("a", 1998) + "€", true, 1998},
		{strings.Repeat("a", 1999) + "😀b", true, 1999},
		{strings.Repeat("é", 1001), true, 2000},
	}
	for i, c := range cases {
		body, split := SplitLongText(c.in)
		if split != c.split || len(body) != c.bodyBytes || !strings.HasPrefix(c.in, body) {
			t.Errorf("case %d: split=%v body %d bytes", i, split, len(body))
		}
	}
}

func TestSticker(t *testing.T) {
	v := load(t).Sticker
	key := StickerKey(testvec.Hex(t, v.PackKey))
	if hex.EncodeToString(key) != v.Expanded {
		t.Fatalf("key %x", key)
	}
	blob := testvec.Hex(t, v.Blob.Cipher)
	got, err := DecryptSticker(key, blob)
	if err != nil || hex.EncodeToString(got) != v.Blob.Plain {
		t.Fatalf("sticker: %v %x", err, got)
	}
	if b, _ := encrypt(key, testvec.Hex(t, v.Blob.IV), got, false); !bytes.Equal(b, blob) {
		t.Error("sticker re-encryption differs")
	}
	got, err = DecryptSticker(key, testvec.Hex(t, v.Manifest.Cipher))
	if err != nil || hex.EncodeToString(got) != v.Manifest.Pack {
		t.Fatalf("manifest: %v", err)
	}
	if _, err := DecryptSticker(key, flip(blob, 17)); err == nil {
		t.Error("tampered sticker accepted")
	}
}

func TestFromArgument(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o700)
	img := image.NewRGBA(image.Rect(0, 0, 7, 3))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	os.WriteFile(filepath.Join(dir, "pic.png"), buf.Bytes(), 0o600)
	os.WriteFile(filepath.Join(dir, "noext"), buf.Bytes(), 0o600)
	os.WriteFile(filepath.Join(dir, "notes"), []byte("plain text"), 0o600)
	os.WriteFile(filepath.Join(dataDir, "secret"), []byte("x"), 0o600)
	os.Symlink(filepath.Join(dataDir, "secret"), filepath.Join(dir, "link"))

	s, err := FromArgument(filepath.Join(dir, "pic.png"), dataDir)
	if err != nil || s.ContentType != "image/png" || s.FileName != "pic.png" || s.Width != 7 || s.Height != 3 {
		t.Errorf("png: %v %+v", err, s)
	}
	if s, _ := FromArgument(filepath.Join(dir, "noext"), dataDir); s.ContentType != "image/png" || s.Width != 7 {
		t.Errorf("sniffed: %+v", s.ContentType)
	}
	if s, _ := FromArgument(filepath.Join(dir, "notes"), dataDir); s.ContentType != "application/octet-stream" || s.Width != 0 {
		t.Errorf("text: %+v", s.ContentType)
	}
	for _, p := range []string{filepath.Join(dataDir, "secret"), filepath.Join(dir, "link")} {
		if _, err := FromArgument(p, dataDir); !errors.Is(err, ErrDataDirectory) {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := FromArgument(filepath.Join(dir, "missing"), dataDir); err == nil {
		t.Error("missing file accepted")
	}
	s, err = FromArgument("data:image/png;filename=a.png;base64,"+base64.StdEncoding.EncodeToString(buf.Bytes()), dataDir)
	if err != nil || s.FileName != "a.png" || s.Width != 7 || s.ContentType != "image/png" {
		t.Errorf("data uri: %v %+v", err, s)
	}
	// Bad base64 is retried as a path (observed), which does not exist.
	if _, err := FromArgument("data:image/png;base64,not*base64", dataDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("bad base64: %v", err)
	}
}

// TestDataDirRelative: the refusal also holds for a relative data
// directory, and files beside it are allowed.
func TestDataDirRelative(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cfg", "data"), 0o700)
	os.WriteFile(filepath.Join(dir, "cfg", "data", "accounts.json"), []byte("{}"), 0o600)
	os.WriteFile(filepath.Join(dir, "cfg", "ok.txt"), []byte("ok"), 0o600)
	t.Chdir(dir)
	if _, err := FromArgument("cfg/data/accounts.json", "cfg/data"); !errors.Is(err, ErrDataDirectory) {
		t.Errorf("relative data dir: %v", err)
	}
	if _, err := FromArgument("./cfg/../cfg/data/accounts.json", "./cfg/data/"); !errors.Is(err, ErrDataDirectory) {
		t.Errorf("unclean paths: %v", err)
	}
	if s, err := FromArgument("cfg/ok.txt", "cfg/data"); err != nil || string(s.Data) != "ok" {
		t.Errorf("sibling file: %v", err)
	}
}
