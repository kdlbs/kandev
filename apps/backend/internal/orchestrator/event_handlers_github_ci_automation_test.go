package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
)

func TestCIAutomationShouldAutoFix(t *testing.T) {
	tests := []struct {
		name string
		pr   *github.TaskPR
		want bool
	}{
		{name: "failed checks", pr: &github.TaskPR{ChecksState: "failure"}, want: true},
		{name: "changes requested", pr: &github.TaskPR{ReviewState: "changes_requested"}, want: true},
		{name: "unresolved threads", pr: &github.TaskPR{UnresolvedReviewThreads: 1}, want: true},
		{name: "passing approved", pr: &github.TaskPR{ChecksState: "success", ReviewState: "approved"}, want: false},
		{name: "closed ignored", pr: &github.TaskPR{State: "closed", ChecksState: "failure"}, want: false},
		{name: "merged ignored", pr: &github.TaskPR{State: "merged", ReviewState: "changes_requested"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ciAutomationShouldAutoFix(tt.pr); got != tt.want {
				t.Fatalf("ciAutomationShouldAutoFix=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIAutomationReadyToMerge(t *testing.T) {
	required := 1
	ready := github.TaskPR{
		State:                   "open",
		ChecksState:             "success",
		ReviewState:             "approved",
		MergeableState:          "clean",
		ReviewCount:             1,
		PendingReviewCount:      0,
		RequiredReviews:         &required,
		UnresolvedReviewThreads: 0,
	}
	tests := []struct {
		name   string
		mutate func(*github.TaskPR)
		want   bool
	}{
		{name: "ready", want: true},
		{name: "failing checks", mutate: func(pr *github.TaskPR) { pr.ChecksState = "failure" }, want: false},
		{name: "dirty", mutate: func(pr *github.TaskPR) { pr.MergeableState = "dirty" }, want: false},
		{name: "pending review", mutate: func(pr *github.TaskPR) { pr.PendingReviewCount = 1 }, want: false},
		{name: "not enough approvals", mutate: func(pr *github.TaskPR) { pr.ReviewCount = 0 }, want: false},
		{name: "unresolved threads", mutate: func(pr *github.TaskPR) { pr.UnresolvedReviewThreads = 1 }, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := ready
			if tt.mutate != nil {
				tt.mutate(&pr)
			}
			if got := ciAutomationReadyToMerge(&pr); got != tt.want {
				t.Fatalf("ciAutomationReadyToMerge=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIAutomationFeedbackDelta(t *testing.T) {
	feedback := &github.PRFeedback{
		Checks: []github.CheckRun{
			{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/1"},
			{Name: "lint", Status: "completed", Conclusion: "success", HTMLURL: "https://ci/2"},
		},
		Comments: []github.PRComment{
			{ID: 10, Body: "fix this", Path: "main.go", Line: 12},
			{ID: 11, Body: "also this", Path: "main.go", Line: 20},
		},
	}
	checkpoint := ciAutomationCheckpoint{
		FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit", Conclusion: "failure", HTMLURL: "https://ci/1"}},
		Comments:     []ciAutomationCommentSnapshot{{ID: 10}},
	}

	delta := ciAutomationBuildDelta(feedback, checkpoint)
	if len(delta.FailedChecks) != 0 {
		t.Fatalf("expected no new failed checks, got %+v", delta.FailedChecks)
	}
	if len(delta.Comments) != 1 || delta.Comments[0].ID != 11 {
		t.Fatalf("expected only comment 11, got %+v", delta.Comments)
	}
	prompt := ciAutomationRenderPrompt("Base instructions", &github.TaskPR{Owner: "acme", Repo: "widget", PRNumber: 42}, delta)
	if !strings.Contains(prompt, "Base instructions") || !strings.Contains(prompt, "acme/widget#42") || !strings.Contains(prompt, "also this") {
		t.Fatalf("rendered prompt missing expected content:\n%s", prompt)
	}
}
