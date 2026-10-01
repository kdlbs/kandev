package scope

import (
	"context"
	"fmt"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// Principal is the server-derived identity of an in-session MCP caller.
// Request payloads must never be used to construct or replace these fields.
// Automation handlers use the principal as their workspace and self-target
// boundary in addition to the normal owner identity attached to the context.
type Principal struct {
	AutomationID      string
	WorkspaceID       string
	CallerTaskID      string
	CallerSessionID   string
	Surface           mcpprofile.Surface
	AgentProjectID    string
	AgentProjectTier  string
	ProjectMainTaskID string
}

func (p Principal) IsAutomation() bool {
	return p.AutomationID != "" && p.Surface == mcpprofile.SurfaceAutomation
}

func (p Principal) IsProjectCoordinator() bool {
	return p.AgentProjectID != "" && p.AgentProjectTier == models.AgentProjectTierCoordinator && p.Surface == mcpprofile.SurfaceProjectCoordinator
}

func (p Principal) IsProjectWorker() bool {
	return p.AgentProjectID != "" && (p.AgentProjectTier == models.AgentProjectTierEconomy || p.AgentProjectTier == models.AgentProjectTierFrontier) && p.Surface == mcpprofile.SurfaceProjectWorker
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

	automationID, surface, err := principalSurface(task)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: %w", taskID, err)
	}
	mainTaskID := ""
	if task.AgentProjectID != "" {
		mainTaskID = task.ID
		if task.AgentProjectTier != models.AgentProjectTierCoordinator {
			mainTaskID = task.ParentID
		}
	}
	return WithPrincipal(ctx, Principal{
		AutomationID:      automationID,
		WorkspaceID:       workspaceID,
		CallerTaskID:      taskID,
		CallerSessionID:   sessionID,
		Surface:           surface,
		AgentProjectID:    task.AgentProjectID,
		AgentProjectTier:  task.AgentProjectTier,
		ProjectMainTaskID: mainTaskID,
	}), nil
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
	lookup, ok := r.tasks.(interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return nil
	}
	session, err := lookup.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("resolve MCP principal session %s: %w", sessionID, err)
	}
	if session == nil || session.TaskID != taskID {
		return fmt.Errorf("resolve MCP principal: session %s does not belong to task %s", sessionID, taskID)
	}
	return nil
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
	if task.AgentProjectID != "" {
		switch task.AgentProjectTier {
		case models.AgentProjectTierCoordinator:
			if task.ParentID != "" {
				return "", mcpprofile.SurfaceKanbanTask, fmt.Errorf("project coordinator has a parent task")
			}
			return "", mcpprofile.SurfaceProjectCoordinator, nil
		case models.AgentProjectTierEconomy, models.AgentProjectTierFrontier:
			if task.ParentID == "" {
				return "", mcpprofile.SurfaceKanbanTask, fmt.Errorf("project worker has no coordinator parent")
			}
			return "", mcpprofile.SurfaceProjectWorker, nil
		default:
			return "", mcpprofile.SurfaceKanbanTask, fmt.Errorf("project task tier is invalid")
		}
	}
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
