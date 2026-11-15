package sqlite

import "testing"

func TestHandoffCompletionAnchorLikePatternEscapesWildcards(t *testing.T) {
	got := handoffCompletionAnchorLikePattern("task%_!id")
	want := "%<!!-- handoff-completion:task!%!_!!id -->%"
	if got != want {
		t.Fatalf("completion marker LIKE pattern = %q, want %q", got, want)
	}
}
