package storeconformance

import (
	"context"
	"testing"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestration/models"
	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskstore "github.com/kandev/kandev/internal/task/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	workflow "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/stretchr/testify/require"
)

func TestAssistantAuthorityPersistenceEngines(t *testing.T) {
	for _, name := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(name), func(t *testing.T) {
			repo, binding := authorityStoreFixture(t, name)
			ctx := context.Background()
			require.Equal(t, "inspect", binding.ExecutionMode)
			request := models.Operation{BindingID: binding.ID, OperationID: "once", ConversationID: binding.ConversationID,
				RunID: "run", Target: "native-task", RequestHash: "synthetic", BindingVersion: binding.Version}
			prepared, created, err := repo.BeginOperation(ctx, request)
			require.NoError(t, err)
			require.True(t, created)
			require.Equal(t, "prepared", prepared.State)
			binding.ExecutionMode = "execute"
			require.NoError(t, repo.SelectAssistant(ctx, binding, binding.Version))
			require.ErrorIs(t, repo.DispatchOperation(ctx, prepared), models.ErrConflict)
			require.NoError(t, repo.RecoverOperations(ctx))
			unknown, created, err := repo.BeginOperation(ctx, request)
			require.NoError(t, err)
			require.False(t, created)
			require.Equal(t, "unknown", unknown.State)
			request.OperationID, request.BindingVersion = "new-id-same-unknown-target", binding.Version
			_, _, err = repo.BeginOperation(ctx, request)
			require.ErrorIs(t, err, models.ErrConflict)
			request.OperationID, request.BindingVersion = "current", binding.Version
			request.Target = "another-native-target"
			prepared, created, err = repo.BeginOperation(ctx, request)
			require.NoError(t, err)
			require.True(t, created)
			require.NoError(t, repo.DispatchOperation(ctx, prepared))
			require.ErrorIs(t, repo.DispatchOperation(ctx, prepared), models.ErrConflict)
			require.NoError(t, repo.FinishOperation(ctx, prepared.ID, "acknowledged", `{"id":"synthetic-task"}`, 201))
			replay, created, err := repo.BeginOperation(ctx, request)
			require.NoError(t, err)
			require.False(t, created)
			require.Equal(t, "acknowledged", replay.State)
			require.NoError(t, repo.Migrate())
			retained, err := repo.AssistantBinding(ctx, "owner")
			require.NoError(t, err)
			require.Equal(t, "execute", retained.ExecutionMode)
			require.EqualValues(t, 2, retained.Version)
		})
	}
}

func authorityStoreFixture(t *testing.T, name testconformance.EngineName, extraWorkspaces ...string) (*orchstore.Repository, *models.AssistantBinding) {
	t.Helper()
	engine := testconformance.OpenEngine(t, name, "")
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	tasks, err := taskstore.NewWithDB(engine.DB, engine.DB, log)
	require.NoError(t, err)
	_, err = workflow.NewWithDB(engine.DB, engine.DB, log)
	require.NoError(t, err)
	profiles, _, err := settingsstore.Provide(engine.DB, engine.DB, log)
	require.NoError(t, err)
	require.NoError(t, tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "workspace", Name: "Synthetic workspace"}))
	for _, id := range extraWorkspaces {
		require.NoError(t, tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: id, Name: "Synthetic linked workspace"}))
	}
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "provider", Name: "Synthetic provider"}))
	persona := &settings.AgentProfile{ID: "assistant", AgentID: "provider", WorkspaceID: "workspace", Role: settings.AgentRoleAssistant, Name: "Synthetic assistant", Model: "default"}
	require.NoError(t, profiles.CreateAgentProfile(ctx, persona))
	repo := orchstore.New(engine.DB, engine.DB)
	require.NoError(t, repo.Migrate())
	require.NoError(t, repo.RegisterOrchestrator(ctx, persona.ID, "workspace", "chief-of-staff"))
	conversation, err := repo.EnsureAgentConversation(ctx, persona)
	require.NoError(t, err)
	binding := &models.AssistantBinding{OwnerUserID: "owner", OrchestratorID: persona.ID, WorkspaceID: "workspace", ConversationID: conversation.TaskID}
	require.NoError(t, repo.SelectAssistant(ctx, binding, 0))
	return repo, binding
}
