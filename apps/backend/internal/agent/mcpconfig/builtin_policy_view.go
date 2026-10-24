package mcpconfig

import (
	"context"
	"github.com/kandev/kandev/internal/authz"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

type BuiltinManagementView struct {
	*BuiltinPolicySnapshot
	Catalog []p.BuiltinPackageDescriptor `json:"catalog"`
	Desired *p.BuiltinManifest           `json:"desired,omitempty"`
	Surface p.Surface                    `json:"surface,omitempty"`
	CanEdit bool                         `json:"can_edit"`
}

func (s *BuiltinPolicyService) ReadWorkspace(ctx context.Context, workspaceID string) (*BuiltinManagementView, error) {
	if err := s.authorizeWorkspace(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	snapshot, err := s.repo.GetBuiltinPolicySnapshot(ctx, workspaceID, "")
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.Policy == nil {
		return nil, ErrBuiltinPolicyInvalid
	}
	return &BuiltinManagementView{BuiltinPolicySnapshot: &BuiltinPolicySnapshot{Policy: snapshot.Policy}, Catalog: p.BuiltinPackageCatalog(), CanEdit: s.canEditBuiltin(ctx, workspaceID, authz.ScopeWorkspaceManage)}, nil
}

func (s *BuiltinPolicyService) ReadTask(ctx context.Context, taskID string) (*BuiltinManagementView, error) {
	if s.taskScope == nil {
		return nil, ErrBuiltinPolicyInvalid
	}
	workspaceID, surface, err := s.taskScope(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspace(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	snapshot, err := s.repo.GetBuiltinPolicySnapshot(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.Policy == nil {
		return nil, ErrBuiltinPolicyInvalid
	}
	desired, err := ResolveBuiltinPackages(workspaceID, taskID, surface, snapshot.Policy, snapshot.Preferences)
	if err != nil {
		return nil, err
	}
	editable := surface == p.SurfaceKanbanTask || surface == p.SurfaceOfficeTask
	return &BuiltinManagementView{BuiltinPolicySnapshot: snapshot, Catalog: p.BuiltinPackageCatalog(), Desired: desired, Surface: surface, CanEdit: editable && s.canEditBuiltin(ctx, workspaceID, authz.ScopeTaskWrite)}, nil
}

func (s *BuiltinPolicyService) canEditBuiltin(ctx context.Context, workspaceID string, requested authz.Scope) bool {
	_, err := builtinHumanActor(ctx)
	return err == nil && s.authorizeWorkspace(ctx, workspaceID, requested) == nil
}
