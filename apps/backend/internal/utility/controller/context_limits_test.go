package controller

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateContextLeavesShortValuesAlone(t *testing.T) {
	const value = "short diff"
	if got := truncateContext(value, MaxGitDiffBytes); got != value {
		t.Fatalf("truncateContext() = %q, want %q", got, value)
	}
}

func TestTruncateContextCutsAndMarks(t *testing.T) {
	value := strings.Repeat("a", 120)

	got := truncateContext(value, 100)

	if len(got) > 100 {
		t.Fatalf("truncated value is %d bytes, cap is 100", len(got))
	}
	body, marker, found := strings.Cut(got, "\n[... truncated")
	if !found || !strings.HasPrefix(value, body) {
		t.Fatalf("truncated value does not keep a marked prefix: %q", got)
	}
	wantMarker := fmt.Sprintf(" %d bytes ...]", len(value)-len(body))
	if !strings.HasPrefix(marker, wantMarker) {
		t.Fatalf("marker = %q, want omitted byte count %q", marker, wantMarker)
	}
}

func TestTruncateContextKeepsUTF8Valid(t *testing.T) {
	// Two bytes per rune: a 101-byte cut lands inside the 51st character and
	// must back off to the previous rune boundary.
	value := strings.Repeat("é", 60)

	got := truncateContext(value, 101)

	if !utf8.ValidString(got) {
		t.Fatalf("truncateContext() produced invalid UTF-8: %q", got)
	}
	body, marker, found := strings.Cut(got, "\n[... truncated")
	if !found {
		t.Fatalf("truncated value is missing the marker: %q", got)
	}
	if len(got) > 101 {
		t.Fatalf("truncated value is %d bytes, cap is 101", len(got))
	}
	wantMarker := fmt.Sprintf(" %d bytes ...]", len(value)-len(body))
	if !strings.HasPrefix(marker, wantMarker) {
		t.Fatalf("marker = %q, want omitted byte count %q", marker, wantMarker)
	}
	if !strings.HasPrefix(value, body) {
		t.Fatalf("retained text is not a prefix of the input: %q", body)
	}
}

// browserTruncatedGitDiff mirrors truncateGitDiff in
// apps/web/hooks/use-utility-agent-generator.ts: the browser cuts an oversized
// diff at the backend cap minus a 64-byte marker reserve, backs off to a UTF-8
// boundary, and appends its own marker.
func browserTruncatedGitDiff(diff string) string {
	const markerReserveBytes = 64
	if len(diff) <= MaxGitDiffBytes {
		return diff
	}
	cut := MaxGitDiffBytes - markerReserveBytes
	for cut > 0 && !utf8.RuneStart(diff[cut]) {
		cut--
	}
	return diff[:cut] + fmt.Sprintf(truncationMarkerFormat, len(diff)-cut)
}

// A diff the browser already cut must reach the template with the browser's
// marker and its real count, not a second marker that counts only the first.
func TestTruncateContextKeepsBrowserTruncationMarker(t *testing.T) {
	sent := browserTruncatedGitDiff(strings.Repeat("x", 3_000_000))

	got := truncateContext(sent, MaxGitDiffBytes)

	if got != sent {
		t.Fatalf("server re-truncated the browser diff: got %d bytes ending %q", len(got), got[len(got)-40:])
	}
	if !strings.HasSuffix(got, "\n[... truncated 2900064 bytes ...]") {
		t.Fatalf("browser marker lost: ends with %q", got[len(got)-40:])
	}
}

func TestTruncateContextKeepsUncutBrowserDiff(t *testing.T) {
	diff := strings.Repeat("x", MaxGitDiffBytes)

	if got := truncateContext(browserTruncatedGitDiff(diff), MaxGitDiffBytes); got != diff {
		t.Fatalf("an uncut %d-byte diff changed to %d bytes", len(diff), len(got))
	}
}

func TestTruncateConversationHistoryKeepsNewestMessagesInOrder(t *testing.T) {
	const early = "early message: choose the old plan"
	const late = "late update: keep the revised plan!"
	const latest = "latest decision: use the new API"
	conversation := early + "\n" + strings.Repeat("é", MaxConversationHistoryBytes/2) + "\n" + late + "\n" + latest

	got := truncateConversationHistory(conversation)

	if !utf8.ValidString(got) {
		t.Fatalf("truncated history is invalid UTF-8: %q", got[:min(len(got), 80)])
	}
	if !strings.HasPrefix(got, "[... truncated ") {
		t.Fatalf("truncation marker is not before retained history: %q", got[:min(len(got), 80)])
	}
	if strings.Contains(got, early) {
		t.Fatalf("truncated history kept the oldest message: %q", got[:min(len(got), 120)])
	}
	lateAt, latestAt := strings.Index(got, late), strings.Index(got, latest)
	if lateAt < 0 || latestAt <= lateAt {
		t.Fatalf("newest messages were not retained in chronological order: %q", got[len(got)-100:])
	}
	if len(got) > MaxConversationHistoryBytes {
		t.Fatalf("history with marker is %d bytes, cap is %d", len(got), MaxConversationHistoryBytes)
	}
	markerEnd := strings.IndexByte(got, '\n')
	var omitted int
	if _, err := fmt.Sscanf(got[:markerEnd], "[... truncated %d bytes ...]", &omitted); err != nil {
		t.Fatalf("parse truncation marker: %v", err)
	}
	if omitted < 0 || omitted > len(conversation) {
		t.Fatalf("marker reports invalid omitted-byte count %d", omitted)
	}
	if conversation[omitted:] != got[markerEnd+1:] {
		t.Fatalf("marker reports %d omitted bytes, but retained history is not the matching suffix", omitted)
	}
}
