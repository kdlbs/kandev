package github

import (
	"context"
	"strings"
)

func (s *Service) resumeExistingCIRunRequest(ctx context.Context, input RequestFreshCIRunInput, hash string) (*CIRunReceipt, bool, error) {
	existing, err := s.store.GetCIRunRequestByCallerKey(ctx, input.ActorTaskID, hash)
	if err != nil || existing == nil {
		return nil, false, err
	}
	if !sameCIRunInputIdentity(existing, input) {
		return nil, true, &CIRunRequestError{Class: CIRunFailureIdempotencyConflict}
	}
	if existing.ProviderCallStartedAt == nil && existing.Status != CIRunRequestSucceeded && existing.Status != CIRunRequestFailed {
		return nil, false, nil
	}
	owner, repo, repoErr := s.loadCIRunRepository(ctx, existing.WorkspaceID, existing.TargetTaskID, existing.RepositoryID)
	if repoErr != nil {
		receipt, reloadErr := s.reloadCIRunResult(ctx, existing)
		return receipt, true, reloadErr
	}
	receipt, err := s.resumeCIRunRequest(ctx, &ciRunBinding{WorkspaceID: existing.WorkspaceID, Owner: owner, Repo: repo}, existing)
	return receipt, true, err
}

func (s *Service) ensureCIRunClaimAudit(ctx context.Context, request *CIRunRequest, created bool) error {
	if created {
		return s.auditCIRun(ctx, request, "claimed", "")
	}
	hasAudit, err := s.store.HasCIRunAuditEvent(ctx, request.ID, "claimed")
	if err != nil {
		return err
	}
	if !hasAudit {
		return s.auditCIRun(ctx, request, "claimed", "")
	}
	return nil
}

func sameCIRunInputIdentity(request *CIRunRequest, input RequestFreshCIRunInput) bool {
	return request != nil && request.ActorTaskID == input.ActorTaskID &&
		request.TargetTaskID == input.TargetTaskID && request.RepositoryID == input.RepositoryID &&
		request.PRNumber == input.PRNumber && equalCIRunSHA(request.ExpectedHeadSHA, input.ExpectedHeadSHA) &&
		request.WorkflowStepID == input.ExpectedWorkflowStepID && request.SourceRunID == input.SourceRunID &&
		request.ExpectedSourceAttempt == input.ExpectedSourceAttempt && request.EvidenceKind == input.EvidenceKind
}

func equalCIRunSHA(left, right string) bool {
	return len(left) == len(right) && len(left) == 40 &&
		(strings.EqualFold(left, right))
}
