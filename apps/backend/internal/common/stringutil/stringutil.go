// Package stringutil provides common string utility functions.
package stringutil

// TruncateString truncates a string to a maximum length.
// If the string is shorter than maxLen, it returns the original string.
// If the string is longer, it returns the first maxLen characters.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// TruncateStringWithEllipsis truncates a string to a maximum length and adds "..." suffix.
// If the string is shorter than maxLen, it returns the original string.
// If the string is longer, it returns the first (maxLen-3) characters followed by "...".
func TruncateStringWithEllipsis(s string, maxLen int) string {
	if maxLen < 4 {
		return TruncateString(s, maxLen)
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

