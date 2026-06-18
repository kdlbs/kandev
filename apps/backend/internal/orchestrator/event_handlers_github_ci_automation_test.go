package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
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
		Comments:     []ciAutomationCommentSnapshot{{ID: 10, Body: "fix this", Path: "main.go", Line: 12}},
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

func TestCIAutomationCheckpointPrunesResolvedFailures(t *testing.T) {
	failed := &github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/stable"}},
	}
	previous := ciAutomationCurrentCheckpoint(failed)

	passing := &github.PRFeedback{
		Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "success", HTMLURL: "https://ci/stable"}},
	}
	pruned := ciAutomationCurrentCheckpoint(passing)
	if len(pruned.FailedChecks) != 0 {
		t.Fatalf("expected passing check to be pruned, got %+v", pruned.FailedChecks)
	}

	regressed := ciAutomationBuildDelta(failed, pruned)
	if len(regressed.FailedChecks) != 1 {
		t.Fatalf("expected same check to retrigger after prune, got %+v", regressed.FailedChecks)
	}
	if again := ciAutomationBuildDelta(failed, previous); len(again.FailedChecks) != 0 {
		t.Fatalf("expected unchanged failure to remain deduped, got %+v", again.FailedChecks)
	}
}

func TestCIAutomationFeedbackDeltaIncludesEditedComments(t *testing.T) {
	previous := ciAutomationCheckpoint{
		Comments: []ciAutomationCommentSnapshot{{ID: 10, Body: "old body", Path: "main.go", Line: 12}},
	}
	feedback := &github.PRFeedback{
		Comments: []github.PRComment{{ID: 10, Body: "new body", Path: "main.go", Line: 12}},
	}

	delta := ciAutomationBuildDelta(feedback, previous)
	if len(delta.Comments) != 1 || delta.Comments[0].Body != "new body" {
		t.Fatalf("expected edited comment in delta, got %+v", delta.Comments)
	}
}

func TestHandleTaskPRCIAutomationQueuesFixDedupesAndMerges(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	pr := &github.TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widget",
		PRNumber:     42,
		State:        "open",
		ChecksState:  "failure",
	}
	ghSvc := &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID:                 "task-1",
			AutoFixEnabled:         true,
			EffectiveAutoFixPrompt: "Fix the PR",
		},
		prFeedback: &github.PRFeedback{
			Checks: []github.CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/unit"}},
		},
	}
	svc.SetGitHubService(ghSvc)

	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-fix: %v", err)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "acme/widget#42") || !strings.Contains(status.Entries[0].Content, "unit") {
		t.Fatalf("expected queued CI fix prompt, got %+v", status)
	}
	if len(ghSvc.fixAttempts) != 1 {
		t.Fatalf("expected one fix attempt, got %d", len(ghSvc.fixAttempts))
	}

	_, signature := encodeCIAutomationCheckpoint(ciAutomationCurrentCheckpoint(ghSvc.prFeedback))
	ghSvc.ciPRState = &github.TaskCIPRAutomationState{LastFixSignature: signature, LastFixCheckpointJSON: ghSvc.fixAttempts[0].CheckpointJSON}
	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle dedupe: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-1").Count; got != 1 {
		t.Fatalf("expected dedupe to avoid second queued prompt, got %d", got)
	}

	pr.ChecksState = "success"
	pr.ReviewState = "approved"
	pr.MergeableState = "clean"
	ghSvc.ciOptionsResp.AutoFixEnabled = false
	ghSvc.ciOptionsResp.AutoMergeEnabled = true
	if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
		t.Fatalf("handle auto-merge: %v", err)
	}
	if ghSvc.mergeCalls != 1 || len(ghSvc.mergeAttempts) != 1 {
		t.Fatalf("expected one merge call and attempt, got calls=%d attempts=%d", ghSvc.mergeCalls, len(ghSvc.mergeAttempts))
	}
}
