package sqlite

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotImmutableCaptureAndOwnership(t *testing.T) {
	runExecutionSnapshotCapture(t, newRepoForSessionTests(t))
}

func TestExecutionSnapshotPostgresCaptureAndOwnership(t *testing.T) {
	runExecutionSnapshotCapture(t, newPostgresRetentionTestRepo(t))
}

func runExecutionSnapshotCapture(t *testing.T, repo *Repository) {
	t.Helper()
	seedExecutorRunningCleanupTask(t, repo, "snapshot-task")
	seedExecutorRunningCleanupSession(t, repo, "snapshot-task", "snapshot-session", models.TaskSessionStateStarting, "execution-one")
	wrapper, err := launchwrapper.Profile([]string{"/private/wrapper", "secret-value", launchwrapper.WorkDir}).Summary()
	require.NoError(t, err)
	input := models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt-one", SessionID: "snapshot-session", TaskID: "snapshot-task", ExecutionID: "execution-one", ExecutorType: "local", AgentProfileID: "profile", Transport: "acp", Wrapper: wrapper, Agent: launchwrapper.SummarizeCommand([]string{"/private/agent", "credential"})}
	first, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, uint64(1), first.Sequence)
	require.Equal(t, uint64(1), first.Generation)
	require.Equal(t, "executor-1", first.ExecutorID)
	require.False(t, first.CapturedAt.IsZero())
	raw, err := json.Marshal(first)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "/private")
	require.NotContains(t, string(raw), "secret-value")
	require.NotContains(t, string(raw), "credential")
	repeated, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, first, repeated)
	changed := input
	changed.AgentProfileID = "changed-profile"
	_, err = repo.CaptureExecutionSnapshot(t.Context(), changed)
	require.ErrorIs(t, err, errExecutionSnapshotConflict)
	input.AttemptID = "attempt-two"
	input.Wrapper = nil
	second, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, uint64(2), second.Sequence)
	require.Equal(t, uint64(2), second.Generation)
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "snapshot-session", SessionID: "snapshot-session", TaskID: "snapshot-task", ExecutorID: "executor-1", Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting, AgentExecutionID: "execution-two"}))
	input.AttemptID = "obsolete-attempt"
	_, err = repo.CaptureExecutionSnapshot(t.Context(), input)
	require.ErrorIs(t, err, errExecutionSnapshotConflict)
	input.ExecutionID = "execution-two"
	input.AttemptID = "attempt-three"
	third, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, uint64(3), third.Sequence)
	require.Equal(t, uint64(1), third.Generation)
	require.NoError(t, repo.initExecutionSnapshotSchema())
	repeated, err = repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, third, repeated)
}

func TestExecutionSnapshotRejectsPrivateOrInvalidInput(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedExecutorRunningCleanupTask(t, repo, "snapshot-task")
	seedExecutorRunningCleanupSession(t, repo, "snapshot-task", "snapshot-session", models.TaskSessionStateStarting, "execution")
	valid := models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: "snapshot-session", TaskID: "snapshot-task", ExecutionID: "execution", ExecutorType: "local", Transport: "acp", Agent: launchwrapper.SummarizeCommand([]string{"agent"})}
	cases := map[string]func(*models.ExecutionSnapshot){
		"version":                 func(v *models.ExecutionSnapshot) { v.SchemaVersion = 2 },
		"missing attempt":         func(v *models.ExecutionSnapshot) { v.AttemptID = "" },
		"private executable":      func(v *models.ExecutionSnapshot) { v.Agent.Executable = "/private/agent" },
		"opaque argument":         func(v *models.ExecutionSnapshot) { v.Agent.References = []string{"secret-value"} },
		"negative count":          func(v *models.ExecutionSnapshot) { v.Agent.OmittedArguments = -1 },
		"wrapper source":          func(v *models.ExecutionSnapshot) { v.Wrapper = &launchwrapper.Summary{Source: "secret"} },
		"incomplete sandbox":      func(v *models.ExecutionSnapshot) { v.Sandbox = &models.ExecutionSandboxSnapshot{AttemptID: "sandbox"} },
		"foreign sandbox attempt": func(v *models.ExecutionSnapshot) { v.Sandbox = observationSnapshotInput().Sandbox },
		"caller generation":       func(v *models.ExecutionSnapshot) { v.Generation = 9 },
	}
	ordinal := 0
	for name, mutate := range cases {
		ordinal++
		t.Run(name, func(t *testing.T) {
			input := valid
			input.AttemptID = fmt.Sprintf("invalid-%d", ordinal)
			mutate(&input)
			_, err := repo.CaptureExecutionSnapshot(t.Context(), input)
			require.Error(t, err)
		})
	}
	var count int
	require.NoError(t, repo.db.QueryRow(`SELECT COUNT(*) FROM agent_execution_snapshots`).Scan(&count))
	require.Zero(t, count)
	_, err := repo.CaptureExecutionSnapshot(t.Context(), valid)
	require.NoError(t, err)
}

func TestExecutionSnapshotConcurrentCapturePostgres(t *testing.T) {
	repo := newPostgresRetentionTestRepo(t)
	seedExecutorRunningCleanupTask(t, repo, "snapshot-task")
	seedExecutorRunningCleanupSession(t, repo, "snapshot-task", "snapshot-session", models.TaskSessionStateStarting, "execution")
	const writers = 8
	results := make(chan *models.ExecutionSnapshot, writers)
	errors := make(chan error, writers)
	start := make(chan struct{})
	var joined sync.WaitGroup
	for index := range writers {
		joined.Go(func() {
			<-start
			result, err := repo.CaptureExecutionSnapshot(t.Context(), models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: fmt.Sprintf("attempt-%d", index), SessionID: "snapshot-session", TaskID: "snapshot-task", ExecutionID: "execution", ExecutorType: "local", Transport: "acp", Agent: launchwrapper.SummarizeCommand([]string{"agent"})})
			results <- result
			errors <- err
		})
	}
	close(start)
	joined.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	sequences := make(map[uint64]bool)
	for result := range results {
		require.Equal(t, result.Sequence, result.Generation)
		require.False(t, sequences[result.Sequence])
		sequences[result.Sequence] = true
	}
	require.Len(t, sequences, writers)
}
