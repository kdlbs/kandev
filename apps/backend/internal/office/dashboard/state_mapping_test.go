package dashboard

import "testing"

// TestDbStateToOfficeStatus locks down the uppercase-DB → lowercase-API
// mapping. A typo in any case silently returns the raw DB string to the
// frontend, breaking status-picker aria-selected comparisons and
// STATUS_LABELS lookups for all tasks.
func TestDbStateToOfficeStatus(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// uppercase DB constants → canonical lowercase
		{"COMPLETED", "done"},
		{"IN_PROGRESS", "in_progress"},
		{"REVIEW", "in_review"},
		{"TODO", "todo"},
		{"BLOCKED", "blocked"},
		{"CANCELLED", "cancelled"},
		{"BACKLOG", "backlog"},
		// already-lowercase values pass through to themselves
		{"done", "done"},
		{"in_progress", "in_progress"},
		{"in_review", "in_review"},
		{"review", "in_review"},
		{"todo", "todo"},
		{"blocked", "blocked"},
		{"cancelled", "cancelled"},
		{"backlog", "backlog"},
		// empty falls back to backlog so the frontend has a defined value
		{"", "backlog"},
		// unknown values pass through unchanged (default branch)
		{"UNKNOWN", "UNKNOWN"},
	}
	for _, c := range cases {
		if got := dbStateToOfficeStatus(c.in); got != c.want {
			t.Errorf("dbStateToOfficeStatus(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
