package service

import (
	"strings"
	"testing"
)

// TestPlanTruncationDetectedBoundaries pins the exact-boundary behavior from
// AC-001.4: retaining at least half, or starting below the 2000-char floor,
// is never truncation - only strictly less than half of a prior document at
// or above the floor counts.
func TestPlanTruncationDetectedBoundaries(t *testing.T) {
	atFloor := strings.Repeat("a", planTruncationMinPriorChars)
	belowFloor := strings.Repeat("a", planTruncationMinPriorChars-1)

	tests := []struct {
		name    string
		prior   string
		next    string
		wantHit bool
	}{
		{
			name:    "below floor with severe shrink is not truncation",
			prior:   belowFloor,
			next:    "a",
			wantHit: false,
		},
		{
			name:    "at floor retaining exactly half is not truncation",
			prior:   atFloor,
			next:    strings.Repeat("a", planTruncationMinPriorChars/2),
			wantHit: false,
		},
		{
			name:    "at floor retaining one rune under half is truncation",
			prior:   atFloor,
			next:    strings.Repeat("a", planTruncationMinPriorChars/2-1),
			wantHit: true,
		},
		{
			name:    "growing content is never truncation",
			prior:   atFloor,
			next:    atFloor + atFloor,
			wantHit: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planTruncationDetected(tt.prior, tt.next); got != tt.wantHit {
				t.Errorf("planTruncationDetected() = %v, want %v", got, tt.wantHit)
			}
		})
	}
}

// TestPlanTruncationDetectedCountsRunesNotBytes pins AC-001.1: a script
// change (e.g. ASCII rewritten in CJK) is measured by rune count, not byte
// length, so it isn't mistaken for a severe drop when the character count is
// actually preserved.
func TestPlanTruncationDetectedCountsRunesNotBytes(t *testing.T) {
	prior := strings.Repeat("a", planTruncationMinPriorChars)
	// Each CJK rune is 3 bytes in UTF-8, so this has 3x the bytes of prior but
	// the same rune count - must not be flagged as truncation.
	next := strings.Repeat("字", planTruncationMinPriorChars)

	if planTruncationDetected(prior, next) {
		t.Fatal("planTruncationDetected reported truncation for a same-rune-count script change")
	}
}
