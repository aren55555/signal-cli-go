// SPDX-License-Identifier: GPL-3.0-or-later

// Package javahash reproduces the iteration order of Java's HashSet and
// HashMap for outputs the reference builds with them
// (spec/surface/jsonrpc/serialization.md §1.1).
package javahash

import (
	"slices"
	"unicode/utf16"
)

// String is String.hashCode over UTF-16 code units.
func String(s string) int32 {
	var h int32
	for _, u := range utf16.Encode([]rune(s)) {
		h = 31*h + int32(u)
	}
	return h
}

// Optional is Optional<String>.hashCode: 0 when empty ("").
func Optional(s string) int32 {
	if s == "" {
		return 0
	}
	return String(s)
}

// Nullable is Objects.hashCode of a String that may be null ("").
func Nullable(s string) int32 { return Optional(s) }

// Bool is Boolean.hashCode.
func Bool(b bool) int32 {
	if b {
		return 1231
	}
	return 1237
}

// Long is Long.hashCode.
func Long(v int64) int32 { return int32(v ^ int64(uint64(v)>>32)) }

// Record is a record's generated hashCode over its component hashes.
func Record(components ...int32) int32 {
	var r int32
	for _, c := range components {
		r = 31*r + c
	}
	return r
}

// capacity is the final table size for n distinct elements.
func capacity(n int) int32 {
	c := int32(16)
	for float64(n) > 0.75*float64(c) {
		c *= 2
	}
	return c
}

// Order returns items, deduplicated by key (the first occurrence wins),
// in the iteration order of a HashSet filled in the given order.
func Order[T any](items []T, hash func(T) int32, key func(T) string) []T {
	var out []T
	var hashes []int32
	seen := map[string]bool{}
	for _, it := range items {
		k := key(it)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, it)
		hashes = append(hashes, hash(it))
	}
	c := capacity(len(out))
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	bucket := func(h int32) int32 { return (h ^ int32(uint32(h)>>16)) & (c - 1) }
	slices.SortStableFunc(idx, func(x, y int) int { return int(bucket(hashes[x])) - int(bucket(hashes[y])) })
	sorted := make([]T, len(out))
	for i, j := range idx {
		sorted[i] = out[j]
	}
	return sorted
}
