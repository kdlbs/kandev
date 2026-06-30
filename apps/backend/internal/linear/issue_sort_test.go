package linear

import (
	"testing"
	"time"
)

// identifiers extracts the issue identifiers in slice order for assertions.
func identifiers(issues []*LinearIssue) []string {
	out := make([]string, len(issues))
	for i, is := range issues {
		out[i] = is.Identifier
	}
	return out
}

func equalOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSortIssues(t *testing.T) {
	// Priority fixtures: one issue per Linear priority value, in a deliberately
	// scrambled input order so the sort has work to do.
	priorityIssues := func() []*LinearIssue {
		return []*LinearIssue{
			{Identifier: "LOW", Priority: 4},
			{Identifier: "URGENT", Priority: 1},
			{Identifier: "NONE", Priority: 0},
			{Identifier: "MED", Priority: 3},
			{Identifier: "HIGH", Priority: 2},
		}
	}
	// Time fixtures: identifiers ordered by both created and updated for clarity.
	const (
		t1 = "2026-01-01T00:00:00Z"
		t2 = "2026-02-01T00:00:00Z"
		t3 = "2026-03-01T00:00:00Z"
	)
	timeIssues := func() []*LinearIssue {
		return []*LinearIssue{
			{Identifier: "B", Created: t2, Updated: t2},
			{Identifier: "A", Created: t1, Updated: t1},
			{Identifier: "C", Created: t3, Updated: t3},
		}
	}

	cases := []struct {
		name  string
		input []*LinearIssue
		by    IssueSortBy
		want  []string
	}{
		{"priority desc", priorityIssues(), SortByPriorityDesc, []string{"URGENT", "HIGH", "MED", "LOW", "NONE"}},
		{"priority asc", priorityIssues(), SortByPriorityAsc, []string{"NONE", "LOW", "MED", "HIGH", "URGENT"}},
		{"created desc", timeIssues(), SortByCreatedDesc, []string{"C", "B", "A"}},
		{"created asc", timeIssues(), SortByCreatedAsc, []string{"A", "B", "C"}},
		{"updated desc", timeIssues(), SortByUpdatedDesc, []string{"C", "B", "A"}},
		{"updated asc", timeIssues(), SortByUpdatedAsc, []string{"A", "B", "C"}},
		// Default (empty) leaves input order untouched.
		{"default unchanged", timeIssues(), SortByDefault, []string{"B", "A", "C"}},
		// Unknown key is treated like default: no reordering.
		{"unknown unchanged", timeIssues(), IssueSortBy("bogus"), []string{"B", "A", "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sortIssues(tc.input, tc.by)
			if got := identifiers(tc.input); !equalOrder(got, tc.want) {
				t.Errorf("sortIssues(%s) = %v, want %v", tc.by, got, tc.want)
			}
		})
	}
}

func TestSortIssues_Stable(t *testing.T) {
	// Two equal-priority issues must keep their input relative order.
	issues := []*LinearIssue{
		{Identifier: "FIRST", Priority: 2},
		{Identifier: "SECOND", Priority: 2},
		{Identifier: "URGENT", Priority: 1},
	}
	sortIssues(issues, SortByPriorityDesc)
	want := []string{"URGENT", "FIRST", "SECOND"}
	if got := identifiers(issues); !equalOrder(got, want) {
		t.Errorf("stable sort = %v, want %v", got, want)
	}
}

func TestPriorityRank(t *testing.T) {
	// urgent(1) < high(2) < medium(3) < low(4) < none(0→5).
	if priorityRank(0) <= priorityRank(4) {
		t.Errorf("none should rank after low: none=%d low=%d", priorityRank(0), priorityRank(4))
	}
	if priorityRank(1) >= priorityRank(2) {
		t.Errorf("urgent should rank before high: urgent=%d high=%d", priorityRank(1), priorityRank(2))
	}
}

func TestParseLinearTime(t *testing.T) {
	if !parseLinearTime("").IsZero() {
		t.Error("empty input should parse to zero time")
	}
	if !parseLinearTime("not-a-date").IsZero() {
		t.Error("garbage input should parse to zero time")
	}
	got := parseLinearTime("2026-01-02T03:04:05Z")
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseLinearTime = %v, want %v", got, want)
	}
	// Linear's real wire format carries milliseconds; time.RFC3339 accepts the
	// fractional second even though the layout omits it. Pins that a stricter
	// layout swap would break.
	gotMs := parseLinearTime("2026-01-02T03:04:05.123Z")
	wantMs := time.Date(2026, 1, 2, 3, 4, 5, 123_000_000, time.UTC)
	if !gotMs.Equal(wantMs) {
		t.Errorf("parseLinearTime(ms) = %v, want %v", gotMs, wantMs)
	}
}
