package argparse

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Text layout from spec/surface/cli/argparse.md §2-4 and §9.

// Character classes (§2.1, §2.4).
const (
	clW = iota
	clG
	clN
	clD
	clP
	clE
	clL
	clO
	clC
	clQ
	clS
	clB
	clR
)

func charClass(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return clG
	case r == ',' || r == '.':
		return clN
	case r == '-':
		return clD
	case strings.ContainsRune("!%:;?]}", r):
		return clP
	case r == ')':
		return clE
	case r == '(':
		return clL
	case r == '[' || r == '{':
		return clO
	case r == '$' || r == '\'':
		return clC
	case r == '"':
		return clQ
	case r == '\t' || r == '\n' || r == '\f' || r == 0x03:
		return clB
	case r == '\r':
		return clR
	case r == ' ' || r == 0x7f || r < 0x20:
		return clS
	}
	if r < 0x80 {
		return clW
	}
	// Non-ASCII equivalences (§2.4).
	switch r {
	case 0x2019, 0x201D, 0x203A, 0x00BB, 0x00A2, 0x00B0, 0x2030, 0x2032, 0x3001, 0x3002, 0xFF01, 0xFF09, 0xFF0C:
		return clP
	case 0x2018, 0x201C, 0x201E, 0x2039, 0x00AB, 0x20AC, 0x00A3, 0x00A5, 0xFF08:
		return clO
	case 0x2010, 0x2012, 0x2013, 0x2014, 0x2015, 0x00AD:
		return clD
	case 0x2002, 0x2003, 0x2009, 0x3000, 0x0085:
		return clS
	case 0x2028, 0x2029:
		return clB
	}
	switch {
	case unicode.IsDigit(r) || r == 0x00B2 || r == 0x00BD:
		return clG
	case unicode.Is(unicode.Ps, r) || unicode.Is(unicode.Pi, r) || (unicode.Is(unicode.Sc, r) && r != 0x00A2):
		return clO
	case unicode.Is(unicode.Pe, r) || unicode.Is(unicode.Pf, r):
		return clP
	case unicode.Is(unicode.Pd, r):
		return clD
	case unicode.Is(unicode.Zs, r):
		return clS
	}
	return clW
}

// breakDFA is §2.2's automaton: next state per class, -1 = no transition.
var breakDFA = [32][13]int{
	{1, 2, 3, 4, 3, 3, 5, 5, 16, 5, 6, 7, 8},
	{1, 1, 3, 9, 3, 3, 10, -1, 17, 3, 6, 7, 8},
	{1, 2, 11, 9, 3, 3, 10, -1, 17, 3, 6, 7, 8},
	{-1, -1, 3, -1, 3, 3, 10, -1, 17, 3, 6, 7, 8},
	{-1, 12, -1, 4, -1, -1, 13, 14, 24, 14, 6, 7, 8},
	{1, 2, 3, 4, 3, 3, 5, 5, 16, 5, 6, 7, 8},
	{-1, -1, -1, -1, -1, -1, 10, -1, -1, -1, 6, 7, 8},
	{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
	{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 7, -1},
	{-1, -1, -1, 9, -1, -1, 10, -1, 17, -1, 6, 7, 8},
	{-1, -1, -1, -1, -1, 15, -1, -1, -1, -1, -1, -1, -1},
	{-1, 12, 3, -1, 3, 3, 10, -1, 17, 3, 6, 7, 8},
	{-1, 12, 11, 9, 3, 3, 10, -1, 17, 3, 6, 7, 8},
	{-1, 12, -1, 14, -1, 15, 14, 14, 14, 14, -1, -1, -1},
	{-1, 12, -1, 14, -1, -1, 14, 14, 14, 14, -1, -1, -1},
	{-1, -1, 15, -1, 15, 15, -1, -1, -1, 15, -1, 7, 8},
	{1, 2, 3, 18, 3, 3, 16, 16, 28, 16, 6, 7, 8},
	{1, 2, 3, 25, 3, 3, 17, 17, 17, 17, 6, 7, 8},
	{-1, 12, -1, 19, -1, -1, 13, 14, 20, 14, 6, 7, 8},
	{-1, 12, 11, 19, 3, 3, 13, 14, 20, 21, 6, 7, 8},
	{1, 2, 11, 22, 3, 3, 26, 26, 26, 26, 6, 7, 8},
	{-1, 12, 3, 14, 3, 3, 13, 14, 26, 21, 6, 7, 8},
	{-1, 12, -1, 23, -1, -1, 13, 14, 26, 14, 6, 7, 8},
	{-1, 12, -1, 23, -1, -1, 13, 14, 14, 14, 6, 7, 8},
	{1, 2, 3, 27, 3, 3, 24, 24, 26, 24, 6, 7, 8},
	{-1, 12, -1, 25, -1, -1, 13, 14, 17, 14, 6, 7, 8},
	{1, 2, 3, 22, 3, 3, 26, 26, 26, 26, 6, 7, 8},
	{-1, 12, -1, 27, -1, -1, 13, 14, 26, 14, 6, 7, 8},
	{1, 2, 3, 29, 3, 3, 28, 28, 28, 28, 6, 7, 8},
	{-1, 12, -1, 30, -1, -1, 13, 14, 20, 14, 6, 7, 8},
	{-1, 12, 11, 30, 3, 3, 13, 14, 20, 31, 6, 7, 8},
	{-1, 12, 3, 14, 3, 3, 13, 14, 14, 31, 6, 7, 8},
}

var nonAccepting = map[int]bool{0: true, 10: true, 13: true, 14: true}

// Segments splits text at its line-break opportunities (§2.2).
func Segments(text string) []string {
	rs := []rune(text)
	var out []string
	for pos := 0; pos < len(rs); {
		state, last := 0, -1
		for i := pos; i < len(rs); i++ {
			state = breakDFA[state][charClass(rs[i])]
			if state < 0 {
				break
			}
			if !nonAccepting[state] {
				last = i
			}
		}
		if last < pos {
			last = pos
		}
		out = append(out, string(rs[pos:last+1]))
		pos = last + 1
	}
	return out
}

// width is the UTF-16 length (§0).
func width(s string) int { return len(utf16.Encode([]rune(s))) }

// adjustSpace justifies a full line to w; cur is the column after it
// (§3.2).
func adjustSpace(line string, w, cur int) string {
	trimmed := strings.TrimLeft(line, " ")
	cur -= len(line) - len(trimmed)
	t := strings.TrimRight(trimmed, " ")
	cur -= len(trimmed) - len(t)
	// Split into words and the gaps between them.
	var words, gaps []string
	i := 0
	for i < len(t) {
		j := i
		for j < len(t) && t[j] != ' ' {
			j++
		}
		words = append(words, t[i:j])
		k := j
		for k < len(t) && t[k] == ' ' {
			k++
		}
		if k > j {
			gaps = append(gaps, t[j:k])
		}
		i = k
	}
	if len(gaps) == 0 {
		return t
	}
	needed := w - cur
	if needed < 0 {
		needed = 0
	}
	each, r := needed/len(gaps), needed%len(gaps)
	s := (len(gaps) - r + 1) / 2
	var b strings.Builder
	for gi, word := range words {
		b.WriteString(word)
		if gi < len(gaps) {
			b.WriteString(gaps[gi])
			extra := each
			if gi >= s && gi < s+r {
				extra++
			}
			b.WriteString(strings.Repeat(" ", extra))
		}
	}
	return b.String()
}

// Wrap fills and justifies text (§3.1).
func Wrap(text string, w, offset int, firstIndent, contIndent string) string {
	var out, line strings.Builder
	out.WriteString(firstIndent)
	col := offset + width(firstIndent)
	for _, seg := range Segments(text) {
		sw := width(seg)
		if col+sw > w {
			out.WriteString(adjustSpace(line.String(), w, col))
			out.WriteString("\n" + contIndent)
			line.Reset()
			col = width(contIndent) + sw
		} else {
			col += sw
		}
		line.WriteString(seg)
		if strings.HasSuffix(seg, "\n") {
			out.WriteString(line.String())
			out.WriteString(contIndent)
			line.Reset()
			col = width(contIndent)
		}
	}
	out.WriteString(line.String())
	return out.String()
}

// helpRow renders one help row (§4).
func helpRow(title, help string, w int) string {
	var b strings.Builder
	b.WriteString("  " + title)
	if help == "" {
		b.WriteString("\n")
		return b.String()
	}
	if width(title) <= 21 {
		b.WriteString(strings.Repeat(" ", 25-2-width(title)))
	} else {
		b.WriteString("\n" + strings.Repeat(" ", 25))
	}
	b.WriteString(Wrap(help, w, 25, "", strings.Repeat(" ", 25)))
	b.WriteString("\n")
	return b.String()
}

// editDistance is §9's weighted distance from t to c.
func editDistance(t, c string) int {
	a, b := utf16.Encode([]rune(t)), utf16.Encode([]rune(c))
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			sub := 2
			if a[i-1] == b[j-1] {
				sub = 0
			}
			v := d[i-1][j-1] + sub
			if i >= 2 && j >= 2 && a[i-1] != b[j-1] && a[i-2] == b[j-1] && a[i-1] == b[j-2] && d[i-2][j-2] < v {
				v = d[i-2][j-2]
			}
			if x := d[i-1][j] + 4; x < v {
				v = x
			}
			if x := d[i][j-1] + 1; x < v {
				v = x
			}
			d[i][j] = v
		}
	}
	return d[len(a)][len(b)]
}

// suggest returns the "Did you mean" block for input against candidates
// (bodies used for scoring are given by body), or "" (§9).
func suggest(input string, candidates []string, body func(string) string) string {
	type scored struct {
		s    int
		text string
	}
	var list []scored
	in := body(input)
	for _, c := range candidates {
		cb := body(c)
		s := 0
		if !strings.HasPrefix(cb, in) {
			s = editDistance(in, cb)
		}
		list = append(list, scored{s, c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].s != list[j].s {
			return list[i].s < list[j].s
		}
		return lessUTF16(list[i].text, list[j].text)
	})
	if len(list) == 0 || list[0].s >= 7 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nDid you mean:\n")
	for _, x := range list {
		if x.s != list[0].s {
			break
		}
		b.WriteString("\t" + x.text + "\n")
	}
	return b.String()
}

func lessUTF16(a, b string) bool {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}
