package orchestrator

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestDynamicStartupModelEvidenceNeverFallsThroughToPromptRecovery(t *testing.T) {
	tests := []struct {
		name   string
		change func(*dynamicruntime.UnclassifiedFailureEvidence, *routingerr.Error)
	}{
		{"unknown evidence", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) { e.EvidenceKnown = false }},
		{"output", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) { e.OutputObserved = true }},
		{"effect", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) { e.EffectObserved = true }},
		{"stale attempt", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) { e.CurrentAttempt = false }},
		{"other scope", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) { e.TaskScope = false }},
		{"other origin", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) {
			e.Origin = dynamicruntime.UnclassifiedOriginTerminalProvider
		}},
		{"missing typed marker", func(e *dynamicruntime.UnclassifiedFailureEvidence, _ *routingerr.Error) {
			e.StrictStartupModelRejection = false
		}},
		{"generic classification", func(_ *dynamicruntime.UnclassifiedFailureEvidence, c *routingerr.Error) {
			c.ClassifierRule = "generic.model_unavailable"
		}},
		{"fallback forbidden", func(_ *dynamicruntime.UnclassifiedFailureEvidence, c *routingerr.Error) { c.FallbackAllowed = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc Service
			failure, data := startupModelRejection(t)
			svc.beginPromptAttempt(data.SessionID, data.AgentExecutionID, data.PromptGeneration, true)
			data = svc.withDynamicAttemptEvidence(data)
			require.True(t, dynamicPreResultSafe(data))
			classified := unavailableDynamicStartupModel(failure, data)
			require.NotNil(t, classified)
			evidence := dynamicruntime.UnclassifiedFailureEvidence{
				Origin: dynamicruntime.UnclassifiedOriginAgentStartup, StrictStartupModelRejection: true,
				TaskScope: true, CurrentAttempt: true, EvidenceKnown: true,
			}
			_, eligible := svc.prepareDynamicFailureEvidence(context.Background(), data, nil, classified, &evidence, false)
			require.True(t, eligible)
			tt.change(&evidence, classified)
			_, eligible = svc.prepareDynamicFailureEvidence(context.Background(), data, nil, classified, &evidence, false)
			require.False(t, eligible)
		})
	}
}

func TestDynamicStartupObservationsPreserveTypedEvidenceAndRecordedEffects(t *testing.T) {
	tests := []struct {
		name        string
		record      bool
		executionID string
		generation  uint64
		output      bool
		effect      bool
		wantKnown   bool
	}{
		{name: "absent record", wantKnown: true},
		{name: "matching record", record: true, executionID: startupModelExecution, generation: 1, wantKnown: true},
		{name: "recorded output", record: true, executionID: startupModelExecution, generation: 1, output: true, wantKnown: true},
		{name: "recorded effect", record: true, executionID: startupModelExecution, generation: 1, effect: true, wantKnown: true},
		{name: "unbound record", record: true, generation: 1},
		{name: "different execution", record: true, executionID: "replacement-execution", generation: 1},
		{name: "different generation", record: true, executionID: startupModelExecution, generation: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc Service
			_, data := startupModelRejection(t)
			data.EvidenceKnown = true
			if tt.record {
				svc.beginPromptAttempt(data.SessionID, tt.executionID, tt.generation, true)
				svc.observePromptAttempt(data.SessionID, tt.executionID, tt.generation, tt.output, tt.effect)
			}
			got := svc.withStartupAttemptObservations(data)
			require.Equal(t, tt.wantKnown, got.EvidenceKnown)
			require.Equal(t, tt.output, got.OutputObserved)
			require.Equal(t, tt.effect, got.EffectObserved)
		})
	}
}

func TestDynamicStartupAdmissionCannotBindReplacementOrRetiredAttempt(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		var svc Service
		old := svc.beginDynamicAttempt(startupModelSession)
		svc.beginDynamicAttempt(startupModelSession)
		svc.bindDynamicAttemptExecution(startupModelSession, startupModelExecution, old)
		_, data := startupModelRejection(t)
		data.EvidenceKnown = true
		require.False(t, svc.withStartupAttemptObservations(data).EvidenceKnown)
		svc.bindDynamicAttemptExecution(startupModelSession, "replacement-execution")
		data.AgentExecutionID = "replacement-execution"
		require.True(t, svc.withStartupAttemptObservations(data).EvidenceKnown)
	})
	t.Run("retired", func(t *testing.T) {
		var svc Service
		attempt := svc.beginDynamicAttempt(startupModelSession)
		state, release := svc.acquireTransientRetryNoticeState(startupModelSession)
		defer release()
		state.mu.Lock()
		svc.retireTransientRetryNoticeLocked(startupModelSession, state)
		state.mu.Unlock()
		t.Cleanup(func() {
			state.mu.Lock()
			svc.clearTransientRetryNoticeFenceLocked(startupModelSession, state)
			state.mu.Unlock()
		})
		svc.bindDynamicAttemptExecution(startupModelSession, startupModelExecution, attempt)
		_, data := startupModelRejection(t)
		data.EvidenceKnown = true
		require.False(t, svc.withStartupAttemptObservations(data).EvidenceKnown)
		svc.dynamicAttemptEvidence.Delete(startupModelSession)
		require.False(t, svc.withStartupAttemptObservations(data).EvidenceKnown)
	})
}

func TestDynamicStartupAdmissionBindsBeforeProcessStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, startupModelTask, startupModelSession, models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, startupModelTask, v1.TaskStateInProgress)
	manager := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, startupModelSession, "")
	admitted := make(chan bool, 1)
	started := make(chan bool, 1)
	snapshotKnown := func(executionID string) bool {
		return svc.withStartupAttemptObservations(watcher.AgentEventData{
			SessionID: startupModelSession, AgentExecutionID: executionID,
			PromptGeneration: 1, EvidenceKnown: true,
		}).EvidenceKnown
	}
	manager.startAgentProcessFunc = func(_ context.Context, executionID string) error {
		started <- snapshotKnown(executionID)
		return nil
	}
	downstream := &dynamicTaskDownstream{
		service: svc, task: &v1.Task{ID: startupModelTask, WorkspaceID: "ws1"}, sessionID: startupModelSession,
		options: executor.LaunchOptions{
			StartAgent: true, OnExecutionAdmitted: func(executionID string) { admitted <- snapshotKnown(executionID) },
		},
	}
	_, err := downstream.Launch(ctx, dynamicruntime.DownstreamLaunch{
		AttemptID: "startup-attempt", ExecutionProfileID: "candidate-1", Decision: decision,
	})
	require.NoError(t, err)
	select {
	case known := <-admitted:
		require.True(t, known, "caller admission callback ran before the exact attempt was bound")
	case <-time.After(5 * time.Second):
		t.Fatal("caller admission callback was not preserved")
	}
	select {
	case known := <-started:
		require.True(t, known, "agent process started before its execution identity was bound")
	case <-time.After(5 * time.Second):
		t.Fatal("agent process did not start")
	}
	waitForSessionState(t, repo, startupModelSession, models.TaskSessionStateRunning)
}
