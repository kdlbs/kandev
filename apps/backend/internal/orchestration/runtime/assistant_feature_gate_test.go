package runtime

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func defaultAssistantRuntime(s *Service) *Service {
	return &Service{Repo: s.Repo, Personas: s.Personas, Runs: s.Runs, Queue: s.Queue,
		Auth: s.Auth, Tasks: s.Tasks, Manager: s.Manager}
}

// @covers AC-ORCHESTRATION-ASSISTANT-010.1, AC-ORCHESTRATION-ASSISTANT-010.2
func TestAssistantFeatureGateDefaultRoutes(t *testing.T) {
	s, _, _ := newRuntime(t)
	router := assistantRouter(defaultAssistantRuntime(s))
	for _, methodPath := range [][2]string{
		{"GET", "/assistant"}, {"PUT", "/assistant"}, {"GET", "/assistant/memory/example/source"},
		{"POST", "/assistant/control"}, {"GET", "/assistant/attention/example/input"}, {"GET", "/runtime/attention/example/input"}, {"POST", "/assistant/attention/example/resolve"}, {"POST", "/runtime/attention/example/answer"},
		{"GET", "/assistant/capabilities"}, {"GET", "/runtime/capabilities"}, {"GET", "/runtime/memory"},
		{"GET", "/assistant/attention"}, {"GET", "/runtime/attention"}, {"GET", "/assistant/objectives"}, {"GET", "/assistant/memory"},
		{"PUT", "/assistant/memory/example"}, {"DELETE", "/assistant/memory/example"},
		{"GET", "/assistant/credentials"}, {"PUT", "/assistant/credentials/example"},
		{"GET", "/runtime/objectives"}, {"POST", "/runtime/objectives"},
		{"GET", "/assistant/improvements"}, {"GET", "/runtime/improvements"},
		{"GET", "/assistant/improvements/example"}, {"GET", "/runtime/improvements/example"},
		{"PUT", "/assistant/improvements/example/grant"}, {"DELETE", "/assistant/improvements/example/grant"},
		{"POST", "/assistant/improvements/example/maintenance"}, {"POST", "/runtime/improvements/example/maintenance"},
		{"GET", "/assistant/maintenance-options"}, {"GET", "/assistant/improvements/example/file"}, {"GET", "/runtime/improvements/example/file"},
		{"GET", "/assistant/improvements/example/artifact"}, {"GET", "/runtime/improvements/example/artifact"},
		{"POST", "/assistant/improvements/example/review"}, {"POST", "/assistant/improvements/example/reconcile"},
		{"GET", "/assistant/improvements/example/successes"},
		{"GET", "/assistant/improvements/example/evidence"}, {"GET", "/runtime/improvements/example/evidence"},
		{"GET", "/assistant/workspace-links"}, {"GET", "/assistant/workspace-options"},
		{"GET", "/runtime/workspace-links"}, {"GET", "/runtime/tasks"},
		{"POST", "/assistant/workspace-links/example/forget"}, {"GET", "/assistant/workspace-exports"},
		{"PUT", "/assistant/workspace-links/example"}, {"DELETE", "/assistant/workspace-links/example"},
		{"GET", "/assistant/workspace-links/example/events"},
		{"PATCH", "/runtime/objectives/example"}, {"GET", "/runtime/context/example"},
	} {
		t.Run(methodPath[0]+methodPath[1], func(t *testing.T) {
			response := runtimeRequest(t, router, methodPath[0], "/api/v1/orchestration"+methodPath[1], "", "", map[string]any{"orchestrator_id": "chief"})
			require.Equal(t, 404, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "personal_assistant_disabled")
		})
	}
}

func bindFeatureTestAssistant(t *testing.T, s *Service, task string) {
	t.Helper()
	require.NoError(t, s.Repo.SelectAssistant(context.Background(), &models.AssistantBinding{
		OwnerUserID: "owner", OrchestratorID: "chief", WorkspaceID: "ws", ConversationID: task,
	}, 0))
}

// @covers AC-ORCHESTRATION-ASSISTANT-010.2, AC-ORCHESTRATION-ASSISTANT-010.3
func TestAssistantFeatureGateRetainsHistoryWithoutAdmittingTurns(t *testing.T) {
	s, _, task := newRuntime(t)
	bindFeatureTestAssistant(t, s, task)
	disabled := defaultAssistantRuntime(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(disabled), "GET", path, "", "", nil).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(disabled, "foreign"), "GET", path, "", "", nil).Code)
	response := runtimeRequest(t, assistantRouter(disabled), "POST", path, "", "", map[string]string{"body": "Summarize the example tasks"})
	require.Equal(t, 404, response.Code, response.Body.String())
	rows, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Empty(t, rows)
	owner, err := s.Repo.ConversationUserOwner(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, "owner", owner)
}

func TestAssistantFeatureGateBlocksQueueAndLaunch(t *testing.T) {
	s, _, task := newRuntime(t)
	bindFeatureTestAssistant(t, s, task)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "callback", "queued-before-disable", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	disabled := defaultAssistantRuntime(s)
	require.Error(t, disabled.QueueTurn(ctx, "chief", task, "callback", "disabled", map[string]any{"binding_version": 1}))
	started := false
	disabled.Start = func(context.Context, Launch) error { started = true; return nil }
	handled, err := disabled.Process(ctx, run)
	require.True(t, handled)
	require.Error(t, err)
	require.False(t, started)
}

func TestAssistantFeatureGatePreservesPendingIntake(t *testing.T) {
	s, _, task := newRuntime(t)
	bindFeatureTestAssistant(t, s, task)
	ctx := context.Background()
	_, _, err := s.Repo.AcceptComment(ctx, "chief", "pending", &models.TaskComment{TaskID: task, AuthorID: "owner", Body: "Summarize the example tasks"})
	require.NoError(t, err)
	require.NoError(t, defaultAssistantRuntime(s).DispatchIntake(ctx))
	rows, err := s.Repo.PendingIntake(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Nil(t, run)
}

func TestAssistantFeatureGateKeepsOrdinaryCoordinatorAvailable(t *testing.T) {
	s, _, task := newRuntime(t)
	disabled := defaultAssistantRuntime(s)
	response := runtimeRequest(t, assistantRouter(disabled), "POST", "/api/v1/orchestration/tasks/"+task+"/comments", "", "", map[string]string{"body": "Summarize the example tasks"})
	require.Equal(t, 201, response.Code, response.Body.String())
}
