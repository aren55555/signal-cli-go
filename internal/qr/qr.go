// SPDX-License-Identifier: GPL-3.0-or-later

// Package qr encodes QR codes (ISO/IEC 18004) the way `signal-cli link`
// prints them (spec/surface/cli/link.md): byte mode, error correction
// level L, the smallest version that fits, and the mask with the lowest
// penalty score (ties go to the lower mask number).
package qr

import "errors"

// Code is a QR symbol; Dark[y][x] is true for a dark module.
type Code struct {
	Version int
	Size    int
	Dark    [][]bool
}

// ErrTooLong means the data does not fit version 40.
var ErrTooLong = errors.New("qr: data too long")

// ecL is error correction level L per version: EC codewords per block,
// then (block count, data codewords) for the two block groups.
var ecL = [41][5]int{
	{}, {7, 1, 19, 0, 0}, {10, 1, 34, 0, 0}, {15, 1, 55, 0, 0}, {20, 1, 80, 0, 0}, {26, 1, 108, 0, 0},
	{18, 2, 68, 0, 0}, {20, 2, 78, 0, 0}, {24, 2, 97, 0, 0}, {30, 2, 116, 0, 0}, {18, 2, 68, 2, 69},
	{20, 4, 81, 0, 0}, {24, 2, 92, 2, 93}, {26, 4, 107, 0, 0}, {30, 3, 115, 1, 116}, {22, 5, 87, 1, 88},
	{24, 5, 98, 1, 99}, {28, 1, 107, 5, 108}, {30, 5, 120, 1, 121}, {28, 3, 113, 4, 114}, {28, 3, 107, 5, 108},
	{28, 4, 116, 4, 117}, {28, 2, 111, 7, 112}, {30, 4, 121, 5, 122}, {30, 6, 117, 4, 118}, {26, 8, 106, 4, 107},
	{28, 10, 114, 2, 115}, {30, 8, 122, 4, 123}, {30, 3, 117, 10, 118}, {30, 7, 116, 7, 117}, {30, 5, 115, 10, 116},
	{30, 13, 115, 3, 116}, {30, 17, 115, 0, 0}, {30, 17, 115, 1, 116}, {30, 13, 115, 6, 116}, {30, 12, 121, 7, 122},
	{30, 6, 121, 14, 122}, {30, 17, 122, 4, 123}, {30, 4, 122, 18, 123}, {30, 20, 117, 4, 118}, {30, 19, 118, 6, 119},
}

func dataCodewords(v int) int { e := ecL[v]; return e[1]*e[2] + e[3]*e[4] }

// rawCodewords is the number of codewords a version holds (data + EC).
func rawCodewords(v int) int {
	n := (16*v+128)*v + 64
	if v >= 2 {
		a := v/7 + 2
		n -= (25*a-10)*a - 55
		if v >= 7 {
			n -= 36
		}
	}
	return n / 8
}

// alignment returns the alignment pattern centre coordinates.
func alignment(v int) []int {
	if v == 1 {
		return nil
	}
	n := v/7 + 2
	step := 26
	if v != 32 {
		step = (v*4 + n*2 + 1) / (n*2 - 2) * 2
	}
	out := make([]int, n)
	out[0] = 6
	for i, pos := n-1, 4*v+10; i >= 1; i, pos = i-1, pos-step {
		out[i] = pos
	}
	return out
}

type bits struct {
	b []bool
}

func (w *bits) put(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.b = append(w.b, v>>uint(i)&1 == 1)
	}
}

// Encode makes the QR code for data in byte mode.
func Encode(data []byte) (*Code, error) {
	v := 1
	for ; v <= 40; v++ {
		count := 8
		if v >= 10 {
			count = 16
		}
		if 4+count+8*len(data) <= dataCodewords(v)*8 {
			break
		}
	}
	if v > 40 {
		return nil, ErrTooLong
	}
	capBits := dataCodewords(v) * 8
	var w bits
	w.put(0b0100, 4)
	if v >= 10 {
		w.put(uint32(len(data)), 16)
	} else {
		w.put(uint32(len(data)), 8)
	}
	for _, c := range data {
		w.put(uint32(c), 8)
	}
	w.put(0, min(4, capBits-len(w.b)))
	for len(w.b)%8 != 0 {
		w.b = append(w.b, false)
	}
	cw := make([]byte, 0, dataCodewords(v))
	for i := 0; i < len(w.b); i += 8 {
		var c byte
		for j := 0; j < 8; j++ {
			if w.b[i+j] {
				c |= 0x80 >> uint(j)
			}
		}
		cw = append(cw, c)
	}
	for pad := byte(0xEC); len(cw) < dataCodewords(v); pad ^= 0xEC ^ 0x11 {
		cw = append(cw, pad)
	}
	return place(v, interleave(v, cw)), nil
}

// interleave splits the data into blocks, adds Reed-Solomon codewords
// and interleaves them.
func interleave(v int, data []byte) []byte {
	e := ecL[v]
	var blocks, ecs [][]byte
	off := 0
	for g := 0; g < 2; g++ {
		for i := 0; i < e[1+2*g]; i++ {
			n := e[2+2*g]
			b := data[off : off+n]
			off += n
			blocks = append(blocks, b)
			ecs = append(ecs, rsRemainder(b, e[0]))
		}
	}
	var out []byte
	for i := 0; ; i++ {
		any := false
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
				any = true
			}
		}
		if !any {
			break
		}
	}
	for i := 0; i < e[0]; i++ {
		for _, ec := range ecs {
			out = append(out, ec[i])
		}
	}
	return out
}

// GF(256) with the polynomial x^8+x^4+x^3+x^2+1.
func gfMul(a, b byte) byte {
	var p byte
	for b > 0 {
		if b&1 == 1 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1D
		}
		b >>= 1
	}
	return p
}

// rsRemainder is the n EC codewords for data (generator with roots
// α^0..α^(n-1)).
func rsRemainder(data []byte, n int) []byte {
	gen := []byte{1}
	root := byte(1)
	for i := 0; i < n; i++ {
		next := make([]byte, len(gen)+1)
		for j, c := range gen {
			next[j] ^= c
			next[j+1] ^= gfMul(c, root)
		}
		gen = next
		root = gfMul(root, 2)
	}
	rem := make([]byte, n)
	for _, d := range data {
		f := d ^ rem[0]
		copy(rem, rem[1:])
		rem[n-1] = 0
		for j := 0; j < n; j++ {
			rem[j] ^= gfMul(gen[j+1], f)
		}
	}
	return rem
}

type matrix struct {
	size     int
	dark, fn [][]bool
}

func newMatrix(size int) *matrix {
	m := &matrix{size: size}
	m.dark = make([][]bool, size)
	m.fn = make([][]bool, size)
	for i := range m.dark {
		m.dark[i] = make([]bool, size)
		m.fn[i] = make([]bool, size)
	}
	return m
}

func (m *matrix) setFn(x, y int, dark bool) {
	m.dark[y][x] = dark
	m.fn[y][x] = true
}

func (m *matrix) finder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= m.size || y >= m.size {
				continue
			}
			d := max(abs(dx), abs(dy))
			m.setFn(x, y, d != 2 && d != 4)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// functionPatterns draws finders, timing, alignment, the dark module
// and reserves the format and version areas.
func (m *matrix) functionPatterns(v int) {
	n := m.size
	m.finder(3, 3)
	m.finder(n-4, 3)
	m.finder(3, n-4)
	for i := 8; i < n-8; i++ {
		m.setFn(6, i, i%2 == 0)
		m.setFn(i, 6, i%2 == 0)
	}
	al := alignment(v)
	for i, ax := range al {
		for j, ay := range al {
			if (i == 0 && j == 0) || (i == 0 && j == len(al)-1) || (i == len(al)-1 && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					m.setFn(ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	m.format(0) // reserve; rewritten after masking
	if v >= 7 {
		m.version(v)
	}
}

// format writes the 15 format bits for level L and mask.
func (m *matrix) format(mask int) {
	data := uint32(1<<3 | mask) // L = 01
	rem := data
	for i := 0; i < 10; i++ {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	b := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return b>>uint(i)&1 == 1 }
	n := m.size
	for i := 0; i <= 5; i++ {
		m.setFn(8, i, bit(i))
	}
	m.setFn(8, 7, bit(6))
	m.setFn(8, 8, bit(7))
	m.setFn(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		m.setFn(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		m.setFn(n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		m.setFn(8, n-15+i, bit(i))
	}
	m.setFn(8, n-8, true) // the dark module
}

func (m *matrix) version(v int) {
	rem := uint32(v)
	for i := 0; i < 12; i++ {
		rem = rem<<1 ^ (rem>>11)*0x1F25
	}
	b := uint32(v)<<12 | rem
	for i := 0; i < 18; i++ {
		dark := b>>uint(i)&1 == 1
		a, c := m.size-11+i%3, i/3
		m.setFn(a, c, dark)
		m.setFn(c, a, dark)
	}
}

// placeData fills the non-function modules in the zigzag order.
func (m *matrix) placeData(cw []byte) {
	i := 0
	n := m.size
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < n; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				upward := (right+1)&2 == 0
				y := vert
				if upward {
					y = n - 1 - vert
				}
				if m.fn[y][x] {
					continue
				}
				if i < len(cw)*8 {
					m.dark[y][x] = cw[i>>3]>>(7-uint(i&7))&1 == 1
					i++
				}
			}
		}
	}
}

func maskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	}
	return ((x+y)%2+x*y%3)%2 == 0
}

func (m *matrix) applyMask(mask int) {
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if !m.fn[y][x] && maskBit(mask, x, y) {
				m.dark[y][x] = !m.dark[y][x]
			}
		}
	}
}

// penalty is the standard mask evaluation (rules N1-N4).
func (m *matrix) penalty() int {
	n := m.size
	get := func(x, y int, transpose bool) bool {
		if transpose {
			return m.dark[x][y]
		}
		return m.dark[y][x]
	}
	p := 0
	for _, t := range []bool{false, true} {
		for y := 0; y < n; y++ {
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && get(x, y, t) == get(x-1, y, t) {
					run++
					continue
				}
				if run >= 5 {
					p += 3 + run - 5
				}
				run = 1
			}
			// N3: 1:1:3:1:1 with four light modules on one side
			// (outside the symbol counts as light).
			light := func(from, to int) bool {
				for x := max(from, 0); x < min(to, n); x++ {
					if get(x, y, t) {
						return false
					}
				}
				return true
			}
			for x := 0; x+7 <= n; x++ {
				if get(x, y, t) && !get(x+1, y, t) && get(x+2, y, t) && get(x+3, y, t) && get(x+4, y, t) && !get(x+5, y, t) && get(x+6, y, t) &&
					(light(x-4, x) || light(x+7, x+11)) {
					p += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if m.dark[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				c := m.dark[y][x]
				if c == m.dark[y][x+1] && c == m.dark[y+1][x] && c == m.dark[y+1][x+1] {
					p += 3
				}
			}
		}
	}
	total := n * n
	p += abs(dark*2-total) * 10 / total * 10
	return p
}

func place(v int, cw []byte) *Code {
	size := 4*v + 17
	base := newMatrix(size)
	base.functionPatterns(v)
	base.placeData(cw)
	best, bestScore := -1, 0
	var bestM *matrix
	for mask := 0; mask < 8; mask++ {
		m := newMatrix(size)
		for y := range m.dark {
			copy(m.dark[y], base.dark[y])
			copy(m.fn[y], base.fn[y])
		}
		m.applyMask(mask)
		m.format(mask)
		if s := m.penalty(); best < 0 || s < bestScore {
			best, bestScore, bestM = mask, s, m
		}
	}
	return &Code{Version: v, Size: size, Dark: bestM.dark}
}

// Lines renders the code as signal-cli prints it: a 4-module quiet zone,
// two module rows per line, ' ' for dark over dark, '▄' for dark over
// light, '▀' for light over dark and '█' for light over light.
func (c *Code) Lines() []string {
	const quiet = 4
	n := c.Size + 2*quiet
	dark := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		return x >= 0 && y >= 0 && x < c.Size && y < c.Size && c.Dark[y][x]
	}
	var out []string
	for y := 0; y < n; y += 2 {
		line := make([]rune, n)
		for x := 0; x < n; x++ {
			up, down := dark(x, y), y+1 < n && dark(x, y+1)
			switch {
			case up && down:
				line[x] = ' '
			case up:
				line[x] = '▄'
			case down:
				line[x] = '▀'
			default:
				line[x] = '█'
			}
		}
		out = append(out, string(line))
	}
	return out
}
