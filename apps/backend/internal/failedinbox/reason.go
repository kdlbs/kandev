// Package failedinbox implements the Inbox Failed tab: a workspace-scoped,
// bounded, read-only projection of failed tasks
// (docs/specs/ui/system-design/inbox-failed-bucket-01.md). It is a separate
// endpoint from the clarification bundle read -- a different table, a
// different question, never merged.
package failedinbox

import (
	"strings"
	"unicode/utf8"
)

// maxReasonCodePoints is AC-UI-INBOX-FAILED-001.19a's bound: enough to stop a
// 4096-byte stored message multiplied by a 200-row page from becoming an
// 800KB payload, while still showing the actual triage content.
const maxReasonCodePoints = 512

// SanitizeAndTruncateReason produces the failure reason exactly as
// AC-UI-INBOX-FAILED-001.19a requires, in the order the AC states -- the
// steps are not commutative. FIRST, every byte that is not part of a
// well-formed UTF-8 sequence is replaced with the Unicode replacement
// character, one per invalid byte, so the result is valid UTF-8 for every
// input including one that is not. SECOND, the result is truncated to at
// most 512 Unicode code points, never bytes, never splitting one.
func SanitizeAndTruncateReason(raw string) string {
	return truncateToCodePoints(sanitizeToValidUTF8(raw), maxReasonCodePoints)
}

// sanitizeToValidUTF8 replaces each invalid byte with the replacement
// character individually (unlike strings.ToValidUTF8, which collapses a run
// of invalid bytes into a single replacement), matching the AC's literal
// "each byte" wording.
func sanitizeToValidUTF8(raw string) string {
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[i:])
		// utf8.DecodeRuneInString already advances by exactly one byte on a
		// malformed sequence (size <= 1 whenever r == RuneError for
		// non-empty input), so writing r as-is replaces each invalid byte
		// individually rather than collapsing a run into one replacement.
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

// truncateToCodePoints cuts s at the byte offset where the (max+1)'th code
// point would start, so the result carries exactly min(max, count) code
// points and never splits one.
func truncateToCodePoints(s string, max int) string {
	count := 0
	for i := range s {
		if count == max {
			return s[:i]
		}
		count++
	}
	return s
}
