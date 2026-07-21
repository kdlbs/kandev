package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestCreateIssueTask_SelfHealsWhenAgentProfileSoftDeleted is the regression
// guard for the GitHub side of the production bug: createIssueTask bypasses
// the WatcherDispatchCoordinator, so the soft-deleted-profile pre-flight
// lives directly on Service. When the watcher's agent profile has been
// soft-deleted, createIssueTask MUST disable the watcher and short-circuit
// before reserving a dedup slot or creating a task.
func TestCreateIssueTask_SelfHealsWhenAgentProfileSoftDeleted(t *testing.T) {
	svc, _ := setupIssueTaskTest(t)
	ghSvc := &mockGitHubService{issueReserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingIssueTaskCreator{taskID: "should-not-be-created"}
	svc.SetIssueTaskCreator(creator)
	svc.SetProfileLookup(&fakeProfileLookup{deleted: true, name: "Removed Kilo"})

	evt := newIssueEvent()
	evt.AgentProfileID = "deleted-profile"
	evt.IssueWatchID = "github-iw-1"

	svc.createIssueTask(context.Background(), evt)

	if ghSvc.issueReserveCalls != 0 {
		t.Errorf("Reserve must not run when profile is deleted, got %d calls", ghSvc.issueReserveCalls)
	}
	if creator.calls != 0 {
		t.Errorf("CreateIssueTask must not run when profile is deleted, got %d calls", creator.calls)
	}
	if ghSvc.disableIssueWatchCalls != 1 {
		t.Fatalf("expected DisableIssueWatchWithError to fire once, got %d calls", ghSvc.disableIssueWatchCalls)
	}
	if ghSvc.lastDisableIssueWatchID != "github-iw-1" {
		t.Errorf("disable watch_id = %q, want %q", ghSvc.lastDisableIssueWatchID, "github-iw-1")
	}
	// Pin the invariant the settings UI relies on (see coordinator-level
	// test for the same contract): the stamped cause must carry both the
	// profile name and id so the disabled-watcher banner is actionable.
	if !strings.Contains(ghSvc.lastDisableIssueCause, "Removed Kilo") ||
		!strings.Contains(ghSvc.lastDisableIssueCause, "deleted-profile") {
		t.Errorf("disable cause missing profile name or id: %q", ghSvc.lastDisableIssueCause)
	}
}

// TestCreateIssueTask_LegacyEventWithoutProfileSkipsPreflight pins the
// no-regression contract for rows predating the agent_profile_id column:
// an empty AgentProfileID means the watcher predates self-heal wiring and
// the pre-flight must NOT run (the lookup would resolve "" to a not-found
// row and the empty-id branch must short-circuit cleanly).
func TestCreateIssueTask_LegacyEventWithoutProfileSkipsPreflight(t *testing.T) {
	svc, _ := setupIssueTaskTest(t)
	ghSvc := &mockGitHubService{issueReserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingIssueTaskCreator{taskID: "task-legacy"}
	svc.SetIssueTaskCreator(creator)
	lookup := &fakeProfileLookup{deleted: true, name: "Removed Kilo"}
	svc.SetProfileLookup(lookup)

	evt := newIssueEvent() // AgentProfileID is "" by default

	svc.createIssueTask(context.Background(), evt)

	if lookup.calls != 0 {
		t.Errorf("lookup must be skipped for empty profile id, got %d calls", lookup.calls)
	}
	if ghSvc.disableIssueWatchCalls != 0 {
		t.Errorf("legacy watcher must not be self-healed, got %d disable calls", ghSvc.disableIssueWatchCalls)
	}
	if creator.calls != 1 {
		t.Errorf("legacy event must still flow through pipeline, got CreateIssueTask calls %d", creator.calls)
	}
}

// TestCreateReviewTask_SelfHealsWhenAgentProfileSoftDeleted mirrors the
// issue-side regression guard for the PR review watcher path.
func TestCreateReviewTask_SelfHealsWhenAgentProfileSoftDeleted(t *testing.T) {
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{},
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	ghSvc := &mockGitHubService{reserveReturn: true}
	svc.SetGitHubService(ghSvc)
	reviewCreator := &countingReviewTaskCreator{taskID: "should-not-be-created"}
	svc.SetReviewTaskCreator(reviewCreator)
	svc.SetProfileLookup(&fakeProfileLookup{deleted: true, name: "Removed Opencode"})

	evt := &github.NewReviewPREvent{
		ReviewWatchID:  "github-rw-1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		AgentProfileID: "deleted-profile",
		PR: &github.PR{
			Number: 7, Title: "Fix login bug",
			HTMLURL:   "https://gh/acme/widget/pull/7",
			RepoOwner: "acme", RepoName: "widget",
			HeadBranch: "fix/login", BaseBranch: "main",
		},
	}

	svc.createReviewTask(context.Background(), evt)

	if ghSvc.reserveCalls != 0 {
		t.Errorf("ReserveReviewPRTask must not run when profile is deleted, got %d calls", ghSvc.reserveCalls)
	}
	if reviewCreator.calls != 0 {
		t.Errorf("CreateReviewTask must not run when profile is deleted, got %d calls", reviewCreator.calls)
	}
	if ghSvc.disableReviewWatchCalls != 1 {
		t.Fatalf("expected DisableReviewWatchWithError to fire once, got %d calls", ghSvc.disableReviewWatchCalls)
	}
	if ghSvc.lastDisableReviewWatchID != "github-rw-1" {
		t.Errorf("disable watch_id = %q, want %q", ghSvc.lastDisableReviewWatchID, "github-rw-1")
	}
}
