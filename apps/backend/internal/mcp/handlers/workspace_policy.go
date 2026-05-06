package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/task/service"
)

// Workspace mode values forwarded by the MCP create_task / delegate_task
// payloads (office task-handoffs phase 4).
const (
	workspaceModeInheritParent = "inherit_parent"
	workspaceModeNewWorkspace  = "new_workspace"
	workspaceModeSharedGroup   = "shared_group"
)

// resolveWorkspacePolicy computes the effective workspace policy for a new
// task based on (a) the explicit values on the create payload, (b) the
// parent task's default_child_* metadata when the task has a parent, and
// (c) sensible defaults. Returns a WorkspacePolicy that can both produce
// the metadata block to persist and tell the post-create hook what
// attachment work is needed (group membership, sequential blocker chain).
func (h *Handlers) resolveWorkspacePolicy(
	ctx context.Context,
	parentID, mode, groupID, defaultChildWorkspace, defaultChildOrdering string,
) (service.WorkspacePolicy, error) {
	pol := service.WorkspacePolicy{
		Mode:                  mode,
		GroupID:               groupID,
		DefaultChildWorkspace: defaultChildWorkspace,
		DefaultChildOrdering:  defaultChildOrdering,
	}
	h.applyParentDefaults(ctx, parentID, &pol)
	if pol.Mode == "" {
		pol.Mode = workspaceModeNewWorkspace
	}
	if err := validatePolicy(&pol); err != nil {
		return pol, err
	}
	return pol, nil
}

// applyParentDefaults fills in pol.Mode and pol.ParentOrdering from the
// parent task's metadata when the caller didn't supply them. Pulled out
// of resolveWorkspacePolicy to keep cyclomatic complexity in check.
func (h *Handlers) applyParentDefaults(ctx context.Context, parentID string, pol *service.WorkspacePolicy) {
	if parentID == "" || h.taskSvc == nil {
		return
	}
	parent, err := h.taskSvc.GetTask(ctx, parentID)
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

func validatePolicy(pol *service.WorkspacePolicy) error {
	if !validWorkspaceMode(pol.Mode) {
		return errInvalidWorkspaceMode(pol.Mode)
	}
	if pol.Mode == workspaceModeSharedGroup && pol.GroupID == "" {
		return errSharedGroupRequiresGroupID
	}
	if pol.Mode != workspaceModeSharedGroup {
		// Ignore a stray group_id when the caller didn't ask for shared_group.
		pol.GroupID = ""
	}
	return nil
}

// parentDefaultChildWorkspace reads metadata.workspace.default_child_workspace
// from a parent task's metadata map. Returns empty string when missing.
func parentDefaultChildWorkspace(meta map[string]interface{}) string {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := ws["default_child_workspace"].(string)
	return v
}

// parentDefaultOrdering reads metadata.workspace.default_child_ordering.
func parentDefaultOrdering(meta map[string]interface{}) string {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := ws["default_child_ordering"].(string)
	return v
}

func validWorkspaceMode(m string) bool {
	switch m {
	case workspaceModeInheritParent, workspaceModeNewWorkspace, workspaceModeSharedGroup:
		return true
	}
	return false
}

// Sentinel errors for resolveWorkspacePolicy.
var (
	errSharedGroupRequiresGroupID = &policyError{msg: "workspace_group_id is required when workspace_mode='shared_group'"}
)

func errInvalidWorkspaceMode(m string) error {
	return &policyError{msg: "invalid workspace_mode: " + m + " (allowed: inherit_parent, new_workspace, shared_group)"}
}

type policyError struct{ msg string }

func (e *policyError) Error() string { return e.msg }
