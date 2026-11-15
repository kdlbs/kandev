package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestSessionDTOExecutionRuntime proves observed runtime is additive, nullable,
// and separate from the selected executor fields on both session DTOs.
func TestSessionDTOExecutionRuntime(t *testing.T) {
	t.Run("unknown_is_null", func(t *testing.T) {
		session := &models.TaskSession{ID: "session-1", TaskID: "task-1", ExecutorID: "exec-local-docker"}
		full := FromTaskSession(session)
		if full.ExecutionRuntime != nil {
			t.Fatalf("full ExecutionRuntime = %v, want nil", full.ExecutionRuntime)
		}
		summary := FromTaskSessionSummary(session)
		if summary.ExecutionRuntime != nil {
			t.Fatalf("summary ExecutionRuntime = %v, want nil", summary.ExecutionRuntime)
		}
		encoded, err := json.Marshal(full)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if value, present := decoded["execution_runtime"]; !present || value != nil {
			t.Fatalf("execution_runtime = %v, present = %v; want explicit null", value, present)
		}
	})

	t.Run("observed_runtime_is_additive", func(t *testing.T) {
		session := &models.TaskSession{
			ID: "session-1", TaskID: "task-1",
			ExecutorID: "exec-local-docker", ExecutionRuntime: "docker",
		}
		full := FromTaskSession(session)
		if full.ExecutionRuntime == nil || *full.ExecutionRuntime != "docker" {
			t.Fatalf("full ExecutionRuntime = %v, want docker", full.ExecutionRuntime)
		}
		if full.ExecutorID != "exec-local-docker" {
			t.Fatalf("selected executor changed: %q", full.ExecutorID)
		}
		summary := FromTaskSessionSummary(session)
		if summary.ExecutionRuntime == nil || *summary.ExecutionRuntime != "docker" {
			t.Fatalf("summary ExecutionRuntime = %v, want docker", summary.ExecutionRuntime)
		}
	})

	t.Run("standalone_mismatch_stays_truthful", func(t *testing.T) {
		session := &models.TaskSession{
			ID: "session-1", TaskID: "task-1",
			ExecutorID: "exec-local-docker", ExecutionRuntime: "standalone",
		}
		full := FromTaskSession(session)
		if full.ExecutionRuntime == nil || *full.ExecutionRuntime != "standalone" {
			t.Fatalf("ExecutionRuntime = %v, want standalone", full.ExecutionRuntime)
		}
	})
}
