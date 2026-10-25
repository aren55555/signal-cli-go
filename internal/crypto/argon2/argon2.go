// Package argon2 implements Argon2 version 0x13 (RFC 9106): Argon2d,
// Argon2i and Argon2id, with secret and associated data. Signal's PIN
// hashing uses Argon2id.
package argon2

import (
	"encoding/binary"
	"errors"
	"math/bits"

	"github.com/aren55555/signal-cli-go/internal/crypto/blake2b"
)

// Type is the Argon2 variant y.
type Type uint32

const (
	Argon2d  Type = 0
	Argon2i  Type = 1
	Argon2id Type = 2
)

const (
	version    = 0x13
	syncPoints = 4   // SL, slices per pass
	blockWords = 128 // 1024-byte blocks as 64-bit words
)

type block [blockWords]uint64

// Params are the cost parameters: memory in KiB (blocks), passes, lanes,
// and the tag length in bytes.
type Params struct {
	Memory, Passes, Lanes, TagLen uint32
}

// Key computes the Argon2 tag of password with salt, an optional secret
// K and associated data X.
func Key(y Type, password, salt, secret, ad []byte, p Params) ([]byte, error) {
	if p.Lanes < 1 || p.Passes < 1 || p.TagLen < 4 || p.Memory < 8*p.Lanes {
		return nil, errors.New("argon2: bad parameters")
	}
	h0 := initialHash(y, password, salt, secret, ad, p)
	mPrime := 4 * p.Lanes * (p.Memory / (4 * p.Lanes))
	q := mPrime / p.Lanes
	mem := make([]block, mPrime)
	lane := func(l uint32) []block { return mem[l*q : (l+1)*q] }

	var seed [72]byte
	copy(seed[:], h0)
	for l := range p.Lanes {
		binary.LittleEndian.PutUint32(seed[68:], l)
		for j := range uint32(2) {
			binary.LittleEndian.PutUint32(seed[64:], j)
			lane(l)[j] = blockFromBytes(hPrime(1024, seed[:]))
		}
	}

	segLen := q / syncPoints
	for pass := range p.Passes {
		for slice := range uint32(syncPoints) {
			for l := range p.Lanes {
				fillSegment(mem, y, p, mPrime, q, segLen, pass, slice, l)
			}
		}
	}

	var c block
	for l := range p.Lanes {
		last := &lane(l)[q-1]
		for i := range c {
			c[i] ^= last[i]
		}
	}
	return hPrime(p.TagLen, c.bytes()), nil
}

func initialHash(y Type, password, salt, secret, ad []byte, p Params) []byte {
	d, _ := blake2b.New(64, nil)
	le := func(v uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		d.Write(b[:])
	}
	for _, v := range []uint32{p.Lanes, p.TagLen, p.Memory, p.Passes, version, uint32(y)} {
		le(v)
	}
	for _, s := range [][]byte{password, salt, secret, ad} {
		le(uint32(len(s)))
		d.Write(s)
	}
	return d.Sum(nil)
}

// hPrime is the variable-length hash H' (RFC 9106 §3.3).
func hPrime(t uint32, in []byte) []byte {
	var lenT [4]byte
	binary.LittleEndian.PutUint32(lenT[:], t)
	if t <= 64 {
		d, _ := blake2b.New(int(t), nil)
		d.Write(lenT[:])
		d.Write(in)
		return d.Sum(nil)
	}
	out := make([]byte, 0, t)
	d, _ := blake2b.New(64, nil)
	d.Write(lenT[:])
	d.Write(in)
	v := d.Sum(nil)
	r := (t+31)/32 - 2
	for i := uint32(1); i <= r; i++ { // W_1..W_r; v is V_i
		out = append(out, v[:32]...)
		if i < r {
			v = blake2b.Sum(64, v)
		}
	}
	// V_{r+1} = H^(T-32r)(V_r).
	return append(out, blake2b.Sum(int(t-32*r), v)...)
}

func fillSegment(mem []block, y Type, p Params, mPrime, q, segLen, pass, slice, l uint32) {
	independent := y == Argon2i || (y == Argon2id && pass == 0 && slice < 2)
	var addr, input, zero block
	if independent {
		input[0], input[1], input[2], input[3] = uint64(pass), uint64(l), uint64(slice), uint64(mPrime)
		input[4], input[5] = uint64(p.Passes), uint64(y)
	}
	nextAddresses := func() {
		input[6]++
		addr = input
		compress(&addr, &zero, &addr, false)
		compress(&addr, &zero, &addr, false)
	}
	start := uint32(0)
	if pass == 0 && slice == 0 {
		start = 2
	}
	if independent && start != 0 {
		nextAddresses()
	}
	for idx := start; idx < segLen; idx++ {
		j := slice*segLen + idx
		prev := j - 1
		if j == 0 {
			prev = q - 1
		}
		cur := &mem[l*q+j]
		prevBlk := &mem[l*q+prev]

		var rnd uint64
		if independent {
			if idx%blockWords == 0 {
				nextAddresses()
			}
			rnd = addr[idx%blockWords]
		} else {
			rnd = prevBlk[0]
		}
		j1, j2 := rnd&0xffffffff, rnd>>32

		refLane := uint32(j2 % uint64(p.Lanes))
		if pass == 0 && slice == 0 {
			refLane = l
		}
		// Size of the reference window W (§3.4.2).
		var area uint32
		sameLane := refLane == l
		switch {
		case pass == 0 && sameLane:
			area = slice*segLen + idx - 1
		case pass == 0:
			area = slice * segLen
			if idx == 0 {
				area--
			}
		case sameLane:
			area = q - segLen + idx - 1
		default:
			area = q - segLen
			if idx == 0 {
				area--
			}
		}
		x := (j1 * j1) >> 32
		yy := (uint64(area) * x) >> 32
		rel := uint64(area) - 1 - yy
		winStart := uint64(0)
		if pass != 0 && slice != syncPoints-1 {
			winStart = uint64((slice + 1) * segLen)
		}
		z := uint32((winStart + rel) % uint64(q))
		compress(cur, prevBlk, &mem[refLane*q+z], pass != 0)
	}
}

// compress sets out = G(x, y), or out ^= G(x, y) when xor is set
// (passes after the first, version 0x13). out may alias x.
func compress(out, x, y *block, xor bool) {
	var r, q block
	for i := range r {
		r[i] = x[i] ^ y[i]
	}
	q = r
	for i := range 8 { // rows: 16 consecutive words
		permute(&q, 16*i, 16*i+1, 16*i+2, 16*i+3, 16*i+4, 16*i+5, 16*i+6, 16*i+7,
			16*i+8, 16*i+9, 16*i+10, 16*i+11, 16*i+12, 16*i+13, 16*i+14, 16*i+15)
	}
	for i := range 8 { // columns: register pairs 2i, 2i+1 of every row
		b := 2 * i
		permute(&q, b, b+1, b+16, b+17, b+32, b+33, b+48, b+49,
			b+64, b+65, b+80, b+81, b+96, b+97, b+112, b+113)
	}
	if xor {
		for i := range out {
			out[i] ^= q[i] ^ r[i]
		}
	} else {
		for i := range out {
			out[i] = q[i] ^ r[i]
		}
	}
}

func fBlaMka(a, b uint64) uint64 {
	return a + b + 2*uint64(uint32(a))*uint64(uint32(b))
}

func gb(v *block, a, b, c, d int) {
	v[a] = fBlaMka(v[a], v[b])
	v[d] = bits.RotateLeft64(v[d]^v[a], -32)
	v[c] = fBlaMka(v[c], v[d])
	v[b] = bits.RotateLeft64(v[b]^v[c], -24)
	v[a] = fBlaMka(v[a], v[b])
	v[d] = bits.RotateLeft64(v[d]^v[a], -16)
	v[c] = fBlaMka(v[c], v[d])
	v[b] = bits.RotateLeft64(v[b]^v[c], -63)
}

// permute is P (§3.6) on the 16 words at the given indices, read as the
// 4x4 matrix v_0..v_15.
func permute(v *block, i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15 int) {
	gb(v, i0, i4, i8, i12)
	gb(v, i1, i5, i9, i13)
	gb(v, i2, i6, i10, i14)
	gb(v, i3, i7, i11, i15)
	gb(v, i0, i5, i10, i15)
	gb(v, i1, i6, i11, i12)
	gb(v, i2, i7, i8, i13)
	gb(v, i3, i4, i9, i14)
}

func blockFromBytes(b []byte) block {
	var out block
	for i := range out {
		out[i] = binary.LittleEndian.Uint64(b[8*i:])
	}
	return out
}

func (b *block) bytes() []byte {
	out := make([]byte, 1024)
	for i, w := range b {
		binary.LittleEndian.PutUint64(out[8*i:], w)
	}
	return out
}
