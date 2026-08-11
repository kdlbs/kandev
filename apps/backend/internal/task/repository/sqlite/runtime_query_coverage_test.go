package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPluginMessageFiltersBoundsTypesAndPagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-plugin", "session-plugin", "turn-plugin")
	base := time.Date(2026, time.May, 6, 7, 8, 9, 0, time.UTC)
	for _, message := range []*models.Message{
		{ID: "plugin-a", TaskSessionID: "session-plugin", TaskID: "task-plugin", TurnID: "turn-plugin", Content: "a", Type: models.MessageTypeMessage, CreatedAt: base},
		{ID: "plugin-b", TaskSessionID: "session-plugin", TaskID: "task-plugin", TurnID: "turn-plugin", Content: "b", Type: models.MessageTypeToolCall, CreatedAt: base.Add(time.Second)},
		{ID: "plugin-c", TaskSessionID: "session-plugin", TaskID: "task-plugin", TurnID: "turn-plugin", Content: "c", Type: models.MessageTypeMessage, CreatedAt: base.Add(2 * time.Second)},
	} {
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	since, until := base, base.Add(3*time.Second)
	got, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{SessionIDs: []string{"session-plugin"}, TaskIDs: []string{"task-plugin"}, Types: []string{string(models.MessageTypeMessage)}, Since: &since, Until: &until, Limit: 1, Offset: 1})
	if err != nil || strings.Join(messageIDs(got), ",") != "plugin-c" {
		t.Fatalf("ListMessagesForPlugin = %v, %v", messageIDs(got), err)
	}
	all, err := repo.ListMessagesForPlugin(ctx, models.PluginMessageFilter{})
	if err != nil || len(all) < 3 {
		t.Fatalf("unfiltered ListMessagesForPlugin = %d rows, %v", len(all), err)
	}
}

func TestExecutorRunningCASStatusAndDeleteLifecycle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-running", "session-running", "turn-running")
	running := &models.ExecutorRunning{SessionID: "session-running", TaskID: "task-running", AgentExecutionID: "execution-one", Status: "running", Resumable: true, ResumeToken: "old", Metadata: map[string]any{"key": "value"}}
	if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	if exists, err := repo.HasExecutorRunningRow(ctx, "session-running"); err != nil || !exists {
		t.Fatalf("HasExecutorRunningRow = %v, %v", exists, err)
	}
	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-running")
	if err != nil || got.ID != "session-running" || !got.Resumable || got.Metadata["key"] != "value" {
		t.Fatalf("GetExecutorRunningBySessionID = %+v, %v", got, err)
	}
	if err := repo.UpdateResumeToken(ctx, "session-running", "execution-one", "new", "message-uuid"); err != nil {
		t.Fatalf("UpdateResumeToken: %v", err)
	}
	if err := repo.UpdateResumeToken(ctx, "session-running", "rotated", "bad", "bad"); !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("rotated UpdateResumeToken error = %v", err)
	}
	if err := repo.UpdateExecutorRunningStatus(ctx, "session-running", "stopped"); err != nil {
		t.Fatalf("UpdateExecutorRunningStatus: %v", err)
	}
	got, err = repo.GetExecutorRunningBySessionID(ctx, "session-running")
	if err != nil || got.Status != "stopped" || got.ResumeToken != "new" || got.LastMessageUUID != "message-uuid" {
		t.Fatalf("updated running row = %+v, %v", got, err)
	}
	if err := repo.DeleteExecutorRunningBySessionID(ctx, "session-running"); err != nil {
		t.Fatalf("DeleteExecutorRunningBySessionID: %v", err)
	}
	if exists, err := repo.HasExecutorRunningRow(ctx, "session-running"); err != nil || exists {
		t.Fatalf("HasExecutorRunningRow after delete = %v, %v", exists, err)
	}
	if err := repo.DeleteExecutorRunningBySessionID(ctx, "session-running"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
	if err := repo.UpdateExecutorRunningStatus(ctx, "missing", "stopped"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("missing status update error = %v", err)
	}
}

func TestSessionStateGuardedMetadataReviewAndBatchLookup(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-session-state", "session-session-state", "turn-session-state")
	if err := repo.UpdateSessionReviewStatus(ctx, "session-session-state", "approved"); err != nil {
		t.Fatalf("UpdateSessionReviewStatus: %v", err)
	}
	if err := repo.UpdateSessionReviewStatus(ctx, "missing", "approved"); err == nil {
		t.Fatal("UpdateSessionReviewStatus accepted missing session")
	}
	claimed, err := repo.SetSessionMetadataKeyIfAbsentIfState(ctx, "session-session-state", "claim", map[string]any{"attempt": 1}, models.TaskSessionStateCreated)
	if err != nil || !claimed {
		t.Fatalf("SetSessionMetadataKeyIfAbsentIfState = %v, %v", claimed, err)
	}
	claimed, err = repo.SetSessionMetadataKeyIfAbsentIfState(ctx, "session-session-state", "claim", "replacement", models.TaskSessionStateCreated)
	if err != nil || claimed {
		t.Fatalf("duplicate metadata claim = %v, %v", claimed, err)
	}
	removed, err := repo.RemoveSessionMetadataKeyIfState(ctx, "session-session-state", "claim", models.TaskSessionStateRunning)
	if err != nil || removed {
		t.Fatalf("wrong-state metadata removal = %v, %v", removed, err)
	}
	removed, err = repo.RemoveSessionMetadataKeyIfState(ctx, "session-session-state", "claim", models.TaskSessionStateCreated)
	if err != nil || !removed {
		t.Fatalf("RemoveSessionMetadataKeyIfState = %v, %v", removed, err)
	}
	grouped, err := repo.BatchGetSessionsByTaskIDs(ctx, []string{"task-session-state", "missing"})
	if err != nil || len(grouped["task-session-state"]) != 1 || grouped["task-session-state"][0].ID != "session-session-state" {
		t.Fatalf("BatchGetSessionsByTaskIDs = %+v, %v", grouped, err)
	}
	empty, err := repo.BatchGetSessionsByTaskIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("BatchGetSessionsByTaskIDs(nil) = %+v, %v", empty, err)
	}
}
