// SPDX-License-Identifier: GPL-3.0-or-later

package javahash

import "testing"

func TestHashes(t *testing.T) {
	// Values follow String.hashCode (UTF-16 units; 😀 is two).
	for s, want := range map[string]int32{"": 0, "a": 97, "abc": 96354, "groupId": 293428218, "😀": 1772899} {
		if got := String(s); got != want {
			t.Errorf("%q: %d, want %d", s, got, want)
		}
	}
	if Long(-1) != 0 || Long(5) != 5 || Record(1, 2) != 33 {
		t.Error("Long/Record")
	}
}

// The updateGroup result keys iterate groupId, results, timestamp
// (serialization.md §1.3: buckets 7, 13, 15 at capacity 16).
func TestOrder(t *testing.T) {
	got := Order([]string{"timestamp", "results", "groupId"}, String, func(s string) string { return s })
	if got[0] != "groupId" || got[1] != "results" || got[2] != "timestamp" {
		t.Errorf("%v", got)
	}
	if d := Order([]string{"a", "a", "b"}, String, func(s string) string { return s }); len(d) != 2 {
		t.Errorf("dedup %v", d)
	}
}
