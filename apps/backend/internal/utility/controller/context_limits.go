package controller

import (
	"fmt"
	"unicode/utf8"
)

// Template context ceilings, in UTF-8 bytes. A utility prompt is assembled from
// session data whose size the caller does not control: a large diff alone can
// exceed a provider's context window and fail the whole call. These caps keep
// the request in a range a model can actually accept.
const (
	MaxGitDiffBytes             = 100_000
	MaxChangedFilesBytes        = 20_000
	MaxCommitLogBytes           = 20_000
	MaxTaskDescriptionBytes     = 20_000
	MaxConversationHistoryBytes = 200_000
)

const truncationMarkerFormat = "\n[... truncated %d bytes ...]"

// truncateContext bounds s to max bytes without splitting a UTF-8 sequence and
// appends a marker naming how many bytes were dropped. A string that already
// fits is returned unchanged.
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
