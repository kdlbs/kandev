package plancomments

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestResolvePlaceholderFormatsCanonicalPlanComments(t *testing.T) {
	template := "system\n" + WithPlaceholder("ordinary context\n\nhello")
	got, err := ResolvePlaceholder(template, []*models.TaskPlanComment{
		{SelectedText: "first line", Body: "clarify this\nand that"},
		{Body: "general feedback"},
	})
	if err != nil {
		t.Fatalf("ResolvePlaceholder: %v", err)
	}
	want := "system\n### Plan Comments\n\n```\nfirst line\n```\n" +
		"> clarify this\n> and that\n\n> general feedback\n\n---\n\nordinary context\n\nhello"
	if got != want {
		t.Fatalf("resolved content:\n%s\nwant:\n%s", got, want)
	}
}

func TestResolvePlaceholderRejectsMissingOrRepeatedMarker(t *testing.T) {
	comments := []*models.TaskPlanComment{{Body: "feedback"}}
	if _, err := ResolvePlaceholder("no marker", comments); err == nil {
		t.Fatal("missing marker was accepted")
	}
	if _, err := ResolvePlaceholder(WithPlaceholder("again "+WithPlaceholder("body")), comments); err == nil {
		t.Fatal("repeated marker was accepted")
	}
}
