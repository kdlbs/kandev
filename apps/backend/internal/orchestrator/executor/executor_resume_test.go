package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBuildExecutorRunning(t *testing.T) {
	t.Run("basic field mapping", func(t *testing.T) {
		session := &models.TaskSession{
			ID:         "sess-1",
			ExecutorID: "exec-1",
		}
		resp := &LaunchAgentResponse{
			AgentExecutionID: "agent-exec-1",
			ContainerID:      "container-1",
			WorktreeID:       "wt-1",
			WorktreePath:     "/worktrees/wt-1",
			WorktreeBranch:   "kandev/feature",
			Metadata:         map[string]interface{}{"key": "val"},
		}
		cfg := executorConfig{
			RuntimeName: "standalone",
			Resumable:   true,
		}

		running := buildExecutorRunning(session, "task-1", resp, cfg, nil)

		if running.ID != "sess-1" {
			t.Errorf("ID = %q, want %q", running.ID, "sess-1")
		}
		if running.SessionID != "sess-1" {
			t.Errorf("SessionID = %q, want %q", running.SessionID, "sess-1")
		}
		if running.TaskID != "task-1" {
			t.Errorf("TaskID = %q, want %q", running.TaskID, "task-1")
		}
		if running.ExecutorID != "exec-1" {
			t.Errorf("ExecutorID = %q, want %q", running.ExecutorID, "exec-1")
		}
		if running.Runtime != "standalone" {
			t.Errorf("Runtime = %q, want %q", running.Runtime, "standalone")
		}
		if running.Status != "starting" {
			t.Errorf("Status = %q, want %q", running.Status, "starting")
		}
		if !running.Resumable {
			t.Error("Resumable = false, want true")
		}
		if running.AgentExecutionID != "agent-exec-1" {
			t.Errorf("AgentExecutionID = %q, want %q", running.AgentExecutionID, "agent-exec-1")
		}
		if running.ContainerID != "container-1" {
			t.Errorf("ContainerID = %q, want %q", running.ContainerID, "container-1")
		}
		if running.WorktreeID != "wt-1" {
			t.Errorf("WorktreeID = %q, want %q", running.WorktreeID, "wt-1")
		}
		if running.WorktreePath != "/worktrees/wt-1" {
			t.Errorf("WorktreePath = %q, want %q", running.WorktreePath, "/worktrees/wt-1")
		}
		if running.WorktreeBranch != "kandev/feature" {
			t.Errorf("WorktreeBranch = %q, want %q", running.WorktreeBranch, "kandev/feature")
		}
		if running.Metadata["key"] != "val" {
			t.Errorf("Metadata[key] = %v, want %q", running.Metadata["key"], "val")
		}
		// No existing running: resume token should be empty
		if running.ResumeToken != "" {
			t.Errorf("ResumeToken = %q, want empty", running.ResumeToken)
		}
		if running.LastMessageUUID != "" {
			t.Errorf("LastMessageUUID = %q, want empty", running.LastMessageUUID)
		}
	})

	t.Run("carries forward resume token from existing running", func(t *testing.T) {
		session := &models.TaskSession{ID: "sess-1", ExecutorID: "exec-1"}
		resp := &LaunchAgentResponse{
			AgentExecutionID: "agent-exec-2",
			Metadata:         map[string]interface{}{"new_key": "new_val"},
		}
		cfg := executorConfig{RuntimeName: "docker", Resumable: true}
		existing := &models.ExecutorRunning{
			ResumeToken:     "resume-abc",
			LastMessageUUID: "msg-uuid-123",
			Metadata: map[string]interface{}{
				lifecycle.MetadataKeyMainRepoGitDir: "/repo/.git",
				"ephemeral_key":                     "should_not_carry",
			},
		}

		running := buildExecutorRunning(session, "task-1", resp, cfg, existing)

		if running.ResumeToken != "resume-abc" {
			t.Errorf("ResumeToken = %q, want %q", running.ResumeToken, "resume-abc")
		}
		if running.LastMessageUUID != "msg-uuid-123" {
			t.Errorf("LastMessageUUID = %q, want %q", running.LastMessageUUID, "msg-uuid-123")
		}
		// Response had metadata, so existing metadata should NOT be used as fallback
		if running.Metadata["new_key"] != "new_val" {
			t.Errorf("Metadata should come from response, got %v", running.Metadata)
		}
	})

	t.Run("falls back to filtered existing metadata when response has none", func(t *testing.T) {
		session := &models.TaskSession{ID: "sess-1", ExecutorID: "exec-1"}
		resp := &LaunchAgentResponse{
			AgentExecutionID: "agent-exec-3",
			Metadata:         nil, // No metadata from response
		}
		cfg := executorConfig{RuntimeName: "standalone"}
		existing := &models.ExecutorRunning{
			ResumeToken: "token-xyz",
			Metadata: map[string]interface{}{
				// Use a key that's in the persistent set (sprite_name is persistent)
				"sprite_name":                       "my-sprite",
				lifecycle.MetadataKeyMainRepoGitDir: "/repo/.git", // not persistent, should be filtered
			},
		}

		running := buildExecutorRunning(session, "task-1", resp, cfg, existing)

		// When response metadata is nil, should fall back to filtered existing metadata
		if running.Metadata == nil {
			t.Fatal("expected fallback metadata from existing running, got nil")
		}
		if running.Metadata["sprite_name"] != "my-sprite" {
			t.Errorf("expected persistent key 'sprite_name' to be carried forward, got %v", running.Metadata)
		}
		// Non-persistent keys should be filtered out
		if _, ok := running.Metadata[lifecycle.MetadataKeyMainRepoGitDir]; ok {
			t.Error("expected non-persistent key to be filtered out")
		}
	})

	t.Run("nil existing running does not panic", func(t *testing.T) {
		session := &models.TaskSession{ID: "sess-1"}
		resp := &LaunchAgentResponse{AgentExecutionID: "agent-exec-4"}
		cfg := executorConfig{}

		running := buildExecutorRunning(session, "task-1", resp, cfg, nil)

		if running == nil {
			t.Fatal("expected non-nil result")
		}
		if running.ResumeToken != "" {
			t.Errorf("ResumeToken = %q, want empty", running.ResumeToken)
		}
	})
}
