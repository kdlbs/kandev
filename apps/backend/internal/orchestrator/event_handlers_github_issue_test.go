package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// --- Issue Watch Tests ---

func TestInterpolateIssuePrompt(t *testing.T) {
	issue := &github.Issue{
		Number:      7,
		Title:       "Fix login bug",
		Body:        "Login fails on mobile",
		HTMLURL:     "https://github.com/acme/widget/issues/7",
		AuthorLogin: "bob",
		RepoOwner:   "acme",
		RepoName:    "widget",
		Labels:      []string{"bug", "priority:high"},
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			"empty template uses embedded default",
			"",
			"You have been assigned a GitHub issue to work on.\n\n" +
				"**Issue:** https://github.com/acme/widget/issues/7\n" +
				"**Title:** Fix login bug (#7)\n" +
				"**Repository:** acme/widget\n" +
				"**Author:** bob\n" +
				"**Labels:** bug, priority:high\n\n" +
				"## Instructions\n\n" +
				"1. Read the issue description carefully and understand the requirements.\n" +
				"2. Explore the codebase to understand the relevant code and architecture.\n" +
				"3. Implement the changes described in the issue.\n" +
				"4. Write or update tests to cover the changes.\n" +
				"5. Run the test suite to ensure nothing is broken.\n" +
				"6. Commit your changes with a descriptive commit message referencing the issue.",
		},
		{
			"all placeholders",
			"Fix {{issue.link}} (#{{issue.number}}) by {{issue.author}} in {{issue.repo}}: {{issue.title}} [{{issue.labels}}]\n{{issue.body}}",
			"Fix https://github.com/acme/widget/issues/7 (#7) by bob in acme/widget: Fix login bug [bug, priority:high]\nLogin fails on mobile",
		},
		{
			"no placeholders",
			"Please fix this issue",
			"Please fix this issue",
		},
		{
			"partial placeholders",
			"Check {{issue.link}}",
			"Check https://github.com/acme/widget/issues/7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpolateIssuePrompt(tt.template, issue)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// countingIssueTaskCreator records how many times CreateIssueTask was called.
type countingIssueTaskCreator struct {
	calls  int
	err    error
	taskID string
}

func (c *countingIssueTaskCreator) CreateIssueTask(_ context.Context, _ *IssueTaskRequest) (*models.Task, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	id := c.taskID
	if id == "" {
		id = "issue-task-created"
	}
	return &models.Task{ID: id}, nil
}

// newIssueEvent builds a NewIssueEvent for createIssueTask tests.
func newIssueEvent() *github.NewIssueEvent {
	return &github.NewIssueEvent{
		IssueWatchID:   "iw1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		Issue: &github.Issue{
			Number: 7, Title: "Fix login bug", HTMLURL: "https://gh/acme/widget/issues/7",
			RepoOwner: "acme", RepoName: "widget",
		},
	}
}

// setupIssueTaskTest builds a Service with a seeded workflow step.
func setupIssueTaskTest(t *testing.T) (*Service, *mockStepGetter) {
	t.Helper()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{},
	}
	return createTestService(repo, stepGetter, newMockTaskRepo()), stepGetter
}


func TestCreateIssueTask_SkipsWhenAlreadyReserved(t *testing.T) {
	svc, _ := setupIssueTaskTest(t)
	ghSvc := &mockGitHubService{issueReserveReturn: false}
	svc.SetGitHubService(ghSvc)
	creator := &countingIssueTaskCreator{}
	svc.SetIssueTaskCreator(creator)

	svc.createIssueTask(context.Background(), newIssueEvent())

	if ghSvc.issueReserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.issueReserveCalls)
	}
	if creator.calls != 0 {
		t.Errorf("expected CreateIssueTask NOT to be called when reservation lost, got %d calls", creator.calls)
	}
	if ghSvc.issueReleaseCalls != 0 {
		t.Errorf("expected no Release calls when reservation was never held, got %d", ghSvc.issueReleaseCalls)
	}
}

func TestCreateIssueTask_ReservesThenAssignsTaskID(t *testing.T) {
	svc, _ := setupIssueTaskTest(t)
	ghSvc := &mockGitHubService{issueReserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingIssueTaskCreator{taskID: "issue-task-999"}
	svc.SetIssueTaskCreator(creator)

	svc.createIssueTask(context.Background(), newIssueEvent())

	if ghSvc.issueReserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.issueReserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateIssueTask call, got %d", creator.calls)
	}
	if ghSvc.issueAssignCalls != 1 {
		t.Errorf("expected 1 AssignIssueWatchTaskID call, got %d", ghSvc.issueAssignCalls)
	}
	if ghSvc.issueAssignedID != "issue-task-999" {
		t.Errorf("AssignIssueWatchTaskID got taskID=%q, want %q", ghSvc.issueAssignedID, "issue-task-999")
	}
	if ghSvc.issueReleaseCalls != 0 {
		t.Errorf("expected no Release calls on happy path, got %d", ghSvc.issueReleaseCalls)
	}
}

func TestCreateIssueTask_ReleasesOnTaskCreationFailure(t *testing.T) {
	svc, _ := setupIssueTaskTest(t)
	ghSvc := &mockGitHubService{issueReserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingIssueTaskCreator{err: assertErrTaskCreate}
	svc.SetIssueTaskCreator(creator)

	svc.createIssueTask(context.Background(), newIssueEvent())

	if ghSvc.issueReserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.issueReserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateIssueTask call, got %d", creator.calls)
	}
	if ghSvc.issueReleaseCalls != 1 {
		t.Errorf("expected 1 Release call after task-create failure, got %d", ghSvc.issueReleaseCalls)
	}
	if ghSvc.issueAssignCalls != 0 {
		t.Errorf("expected no Assign call after failure, got %d", ghSvc.issueAssignCalls)
	}
}