package runtime

import (
	"context"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantBindingSwitchKeepsPreviousConversationPrivate(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	human, foreign := assistantRouter(s), assistantRouter(s, "other-user")
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "private-source", TaskID: task, AuthorType: "user", AuthorID: "owner", Body: "PRIVATE_HISTORY_CANARY", Source: "user"}))
	comments := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 404, runtimeRequest(t, foreign, "GET", comments, "", "", nil).Code)
	switchPrivateAssistant(t, s, human, 1)
	after := runtimeRequest(t, foreign, "GET", comments, "", "", nil)
	require.Equal(t, 404, after.Code, "switching the default must not publish previous private history: %s", after.Body.String())
	owner := runtimeRequest(t, human, "GET", comments, "", "", nil)
	require.Equal(t, 200, owner.Code)
	require.Contains(t, owner.Body.String(), "PRIVATE_HISTORY_CANARY")
	claimed := runtimeRequest(t, foreign, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"})
	require.Equal(t, 409, claimed.Code, claimed.Body.String())
	reselected := runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief", "expected_version": 2})
	require.Equal(t, 200, reselected.Code, reselected.Body.String())
}

func switchPrivateAssistant(t *testing.T, s *Service, human *gin.Engine, version int64) {
	t.Helper()
	ctx := context.Background()
	profile := &settings.AgentProfile{ID: "second-chief", AgentID: "claude", WorkspaceID: "ws", Name: "Second", Role: settings.AgentRoleAssistant, Status: settings.AgentStatusIdle,
		Settings: `{"routing":{"execution_profile_id":"personal"}}`, ExecutorPreference: `{"executor_profile_id":"local"}`}
	require.NoError(t, s.Personas.Profiles.CreateAgentProfile(ctx, profile))
	require.NoError(t, s.Repo.RegisterOrchestrator(ctx, profile.ID, "ws", "chief-of-staff"))
	switched := runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": profile.ID, "expected_version": version})
	require.Equal(t, 200, switched.Code, switched.Body.String())
}

func TestAssistantBindingRemovalRetainsOwner(t *testing.T) {
	for _, removal := range []string{"binding", "registration"} {
		t.Run(removal, func(t *testing.T) {
			s, db, task := newRuntime(t)
			ctx := context.Background()
			human := assistantRouter(s)
			require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
			if removal == "binding" {
				_, err := db.Exec("DELETE FROM orchestration_assistant_bindings")
				require.NoError(t, err)
			} else {
				require.NoError(t, s.Repo.UnregisterOrchestrator(ctx, "chief"))
			}
			require.NoError(t, s.Repo.Migrate())
			owner, err := s.Repo.ConversationUserOwner(ctx, task)
			require.NoError(t, err)
			require.Equal(t, "owner", owner)
			require.NoError(t, s.Repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"))
			foreign := runtimeRequest(t, assistantRouter(s, "foreign"), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"})
			require.Equal(t, 409, foreign.Code)
		})
	}
}

func TestAssistantBindingMigrationBackfillsAndReplays(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	human := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	// Recreate the pre-ownership schema, whether or not this build has the column.
	var columns int
	require.NoError(t, db.Get(&columns, "SELECT count(*) FROM pragma_table_info('orchestration_conversations') WHERE name='owner_user_id'"))
	if columns > 0 {
		_, err := db.Exec("ALTER TABLE orchestration_conversations DROP COLUMN owner_user_id")
		require.NoError(t, err)
	}
	require.NoError(t, s.Repo.Migrate())
	switchPrivateAssistant(t, s, human, 1)
	for range 2 {
		require.NoError(t, s.Repo.Migrate())
		owner, err := s.Repo.ConversationUserOwner(ctx, task)
		require.NoError(t, err)
		require.Equal(t, "owner", owner)
	}
}

func TestAssistantBindingFailedSelectionDoesNotClaimConversation(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	failed := runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief", "expected_version": 99})
	require.Equal(t, 409, failed.Code)
	owner, err := s.Repo.ConversationUserOwner(ctx, task)
	require.NoError(t, err)
	require.Empty(t, owner)
	success := runtimeRequest(t, assistantRouter(s, "winner"), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"})
	require.Equal(t, 200, success.Code)
	owner, err = s.Repo.ConversationUserOwner(ctx, task)
	require.NoError(t, err)
	require.Equal(t, "winner", owner)
}

func TestAssistantBindingConcurrentClaimsKeepOneOwner(t *testing.T) {
	s, db, task := newRuntime(t)
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for _, user := range []string{"first", "second"} {
		router := assistantRouter(s, user)
		wg.Go(func() {
			<-start
			results <- runtimeRequest(t, router, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code
		})
	}
	close(start)
	wg.Wait()
	close(results)
	var codes []int
	for code := range results {
		codes = append(codes, code)
	}
	require.ElementsMatch(t, []int{200, 409}, codes)
	assertRetainedBindingOwner(t, s, db, task)
}

func assertRetainedBindingOwner(t *testing.T, s *Service, db *sqlx.DB, task string) {
	t.Helper()
	var selected string
	require.NoError(t, db.Get(&selected, "SELECT owner_user_id FROM orchestration_assistant_bindings WHERE conversation_id=?", task))
	owner, err := s.Repo.ConversationUserOwner(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, selected, owner)
}
