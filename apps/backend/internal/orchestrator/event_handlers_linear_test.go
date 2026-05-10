package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/linear"
)

func TestInterpolateLinearPrompt(t *testing.T) {
	issue := &linear.LinearIssue{
		Identifier:    "ENG-7",
		Title:         "Login fails on mobile",
		URL:           "https://linear.app/acme/issue/ENG-7",
		StateName:     "In Progress",
		PriorityLabel: "High",
		TeamKey:       "ENG",
		AssigneeName:  "Alice",
		CreatorName:   "Bob",
		Description:   "Tap submit, nothing happens.",
	}

	// Empty template falls back to the embedded default; assert on the load-
	// bearing fields rather than the full string so editing the markdown
	// doesn't break the test.
	got := interpolateLinearPrompt("", issue)
	if !strings.Contains(got, "ENG-7") {
		t.Errorf("default template missing identifier: %q", got)
	}
	if !strings.Contains(got, "https://linear.app/acme/issue/ENG-7") {
		t.Errorf("default template missing URL: %q", got)
	}

	// All placeholders interpolate.
	got = interpolateLinearPrompt(
		"{{issue.identifier}} | {{issue.title}} | {{issue.state}} | {{issue.priority}} | {{issue.team}} | {{issue.assignee}} | {{issue.creator}}",
		issue,
	)
	want := "ENG-7 | Login fails on mobile | In Progress | High | ENG | Alice | Bob"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
