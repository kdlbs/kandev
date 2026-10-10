package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSelectCurrentPRChecksSupersededSuiteWithoutReplacementJobs(t *testing.T) {
	createdAt := time.Date(2026, time.September, 30, 14, 32, 41, 0, time.UTC)
	completedAt := createdAt.Add(8 * time.Second)
	client := NewMockClient()
	client.AddPR(&PR{
		Number: 143, State: "open", RepoOwner: "kandev", RepoName: "kandev",
		HeadSHA: "head-sha", HeadBranch: "fix/plugin-remote-agentctl-instance",
		HeadRepoOwner: "abhishekbiyala", HeadRepoName: "kandev",
	})
	client.ReplaceCheckRuns("kandev", "kandev", "head-sha", []CheckRun{
		{ID: 101, AppID: 15368, AppSlug: "github-actions", CheckSuiteID: 99475252942, Name: "deploy-fork", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionCancelled, StartedAt: checkSelectionTimePtr(createdAt), CompletedAt: checkSelectionTimePtr(completedAt)},
		{ID: 102, AppID: 15368, AppSlug: "github-actions", CheckSuiteID: 99475252942, Name: "update-description-fork", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionCancelled, StartedAt: checkSelectionTimePtr(createdAt), CompletedAt: checkSelectionTimePtr(completedAt)},
		{ID: 201, AppID: 15368, AppSlug: "github-actions", CheckSuiteID: 99475253346, Name: "package-preview", Source: checkSourceCheckRun, Status: "in_progress", StartedAt: checkSelectionTimePtr(createdAt)},
	})
	client.ReplaceWorkflowRuns("kandev", "kandev", "head-sha", []WorkflowRun{
		{
			ID: 36729911126, CheckSuiteID: 99475252942, WorkflowID: 266562986, Name: "Preview", Event: "pull_request_target",
			Status: workflowStatusCompleted, Conclusion: checkConclusionCancelled,
			HeadSHA: "head-sha", HeadBranch: "fix/plugin-remote-agentctl-instance",
			HeadRepoOwner: "abhishekbiyala", HeadRepoName: "kandev",
			CreatedAt: createdAt, UpdatedAt: completedAt,
		},
		{
			ID: 36729911232, CheckSuiteID: 99475253346, WorkflowID: 266562986, Name: "Preview", Event: "pull_request_target",
			Status: "in_progress", HeadSHA: "head-sha",
			HeadBranch:    "fix/plugin-remote-agentctl-instance",
			HeadRepoOwner: "abhishekbiyala", HeadRepoName: "kandev",
			CreatedAt: createdAt, UpdatedAt: createdAt,
		},
	})

	feedback, err := client.GetPRFeedback(context.Background(), "kandev", "kandev", 143)
	if err != nil {
		t.Fatalf("GetPRFeedback() error = %v", err)
	}
	if len(feedback.Checks) != 1 || feedback.Checks[0].Name != "package-preview" {
		t.Fatalf("selected checks = %#v, want only the active replacement packaging check", feedback.Checks)
	}
}

func checkSelectionTimePtr(value time.Time) *time.Time {
	return &value
}

func TestSelectCurrentPRChecksPreservesIndependentApplications(t *testing.T) {
	checks := selectCurrentPRChecks(&PR{HeadSHA: "head"}, []CheckRun{
		{ID: 10, AppID: 11, AppSlug: "ci-one", Name: "CI / test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionFail},
		{ID: 20, AppID: 22, AppSlug: "ci-two", Name: "CI / test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
	}, nil)
	if len(checks.Checks) != 2 {
		t.Fatalf("selected checks = %#v, want both applications", checks.Checks)
	}
}

func TestSelectCurrentPRChecksKeepsThirdPartyAndLegacyFallbacks(t *testing.T) {
	t.Run("third-party latest job within application", func(t *testing.T) {
		selection := selectCurrentPRChecks(&PR{HeadSHA: "head"}, []CheckRun{
			{ID: 10, AppID: 11, AppSlug: "ci-provider", CheckSuiteID: 101, Name: "test",
				Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionFail},
			{ID: 20, AppID: 11, AppSlug: "ci-provider", CheckSuiteID: 202, Name: "test",
				Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
		}, nil)
		if len(selection.Checks) != 1 || selection.Checks[0].ID != 20 {
			t.Fatalf("third-party checks = %#v, want latest job within the application", selection.Checks)
		}
	})

	t.Run("legacy name fallback", func(t *testing.T) {
		older := time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC)
		newer := older.Add(time.Minute)
		selection := selectCurrentPRChecks(&PR{HeadSHA: "head"}, []CheckRun{
			{Name: "legacy test", Source: checkSourceCheckRun, Status: checkStatusCompleted,
				Conclusion: checkConclusionFail, StartedAt: &older},
			{Name: "legacy test", Source: checkSourceCheckRun, Status: checkStatusCompleted,
				Conclusion: checkConclusionSuccess, StartedAt: &newer},
		}, nil)
		if len(selection.Checks) != 1 || selection.Checks[0].Conclusion != checkConclusionSuccess {
			t.Fatalf("legacy checks = %#v, want newest same-name legacy observation", selection.Checks)
		}
	})
}

func TestSelectCurrentPRChecksPreservesWorkflowEventAndRepositoryIdentity(t *testing.T) {
	pr := &PR{
		Number: 43, HeadSHA: "head", HeadBranch: "feature",
		HeadRepoOwner: "fork", HeadRepoName: "repo",
	}
	checks := []CheckRun{
		{ID: 10, AppID: 1, CheckSuiteID: 101, Name: "CI / test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionFail},
		{ID: 20, AppID: 1, CheckSuiteID: 202, Name: "CI / test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
		{ID: 30, AppID: 1, CheckSuiteID: 303, Name: "CI / test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
	}
	association := func(owner string) []WorkflowRunPullRequest {
		return []WorkflowRunPullRequest{{Number: pr.Number, HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: owner, HeadRepoName: "repo"}}
	}
	runs := []WorkflowRun{
		{ID: 1, CheckSuiteID: 101, WorkflowID: 5, Name: "CI", Event: "pull_request", Status: workflowStatusCompleted,
			HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "fork", HeadRepoName: "repo", PullRequests: association("fork")},
		{ID: 2, CheckSuiteID: 202, WorkflowID: 5, Name: "CI", Event: "pull_request_target", Status: workflowStatusCompleted,
			HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "fork", HeadRepoName: "repo", PullRequests: association("fork")},
		{ID: 3, CheckSuiteID: 303, WorkflowID: 5, Name: "CI", Event: "pull_request", Status: workflowStatusCompleted,
			HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "other-fork", HeadRepoName: "repo", PullRequests: association("other-fork")},
	}
	selection := selectCurrentPRChecks(pr, checks, runs)
	if len(selection.Checks) != 3 {
		t.Fatalf("selected checks = %#v, want checks from both events and the independent source", selection.Checks)
	}
	seenRuns := make(map[int64]bool)
	for _, check := range selection.Checks {
		seenRuns[check.WorkflowRunID] = true
	}
	for _, runID := range []int64{1, 2, 3} {
		if !seenRuns[runID] {
			t.Errorf("workflow run %d was collapsed", runID)
		}
	}
}

func TestSelectCurrentPRChecksMissingEvidencePreservesFailure(t *testing.T) {
	failure := CheckRun{
		ID: 10, AppID: 11, AppSlug: "github-actions", CheckSuiteID: 99,
		Name: "Preview / deploy", Source: checkSourceCheckRun,
		Status: checkStatusCompleted, Conclusion: checkConclusionFail,
	}
	selection := selectCurrentPRChecks(&PR{HeadSHA: "head"}, []CheckRun{failure}, nil)
	if len(selection.Checks) != 1 || selection.Checks[0].Conclusion != checkConclusionFail {
		t.Fatalf("selected checks = %#v, want the unclassified failure retained", selection.Checks)
	}
}

func TestGetPRFeedbackPreservesUnmatchedActionsSuitesWhenWorkflowEvidenceUnavailable(t *testing.T) {
	identities := []struct {
		name    string
		appID   int64
		appSlug string
	}{
		{name: "app id and slug", appID: 15368, appSlug: "github-actions"},
		{name: "app id only", appID: 15368},
		{name: "app slug only", appSlug: "github-actions"},
		{name: "unknown producer"},
	}
	evidence := []struct {
		name string
		err  bool
	}{
		{name: "missing"},
		{name: "read error", err: true},
	}

	for _, identity := range identities {
		for _, workflowEvidence := range evidence {
			t.Run(identity.name+"/"+workflowEvidence.name, func(t *testing.T) {
				client := NewMockClient()
				client.AddPR(&PR{Number: 1, State: prStateOpen, RepoOwner: "owner", RepoName: "repo", HeadSHA: "head"})
				client.ReplaceCheckRuns("owner", "repo", "head", []CheckRun{
					{ID: 10, AppID: identity.appID, AppSlug: identity.appSlug, CheckSuiteID: 101,
						Name: "test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionFail},
					{ID: 20, AppID: identity.appID, AppSlug: identity.appSlug, CheckSuiteID: 202,
						Name: "test", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
				})

				var feedback *PRFeedback
				var err error
				if workflowEvidence.err {
					feedback, err = getPRFeedback(context.Background(), workflowRunErrorClient{Client: client}, "owner", "repo", 1)
				} else {
					feedback, err = client.GetPRFeedback(context.Background(), "owner", "repo", 1)
				}
				if err != nil {
					t.Fatalf("GetPRFeedback() error = %v", err)
				}
				if len(feedback.Checks) != 2 {
					t.Fatalf("feedback checks = %#v, want both unmatched suites retained", feedback.Checks)
				}
				if feedback.ChecksState == nil || *feedback.ChecksState != checkConclusionFail {
					t.Fatalf("feedback checks state = %v, want failure", feedback.ChecksState)
				}
				if !feedback.HasIssues {
					t.Fatal("feedback HasIssues = false, want true for retained Actions failure")
				}
			})
		}
	}
}

type workflowRunErrorClient struct {
	Client
}

func (workflowRunErrorClient) ListWorkflowRuns(context.Context, string, string, string) ([]WorkflowRun, error) {
	return nil, errors.New("workflow run read failed")
}

func TestSelectCurrentPRChecksQueuedReplacementAndPartialRerun(t *testing.T) {
	pr := &PR{HeadSHA: "head"}
	run := WorkflowRun{ID: 7, CheckSuiteID: 70, WorkflowID: 5, Name: "CI", Event: workflowEventPullRequest,
		Status: workflowStatusCompleted, HeadSHA: "head", HeadBranch: "feature", HeadRepoOwner: "fork", HeadRepoName: "repo"}
	selection := selectCurrentPRChecks(pr, []CheckRun{
		{ID: 10, AppID: 1, CheckSuiteID: 70, Name: "CI / unit", Source: checkSourceCheckRun,
			Status: checkStatusCompleted, Conclusion: checkConclusionFail},
		{ID: 11, AppID: 1, CheckSuiteID: 70, Name: "CI / unit", Source: checkSourceCheckRun,
			Status: "queued"},
		{ID: 12, AppID: 1, CheckSuiteID: 70, Name: "CI / e2e", Source: checkSourceCheckRun,
			Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
	}, []WorkflowRun{run})
	if len(selection.Checks) != 2 {
		t.Fatalf("partial rerun checks = %#v, want queued unit and retained e2e", selection.Checks)
	}
	if selection.Checks[0].ID != 11 || selection.Checks[0].StartedAt != nil {
		t.Fatalf("queued replacement = %+v, want newer ID retained without start time", selection.Checks[0])
	}
}

func TestGetPRStatusActiveWorkflowWithoutJobsRemainsPending(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{
		Number: 7, State: prStateOpen, RepoOwner: "owner", RepoName: "repo", HeadSHA: "head",
		HeadBranch: "feature", HeadRepoOwner: "fork", HeadRepoName: "repo",
	})
	client.ReplaceWorkflowRuns("owner", "repo", "head", []WorkflowRun{{
		ID: 8, CheckSuiteID: 80, WorkflowID: 5, Name: "CI", Event: "pull_request_target",
		Status: "in_progress", HeadSHA: "head", HeadBranch: "feature",
		HeadRepoOwner: "fork", HeadRepoName: "repo",
	}})
	status, err := client.GetPRStatus(context.Background(), "owner", "repo", 7)
	if err != nil {
		t.Fatalf("GetPRStatus() error = %v", err)
	}
	if status.ChecksState != checkStatusPending || status.ChecksTotal != 0 || status.ChecksPassing != 0 {
		t.Fatalf("status = %+v, want pending with zero invented check counts", status)
	}
	feedback, err := client.GetPRFeedback(context.Background(), "owner", "repo", 7)
	if err != nil {
		t.Fatalf("GetPRFeedback() error = %v", err)
	}
	if feedback.ChecksState == nil || *feedback.ChecksState != checkStatusPending || len(feedback.Checks) != 0 {
		t.Fatalf("feedback = %+v, want pending snapshot state with no fabricated checks", feedback)
	}
}
