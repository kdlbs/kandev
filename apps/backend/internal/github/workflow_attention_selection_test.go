package github

import (
	"context"
	"testing"
	"time"
)

func TestTerminalPRFeedbackSelectsCurrentWorkflowSuite(t *testing.T) {
	client := &workflowAttentionCacheClient{MockClient: NewMockClient()}
	client.AddPR(workflowSelectionTestPR(prStateClosed))
	client.ReplaceCheckRuns("acme", "widget", "head", []CheckRun{
		{ID: 101, AppID: githubActionsAppID, AppSlug: "github-actions", CheckSuiteID: 77,
			Name: "CI / unit", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionFail},
		{ID: 202, AppID: githubActionsAppID, AppSlug: "github-actions", CheckSuiteID: 88,
			Name: "CI / unit", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
	})
	client.ReplaceWorkflowRuns("acme", "widget", "head", workflowSelectionTestRuns())

	feedback, err := newTestService(client).GetPRFeedback(context.Background(), "acme", "widget", 143)
	if err != nil {
		t.Fatalf("GetPRFeedback() error = %v", err)
	}
	if len(feedback.Checks) != 1 || feedback.Checks[0].ID != 202 {
		t.Fatalf("terminal feedback checks = %#v, want the current successful suite only", feedback.Checks)
	}
	if feedback.ChecksState == nil || *feedback.ChecksState != checkConclusionSuccess {
		t.Fatalf("terminal feedback checks_state = %v, want success", feedback.ChecksState)
	}
	if feedback.WorkflowAttention == nil || feedback.WorkflowAttention.State != WorkflowAttentionNone {
		t.Fatalf("terminal workflow attention = %+v, want none", feedback.WorkflowAttention)
	}
	if got := client.runCalls.Load(); got != 1 {
		t.Fatalf("workflow reads = %d, want one read for terminal check selection", got)
	}
}

func TestFreshCompletedChecksOverrideCachedActiveWorkflowStatus(t *testing.T) {
	client := &workflowAttentionCacheClient{MockClient: NewMockClient()}
	client.AddPR(workflowSelectionTestPR(prStateOpen))
	started := time.Date(2026, time.September, 30, 14, 32, 41, 0, time.UTC)
	completed := started.Add(time.Minute)
	activeRun := workflowSelectionTestRuns()[0]
	activeRun.Status = "in_progress"
	activeRun.Conclusion = ""
	client.ReplaceWorkflowRuns("acme", "widget", "head", []WorkflowRun{activeRun})
	client.ReplaceCheckRuns("acme", "widget", "head", []CheckRun{{
		ID: 101, AppID: githubActionsAppID, AppSlug: "github-actions", CheckSuiteID: 77,
		Name: "CI / unit", Source: checkSourceCheckRun, Status: "in_progress", StartedAt: &started,
	}})
	svc := newTestService(client)

	first, err := svc.GetPRFeedback(context.Background(), "acme", "widget", 143)
	if err != nil {
		t.Fatalf("initial GetPRFeedback() error = %v", err)
	}
	if first.ChecksState == nil || *first.ChecksState != checkStatusPending {
		t.Fatalf("initial checks_state = %v, want pending", first.ChecksState)
	}

	client.ReplaceCheckRuns("acme", "widget", "head", []CheckRun{{
		ID: 101, AppID: githubActionsAppID, AppSlug: "github-actions", CheckSuiteID: 77,
		Name: "CI / unit", Source: checkSourceCheckRun, Status: checkStatusCompleted,
		Conclusion: checkConclusionSuccess, StartedAt: &started, CompletedAt: &completed,
	}})
	svc.prFeedbackCache.clear()
	second, err := svc.GetPRFeedback(context.Background(), "acme", "widget", 143)
	if err != nil {
		t.Fatalf("refreshed GetPRFeedback() error = %v", err)
	}
	if second.ChecksState == nil || *second.ChecksState != checkConclusionSuccess {
		t.Fatalf("refreshed checks_state = %v, want success from fresh completed checks", second.ChecksState)
	}
	if got := client.runCalls.Load(); got != 1 {
		t.Fatalf("workflow reads = %d, want the cached active run reused", got)
	}
}

func workflowSelectionTestPR(state string) *PR {
	return &PR{
		Number: 143, State: state, RepoOwner: "acme", RepoName: "widget", HeadSHA: "head",
		HeadBranch: "feature", HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}
}

func workflowSelectionTestRuns() []WorkflowRun {
	created := time.Date(2026, time.September, 30, 14, 32, 41, 0, time.UTC)
	association := []WorkflowRunPullRequest{{
		Number: 143, HeadSHA: "head", HeadBranch: "feature",
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}}
	return []WorkflowRun{
		{ID: 7, CheckSuiteID: 77, WorkflowID: 9, Name: "CI", Event: "pull_request_target",
			Status: workflowStatusCompleted, Conclusion: checkConclusionFail, HeadSHA: "head",
			HeadBranch: "feature", HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
			PullRequests: association, CreatedAt: created, UpdatedAt: created},
		{ID: 8, CheckSuiteID: 88, WorkflowID: 9, Name: "CI", Event: "pull_request_target",
			Status: workflowStatusCompleted, Conclusion: checkConclusionSuccess, HeadSHA: "head",
			HeadBranch: "feature", HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
			PullRequests: association, CreatedAt: created.Add(time.Second), UpdatedAt: created.Add(time.Second)},
	}
}
