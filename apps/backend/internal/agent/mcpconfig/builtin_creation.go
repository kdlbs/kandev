package mcpconfig

import (
	"context"

	"github.com/kandev/kandev/internal/authz"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

type BuiltinCreationPreferences struct {
	Version                int                   `json:"version"`
	ExpectedPolicyRevision *int64                `json:"expected_workspace_policy_revision"`
	Packages               BuiltinPackageChoices `json:"packages"`
}

func (s *BuiltinPolicyService) ValidateTaskCreation(ctx context.Context, workspaceID string, input *BuiltinCreationPreferences) error {
	if _, err := builtinHumanActor(ctx); err != nil {
		return err
	}
	if err := s.authorizeWorkspace(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return err
	}
	if err := validateBuiltinCreationInput(workspaceID, input); err != nil {
		return err
	}
	snapshot, err := s.repo.GetBuiltinPolicySnapshot(ctx, workspaceID, "")
	if err != nil {
		return err
	}
	if snapshot == nil || snapshot.Policy == nil {
		return ErrBuiltinPolicyInvalid
	}
	if snapshot.Policy.Revision != *input.ExpectedPolicyRevision {
		return &BuiltinPolicyConflictError{Current: snapshot}
	}
	for id, choice := range input.Packages {
		if choice == BuiltinPreferenceOn && snapshot.Policy.Packages[id] == BuiltinPolicyBlocked {
			return ErrBuiltinPackageBlocked
		}
	}
	return nil
}

func (s *BuiltinPolicyService) ApplyTaskCreation(ctx context.Context, taskID string, input *BuiltinCreationPreferences) error {
	if input == nil || input.ExpectedPolicyRevision == nil {
		return ErrBuiltinPolicyInvalid
	}
	_, err := s.UpdateTask(ctx, taskID, 0, *input.ExpectedPolicyRevision, input.Packages)
	return err
}

func validateBuiltinCreationInput(workspaceID string, input *BuiltinCreationPreferences) error {
	if input == nil || input.Version != 1 || input.ExpectedPolicyRevision == nil || *input.ExpectedPolicyRevision < 0 || input.Packages == nil {
		return ErrBuiltinPolicyInvalid
	}
	preferences := &BuiltinTaskPreferences{Version: 1, WorkspaceID: workspaceID, TaskID: "pending", Packages: input.Packages}
	if err := preferences.Validate(p.SurfaceKanbanTask); err != nil {
		return err
	}
	return nil
}
