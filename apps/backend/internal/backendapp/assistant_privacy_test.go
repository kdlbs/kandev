package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	officestore "github.com/kandev/kandev/internal/office/repository/sqlite"
	orchmodels "github.com/kandev/kandev/internal/orchestration/models"
	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func TestAssistantPrivacyCoreTaskAndSessionAccess(t *testing.T) {
	a, svc, _, taskID := privateConversationFixture(t)
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	ctx := context.Background()
	require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "private-session", TaskID: taskID, AgentProfileID: "private-chief", State: models.TaskSessionStateCompleted}))
	require.NoError(t, a.taskRepo.CreateTurn(ctx, &models.Turn{ID: "private-turn", TaskID: taskID, TaskSessionID: "private-session"}))
	require.NoError(t, a.taskRepo.CreateMessage(ctx, &models.Message{ID: "private-message", TaskID: taskID, TaskSessionID: "private-session", TurnID: "private-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "PRIVATE_NATIVE_HISTORY"}))
	// Deleting the default and disabling the runtime must not bypass the native
	// task/session authorization used by HTTP, WebSocket and MCP consumers.
	_, err := db.Exec("DELETE FROM orchestration_assistant_bindings")
	require.NoError(t, err)
	for _, identity := range []authn.Identity{
		{UserID: "foreign", Role: authn.RoleMember},
		{UserID: "foreign-admin", Role: authn.RoleAdmin},
		{UserID: "foreign-synthetic", Role: authn.RoleAdmin, Synthetic: true},
	} {
		foreign := authn.WithIdentity(ctx, identity)
		_, err = svc.GetTask(foreign, taskID)
		require.ErrorIs(t, err, repoerrors.ErrTaskNotFound, identity.UserID)
		require.ErrorIs(t, svc.AuthorizeTaskAccess(foreign, taskID), repoerrors.ErrTaskNotFound)
		require.ErrorIs(t, svc.AuthorizeSessionAccess(foreign, "private-session"), repoerrors.ErrTaskNotFound)
		_, err = svc.ListMessages(foreign, "private-session")
		require.ErrorIs(t, err, repoerrors.ErrTaskNotFound)
	}
	for _, allowed := range []context.Context{ctx, authn.WithIdentity(ctx, authn.Identity{UserID: "owner"}), authn.WithIdentity(ctx, authn.Identity{UserID: "owner", Synthetic: true})} {
		messages, err := svc.ListMessages(allowed, "private-session")
		require.NoError(t, err)
		require.Len(t, messages, 1)
		require.Equal(t, "PRIVATE_NATIVE_HISTORY", messages[0].Content)
	}
}

func privateConversationFixture(t *testing.T) (*taskCreatorAdapter, *taskservice.Service, *orchstore.Repository, string) {
	t.Helper()
	a, svc := newOfficeTaskAdapterHarness(t)
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	repo := orchstore.New(db, db)
	require.NoError(t, repo.Migrate())
	wireAssistantOwnership(svc, repo)
	ctx := context.Background()
	profiles, _, err := settingsstore.Provide(db, db, nil)
	require.NoError(t, err)
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "provider", Name: "provider"}))
	persona := &settings.AgentProfile{ID: "private-chief", AgentID: "provider", Name: "Private chief", Role: settings.AgentRoleAssistant, WorkspaceID: "ws-1"}
	require.NoError(t, profiles.CreateAgentProfile(ctx, persona))
	require.NoError(t, repo.RegisterOrchestrator(ctx, persona.ID, persona.WorkspaceID, "chief-of-staff"))
	conversation, err := repo.EnsureAgentConversation(ctx, persona)
	require.NoError(t, err)
	require.NoError(t, repo.SelectAssistant(ctx, &orchmodels.AssistantBinding{OwnerUserID: "owner", OrchestratorID: persona.ID, WorkspaceID: persona.WorkspaceID, ConversationID: conversation.TaskID}, 0))
	return a, svc, repo, conversation.TaskID
}

func TestAssistantPrivacyOfficeCompatibilityRetainsOwnership(t *testing.T) {
	a, svc, repo, taskID := privateConversationFixture(t)
	ctx := context.Background()
	require.NoError(t, repo.UnregisterOrchestrator(ctx, "private-chief"))
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	office, err := officestore.NewWithDB(db, db, nil)
	require.NoError(t, err)
	params := routeParams{features: config.FeaturesConfig{Office: true}, officeRepo: office, orchestrationRepo: repo, taskSvc: svc}
	for _, path := range []string{"/api/v1/office/agents/private-chief", "/api/v1/office/tasks/" + taskID + "/comments"} {
		allowed, err := orchestrationBrowserRouteAllowed(ctx, params, path)
		require.NoError(t, err)
		require.False(t, allowed, path)
	}
	allowed, err := orchestrationRunGuard(params.features, office)(ctx, "private-chief")
	require.NoError(t, err)
	require.False(t, allowed, "unregistering a private persona must not make it an Office execution target")
}
