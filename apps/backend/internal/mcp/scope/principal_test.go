package scope

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type principalLookup struct {
	task      *models.Task
	workspace *models.Workspace
	session   *models.TaskSession
}

type lifecyclePrincipalLookup struct {
	principalLookup
	principal *models.WorkspaceAgentPrincipal
}

// lifecycleSessionLookup adds per-ID session rows to the lifecycle lookup so
// adversarial rotation cases can give the dispatching session and a
// concurrent holder different live states.
type lifecycleSessionLookup struct {
	lifecyclePrincipalLookup
	sessions map[string]models.TaskSession
}

func (l *lifecycleSessionLookup) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if s, ok := l.sessions[id]; ok {
		session := s
		return &session, nil
	}
	return nil, fmt.Errorf("agent session not found: %s", id)
}

func (l *lifecyclePrincipalLookup) GetActiveWorkspaceAgentPrincipalForTask(_ context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error) {
	if l.principal == nil || l.principal.RevokedAt != nil {
		return nil, nil
	}
	if l.principal.WorkspaceID != workspaceID || l.principal.BackingTaskID != taskID {
		// The real repository scopes the active lookup per workspace and task;
		// mirrors that so cross-workspace isolation cases behave identically.
		return nil, nil
	}
	return l.principal, nil
}

func (l *lifecyclePrincipalLookup) GetWorkspaceAgentPrincipalByContext(_ context.Context, workspaceID, pluginInstallationID, logicalKey string) (*models.WorkspaceAgentPrincipal, error) {
	if l.principal == nil {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	if l.principal.WorkspaceID != workspaceID ||
		l.principal.PluginInstallationID != pluginInstallationID ||
		l.principal.LogicalKey != logicalKey {
		// The real repository keys this lookup by the full context tuple;
		// mirror that so cross-workspace isolation matches production.
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	return l.principal, nil
}

func (l *lifecyclePrincipalLookup) CreateWorkspaceAgentPrincipal(_ context.Context, principal *models.WorkspaceAgentPrincipal) error {
	principal.ID = "principal-1"
	l.principal = principal
	return nil
}

func (l *lifecyclePrincipalLookup) RebindWorkspaceAgentPrincipal(_ context.Context, id, taskID, sessionID string, _ time.Time) error {
	if l.principal == nil || l.principal.ID != id {
		return repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	l.principal.BackingTaskID = taskID
	l.principal.BackingSessionID = sessionID
	return nil
}

func (l principalLookup) GetTask(context.Context, string) (*models.Task, error) {
	return l.task, nil
}

func (l principalLookup) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return l.workspace, nil
}

func (l principalLookup) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return l.session, nil
}

func TestScopePrincipalDerivesAutomationIdentityFromExecution(t *testing.T) {
	resolver := &Resolver{tasks: principalLookup{
		task: &models.Task{
			ID:          "automation-task",
			WorkspaceID: "workspace-1",
			Origin:      models.TaskOriginAutomationRun,
			Metadata:    map[string]interface{}{"automation_id": "automation-1"},
		},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "automation-task"},
	}}

	ctx, err := resolver.ScopePrincipal(context.Background(), "automation-task", "session-1")
	require.NoError(t, err)

	principal, ok := PrincipalFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, Principal{
		AutomationID:    "automation-1",
		WorkspaceID:     "workspace-1",
		CallerTaskID:    "automation-task",
		CallerSessionID: "session-1",
		Surface:         mcpprofile.SurfaceAutomation,
	}, principal)
	require.True(t, principal.IsAutomation())
}

func TestScopePrincipalRejectsSessionFromAnotherTask(t *testing.T) {
	resolver := &Resolver{tasks: principalLookup{
		task:      &models.Task{ID: "automation-task", WorkspaceID: "workspace-1"},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "other-task"},
	}}

	_, err := resolver.ScopePrincipal(context.Background(), "automation-task", "session-1")
	require.Error(t, err)
}

func TestScopePrincipalRegistersAndBindsNormalTask(t *testing.T) {
	lookup := &lifecyclePrincipalLookup{principalLookup: principalLookup{
		task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "task-1"},
	}}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	require.NotNil(t, lookup.principal)
	require.Equal(t, coordinator.TaskPrincipalInstallationID, lookup.principal.PluginInstallationID)
	require.Equal(t, coordinator.TaskPrincipalLogicalKey("task-1"), lookup.principal.LogicalKey)
	require.Equal(t, "task-1", lookup.principal.BackingTaskID)
	require.Equal(t, "session-1", lookup.principal.BackingSessionID)
}

func TestScopePrincipalDoesNotResurrectRevokedTaskPrincipal(t *testing.T) {
	revokedAt := time.Now().UTC()
	lookup := &lifecyclePrincipalLookup{
		principalLookup: principalLookup{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1"},
			session:   &models.TaskSession{ID: "session-1", TaskID: "task-1"},
		},
		principal: &models.WorkspaceAgentPrincipal{
			ID: "principal-1", WorkspaceID: "workspace-1",
			PluginInstallationID: coordinator.TaskPrincipalInstallationID,
			LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
			BackingTaskID:        "task-1", RevokedAt: &revokedAt,
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.Error(t, err)
	require.True(t, lookup.principal.RevokedAt.Equal(revokedAt))
}

func TestScopePrincipalPreservesExistingCustomTaskPrincipal(t *testing.T) {
	lookup := &lifecyclePrincipalLookup{
		principalLookup: principalLookup{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1"},
			session:   &models.TaskSession{ID: "session-1", TaskID: "task-1"},
		},
		principal: &models.WorkspaceAgentPrincipal{
			ID:                   "custom-principal",
			WorkspaceID:          "workspace-1",
			PluginInstallationID: "plugin-1",
			LogicalKey:           "custom-key",
			BackingTaskID:        "task-1",
			BackingSessionID:     "session-1",
		},
	}
	resolver := &Resolver{tasks: lookup}

	ctx, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	require.Equal(t, "custom-principal", lookup.principal.ID)
	principal, ok := PrincipalFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "task-1", principal.CallerTaskID)
	require.Equal(t, "session-1", principal.CallerSessionID)
}

func TestScopePrincipalRejectsExistingCustomTaskPrincipalOnAnotherSession(t *testing.T) {
	lookup := &lifecyclePrincipalLookup{
		principalLookup: principalLookup{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1"},
			session:   &models.TaskSession{ID: "session-2", TaskID: "task-1"},
		},
		principal: &models.WorkspaceAgentPrincipal{
			ID:                   "custom-principal",
			WorkspaceID:          "workspace-1",
			PluginInstallationID: "plugin-1",
			LogicalKey:           "custom-key",
			BackingTaskID:        "task-1",
			BackingSessionID:     "session-1",
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-2")
	require.Error(t, err)
	require.Equal(t, "session-1", lookup.principal.BackingSessionID)
}

func TestScopePrincipalRejectsTerminalDispatchingSession(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			lookup := &lifecyclePrincipalLookup{principalLookup: principalLookup{
				task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
				workspace: &models.Workspace{ID: "workspace-1"},
				session:   &models.TaskSession{ID: "session-1", TaskID: "task-1", State: state},
			}}
			resolver := &Resolver{tasks: lookup}

			_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
			require.Error(t, err)
			require.Contains(t, err.Error(), "terminal")
			require.Nil(t, lookup.principal)
		})
	}
}

func TestScopePrincipalRejectsConcurrentTakeoverOfActiveHolderSession(t *testing.T) {
	for _, holderState := range []models.TaskSessionState{
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
	} {
		t.Run("holder_"+string(holderState), func(t *testing.T) {
			lookup := &lifecycleSessionLookup{
				lifecyclePrincipalLookup: lifecyclePrincipalLookup{
					principalLookup: principalLookup{
						task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
						workspace: &models.Workspace{ID: "workspace-1"},
					},
					principal: &models.WorkspaceAgentPrincipal{
						ID:                   "principal-1",
						WorkspaceID:          "workspace-1",
						PluginInstallationID: coordinator.TaskPrincipalInstallationID,
						LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
						BackingTaskID:        "task-1",
						BackingSessionID:     "session-1",
					},
				},
				sessions: map[string]models.TaskSession{
					"session-1": {ID: "session-1", TaskID: "task-1", State: holderState},
					"session-2": {ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateCreated},
				},
			}
			resolver := &Resolver{tasks: lookup}

			_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-2")
			require.Error(t, err)
			require.Contains(t, err.Error(), "bound to an active session")
			require.Equal(t, "session-1", lookup.principal.BackingSessionID,
				"a concurrent session must not steal the binding from a mid-turn holder")
		})
	}
}

func TestScopePrincipalRotatesBindingFromSettledHolderSession(t *testing.T) {
	for _, holderState := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateIdle,
		models.TaskSessionStateCreated,
	} {
		t.Run("holder_"+string(holderState), func(t *testing.T) {
			lookup := &lifecycleSessionLookup{
				lifecyclePrincipalLookup: lifecyclePrincipalLookup{
					principalLookup: principalLookup{
						task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
						workspace: &models.Workspace{ID: "workspace-1"},
					},
					principal: &models.WorkspaceAgentPrincipal{
						ID:                   "principal-1",
						WorkspaceID:          "workspace-1",
						PluginInstallationID: coordinator.TaskPrincipalInstallationID,
						LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
						BackingTaskID:        "task-1",
						BackingSessionID:     "session-1",
					},
				},
				sessions: map[string]models.TaskSession{
					"session-1": {ID: "session-1", TaskID: "task-1", State: holderState},
					"session-2": {ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateRunning},
				},
			}
			resolver := &Resolver{tasks: lookup}

			_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-2")
			require.NoError(t, err)
			require.Equal(t, "session-2", lookup.principal.BackingSessionID,
				"a live session must rotate the binding off a settled (terminal or at-rest) holder")
		})
	}
}

func TestScopePrincipalRotationFromUnreadableHolderKeepsSequentialContract(t *testing.T) {
	// The holder row cannot be read in this repository (unknown session).
	// Without a live-state answer the rotation path keeps the sequential
	// contract: the live dispatching session still takes the binding, exactly
	// like a holder that read as settled.
	lookup := &lifecycleSessionLookup{
		lifecyclePrincipalLookup: lifecyclePrincipalLookup{
			principalLookup: principalLookup{
				task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
				workspace: &models.Workspace{ID: "workspace-1"},
			},
			principal: &models.WorkspaceAgentPrincipal{
				ID:                   "principal-1",
				WorkspaceID:          "workspace-1",
				PluginInstallationID: coordinator.TaskPrincipalInstallationID,
				LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
				BackingTaskID:        "task-1",
				BackingSessionID:     "session-1",
			},
		},
		sessions: map[string]models.TaskSession{
			"session-2": {ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateRunning},
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-2")
	require.NoError(t, err)
	require.Equal(t, "session-2", lookup.principal.BackingSessionID)
}

func TestScopePrincipalKeepsSettledSessionBindingWithoutChurn(t *testing.T) {
	lookup := &lifecycleSessionLookup{
		lifecyclePrincipalLookup: lifecyclePrincipalLookup{
			principalLookup: principalLookup{
				task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
				workspace: &models.Workspace{ID: "workspace-1"},
			},
			principal: &models.WorkspaceAgentPrincipal{
				ID:                   "principal-1",
				WorkspaceID:          "workspace-1",
				PluginInstallationID: coordinator.TaskPrincipalInstallationID,
				LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
				BackingTaskID:        "task-1",
				BackingSessionID:     "session-1",
			},
		},
		sessions: map[string]models.TaskSession{
			"session-1": {ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	require.Equal(t, "session-1", lookup.principal.BackingSessionID,
		"the settled session keeps its binding; repeated dispatches do not churn")
}

func TestScopePrincipalIsolatesRotationPerWorkspace(t *testing.T) {
	// A principal bound in workspace-1 must never rotate for a task in
	// workspace-2: the active lookup is workspace-scoped, so the second
	// workspace creates its own principal. The fixture keeps only one slot,
	// so the create replaces it with the new principal; the original
	// workspace-1 principal therefore survived untouched (its workspace
	// binding persists in the caller's copy).
	lookup := &lifecycleSessionLookup{
		lifecyclePrincipalLookup: lifecyclePrincipalLookup{
			principalLookup: principalLookup{
				task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-2"},
				workspace: &models.Workspace{ID: "workspace-2"},
			},
			principal: &models.WorkspaceAgentPrincipal{
				ID:                   "principal-w1",
				WorkspaceID:          "workspace-1",
				PluginInstallationID: coordinator.TaskPrincipalInstallationID,
				LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
				BackingTaskID:        "task-1",
				BackingSessionID:     "session-w1",
			},
		},
		sessions: map[string]models.TaskSession{
			"session-w2": {ID: "session-w2", TaskID: "task-1", State: models.TaskSessionStateRunning},
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-w2")
	require.NoError(t, err)
	require.NotNil(t, lookup.principal)
	require.Equal(t, "workspace-2", lookup.principal.WorkspaceID,
		"the second workspace gets its own principal")
	require.Equal(t, "session-w2", lookup.principal.BackingSessionID)
}
