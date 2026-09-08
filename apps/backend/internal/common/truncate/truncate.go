// Package truncate provides dependency-neutral rune-safe byte truncation.
package truncate

// UTF8 returns s truncated to at most maxBytes, cutting at a rune boundary.
// When the cut would split a multi-byte rune, the incomplete rune is dropped.
func UTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
