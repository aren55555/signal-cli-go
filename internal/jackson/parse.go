// SPDX-License-Identifier: GPL-3.0-or-later

package jackson

import (
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/aren55555/signal-cli-go/internal/errcode"
)

// Units says how columns are counted: Java chars (UTF-16 code units),
// as for text read from a stream line, or bytes, as for a body parsed
// from an InputStream.
type Units int

const (
	Chars Units = iota
	Bytes
)

// maxDepth is StreamReadConstraints' default maximum nesting depth.
const maxDepth = 1000

// maxTokenLen bounds the token quoted in "Unrecognized token" messages.
const maxTokenLen = 256

type parser struct {
	in    []rune
	units Units
	pos   int
	// line is the current line number and lineStart the index where it
	// starts.
	line, lineStart int
	depth           int
}

// Parse reads the first JSON value of s and ignores whatever follows it,
// as Jackson's ObjectMapper.readTree does. An input that holds only
// whitespace yields a Missing node. Errors are *errcode.Error values with
// Jackson's message and location.
//
// Column conventions differ between Jackson's error paths; the offsets
// below were measured against the reference (see jackson_test.go).
func Parse(s string, units Units) (*Node, error) {
	p := &parser{in: []rune(s), units: units, line: 1}
	ok, err := p.skipWS()
	if err != nil || !ok {
		if err == nil {
			return &Node{Kind: Missing}, nil
		}
		return nil, err
	}
	return p.value(true)
}

func (p *parser) eof() bool { return p.pos >= len(p.in) }

// col is the 1-based column of rune index i on the current line.
func (p *parser) col(i int) int {
	c := 1
	for _, r := range p.in[p.lineStart:min(i, len(p.in))] {
		c += p.width(r)
	}
	if i > len(p.in) {
		c += i - len(p.in)
	}
	return c
}

func (p *parser) width(r rune) int {
	if p.units == Bytes {
		return utf8.RuneLen(r)
	}
	if r > 0xFFFF {
		return 2
	}
	return 1
}

// at is an error located at rune index i, reported as column i+1 on
// the line (col counts from 1).
func (p *parser) at(c errcode.Code, i int, args ...any) error {
	return c.New(append(args, p.line, p.col(i))...)
}

// atEOF is an error located just past the end of the input.
func (p *parser) atEOF(c errcode.Code, args ...any) error { return p.at(c, len(p.in), args...) }

// skipWS skips JSON whitespace, counting lines; false at end of input.
// Other control characters and comments between tokens are errors.
func (p *parser) skipWS() (bool, error) {
	for p.pos < len(p.in) {
		switch r := p.in[p.pos]; {
		case r == ' ' || r == '\t':
			p.pos++
		case r == '\n':
			p.pos++
			p.line, p.lineStart = p.line+1, p.pos
		case r == '\r':
			p.pos++
			if p.pos < len(p.in) && p.in[p.pos] == '\n' {
				p.pos++
			}
			p.line, p.lineStart = p.line+1, p.pos
		case r < 0x20:
			return false, p.at(errcode.JSONIllegalWhitespace, p.pos+1, charDesc(r))
		case r == '/':
			return false, p.at(errcode.JSONComment, p.pos)
		default:
			return true, nil
		}
	}
	return false, nil
}

// charDesc is Jackson's description of an unexpected character. Java
// sees a supplementary character as its high surrogate, which prints
// as '?'.
func charDesc(r rune) string {
	if r > 0xFFFF {
		hi, _ := utf16.EncodeRune(r)
		return "'?' (code " + strconv.Itoa(int(hi)) + " / 0x" + strconv.FormatInt(int64(hi), 16) + ")"
	}
	switch {
	case r < 0x20 || (r >= 0x7f && r <= 0x9f):
		return "(CTRL-CHAR, code " + strconv.Itoa(int(r)) + ")"
	case r > 255:
		return "'" + string(r) + "' (code " + strconv.Itoa(int(r)) + " / 0x" + strconv.FormatInt(int64(r), 16) + ")"
	}
	return "'" + string(r) + "' (code " + strconv.Itoa(int(r)) + ")"
}

// isIdentStart and isIdentPart are Java's Character.isJavaIdentifierStart
// and isJavaIdentifierPart for BMP characters.
func isIdentStart(r rune) bool {
	return r <= 0xFFFF && (unicode.IsLetter(r) || unicode.Is(unicode.Sc, r) || unicode.Is(unicode.Pc, r) || unicode.Is(unicode.Nl, r))
}

func isIdentPart(r rune) bool {
	if r > 0xFFFF {
		return false
	}
	return isIdentStart(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) ||
		(r <= 8) || (r >= 0x0e && r <= 0x1b) || (r >= 0x7f && r <= 0x9f) || unicode.Is(unicode.Cf, r)
}

// value parses a value at p.pos (not at end of input).
func (p *parser) value(root bool) (*Node, error) {
	r := p.in[p.pos]
	switch {
	case r == '{':
		return p.object()
	case r == '[':
		return p.array()
	case r == '"':
		s, err := p.str(errcode.JSONEOFInString, "string value")
		if err != nil {
			return nil, err
		}
		return NewString(s), nil
	case r == '-' || (r >= '0' && r <= '9'):
		return p.number(root)
	case r == '+':
		return nil, p.at(errcode.JSONPlusSign, p.pos+1)
	case r == 't':
		return p.literal("true", NewBool(true))
	case r == 'f':
		return p.literal("false", NewBool(false))
	case r == 'n':
		return p.literal("null", NewNull())
	case r == 'N' || r == 'I':
		tok := map[rune]string{'N': "NaN", 'I': "Infinity"}[r]
		if p.match(p.pos, tok) {
			return nil, p.at(errcode.JSONNonStandardToken, p.pos+len(tok), tok)
		}
		return nil, p.invalidToken(p.pos)
	case r == ']' && root:
		return nil, p.at(errcode.JSONUnmatchedClose, p.pos, "]", "Array")
	case r == '}' && root:
		return nil, p.at(errcode.JSONUnmatchedClose, p.pos, "}", "Object")
	case isIdentStart(r):
		return nil, p.invalidToken(p.pos)
	}
	return nil, p.at(errcode.JSONExpectedValue, p.pos, charDesc(r))
}

// match reports whether tok appears at i and is not followed by an
// identifier character.
func (p *parser) match(i int, tok string) bool {
	end := i + len([]rune(tok))
	if end > len(p.in) || string(p.in[i:end]) != tok {
		return false
	}
	return end == len(p.in) || !isIdentPart(p.in[end])
}

// invalidToken reports the token starting at start: its first character
// and the identifier characters after it, at most 256 of them.
func (p *parser) invalidToken(start int) error {
	var b strings.Builder
	n := 0
	for i := start; i < len(p.in) && n < maxTokenLen && (i == start || isIdentPart(p.in[i])); i++ {
		b.WriteRune(p.in[i])
		n++
	}
	if n == maxTokenLen {
		b.WriteString("...")
	}
	return p.at(errcode.JSONUnrecognizedToken, start, b.String())
}

func (p *parser) literal(word string, n *Node) (*Node, error) {
	if !p.match(p.pos, word) {
		return nil, p.invalidToken(p.pos)
	}
	p.pos += len(word)
	return n, nil
}

func (p *parser) enter() error {
	p.depth++
	if p.depth > maxDepth {
		return errcode.JSONNestingDepth.New(p.depth, maxDepth)
	}
	return nil
}

// ws skips whitespace; at end of input it returns eofErr.
func (p *parser) ws(eofErr func() error) error {
	ok, err := p.skipWS()
	switch {
	case err != nil:
		return err
	case !ok:
		return eofErr()
	}
	return nil
}

func (p *parser) object() (*Node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	startLine, startCol := p.line, p.col(p.pos)
	closeMarker := func() error { return p.atEOF(errcode.JSONEOFCloseMarker, "Object", startLine, startCol) }
	between := func() error { return p.atEOF(errcode.JSONEOFInObject) }
	mismatch := func() error {
		return p.at(errcode.JSONMismatchedClose, p.pos, "]", "}", "Object", startLine, startCol)
	}
	obj := NewObject()
	p.pos++
	if err := p.ws(closeMarker); err != nil {
		return nil, err
	}
	switch p.in[p.pos] {
	case '}':
		p.pos++
		return obj, nil
	case ']':
		return nil, mismatch()
	}
	for {
		if p.in[p.pos] != '"' {
			return nil, p.at(errcode.JSONExpectedFieldName, p.pos, charDesc(p.in[p.pos]))
		}
		name, err := p.str(errcode.JSONEOFInFieldName, "name")
		if err != nil {
			return nil, err
		}
		if err := p.ws(between); err != nil {
			return nil, err
		}
		if p.in[p.pos] != ':' {
			return nil, p.at(errcode.JSONExpectedColon, p.pos, charDesc(p.in[p.pos]))
		}
		p.pos++
		if err := p.ws(between); err != nil {
			return nil, err
		}
		v, err := p.value(false)
		if err != nil {
			return nil, err
		}
		obj.Set(name, v)
		if err := p.ws(closeMarker); err != nil {
			return nil, err
		}
		switch c := p.in[p.pos]; c {
		case '}':
			p.pos++
			return obj, nil
		case ']':
			return nil, mismatch()
		case ',':
			p.pos++
			if err := p.ws(between); err != nil {
				return nil, err
			}
		default:
			return nil, p.at(errcode.JSONExpectedObjectComma, p.pos, charDesc(c))
		}
	}
}

func (p *parser) array() (*Node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	startLine, startCol := p.line, p.col(p.pos)
	closeMarker := func() error { return p.atEOF(errcode.JSONEOFCloseMarker, "Array", startLine, startCol) }
	mismatch := func() error {
		return p.at(errcode.JSONMismatchedClose, p.pos, "}", "]", "Array", startLine, startCol)
	}
	arr := NewArray()
	p.pos++
	if err := p.ws(closeMarker); err != nil {
		return nil, err
	}
	switch p.in[p.pos] {
	case ']':
		p.pos++
		return arr, nil
	case '}':
		return nil, mismatch()
	}
	for {
		v, err := p.value(false)
		if err != nil {
			return nil, err
		}
		arr.Elems = append(arr.Elems, v)
		if err := p.ws(closeMarker); err != nil {
			return nil, err
		}
		switch c := p.in[p.pos]; c {
		case ']':
			p.pos++
			return arr, nil
		case '}':
			return nil, mismatch()
		case ',':
			p.pos++
			if err := p.ws(func() error { return p.atEOF(errcode.JSONEOFInArray) }); err != nil {
				return nil, err
			}
		default:
			return nil, p.at(errcode.JSONExpectedArrayComma, p.pos, charDesc(c))
		}
	}
}

// str reads a string starting at the opening quote; what names the
// string in the control-character message.
func (p *parser) str(eofCode errcode.Code, what string) (string, error) {
	p.pos++
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.atEOF(eofCode)
		}
		r := p.in[p.pos]
		switch {
		case r == '"':
			p.pos++
			return b.String(), nil
		case r == '\\':
			p.pos++
			if p.eof() {
				return "", p.atEOF(errcode.JSONEOFInEscape)
			}
			e := p.in[p.pos]
			switch e {
			case '"', '\\', '/':
				b.WriteRune(e)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				u, err := p.hex4()
				if err != nil {
					return "", err
				}
				if utf16.IsSurrogate(u) && p.pos+6 < len(p.in) && p.in[p.pos+1] == '\\' && p.in[p.pos+2] == 'u' {
					save := p.pos
					p.pos += 2
					if lo, err := p.hex4(); err == nil {
						if c := utf16.DecodeRune(u, lo); c != unicode.ReplacementChar {
							b.WriteRune(c)
							break
						}
					}
					p.pos = save
				}
				b.WriteRune(u)
			default:
				return "", p.at(errcode.JSONBadEscape, p.pos, charDesc(e))
			}
			p.pos++
		case r < 0x20:
			return "", p.at(errcode.JSONControlChar, p.pos, charDesc(r), what)
		default:
			b.WriteRune(r)
			p.pos++
		}
	}
}

// hex4 reads the four hex digits after \u; p.pos is on the 'u' and ends
// on the last digit.
func (p *parser) hex4() (rune, error) {
	var v rune
	for i := 0; i < 4; i++ {
		p.pos++
		if p.eof() {
			return 0, p.atEOF(errcode.JSONEOFInEscape)
		}
		c := p.in[p.pos]
		d, err := strconv.ParseUint(string(c), 16, 8)
		if err != nil {
			return 0, p.at(errcode.JSONBadUnicodeEscape, p.pos, charDesc(c))
		}
		v = v<<4 | rune(d)
	}
	return v, nil
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func (p *parser) number(root bool) (*Node, error) {
	start := p.pos
	if p.in[p.pos] == '-' {
		p.pos++
		if p.eof() {
			return nil, p.atEOF(errcode.JSONNoDigitAfterSign)
		}
		if c := p.in[p.pos]; !isDigit(c) {
			if c == 'I' {
				// "-Infinity", "-INF" or the character after the 'I'.
				if p.pos+1 >= len(p.in) {
					return nil, p.atEOF(errcode.JSONEOFInNumber)
				}
				switch p.in[p.pos+1] {
				case 'n':
					if p.match(start, "-Infinity") {
						return nil, p.at(errcode.JSONNonStandardToken, start+len("-Infinity"), "-Infinity")
					}
					return nil, p.invalidToken(start)
				case 'N':
					return nil, p.invalidToken(start)
				}
				return nil, p.at(errcode.JSONMinusNotDigit, p.pos+1, charDesc(p.in[p.pos+1]))
			}
			return nil, p.at(errcode.JSONMinusNotDigit, p.pos, charDesc(c))
		}
	}
	if p.in[p.pos] == '0' && p.pos+1 < len(p.in) && isDigit(p.in[p.pos+1]) {
		return nil, p.at(errcode.JSONLeadingZeroes, p.pos+1)
	}
	for !p.eof() && isDigit(p.in[p.pos]) {
		p.pos++
	}
	float := false
	if !p.eof() && p.in[p.pos] == '.' {
		float = true
		dot := p.pos
		p.pos++
		if p.eof() {
			return nil, p.at(errcode.JSONDecimalPoint, dot, charDesc('.'))
		}
		if !isDigit(p.in[p.pos]) {
			return nil, p.at(errcode.JSONDecimalPoint, dot, charDesc(p.in[p.pos]))
		}
		for !p.eof() && isDigit(p.in[p.pos]) {
			p.pos++
		}
	}
	if !p.eof() && (p.in[p.pos] == 'e' || p.in[p.pos] == 'E') {
		float = true
		e := p.pos
		p.pos++
		if !p.eof() && (p.in[p.pos] == '+' || p.in[p.pos] == '-') {
			p.pos++
		}
		if p.eof() {
			return nil, p.atEOF(errcode.JSONExponentEOF)
		}
		if !isDigit(p.in[p.pos]) {
			return nil, p.at(errcode.JSONExponentNotDigit, e, charDesc(p.in[p.pos]))
		}
		for !p.eof() && isDigit(p.in[p.pos]) {
			p.pos++
		}
	}
	if root && !p.eof() {
		switch c := p.in[p.pos]; c {
		case ' ', '\t', '\n', '\r':
		default:
			return nil, p.at(errcode.JSONRootSpace, p.pos, charDesc(c))
		}
	}
	text := string(p.in[start:p.pos])
	if float {
		f, _ := strconv.ParseFloat(text, 64) // ±Inf on overflow, as Java
		return &Node{Kind: Double, F: f}, nil
	}
	if v, err := strconv.ParseInt(text, 10, 64); err == nil {
		return NewInt(v), nil
	}
	b, _ := new(big.Int).SetString(text, 10)
	return &Node{Kind: BigInt, Big: b}, nil
}
