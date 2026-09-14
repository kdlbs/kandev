package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type workflowAttentionTestClient struct {
	*MockClient
	runsErr error
	jobsErr error
}

func (c *workflowAttentionTestClient) ListWorkflowRuns(ctx context.Context, owner, repo, headSHA string) ([]WorkflowRun, error) {
	if c.runsErr != nil {
		return nil, c.runsErr
	}
	return c.MockClient.ListWorkflowRuns(ctx, owner, repo, headSHA)
}

func (c *workflowAttentionTestClient) ListWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64, attempt int) ([]WorkflowJob, error) {
	if c.jobsErr != nil {
		return nil, c.jobsErr
	}
	return c.MockClient.ListWorkflowRunJobs(ctx, owner, repo, runID, attempt)
}

func TestWorkflowAttention_JoblessForkApproval(t *testing.T) {
	client := &workflowAttentionTestClient{MockClient: NewMockClient()}
	client.AddPR(&PR{
		RepoOwner:     "acme",
		RepoName:      "widget",
		Number:        143,
		State:         "open",
		HeadSHA:       "head-sha",
		HeadBranch:    "feature/approval",
		HeadRepoOwner: "contributor",
		HeadRepoName:  "widget-fork",
	})
	client.ReplaceWorkflowRuns("acme", "widget", "head-sha", []WorkflowRun{
		{
			ID:            34494307522,
			RunAttempt:    1,
			WorkflowID:    77,
			Name:          "Run tests",
			Event:         "pull_request",
			Status:        "completed",
			Conclusion:    "action_required",
			HeadSHA:       "head-sha",
			HeadBranch:    "feature/approval",
			HeadRepoOwner: "contributor",
			HeadRepoName:  "widget-fork",
			CreatedAt:     time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
			PullRequests:  nil,
			HTMLURL:       "https://github.com/acme/widget/actions/runs/34494307522",
		},
	})
	client.ReplaceWorkflowRunJobs("acme", "widget", 34494307522, 1, nil)

	status, err := client.GetPRStatus(context.Background(), "acme", "widget", 143)
	if err != nil {
		t.Fatalf("GetPRStatus() error = %v", err)
	}
	if status.WorkflowAttention == nil {
		t.Fatal("WorkflowAttention = nil, want approval observation")
	}
	if got := status.WorkflowAttention.State; got != WorkflowAttentionApprovalRequired {
		t.Fatalf("WorkflowAttention.State = %q, want %q", got, WorkflowAttentionApprovalRequired)
	}
	if status.ChecksTotal != 0 || status.ChecksPassing != 0 {
		t.Fatalf("check counts = (%d, %d), want (0, 0)", status.ChecksTotal, status.ChecksPassing)
	}
	if len(status.WorkflowAttention.Runs) != 1 {
		t.Fatalf("workflow attention runs = %d, want 1", len(status.WorkflowAttention.Runs))
	}
	if got := status.WorkflowAttention.Runs[0].Name; got != "Run tests" {
		t.Fatalf("workflow attention run name = %q, want %q", got, "Run tests")
	}
}

func TestWorkflowAttention_LatestRunAndAssociationFiltering(t *testing.T) {
	client := &workflowAttentionTestClient{MockClient: NewMockClient()}
	pr := &PR{
		Number:        7,
		State:         "open",
		HeadSHA:       "head-sha",
		HeadBranch:    "feature/approval",
		HeadRepoOwner: "contributor",
		HeadRepoName:  "widget-fork",
		RepoOwner:     "acme",
		RepoName:      "widget",
	}
	client.AddPR(pr)
	old := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	newer := old.Add(time.Hour)
	client.ReplaceWorkflowRuns("acme", "widget", pr.HeadSHA, []WorkflowRun{
		{ID: 100, RunAttempt: 1, WorkflowID: 9, Name: "CI", Event: "pull_request", Status: "completed", Conclusion: "action_required", HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "contributor", HeadRepoName: "widget-fork", CreatedAt: old, UpdatedAt: old},
		{ID: 101, RunAttempt: 1, WorkflowID: 9, Name: "CI", Event: "pull_request", Status: "completed", Conclusion: "success", HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "contributor", HeadRepoName: "widget-fork", CreatedAt: newer, UpdatedAt: newer},
		{ID: 102, RunAttempt: 1, WorkflowID: 10, Name: "Other PR", Event: "pull_request", Status: "completed", Conclusion: "action_required", HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "contributor", HeadRepoName: "widget-fork", PullRequests: []WorkflowRunPullRequest{{Number: 99, HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "contributor", HeadRepoName: "widget-fork"}}, CreatedAt: newer, UpdatedAt: newer},
	})

	attention, err := collectWorkflowAttention(context.Background(), client, "acme", "widget", pr)
	if err != nil {
		t.Fatalf("collectWorkflowAttention() error = %v", err)
	}
	if attention.State != WorkflowAttentionNone || len(attention.Runs) != 0 {
		t.Fatalf("attention = %#v, want authoritative none after newer success and association filtering", attention)
	}
}

func TestWorkflowAttention_RESTAssociationUsesRepositoryURLIdentity(t *testing.T) {
	runs, err := decodeGHWorkflowRuns(`{"id":7,"run_attempt":1,"workflow_id":9,"name":"Run tests","event":"pull_request","status":"completed","conclusion":"action_required","head_sha":"head-sha","head_branch":"feature/approval","head_repository":{"id":123,"full_name":"contributor/widget-fork","name":"widget-fork","owner":{"login":"contributor"}},"pull_requests":[{"number":143,"head":{"ref":"feature/approval","sha":"head-sha","repo":{"id":123,"name":"widget-fork","url":"https://api.github.com/repos/contributor/widget-fork"}}}]}`)
	if err != nil {
		t.Fatalf("decodeGHWorkflowRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("decoded runs = %d, want 1", len(runs))
	}
	run := convertRawWorkflowRun(runs[0])

	matchingPR := &PR{
		Number:        143,
		HeadSHA:       "head-sha",
		HeadBranch:    "feature/approval",
		HeadRepoID:    123,
		HeadRepoOwner: "contributor",
		HeadRepoName:  "widget-fork",
	}
	if !workflowRunMatchesPR(run, matchingPR) {
		t.Fatalf("workflowRunMatchesPR() = false for the REST association shape: %#v", run)
	}

	mismatchingPR := *matchingPR
	mismatchingPR.HeadRepoID = 456
	if workflowRunMatchesPR(run, &mismatchingPR) {
		t.Fatal("workflowRunMatchesPR() = true for a mismatching head repository")
	}
}

func TestWorkflowAttention_NewAssociatedSuccessSupersedesOlderUnassociatedApproval(t *testing.T) {
	runs, err := decodeGHWorkflowRuns(`{"id":100,"run_attempt":1,"workflow_id":9,"name":"CI","event":"pull_request","status":"completed","conclusion":"action_required","head_sha":"head-sha","head_branch":"feature/approval","head_repository":{"id":123,"full_name":"contributor/widget-fork","name":"widget-fork","owner":{"login":"contributor"}},"created_at":"2026-09-10T12:00:00Z","updated_at":"2026-09-10T12:00:00Z","pull_requests":[]}
{"id":101,"run_attempt":1,"workflow_id":9,"name":"CI","event":"pull_request","status":"completed","conclusion":"success","head_sha":"head-sha","head_branch":"feature/approval","head_repository":{"id":123,"full_name":"contributor/widget-fork","name":"widget-fork","owner":{"login":"contributor"}},"created_at":"2026-09-10T13:00:00Z","updated_at":"2026-09-10T13:00:00Z","pull_requests":[{"number":143,"head":{"ref":"feature/approval","sha":"head-sha","repo":{"id":123,"name":"widget-fork","url":"https://api.github.com/repos/contributor/widget-fork"}}}]}`)
	if err != nil {
		t.Fatalf("decodeGHWorkflowRuns() error = %v", err)
	}
	converted := make([]WorkflowRun, 0, len(runs))
	for _, raw := range runs {
		converted = append(converted, convertRawWorkflowRun(raw))
	}
	pr := &PR{
		Number:        143,
		State:         "open",
		HeadSHA:       "head-sha",
		HeadBranch:    "feature/approval",
		HeadRepoID:    123,
		HeadRepoOwner: "contributor",
		HeadRepoName:  "widget-fork",
		RepoOwner:     "acme",
		RepoName:      "widget",
	}

	selected := selectCurrentWorkflowRuns(converted, pr)
	if len(selected) != 1 || selected[0].ID != 101 {
		t.Fatalf("selected workflow runs = %#v, want only newer associated success", selected)
	}
}

func TestWorkflowAttention_NewExecutionWinsOverLateUpdate(t *testing.T) {
	pr := &PR{
		Number:        7,
		State:         "open",
		HeadSHA:       "head-sha",
		HeadBranch:    "feature/approval",
		HeadRepoOwner: "contributor",
		HeadRepoName:  "widget-fork",
		RepoOwner:     "acme",
		RepoName:      "widget",
	}
	created := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	selected := selectCurrentWorkflowRuns([]WorkflowRun{
		{
			ID:            100,
			RunAttempt:    1,
			WorkflowID:    9,
			Name:          "CI",
			Event:         "pull_request",
			Status:        "completed",
			Conclusion:    "action_required",
			HeadSHA:       pr.HeadSHA,
			HeadBranch:    pr.HeadBranch,
			HeadRepoOwner: pr.HeadRepoOwner,
			HeadRepoName:  pr.HeadRepoName,
			CreatedAt:     created,
			UpdatedAt:     created.Add(2 * time.Hour),
		},
		{
			ID:            101,
			RunAttempt:    1,
			WorkflowID:    9,
			Name:          "CI",
			Event:         "pull_request",
			Status:        "completed",
			Conclusion:    "success",
			HeadSHA:       pr.HeadSHA,
			HeadBranch:    pr.HeadBranch,
			HeadRepoOwner: pr.HeadRepoOwner,
			HeadRepoName:  pr.HeadRepoName,
			CreatedAt:     created.Add(time.Hour),
			UpdatedAt:     created.Add(time.Hour),
		},
	}, pr)
	if len(selected) != 1 || selected[0].ID != 101 {
		t.Fatalf("selected workflow runs = %#v, want only newer execution 101", selected)
	}
}

func TestWorkflowAttention_ActionRequiredWithJobsIsGeneric(t *testing.T) {
	client := &workflowAttentionTestClient{MockClient: NewMockClient()}
	pr := &PR{
		Number: 1, State: "open", HeadSHA: "head-sha", HeadBranch: "feature",
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork", RepoOwner: "acme", RepoName: "widget",
	}
	client.AddPR(pr)
	client.ReplaceWorkflowRuns("acme", "widget", pr.HeadSHA, []WorkflowRun{{
		ID: 44, RunAttempt: 2, WorkflowID: 12, Name: "CI", Event: "pull_request", Status: "completed", Conclusion: "action_required",
		HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch, HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}})
	client.ReplaceWorkflowRunJobs("acme", "widget", 44, 2, []WorkflowJob{{ID: 55, Name: "test", Status: "completed", Conclusion: "success"}})

	attention, err := collectWorkflowAttention(context.Background(), client, "acme", "widget", pr)
	if err != nil {
		t.Fatalf("collectWorkflowAttention() error = %v", err)
	}
	if attention.State != WorkflowAttentionActionRequired || len(attention.Runs) != 1 || attention.Runs[0].Reason != workflowAttentionActionRequiredReason {
		t.Fatalf("attention = %#v, want generic action_required", attention)
	}
}

func TestWorkflowAttention_ClassifierIgnoresTerminalAndHeadlessPRs(t *testing.T) {
	runs := []WorkflowRun{{
		ID: 44, RunAttempt: 1, WorkflowID: 12, Name: "CI", Event: "pull_request", Status: "completed", Conclusion: "action_required",
		HeadSHA: "head-sha", HeadBranch: "feature", HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}}
	readJobs := func(context.Context, int64, int) ([]WorkflowJob, error) {
		t.Fatal("readJobs called for a PR that cannot have active workflow attention")
		return nil, nil
	}

	terminal := classifyWorkflowAttentionWithJobs(context.Background(), "acme", "widget", &PR{
		State: "merged", HeadSHA: "head-sha",
	}, runs, readJobs)
	if terminal.State != WorkflowAttentionNone || terminal.HeadSHA != "head-sha" {
		t.Fatalf("terminal attention = %#v, want authoritative none", terminal)
	}

	headless := classifyWorkflowAttentionWithJobs(context.Background(), "acme", "widget", &PR{
		State: "open",
	}, runs, readJobs)
	if headless.State != WorkflowAttentionUnknown || headless.HeadSHA != "" {
		t.Fatalf("headless attention = %#v, want unknown", headless)
	}
}

func TestWorkflowAttention_DeniedReadIsUnknown(t *testing.T) {
	client := &workflowAttentionTestClient{MockClient: NewMockClient(), runsErr: errors.New("forbidden")}
	pr := &PR{State: "open", HeadSHA: "head-sha", RepoOwner: "acme", RepoName: "widget"}
	attention, err := collectWorkflowAttention(context.Background(), client, "acme", "widget", pr)
	if !errors.Is(err, client.runsErr) {
		t.Fatalf("collectWorkflowAttention() error = %v, want forbidden error", err)
	}
	if attention.State != WorkflowAttentionUnknown || attention.HeadSHA != pr.HeadSHA {
		t.Fatalf("attention = %#v, want unknown same-head observation", attention)
	}
}

func TestWorkflowAttention_UnknownPreservesSameHeadAndClearsOnHeadChange(t *testing.T) {
	previous := &TaskPR{
		HeadSHA: "old-head",
		WorkflowAttention: &WorkflowAttention{
			State:      WorkflowAttentionApprovalRequired,
			HeadSHA:    "old-head",
			ObservedAt: time.Now().UTC().Add(-time.Minute),
			Runs:       []WorkflowAttentionRun{{RunID: 1, Name: "CI", Reason: workflowAttentionApprovalRequiredReason}},
		},
	}
	unknown := &PRStatus{
		PR:                         &PR{HeadSHA: "old-head"},
		WorkflowAttention:          workflowAttentionUnknown("old-head"),
		WorkflowAttentionPopulated: true,
	}
	got := resolveTaskPRWorkflowAttention(previous, unknown, "old-head")
	if got == nil || got.State != WorkflowAttentionApprovalRequired || !got.Stale {
		t.Fatalf("same-head unknown result = %#v, want stale preserved approval", got)
	}
	if !got.ObservedAt.Equal(previous.WorkflowAttention.ObservedAt) {
		t.Fatalf("same-head unavailable observed_at = %v, want last successful observation %v", got.ObservedAt, previous.WorkflowAttention.ObservedAt)
	}

	newHead := resolveTaskPRWorkflowAttention(previous, &PRStatus{
		PR:                         &PR{HeadSHA: "new-head"},
		WorkflowAttention:          workflowAttentionUnknown("new-head"),
		WorkflowAttentionPopulated: true,
	}, "new-head")
	if newHead == nil || newHead.State != WorkflowAttentionUnknown || newHead.HeadSHA != "new-head" || HasActiveWorkflowAttention(&TaskPR{HeadSHA: "new-head", WorkflowAttention: newHead}) {
		t.Fatalf("new-head result = %#v, want unknown without old active claim", newHead)
	}
}

func TestWorkflowAttentionJSONKeepsRunArraysNonNil(t *testing.T) {
	encoded, err := json.Marshal(workflowAttentionNone("head-sha"))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, `"runs":[]`) {
		t.Fatalf("workflow attention JSON = %s, want an empty run array", got)
	}

	tp := &TaskPR{WorkflowAttentionJSON: `{"state":"unknown","head_sha":"head-sha"}`}
	if err := hydrateTaskPRWorkflowAttention(tp); err != nil {
		t.Fatalf("hydrateTaskPRWorkflowAttention: %v", err)
	}
	if tp.WorkflowAttention == nil || tp.WorkflowAttention.Runs == nil {
		t.Fatalf("hydrated runs = %#v, want non-nil empty array", tp.WorkflowAttention.Runs)
	}
}
