package handlers

import (
	"github.com/kandev/kandev/internal/common/truncate"
)

// stepCompletionSignalFieldLimitBytes is the fixed, compiled-in ceiling for
// the step_complete_kandev handoff and blockers arguments (REQ-003). It is
// inclusive of stepCompletionSignalTruncationMarker and applies to each field
// independently, so it is never configurable at runtime or per deployment.
const stepCompletionSignalFieldLimitBytes = 8192

// stepCompletionSignalTruncationMarker is appended to a truncated handoff or
// blockers value so a later reader cannot mistake a cut value for a complete
// one. Its byte length (33) is subtracted from the ceiling before cutting.
const stepCompletionSignalTruncationMarker = "[truncated: over 8192-byte limit]"

// boundStepCompletionSignalField bounds an already-trimmed handoff or
// blockers value to stepCompletionSignalFieldLimitBytes, measured as UTF-8
// byte length. A value at or under the ceiling is stored byte-for-byte; a
// larger value is cut on a UTF-8 character boundary and the fixed marker is
// appended, so the stored result is never larger than the ceiling.
func boundStepCompletionSignalField(trimmed string) (bounded string, wasTruncated bool) {
	if len(trimmed) <= stepCompletionSignalFieldLimitBytes {
		return trimmed, false
	}
	prefixLimit := stepCompletionSignalFieldLimitBytes - len(stepCompletionSignalTruncationMarker)
	return truncate.UTF8(trimmed, prefixLimit) + stepCompletionSignalTruncationMarker, true
}
