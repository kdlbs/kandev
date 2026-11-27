package sqlite

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type taskSessionSummaryProjectionReader interface {
	BatchGetTaskSessionSummaryObservations(context.Context, []string) (map[string][]*models.TaskSessionSummaryObservation, error)
	ListTaskSessionSummaryObservations(context.Context, string) ([]*models.TaskSessionSummaryObservation, error)
}

func TestSessionSummaryProjectionExcludesUnrelatedMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertSessionSummaryProjection(t, repo)
	assertSessionSummaryProviderModel(t, repo)
}

func TestSessionSummaryProjectionPostgres(t *testing.T) {
	repo := openPostgresRepo(t)
	assertSessionSummaryProjection(t, repo)
	assertSessionSummaryProviderModel(t, repo)
}

func TestSessionSummaryProjectionSelectsOnlySummaryFields(t *testing.T) {
	for _, driver := range []string{"sqlite3", "pgx"} {
		query := taskSessionSummarySelectCols(driver)
		for _, fullColumn := range []string{
			", ts.metadata,", ", ts.agent_profile_snapshot,", ", ts.executor_snapshot,",
			", ts.environment_snapshot,", ", ts.repository_snapshot,",
		} {
			if strings.Contains(query, fullColumn) {
				t.Errorf("%s projection selected full payload column %q: %s", driver, fullColumn, query)
			}
		}
		if !strings.Contains(query, "last_agent_error") || !strings.Contains(query, "executor_type") || !strings.Contains(query, "executor_name") {
			t.Errorf("%s projection omitted a required error or label field: %s", driver, query)
		}
	}
}

func assertSessionSummaryProjection(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	reader, ok := any(repo).(taskSessionSummaryProjectionReader)
	if !ok {
		t.Fatal("task repository does not implement narrow session summary projections")
	}
	const (
		taskID    = "task-session-summary-projection"
		sessionID = "session-summary-projection"
	)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Summary projection"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-summary", Name: "Current local runner", Type: models.ExecutorTypeLocal,
		Status: models.ExecutorStatusActive,
	}); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	startedAt := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	lastError := models.LastAgentError{
		Message: "The runner could not start.", OccurredAt: startedAt,
		Code: "RUNNER_START_FAILED", RecoveryActions: []string{models.RecoveryActionRetryLaunch},
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: "incarnation-summary-projection",
		Name: "Compact row", State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "agent-profile-summary", ExecutionProfileID: "execution-profile-summary",
		RouteGeneration: 4, RouteState: "committed", RouteReason: "selected",
		ExecutorID: "executor-summary", ExecutorProfileID: "executor-profile-summary",
		EnvironmentID: "environment-summary", RepositoryID: "repository-summary",
		BaseBranch: "main", BaseCommitSHA: "abc123", WorkspacePath: "/workspace/summary",
		AgentProfileSnapshot: map[string]interface{}{"name": "Agent label", "irrelevant": strings.Repeat("a", 1<<20)},
		ExecutorSnapshot:     map[string]interface{}{"executor_type": "local", "executor_name": "Local runner", "irrelevant": strings.Repeat("b", 1<<20)},
		RepositorySnapshot:   map[string]interface{}{"path": "/repo/summary", "irrelevant": strings.Repeat("c", 1<<20)},
		EnvironmentSnapshot:  map[string]interface{}{"irrelevant": strings.Repeat("d", 1<<20)},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: map[string]interface{}{
				"current_model_id": "model-old",
				"models":           []interface{}{map[string]interface{}{"model_id": "model-current", "name": "Current model"}},
				"config_options":   strings.Repeat("z", 1<<20),
			},
			"runtime_config_overrides":          map[string]interface{}{"model": "model-current"},
			models.SessionMetaKeyLastAgentError: lastError,
			"unrelated_blob":                    strings.Repeat("x", 1<<20),
		},
		StartedAt: startedAt, UpdatedAt: startedAt.Add(time.Minute), IsPrimary: true,
		IsPassthrough: true, ReviewStatus: models.ReviewStatusApproved,
		TaskEnvironmentID: "task-environment-summary", LastReadMessageID: "message-summary",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-summary-secondary", TaskID: taskID, Name: "Compact row",
		QueueIncarnationID: "incarnation-summary-projection", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-profile-summary", ExecutionProfileID: "execution-profile-summary",
		RouteGeneration: 4, RouteState: "committed", RouteReason: "selected",
		ExecutorID: "executor-summary", ExecutorProfileID: "executor-profile-summary",
		EnvironmentID: "environment-summary", RepositoryID: "repository-summary",
		BaseBranch: "main", BaseCommitSHA: "abc123", WorkspacePath: "/workspace/summary",
		AgentProfileSnapshot: map[string]interface{}{"name": "Agent label", "irrelevant": strings.Repeat("a", 4<<10)},
		ExecutorSnapshot:     map[string]interface{}{"executor_type": "local", "executor_name": "Stale runner", "irrelevant": strings.Repeat("b", 4<<10)},
		RepositorySnapshot:   map[string]interface{}{"path": "/repo/summary", "irrelevant": strings.Repeat("c", 4<<10)},
		EnvironmentSnapshot:  map[string]interface{}{"irrelevant": strings.Repeat("d", 4<<10)},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: map[string]interface{}{
				"current_model_id": "model-old",
				"models":           []interface{}{map[string]interface{}{"model_id": "model-current", "name": "Current model"}},
				"config_options":   strings.Repeat("z", 4<<10),
			},
			"runtime_config_overrides":          map[string]interface{}{"model": "model-current"},
			models.SessionMetaKeyLastAgentError: lastError,
			"unrelated_blob":                    strings.Repeat("y", 4<<10),
		},
		StartedAt: startedAt, UpdatedAt: startedAt.Add(time.Minute), IsPassthrough: true,
		TaskEnvironmentID: "task-environment-summary",
		ReviewStatus:      models.ReviewStatusApproved, LastReadMessageID: "message-summary",
	}); err != nil {
		t.Fatalf("create secondary session: %v", err)
	}
	const noPrimaryTaskID = "task-session-summary-missing-primary"
	if err := repo.CreateTask(ctx, &models.Task{ID: noPrimaryTaskID, Title: "Missing primary"}); err != nil {
		t.Fatalf("create task without a primary session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-summary-no-primary", TaskID: noPrimaryTaskID, State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("create session without a primary: %v", err)
	}

	byTask, err := reader.BatchGetTaskSessionSummaryObservations(ctx, []string{taskID, "task-without-sessions", noPrimaryTaskID})
	if err != nil {
		t.Fatalf("batch-load summary observations: %v", err)
	}
	observations := byTask[taskID]
	if len(observations) != 2 {
		t.Fatalf("summary observations = %+v, want primary and secondary sessions", observations)
	}
	var got *models.TaskSessionSummaryObservation
	var secondary *models.TaskSessionSummaryObservation
	for _, observation := range observations {
		switch observation.ID {
		case sessionID:
			got = observation
		case "session-summary-secondary":
			secondary = observation
		}
	}
	if got == nil || secondary == nil || secondary.IsPrimary || secondary.State != models.TaskSessionStateRunning {
		t.Fatalf("mixed-state summary observations = %+v", observations)
	}
	if got.ID != sessionID || got.TaskID != taskID || !got.IsPrimary || got.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("summary identity/status = %+v", got)
	}
	if got.AgentProfileName != "Agent label" || got.ExecutorType != "local" || got.ExecutorName != "Current local runner" || got.RepositoryPath != "/repo/summary" {
		t.Fatalf("summary labels = %+v", got)
	}
	if raw, ok := got.Metadata[models.SessionMetaKeyLastAgentError]; !ok || raw == nil {
		t.Fatalf("last-agent-error projection = %#v, want the typed visible error", got.Metadata)
	}
	modelState, ok := got.Metadata[models.SessionMetaKeyACPModelState].(map[string]interface{})
	if !ok || modelState["current_model_id"] != "model-current" {
		t.Fatalf("compact model state = %#v", modelState)
	}
	if _, ok := modelState["config_options"]; ok {
		t.Fatal("compact label included rich configuration")
	}
	if _, ok := got.Metadata["unrelated_blob"]; ok {
		t.Fatal("narrow summary observation returned unrelated session metadata")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal summary observation: %v", err)
	}
	if len(encoded) > 16<<10 {
		t.Fatalf("summary projection encoded to %d bytes, want at most 16 KiB regardless of 1 MiB metadata/snapshots", len(encoded))
	}
	secondaryJSON, err := json.Marshal(secondary)
	if err != nil {
		t.Fatalf("marshal 4 KiB summary observation: %v", err)
	}
	var largePayload, smallPayload map[string]interface{}
	if err := json.Unmarshal(encoded, &largePayload); err != nil {
		t.Fatalf("decode 1 MiB summary payload: %v", err)
	}
	if err := json.Unmarshal(secondaryJSON, &smallPayload); err != nil {
		t.Fatalf("decode 4 KiB summary payload: %v", err)
	}
	for _, key := range []string{"ID", "State", "IsPrimary"} {
		delete(largePayload, key)
		delete(smallPayload, key)
	}
	largeComparable, err := json.Marshal(largePayload)
	if err != nil {
		t.Fatalf("marshal normalized 1 MiB projection: %v", err)
	}
	smallComparable, err := json.Marshal(smallPayload)
	if err != nil {
		t.Fatalf("marshal normalized 4 KiB projection: %v", err)
	}
	if len(largeComparable) != len(smallComparable) {
		for key, largeValue := range largePayload {
			if smallValue, exists := smallPayload[key]; !exists || !reflect.DeepEqual(largeValue, smallValue) {
				t.Errorf("summary field %q differs between padded fixtures: large=%#v small=%#v", key, largeValue, smallValue)
			}
		}
		t.Fatalf("projection bytes grew from %d to %d when unrelated metadata/config snapshots grew from 4 KiB to 1 MiB",
			len(smallComparable), len(largeComparable))
	}
	noPrimary := byTask[noPrimaryTaskID]
	if len(noPrimary) != 1 || noPrimary[0].IsPrimary || noPrimary[0].State != models.TaskSessionStateRunning {
		t.Fatalf("missing-primary summary observations = %+v", noPrimary)
	}

	listed, err := reader.ListTaskSessionSummaryObservations(ctx, taskID)
	if err != nil {
		t.Fatalf("list compact session summaries: %v", err)
	}
	var listedPrimary *models.TaskSessionSummaryObservation
	for _, observation := range listed {
		if observation.ID == sessionID {
			listedPrimary = observation
		}
	}
	if len(listed) != 2 || !reflect.DeepEqual(listedPrimary, got) {
		t.Fatalf("compact list projection = %+v, want batch observation %+v", listed, got)
	}
	full, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load full selected session: %v", err)
	}
	if len(full.Metadata["unrelated_blob"].(string)) != 1<<20 || len(full.AgentProfileSnapshot["irrelevant"].(string)) != 1<<20 {
		t.Fatal("full selected-session read lost the complete metadata or configuration snapshots")
	}
}

func assertSessionSummaryProviderModel(t *testing.T, repo *Repository) {
	t.Helper()
	taskID := "summary-provider-model"
	if err := repo.CreateTask(t.Context(), &models.Task{ID: taskID, Title: "Provider model"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: taskID + "-session", TaskID: taskID, State: models.TaskSessionStateWaitingForInput, Metadata: map[string]interface{}{
		models.SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "observed", "settings_policy": "provider_restored", "models": []interface{}{map[string]interface{}{"model_id": "observed", "name": "Observed"}}},
		"runtime_config_overrides":         map[string]interface{}{"model": "stale-local"},
	}}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListTaskSessionSummaryObservations(t.Context(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("provider rows = %d, want 1", len(rows))
	}
	label, _ := rows[0].Metadata[models.SessionMetaKeyACPModelState].(map[string]interface{})
	if label["current_model_id"] != "observed" {
		t.Fatalf("provider projection = %#v, want observed", label)
	}
}
