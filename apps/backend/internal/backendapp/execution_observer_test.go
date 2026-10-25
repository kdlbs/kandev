package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type observationSource struct {
	nilResult       bool
	waitForDeadline bool
	value           contract.Observation
	err             error
	calls           int
	before          func()
}

func (s *observationSource) Observe(ctx context.Context, org, attempt string) (*contract.Observation, error) {
	s.calls++
	if s.before != nil {
		s.before()
	}
	if org != "org" || attempt != s.value.AttemptID {
		return nil, errors.New("observation scope mismatch")
	}
	if s.waitForDeadline {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.nilResult {
		return nil, nil
	}
	value := s.value
	return &value, s.err
}

func observationTestScope(t *testing.T) (*taskrepo.Repository, *models.ExecutionSnapshot, models.ExecutionSnapshot) {
	t.Helper()
	h := newBootStateTestHarness(t)
	require.NoError(t, h.taskRepo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace", Name: "Workspace", OrgID: "org"}))
	require.NoError(t, h.taskRepo.CreateTask(t.Context(), &models.Task{ID: "task", Title: "Task", WorkspaceID: "workspace"}))
	require.NoError(t, h.taskRepo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateRunning}))
	require.NoError(t, h.taskRepo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "execution", Status: models.ExecutorRunningStatusRunning}))
	input := models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: "session", TaskID: "task", ExecutionID: "execution", Agent: launchwrapper.SummarizeCommand([]string{"agent"}), Sandbox: &models.ExecutionSandboxSnapshot{
		AttemptID: "attempt", Generation: 1, InstallationID: "installation", ProviderKey: "fixture", ProviderVersion: "1", PackageDigest: strings.Repeat("a", 64), RuntimeVersion: "1", RuntimeDigest: strings.Repeat("b", 64), RevisionID: "policy", RevisionNumber: 1, PolicyName: "Policy", PolicyDigest: strings.Repeat("c", 64), SelectionSource: "task", CeilingRevisionID: "ceiling", CeilingDigest: strings.Repeat("d", 64),
	}}
	snapshot, err := h.taskRepo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	return h.taskRepo, snapshot, input
}

func activeObservation() contract.Observation {
	now := time.Now().UTC().Truncate(time.Millisecond)
	until := now.Add(30 * time.Second)
	return contract.Observation{AttemptID: "attempt", Generation: 1, AttemptVersion: 3, Status: "active", ProviderHealth: "available", Mechanism: "fixture-kernel", CheckedAt: now, EvidenceAt: &now, ValidUntil: &until}
}

func TestExecutionObservationRefreshAndFailure(t *testing.T) {
	tasks, snapshot, _ := observationTestScope(t)
	source := &observationSource{value: activeObservation()}
	published := 0
	observer, err := newExecutionObserver(t.Context(), tasks, source, func(ctx context.Context, value *models.ExecutionSnapshot) error {
		require.Equal(t, snapshot, value)
		session, readErr := tasks.GetTaskSession(ctx, value.SessionID)
		require.NoError(t, readErr)
		require.NotNil(t, session.ExecutionObservation)
		require.Equal(t, models.TaskSessionStateRunning, session.State)
		published++
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, observer.refresh(t.Context()))
	require.Equal(t, 1, published)
	session, err := tasks.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, "active", session.ExecutionObservation.Status)
	source.err = errors.New("private-provider-credential")
	require.Error(t, observer.refresh(t.Context()))
	require.Equal(t, 2, published, "failed observation must publish unknown immediately")
	session, err = tasks.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, "unknown", session.ExecutionObservation.Status)
	require.Nil(t, session.ExecutionObservation.ValidUntil)
	require.Equal(t, source.value.EvidenceAt, session.ExecutionObservation.EvidenceAt)
	raw, err := json.Marshal(session.ToAPI())
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-provider-credential")
	source.err = nil
	source.value = contract.Observation{AttemptID: "attempt", Generation: 1, AttemptVersion: 4, Status: "stopped", ProviderHealth: "unknown", CheckedAt: time.Now().UTC()}
	require.NoError(t, observer.refresh(t.Context()))
	require.Equal(t, 3, published)
	require.NoError(t, observer.refresh(t.Context()))
	require.Equal(t, 3, source.calls, "confirmed terminal attempts must leave the observer queue")
}

func TestExecutionObservationBackendReplacementWithoutProvider(t *testing.T) {
	tasks, _, _ := observationTestScope(t)
	source := &observationSource{value: activeObservation()}
	published := 0
	first, err := newExecutionObserver(t.Context(), tasks, source, func(context.Context, *models.ExecutionSnapshot) error { published++; return nil })
	require.NoError(t, err)
	require.NoError(t, first.refresh(t.Context()))
	require.Equal(t, 1, published)
	replacement, err := newExecutionObserver(t.Context(), tasks, nil, nil)
	require.NoError(t, err)
	require.NoError(t, replacement.refresh(t.Context()))
	session, err := tasks.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, "unknown", session.ExecutionObservation.Status, "disabled provider must still invalidate previous backend evidence")
	require.Error(t, first.refresh(t.Context()))
	require.Equal(t, 1, published, "old observer must not publish")
}

func TestExecutionObservationAttemptChangesDuringRead(t *testing.T) {
	tasks, _, input := observationTestScope(t)
	source := &observationSource{value: activeObservation()}
	var next *models.ExecutionSnapshot
	source.before = func() {
		input.AttemptID, input.Sandbox.AttemptID = "replacement", "replacement"
		var err error
		next, err = tasks.CaptureExecutionSnapshot(t.Context(), input)
		require.NoError(t, err)
	}
	published := 0
	observer, err := newExecutionObserver(t.Context(), tasks, source, func(context.Context, *models.ExecutionSnapshot) error { published++; return nil })
	require.NoError(t, err)
	require.Error(t, observer.refresh(t.Context()))
	require.Zero(t, published)
	session, err := tasks.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, next, session.ExecutionSnapshot)
	require.Nil(t, session.ExecutionObservation)
	source.before = nil
	source.value.AttemptID = "replacement"
	require.NoError(t, observer.refresh(t.Context()))
	require.Equal(t, 1, published)
}

func TestExecutionObservationNilAndDeadlineInvalidateLiveStatus(t *testing.T) {
	for _, scenario := range []string{"nil", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			tasks, _, _ := observationTestScope(t)
			source := &observationSource{value: activeObservation()}
			observer, err := newExecutionObserver(t.Context(), tasks, source, nil)
			require.NoError(t, err)
			require.NoError(t, observer.refresh(t.Context()))
			source.nilResult = scenario == "nil"
			source.waitForDeadline = scenario == "deadline"
			require.Error(t, observer.refresh(t.Context()))
			session, err := tasks.GetTaskSession(t.Context(), "session")
			require.NoError(t, err)
			require.Equal(t, "unknown", session.ExecutionObservation.Status)
			require.Nil(t, session.ExecutionObservation.ValidUntil)
			require.Equal(t, source.value.EvidenceAt, session.ExecutionObservation.EvidenceAt)
		})
	}
}
