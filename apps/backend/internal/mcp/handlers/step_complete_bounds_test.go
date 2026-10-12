package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBoundStepCompletionSignalField_Boundary covers AC-003.2's exact
// boundary pair: 8,192 bytes stores whole, 8,193 truncates.
func TestBoundStepCompletionSignalField_Boundary(t *testing.T) {
	atLimit := strings.Repeat("a", stepCompletionSignalFieldLimitBytes)
	bounded, truncated := boundStepCompletionSignalField(atLimit)
	assert.False(t, truncated)
	assert.Equal(t, atLimit, bounded)
	assert.Len(t, bounded, stepCompletionSignalFieldLimitBytes)

	overLimit := strings.Repeat("a", stepCompletionSignalFieldLimitBytes+1)
	bounded, truncated = boundStepCompletionSignalField(overLimit)
	assert.True(t, truncated)
	assert.LessOrEqual(t, len(bounded), stepCompletionSignalFieldLimitBytes)
	assert.True(t, strings.HasSuffix(bounded, stepCompletionSignalTruncationMarker))
}

// TestBoundStepCompletionSignalField_MultiByteBoundary ensures a cut landing
// mid-rune is avoided: the retained prefix must be valid UTF-8 and the total
// length (marker included) must stay within the ceiling (AC-003.3).
func TestBoundStepCompletionSignalField_MultiByteBoundary(t *testing.T) {
	// Each rune is 3 bytes (e.g. "世"), chosen so a naive byte-count cut would
	// very likely split a character.
	rune3 := "世"
	require.Len(t, rune3, 3)
	overLimit := strings.Repeat(rune3, (stepCompletionSignalFieldLimitBytes/3)+50)

	bounded, truncated := boundStepCompletionSignalField(overLimit)
	require.True(t, truncated)
	assert.LessOrEqual(t, len(bounded), stepCompletionSignalFieldLimitBytes)
	assert.True(t, strings.HasSuffix(bounded, stepCompletionSignalTruncationMarker))
	prefix := strings.TrimSuffix(bounded, stepCompletionSignalTruncationMarker)
	assert.True(t, utf8ValidString(prefix), "retained prefix must not split a multi-byte character")
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// TestBoundStepCompletionSignalField_WellUnderLimit is the trivial pass-
// through case: a short value is stored unchanged with no marker.
func TestBoundStepCompletionSignalField_WellUnderLimit(t *testing.T) {
	bounded, truncated := boundStepCompletionSignalField("short handoff")
	assert.False(t, truncated)
	assert.Equal(t, "short handoff", bounded)
}
