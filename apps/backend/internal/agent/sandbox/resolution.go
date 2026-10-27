package sandbox

import (
	"context"
	"slices"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) Resolve(ctx context.Context, selection SelectionContext) (*Resolved, error) {
	if selection.ExecutorID == "" {
		return nil, ErrIneligible
	}
	bindings, err := s.readBindings(ctx, selection)
	if err != nil {
		return nil, err
	}
	binding := bindings[ScopeExecutor]
	executor := binding.Executor
	ref, source := selectedReference(bindings)
	if executor == nil || executor.Mode == ModeDisabled {
		if ref != "" || selection.Prior != nil {
			return nil, ErrIneligible
		}
		return nil, nil
	}
	if !s.enabled {
		return nil, ErrDisabled
	}
	if executor.Mode != ModeRequired || !slices.Contains(executor.EligibleRevisionIDs, ref) {
		return nil, ErrIneligible
	}
	if err = validateBindingWorkspaces(bindings, selection.WorkspaceID); err != nil {
		return nil, err
	}
	revision, ceiling, err := s.resolveRevisions(ctx, selection, executor, ref)
	if err != nil {
		return nil, err
	}
	if err = s.validateResolvedGrants(revision, ceiling); err != nil {
		return nil, err
	}
	if err = s.checkPrior(ctx, selection, revision); err != nil {
		return nil, err
	}
	return &Resolved{Bindings: captureBindings(selection, bindings), Revision: revision, Ceiling: ceiling, SelectionSource: source, ExecutorID: selection.ExecutorID, BindingVersion: binding.Version}, nil
}

func selectedReference(bindings map[string]Binding) (string, string) {
	for _, scope := range []string{ScopeTask, ScopeWorkflowStep, ScopeAgent} {
		if binding := bindings[scope]; binding.RevisionID != "" {
			return binding.RevisionID, scope
		}
	}
	if executor := bindings[ScopeExecutor].Executor; executor != nil {
		return executor.DefaultRevisionID, ScopeExecutor
	}
	return "", ""
}

func validateBindingWorkspaces(bindings map[string]Binding, workspaceID string) error {
	for _, binding := range bindings {
		if binding.WorkspaceID != "" && binding.WorkspaceID != workspaceID {
			return ErrIneligible
		}
	}
	return nil
}

func (s *Service) checkPrior(ctx context.Context, selection SelectionContext, revision *Revision) error {
	if selection.Prior == nil {
		return nil
	}
	prior := selection.Prior.Revision
	if prior == nil {
		return ErrIneligible
	}
	sameProvider := prior.Policy.InstallationID == revision.Policy.InstallationID && prior.Policy.ProviderKey == revision.Policy.ProviderKey
	if sameProvider && contract.WithinCeiling(revision.Policy.Grants, prior.Policy.Grants) == nil {
		return nil
	}
	if _, err := s.operator(ctx); err == nil {
		return nil
	}
	return ErrIneligible
}

func (s *Service) validateResolvedGrants(revision, ceiling *Revision) error {
	canonical, err := contract.Canonicalize(revision.Policy.Grants, s.protectedRoots)
	if err != nil {
		return err
	}
	ceilingGrants, err := contract.Canonicalize(ceiling.Policy.Grants, s.protectedRoots)
	if err != nil {
		return err
	}
	if err = contract.WithinCeiling(canonical, ceilingGrants); err != nil {
		return err
	}
	return nil
}

func (s *Service) resolveRevisions(ctx context.Context, selection SelectionContext, executor *ExecutorPolicy, ref string) (*Revision, *Revision, error) {
	revision, err := s.readRevision(ctx, selection.OrgID, ref)
	if err != nil {
		return nil, nil, err
	}
	ceiling, err := s.readRevision(ctx, selection.OrgID, executor.CeilingRevisionID)
	if err != nil {
		return nil, nil, err
	}
	if !matchesExecutor(revision, executor, selection.WorkspaceID) || !matchesExecutor(ceiling, executor, selection.WorkspaceID) {
		return nil, nil, ErrIneligible
	}
	return revision, ceiling, nil
}

func captureBindings(selection SelectionContext, bindings map[string]Binding) []CapturedBinding {
	result := make([]CapturedBinding, 0, 4)
	for _, item := range []CapturedBinding{{Scope: ScopeExecutor, TargetID: selection.ExecutorID}, {Scope: ScopeAgent, TargetID: selection.AgentProfileID}, {Scope: ScopeWorkflowStep, TargetID: selection.WorkflowStepID}, {Scope: ScopeTask, TargetID: selection.TaskID}} {
		if item.TargetID != "" {
			item.Version = bindings[item.Scope].Version
			result = append(result, item)
		}
	}
	return result
}
