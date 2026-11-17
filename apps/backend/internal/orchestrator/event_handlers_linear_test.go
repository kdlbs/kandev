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

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.1, AC-INTEGRATIONS-LINEAR-PROJECTS-002.2
func TestInterpolateLinearPrompt_ProjectPlaceholders(t *testing.T) {
	issue := &linear.LinearIssue{
		Identifier:         "ENG-7",
		ProjectName:        "Checkout",
		ProjectURL:         "https://linear.app/acme/project/checkout",
		ProjectSummary:     "Faster checkout",
		ProjectDescription: "Audience: shoppers.",
	}
	got := interpolateLinearPrompt(
		"{{issue.project.name}} | {{issue.project.url}} | {{issue.project.summary}} | {{issue.project.description}}",
		issue,
	)
	want := "Checkout | https://linear.app/acme/project/checkout | Faster checkout | Audience: shoppers."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	noProject := interpolateLinearPrompt(
		"[{{issue.project.name}}][{{issue.project.url}}][{{issue.project.summary}}][{{issue.project.description}}] {{issue.identifier}}",
		&linear.LinearIssue{Identifier: "ENG-8"},
	)
	if noProject != "[][][][] ENG-8" {
		t.Errorf("issue without a project must render empty placeholders, got %q", noProject)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.3
func TestInterpolateLinearPrompt_TruncatesProjectDescription(t *testing.T) {
	exact := strings.Repeat("é", linearProjectDescriptionMaxRunes)
	if got := interpolateLinearPrompt("{{issue.project.description}}", &linear.LinearIssue{ProjectDescription: exact}); got != exact {
		t.Errorf("a description at the limit must render unchanged (len %d)", len([]rune(got)))
	}

	long := strings.Repeat("é", linearProjectDescriptionMaxRunes) + "TAIL"
	got := interpolateLinearPrompt("{{issue.project.description}}", &linear.LinearIssue{ProjectDescription: long})
	want := exact + "\n\n[Project description truncated to 4000 characters; see the project page for the full text.]"
	if got != want {
		t.Errorf("truncated description mismatch: tail %q", got[len(got)-120:])
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.4
func TestInterpolateLinearPrompt_DoesNotExpandPlaceholdersInValues(t *testing.T) {
	issue := &linear.LinearIssue{
		Title:              "literal {{issue.project.name}}",
		ProjectName:        "Checkout",
		ProjectDescription: "see {{issue.title}}",
	}
	got := interpolateLinearPrompt("{{issue.title}} / {{issue.project.description}}", issue)
	want := "literal {{issue.project.name}} / see {{issue.title}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
