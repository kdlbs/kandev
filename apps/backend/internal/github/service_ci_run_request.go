package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

const (
	ciRunExecutionLease        = 30 * time.Second
	ciRunDefaultRateLimitDelay = time.Hour
)

func (s *Service) RequestFreshCIRun(
	ctx context.Context,
	input RequestFreshCIRunInput,
) (*CIRunReceipt, error) {
	if failure := validateFreshCIRunInput(input); failure != "" {
		return nil, &CIRunRequestError{Class: failure}
	}
	digest := sha256.Sum256([]byte(input.IdempotencyKey))
	if existing, lookupErr := s.store.GetCIRunRequestByCallerKey(ctx, input.ActorTaskID, input.ActorSessionID, hex.EncodeToString(digest[:])); lookupErr != nil {
		return nil, lookupErr
	} else if existing != nil && (existing.ProviderCallStartedAt != nil || existing.Status == CIRunRequestSucceeded || existing.Status == CIRunRequestFailed) {
		// Once GitHub may have accepted the request, recovery is read-only and must
		// remain possible even if the grant, lane, or PR was subsequently changed.
		owner, repo, repoErr := s.loadCIRunRepository(ctx, existing.WorkspaceID, existing.TargetTaskID, existing.RepositoryID)
		if repoErr != nil {
			return s.reloadCIRunResult(ctx, existing)
		}
		return s.resumeCIRunRequest(ctx, &ciRunBinding{WorkspaceID: existing.WorkspaceID, Owner: owner, Repo: repo}, existing)
	}
	binding, err := s.loadCIRunBinding(ctx, input)
	if err != nil {
		return nil, err
	}
	request := newCIRunRequest(binding, input, s.ciRunClock()())
	claimed, created, err := s.store.ClaimCIRunRequest(ctx, request)
	if err != nil {
		if errors.Is(err, ErrCIRunSemanticConflict) && claimed != nil {
			return s.continueCIRunRequest(ctx, binding, claimed, input)
		}
		if errors.Is(err, ErrCIRunIdempotencyConflict) {
			return nil, &CIRunRequestError{Class: CIRunFailureIdempotencyConflict}
		}
		return nil, err
	}
	if !created {
		return s.continueCIRunRequest(ctx, binding, claimed, input)
	}
	if err := s.auditCIRun(ctx, claimed, "claimed", ""); err != nil {
		return nil, err
	}
	return s.continueCIRunRequest(ctx, binding, claimed, input)
}

func (s *Service) continueCIRunRequest(
	ctx context.Context,
	binding *ciRunBinding,
	request *CIRunRequest,
	input RequestFreshCIRunInput,
) (*CIRunReceipt, error) {
	if request.Status == CIRunRequestSucceeded || request.Status == CIRunRequestFailed ||
		request.ProviderCallStartedAt != nil {
		return s.resumeCIRunRequest(ctx, binding, request)
	}
	now := s.ciRunClock()().UTC()
	if request.ProviderRetryAfter != nil && now.Before(*request.ProviderRetryAfter) {
		return receiptFromCIRunRequest(request), &CIRunRequestError{Class: CIRunFailureProviderRateLimited}
	}
	acquired, err := s.store.AcquireCIRunExecution(
		ctx, request, uuid.NewString(), now, ciRunExecutionLease,
	)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return receiptFromCIRunRequest(request), nil
	}
	if input.EvidenceKind == CIRunEvidenceCurrentMerge {
		return s.failCIRunRequest(ctx, request, CIRunFailureMergeEvidenceUnavailable)
	}
	client, err := s.resolveCIRunClientForPurpose(ctx, binding.WorkspaceID, binding.Owner, binding.Repo, request.Operation)
	if err != nil {
		return s.handleCIRunPreflightError(ctx, request, err)
	}
	verified, err := verifyCIRunProviderBinding(ctx, client, binding, input)
	if err != nil {
		return s.handleCIRunPreflightError(ctx, request, err)
	}
	if request.Operation == CIRunOperationWorkflowDispatch {
		return s.executeCIRunDispatchFallback(ctx, client, binding, request, verified)
	}
	return s.executeCIRunRequest(ctx, client, request, verified)
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
	grant, err := s.store.GetAuthorizedCIRunGrant(ctx, input.ActorTaskID, input.ActorSessionID,
		input.TargetTaskID, input.RepositoryID)
	if err != nil || grant == nil {
		return nil, &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	if grant.WorkflowStepID != input.ExpectedWorkflowStepID {
		return nil, &CIRunRequestError{Class: CIRunFailureWorkflowStepMismatch}
	}
	var target taskIdentity
	if err := s.store.ro.GetContext(ctx, &target, s.store.ro.Rebind(
		`SELECT workspace_id, workflow_id, workflow_step_id FROM tasks WHERE id = ?`), input.TargetTaskID); err != nil {
		return nil, &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	if target.WorkspaceID == "" || target.WorkspaceID != grant.WorkspaceID ||
		target.WorkflowID != grant.WorkflowID {
		return nil, &CIRunRequestError{Class: CIRunFailureNotAuthorized}
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
	return s.resolveCIRunClientForPurpose(ctx, workspaceID, owner, repo, CIRunOperationRerunFailedJobs)
}

func (s *Service) resolveCIRunClientForPurpose(
	ctx context.Context, workspaceID, owner, repo string, operation CIRunOperation,
) (ciRunActionsClient, error) {
	if s.ciRunClientResolver != nil {
		return s.ciRunClientResolver(ctx, workspaceID, owner, repo)
	}
	if s.resolver == nil {
		return nil, &CIRunProviderError{Class: CIRunFailureInstallationPermission}
	}
	purpose := CredentialPurposeScopedActionsRerun
	if operation == CIRunOperationWorkflowDispatch {
		purpose = CredentialPurposeScopedActionsDispatch
	}
	resolved, err := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID, Purpose: purpose,
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
	if len(run.PullRequests) > 0 {
		return false
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
	verified, err = revalidateCIRunProviderSource(
		ctx, client, latestBinding, request, latestPR, verified.Workflow,
	)
	if err != nil {
		return s.handleCIRunPreflightError(ctx, request, err)
	}
	now := s.ciRunClock()().UTC()
	request.Operation = CIRunOperationRerunFailedJobs
	request.ProviderWorkflowID = verified.Workflow.ID
	request.ProviderWorkflowName = verified.Workflow.Name
	request.ProviderWorkflowPath = verified.Workflow.Path
	request.ProviderHeadRepo = verified.Run.HeadRepository
	request.ProviderHeadRef = verified.Run.HeadBranch
	request.ProviderHeadSHA = verified.Run.HeadSHA
	if err := s.store.MarkCIRunProviderCallStarted(ctx, request, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.handleCIRunProviderStartConflict(ctx, client, request)
		}
		return nil, err
	}
	request.ProviderCallStartedAt = &now
	request.Status = CIRunRequestReconciling
	if err := s.auditCIRun(ctx, request, "provider_started", ""); err != nil {
		return receiptFromCIRunRequest(request), err
	}
	err = client.RerunFailedActionsJobs(ctx, latestBinding.Owner, latestBinding.Repo, request.SourceRunID)
	if err == nil {
		return s.reconcileCIRunRequest(ctx, client, latestBinding, request, verified)
	}
	if ciRunFailureFromError(err) != CIRunFailureRerunIneligible {
		return s.handleCIRunMutationError(ctx, request, err)
	}
	if err := s.store.PrepareCIRunDispatchFallback(ctx, request, s.ciRunClock()().UTC()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.reloadCIRunResult(ctx, request)
		}
		return nil, err
	}
	if err := s.auditCIRun(ctx, request, "rerun_ineligible", CIRunFailureRerunIneligible); err != nil {
		return nil, err
	}
	return s.continueCIRunRequest(ctx, latestBinding, request, inputFromCIRunRequest(request))
}

func revalidateCIRunProviderSource(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	request *CIRunRequest,
	pr *PR,
	expectedWorkflow *GitHubActionsWorkflow,
) (*verifiedCIRun, error) {
	input := inputFromCIRunRequest(request)
	if expectedWorkflow == nil || expectedWorkflow.ID <= 0 {
		return nil, &CIRunRequestError{Class: CIRunFailureSourceRunMismatch}
	}
	workflow, err := client.GetActionsWorkflow(
		ctx, binding.Owner, binding.Repo, expectedWorkflow.ID,
	)
	if err != nil {
		return nil, err
	}
	if workflow == nil || workflow.ID != expectedWorkflow.ID ||
		workflow.State != string(AppRegistrationStatusActive) || workflow.Path == "" ||
		workflow.Path != expectedWorkflow.Path {
		return nil, &CIRunRequestError{Class: CIRunFailureSourceRunMismatch}
	}
	// Keep the source run as the final provider read before persisting and sending
	// the rerun so a newly advanced attempt cannot reuse stale admission evidence.
	run, err := client.GetActionsRun(ctx, binding.Owner, binding.Repo, request.SourceRunID)
	if err != nil {
		return nil, err
	}
	if !sourceRunMatches(run, pr, binding, input) || workflow.ID != run.WorkflowID ||
		(run.WorkflowPath != "" && workflow.Path != run.WorkflowPath) {
		return nil, &CIRunRequestError{Class: CIRunFailureSourceRunMismatch}
	}
	return &verifiedCIRun{PR: pr, Run: run, Workflow: workflow}, nil
}

func inputFromCIRunRequest(request *CIRunRequest) RequestFreshCIRunInput {
	return RequestFreshCIRunInput{
		ActorTaskID: request.ActorTaskID, ActorSessionID: request.ActorSessionID,
		TargetTaskID: request.TargetTaskID, RepositoryID: request.RepositoryID,
		PRNumber: request.PRNumber, ExpectedHeadSHA: request.ExpectedHeadSHA,
		ExpectedWorkflowStepID: request.WorkflowStepID, SourceRunID: request.SourceRunID,
		ExpectedSourceAttempt: request.ExpectedSourceAttempt, EvidenceKind: request.EvidenceKind,
		IdempotencyKey: "recovery-only",
	}
}

//nolint:cyclop // ordered trust gates must remain visible
func (s *Service) executeCIRunDispatchFallback(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	request *CIRunRequest,
	verified *verifiedCIRun,
) (*CIRunReceipt, error) {
	if s.resolver != nil {
		dispatchClient, resolveErr := s.resolveCIRunClientForPurpose(ctx, binding.WorkspaceID, binding.Owner, binding.Repo, CIRunOperationWorkflowDispatch)
		if resolveErr != nil {
			return s.failCIRunRequest(ctx, request, ciRunFailureFromError(resolveErr))
		}
		client = dispatchClient
	}
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
	baseline, err := client.ListActionsWorkflowRuns(ctx, latestBinding.Owner, latestBinding.Repo,
		verified.Workflow.ID, request.ExpectedHeadSHA)
	if err != nil {
		return s.handleCIRunPreflightError(ctx, request, err)
	}
	request.ProviderRunWatermark = maxCIRunID(baseline)
	// Re-read the PR after all workflow and watermark reads. Dispatch accepts
	// only a mutable branch ref, so no earlier head check is sufficient.
	latestBinding, latestPR, err = s.revalidateCIRunAdmission(ctx, client, request)
	if err != nil || latestPR.HeadBranch != verified.PR.HeadBranch {
		if err != nil {
			return s.failCIRunAdmission(ctx, request, err)
		}
		return s.failCIRunRequest(ctx, request, CIRunFailureHeadDrift)
	}
	dispatchStartedAt := s.ciRunClock()().UTC()
	if err := s.store.MarkCIRunProviderCallStarted(ctx, request, dispatchStartedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.handleCIRunProviderStartConflict(ctx, client, request)
		}
		return nil, err
	}
	request.ProviderCallStartedAt = &dispatchStartedAt
	if err := s.auditCIRun(ctx, request, "provider_started", ""); err != nil {
		return receiptFromCIRunRequest(request), err
	}
	if err := client.DispatchActionsWorkflow(ctx, latestBinding.Owner, latestBinding.Repo,
		verified.Workflow.ID, latestPR.HeadBranch, inputs); err != nil {
		return s.handleCIRunMutationError(ctx, request, err)
	}
	return s.reconcileCIRunRequest(ctx, client, latestBinding, request, verified)
}

func maxCIRunID(runs []GitHubActionsRun) int64 {
	var maximum int64
	for i := range runs {
		if runs[i].ID > maximum {
			maximum = runs[i].ID
		}
	}
	return maximum
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

func (s *Service) handleCIRunProviderStartConflict(
	ctx context.Context,
	client ciRunActionsClient,
	request *CIRunRequest,
) (*CIRunReceipt, error) {
	if _, _, err := s.revalidateCIRunAdmission(ctx, client, request); err != nil {
		return s.failCIRunAdmission(ctx, request, err)
	}
	return s.reloadCIRunResult(ctx, request)
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

const reviewedDispatchWorkflow = ".github/workflows/e2e-tests.yml"

func reviewedWorkflowDispatchInputs(path string) (map[string]string, bool) {
	if path != reviewedDispatchWorkflow {
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
		return receiptFromCIRunRequest(request), &CIRunRequestError{Class: ciRunFailureFromError(err)}
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
	if err := s.store.CompleteCIRunRequest(ctx, request); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.reloadCIRunResult(ctx, request)
		}
		return nil, err
	}
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
		run.ID > request.ProviderRunWatermark &&
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
		if errors.Is(err, sql.ErrNoRows) {
			return s.reloadCIRunResult(ctx, request)
		}
		return nil, err
	}
	receipt := receiptFromCIRunRequest(request)
	receipt.WorkflowName = verified.Workflow.Name
	receipt.WorkflowPath = verified.Workflow.Path
	if err := s.auditCIRun(ctx, request, "succeeded", ""); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (s *Service) failCIRunRequest(
	ctx context.Context, request *CIRunRequest, class CIRunFailureClass,
) (*CIRunReceipt, error) {
	request.Status = CIRunRequestFailed
	request.FailureClass = string(class)
	request.UpdatedAt = s.ciRunClock()().UTC()
	if err := s.store.CompleteCIRunRequest(ctx, request); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.reloadCIRunResult(ctx, request)
		}
		return nil, err
	}
	if err := s.auditCIRun(ctx, request, "failed", class); err != nil {
		return receiptFromCIRunRequest(request), err
	}
	return receiptFromCIRunRequest(request), &CIRunRequestError{Class: class}
}

func (s *Service) handleCIRunMutationError(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	class := ciRunFailureFromError(err)
	if class == CIRunFailureProviderRateLimited {
		return s.deferCIRunForRateLimit(ctx, request, err)
	}
	if class == CIRunFailureProviderCallAmbiguous {
		request.Status = CIRunRequestReconciling
		request.FailureClass = string(class)
		request.UpdatedAt = s.ciRunClock()().UTC()
		if err := s.store.CompleteCIRunRequest(ctx, request); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return s.reloadCIRunResult(ctx, request)
			}
			return nil, err
		}
		if err := s.auditCIRun(ctx, request, "provider_ambiguous", class); err != nil {
			return nil, err
		}
		return receiptFromCIRunRequest(request), &CIRunRequestError{Class: class}
	}
	return s.failCIRunRequest(ctx, request, class)
}

func (s *Service) reloadCIRunResult(
	ctx context.Context, request *CIRunRequest,
) (*CIRunReceipt, error) {
	loaded, err := s.store.GetCIRunRequest(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	if loaded.Status == CIRunRequestFailed {
		return receiptFromCIRunRequest(loaded), &CIRunRequestError{
			Class: CIRunFailureClass(loaded.FailureClass),
		}
	}
	if loaded.Status == CIRunRequestReconciling {
		return receiptFromCIRunRequest(loaded), &CIRunRequestError{
			Class: CIRunFailureProviderCallAmbiguous,
		}
	}
	return receiptFromCIRunRequest(loaded), nil
}

func (s *Service) handleCIRunPreflightError(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	if ciRunFailureFromError(err) == CIRunFailureProviderRateLimited {
		return s.deferCIRunForRateLimit(ctx, request, err)
	}
	return s.failCIRunRequest(ctx, request, ciRunFailureFromError(err))
}

func (s *Service) deferCIRunForRateLimit(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	now := s.ciRunClock()().UTC()
	retryAfter := now.Add(ciRunDefaultRateLimitDelay)
	var providerErr *CIRunProviderError
	if errors.As(err, &providerErr) && providerErr.RetryAfter != nil && providerErr.RetryAfter.After(now) {
		retryAfter = providerErr.RetryAfter.UTC()
	}
	if err := s.store.DeferCIRunForRateLimit(ctx, request, retryAfter, now); err != nil {
		return nil, err
	}
	if err := s.auditCIRun(ctx, request, "provider_rate_limited", CIRunFailureProviderRateLimited); err != nil {
		return nil, err
	}
	return receiptFromCIRunRequest(request), &CIRunRequestError{Class: CIRunFailureProviderRateLimited}
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
