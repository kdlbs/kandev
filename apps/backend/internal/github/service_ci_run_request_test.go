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
	mu               sync.Mutex
	pr               *PR
	prSequence       []*PR
	prErrSequence    []error
	prCalls          int
	run              *GitHubActionsRun
	runSequence      []*GitHubActionsRun
	runCalls         int
	workflow         *GitHubActionsWorkflow
	workflowSource   []byte
	workflowHead     []byte
	workflowRefs     []string
	workflowHook     func()
	runHook          func(int)
	listHook         func()
	listErr          error
	listErrSequence  []error
	listCalls        int
	runs             []GitHubActionsRun
	preDispatchRuns  []GitHubActionsRun
	rerunErr         error
	dispatchErr      error
	reruns           int
	dispatches       int
	dispatchRef      string
	dispatchInputs   map[string]string
	principal        TokenPrincipal
	mutationMetadata GitHubRequestMetadata
}

func (f *fakeCIRunActionsClient) GetPR(context.Context, string, string, int) (*PR, error) {
	f.prCalls++
	if f.prCalls <= len(f.prErrSequence) && f.prErrSequence[f.prCalls-1] != nil {
		return nil, f.prErrSequence[f.prCalls-1]
	}
	if f.prCalls <= len(f.prSequence) {
		return f.prSequence[f.prCalls-1], nil
	}
	return f.pr, nil
}

func (f *fakeCIRunActionsClient) GetRepoFileContent(
	_ context.Context, _, _, _, ref string,
) ([]byte, error) {
	f.workflowRefs = append(f.workflowRefs, ref)
	if f.workflowHead != nil && ref == "feature/x" {
		return f.workflowHead, nil
	}
	return f.workflowSource, nil
}
func (f *fakeCIRunActionsClient) GetActionsRun(context.Context, string, string, int64) (*GitHubActionsRun, error) {
	f.runCalls++
	if f.runHook != nil {
		f.runHook(f.runCalls)
	}
	if f.runCalls <= len(f.runSequence) {
		return f.runSequence[f.runCalls-1], nil
	}
	return f.run, nil
}
func (f *fakeCIRunActionsClient) GetActionsWorkflow(context.Context, string, string, int64) (*GitHubActionsWorkflow, error) {
	if f.workflowHook != nil {
		f.workflowHook()
	}
	return f.workflow, nil
}
func (f *fakeCIRunActionsClient) RerunFailedActionsJobs(context.Context, string, string, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reruns++
	return f.rerunErr
}
func (f *fakeCIRunActionsClient) RerunFailedActionsJobsWithMetadata(
	context.Context, string, string, int64,
) (GitHubRequestMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reruns++
	return f.mutationMetadata, f.rerunErr
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
func (f *fakeCIRunActionsClient) DispatchActionsWorkflowWithMetadata(
	_ context.Context, _, _ string, _ int64, ref string, inputs map[string]string,
) (GitHubRequestMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatches++
	f.dispatchRef = ref
	f.dispatchInputs = inputs
	return f.mutationMetadata, f.dispatchErr
}
func (f *fakeCIRunActionsClient) Principal() TokenPrincipal { return f.principal }
func (f *fakeCIRunActionsClient) ListActionsWorkflowRuns(context.Context, string, string, int64, string) ([]GitHubActionsRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if f.listHook != nil {
		f.listHook()
	}
	if f.listCalls <= len(f.listErrSequence) && f.listErrSequence[f.listCalls-1] != nil {
		return nil, f.listErrSequence[f.listCalls-1]
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.dispatches == 0 && (f.reruns == 0 ||
		ciRunFailureFromError(f.rerunErr) == CIRunFailureRerunIneligible) {
		return f.preDispatchRuns, nil
	}
	return f.runs, nil
}

func TestRequestFreshCIRunRejectsWrongExplicitPRAssociation(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.run.PullRequests = []int{input.PRNumber + 1}

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureSourceRunMismatch {
		t.Fatalf("error = %#v, want source_run_mismatch", err)
	}
	if client.reruns != 0 || client.dispatches != 0 {
		t.Fatalf("provider mutated for wrong PR association: rerun=%d dispatch=%d",
			client.reruns, client.dispatches)
	}
}

func TestRequestFreshCIRunUsesTupleFallbackOnlyWithoutPRAssociations(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.run.PullRequests = nil
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || client.reruns != 1 {
		t.Fatalf("receipt = %+v, reruns = %d", receipt, client.reruns)
	}
}

func TestRequestFreshCIRunRevalidatesSourceAttemptAtRerunBoundary(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	advanced := *client.run
	advanced.Attempt++
	client.runSequence = []*GitHubActionsRun{client.run, &advanced}

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureSourceRunMismatch {
		t.Fatalf("error = %#v, want source_run_mismatch", err)
	}
	if client.runCalls != 2 || client.reruns != 0 {
		t.Fatalf("source reads = %d, provider reruns = %d, want 2 and 0", client.runCalls, client.reruns)
	}
}

func TestRequestFreshCIRunRerunsVerifiedForkSource(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, true)
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
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

func TestRequestFreshCIRunRejectsSourceThatIsNotACompletedFailure(t *testing.T) {
	for _, source := range []struct {
		name       string
		status     string
		conclusion string
	}{
		{name: "still running", status: "in_progress"},
		{name: "already successful", status: "completed", conclusion: "success"},
		{name: "cancelled", status: "completed", conclusion: "cancelled"},
	} {
		t.Run(source.name, func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			client.run.Status = source.status
			client.run.Conclusion = source.conclusion

			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureSourceRunMismatch {
				t.Fatalf("error = %#v, want source_run_mismatch", err)
			}
			if client.reruns != 0 || client.dispatches != 0 {
				t.Fatalf("provider mutated for ineligible source: rerun=%d dispatch=%d",
					client.reruns, client.dispatches)
			}
		})
	}
}

func TestRequestFreshCIRunConfirmsRerunAttemptBeforeSuccess(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, true)

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || receipt.Attempt != 0 {
		t.Fatalf("unconfirmed receipt = %+v", receipt)
	}
	if client.reruns != 1 {
		t.Fatalf("provider reruns = %d, want one", client.reruns)
	}

	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
	receipt, err = service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || receipt.Attempt != 2 || client.reruns != 1 {
		t.Fatalf("confirmed receipt = %+v, provider reruns=%d", receipt, client.reruns)
	}
}

func TestRequestFreshCIRunDoesNotClaimALaterRerunAttempt(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, true)
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 2

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || receipt.Attempt != 0 {
		t.Fatalf("misattributed receipt = %+v", receipt)
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
		CreatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
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
	if len(client.workflowRefs) != 2 || client.workflowRefs[0] != "main" ||
		client.workflowRefs[1] != "feature/x" {
		t.Fatalf("workflow source refs = %#v, want trusted base then live PR head", client.workflowRefs)
	}
	persisted, err := service.store.GetCIRunRequest(context.Background(), receipt.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Operation != CIRunOperationWorkflowDispatch {
		t.Fatalf("persisted operation = %q, want workflow_dispatch", persisted.Operation)
	}
}

func TestRequestFreshCIRunRejectsAmbiguousDispatchMatches(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	matching := GitHubActionsRun{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
	}
	client.runs = []GitHubActionsRun{matching, matching}
	client.runs[1].ID = 102

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || receipt.RunID != 0 {
		t.Fatalf("ambiguous receipt = %+v", receipt)
	}
	if client.dispatches != 1 {
		t.Fatalf("provider dispatches = %d, want one", client.dispatches)
	}
}

func TestRequestFreshCIRunRejectsDispatchRunCreatedBeforeProviderCall(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	client.runs = []GitHubActionsRun{{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
		CreatedAt: time.Date(2026, 8, 30, 11, 59, 59, 0, time.UTC),
	}}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || receipt.RunID != 0 {
		t.Fatalf("stale-match receipt = %+v", receipt)
	}
}

func TestRequestFreshCIRunDoesNotAttributeOlderSameSecondDispatch(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	service.ciRunNow = func() time.Time {
		return time.Date(2026, 8, 30, 12, 0, 0, 900_000_000, time.UTC)
	}
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	older := GitHubActionsRun{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
		CreatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	}
	client.preDispatchRuns = []GitHubActionsRun{older}
	client.runs = []GitHubActionsRun{older}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if receipt.Status != CIRunRequestReconciling || receipt.RunID != 0 {
		t.Fatalf("older run was attributed: %+v", receipt)
	}

	actual := older
	actual.ID = 102
	client.runs = append(client.runs, actual)
	receipt, err = service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || receipt.RunID != actual.ID || client.dispatches != 1 {
		t.Fatalf("receipt = %+v, dispatches = %d", receipt, client.dispatches)
	}
}

func TestRequestFreshCIRunResumesPreparedDispatchWithoutRepeatingRerun(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	client.runs = []GitHubActionsRun{{
		ID: 101, Attempt: 1, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: reviewedDispatchWorkflow, HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
		CreatedAt: service.ciRunClock()().UTC(),
	}}

	binding, err := service.loadCIRunBinding(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := service.store.ClaimCIRunRequest(
		context.Background(), newCIRunRequest(binding, input, service.ciRunClock()()),
	)
	if err != nil {
		t.Fatal(err)
	}
	now := service.ciRunClock()().UTC()
	if acquired, err := service.store.AcquireCIRunExecution(
		context.Background(), request, "crashed-after-422", now, time.Minute,
	); err != nil || !acquired {
		t.Fatalf("acquire rerun worker = %v, %v", acquired, err)
	}
	request.Operation = CIRunOperationRerunFailedJobs
	request.ProviderWorkflowID = client.workflow.ID
	request.ProviderWorkflowName = client.workflow.Name
	request.ProviderWorkflowPath = client.workflow.Path
	request.ProviderHeadRepo = client.run.HeadRepository
	request.ProviderHeadRef = client.run.HeadBranch
	request.ProviderHeadSHA = client.run.HeadSHA
	if err := service.store.MarkCIRunProviderCallStarted(context.Background(), request, now); err != nil {
		t.Fatal(err)
	}
	if err := service.store.PrepareCIRunDispatchFallback(context.Background(), request, now); err != nil {
		t.Fatal(err)
	}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || receipt.Operation != CIRunOperationWorkflowDispatch ||
		client.reruns != 0 || client.dispatches != 1 {
		t.Fatalf("receipt = %+v, reruns=%d dispatches=%d", receipt, client.reruns, client.dispatches)
	}
}

func TestRequestFreshCIRunDoesNotClaimARerunOfAnOlderDispatch(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	client.runs = []GitHubActionsRun{{
		ID: 101, Attempt: 2, WorkflowID: 77, WorkflowName: "E2E",
		WorkflowPath: ".github/workflows/e2e-tests.yml", HeadSHA: input.ExpectedHeadSHA,
		HeadBranch: "feature/x", Event: "workflow_dispatch",
		Repository: "kdlbs/kandev", HeadRepository: "kdlbs/kandev",
		CreatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	}}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderCallAmbiguous {
		t.Fatalf("error = %#v, want provider_call_ambiguous", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestReconciling || receipt.RunID != 0 {
		t.Fatalf("misattributed receipt = %+v", receipt)
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
		}, want: CIRunFailureNotAuthorized},
		{name: "missing target", edit: func(s *Service, _ *RequestFreshCIRunInput) {
			_, _ = s.store.db.Exec(`DELETE FROM tasks WHERE id = 'target-1'`)
		}, want: CIRunFailureNotAuthorized},
		{name: "unrelated target", edit: func(_ *Service, in *RequestFreshCIRunInput) {
			in.TargetTaskID = "other-task"
		}, want: CIRunFailureNotAuthorized},
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
		}, want: CIRunFailureNotAuthorized},
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

func TestRequestFreshCIRunDeniesSessionsOutsideTheCoordinatorTask(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
	}{
		{name: "missing session", sessionID: "session-missing"},
		{name: "session owned by another task", sessionID: "session-other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			input.ActorSessionID = tt.sessionID

			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureNotAuthorized {
				t.Fatalf("error = %#v, want not_authorized", err)
			}
			if client.reruns != 0 || client.dispatches != 0 {
				t.Fatalf("provider mutated for foreign session: rerun=%d dispatch=%d",
					client.reruns, client.dispatches)
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

func TestRequestFreshCIRunDeniesChangedHeadWorkflow(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	client.workflowSource = []byte("on:\n  workflow_dispatch:\n")
	client.workflowHead = []byte("on:\n  workflow_dispatch:\njobs:\n  unreviewed: {}\n")

	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureDispatchDenied {
		t.Fatalf("error = %#v, want workflow_dispatch_denied", err)
	}
	if client.dispatches != 0 {
		t.Fatal("changed head workflow was dispatched")
	}
}

func TestRequestFreshCIRunRevalidatesGrantAndLaneBeforeProviderWrite(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Service)
		want CIRunFailureClass
	}{
		{name: "grant revoked", edit: func(service *Service) {
			_, _ = service.store.db.Exec(`UPDATE github_ci_run_grants SET revoked_at = CURRENT_TIMESTAMP`)
		}, want: CIRunFailureNotAuthorized},
		{name: "lane changed", edit: func(service *Service) {
			_, _ = service.store.db.Exec(`UPDATE tasks SET workflow_step_id = 'review' WHERE id = 'target-1'`)
		}, want: CIRunFailureWorkflowStepMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			client.workflowHook = func() { tt.edit(service) }
			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != tt.want {
				t.Fatalf("error = %#v, want %q", err, tt.want)
			}
			if client.reruns != 0 || client.dispatches != 0 {
				t.Fatalf("provider mutated after admission drift: rerun=%d dispatch=%d",
					client.reruns, client.dispatches)
			}
		})
	}
}

func TestRequestFreshCIRunDeniesForkDispatchAndPreservesProviderClasses(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, true)
	client.rerunErr = &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: 422}
	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureForkDispatchDisallowed {
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
	service, client, input := setupCIRunServiceTest(t, false)
	client.rerunErr = &CIRunProviderError{
		Class: CIRunFailureProviderUnavailable, StatusCode: 503, Retryable: true,
	}
	_, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureProviderUnavailable {
		t.Fatalf("error = %#v, want provider_unavailable", err)
	}
	var storedClass string
	if err := service.store.ro.Get(&storedClass,
		`SELECT failure_class FROM github_ci_run_requests LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if storedClass != string(CIRunFailureProviderUnavailable) {
		t.Fatalf("stored failure class = %q", storedClass)
	}
}

func TestRequestFreshCIRunIdempotencyIsActorScopedAcrossSessions(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1
	if _, err := service.RequestFreshCIRun(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.store.db.Exec(`INSERT INTO task_sessions(id, task_id) VALUES ('session-2', 'coordinator-1')`); err != nil {
		t.Fatal(err)
	}
	input.ActorSessionID = "session-2"
	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != CIRunRequestSucceeded || receipt.IdempotencyStatus != "replayed" || client.reruns != 1 {
		t.Fatalf("receipt = %+v, provider reruns = %d, want one", receipt, client.reruns)
	}
}

func TestRequestFreshCIRunAuditFailurePreventsProviderMutation(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.workflowHook = func() {
		client.workflowHook = nil
		if _, err := service.store.db.Exec(`DROP TABLE github_ci_run_audit_events`); err != nil {
			t.Fatalf("drop audit table: %v", err)
		}
	}
	_, err := service.RequestFreshCIRun(context.Background(), input)
	if err == nil {
		t.Fatal("request succeeded despite audit failure")
	}
	if client.reruns != 0 || client.dispatches != 0 {
		t.Fatalf("provider mutated after audit failure: rerun=%d dispatch=%d", client.reruns, client.dispatches)
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
		"on: workflow_dispatch\n",
		"on: [push, workflow_dispatch]\n",
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
		`CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL)`,
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
		('target-1','workspace-1','workflow-1','ci-fixup'),
		('other-task','workspace-1','other-workflow','other-step')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO task_sessions(id, task_id) VALUES
		('session-1', 'coordinator-1'), ('session-other', 'other-task')`); err != nil {
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
