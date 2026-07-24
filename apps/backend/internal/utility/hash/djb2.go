// Package hash provides the small non-cryptographic hashes kandev shares
// between the backend and the web client.
package hash

import (
	"strconv"
	"unicode/utf16"
)

// DJB2 is the djb2 variant used by `apps/web/lib/utils/hash.ts`. Both halves
// hash the same normalized diff text — the backend stamps
// `task_review_findings.file_diff_hash` and `session_file_reviews.diff_hash`,
// the client recomputes it to decide whether a diff moved — so the two
// implementations must agree bit for bit.
//
// Two details make it match the JavaScript original rather than a naive port:
// the accumulator wraps as a signed 32-bit integer (JS `| 0`), and iteration is
// over UTF-16 code units because `String.prototype.charCodeAt` yields
// surrogates for astral-plane runes, not Go's decoded runes.
func DJB2(s string) string {
	var h int32 = 5381
	for _, unit := range utf16.Encode([]rune(s)) {
		h = h<<5 + h + int32(unit)
	}
	return strconv.FormatUint(uint64(uint32(h)), 16)
}
