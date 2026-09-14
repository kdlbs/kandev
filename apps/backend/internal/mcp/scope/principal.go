package scope

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/coordinator"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// Principal is the server-derived identity of an in-session MCP caller.
// Request payloads must never be used to construct or replace these fields.
// Automation handlers use the principal as their workspace and self-target
// boundary in addition to the normal owner identity attached to the context.
type Principal struct {
	AutomationID    string
	WorkspaceID     string
	CallerTaskID    string
	CallerSessionID string
	Surface         mcpprofile.Surface
}

func (p Principal) IsAutomation() bool {
	return p.AutomationID != "" && p.Surface == mcpprofile.SurfaceAutomation
}

type principalContextKey struct{}

// WithPrincipal attaches a trusted principal to a dispatch context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the server-derived caller principal.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok && principal.CallerTaskID != "" && principal.CallerSessionID != ""
}

// ScopePrincipal derives the MCP principal from the execution's own task and
// session. It intentionally does not read any identity or surface fields from
// an agent payload. Non-automation tasks still receive their normal surface so
// downstream code has one consistent trusted caller shape.
func (r *Resolver) ScopePrincipal(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	if r == nil || taskID == "" || sessionID == "" || r.tasks == nil {
		return nil, fmt.Errorf("resolve MCP principal: task and session are required")
	}
	task, err := r.resolvePrincipalTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := r.validatePrincipalSession(ctx, taskID, sessionID); err != nil {
		return nil, err
	}

	workspaceID, err := r.resolvePrincipalWorkspace(ctx, task)
	if err != nil {
		return nil, err
	}
	if lifecycle, ok := r.tasks.(coordinator.PrincipalLifecycleStore); ok {
		if err := r.bindTaskPrincipal(ctx, lifecycle, workspaceID, taskID, sessionID); err != nil {
			return nil, err
		}
	}

	automationID, surface, err := principalSurface(task)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: %w", taskID, err)
	}
	return WithPrincipal(ctx, Principal{
		AutomationID:    automationID,
		WorkspaceID:     workspaceID,
		CallerTaskID:    taskID,
		CallerSessionID: sessionID,
		Surface:         surface,
	}), nil
}

func (r *Resolver) bindTaskPrincipal(ctx context.Context, lifecycle coordinator.PrincipalLifecycleStore, workspaceID, taskID, sessionID string) error {
	if models.IsTerminalTaskSessionState(r.dispatchingSessionState(ctx, sessionID)) {
		return fmt.Errorf("bind MCP task principal: dispatching session is terminal")
	}
	principal, err := lifecycle.GetActiveWorkspaceAgentPrincipalForTask(ctx, workspaceID, taskID)
	if err != nil {
		return fmt.Errorf("resolve MCP task principal: %w", err)
	}
	if principal != nil && !coordinator.IsTaskPrincipal(principal, workspaceID, taskID) {
		if principal.BackingSessionID != sessionID {
			return fmt.Errorf("bind MCP task principal: task is bound to another session")
		}
		return nil
	}
	if err := r.requireDisplaceableHolder(ctx, principal, sessionID); err != nil {
		return err
	}
	if principal, err = coordinator.EnsureTaskPrincipal(ctx, lifecycle, workspaceID, taskID, sessionID); err != nil {
		return fmt.Errorf("bind MCP task principal: %w", err)
	} else if principal == nil {
		return fmt.Errorf("bind MCP task principal: principal is revoked")
	}
	return nil
}

// requireDisplaceableHolder rejects rotation while the current holder session
// is mid-turn. Rotation is the designed relief for a stale sequential
// binding, but a live holder mid-turn must not be displaced by a concurrent
// session: its dispatches are legitimate and stealing the binding mid-flight
// would let a second session revoke the first's authority underneath an
// in-progress privileged operation. A holder that is terminal, at rest
// (waiting/idle), not yet started (created), or no longer readable may be
// superseded; an unknown holder cannot dispatch, so rotation from it stays
// within the sequential contract.
func (r *Resolver) requireDisplaceableHolder(ctx context.Context, principal *models.WorkspaceAgentPrincipal, sessionID string) error {
	if principal == nil || principal.BackingSessionID == "" || principal.BackingSessionID == sessionID {
		return nil
	}
	holderState, ok, err := r.principalSessionState(ctx, principal.BackingSessionID)
	if err != nil {
		return fmt.Errorf("bind MCP task principal: %w", err)
	}
	if !ok || (holderState != models.TaskSessionStateStarting && holderState != models.TaskSessionStateRunning) {
		return nil
	}
	return fmt.Errorf("bind MCP task principal: task is bound to an active session")
}

// dispatchingSessionState reads the session that is dispatching this MCP
// request; a repository without session reads yields CREATED, the resting
// state, so the principal path keeps its pre-session-reads behavior.
func (r *Resolver) dispatchingSessionState(ctx context.Context, sessionID string) models.TaskSessionState {
	state, ok, err := r.principalSessionState(ctx, sessionID)
	if err != nil || !ok {
		return models.TaskSessionStateCreated
	}
	return state
}

func (r *Resolver) resolvePrincipalTask(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: %w", taskID, err)
	}
	if task == nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: task not found", taskID)
	}
	return task, nil
}

func (r *Resolver) validatePrincipalSession(ctx context.Context, taskID, sessionID string) error {
	session, err := r.principalSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != taskID {
		return fmt.Errorf("resolve MCP principal: session %s does not belong to task %s", sessionID, taskID)
	}
	if models.IsTerminalTaskSessionState(session.State) {
		return fmt.Errorf("resolve MCP principal: session %s is terminal", sessionID)
	}
	return nil
}

// principalSession reads the dispatching session's row through the optional
// task-repository capability. A repository without session reads reports no
// session; the caller decides whether that is acceptable for its path.
func (r *Resolver) principalSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	lookup, ok := r.tasks.(interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return nil, nil
	}
	session, err := lookup.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal session %s: %w", sessionID, err)
	}
	return session, nil
}

// principalSessionState resolves the live state of a session ID without
// requiring the caller to hold any session row itself. A session whose row is
// gone reports (unknown, false) without an error: it can no longer dispatch.
func (r *Resolver) principalSessionState(ctx context.Context, sessionID string) (models.TaskSessionState, bool, error) {
	if sessionID == "" {
		return "", false, nil
	}
	lookup, ok := r.tasks.(interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return "", false, nil
	}
	session, err := lookup.GetTaskSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) || strings.Contains(err.Error(), "not found") {
			return "", false, nil
		}
		return "", false, err
	}
	if session == nil {
		return "", false, nil
	}
	return session.State, true, nil
}

func (r *Resolver) resolvePrincipalWorkspace(ctx context.Context, task *models.Task) (string, error) {
	if task.WorkspaceID == "" {
		return "", fmt.Errorf("resolve MCP principal task %s: workspace is required", task.ID)
	}
	workspace, err := r.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("resolve MCP principal workspace %s: %w", task.WorkspaceID, err)
	}
	if workspace == nil {
		return "", fmt.Errorf("resolve MCP principal workspace %s: workspace not found", task.WorkspaceID)
	}
	return task.WorkspaceID, nil
}

func principalSurface(task *models.Task) (string, mcpprofile.Surface, error) {
	if task.Origin != models.TaskOriginAutomationRun {
		if task.IsFromOffice {
			return "", mcpprofile.SurfaceOfficeTask, nil
		}
		return "", mcpprofile.SurfaceKanbanTask, nil
	}
	automationID := models.StringFromAny(task.Metadata["automation_id"])
	if automationID == "" {
		return "", mcpprofile.SurfaceAutomation, fmt.Errorf("automation ID is missing")
	}
	return automationID, mcpprofile.SurfaceAutomation, nil
}
