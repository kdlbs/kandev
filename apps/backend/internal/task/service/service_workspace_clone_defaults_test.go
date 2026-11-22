package service

import (
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type cloneDefaultsFixture struct {
	svc    *Service
	events *MockEventBus
	repo   *sqliterepo.Repository
	source *models.Workspace
	copier *recordingWorkspaceCloner
}

func newCloneDefaultsFixture(t *testing.T) cloneDefaultsFixture {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	deleted := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, executor := range []*models.Executor{
		{ID: "active-executor", Name: "Active", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
		{ID: "disabled-executor", Name: "Disabled", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusDisabled},
		{ID: "deleted-executor", Name: "Deleted", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive, DeletedAt: &deleted},
	} {
		require.NoError(t, repo.CreateExecutor(t.Context(), executor))
	}
	for _, environment := range []*models.Environment{
		{ID: "active-environment", Name: "Active", Kind: models.EnvironmentKindLocalPC},
		{ID: "deleted-environment", Name: "Deleted", Kind: models.EnvironmentKindLocalPC, DeletedAt: &deleted},
	} {
		require.NoError(t, repo.CreateEnvironment(t.Context(), environment))
	}
	svc.agentProfiles = workflowAgentOverrideProfilesStub{profiles: map[string]*settingsmodels.AgentProfile{
		"shared-profile":   {ID: "shared-profile", Enabled: true},
		"disabled-profile": {ID: "disabled-profile", Enabled: false},
		"deleted-profile":  {ID: "deleted-profile", Enabled: true, DeletedAt: &deleted},
		"scoped-profile":   {ID: "scoped-profile", Enabled: true, WorkspaceID: "source-workspace"},
	}}
	executorID, environmentID, profileID := "active-executor", "active-environment", "shared-profile"
	source := &models.Workspace{
		ID: "source-workspace", Name: "Source", OwnerID: "creator",
		DefaultExecutorID: &executorID, DefaultEnvironmentID: &environmentID,
		DefaultAgentProfileID: &profileID, DefaultConfigAgentProfileID: &profileID,
	}
	require.NoError(t, repo.CreateWorkspace(t.Context(), source))
	copier := &recordingWorkspaceCloner{}
	svc.SetWorkspaceCloner(copier)
	return cloneDefaultsFixture{svc: svc, events: eventBus, repo: repo, source: source, copier: copier}
}

// @covers AC-WORKSPACES-CLONE-001.1
func TestWorkspaceCloneRejectsUnavailableDefaults(t *testing.T) {
	for _, test := range []struct{ field, id string }{
		{"executor", "missing-executor"},
		{"executor", "disabled-executor"},
		{"executor", "deleted-executor"},
		{"environment", "missing-environment"},
		{"environment", "deleted-environment"},
		{"agent", "missing-profile"},
		{"agent", "disabled-profile"},
		{"agent", "deleted-profile"},
		{"agent", "scoped-profile"},
		{"config-agent", "missing-profile"},
		{"config-agent", "disabled-profile"},
		{"config-agent", "deleted-profile"},
		{"config-agent", "scoped-profile"},
	} {
		t.Run(test.field+"/"+test.id, func(t *testing.T) {
			fixture := newCloneDefaultsFixture(t)
			var beforeCount int
			require.NoError(t, fixture.repo.DB().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM workspaces").Scan(&beforeCount))
			fields := map[string]**string{
				"executor": &fixture.source.DefaultExecutorID, "environment": &fixture.source.DefaultEnvironmentID,
				"agent": &fixture.source.DefaultAgentProfileID, "config-agent": &fixture.source.DefaultConfigAgentProfileID,
			}
			*fields[test.field] = &test.id
			require.NoError(t, fixture.repo.UpdateWorkspace(t.Context(), fixture.source))
			target, err := fixture.svc.CloneWorkspace(ctxAs("creator"), fixture.source.ID, "Copy")
			require.ErrorIs(t, err, repoerrors.ErrWorkspaceCloneConfiguration)
			require.Nil(t, target)
			require.Nil(t, fixture.copier.target)
			require.Empty(t, fixture.events.GetPublishedEvents())
			var workspaceCount int
			require.NoError(t, fixture.repo.DB().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM workspaces").Scan(&workspaceCount))
			require.Equal(t, beforeCount, workspaceCount)
		})
	}
}

func TestWorkspaceClonePreservesAvailableDefaults(t *testing.T) {
	fixture := newCloneDefaultsFixture(t)
	target, err := fixture.svc.CloneWorkspace(ctxAs("creator"), fixture.source.ID, "Copy")
	require.NoError(t, err)
	require.Equal(t, "active-executor", *target.DefaultExecutorID)
	require.Equal(t, "active-environment", *target.DefaultEnvironmentID)
	require.Equal(t, "shared-profile", *target.DefaultAgentProfileID)
	require.Equal(t, "shared-profile", *target.DefaultConfigAgentProfileID)
	require.NotNil(t, fixture.copier.target)
	require.Len(t, fixture.events.GetPublishedEvents(), 2)
}
