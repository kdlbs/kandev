package backendapp

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type usageOwnerFixture struct {
	workspace string
	err       error
}

func (f usageOwnerFixture) GetMCPUsageProfile(_ context.Context, w, id string) (*mcpconfig.UsageOwner, error) {
	return &mcpconfig.UsageOwner{UsageReference: mcpconfig.UsageReference{Scope: mcpconfig.SelectionScopeProfile, OwnerID: id}, WorkspaceID: w, DisplayName: "Profile", AgentID: "agent", OwnerState: "disabled"}, f.err
}
func (f usageOwnerFixture) GetRepository(context.Context, string) (*models.Repository, error) {
	return &models.Repository{ID: "owner", WorkspaceID: f.workspace, Name: "Repository"}, f.err
}
func (f usageOwnerFixture) GetTask(_ context.Context, id string) (*models.Task, error) {
	now := time.Now()
	return &models.Task{ID: id, WorkspaceID: f.workspace, Title: "Task", ArchivedAt: &now}, f.err
}
func (f usageOwnerFixture) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: "owner", TaskID: "task", Name: "Session"}, f.err
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-001.3
func TestCatalogUsageOwnerProjection(t *testing.T) {
	fixture := usageOwnerFixture{workspace: "workspace"}
	reader := mcpUsageOwnerReader(fixture, fixture)
	for _, scope := range []mcpconfig.SelectionScope{mcpconfig.SelectionScopeProfile, mcpconfig.SelectionScopeRepository, mcpconfig.SelectionScopeTask, mcpconfig.SelectionScopeTaskSession} {
		owner, err := reader(context.Background(), "workspace", mcpconfig.UsageReference{Scope: scope, OwnerID: "owner"})
		require.NoError(t, err)
		require.NotNil(t, owner)
		require.NotEmpty(t, owner.DisplayName)
		require.Equal(t, "workspace", owner.WorkspaceID)
		if scope == mcpconfig.SelectionScopeTaskSession {
			require.Equal(t, "task", owner.TaskID)
			require.Equal(t, "archived", owner.OwnerState)
		}
	}
	for _, scope := range []mcpconfig.SelectionScope{mcpconfig.SelectionScopeRepository, mcpconfig.SelectionScopeTask, mcpconfig.SelectionScopeTaskSession} {
		owner, err := reader(context.Background(), "other", mcpconfig.UsageReference{Scope: scope, OwnerID: "owner"})
		require.NoError(t, err)
		require.Nil(t, owner)
	}
}
func TestCatalogUsageOwnerReadFailure(t *testing.T) {
	for _, testErr := range []error{repoerrors.ErrTaskNotFound, models.ErrTaskSessionNotFound, repoerrors.ErrRepositoryNotFound, errors.New("database failed")} {
		fixture := usageOwnerFixture{workspace: "workspace", err: testErr}
		reader := mcpUsageOwnerReader(fixture, fixture)
		owner, err := reader(context.Background(), "workspace", mcpconfig.UsageReference{Scope: mcpconfig.SelectionScopeTaskSession, OwnerID: "owner"})
		require.Nil(t, owner)
		if testErr.Error() == "database failed" {
			require.ErrorIs(t, err, testErr)
		} else {
			require.NoError(t, err)
		}
	}
}
