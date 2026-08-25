package sqlite

import (
	"context"
)

// Workspace resolution for the Office HTTP authorization guard.
//
// Office routes are keyed by a resource id (`/agents/:id`, `/tasks/:id`,
// `/approvals/:id/decide`, ...) far more often than by `:wsId`, so the guard
// in internal/backendapp cannot read the owning workspace off the path. These
// resolvers are the lookup half of that guard: one per resource kind, each
// answering "which workspace owns this id".
//
// Two properties every function here must keep:
//
//  1. A missing row is an ERROR (sql.ErrNoRows), never ("", nil). The guard
//     hands the result to taskservice.AuthorizeWorkspaceAccess, whose
//     workspaceID=="" branch means "no workspace scoping applies" and allows
//     everything. Returning "" for an unknown id would therefore turn a
//     guessed id into an unconditional allow — the exact trap documented on
//     backendapp's runSubscriptionCheck.
//
//     This is why these deliberately do NOT reuse the pre-existing
//     GetTaskWorkspaceID / GetProjectWorkspaceID in tasks.go: those return
//     ("", nil) for a missing row by design, for dashboard consistency checks
//     that want a soft answer. Feeding one of those to the guard fails open.
//
//  2. No deleted_at filtering. GetRunWorkspaceID documents the reason: a
//     soft-deleted row that cannot be resolved cannot be authorized either,
//     so its own owner would be permanently denied.

// scopeWorkspaceID runs a single-column workspace lookup and returns the
// driver's sql.ErrNoRows unchanged when the id names nothing.
func (r *Repository) scopeWorkspaceID(ctx context.Context, query, id string) (string, error) {
	var workspaceID string
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), id).Scan(&workspaceID); err != nil {
		return "", err
	}
	return workspaceID, nil
}

// WorkspaceIDForAgent resolves an Office agent (agent_profiles row) to its
// workspace.
func (r *Repository) WorkspaceIDForAgent(ctx context.Context, agentID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM agent_profiles WHERE id = ?`, agentID)
}

// WorkspaceIDForTask resolves an Office task to its workspace. Office tasks
// are ordinary rows in the shared `tasks` table, whose workspace_id is
// nullable, so NULL is normalised to "" and denied by the caller.
func (r *Repository) WorkspaceIDForTask(ctx context.Context, taskID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT COALESCE(workspace_id, '') FROM tasks WHERE id = ?`, taskID)
}

// WorkspaceIDForRoutine resolves a routine to its workspace.
func (r *Repository) WorkspaceIDForRoutine(ctx context.Context, routineID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_routines WHERE id = ?`, routineID)
}

// WorkspaceIDForRoutineTrigger resolves a trigger via its owning routine.
func (r *Repository) WorkspaceIDForRoutineTrigger(ctx context.Context, triggerID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `
		SELECT o.workspace_id FROM office_routine_triggers t
		JOIN office_routines o ON o.id = t.routine_id
		WHERE t.id = ?`, triggerID)
}

// WorkspaceIDForRoutineTriggerPublicID resolves a webhook trigger by the
// public id its fire URL carries, rather than by primary key.
func (r *Repository) WorkspaceIDForRoutineTriggerPublicID(ctx context.Context, publicID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `
		SELECT o.workspace_id FROM office_routine_triggers t
		JOIN office_routines o ON o.id = t.routine_id
		WHERE t.public_id = ?`, publicID)
}

// WorkspaceIDForProject resolves a project to its workspace.
func (r *Repository) WorkspaceIDForProject(ctx context.Context, projectID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_projects WHERE id = ?`, projectID)
}

// WorkspaceIDForSkill resolves a skill to its workspace.
func (r *Repository) WorkspaceIDForSkill(ctx context.Context, skillID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_skills WHERE id = ?`, skillID)
}

// WorkspaceIDForBudget resolves a budget policy to its workspace.
func (r *Repository) WorkspaceIDForBudget(ctx context.Context, budgetID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_budget_policies WHERE id = ?`, budgetID)
}

// WorkspaceIDForApproval resolves an approval to its workspace.
func (r *Repository) WorkspaceIDForApproval(ctx context.Context, approvalID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_approvals WHERE id = ?`, approvalID)
}

// WorkspaceIDForLabel resolves a label to its workspace. The label handlers
// update and delete by label id alone, ignoring the `:wsId` on their own
// route, so this is the only thing standing between a caller's own workspace
// id and another workspace's label.
func (r *Repository) WorkspaceIDForLabel(ctx context.Context, labelID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_labels WHERE id = ?`, labelID)
}

// WorkspaceIDForChannel resolves an agent communication channel to its
// workspace.
func (r *Repository) WorkspaceIDForChannel(ctx context.Context, channelID string) (string, error) {
	return r.scopeWorkspaceID(ctx, `SELECT workspace_id FROM office_channels WHERE id = ?`, channelID)
}
