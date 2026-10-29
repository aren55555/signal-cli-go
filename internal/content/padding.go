// SPDX-License-Identifier: GPL-3.0-or-later

// Package content holds Signal's application-level content messages and
// their transport padding (spec/service/content.md).
package content

// Pad applies Signal's transport padding to a serialized Content: the
// bytes, 0x80, then zeros up to the smallest length of the form 80k-1
// (spec/service/content.md §2). The 1:1 cipher's PKCS#7 then adds exactly
// one byte.
func Pad(b []byte) []byte {
	p := (len(b)+2+79)/80*80 - 1
	out := make([]byte, p)
	copy(out, b)
	out[len(b)] = 0x80
	return out
}

// Unpad strips trailing zeros and the 0x80 terminator. If the padding is
// malformed it returns the whole buffer and false, as signal-service-java
// does (it logs and uses the unstripped bytes). An empty or all-zero
// buffer gives empty content and false.
func Unpad(b []byte) ([]byte, bool) {
	i := len(b) - 1
	for i >= 0 && b[i] == 0 {
		i--
	}
	switch {
	case i < 0:
		return nil, false
	case b[i] != 0x80:
		return b, false
	}
	return b[:i], true
}
