package coordinator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TaskPrincipalInstallationID identifies the server-owned task principal
// namespace. It is not an agent-provided plugin identity.
const TaskPrincipalInstallationID = "kandev-task"

// TaskPrincipalLogicalKey returns the stable logical identity for a task's
// operator-grantable principal. Session bindings remain replaceable.
func TaskPrincipalLogicalKey(taskID string) string {
	return "task:" + taskID
}

// PrincipalLifecycleStore is the trusted persistence seam used when an agent
// session is admitted and when an operator grants a normal task authority.
type PrincipalLifecycleStore interface {
	GetWorkspaceAgentPrincipalByContext(ctx context.Context, workspaceID, pluginInstallationID, logicalKey string) (*models.WorkspaceAgentPrincipal, error)
	CreateWorkspaceAgentPrincipal(ctx context.Context, principal *models.WorkspaceAgentPrincipal) error
	RebindWorkspaceAgentPrincipal(ctx context.Context, id, taskID, sessionID string, updatedAt time.Time) error
}

// EnsureTaskPrincipal creates the server-owned principal for taskID when it is
// first seen, and updates only its server-authored session binding thereafter.
// A revoked principal is never resurrected implicitly; explicit operator
// re-consent must use a new identity or an explicit lifecycle operation.
func EnsureTaskPrincipal(ctx context.Context, store PrincipalLifecycleStore, workspaceID, taskID, sessionID string) (*models.WorkspaceAgentPrincipal, error) {
	if store == nil || workspaceID == "" || taskID == "" {
		return nil, fmt.Errorf("ensure task principal: workspace and task are required")
	}
	logicalKey := TaskPrincipalLogicalKey(taskID)
	principal, err := lookupOrCreateTaskPrincipal(ctx, store, workspaceID, taskID, logicalKey, sessionID)
	if err != nil {
		return nil, err
	}
	if principal == nil || principal.RevokedAt != nil {
		return nil, nil
	}
	if sessionID == "" || principal.BackingSessionID == sessionID {
		return principal, nil
	}
	if err := store.RebindWorkspaceAgentPrincipal(ctx, principal.ID, taskID, sessionID, time.Now().UTC()); err != nil {
		return nil, err
	}
	principal.BackingTaskID = taskID
	principal.BackingSessionID = sessionID
	return principal, nil
}

func lookupOrCreateTaskPrincipal(ctx context.Context, store PrincipalLifecycleStore, workspaceID, taskID, logicalKey, sessionID string) (*models.WorkspaceAgentPrincipal, error) {
	principal, err := store.GetWorkspaceAgentPrincipalByContext(ctx, workspaceID, TaskPrincipalInstallationID, logicalKey)
	if err == nil {
		return principal, nil
	}
	if !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalNotFound) {
		return nil, err
	}
	principal = &models.WorkspaceAgentPrincipal{
		WorkspaceID:          workspaceID,
		PluginInstallationID: TaskPrincipalInstallationID,
		LogicalKey:           logicalKey,
		BackingTaskID:        taskID,
		BackingSessionID:     sessionID,
	}
	if err := store.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
		if !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalConflict) {
			return nil, err
		}
		principal, err = store.GetWorkspaceAgentPrincipalByContext(ctx, workspaceID, TaskPrincipalInstallationID, logicalKey)
		if err != nil {
			return nil, err
		}
	}
	if principal == nil {
		return nil, fmt.Errorf("ensure task principal: concurrent registration returned no principal")
	}
	return principal, nil
}
