// SPDX-License-Identifier: GPL-3.0-or-later

package attachment

import "unicode/utf8"

// MaxBodyBytes is the longest body sent without a long-text attachment.
const MaxBodyBytes = 2000

// LongTextType is the content type of a long-text attachment.
const LongTextType = "text/x-signal-plain"

// SplitLongText returns the body to send: text itself if it fits in 2000
// UTF-8 bytes, else its longest prefix of whole code points that does
// (§6.5); split reports the second case.
func SplitLongText(text string) (body string, split bool) {
	if len(text) <= MaxBodyBytes {
		return text, false
	}
	n := 0
	for n < len(text) {
		_, size := utf8.DecodeRuneInString(text[n:])
		if n+size > MaxBodyBytes {
			break
		}
		n += size
	}
	return text[:n], true
}
