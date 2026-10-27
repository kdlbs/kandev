package sandbox

import (
	"context"
	"encoding/json"
	"slices"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) PutBinding(ctx context.Context, binding Binding, expected int64) (*Binding, error) {
	id, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	if expected < 0 || binding.TargetID == "" || len(binding.TargetID) > 128 || !slices.Contains([]string{ScopeExecutor, ScopeAgent, ScopeWorkflowStep, ScopeTask}, binding.Scope) {
		return nil, ErrIneligible
	}
	if err = s.authorizeBindingTarget(ctx, binding, id.OrgID); err != nil {
		return nil, err
	}
	if err = s.validateBinding(ctx, binding); err != nil {
		return nil, err
	}
	binding.Version = expected + 1
	raw, err := json.Marshal(binding)
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO sandbox_policy_bindings (org_id,scope,target_id,version,content_json)
	SELECT ?,?,?,?,? WHERE ?=0
	ON CONFLICT (org_id,scope,target_id) DO NOTHING`), id.OrgID, binding.Scope, binding.TargetID, binding.Version, string(raw), expected)
	if err != nil {
		return nil, err
	}
	if expected > 0 {
		result, err = s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_policy_bindings SET version=?,content_json=? WHERE org_id=? AND scope=? AND target_id=? AND version=?`), binding.Version, string(raw), id.OrgID, binding.Scope, binding.TargetID, expected)
		if err != nil {
			return nil, err
		}
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, ErrConflict
	}
	return &binding, nil
}

func (s *Service) validateBinding(ctx context.Context, binding Binding) error {
	if binding.Scope != ScopeExecutor {
		if binding.Executor != nil {
			return ErrIneligible
		}
		if binding.RevisionID == "" {
			return nil
		}
		revision, err := s.GetRevision(ctx, binding.RevisionID)
		if err != nil {
			return err
		}
		if !eligibleWorkspace(revision, binding.WorkspaceID) || revision.Revoked {
			return ErrIneligible
		}
		return nil
	}
	if binding.RevisionID != "" || binding.Executor == nil {
		return ErrIneligible
	}
	if err := validateExecutorPolicy(binding.Executor); err != nil {
		return err
	}
	if binding.Executor.Mode == ModeDisabled {
		return nil
	}
	return s.validateExecutorRevisions(ctx, binding)
}

func (s *Service) validateExecutorRevisions(ctx context.Context, binding Binding) error {
	executor := binding.Executor
	ceiling, err := s.GetRevision(ctx, executor.CeilingRevisionID)
	if err != nil {
		return err
	}
	if !matchesExecutor(ceiling, executor, binding.WorkspaceID) {
		return ErrIneligible
	}
	for _, ref := range executor.EligibleRevisionIDs {
		revision, err := s.GetRevision(ctx, ref)
		if err != nil {
			return err
		}
		workspaceID := binding.WorkspaceID
		// Eligibility permits a workspace choice; it does not make that
		// revision a default or authorize launches in another workspace.
		if workspaceID == "" && ref != executor.DefaultRevisionID {
			workspaceID = revision.Policy.WorkspaceID
		}
		if !matchesExecutor(revision, executor, workspaceID) {
			return ErrIneligible
		}
		if err = contract.WithinCeiling(revision.Policy.Grants, ceiling.Policy.Grants); err != nil {
			return err
		}
	}
	return nil
}

func eligibleWorkspace(revision *Revision, workspaceID string) bool {
	return revision.Policy.WorkspaceID == "" || revision.Policy.WorkspaceID == workspaceID
}

func matchesExecutor(revision *Revision, executor *ExecutorPolicy, workspaceID string) bool {
	return !revision.Revoked && eligibleWorkspace(revision, workspaceID) && revision.Policy.InstallationID == executor.InstallationID && revision.Policy.ProviderKey == executor.ProviderKey
}

func (s *Service) readBindings(ctx context.Context, selection SelectionContext) (map[string]Binding, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`SELECT content_json FROM sandbox_policy_bindings WHERE org_id=? AND
	((scope=? AND target_id=?) OR (scope=? AND target_id=?) OR (scope=? AND target_id=?) OR (scope=? AND target_id=?))`), selection.OrgID, ScopeExecutor, selection.ExecutorID, ScopeAgent, selection.AgentProfileID, ScopeWorkflowStep, selection.WorkflowStepID, ScopeTask, selection.TaskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	bindings := make(map[string]Binding)
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var binding Binding
		if err = json.Unmarshal([]byte(raw), &binding); err != nil {
			return nil, err
		}
		bindings[binding.Scope] = binding
	}
	return bindings, rows.Err()
}

func (s *Service) authorizeBindingTarget(ctx context.Context, binding Binding, orgID string) error {
	if s.resolveTarget == nil {
		return ErrAuthority
	}
	target, err := s.resolveTarget(ctx, binding.Scope, binding.TargetID)
	if err != nil {
		return err
	}
	if target.OrgID != orgID || target.WorkspaceID != binding.WorkspaceID {
		return ErrAuthority
	}
	if err = s.workspaceAccess(ctx, target.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func validateExecutorPolicy(executor *ExecutorPolicy) error {
	if executor.Mode == ModeDisabled {
		if executor.InstallationID != "" || executor.ProviderKey != "" || executor.CeilingRevisionID != "" || executor.DefaultRevisionID != "" || len(executor.EligibleRevisionIDs) != 0 {
			return ErrIneligible
		}
		return nil
	}
	if executor.Mode != ModeRequired || executor.InstallationID == "" || executor.ProviderKey == "" || len(executor.EligibleRevisionIDs) > 128 || !slices.Contains(executor.EligibleRevisionIDs, executor.DefaultRevisionID) {
		return ErrIneligible
	}
	return nil
}
