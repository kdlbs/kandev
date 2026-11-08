package coordinator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/clarification"
)

func TestClarificationDetailFollowsTheRecordedShape(t *testing.T) {
	target := &ClarificationTarget{Questions: []clarification.Question{
		{ID: "q1", Prompt: "Secret prompt one", Options: []clarification.Option{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}},
		{ID: "q2", Prompt: "Secret prompt two", Options: []clarification.Option{{ID: "a", Label: "Alpha"}}},
		{ID: "q3", Prompt: "Secret prompt three"},
	}}
	draft := answerDraft{Answers: []answerDraftEntry{
		{QuestionID: "q1", OptionIDs: []string{"a", "b"}, CustomText: "ignored free text"},
		{QuestionID: "q3", CustomText: "because"},
	}}
	got := clarificationDetail(target, draft)
	want := "Q1: Alpha, Beta; Q3: because"
	if got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
}

func TestClarificationDetailNumbersByBundlePositionAndTruncates(t *testing.T) {
	target := &ClarificationTarget{Questions: []clarification.Question{{ID: "q1"}, {ID: "q2"}}}
	draft := answerDraft{Answers: []answerDraftEntry{{QuestionID: "q2", CustomText: strings.Repeat("x", 300)}}}
	got := clarificationDetail(target, draft)
	if !strings.HasPrefix(got, "Q2: xxx") || len([]rune(got)) != answerDetailRunes {
		t.Fatalf("detail = %q (%d runes)", got, len([]rune(got)))
	}
}

func TestPermissionDetailIsTheOptionNameOnly(t *testing.T) {
	target := &PermissionTarget{Title: "Run rm -rf", Options: []PermissionOptionTarget{{ID: "allow", Name: "Allow once"}, {ID: "deny", Name: "Deny"}}}
	if got := permissionDetail(target, answerDraft{OptionID: "deny"}); got != "Deny" {
		t.Fatalf("detail = %q", got)
	}
}
