package controller

import (
	"fmt"
	"unicode/utf8"
)

// Template context ceilings are measured in UTF-8 bytes.
const (
	MaxGitDiffBytes             = 100_000
	MaxChangedFilesBytes        = 20_000
	MaxCommitLogBytes           = 20_000
	MaxTaskDescriptionBytes     = 20_000
	MaxConversationHistoryBytes = 200_000
)

const truncationMarkerFormat = "\n[... truncated %d bytes ...]"

// truncateContext keeps the leading text within max bytes and appends a marker
// with the number of omitted bytes. A fitting string is returned unchanged.
func truncateContext(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	cut := max
	for {
		marker := fmt.Sprintf(truncationMarkerFormat, len(s)-cut)
		if len(marker) > max {
			return ""
		}
		next := utf8PrefixLength(s, max-len(marker))
		if next == cut {
			return s[:cut] + marker
		}
		cut = next
	}
}

// truncateConversationHistory keeps the newest text within the history cap.
// Its marker precedes the retained text and reports the omitted byte count.
func truncateConversationHistory(s string) string {
	if len(s) <= MaxConversationHistoryBytes {
		return s
	}
	cut := len(s) - MaxConversationHistoryBytes
	for {
		marker := fmt.Sprintf("[... truncated %d bytes ...]", cut)
		available := MaxConversationHistoryBytes - len(marker) - 1
		if available < 0 {
			return ""
		}
		next := len(s) - available
		for next < len(s) && !utf8.RuneStart(s[next]) {
			next++
		}
		if next == cut {
			return marker + "\n" + s[cut:]
		}
		cut = next
	}
}

func utf8PrefixLength(s string, max int) int {
	if max >= len(s) {
		return len(s)
	}
	if max <= 0 {
		return 0
	}
	for !utf8.RuneStart(s[max]) {
		max--
	}
	return max
}
