package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskSessionSummaryProjectionPreservesCompactFieldsAndErrorClearing(t *testing.T) {
	startedAt := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	observation := &models.TaskSessionSummaryObservation{
		ID: "session-summary-dto", TaskID: "task-summary-dto", QueueIncarnationID: "incarnation-dto",
		Name: "Review", AgentExecutionID: "execution-dto", ContainerID: "container-dto",
		AgentProfileID: "agent-dto", ExecutionProfileID: "profile-dto", RouteGeneration: 3,
		RouteState: "committed", RouteReason: "selected", ExecutorID: "executor-dto",
		ExecutorProfileID: "executor-profile-dto", EnvironmentID: "environment-dto",
		RepositoryID: "repository-dto", RepositoryPath: "/repo/task", BaseBranch: "main",
		BaseCommitSHA: "abc123", WorkspacePath: "/workspace/task", State: models.TaskSessionStateFailed,
		ErrorMessage: "launch failed", Metadata: map[string]interface{}{models.SessionMetaKeyLastAgentError: nil},
		StartedAt: startedAt, UpdatedAt: startedAt.Add(time.Minute), IsPrimary: true,
		ReviewStatus: models.ReviewStatusPending, LastReadMessageID: "message-dto",
		Worktrees: []*models.TaskEnvironmentRepo{{
			ID: "association-dto", WorktreeID: "worktree-dto",
			RepositoryID: "repository-dto", WorktreePath: "/repo/task", WorktreeBranch: "review/task",
		}},
	}
	got := FromTaskSessionSummaryObservation(observation)
	if got.ID != observation.ID || got.TaskID != observation.TaskID || got.Name != observation.Name ||
		got.State != observation.State || !got.IsPrimary || got.ErrorMessage != observation.ErrorMessage ||
		got.ReviewStatus != observation.ReviewStatus || got.LastReadMessageID != observation.LastReadMessageID {
		t.Fatalf("compact summary fields = %+v", got)
	}
	if got.WorktreeID != "worktree-dto" || got.WorktreePath != "/repo/task" || got.WorktreeBranch != "review/task" {
		t.Fatalf("worktree fields = %+v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal summary DTO: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode summary DTO: %v", err)
	}
	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata projection = %#v, want a compact object", payload["metadata"])
	}
	if value, exists := metadata[models.SessionMetaKeyLastAgentError]; !exists || value != nil {
		t.Fatalf("last_agent_error = %#v, present=%v; want explicit null to clear a retained error", value, exists)
	}
	if len(metadata) != 1 {
		t.Fatalf("metadata contains unrelated keys: %v", metadata)
	}
}
