package lifecycle

import "testing"

func TestExecutorInstanceCarriesExactProfileSnapshot(t *testing.T) {
	execution := (&ExecutorInstance{InstanceID: "execution-1"}).ToAgentExecution(&ExecutorCreateRequest{
		TaskID:               "task-1",
		SessionID:            "session-1",
		ExactProfile:         true,
		ExactProfileModel:    "gpt-5.6-codex",
		ExactProfileRevision: 1726500000000000000,
	})

	if !execution.ExactProfile || execution.ExactProfileModel != "gpt-5.6-codex" || execution.ExactProfileRevision != 1726500000000000000 {
		t.Fatalf("exact profile snapshot = %+v, want exact model and revision", execution)
	}
}
