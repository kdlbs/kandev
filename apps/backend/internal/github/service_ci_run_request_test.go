package github

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCIRunActionsClient struct {
	mu             sync.Mutex
	pr             *PR
	prSequence     []*PR
	prCalls        int
	run            *GitHubActionsRun
	workflow       *GitHubActionsWorkflow
	workflowSource []byte
	workflowRef    string
	runs           []GitHubActionsRun
	rerunErr       error
	dispatchErr    error
	reruns         int
	dispatches     int
	dispatchRef    string
	dispatchInputs map[string]string
}

func (f *fakeCIRunActionsClient) GetPR(context.Context, string, string, int) (*PR, error) {
	f.prCalls++
	if f.prCalls <= len(f.prSequence) {
		return f.prSequence[f.prCalls-1], nil
	}
	return f.pr, nil
}
func (f *fakeCIRunActionsClient) GetRepoFileContent(
	_ context.Context, _, _, _, ref string,
) ([]byte, error) {
	f.workflowRef = ref
	return f.workflowSource, nil
}
func (f *fakeCIRunActionsClient) GetActionsRun(context.Context, string, string, int64) (*GitHubActionsRun, error) {
	return f.run, nil
}
func (f *fakeCIRunActionsClient) GetActionsWorkflow(context.Context, string, string, int64) (*GitHubActionsWorkflow, error) {
	return f.workflow, nil
}
func (f *fakeCIRunActionsClient) RerunFailedActionsJobs(context.Context, string, string, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reruns++
	return f.rerunErr
}
func (f *fakeCIRunActionsClient) DispatchActionsWorkflow(
	_ context.Context, _, _ string, _ int64, ref string, inputs map[string]string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatches++
	f.dispatchRef = ref
	f.dispatchInputs = inputs
	return f.dispatchErr
}
func (f *fakeCIRunActionsClient) ListActionsWorkflowRuns(context.Context, string, string, int64, string) ([]GitHubActionsRun, error) {
	return f.runs, nil
}

func TestRequestFreshCIRunRerunsVerifiedForkSource(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, true)
	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Operation != CIRunOperationRerunFailedJobs || receipt.RunID != input.SourceRunID || receipt.Attempt != 2 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if client.reruns != 1 || client.dispatches != 0 {
		t.Fatalf("provider calls rerun=%d dispatch=%d", client.reruns, client.dispatches)
	}
	var audit []CIRunAuditEvent
	if err := service.store.ro.Select(&audit, `SELECT id, request_id, event_type, failure_class, details_json, created_at
		FROM github_ci_run_audit_events ORDER BY created_at, event_type`); err != nil {
		t.Fatal(err)
	}
	if len(audit) != 3 {
		t.Fatalf("audit events = %+v, want claimed/provider_started/succeeded", audit)
	}
	for _, event := range audit {
		lower := strings.ToLower(event.DetailsJSON)
		if strings.Contains(lower, "token") || strings.Contains(lower, "authorization") ||
			strings.Contains(lower, "private_key") {
			t.Fatalf("audit leaked a secret-shaped field: %s", event.DetailsJSON)
		}
	}

	replay, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if replay.RequestID != receipt.RequestID || replay.WorkflowName != receipt.WorkflowName ||
		replay.WorkflowPath != receipt.WorkflowPath || client.reruns != 1 {
		t.Fatalf("replay=%+v provider reruns=%d", replay, client.reruns)
	}
}

func TestRequestFreshCIRunDispatchesOnlyReviewedSameRepoWorkflow(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("name: E2E\non:\n  workflow_dispatch:\n    inputs:\n      fail_on_flaky:\n")
	client.runs = []GitHubActionsRun{{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
	}}
	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Operation != CIRunOperationWorkflowDispatch || receipt.RunID != 101 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if client.dispatchRef != "feature/x" || client.dispatchInputs["fail_on_flaky"] != "false" {
		t.Fatalf("dispatch ref/inputs = %q %#v", client.dispatchRef, client.dispatchInputs)
	}
	if client.workflowRef != "main" {
		t.Fatalf("workflow source ref = %q, want live PR base branch", client.workflowRef)
	}
	persisted, err := service.store.GetCIRunRequest(context.Background(), receipt.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Operation != CIRunOperationWorkflowDispatch {
		t.Fatalf("persisted operation = %q, want workflow_dispatch", persisted.Operation)
	}
}

func TestRequestFreshCIRunFailsClosedBeforeProviderMutation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*fakeCIRunActionsClient, *RequestFreshCIRunInput)
		want CIRunFailureClass
	}{
		{name: "head drift", edit: func(c *fakeCIRunActionsClient, _ *RequestFreshCIRunInput) {
			c.pr.HeadSHA = strings.Repeat("b", 40)
		}, want: CIRunFailureHeadDrift},
		{name: "source attempt", edit: func(c *fakeCIRunActionsClient, _ *RequestFreshCIRunInput) {
			c.run.Attempt = 2
		}, want: CIRunFailureSourceRunMismatch},
		{name: "merge evidence", edit: func(_ *fakeCIRunActionsClient, in *RequestFreshCIRunInput) {
			in.EvidenceKind = CIRunEvidenceCurrentMerge
		}, want: CIRunFailureMergeEvidenceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			tt.edit(client, &input)
			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != tt.want {
				t.Fatalf("error = %#v, want %q", err, tt.want)
			}
			if client.reruns != 0 || client.dispatches != 0 {
				t.Fatalf("provider mutated before denial: rerun=%d dispatch=%d", client.reruns, client.dispatches)
			}
		})
	}
}

func TestRequestFreshCIRunDeniesStaticScopeMismatches(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Service, *RequestFreshCIRunInput)
		want CIRunFailureClass
	}{
		{name: "cross workspace actor", edit: func(s *Service, _ *RequestFreshCIRunInput) {
			_, _ = s.store.db.Exec(`UPDATE tasks SET workspace_id = 'workspace-other' WHERE id = 'coordinator-1'`)
		}, want: CIRunFailureCrossWorkspace},
		{name: "wrong current step", edit: func(_ *Service, in *RequestFreshCIRunInput) {
			in.ExpectedWorkflowStepID = "review"
		}, want: CIRunFailureWorkflowStepMismatch},
		{name: "unlinked PR", edit: func(s *Service, _ *RequestFreshCIRunInput) {
			_, _ = s.store.db.Exec(`UPDATE github_task_prs SET detached_at = CURRENT_TIMESTAMP`)
		}, want: CIRunFailureUnlinkedPR},
		{name: "missing grant", edit: func(s *Service, _ *RequestFreshCIRunInput) {
			_, _ = s.store.db.Exec(`DELETE FROM github_ci_run_grants`)
		}, want: CIRunFailureNotAuthorized},
		{name: "unlinked repository", edit: func(_ *Service, in *RequestFreshCIRunInput) {
			in.RepositoryID = "repository-other"
		}, want: CIRunFailureRepositoryMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			tt.edit(service, &input)
			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != tt.want {
				t.Fatalf("error = %#v, want %q", err, tt.want)
			}
			if client.reruns != 0 || client.dispatches != 0 {
				t.Fatalf("provider mutated before denial: rerun=%d dispatch=%d", client.reruns, client.dispatches)
			}
		})
	}
}

func TestRequestFreshCIRunDeniesUnreviewedDispatchWorkflow(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.run.WorkflowPath = ".github/workflows/arbitrary.yml"
	client.workflow.Path = client.run.WorkflowPath
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureDispatchDenied {
		t.Fatalf("error = %#v", err)
	}
	if client.dispatches != 0 {
		t.Fatal("unreviewed workflow was dispatched")
	}
}

func TestRequestFreshCIRunRechecksHeadImmediatelyBeforeDispatch(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	drifted := *client.pr
	drifted.HeadSHA = strings.Repeat("b", 40)
	client.prSequence = []*PR{client.pr, &drifted}

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureHeadDrift {
		t.Fatalf("error = %#v, want head_drift", err)
	}
	if client.dispatches != 0 {
		t.Fatal("workflow was dispatched after the PR head changed")
	}
}

func TestRequestFreshCIRunDeniesForkDispatchAndPreservesProviderClasses(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, true)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureDispatchDenied {
		t.Fatalf("fork fallback error = %#v", err)
	}
	if client.dispatches != 0 {
		t.Fatal("fork workflow was dispatched")
	}

	service, _, input = setupCIRunServiceTest(t, false)
	service.ciRunClientResolver = func(context.Context, string, string, string) (ciRunActionsClient, error) {
		return nil, &CIRunProviderError{Class: CIRunFailureInstallationPermission, StatusCode: 403}
	}
	_, err = service.RequestFreshCIRun(context.Background(), input)
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureInstallationPermission {
		t.Fatalf("permission error = %#v", err)
	}
}

func TestRequestFreshCIRunPersistsProviderFailureClass(t *testing.T) {
	for _, class := range []CIRunFailureClass{
		CIRunFailureProviderRateLimited, CIRunFailureProviderUnavailable,
	} {
		t.Run(string(class), func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			client.rerunErr = &CIRunProviderError{Class: class, StatusCode: 503, Retryable: true}
			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != class {
				t.Fatalf("error = %#v, want %q", err, class)
			}
			var storedClass string
			if err := service.store.ro.Get(&storedClass,
				`SELECT failure_class FROM github_ci_run_requests LIMIT 1`); err != nil {
				t.Fatal(err)
			}
			if storedClass != string(class) {
				t.Fatalf("stored failure class = %q", storedClass)
			}
		})
	}
}

func TestRequestFreshCIRunReconcilesAmbiguousMutationWithoutResending(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureProviderCallAmbiguous}
	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("first error = %#v", err)
	}
	client.runs = []GitHubActionsRun{{
		ID: input.SourceRunID, Attempt: 2, WorkflowID: 77, Event: "pull_request",
		HeadSHA: input.ExpectedHeadSHA, HeadBranch: "feature/x",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
	}}
	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || receipt.Attempt != 2 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if client.reruns != 1 {
		t.Fatalf("provider reruns = %d, want one ambiguous send only", client.reruns)
	}
}

func TestWorkflowDispatchDeclarationMustBeUnderTopLevelOn(t *testing.T) {
	if !workflowDispatchDeclared([]byte("name: E2E\non:\n  workflow_dispatch:\njobs: {}\n")) {
		t.Fatal("top-level on.workflow_dispatch was not recognized")
	}
	for _, source := range []string{
		"jobs:\n  workflow_dispatch:\n",
		"# workflow_dispatch:\non:\n  pull_request:\n",
		"name: workflow_dispatch:\non:\n  push:\n",
	} {
		if workflowDispatchDeclared([]byte(source)) {
			t.Fatalf("untrusted declaration accepted: %q", source)
		}
	}
}

func setupCIRunServiceTest(t *testing.T, fork bool) (*Service, *fakeCIRunActionsClient, RequestFreshCIRunInput) {
	t.Helper()
	store := newTestStore(t)
	ctx := context.Background()
	for _, statement := range []string{
		`ALTER TABLE tasks ADD COLUMN workflow_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN workflow_step_id TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE repositories (id TEXT PRIMARY KEY, workspace_id TEXT, provider TEXT, provider_owner TEXT, provider_name TEXT, provider_repo_id TEXT)`,
		`CREATE TABLE task_repositories (id TEXT PRIMARY KEY, task_id TEXT, repository_id TEXT)`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`INSERT INTO workspaces(id) VALUES ('workspace-1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO tasks(id, workspace_id, workflow_id, workflow_step_id) VALUES
		('coordinator-1','workspace-1','coordinator-workflow','coordinator-step'),
		('target-1','workspace-1','workflow-1','ci-fixup')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO repositories VALUES
		('repository-1','workspace-1','github','kdlbs','kandev','123')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO task_repositories VALUES ('task-repo-1','target-1','repository-1')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	grant := testCIRunGrant(now)
	if err := store.UpsertCIRunGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{
		ID: "task-pr-1", WorkspaceID: "workspace-1", TaskID: "target-1",
		RepositoryID: "repository-1", Owner: "kdlbs", Repo: "kandev", PRNumber: 42,
		PRURL: "https://github.com/kdlbs/kandev/pull/42", PRTitle: "test",
		HeadBranch: "feature/x", BaseBranch: "main", State: "open", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	headRepo := "kdlbs"
	if fork {
		headRepo = "fork"
	}
	headSHA := strings.Repeat("a", 40)
	client := &fakeCIRunActionsClient{
		pr: &PR{
			Number: 42, State: "open", HeadSHA: headSHA, HeadBranch: "feature/x",
			RepoOwner: "kdlbs", RepoName: "kandev", BaseRepoOwner: "kdlbs", BaseRepoName: "kandev",
			HeadRepoOwner: headRepo, HeadRepoName: "kandev", BaseBranch: "main",
		},
		run: &GitHubActionsRun{
			ID: 100, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
			WorkflowPath: ".github/workflows/e2e-tests.yml", Event: "pull_request",
			Status: "completed", Conclusion: "failure", HeadSHA: headSHA, HeadBranch: "feature/x",
			Repository: "kdlbs/kandev", HeadRepository: headRepo + "/kandev",
		},
		workflow: &GitHubActionsWorkflow{ID: 77, Name: "E2E", Path: ".github/workflows/e2e-tests.yml", State: "active"},
	}
	service := NewService(nil, AuthMethodNone, nil, store, nil, nil)
	service.ciRunClientResolver = func(context.Context, string, string, string) (ciRunActionsClient, error) {
		return client, nil
	}
	service.ciRunNow = func() time.Time { return now }
	input := RequestFreshCIRunInput{
		ActorTaskID: "coordinator-1", ActorSessionID: "session-1", TargetTaskID: "target-1",
		RepositoryID: "repository-1", PRNumber: 42, ExpectedHeadSHA: headSHA,
		ExpectedWorkflowStepID: "ci-fixup", SourceRunID: 100, ExpectedSourceAttempt: 1,
		EvidenceKind: CIRunEvidencePRHead, IdempotencyKey: "consumer-42-attempt-1",
	}
	return service, client, input
}
