package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type failTaskLaunchErrorPersistRepo struct {
	repoStore
	err error
}

func (r *failTaskLaunchErrorPersistRepo) SetTaskMetadataKeyIfDifferentStamp(context.Context, string, string, string, interface{}) (bool, bool, error) {
	return false, false, r.err
}

func TestSelectRelevantTaskPRsUsesExactTaskRepositoryIdentity(t *testing.T) {
	taskRepo := &models.TaskRepository{
		ID:             "task-repo-1",
		RepositoryID:   "repo-1",
		CheckoutBranch: "feature/current",
		Metadata:       map[string]interface{}{"pr_number": float64(42)},
	}
	prs := []*github.TaskPR{
		{ID: "wrong-repo", RepositoryID: "repo-2", PRNumber: 42, State: githubPRStateClosed},
		{ID: "wrong-number", RepositoryID: "repo-1", PRNumber: 41, HeadBranch: "feature/current", State: githubPRStateClosed},
		{ID: "match", RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "other", State: githubPRStateClosed},
	}

	matches := selectRelevantTaskPRs([]*models.TaskRepository{taskRepo}, prs)
	if len(matches) != 1 || matches[0].pr.ID != "match" {
		t.Fatalf("selectRelevantTaskPRs() = %#v, want only exact repository/PR match", matches)
	}
}

func TestSelectRelevantTaskPRsFallsBackToNormalizedCaseSensitiveBranches(t *testing.T) {
	taskRepo := &models.TaskRepository{
		ID:             "task-repo-1",
		RepositoryID:   "repo-1",
		CheckoutBranch: " refs/remotes/origin/feature/current ",
	}
	prs := []*github.TaskPR{
		{ID: "match", RepositoryID: "repo-1", HeadBranch: "origin/feature/current", State: githubPRStateMerged},
		{ID: "case-mismatch", RepositoryID: "repo-1", HeadBranch: "feature/Current", State: githubPRStateMerged},
		{ID: "prefix-mismatch", RepositoryID: "repo-1", HeadBranch: "refs/heads/refs/remotes/origin/feature/current", State: githubPRStateMerged},
	}

	matches := selectRelevantTaskPRs([]*models.TaskRepository{taskRepo}, prs)
	if len(matches) != 1 || matches[0].pr.ID != "match" {
		t.Fatalf("selectRelevantTaskPRs() = %#v, want one normalized branch match", matches)
	}
}

func TestTerminalTaskPRGateRequiresAllRelevantPRsTerminal(t *testing.T) {
	tests := []struct {
		name string
		prs  []*github.TaskPR
		want bool
	}{
		{
			name: "merged and closed",
			prs: []*github.TaskPR{
				{RepositoryID: "repo-1", PRNumber: 1, State: githubPRStateMerged},
				{RepositoryID: "repo-1", PRNumber: 2, State: githubPRStateClosed},
			},
			want: true,
		},
		{name: "open wins", prs: []*github.TaskPR{{State: githubPRStateClosed}, {State: githubPRStateOpen}}, want: false},
		{name: "empty state fails open", prs: []*github.TaskPR{{State: githubPRStateClosed}, {State: ""}}, want: false},
		{name: "unknown state fails open", prs: []*github.TaskPR{{State: githubPRStateClosed}, {State: "draft"}}, want: false},
		{name: "absent matches fails open", prs: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipForTerminalTaskPRs(tt.prs); got != tt.want {
				t.Fatalf("shouldSkipForTerminalTaskPRs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTerminalTaskPRGateDoesNotUseSiblingPR(t *testing.T) {
	taskRepos := []*models.TaskRepository{
		{ID: "current", RepositoryID: "repo-1", CheckoutBranch: "feature/current"},
		{ID: "sibling", RepositoryID: "repo-1", CheckoutBranch: "feature/sibling"},
	}
	prs := []*github.TaskPR{
		{ID: "current-pr", RepositoryID: "repo-1", HeadBranch: "feature/current", State: githubPRStateOpen},
		{ID: "sibling-pr", RepositoryID: "repo-1", HeadBranch: "feature/sibling", State: githubPRStateMerged},
	}

	matches := selectRelevantTaskPRs(taskRepos, prs)
	if len(matches) != 2 {
		t.Fatalf("selectRelevantTaskPRs() returned %d matches, want current and sibling", len(matches))
	}
	if shouldSkipForTerminalTaskPRs(matchesToPRs(matches)) {
		t.Fatal("an open current branch must prevent the terminal sibling from gating launch")
	}
}

func TestTaskPRLaunchErrorStampIsStableAndIncludesState(t *testing.T) {
	matches := []taskPRMatch{
		{taskRepositoryID: "repo-row", pr: &github.TaskPR{RepositoryID: "repo-1", PRNumber: 42, State: githubPRStateMerged}},
	}
	first := taskPRLaunchErrorStamp(matches)
	second := taskPRLaunchErrorStamp([]taskPRMatch{{
		taskRepositoryID: "repo-row",
		pr:               &github.TaskPR{RepositoryID: "repo-1", PRNumber: 42, State: " merged ", HeadBranch: "different"},
	}})
	if first == "" || first != second {
		t.Fatalf("stamps differ for equivalent PR identity/state: %q vs %q", first, second)
	}
	changed := taskPRLaunchErrorStamp([]taskPRMatch{{
		taskRepositoryID: "repo-row",
		pr:               &github.TaskPR{RepositoryID: "repo-1", PRNumber: 42, State: githubPRStateClosed},
	}})
	if first == changed {
		t.Fatal("PR state must be part of the launch-error stamp")
	}
}

func TestWorkflowHasValidTerminalFinalStep(t *testing.T) {
	getter := newMockStepGetter()
	getter.steps["review"] = &wfmodels.WorkflowStep{ID: "review", WorkflowID: "workflow", Position: 1, Name: "Review"}
	getter.steps["done"] = &wfmodels.WorkflowStep{ID: "done", WorkflowID: "workflow", Position: 2, Name: "Done"}

	if !workflowHasValidTerminalFinalStep(t.Context(), getter, "review") {
		t.Fatal("final Done step should enable mark_review_done")
	}

	getter.steps["done"].Name = "Archive"
	if workflowHasValidTerminalFinalStep(t.Context(), getter, "review") {
		t.Fatal("non-terminal final step should not enable mark_review_done")
	}
}

func TestShouldSkipTerminalPRAutoStartPersistsAndClearsStampedError(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-pr-gate", "session-pr-gate", "review")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-1", WorkspaceID: "ws1", Name: "repo", SourceType: "local",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-pr-gate", TaskID: "task-pr-gate", RepositoryID: "repo-1",
		CheckoutBranch: "feature/current", Metadata: map[string]interface{}{"pr_number": float64(42)},
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	steps := newMockStepGetter()
	steps.steps["review"] = &wfmodels.WorkflowStep{ID: "review", WorkflowID: "wf1", Position: 1, Name: "Review"}
	steps.steps["done"] = &wfmodels.WorkflowStep{ID: "done", WorkflowID: "wf1", Position: 2, Name: "Done"}
	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task-pr-gate", RepositoryID: "repo-1", PRNumber: 42, State: githubPRStateMerged},
	}})

	task, err := repo.GetTask(ctx, "task-pr-gate")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !svc.shouldSkipTerminalPRAutoStart(ctx, task) {
		t.Fatal("terminal relevant PR should suppress automated launch")
	}
	stored, err := repo.GetTask(ctx, "task-pr-gate")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	errorValue, found := models.LoadTaskLaunchError(stored.Metadata)
	if !found || errorValue.Code != models.LaunchErrorCategoryPRAlreadyClosed {
		t.Fatalf("stored launch error = %#v, found=%v", errorValue, found)
	}
	if len(errorValue.RecoveryActions) != 1 || errorValue.RecoveryActions[0] != models.RecoveryActionMarkReviewDone {
		t.Fatalf("recovery actions = %#v, want mark_review_done", errorValue.RecoveryActions)
	}

	if !svc.shouldSkipTerminalPRAutoStart(ctx, stored) {
		t.Fatal("replaying the same terminal PR gate should still suppress launch")
	}
	if _, err := repo.GetTask(ctx, "task-pr-gate"); err != nil {
		t.Fatalf("reload task after replay: %v", err)
	}
	svc.clearTaskLaunchErrorIfStamp(ctx, "task-pr-gate", "stale-stamp")
	stillPresent, err := repo.GetTask(ctx, "task-pr-gate")
	if err != nil {
		t.Fatalf("reload after stale clear: %v", err)
	}
	if _, found := models.LoadTaskLaunchError(stillPresent.Metadata); !found {
		t.Fatal("stale clear removed the current launch error")
	}
	svc.clearTaskLaunchErrorIfStamp(ctx, "task-pr-gate", errorValue.Stamp())
	cleared, err := repo.GetTask(ctx, "task-pr-gate")
	if err != nil {
		t.Fatalf("reload after current clear: %v", err)
	}
	if _, found := models.LoadTaskLaunchError(cleared.Metadata); found {
		t.Fatal("current stamped launch error was not cleared")
	}
}

func TestShouldSkipTerminalPRAutoStartFailsOpenWhenErrorPersistenceFails(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-pr-gate-persist-failure", "session-pr-gate-persist-failure", "review")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-pr-gate-persist-failure", WorkspaceID: "ws1", Name: "repo", SourceType: "local",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-pr-gate-persist-failure", TaskID: "task-pr-gate-persist-failure",
		RepositoryID: "repo-pr-gate-persist-failure", CheckoutBranch: "feature/current",
		Metadata: map[string]interface{}{"pr_number": float64(42)},
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	steps := newMockStepGetter()
	steps.steps["review"] = &wfmodels.WorkflowStep{ID: "review", WorkflowID: "wf1", Position: 1, Name: "Review"}
	steps.steps["done"] = &wfmodels.WorkflowStep{ID: "done", WorkflowID: "wf1", Position: 2, Name: "Done"}
	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.repo = &failTaskLaunchErrorPersistRepo{repoStore: repo, err: errors.New("metadata unavailable")}
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task-pr-gate-persist-failure", RepositoryID: "repo-pr-gate-persist-failure", PRNumber: 42, State: githubPRStateMerged},
	}})

	task, err := repo.GetTask(ctx, "task-pr-gate-persist-failure")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if svc.shouldSkipTerminalPRAutoStart(ctx, task) {
		t.Fatal("PR gate suppressed launch after durable error persistence failed")
	}
}

func TestPersistTaskLaunchErrorConcurrentSameStampPreservesOneOccurrence(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-pr-gate-concurrent", "session-pr-gate-concurrent", "review")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	first := models.TaskLaunchError{
		Message: "terminal PR", OccurredAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		Code: models.LaunchErrorCategoryPRAlreadyClosed, StampValue: "same-pr-stamp",
	}
	second := models.TaskLaunchError{
		Message: "terminal PR", OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		Code: models.LaunchErrorCategoryPRAlreadyClosed, StampValue: "same-pr-stamp",
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for _, value := range []models.TaskLaunchError{first, second} {
		value := value
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- svc.persistTaskLaunchError(ctx, "task-pr-gate-concurrent", value)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var storedCount int
	for stored := range results {
		if stored {
			storedCount++
		}
	}
	if storedCount != 2 {
		t.Fatalf("successful persistence calls = %d, want store plus confirmed no-op", storedCount)
	}

	task, err := repo.GetTask(ctx, "task-pr-gate-concurrent")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	launchError, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok || launchError.Stamp() != "same-pr-stamp" {
		t.Fatalf("stored launch error = %#v, want same-pr-stamp", launchError)
	}
	if !launchError.OccurredAt.Equal(first.OccurredAt) && !launchError.OccurredAt.Equal(second.OccurredAt) {
		t.Fatalf("stored occurrence = %s, want one of the concurrent first occurrences", launchError.OccurredAt)
	}
}

func matchesToPRs(matches []taskPRMatch) []*github.TaskPR {
	prs := make([]*github.TaskPR, 0, len(matches))
	for _, match := range matches {
		prs = append(prs, match.pr)
	}
	return prs
}
