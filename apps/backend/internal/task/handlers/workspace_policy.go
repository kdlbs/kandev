package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/service"
)

// Workspace mode values mirrored from the office task-handoffs phase 4
// MCP surface so the HTTP path validates payloads identically.
const (
	workspaceModeInheritParent = "inherit_parent"
	workspaceModeNewWorkspace  = "new_workspace"
	workspaceModeSharedGroup   = "shared_group"
)

// resolveWorkspacePolicy resolves the effective workspace policy for an
// HTTP create-task request, applying parent defaults when the caller
// didn't supply explicit values. Mirrors the MCP-side helper in
// internal/mcp/handlers/workspace_policy.go so both surfaces produce the
// same metadata block.
func (h *TaskHandlers) resolveWorkspacePolicy(ctx context.Context, body httpCreateTaskRequest) (service.WorkspacePolicy, error) {
	pol := service.WorkspacePolicy{
		Mode:                  body.WorkspaceMode,
		GroupID:               body.WorkspaceGroupID,
		DefaultChildWorkspace: body.DefaultChildWorkspace,
		DefaultChildOrdering:  body.DefaultChildOrdering,
	}
	h.applyParentDefaults(ctx, body.ParentID, &pol)
	if pol.Mode == "" {
		pol.Mode = workspaceModeNewWorkspace
	}
	if err := validatePolicy(&pol); err != nil {
		return pol, err
	}
	return pol, nil
}

func (h *TaskHandlers) applyParentDefaults(ctx context.Context, parentID string, pol *service.WorkspacePolicy) {
	if parentID == "" || h.service == nil {
		return
	}
	parent, err := h.service.GetTask(ctx, parentID)
	if err != nil || parent == nil {
		return
	}
	if pol.ParentOrdering == "" {
		pol.ParentOrdering = parentDefaultOrdering(parent.Metadata)
	}
	if pol.Mode == "" {
		if def := parentDefaultChildWorkspace(parent.Metadata); def != "" {
			pol.Mode = def
		}
	}
}

func parentDefaultChildWorkspace(meta map[string]interface{}) string {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := ws["default_child_workspace"].(string)
	return v
}

func parentDefaultOrdering(meta map[string]interface{}) string {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := ws["default_child_ordering"].(string)
	return v
}

func validatePolicy(pol *service.WorkspacePolicy) error {
	switch pol.Mode {
	case workspaceModeInheritParent, workspaceModeNewWorkspace, workspaceModeSharedGroup:
	default:
		return errors.New("invalid workspace_mode: " + pol.Mode + " (allowed: inherit_parent, new_workspace, shared_group)")
	}
	if pol.Mode == workspaceModeSharedGroup && pol.GroupID == "" {
		return errors.New("workspace_group_id is required when workspace_mode='shared_group'")
	}
	if pol.Mode != workspaceModeSharedGroup {
		pol.GroupID = ""
	}
	return nil
}

// mergeWorkspaceMetadata merges the workspace-policy metadata block into
// the user-supplied metadata. The workspace block always wins for the
// "workspace" key so callers can't inject conflicting policy via raw
// metadata.
func mergeWorkspaceMetadata(base map[string]interface{}, ws map[string]interface{}) map[string]interface{} {
	if len(ws) == 0 {
		return base
	}
	if base == nil {
		base = map[string]interface{}{}
	}
	for k, v := range ws {
		base[k] = v
	}
	return base
}
