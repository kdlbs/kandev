package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	orchmodels "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestE2EResetOrchestrationPreservesOtherWorkspace(t *testing.T) {
	a, svc, repo, conversation := privateConversationFixture(t)
	ctx := context.Background()
	database := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	profiles, _, err := settingsstore.Provide(database, database, nil)
	require.NoError(t, err)
	require.NoError(t, a.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "other-workspace", Name: "Other example"}))
	foreign := &settings.AgentProfile{ID: "other-chief", AgentID: "provider", Name: "Other assistant", WorkspaceID: "other-workspace", Role: settings.AgentRoleAssistant}
	require.NoError(t, profiles.CreateAgentProfile(ctx, foreign))
	require.NoError(t, repo.SaveOrchestratorRole(ctx, &orchmodels.OrchestratorRole{ID: "shared-example-role", Name: "Shared example"}))
	require.NoError(t, repo.RegisterOrchestrator(ctx, foreign.ID, foreign.WorkspaceID, "shared-example-role"))
	require.NoError(t, repo.RegisterOrchestrator(ctx, "private-chief", "ws-1", "shared-example-role"))
	other, err := repo.EnsureAgentConversation(ctx, foreign)
	require.NoError(t, err)
	foreignBinding := &orchmodels.AssistantBinding{OwnerUserID: "other-owner", OrchestratorID: foreign.ID, WorkspaceID: foreign.WorkspaceID, ConversationID: other.TaskID}
	require.NoError(t, repo.SelectAssistant(ctx, foreignBinding, 0))
	for _, id := range []string{"private-chief", foreign.ID} {
		require.NoError(t, repo.UpsertAgentMemory(ctx, &orchmodels.AgentMemory{ID: id + "-memory", AgentProfileID: id, Layer: "user", Key: "example", Content: "Use short headings."}))
	}
	require.NoError(t, deleteTaskForE2EReset(ctx, svc, conversation))
	require.NoError(t, resetOrchestrationForE2E(ctx, database, "ws-1"))
	for _, table := range []string{"workspace_orchestrators", "orchestration_conversations"} {
		assertWorkspaceRows(t, database, table, "ws-1", 0)
		assertWorkspaceRows(t, database, table, foreign.WorkspaceID, 1)
	}
	var count int
	require.NoError(t, database.Get(&count, `SELECT count(*) FROM orchestration_memory WHERE agent_profile_id='private-chief'`))
	require.Zero(t, count)
	require.NoError(t, database.Get(&count, `SELECT count(*) FROM agent_profiles WHERE id='private-chief'`))
	require.Zero(t, count)
	require.NoError(t, database.Get(&count, `SELECT count(*) FROM orchestration_memory WHERE agent_profile_id=?`, foreign.ID))
	require.Equal(t, 1, count)
	_, err = repo.AssistantBinding(ctx, foreignBinding.OwnerUserID)
	require.NoError(t, err)
	_, err = repo.GetOrchestratorRole(ctx, "shared-example-role")
	require.NoError(t, err, "a role used by a different fixture workspace must remain")
	require.NoError(t, deleteTaskForE2EReset(ctx, svc, other.TaskID))
	require.NoError(t, resetOrchestrationForE2E(ctx, database, foreign.WorkspaceID))
	_, err = repo.GetOrchestratorRole(ctx, "shared-example-role")
	require.Error(t, err, "the last owning workspace releases its disposable role")
	_, err = repo.GetOrchestratorRole(ctx, "chief-of-staff")
	require.NoError(t, err)
}

func TestE2EResetRoutingPreservesExecutionAndCoordinatorProfiles(t *testing.T) {
	a, _, _, _ := privateConversationFixture(t)
	ctx := context.Background()
	database := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	profiles, _, err := settingsstore.Provide(database, database, nil)
	require.NoError(t, err)
	const original = `{"routing":{"execution_profile_id":"synthetic-account"}}`
	for _, p := range []*settings.AgentProfile{
		{ID: "worker-profile", AgentID: "provider", WorkspaceID: "ws-1", Name: "Example worker", Settings: original},
		{ID: "office-chief", AgentID: "provider", WorkspaceID: "ws-1", Name: "Example office", Role: "ceo", Settings: original},
	} {
		require.NoError(t, profiles.CreateAgentProfile(ctx, p))
	}
	_, err = database.Exec(`UPDATE agent_profiles SET settings=? WHERE id='private-chief'`, original)
	require.NoError(t, err)
	require.NoError(t, resetOfficeRoutingForE2E(ctx, database, "ws-1"))
	for _, id := range []string{"private-chief", "worker-profile"} {
		p, getErr := profiles.GetAgentProfile(ctx, id)
		require.NoError(t, getErr)
		require.Equal(t, original, p.Settings)
	}
	p, err := profiles.GetAgentProfile(ctx, "office-chief")
	require.NoError(t, err)
	require.JSONEq(t, `{"routing":{"provider_order_source":"inherit","tier_source":"inherit"}}`, p.Settings)
}
