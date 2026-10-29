// SPDX-License-Identifier: GPL-3.0-or-later

// Package interop holds what the live group interop runs and their
// offline replay share (spec/testing/groups-interop.md §5, §6): picking
// one envelope's output out of a receive run and normalizing it.
package interop

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Normalizations allowed by the plan (§5): the per-device delivery time
// and the data directory in stored attachment paths.
var (
	plainDelivered = regexp.MustCompile(`delivered: \d+ \([^)]*\)`)
	jsonDelivered  = regexp.MustCompile(`"serverDeliveredTimestamp":\d+`)
	attachmentDir  = regexp.MustCompile(`[^" ]*/attachments/`)
)

// Normalize applies the §5 normalizations.
func Normalize(s string) string {
	s = plainDelivered.ReplaceAllString(s, "delivered: <T>")
	s = jsonDelivered.ReplaceAllString(s, `"serverDeliveredTimestamp":0`)
	return attachmentDir.ReplaceAllString(s, "<DATA>/attachments/")
}

// Block returns the normalized output for the envelopes with timestamp ts
// in a receive run's output, "" if there are none. Plain-text blocks start
// at "Envelope from:"; JSON output is one object per line.
func Block(output []byte, json bool, ts string) string {
	var out []string
	if json {
		for _, line := range strings.Split(string(output), "\n") {
			if envelopeTimestamp(line) == ts {
				out = append(out, Normalize(line))
			}
		}
	} else {
		for _, blk := range strings.Split(string(output), "\nEnvelope from: ") {
			if !strings.HasPrefix(blk, "Envelope from: ") {
				blk = "Envelope from: " + blk
			}
			if strings.Contains(blk, "\nTimestamp: "+ts+" ") {
				out = append(out, Normalize(strings.TrimRight(blk, "\n")))
			}
		}
	}
	return strings.Join(out, "\n\n")
}

func envelopeTimestamp(line string) string {
	var v struct {
		Envelope struct {
			Timestamp json.Number `json:"timestamp"`
		} `json:"envelope"`
	}
	if json.Unmarshal([]byte(line), &v) != nil {
		return ""
	}
	return v.Envelope.Timestamp.String()
}
