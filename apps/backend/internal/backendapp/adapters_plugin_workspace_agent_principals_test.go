package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestPluginsWorkspaceAgentPrincipalSourceAdapter_ScopesAndRedacts(t *testing.T) {
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "principals.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-owned", Name: "Owned"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "backing-task", WorkspaceID: "ws-owned", Title: "replaceable backing"}))
	principal := &models.WorkspaceAgentPrincipal{ID: "opaque-principal", WorkspaceID: "ws-owned", PluginInstallationID: "plugin-owned", LogicalKey: "agent", BackingTaskID: "backing-task", BackingSessionID: "private-session"}
	require.NoError(t, repo.CreateWorkspaceAgentPrincipal(ctx, principal))
	require.NoError(t, repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{ID: "grant-1", CoordinatorTaskID: "backing-task", PrincipalID: principal.ID, WorkspaceID: "ws-owned", ScopeKind: "workspace", ScopeID: "ws-owned", Capabilities: "inspect,orchestrate", Note: "private operator note", GrantedByUserID: "user-private"}))
	require.NoError(t, repo.CreateCoordinatorAuditEvent(ctx, &models.CoordinatorAuditEvent{ID: "audit-1", PrincipalID: principal.ID, WorkspaceID: "ws-owned", ActorTaskID: "backing-task", ActorSessionID: "private-session", TargetTaskID: "target-private", Action: "task.stop", Capability: "orchestrate", Decision: "allowed", Result: "ok", Detail: "grant"}))
	adapter := pluginsWorkspaceAgentPrincipalSourceAdapter{repo: repo}

	descriptor, principalStatus, err := adapter.GetPluginWorkspaceAgentPrincipal(ctx, "plugin-owned", "ws-owned", "agent")
	require.NoError(t, err)
	require.Equal(t, "opaque-principal", descriptor.ID)
	require.Equal(t, "active", principalStatus.State)
	require.Equal(t, []string{"inspect", "orchestrate"}, principalStatus.GrantedCapabilities)
	// The safe DTO has no backing task/session, grant note/scope, user, or
	// target fields; only the explicit public projection can cross this seam.
	audit, err := adapter.ListPluginWorkspaceAgentPrincipalAudit(ctx, "plugin-owned", "ws-owned", "agent")
	require.NoError(t, err)
	require.Equal(t, []string{"audit-1"}, []string{audit[0].ID})
	require.Equal(t, "grant", audit[0].DetailCode)

	_, _, err = adapter.GetPluginWorkspaceAgentPrincipal(ctx, "plugin-other", "ws-owned", "agent")
	require.Equal(t, codes.NotFound, status.Code(err), "foreign installation is indistinguishable from absent")
	_, _, err = adapter.GetPluginWorkspaceAgentPrincipal(ctx, "plugin-owned", "ws-other", "agent")
	require.Equal(t, codes.NotFound, status.Code(err), "foreign workspace is indistinguishable from absent")
}
