package sandbox

import (
	"context"
	"errors"
	"strings"
	"unicode"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type PreviewContext struct {
	WorkspaceID    string `json:"workspace_id"`
	ExecutorID     string `json:"executor_id"`
	AgentProfileID string `json:"agent_profile_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	TaskID         string `json:"task_id"`
}

type SelectionPreview struct {
	Resolved        *Resolved   `json:"resolved"`
	Eligible        []*Revision `json:"eligible"`
	ResolutionError string      `json:"resolution_error,omitempty"`
}

// PreviewSelection resolves saved defaults for an authorized prospective context.
// Launch admission still derives its own context from persisted task state.
func (s *Service) PreviewSelection(ctx context.Context, request PreviewContext) (*SelectionPreview, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.authorizePreview(ctx, request, identity.OrgID); err != nil {
		return nil, err
	}
	selection := SelectionContext{OrgID: identity.OrgID, WorkspaceID: request.WorkspaceID,
		ExecutorID: request.ExecutorID, AgentProfileID: request.AgentProfileID,
		WorkflowStepID: request.WorkflowStepID, TaskID: request.TaskID}
	binding, err := s.GetBinding(ctx, ScopeExecutor, request.ExecutorID)
	if err != nil {
		return nil, err
	}
	resolved, resolutionErr := s.Resolve(ctx, selection)
	if resolutionErr != nil && !previewPolicyError(resolutionErr) {
		return nil, resolutionErr
	}
	if resolved != nil && binding.Version != resolved.BindingVersion {
		return nil, ErrConflict
	}
	result := &SelectionPreview{Resolved: resolved}
	if resolutionErr != nil {
		_, result.ResolutionError = sandboxHTTPError(resolutionErr)
	}
	result.Eligible, err = s.previewEligiblePolicies(ctx, binding, request.WorkspaceID)
	return result, err
}

func previewPolicyError(err error) bool {
	return errors.Is(err, ErrIneligible) || errors.Is(err, ErrNotFound) || errors.Is(err, contract.ErrOutsideCeiling) || errors.Is(err, contract.ErrInvalidPolicy)
}

func (s *Service) previewEligiblePolicies(ctx context.Context, binding *Binding, workspaceID string) ([]*Revision, error) {
	eligible := []*Revision{}
	if binding.Executor == nil || binding.Executor.Mode != ModeRequired {
		return eligible, nil
	}
	ceiling, err := s.GetRevision(ctx, binding.Executor.CeilingRevisionID)
	if err != nil {
		return nil, err
	}
	if !matchesExecutor(ceiling, binding.Executor, workspaceID) {
		return eligible, nil
	}
	for _, ref := range binding.Executor.EligibleRevisionIDs {
		revision, err := s.GetRevision(ctx, ref)
		if err != nil {
			return nil, err
		}
		if !matchesExecutor(revision, binding.Executor, workspaceID) || s.validateResolvedGrants(revision, ceiling) != nil {
			continue
		}
		eligible = append(eligible, revision)
	}
	return eligible, nil
}

func (s *Service) authorizePreview(ctx context.Context, request PreviewContext, orgID string) error {
	if request.ExecutorID == "" || s.resolveTarget == nil {
		return ErrIneligible
	}
	for _, value := range []string{request.WorkspaceID, request.ExecutorID, request.AgentProfileID, request.WorkflowStepID, request.TaskID} {
		if len(value) > 128 || strings.ContainsFunc(value, unicode.IsControl) {
			return contract.ErrInvalidPolicy
		}
	}
	if err := s.workspaceAccess(ctx, request.WorkspaceID); err != nil {
		return ErrNotFound
	}
	for _, item := range []CapturedBinding{
		{Scope: ScopeExecutor, TargetID: request.ExecutorID}, {Scope: ScopeAgent, TargetID: request.AgentProfileID},
		{Scope: ScopeWorkflowStep, TargetID: request.WorkflowStepID}, {Scope: ScopeTask, TargetID: request.TaskID},
	} {
		if item.TargetID == "" {
			continue
		}
		target, err := s.resolveTarget(ctx, item.Scope, item.TargetID)
		if err != nil || target.OrgID != orgID || (target.WorkspaceID != "" && target.WorkspaceID != request.WorkspaceID) {
			return ErrNotFound
		}
	}
	return nil
}
