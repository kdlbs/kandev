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
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf(truncationMarkerFormat, len(s)-cut)
}

// truncateConversationHistory keeps the newest text within the history cap.
// Its marker precedes the retained text and reports the omitted byte count.
func truncateConversationHistory(s string) string {
	if len(s) <= MaxConversationHistoryBytes {
		return s
	}
	cut := len(s) - MaxConversationHistoryBytes
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return fmt.Sprintf("[... truncated %d bytes ...]\n%s", cut, s[cut:])
}
