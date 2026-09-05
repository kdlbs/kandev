package dynamic

import (
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// followingAnchorBeforeBoundary returns the first credential anchor that
// starts in the run beginning at pos, after any leading whitespace. If the
// window begins shortly before an anchor, skipping the run would otherwise
// exclude the anchor and leave its short value unredacted in the window.
func followingAnchorBeforeBoundary(raw string, pos, limit int) int {
	for pos < limit && isRedactionBoundaryByte(raw[pos]) {
		pos++
	}
	boundary := limit
	for i := pos; i < limit; i++ {
		if isRedactionBoundaryByte(raw[i]) {
			boundary = i
			break
		}
	}
	for _, lit := range anchorLiterals {
		for start := pos; start+len(lit) <= boundary; start++ {
			if strings.EqualFold(raw[start:start+len(lit)], lit) {
				return start
			}
		}
	}
	return -1
}

// skipBisectedRunForward returns pos unchanged unless a window boundary at
// pos would either bisect a literal credential anchor or land just past one
// separated only by its own \s (see precedingAnchorAtRisk) — in which case
// it retreats to the anchor's start so the anchor is included whole rather
// than excluded: excluding it here would leave its value in the window with
// no anchor to trigger its rule, an orphaned value that no other rule
// matches. Failing that, it returns pos unchanged unless pos sits strictly
// inside an ordinary run of non-whitespace bytes that continues across pos
// (i.e. both raw[pos-1] and raw[pos] are non-whitespace) — the signature of
// a token bisected by a window cut landing at pos with no clean line
// boundary nearby. When that happens, it advances to just past the nearest
// whitespace byte at or after pos, excluding the bisected fragment from the
// window entirely. The forward scan is bounded by one redaction window plus
// the edge extension. If it cannot find a clean boundary in that range, it
// drops the remaining input instead of returning a possibly bisected token.
func skipBisectedRunForward(raw string, pos int) int {
	if pos <= 0 || pos >= len(raw) {
		return pos
	}
	if start := precedingAnchorAtRisk(raw, pos); start >= 0 {
		return start
	}
	// A boundary may sit farther than the edge lookback into a whitespace
	// separator. Search through at most one complete redaction window plus the
	// edge extension so the first value can still be discarded without making
	// scan cost depend on the full conversation size.
	limit := pos + maxRedactionInputBytes + redactionLookbackBytes
	if limit > len(raw) {
		limit = len(raw)
	}
	if start := followingAnchorBeforeBoundary(raw, pos, limit); start >= 0 {
		return start
	}
	if isRedactionBoundaryByte(raw[pos-1]) || isRedactionBoundaryByte(raw[pos]) {
		for i := pos; i < limit; i++ {
			if !isRedactionBoundaryByte(raw[i]) {
				for j := i; j < limit; j++ {
					if isRedactionBoundaryByte(raw[j]) {
						return j + 1
					}
				}
				return len(raw)
			}
		}
		return pos
	}
	for i := pos; i < limit; i++ {
		if isRedactionBoundaryByte(raw[i]) {
			return i + 1
		}
	}
	return len(raw)
}

// sanitizeContinuation normalizes both newly built and persisted packages at
// the provider boundary. This protects launches that load a continuation
// written by an older process before the redaction rules were added.
func sanitizeContinuation(continuation Continuation) Continuation {
	return Continuation{
		TaskDescription:   bounded(continuation.TaskDescription),
		WorkflowStep:      bounded(continuation.WorkflowStep),
		Conversation:      sanitizedTail(continuation.Conversation, continuationFieldLimit),
		ToolSummary:       sanitizedHead(continuation.ToolSummary),
		RepositorySummary: bounded(continuation.RepositorySummary),
		PlanSummary:       bounded(continuation.PlanSummary),
		FailureReason:     bounded(routingerr.Sanitize(continuation.FailureReason)),
	}
}
