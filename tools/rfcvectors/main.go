// SPDX-License-Identifier: GPL-3.0-or-later

// Command rfcvectors extracts test vectors from the plain-text RFCs into
// JSON files under testdata/vectors/, so the vectors are reproducible from
// the published documents instead of hand-copied.
//
// Supported: RFC 8032 section 7.1 (Ed25519), RFC 7748 sections 5.2 and 6.1
// (X25519), RFC 9496 appendix A (ristretto255), RFC 8452 appendix C
// (AES-GCM-SIV), RFC 7541 appendices B and C (HPACK).
//
// Usage (from the repo root, with the RFC text downloaded from
// https://www.rfc-editor.org/rfc/rfcNNNN.txt):
//
//	go run ./tools/rfcvectors -rfc 8032 -in rfc8032.txt > testdata/vectors/rfc8032_ed25519.json
//	go run ./tools/rfcvectors -rfc 7748 -in rfc7748.txt > testdata/vectors/rfc7748_x25519.json
//	go run ./tools/rfcvectors -rfc 9496 -in rfc9496.txt > testdata/vectors/rfc9496_ristretto255.json
//	go run ./tools/rfcvectors -rfc 8452 -in rfc8452.txt > testdata/vectors/rfc8452_aes_gcm_siv.json
//	go run ./tools/rfcvectors -rfc 7541 -in rfc7541.txt > testdata/vectors/rfc7541_hpack.json
//	go run ./tools/rfcvectors -rfc 7541 -hpack-huffman-go -in rfc7541.txt | gofmt > internal/service/h2/hpack_tables.go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type source struct {
	Document string `json:"document"`
	Sections string `json:"sections"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Tool     string `json:"tool"`
}

var huffmanGo *bool

func main() {
	num := flag.Int("rfc", 0, "RFC number: 8032, 7748 or 9496")
	in := flag.String("in", "", "path to the RFC plain text")
	huffmanGo = flag.Bool("hpack-huffman-go", false, "for RFC 7541: write Appendices A and B as Go tables instead of the vectors")
	flag.Parse()

	if err := run(*num, *in); err != nil {
		fmt.Fprintln(os.Stderr, "rfcvectors:", err)
		os.Exit(1)
	}
}

func run(num int, in string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	src := source{
		Document: fmt.Sprintf("RFC %d", num),
		URL:      fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%d.txt", num),
		SHA256:   hex.EncodeToString(sum[:]),
		Tool:     "go run ./tools/rfcvectors -rfc " + strconv.Itoa(num),
	}
	lines := cleanLines(string(raw), num)

	var out any
	switch num {
	case 8032:
		src.Sections = "7.1"
		out, err = parse8032(src, lines)
	case 7748:
		src.Sections = "5.2, 6.1"
		out, err = parse7748(src, lines)
	case 9496:
		src.Sections = "Appendix A"
		out, err = parse9496(src, lines)
	case 8452:
		src.Sections = "Appendix C"
		out, err = parse8452(src, lines)
	case 7541:
		if *huffmanGo {
			return writeHuffmanGo(src, lines)
		}
		src.Sections = "Appendix C"
		out, err = parse7541(src, lines)
	default:
		return fmt.Errorf("unsupported RFC %d", num)
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// cleanLines drops form feeds and the page footer and header lines that
// interrupt the RFC body, together with the blank lines around them, so a
// hex block split across pages reads as one block. (If two separate blocks
// get joined, the length checks below catch it.)
func cleanLines(text string, num int) []string {
	header := fmt.Sprintf("RFC %d ", num)
	var out []string
	skipBlank := false
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(strings.ReplaceAll(l, "\f", ""), " \r")
		switch {
		case strings.Contains(l, "[Page "):
			for len(out) > 0 && out[len(out)-1] == "" {
				out = out[:len(out)-1]
			}
			skipBlank = true
			continue
		case strings.HasPrefix(l, header):
			continue
		case l == "" && skipBlank:
			continue
		}
		skipBlank = false
		out = append(out, l)
	}
	return out
}

// section returns the lines strictly between the first line that starts
// with from and the next line that starts with to.
func section(lines []string, from, to string) ([]string, error) {
	start := -1
	for i, l := range lines {
		if start < 0 && strings.HasPrefix(l, from) {
			start = i + 1
			continue
		}
		if start >= 0 && strings.HasPrefix(l, to) {
			return lines[start:i], nil
		}
	}
	return nil, fmt.Errorf("section %q..%q not found", from, to)
}

var hexLine = regexp.MustCompile(`^[0-9a-f ]+$`)

// hexAfter concatenates the hex lines that follow lines[i], stopping at the
// first line that is not hex (blank lines stop it too).
func hexAfter(lines []string, i int) (string, int) {
	var b strings.Builder
	j := i + 1
	for ; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "" || !hexLine.MatchString(t) {
			break
		}
		b.WriteString(strings.ReplaceAll(t, " ", ""))
	}
	return b.String(), j
}

func checkHex(s string, n int, what string) error {
	b, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n >= 0 && len(b) != n {
		return fmt.Errorf("%s: %d bytes, want %d", what, len(b), n)
	}
	return nil
}

type ed25519Vector struct {
	Name      string `json:"name"`
	SecretKey string `json:"secret_key"`
	PublicKey string `json:"public_key"`
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

var msgLabel = regexp.MustCompile(`^MESSAGE \(length (\d+) bytes?\):$`)

func parse8032(src source, lines []string) (any, error) {
	body, err := section(lines, "7.1.  ", "7.2.  ")
	if err != nil {
		return nil, err
	}
	var vs []ed25519Vector
	var cur *ed25519Vector
	for i := 0; i < len(body); i++ {
		t := strings.TrimSpace(body[i])
		if name, ok := strings.CutPrefix(t, "-----"); ok {
			if name != "" {
				vs = append(vs, ed25519Vector{Name: name})
				cur = &vs[len(vs)-1]
			}
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case t == "SECRET KEY:":
			cur.SecretKey, i = hexAfter(body, i)
			i--
		case t == "PUBLIC KEY:":
			cur.PublicKey, i = hexAfter(body, i)
			i--
		case t == "SIGNATURE:":
			cur.Signature, i = hexAfter(body, i)
			i--
		case msgLabel.MatchString(t):
			n, _ := strconv.Atoi(msgLabel.FindStringSubmatch(t)[1])
			cur.Message, i = hexAfter(body, i)
			i--
			if err := checkHex(cur.Message, n, cur.Name+" message"); err != nil {
				return nil, err
			}
		}
	}
	for _, v := range vs {
		for _, c := range []struct {
			s    string
			n    int
			what string
		}{{v.SecretKey, 32, "secret key"}, {v.PublicKey, 32, "public key"}, {v.Signature, 64, "signature"}} {
			if err := checkHex(c.s, c.n, v.Name+" "+c.what); err != nil {
				return nil, err
			}
		}
	}
	if len(vs) != 5 {
		return nil, fmt.Errorf("found %d Ed25519 vectors, want 5", len(vs))
	}
	return struct {
		Source  source          `json:"source"`
		Vectors []ed25519Vector `json:"vectors"`
	}{src, vs}, nil
}

type x25519Vector struct {
	Scalar string `json:"scalar"`
	U      string `json:"u"`
	Output string `json:"output"`
}

type iterated struct {
	Iterations int    `json:"iterations"`
	Output     string `json:"output"`
}

type ecdh struct {
	AlicePrivate string `json:"alice_private"`
	AlicePublic  string `json:"alice_public"`
	BobPrivate   string `json:"bob_private"`
	BobPublic    string `json:"bob_public"`
	Shared       string `json:"shared"`
}

func parse7748(src source, lines []string) (any, error) {
	body, err := section(lines, "5.2.  ", "6.  ")
	if err != nil {
		return nil, err
	}
	var vs []x25519Vector
	var its []iterated
	inX25519 := false
	for i, l := range body {
		t := strings.TrimSpace(l)
		switch t {
		case "X25519:":
			inX25519 = true
			continue
		case "X448:":
			inX25519 = false
			continue
		}
		if !inX25519 {
			continue
		}
		val, _ := hexAfter(body, i)
		switch t {
		case "Input scalar:":
			vs = append(vs, x25519Vector{Scalar: val})
		case "Input u-coordinate:":
			vs[len(vs)-1].U = val
		case "Output u-coordinate:":
			vs[len(vs)-1].Output = val
		case "After one iteration:":
			its = append(its, iterated{1, val})
		case "After 1,000 iterations:":
			its = append(its, iterated{1000, val})
		case "After 1,000,000 iterations:":
			its = append(its, iterated{1000000, val})
		}
	}
	if len(vs) != 2 || len(its) != 3 {
		return nil, fmt.Errorf("found %d X25519 vectors and %d iterated results, want 2 and 3", len(vs), len(its))
	}

	body, err = section(lines, "6.1.  ", "6.2.  ")
	if err != nil {
		return nil, err
	}
	var e ecdh
	fields := map[string]*string{
		"Alice's private key, a:":           &e.AlicePrivate,
		"Alice's public key, X25519(a, 9):": &e.AlicePublic,
		"Bob's private key, b:":             &e.BobPrivate,
		"Bob's public key, X25519(b, 9):":   &e.BobPublic,
		"Their shared secret, K:":           &e.Shared,
	}
	for i, l := range body {
		if p, ok := fields[strings.TrimSpace(l)]; ok {
			*p, _ = hexAfter(body, i)
		}
	}

	all := []string{e.AlicePrivate, e.AlicePublic, e.BobPrivate, e.BobPublic, e.Shared}
	for _, v := range vs {
		all = append(all, v.Scalar, v.U, v.Output)
	}
	for _, it := range its {
		all = append(all, it.Output)
	}
	for _, s := range all {
		if err := checkHex(s, 32, "RFC 7748 value"); err != nil {
			return nil, err
		}
	}
	return struct {
		Source   source         `json:"source"`
		Vectors  []x25519Vector `json:"vectors"`
		Iterated []iterated     `json:"iterated"`
		ECDH     ecdh           `json:"ecdh"`
	}{src, vs, its, e}, nil
}

type invalidEncoding struct {
	Reason   string `json:"reason"`
	Encoding string `json:"encoding"`
}

type uniformVector struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type sqrtRatioVector struct {
	U         string `json:"u"`
	V         string `json:"v"`
	WasSquare bool   `json:"was_square"`
	R         string `json:"r"`
}

var multipleLabel = regexp.MustCompile(`^B\[ ?(\d+)\]: (.*)$`)

func parse9496(src source, lines []string) (any, error) {
	// A.1: multiples of the generator.
	body, err := section(lines, "A.1.  ", "A.2.  ")
	if err != nil {
		return nil, err
	}
	var multiples []string
	for i, l := range body {
		m := multipleLabel.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		rest, _ := hexAfter(body, i)
		v := strings.ReplaceAll(m[2], " ", "") + rest
		if n, _ := strconv.Atoi(m[1]); n != len(multiples) {
			return nil, fmt.Errorf("B[%d] out of order", n)
		}
		multiples = append(multiples, v)
	}

	// A.2: invalid encodings, grouped under "# reason" comments.
	body, err = section(lines, "A.2.  ", "A.3.  ")
	if err != nil {
		return nil, err
	}
	var invalid []invalidEncoding
	reason := ""
	for i := 0; i < len(body); i++ {
		t := strings.TrimSpace(body[i])
		if r, ok := strings.CutPrefix(t, "# "); ok {
			reason = strings.TrimSuffix(r, ".")
			continue
		}
		if t == "" || !hexLine.MatchString(t) || reason == "" {
			continue
		}
		v, j := hexAfter(body, i-1)
		invalid = append(invalid, invalidEncoding{reason, v})
		i = j
	}

	// A.3: element derivation from 64 uniform bytes. Entries are I/O pairs,
	// except for a final group of inputs that share one output.
	body, err = section(lines, "A.3.  ", "A.4.  ")
	if err != nil {
		return nil, err
	}
	var uniform []uniformVector
	var pending []string
	for i, l := range body {
		t := strings.TrimSpace(l)
		if v, ok := strings.CutPrefix(t, "I: "); ok {
			rest, _ := hexAfter(body, i)
			pending = append(pending, strings.ReplaceAll(v, " ", "")+rest)
		} else if v, ok := strings.CutPrefix(t, "O: "); ok {
			rest, _ := hexAfter(body, i)
			out := strings.ReplaceAll(v, " ", "") + rest
			for _, in := range pending {
				uniform = append(uniform, uniformVector{in, out})
			}
			pending = nil
		}
	}

	// A.4: SQRT_RATIO_M1.
	body, err = section(lines, "A.4.  ", "Appendix B.")
	if err != nil {
		return nil, err
	}
	var sqrt []sqrtRatioVector
	for _, l := range body {
		t := strings.TrimSpace(l)
		k, v, ok := strings.Cut(t, ": ")
		if !ok {
			continue
		}
		switch k {
		case "u":
			sqrt = append(sqrt, sqrtRatioVector{U: v})
		case "v":
			sqrt[len(sqrt)-1].V = v
		case "was_square":
			sqrt[len(sqrt)-1].WasSquare = v == "TRUE"
		case "r":
			sqrt[len(sqrt)-1].R = v
		}
	}

	if len(multiples) != 16 || len(invalid) != 29 || len(uniform) != 11 || len(sqrt) != 6 {
		return nil, fmt.Errorf("found %d multiples, %d invalid, %d uniform, %d sqrt-ratio; want 16, 29, 11, 6",
			len(multiples), len(invalid), len(uniform), len(sqrt))
	}
	for _, m := range multiples {
		if err := checkHex(m, 32, "multiple"); err != nil {
			return nil, err
		}
	}
	for _, e := range invalid {
		if err := checkHex(e.Encoding, 32, "invalid encoding"); err != nil {
			return nil, err
		}
	}
	for _, u := range uniform {
		if err := checkHex(u.Input, 64, "uniform input"); err != nil {
			return nil, err
		}
		if err := checkHex(u.Output, 32, "uniform output"); err != nil {
			return nil, err
		}
	}
	for _, s := range sqrt {
		for _, x := range []string{s.U, s.V, s.R} {
			if err := checkHex(x, 32, "sqrt-ratio value"); err != nil {
				return nil, err
			}
		}
	}
	return struct {
		Source             source            `json:"source"`
		GeneratorMultiples []string          `json:"generator_multiples"`
		InvalidEncodings   []invalidEncoding `json:"invalid_encodings"`
		FromUniformBytes   []uniformVector   `json:"from_uniform_bytes"`
		SqrtRatioM1        []sqrtRatioVector `json:"sqrt_ratio_m1"`
	}{src, multiples, invalid, uniform, sqrt}, nil
}

type gcmSIVVector struct {
	Section    string `json:"section"`
	Plaintext  string `json:"plaintext"`
	AAD        string `json:"aad"`
	Key        string `json:"key"`
	Nonce      string `json:"nonce"`
	AuthKey    string `json:"record_authentication_key"`
	EncKey     string `json:"record_encryption_key"`
	PolyvalOut string `json:"polyval_result"`
	Tag        string `json:"tag"`
	Result     string `json:"result"`
}

var sivLabel = regexp.MustCompile(`^(Plaintext|AAD|Key|Nonce|Record authentication key|Record encryption key|POLYVAL input|POLYVAL result|POLYVAL result XOR nonce|\.\.\. and masked|Tag|Initial counter|Result)( \((\d+) bytes?\))? *=(.*)$`)

func parse8452(src source, lines []string) (any, error) {
	var vs []gcmSIVVector
	for _, sec := range []struct{ from, to, name string }{
		{"C.1.  ", "C.2.  ", "C.1"},
		{"C.2.  ", "C.3.  ", "C.2"},
		{"C.3.  ", "Acknowledgements", "C.3"},
	} {
		body, err := section(lines, sec.from, sec.to)
		if err != nil {
			return nil, err
		}
		var cur *gcmSIVVector
		for i, l := range body {
			m := sivLabel.FindStringSubmatch(strings.TrimSpace(l))
			if m == nil {
				continue
			}
			rest, _ := hexAfter(body, i)
			val := strings.ReplaceAll(strings.TrimSpace(m[4]), " ", "") + rest
			if m[3] != "" {
				n, _ := strconv.Atoi(m[3])
				if err := checkHex(val, n, sec.name+" "+m[1]); err != nil {
					return nil, err
				}
			}
			switch m[1] {
			case "Plaintext":
				vs = append(vs, gcmSIVVector{Section: sec.name, Plaintext: val})
				cur = &vs[len(vs)-1]
			case "AAD":
				cur.AAD = val
			case "Key":
				cur.Key = val
			case "Nonce":
				cur.Nonce = val
			case "Record authentication key":
				cur.AuthKey = val
			case "Record encryption key":
				cur.EncKey = val
			case "POLYVAL result":
				cur.PolyvalOut = val
			case "Tag":
				cur.Tag = val
			case "Result":
				cur.Result = val
			}
		}
	}
	for _, v := range vs {
		if err := checkHex(v.Nonce, 12, v.Section+" nonce"); err != nil {
			return nil, err
		}
		if err := checkHex(v.Tag, 16, v.Section+" tag"); err != nil {
			return nil, err
		}
		if len(v.Key) != 32 && len(v.Key) != 64 {
			return nil, fmt.Errorf("%s: key of %d hex digits", v.Section, len(v.Key))
		}
		if len(v.Result) != len(v.Plaintext)+32 {
			return nil, fmt.Errorf("%s: result length %d for plaintext %d", v.Section, len(v.Result), len(v.Plaintext))
		}
	}
	return struct {
		Source  source         `json:"source"`
		Vectors []gcmSIVVector `json:"vectors"`
	}{src, vs}, nil
}

// RFC 7541 (HPACK).

type hpackStep struct {
	Headers    [][2]string `json:"headers"`
	Encoded    string      `json:"encoded"`
	EntrySizes []int       `json:"dynamic_table_entry_sizes"`
	TableSize  int         `json:"dynamic_table_size"`
}

type hpackSequence struct {
	Name           string      `json:"name"`
	Representation string      `json:"representation,omitempty"`
	Huffman        bool        `json:"huffman"`
	TableSizeLimit int         `json:"table_size_limit"`
	Steps          []hpackStep `json:"steps"`
}

var (
	hpackEntry     = regexp.MustCompile(`^\[\s*\d+\]\s+\(s\s*=\s*(\d+)\)`)
	hpackTableSize = regexp.MustCompile(`^Table size:\s*(\d+)$`)
)

// splitHeader splits "name: value"; names may start with ':'.
func splitHeader(l string) ([2]string, error) {
	i := strings.Index(l[1:], ": ")
	if i < 0 {
		return [2]string{}, fmt.Errorf("bad header line %q", l)
	}
	return [2]string{l[:i+1], l[i+3:]}, nil
}

// parseHPACKSteps reads the examples between from and to, one step per
// subsection heading.
func parseHPACKSteps(lines []string, from, to string) ([]hpackStep, error) {
	body, err := section(lines, from, to)
	if err != nil {
		return nil, err
	}
	var steps []hpackStep
	var cur *hpackStep
	mode := ""
	for _, l := range body {
		t := strings.TrimSpace(l)
		switch {
		case t == "Header list to encode:":
			steps = append(steps, hpackStep{})
			cur, mode = &steps[len(steps)-1], "headers"
			continue
		case t == "Hex dump of encoded data:":
			mode = "hex"
			continue
		case strings.HasPrefix(t, "Decoding process:"), strings.HasPrefix(t, "Decoded header list:"), strings.HasPrefix(t, "Dynamic Table"):
			mode = ""
			continue
		}
		if cur == nil {
			continue
		}
		switch mode {
		case "headers":
			if t == "" {
				continue
			}
			h, err := splitHeader(t)
			if err != nil {
				return nil, err
			}
			cur.Headers = append(cur.Headers, h)
		case "hex":
			if t == "" && cur.Encoded == "" {
				continue // the blank line before the first hex row
			}
			left, _, ok := strings.Cut(t, "|")
			if !ok {
				mode = ""
				continue
			}
			cur.Encoded += strings.ReplaceAll(strings.TrimSpace(left), " ", "")
		default:
			if m := hpackEntry.FindStringSubmatch(t); m != nil {
				n, _ := strconv.Atoi(m[1])
				cur.EntrySizes = append(cur.EntrySizes, n)
			} else if m := hpackTableSize.FindStringSubmatch(t); m != nil {
				cur.TableSize, _ = strconv.Atoi(m[1])
			}
		}
	}
	return steps, nil
}

func parse7541(src source, lines []string) (any, error) {
	var seqs []hpackSequence
	for _, c := range []struct{ from, to, name, rep string }{
		{"C.2.1.  ", "C.2.2.  ", "C.2.1", "literal with incremental indexing"},
		{"C.2.2.  ", "C.2.3.  ", "C.2.2", "literal without indexing"},
		{"C.2.3.  ", "C.2.4.  ", "C.2.3", "literal never indexed"},
		{"C.2.4.  ", "C.3.  ", "C.2.4", "indexed"},
	} {
		steps, err := parseHPACKSteps(lines, c.from, c.to)
		if err != nil {
			return nil, err
		}
		seqs = append(seqs, hpackSequence{Name: c.name, Representation: c.rep, TableSizeLimit: 4096, Steps: steps})
	}
	for _, c := range []struct {
		from, to, name string
		huffman        bool
		limit          int
	}{
		{"C.3.  ", "C.4.  ", "C.3", false, 4096},
		{"C.4.  ", "C.5.  ", "C.4", true, 4096},
		{"C.5.  ", "C.6.  ", "C.5", false, 256},
		{"C.6.  ", "Acknowledgments", "C.6", true, 256},
	} {
		steps, err := parseHPACKSteps(lines, c.from, c.to)
		if err != nil {
			return nil, err
		}
		seqs = append(seqs, hpackSequence{Name: c.name, Huffman: c.huffman, TableSizeLimit: c.limit, Steps: steps})
	}
	for _, s := range seqs {
		want := 1
		if len(s.Name) == 3 {
			want = 3
		}
		if len(s.Steps) != want {
			return nil, fmt.Errorf("%s: %d steps, want %d", s.Name, len(s.Steps), want)
		}
		for i, st := range s.Steps {
			if err := checkHex(st.Encoded, -1, fmt.Sprintf("%s step %d", s.Name, i)); err != nil {
				return nil, err
			}
			if st.Encoded == "" {
				return nil, fmt.Errorf("%s step %d: no encoded data found", s.Name, i)
			}
			sum := 0
			for _, n := range st.EntrySizes {
				sum += n
			}
			if sum != st.TableSize || len(st.Headers) == 0 {
				return nil, fmt.Errorf("%s step %d: entry sizes add to %d, table size %d, %d headers", s.Name, i, sum, st.TableSize, len(st.Headers))
			}
		}
	}
	return struct {
		Source    source          `json:"source"`
		Sequences []hpackSequence `json:"sequences"`
	}{src, seqs}, nil
}

var huffmanRow = regexp.MustCompile(`\(\s*(\d+)\)\s+\|([01|]+)\s+([0-9a-f]+)\s+\[\s*(\d+)\]`)

// writeHuffmanGo prints RFC 7541 Appendix B as a Go table, checking each
// row's bit string against its hex code and length.
func writeHuffmanGo(src source, lines []string) error {
	body, err := section(lines, "Appendix B.", "Appendix C.")
	if err != nil {
		return err
	}
	type row struct {
		code uint32
		bits int
	}
	var rows []row
	for _, l := range body {
		m := huffmanRow.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		sym, _ := strconv.Atoi(m[1])
		if sym != len(rows) {
			return fmt.Errorf("symbol %d out of order", sym)
		}
		bitStr := strings.ReplaceAll(m[2], "|", "")
		code, err := strconv.ParseUint(m[3], 16, 32)
		if err != nil {
			return err
		}
		n, _ := strconv.Atoi(m[4])
		fromBits, err := strconv.ParseUint(bitStr, 2, 32)
		if err != nil || len(bitStr) != n || fromBits != code {
			return fmt.Errorf("symbol %d: bits %q, hex %s, len %d disagree", sym, bitStr, m[3], n)
		}
		rows = append(rows, row{uint32(code), n})
	}
	if len(rows) != 257 {
		return fmt.Errorf("found %d Huffman codes, want 257", len(rows))
	}
	fmt.Printf("// Code generated by %s -hpack-huffman-go from %s Appendices A and B\n", src.Tool, src.Document)
	fmt.Printf("// (%s, sha256 %s). DO NOT EDIT.\n\npackage h2\n\n", src.URL, src.SHA256)
	fmt.Println("// huffmanCodes[sym] is the HPACK Huffman code for symbol sym (256 is EOS),")
	fmt.Println("// aligned to the least significant bit, and huffmanLengths[sym] its length.")
	fmt.Println("var huffmanCodes = [257]uint32{")
	for i, r := range rows {
		fmt.Printf("\t%#x, // %d\n", r.code, i)
	}
	fmt.Println("}\n\nvar huffmanLengths = [257]uint8{")
	for i := 0; i < len(rows); i += 16 {
		fmt.Print("\t")
		for j := i; j < i+16 && j < len(rows); j++ {
			fmt.Printf("%d, ", rows[j].bits)
		}
		fmt.Println()
	}
	fmt.Println("}")
	return writeStaticTableGo(lines)
}

var staticRow = regexp.MustCompile(`^\|\s*(\d+)\s*\|\s*(\S+)\s*\|\s*(.*?)\s*\|$`)

// writeStaticTableGo prints RFC 7541 Appendix A, the 61-entry static table.
func writeStaticTableGo(lines []string) error {
	body, err := section(lines, "Appendix A.", "Appendix B.")
	if err != nil {
		return err
	}
	var entries [][2]string
	for _, l := range body {
		m := staticRow.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		if n, _ := strconv.Atoi(m[1]); n != len(entries)+1 {
			return fmt.Errorf("static table index %d out of order", n)
		}
		entries = append(entries, [2]string{m[2], m[3]})
	}
	if len(entries) != 61 {
		return fmt.Errorf("found %d static table entries, want 61", len(entries))
	}
	fmt.Println("\n// staticTable is RFC 7541 Appendix A; index i+1 is staticTable[i].")
	fmt.Println("var staticTable = [61][2]string{")
	for _, e := range entries {
		fmt.Printf("\t{%q, %q},\n", e[0], e[1])
	}
	fmt.Println("}")
	return nil
}
