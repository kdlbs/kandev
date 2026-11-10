package backendapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestDynamicRouteActionCandidateSwitchConflictRequiresRecovery(t *testing.T) {
	for _, scenario := range []string{"revision_conflict", "cancellation", "successful_launch"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			_, profiles, repo := openTasklessDynamicRepo(t)
			seedDynamicRouteActionConflictFixture(t, profiles, repo)
			resolver := newDynamicRouteActionConflictResolver(t, profiles, repo)

			launcherCalls := 0
			launcher := func(context.Context, string) error {
				launcherCalls++
				return nil
			}
			persister := func(
				ctx context.Context,
				session *models.TaskSession,
				expectedState models.TaskSessionState,
				expectedUpdatedAt time.Time,
			) (bool, error) {
				route, err := repo.LoadRouteState(ctx, session.ID)
				require.NoError(t, err)
				require.NotNil(t, route)
				require.Equal(t, int64(2), route.Generation, "the resolver must claim the successor before the session write")
				require.Equal(t, "candidate-b", route.ExecutionProfileID)

				switch scenario {
				case "revision_conflict":
					require.NoError(t, repo.SetSessionMetadataKey(
						ctx, session.ID, models.SessionMetaKeyRuntimeConfig, map[string]interface{}{"model": "old-model"},
					))
				case "cancellation":
					latest, err := repo.GetTaskSession(ctx, session.ID)
					require.NoError(t, err)
					latest.State = models.TaskSessionStateCancelled
					require.NoError(t, repo.UpdateTaskSession(ctx, latest))
				}
				return repo.UpdateTaskSessionIfCurrentSnapshot(
					ctx, session, expectedState, expectedUpdatedAt, session.Metadata,
				)
			}

			handler := dynamicRouteActionHandler(repo, profiles, resolver, persister, launcher)
			result, err := handler(ctx, orchestrator.RouteActionRequest{
				SessionID: "session-conflict", Action: orchestrator.RouteActionTryNext, ExpectedGeneration: 1,
			})

			if scenario == "successful_launch" {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 1, launcherCalls)
				require.Equal(t, "candidate-b", result.ExecutionProfileID)
				persisted, getErr := repo.GetTaskSession(ctx, "session-conflict")
				require.NoError(t, getErr)
				require.Equal(t, "candidate-b", persisted.ExecutionProfileID)
				require.Equal(t, int64(2), persisted.RouteGeneration)
				return
			}

			var conflict *orchestrator.RouteActionConflictError
			require.ErrorAs(t, err, &conflict)
			require.Nil(t, result)
			require.Equal(t, "action_required", conflict.Result.State)
			require.Equal(t, int64(2), conflict.Result.RouteGeneration)
			require.Equal(t, "candidate-b", conflict.Result.ExecutionProfileID)
			require.Zero(t, launcherCalls, "a session projection conflict must not launch the successor")

			persisted, getErr := repo.GetTaskSession(ctx, "session-conflict")
			require.NoError(t, getErr)
			if scenario == "revision_conflict" {
				require.Equal(t, map[string]interface{}{"model": "old-model"}, persisted.Metadata[models.SessionMetaKeyRuntimeConfig])
				require.Equal(t, models.TaskSessionStateWaitingForInput, persisted.State)
			} else {
				require.Equal(t, models.TaskSessionStateCancelled, persisted.State)
			}
		})
	}
}

func seedDynamicRouteActionConflictFixture(
	t *testing.T,
	profiles settingsstore.Repository,
	repo *taskrepo.Repository,
) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, profiles.CreateAgentProfile(ctx, &settings.AgentProfile{
		ID: "candidate-b", AgentID: "concrete-agent", Name: "Second", Enabled: true,
	}))
	dynamicProfiles := profiles.(settingsstore.DynamicProfileRepository)
	policyJSON, err := json.Marshal(routingpolicy.DefaultDocument())
	require.NoError(t, err)
	require.NoError(t, dynamicProfiles.UpdateDynamicAgentProfile(
		ctx,
		&settings.DynamicAgentProfile{ProfileID: "profile-dynamic"},
		1,
		[]settings.DynamicAgentRoute{
			{DynamicProfileID: "profile-dynamic", ExecutionProfileID: "concrete-profile", Enabled: true, RulesJSON: string(policyJSON)},
			{DynamicProfileID: "profile-dynamic", ExecutionProfileID: "candidate-b", Enabled: true, Position: 1},
		},
	))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-conflict", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-conflict", WorkspaceID: "workspace-conflict", Name: "Workflow",
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-conflict", WorkspaceID: "workspace-conflict", WorkflowID: "workflow-conflict", Title: "Task", Priority: "medium",
	}))
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-conflict", TaskID: "task-conflict", State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "profile-dynamic", ExecutionProfileID: "concrete-profile", RouteGeneration: 1,
		StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: "session-conflict", LogicalProfileID: "profile-dynamic", ExecutionProfileID: "concrete-profile",
		Generation: 1, ProfileVersion: 2, Status: "active", UpdatedAt: now,
	}))
}

func newDynamicRouteActionConflictResolver(
	t *testing.T,
	profiles settingsstore.Repository,
	repo *taskrepo.Repository,
) *runtime.ProfileExecutionResolver {
	t.Helper()
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	return runtime.NewProfileExecutionResolver(profiles, engine, true)
}
