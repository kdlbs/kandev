package controller

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/office/shared"
)

// AgentHasHandoffPermission reports whether the agent profile identified by
// agentProfileID has the can_handoff_tasks permission (role default merged
// with any per-agent override), for use as the seam that grants
// handoff_task_kandev MCP access and prompt advertisement. An unknown
// profile is treated as not permitted rather than an error, since a stale
// or deleted agent profile id must fail closed, not surface an internal
// error to unrelated callers.
func (c *Controller) AgentHasHandoffPermission(ctx context.Context, agentProfileID string) (bool, error) {
	if agentProfileID == "" {
		return false, nil
	}
	profile, err := c.repo.GetAgentProfile(ctx, agentProfileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	perms := shared.ResolvePermissions(shared.AgentRole(profile.Role), profile.Permissions)
	return shared.HasPermission(perms, shared.PermCanHandoffTasks), nil
}

// AgentProfileBelongsToWorkspace reports whether the agent profile identified
// by profileID may be used in workspaceID: it exists, and its own
// WorkspaceID is either empty (global/kanban-legacy) or equal to
// workspaceID. This is handoff_task_kandev's AC-14b predicate for
// agent_profile_id, matching gitLabWatchDependencyValidator.AgentProfileBelongs
// (internal/backendapp/turn_adapters.go) in semantics only — unlike that
// precedent, a read that fails to execute is returned as an error rather than
// folded into false, so the caller can distinguish AC-12b's InternalError
// (retryable) from a genuine Validation refusal.
func (c *Controller) AgentProfileBelongsToWorkspace(ctx context.Context, profileID, workspaceID string) (bool, error) {
	if profileID == "" {
		return false, nil
	}
	profile, err := c.repo.GetAgentProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return profile.WorkspaceID == "" || profile.WorkspaceID == workspaceID, nil
}
