package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ciRunActionsClient interface {
	GetPR(context.Context, string, string, int) (*PR, error)
	GetRepoFileContent(context.Context, string, string, string, string) ([]byte, error)
	GetActionsRun(context.Context, string, string, int64) (*GitHubActionsRun, error)
	GetActionsWorkflow(context.Context, string, string, int64) (*GitHubActionsWorkflow, error)
	RerunFailedActionsJobs(context.Context, string, string, int64) error
	DispatchActionsWorkflow(context.Context, string, string, int64, string, map[string]string) error
	ListActionsWorkflowRuns(context.Context, string, string, int64, string) ([]GitHubActionsRun, error)
}

type RequestFreshCIRunInput struct {
	ActorTaskID            string            `json:"actor_task_id"`
	ActorSessionID         string            `json:"actor_session_id"`
	TargetTaskID           string            `json:"task_id"`
	RepositoryID           string            `json:"repository_id"`
	PRNumber               int               `json:"pr_number"`
	ExpectedHeadSHA        string            `json:"expected_head_sha"`
	ExpectedWorkflowStepID string            `json:"expected_workflow_step_id"`
	SourceRunID            int64             `json:"source_run_id"`
	ExpectedSourceAttempt  int               `json:"expected_source_attempt"`
	EvidenceKind           CIRunEvidenceKind `json:"evidence_kind"`
	IdempotencyKey         string            `json:"idempotency_key"`
}

type CIRunRequestError struct {
	Class CIRunFailureClass `json:"failure_class"`
}

func (e *CIRunRequestError) Error() string {
	return "fresh CI run request failed (" + string(e.Class) + ")"
}

type ciRunBinding struct {
	WorkspaceID  string
	WorkflowID   string
	WorkflowStep string
	Owner        string
	Repo         string
	TaskPR       *TaskPR
	Grant        *CIRunGrant
}

func (s *Service) RequestFreshCIRun(
	ctx context.Context,
	input RequestFreshCIRunInput,
) (*CIRunReceipt, error) {
	if failure := validateFreshCIRunInput(input); failure != "" {
		return nil, &CIRunRequestError{Class: failure}
	}
	binding, err := s.loadCIRunBinding(ctx, input)
	if err != nil {
		return nil, err
	}
	request := newCIRunRequest(binding, input, s.ciRunClock()())
	claimed, created, err := s.store.ClaimCIRunRequest(ctx, request)
	if err != nil {
		if errors.Is(err, ErrCIRunSemanticConflict) && claimed != nil {
			return s.resumeCIRunRequest(ctx, binding, claimed)
		}
		if errors.Is(err, ErrCIRunIdempotencyConflict) {
			return nil, &CIRunRequestError{Class: CIRunFailureIdempotencyConflict}
		}
		return nil, err
	}
	if !created {
		return s.resumeCIRunRequest(ctx, binding, claimed)
	}
	_ = s.auditCIRun(ctx, claimed, "claimed", "")
	if input.EvidenceKind == CIRunEvidenceCurrentMerge {
		return s.failCIRunRequest(ctx, claimed, CIRunFailureMergeEvidenceUnavailable)
	}
	client, err := s.resolveCIRunClient(ctx, binding.WorkspaceID, binding.Owner, binding.Repo)
	if err != nil {
		return s.failCIRunRequest(ctx, claimed, ciRunFailureFromError(err))
	}
	verified, err := verifyCIRunProviderBinding(ctx, client, binding, input)
	if err != nil {
		return s.failCIRunRequest(ctx, claimed, ciRunFailureFromError(err))
	}
	return s.executeCIRunRequest(ctx, client, claimed, verified)
}

func validateFreshCIRunInput(input RequestFreshCIRunInput) CIRunFailureClass {
	if input.ActorTaskID == "" || input.ActorSessionID == "" || input.TargetTaskID == "" ||
		input.RepositoryID == "" || input.PRNumber <= 0 || len(input.ExpectedHeadSHA) != 40 ||
		input.ExpectedWorkflowStepID == "" || input.SourceRunID <= 0 ||
		input.ExpectedSourceAttempt <= 0 || input.IdempotencyKey == "" ||
		len(input.IdempotencyKey) > 256 {
		return CIRunFailureTaskMismatch
	}
	if input.EvidenceKind != CIRunEvidencePRHead && input.EvidenceKind != CIRunEvidenceCurrentMerge {
		return CIRunFailureTaskMismatch
	}
	return ""
}

func (s *Service) loadCIRunBinding(ctx context.Context, input RequestFreshCIRunInput) (*ciRunBinding, error) {
	type taskIdentity struct {
		WorkspaceID    string `db:"workspace_id"`
		WorkflowID     string `db:"workflow_id"`
		WorkflowStepID string `db:"workflow_step_id"`
	}
	var actor, target taskIdentity
	if err := s.store.ro.GetContext(ctx, &actor, s.store.ro.Rebind(
		`SELECT workspace_id, workflow_id, workflow_step_id FROM tasks WHERE id = ?`), input.ActorTaskID); err != nil {
		return nil, &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	if err := s.store.ro.GetContext(ctx, &target, s.store.ro.Rebind(
		`SELECT workspace_id, workflow_id, workflow_step_id FROM tasks WHERE id = ?`), input.TargetTaskID); err != nil {
		return nil, &CIRunRequestError{Class: CIRunFailureTaskMismatch}
	}
	if actor.WorkspaceID == "" || actor.WorkspaceID != target.WorkspaceID {
		return nil, &CIRunRequestError{Class: CIRunFailureCrossWorkspace}
	}
	if target.WorkflowStepID != input.ExpectedWorkflowStepID {
		return nil, &CIRunRequestError{Class: CIRunFailureWorkflowStepMismatch}
	}
	owner, repo, err := s.loadCIRunRepository(ctx, target.WorkspaceID, input.TargetTaskID, input.RepositoryID)
	if err != nil {
		return nil, err
	}
	taskPR, err := s.store.GetTaskPRByRepoAndNumber(ctx, input.TargetTaskID, input.RepositoryID, input.PRNumber)
	if err != nil || taskPR == nil || taskPR.State != defaultPRState {
		return nil, &CIRunRequestError{Class: CIRunFailureUnlinkedPR}
	}
	if !strings.EqualFold(taskPR.Owner, owner) || !strings.EqualFold(taskPR.Repo, repo) {
		return nil, &CIRunRequestError{Class: CIRunFailureRepositoryMismatch}
	}
	grant, err := s.store.GetActiveCIRunGrant(ctx, target.WorkspaceID, input.ActorTaskID,
		input.TargetTaskID, target.WorkflowID, target.WorkflowStepID, input.RepositoryID)
	if err != nil || grant == nil {
		return nil, &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	return &ciRunBinding{WorkspaceID: target.WorkspaceID, WorkflowID: target.WorkflowID,
		WorkflowStep: target.WorkflowStepID, Owner: owner, Repo: repo, TaskPR: taskPR, Grant: grant}, nil
}

func (s *Service) loadCIRunRepository(
	ctx context.Context, workspaceID, taskID, repositoryID string,
) (string, string, error) {
	var repository struct {
		Provider string `db:"provider"`
		Owner    string `db:"provider_owner"`
		Name     string `db:"provider_name"`
	}
	err := s.store.ro.GetContext(ctx, &repository, s.store.ro.Rebind(`
		SELECT r.provider, r.provider_owner, r.provider_name
		FROM repositories r JOIN task_repositories tr ON tr.repository_id = r.id
		WHERE r.id = ? AND r.workspace_id = ? AND tr.task_id = ?`), repositoryID, workspaceID, taskID)
	if err != nil || repository.Provider != "github" || repository.Owner == "" || repository.Name == "" {
		return "", "", &CIRunRequestError{Class: CIRunFailureRepositoryMismatch}
	}
	return repository.Owner, repository.Name, nil
}

func newCIRunRequest(binding *ciRunBinding, input RequestFreshCIRunInput, now time.Time) *CIRunRequest {
	digest := sha256.Sum256([]byte(input.IdempotencyKey))
	return &CIRunRequest{
		ID: uuid.NewString(), GrantID: binding.Grant.ID, WorkspaceID: binding.WorkspaceID,
		ActorTaskID: input.ActorTaskID, ActorSessionID: input.ActorSessionID,
		TargetTaskID: input.TargetTaskID, WorkflowID: binding.WorkflowID,
		WorkflowStepID: binding.WorkflowStep, RepositoryID: input.RepositoryID,
		PRNumber: input.PRNumber, ExpectedHeadSHA: strings.ToLower(input.ExpectedHeadSHA),
		SourceRunID: input.SourceRunID, ExpectedSourceAttempt: input.ExpectedSourceAttempt,
		EvidenceKind: input.EvidenceKind, IdempotencyHash: hex.EncodeToString(digest[:]),
		Status: CIRunRequestPending, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
}

func (s *Service) ciRunClock() func() time.Time {
	if s.ciRunNow != nil {
		return s.ciRunNow
	}
	return time.Now
}

func (s *Service) resolveCIRunClient(
	ctx context.Context, workspaceID, owner, repo string,
) (ciRunActionsClient, error) {
	if s.ciRunClientResolver != nil {
		return s.ciRunClientResolver(ctx, workspaceID, owner, repo)
	}
	if s.resolver == nil {
		return nil, &CIRunProviderError{Class: CIRunFailureInstallationPermission}
	}
	resolved, err := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID, Purpose: CredentialPurposeScopedActionsWrite,
		RepoOwner: owner, RepoName: repo,
	})
	if err != nil || resolved == nil || resolved.Principal.Kind != AuthPrincipalApp ||
		resolved.Principal.Source != ConnectionSourceGitHubAppInstallation ||
		!resolved.Capabilities[CapabilityActionsWrite] {
		return nil, &CIRunProviderError{Class: CIRunFailureInstallationPermission}
	}
	client, ok := resolved.Client.(ciRunActionsClient)
	if !ok {
		return nil, &CIRunProviderError{Class: CIRunFailureInstallationPermission}
	}
	return client, nil
}

type verifiedCIRun struct {
	PR       *PR
	Run      *GitHubActionsRun
	Workflow *GitHubActionsWorkflow
}

func verifyCIRunProviderBinding(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	input RequestFreshCIRunInput,
) (*verifiedCIRun, error) {
	pr, err := client.GetPR(ctx, binding.Owner, binding.Repo, input.PRNumber)
	if err != nil {
		return nil, err
	}
	if err := verifyCIRunPR(pr, binding, input); err != nil {
		return nil, err
	}
	run, err := client.GetActionsRun(ctx, binding.Owner, binding.Repo, input.SourceRunID)
	if err != nil {
		return nil, err
	}
	if !sourceRunMatches(run, pr, binding, input) {
		return nil, &CIRunRequestError{Class: CIRunFailureSourceRunMismatch}
	}
	workflow, err := client.GetActionsWorkflow(ctx, binding.Owner, binding.Repo, run.WorkflowID)
	if err != nil {
		return nil, err
	}
	if workflow == nil || workflow.ID != run.WorkflowID ||
		workflow.State != string(AppRegistrationStatusActive) ||
		workflow.Path == "" || (run.WorkflowPath != "" && workflow.Path != run.WorkflowPath) {
		return nil, &CIRunRequestError{Class: CIRunFailureSourceRunMismatch}
	}
	return &verifiedCIRun{PR: pr, Run: run, Workflow: workflow}, nil
}

func verifyCIRunPR(pr *PR, binding *ciRunBinding, input RequestFreshCIRunInput) error {
	if pr == nil || pr.Number != input.PRNumber || pr.State != defaultPRState ||
		!strings.EqualFold(pr.BaseRepoOwner, binding.Owner) ||
		!strings.EqualFold(pr.BaseRepoName, binding.Repo) {
		return &CIRunRequestError{Class: CIRunFailureRepositoryMismatch}
	}
	if !strings.EqualFold(pr.HeadSHA, input.ExpectedHeadSHA) {
		return &CIRunRequestError{Class: CIRunFailureHeadDrift}
	}
	return nil
}

func sourceRunMatches(
	run *GitHubActionsRun, pr *PR, binding *ciRunBinding, input RequestFreshCIRunInput,
) bool {
	if run == nil || run.ID != input.SourceRunID || run.Attempt != input.ExpectedSourceAttempt ||
		run.Event != "pull_request" || run.Status != checkStatusCompleted || run.Conclusion != checkConclusionFail ||
		!strings.EqualFold(run.HeadSHA, input.ExpectedHeadSHA) ||
		!strings.EqualFold(run.Repository, binding.Owner+"/"+binding.Repo) {
		return false
	}
	for _, number := range run.PullRequests {
		if number == input.PRNumber {
			return true
		}
	}
	return strings.EqualFold(run.HeadRepository, pr.HeadRepoOwner+"/"+pr.HeadRepoName) &&
		run.HeadBranch == pr.HeadBranch
}

func (s *Service) executeCIRunRequest(
	ctx context.Context,
	client ciRunActionsClient,
	request *CIRunRequest,
	verified *verifiedCIRun,
) (*CIRunReceipt, error) {
	latestBinding, latestPR, err := s.revalidateCIRunAdmission(ctx, client, request)
	if err != nil {
		return s.failCIRunAdmission(ctx, request, err)
	}
	verified.PR = latestPR
	now := s.ciRunClock()().UTC()
	request.Operation = CIRunOperationRerunFailedJobs
	request.ProviderWorkflowID = verified.Workflow.ID
	request.ProviderWorkflowName = verified.Workflow.Name
	request.ProviderWorkflowPath = verified.Workflow.Path
	request.ProviderHeadRepo = verified.Run.HeadRepository
	request.ProviderHeadRef = verified.Run.HeadBranch
	request.ProviderHeadSHA = verified.Run.HeadSHA
	if err := s.store.MarkCIRunProviderCallStarted(ctx, request, now); err != nil {
		return nil, err
	}
	request.ProviderCallStartedAt = &now
	request.Status = CIRunRequestReconciling
	_ = s.auditCIRun(ctx, request, "provider_started", "")
	err = client.RerunFailedActionsJobs(ctx, latestBinding.Owner, latestBinding.Repo, request.SourceRunID)
	if err == nil {
		return s.reconcileCIRunRequest(ctx, client, latestBinding, request, verified)
	}
	if ciRunFailureFromError(err) != CIRunFailureRerunIneligible {
		return s.handleCIRunMutationError(ctx, request, err)
	}
	return s.executeCIRunDispatchFallback(ctx, client, latestBinding, request, verified)
}

func (s *Service) executeCIRunDispatchFallback(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	request *CIRunRequest,
	verified *verifiedCIRun,
) (*CIRunReceipt, error) {
	if !sameRepositoryPR(verified.PR, binding) {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	inputs, ok := reviewedWorkflowDispatchInputs(verified.Workflow.Path)
	if !ok {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	source, err := client.GetRepoFileContent(ctx, binding.Owner, binding.Repo,
		verified.Workflow.Path, verified.PR.BaseBranch)
	if err != nil {
		class := ciRunFailureFromError(classifyCIRunProviderError(err, false, false))
		if class == CIRunFailureProviderRateLimited || class == CIRunFailureProviderUnavailable ||
			class == CIRunFailureInstallationPermission {
			return s.failCIRunRequest(ctx, request, class)
		}
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	if !workflowDispatchDeclared(source) {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	latestBinding, latestPR, err := s.revalidateCIRunAdmission(ctx, client, request)
	if err != nil {
		return s.failCIRunAdmission(ctx, request, err)
	}
	headSource, err := client.GetRepoFileContent(ctx, latestBinding.Owner, latestBinding.Repo,
		verified.Workflow.Path, latestPR.HeadBranch)
	if err != nil {
		return s.failCIRunRequest(ctx, request, ciRunFailureFromError(err))
	}
	if !bytes.Equal(source, headSource) {
		return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
	}
	request.Operation = CIRunOperationWorkflowDispatch
	dispatchStartedAt := s.ciRunClock()().UTC()
	if err := s.store.MarkCIRunProviderCallStarted(ctx, request, dispatchStartedAt); err != nil {
		return nil, err
	}
	request.ProviderCallStartedAt = &dispatchStartedAt
	_ = s.auditCIRun(ctx, request, "provider_started", "")
	if err := client.DispatchActionsWorkflow(ctx, latestBinding.Owner, latestBinding.Repo,
		verified.Workflow.ID, latestPR.HeadBranch, inputs); err != nil {
		return s.handleCIRunMutationError(ctx, request, err)
	}
	return s.reconcileCIRunRequest(ctx, client, latestBinding, request, verified)
}

func (s *Service) revalidateCIRunAdmission(
	ctx context.Context,
	client ciRunActionsClient,
	request *CIRunRequest,
) (*ciRunBinding, *PR, error) {
	input := RequestFreshCIRunInput{
		ActorTaskID: request.ActorTaskID, ActorSessionID: request.ActorSessionID,
		TargetTaskID: request.TargetTaskID, RepositoryID: request.RepositoryID,
		PRNumber: request.PRNumber, ExpectedHeadSHA: request.ExpectedHeadSHA,
		ExpectedWorkflowStepID: request.WorkflowStepID, SourceRunID: request.SourceRunID,
		ExpectedSourceAttempt: request.ExpectedSourceAttempt, EvidenceKind: request.EvidenceKind,
		IdempotencyKey: "revalidation-only",
	}
	binding, err := s.loadCIRunBinding(ctx, input)
	if err != nil {
		return nil, nil, err
	}
	if binding.Grant == nil || binding.Grant.ID != request.GrantID {
		return nil, nil, &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	pr, err := client.GetPR(ctx, binding.Owner, binding.Repo, request.PRNumber)
	if err != nil {
		return nil, nil, err
	}
	if err := verifyCIRunPR(pr, binding, input); err != nil {
		return nil, nil, err
	}
	return binding, pr, nil
}

func (s *Service) failCIRunAdmission(
	ctx context.Context,
	request *CIRunRequest,
	err error,
) (*CIRunReceipt, error) {
	var requestErr *CIRunRequestError
	if errors.As(err, &requestErr) {
		return s.failCIRunRequest(ctx, request, requestErr.Class)
	}
	return s.failCIRunRequest(ctx, request, ciRunFailureFromError(err))
}

func sameRepositoryPR(pr *PR, binding *ciRunBinding) bool {
	return pr != nil && strings.EqualFold(pr.HeadRepoOwner, binding.Owner) &&
		strings.EqualFold(pr.HeadRepoName, binding.Repo)
}

func reviewedWorkflowDispatchInputs(path string) (map[string]string, bool) {
	if path != ".github/workflows/e2e-tests.yml" {
		return nil, false
	}
	return map[string]string{"fail_on_flaky": "false"}, true
}

func workflowDispatchDeclared(source []byte) bool {
	inOnBlock := false
	for _, line := range strings.Split(string(source), "\n") {
		withoutComment := strings.SplitN(line, "#", 2)[0]
		trimmed := strings.TrimSpace(withoutComment)
		if trimmed == "" {
			continue
		}
		indent := len(withoutComment) - len(strings.TrimLeft(withoutComment, " \t"))
		if indent == 0 {
			inOnBlock = trimmed == "on:"
			continue
		}
		if inOnBlock && (trimmed == "workflow_dispatch:" || trimmed == "workflow_dispatch: {}") {
			return true
		}
	}
	return false
}

func (s *Service) resumeCIRunRequest(
	ctx context.Context, binding *ciRunBinding, request *CIRunRequest,
) (*CIRunReceipt, error) {
	if request.Status == CIRunRequestSucceeded {
		return receiptFromCIRunRequest(request), nil
	}
	if request.Status == CIRunRequestFailed {
		return receiptFromCIRunRequest(request), &CIRunRequestError{Class: CIRunFailureClass(request.FailureClass)}
	}
	if request.ProviderCallStartedAt == nil {
		return receiptFromCIRunRequest(request), nil
	}
	client, err := s.resolveCIRunClient(ctx, binding.WorkspaceID, binding.Owner, binding.Repo)
	if err != nil {
		return nil, &CIRunRequestError{Class: ciRunFailureFromError(err)}
	}
	verified := &verifiedCIRun{Workflow: &GitHubActionsWorkflow{
		ID: request.ProviderWorkflowID, Name: request.ProviderWorkflowName,
		Path: request.ProviderWorkflowPath,
	}}
	return s.reconcileCIRunRequest(ctx, client, binding, request, verified)
}

func (s *Service) reconcileCIRunRequest(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	request *CIRunRequest,
	verified *verifiedCIRun,
) (*CIRunReceipt, error) {
	workflowID := request.ProviderWorkflowID
	if workflowID == 0 && verified != nil && verified.Workflow != nil {
		workflowID = verified.Workflow.ID
	}
	runs, err := client.ListActionsWorkflowRuns(ctx, binding.Owner, binding.Repo, workflowID, request.ExpectedHeadSHA)
	if err != nil {
		return s.handleCIRunMutationError(ctx, request, err)
	}
	var matched *GitHubActionsRun
	for i := range runs {
		run := &runs[i]
		if !reconciledCIRunMatches(run, workflowID, request) {
			continue
		}
		if matched != nil {
			matched = nil
			break
		}
		matched = run
	}
	if matched != nil {
		verified.Run = matched
		return s.completeCIRunSuccess(ctx, request, verified, matched.ID, matched.Attempt, request.Operation)
	}
	request.Status = CIRunRequestReconciling
	request.FailureClass = string(CIRunFailureProviderCallAmbiguous)
	request.UpdatedAt = s.ciRunClock()().UTC()
	_ = s.store.CompleteCIRunRequest(ctx, request)
	return receiptFromCIRunRequest(request), &CIRunRequestError{Class: CIRunFailureProviderCallAmbiguous}
}

func reconciledCIRunMatches(run *GitHubActionsRun, workflowID int64, request *CIRunRequest) bool {
	if run == nil {
		return false
	}
	if run.WorkflowID != workflowID || !strings.EqualFold(run.HeadSHA, request.ExpectedHeadSHA) ||
		!strings.EqualFold(run.HeadRepository, request.ProviderHeadRepo) ||
		run.HeadBranch != request.ProviderHeadRef {
		return false
	}
	if request.Operation == CIRunOperationRerunFailedJobs {
		return run.ID == request.SourceRunID && run.Attempt == request.ExpectedSourceAttempt+1
	}
	return request.Operation == CIRunOperationWorkflowDispatch &&
		request.ProviderCallStartedAt != nil && !run.CreatedAt.IsZero() &&
		!run.CreatedAt.Before(request.ProviderCallStartedAt.Truncate(time.Second)) &&
		run.ID != request.SourceRunID && run.Attempt == 1 && run.Event == "workflow_dispatch"
}

func (s *Service) completeCIRunSuccess(
	ctx context.Context,
	request *CIRunRequest,
	verified *verifiedCIRun,
	runID int64,
	attempt int,
	operation CIRunOperation,
) (*CIRunReceipt, error) {
	request.Status = CIRunRequestSucceeded
	request.Operation = operation
	request.ProviderRunID = runID
	request.ProviderAttempt = attempt
	request.ProviderWorkflowID = verified.Workflow.ID
	request.ProviderWorkflowName = verified.Workflow.Name
	request.ProviderWorkflowPath = verified.Workflow.Path
	request.ProviderHeadRepo = verified.Run.HeadRepository
	request.ProviderHeadRef = verified.Run.HeadBranch
	request.ProviderHeadSHA = verified.Run.HeadSHA
	request.FailureClass = ""
	request.UpdatedAt = s.ciRunClock()().UTC()
	if err := s.store.CompleteCIRunRequest(ctx, request); err != nil {
		return nil, err
	}
	_ = s.auditCIRun(ctx, request, "succeeded", "")
	receipt := receiptFromCIRunRequest(request)
	receipt.WorkflowName = verified.Workflow.Name
	receipt.WorkflowPath = verified.Workflow.Path
	return receipt, nil
}

func (s *Service) failCIRunRequest(
	ctx context.Context, request *CIRunRequest, class CIRunFailureClass,
) (*CIRunReceipt, error) {
	request.Status = CIRunRequestFailed
	request.FailureClass = string(class)
	request.UpdatedAt = s.ciRunClock()().UTC()
	if err := s.store.CompleteCIRunRequest(ctx, request); err != nil {
		return nil, err
	}
	_ = s.auditCIRun(ctx, request, "failed", class)
	return receiptFromCIRunRequest(request), &CIRunRequestError{Class: class}
}

func (s *Service) handleCIRunMutationError(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	class := ciRunFailureFromError(err)
	if class == CIRunFailureProviderCallAmbiguous {
		request.Status = CIRunRequestReconciling
		request.FailureClass = string(class)
		request.UpdatedAt = s.ciRunClock()().UTC()
		_ = s.store.CompleteCIRunRequest(ctx, request)
		_ = s.auditCIRun(ctx, request, "provider_ambiguous", class)
		return receiptFromCIRunRequest(request), &CIRunRequestError{Class: class}
	}
	return s.failCIRunRequest(ctx, request, class)
}

func ciRunFailureFromError(err error) CIRunFailureClass {
	var requestErr *CIRunRequestError
	if errors.As(err, &requestErr) {
		return requestErr.Class
	}
	var providerErr *CIRunProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Class
	}
	return CIRunFailureProviderUnavailable
}

func receiptFromCIRunRequest(request *CIRunRequest) *CIRunReceipt {
	return &CIRunReceipt{
		RequestID: request.ID, TaskID: request.TargetTaskID, RunID: request.ProviderRunID,
		WorkflowID: request.ProviderWorkflowID, WorkflowName: request.ProviderWorkflowName,
		WorkflowPath: request.ProviderWorkflowPath, HeadRepository: request.ProviderHeadRepo,
		HeadRef: request.ProviderHeadRef, HeadSHA: request.ProviderHeadSHA,
		Attempt: request.ProviderAttempt, Operation: request.Operation,
		EvidenceKind: request.EvidenceKind, Status: request.Status,
		FailureClass: request.FailureClass,
	}
}

func (s *Service) auditCIRun(
	ctx context.Context,
	request *CIRunRequest,
	event string,
	class CIRunFailureClass,
) error {
	details, _ := json.Marshal(map[string]any{
		"workspace_id": request.WorkspaceID, "actor_task_id": request.ActorTaskID,
		"actor_session_id": request.ActorSessionID, "target_task_id": request.TargetTaskID,
		"workflow_id": request.WorkflowID, "workflow_step_id": request.WorkflowStepID,
		"repository_id": request.RepositoryID, "pr_number": request.PRNumber,
		"expected_head_sha": request.ExpectedHeadSHA, "source_run_id": request.SourceRunID,
		"expected_source_attempt": request.ExpectedSourceAttempt, "operation": request.Operation,
		"provider_run_id": request.ProviderRunID, "provider_workflow_id": request.ProviderWorkflowID,
		"provider_head_repo": request.ProviderHeadRepo, "provider_head_ref": request.ProviderHeadRef,
		"provider_head_sha": request.ProviderHeadSHA,
	})
	return s.store.AppendCIRunAuditEvent(ctx, &CIRunAuditEvent{
		ID: uuid.NewString(), RequestID: request.ID, EventType: event,
		FailureClass: string(class), DetailsJSON: string(details), CreatedAt: s.ciRunClock()().UTC(),
	})
}
