package sysprompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- InterpolateStepEntryNumber tests (REQ-TWS-001) ---

func TestInterpolateStepEntryNumber_SingleOccurrence(t *testing.T) {
	result := InterpolateStepEntryNumber("this is entry {step_entry_number}", 5)
	assert.Equal(t, "this is entry 5", result)
}

func TestInterpolateStepEntryNumber_EveryOccurrenceReplaced(t *testing.T) {
	result := InterpolateStepEntryNumber("{step_entry_number} and {step_entry_number}", 3)
	assert.Equal(t, "3 and 3", result)
}

func TestInterpolateStepEntryNumber_NoOccurrence(t *testing.T) {
	result := InterpolateStepEntryNumber("no token here", 5)
	assert.Equal(t, "no token here", result)
}

func TestInterpolateStepEntryNumber_DoubleBraceRendersOuterBracesLiteral(t *testing.T) {
	// AC-TWS-001.10: only the exact literal {step_entry_number} is recognized.
	// {{step_entry_number}} renders as {5} on entry 5 — the inner literal
	// matches, the outer braces remain.
	result := InterpolateStepEntryNumber("{{step_entry_number}}", 5)
	assert.Equal(t, "{5}", result)
}

func TestInterpolateStepEntryNumber_Base10NoSeparatorNoSign(t *testing.T) {
	result := InterpolateStepEntryNumber("{step_entry_number}", 12)
	assert.Equal(t, "12", result)
}
