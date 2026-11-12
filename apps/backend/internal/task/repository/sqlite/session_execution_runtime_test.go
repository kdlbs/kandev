package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestSessionExecutionRuntimeProjection proves full sessions and bounded
// summary observations read the observed runtime from the same
// executors_running row as the execution and container identity.
func TestSessionExecutionRuntimeProjection(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-runtime-projection"

	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Runtime projection"}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		sessionID   string
		runtime     agentruntime.Runtime
		containerID string
		want        string
	}{
		{sessionID: "session-no-inventory", want: ""},
		{sessionID: "session-docker", runtime: agentruntime.RuntimeDocker, containerID: "container-docker", want: "docker"},
		{sessionID: "session-standalone-mismatch", runtime: agentruntime.RuntimeStandalone, want: "standalone"},
	}
	for _, tc := range cases {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: tc.sessionID, TaskID: taskID}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", tc.sessionID, err)
		}
		if tc.runtime != "" {
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "running-" + tc.sessionID, SessionID: tc.sessionID, TaskID: taskID,
				Runtime: tc.runtime, ContainerID: tc.containerID,
				AgentExecutionID: "exec-" + tc.sessionID, Status: "running",
			}); err != nil {
				t.Fatalf("UpsertExecutorRunning(%s): %v", tc.sessionID, err)
			}
		}
	}

	for _, tc := range cases {
		session, err := repo.GetTaskSession(ctx, tc.sessionID)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", tc.sessionID, err)
		}
		if session.ExecutionRuntime != tc.want {
			t.Fatalf("GetTaskSession(%s).ExecutionRuntime = %q, want %q", tc.sessionID, session.ExecutionRuntime, tc.want)
		}
		if tc.runtime == "" && session.AgentExecutionID != "" {
			t.Fatalf("GetTaskSession(%s).AgentExecutionID = %q, want empty", tc.sessionID, session.AgentExecutionID)
		}
	}

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(sessions))
	for _, session := range sessions {
		got[session.ID] = session.ExecutionRuntime
	}
	for _, tc := range cases {
		if got[tc.sessionID] != tc.want {
			t.Fatalf("ListTaskSessions(%s) = %q, want %q", tc.sessionID, got[tc.sessionID], tc.want)
		}
	}

	observations, err := repo.ListTaskSessionSummaryObservations(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	summary := make(map[string]*models.TaskSessionSummaryObservation, len(observations))
	for _, observation := range observations {
		summary[observation.ID] = observation
	}
	for _, tc := range cases {
		observation, ok := summary[tc.sessionID]
		if !ok {
			t.Fatalf("summary missing session %s", tc.sessionID)
		}
		if observation.ExecutionRuntime != tc.want {
			t.Fatalf("summary(%s).ExecutionRuntime = %q, want %q", tc.sessionID, observation.ExecutionRuntime, tc.want)
		}
		if observation.ToTaskSession().ExecutionRuntime != tc.want {
			t.Fatalf("summary(%s).ToTaskSession() lost observed runtime", tc.sessionID)
		}
	}

	// A rotated execution projects the new row's runtime, not the stale one.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-session-standalone-mismatch", SessionID: "session-standalone-mismatch", TaskID: taskID,
		Runtime: agentruntime.RuntimeDocker, ContainerID: "container-rotated",
		AgentExecutionID: "exec-rotated", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	rotated, err := repo.GetTaskSession(ctx, "session-standalone-mismatch")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ExecutionRuntime != "docker" || rotated.AgentExecutionID != "exec-rotated" {
		t.Fatalf("rotated projection = runtime %q execution %q", rotated.ExecutionRuntime, rotated.AgentExecutionID)
	}
}
