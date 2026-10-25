package failedinbox

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeAndTruncateReason_ValidShortReasonUnchanged(t *testing.T) {
	if got := SanitizeAndTruncateReason("connection refused"); got != "connection refused" {
		t.Errorf("got %q", got)
	}
}

func TestSanitizeAndTruncateReason_Empty(t *testing.T) {
	if got := SanitizeAndTruncateReason(""); got != "" {
		t.Errorf("got %q", got)
	}
}

// AC-UI-INBOX-FAILED-001.19a: invalid bytes become the replacement
// character, one per invalid byte, never a broken partial character.
func TestSanitizeAndTruncateReason_InvalidUTF8Replaced(t *testing.T) {
	raw := "before\xff\xfeafter"
	got := SanitizeAndTruncateReason(raw)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got)
	}
	want := "before��after"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// AC-UI-INBOX-FAILED-001.19a: truncation is to at most 512 Unicode CODE
// POINTS, never bytes, and never splits a code point -- a multi-byte script
// must still yield exactly 512 runes and no partial trailing character.
func TestSanitizeAndTruncateReason_TruncatesToCodePointsNotBytes(t *testing.T) {
	// Each "日" is 3 bytes in UTF-8; 600 of them is 1800 bytes but only 600
	// code points.
	raw := strings.Repeat("日", 600)
	got := SanitizeAndTruncateReason(raw)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8")
	}
	gotCount := utf8.RuneCountInString(got)
	if gotCount != 512 {
		t.Fatalf("expected exactly 512 code points, got %d", gotCount)
	}
	want := strings.Repeat("日", 512)
	if got != want {
		t.Errorf("got %q, want first 512 repeats of 日", got)
	}
}

func TestSanitizeAndTruncateReason_ExactBoundaryUnchanged(t *testing.T) {
	raw := strings.Repeat("a", 512)
	got := SanitizeAndTruncateReason(raw)
	if got != raw {
		t.Errorf("expected a 512-code-point reason to be returned unchanged")
	}
}

func TestSanitizeAndTruncateReason_SanitizesBeforeTruncating(t *testing.T) {
	// A long run of invalid bytes followed by valid content: sanitizing
	// FIRST means the invalid bytes count toward the 512-code-point budget
	// as individual replacement characters, not as raw bytes that would
	// otherwise never reach the truncation step in a byte-sliced
	// implementation.
	raw := strings.Repeat("\xff", 600)
	got := SanitizeAndTruncateReason(raw)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8")
	}
	if utf8.RuneCountInString(got) != 512 {
		t.Fatalf("expected exactly 512 code points, got %d", utf8.RuneCountInString(got))
	}
}
