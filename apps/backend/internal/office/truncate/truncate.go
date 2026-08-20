// Package truncate holds the rune-safe byte truncation shared by the
// office continuation-summary builder and its repository. Both cap the
// same blob at the same byte budget, so the cut belongs in one place:
// the two copies this replaces were byte-for-byte identical, which is
// how a future correctness fix lands in one and not the other.
package truncate

// UTF8 returns s truncated to at most maxBytes, cutting at a rune
// boundary. When the cut would split a multi-byte rune the straddling
// bytes are dropped, so the result is always valid UTF-8.
func UTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes to the start of the last fully-included
	// rune. UTF-8 continuation bytes match 10xxxxxx (0x80..0xBF); the
	// loop terminates as soon as we land on a non-continuation byte.
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
