package controller

import (
	"context"
	"database/sql"
	"errors"
)

// AgentProfileBelongsToWorkspace reports whether the agent profile identified
// by profileID may be used in workspaceID: it exists, and its own
// WorkspaceID is either empty (global/kanban-legacy) or equal to
// workspaceID. This is the handoff action's AC-14b predicate for
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
