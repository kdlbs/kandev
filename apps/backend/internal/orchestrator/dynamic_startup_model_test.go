package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	startupModelTask      = "task-startup-model"
	startupModelSession   = "session-startup-model"
	startupModelExecution = "execution-startup-model"
	startupModelProfile   = "dynamic-startup-model"
	startupModelCandidate = "candidate-rejected"
	startupModelNext      = "candidate-next"
)

func startupModelRejection(t *testing.T) (*agentruntime.BootstrapFailure, watcher.AgentEventData) {
	t.Helper()
	promptNotSent := true
	failure := &agentruntime.BootstrapFailure{
		Operation:      models.AgentErrorCauseOperationStart,
		Code:           models.AgentErrorCauseCodeModelUnavailable,
		Reason:         models.AgentErrorCauseReasonRequestedNotAdvertised,
		RequestedModel: "requested-model", PromptNotSent: &promptNotSent,
	}
	cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationStart)
	require.True(t, ok)
	return failure, watcher.AgentEventData{
		TaskID: startupModelTask, SessionID: startupModelSession,
		AgentExecutionID: startupModelExecution, PromptGeneration: 1,
		AgentProfileID: startupModelProfile, ExecutionProfileID: startupModelCandidate,
		Causes: []models.AgentErrorCause{cause},
	}
}

// @covers AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.5
func TestDynamicStartupModelRequiresTypedUnsentStartRejection(t *testing.T) {
	tests := []struct {
		name   string
		change func(*agentruntime.BootstrapFailure, *watcher.AgentEventData)
	}{
		{"missing typed prompt evidence", func(f *agentruntime.BootstrapFailure, _ *watcher.AgentEventData) { f.PromptNotSent = nil }},
		{"sent typed prompt", func(f *agentruntime.BootstrapFailure, _ *watcher.AgentEventData) { *f.PromptNotSent = false }},
		{"missing event prompt evidence", func(_ *agentruntime.BootstrapFailure, d *watcher.AgentEventData) { d.Causes[0].PromptNotSent = nil }},
		{"sent event prompt", func(_ *agentruntime.BootstrapFailure, d *watcher.AgentEventData) { *d.Causes[0].PromptNotSent = false }},
		{"resume", func(_ *agentruntime.BootstrapFailure, d *watcher.AgentEventData) {
			d.Causes[0].Operation = models.AgentErrorCauseOperationResume
		}},
		{"missing requested model", func(f *agentruntime.BootstrapFailure, _ *watcher.AgentEventData) { f.RequestedModel = "" }},
		{"other typed code", func(f *agentruntime.BootstrapFailure, _ *watcher.AgentEventData) {
			f.Code = models.AgentErrorCauseCodeUnknown
		}},
		{"other typed reason", func(f *agentruntime.BootstrapFailure, _ *watcher.AgentEventData) { f.Reason = "other" }},
		{"other event code", func(_ *agentruntime.BootstrapFailure, d *watcher.AgentEventData) {
			d.Causes[0].Code = models.AgentErrorCauseCodeUnknown
		}},
		{"other event reason", func(_ *agentruntime.BootstrapFailure, d *watcher.AgentEventData) { d.Causes[0].Reason = "other" }},
		{"missing event cause", func(_ *agentruntime.BootstrapFailure, d *watcher.AgentEventData) { d.Causes = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure, data := startupModelRejection(t)
			tt.change(failure, &data)
			require.Nil(t, unavailableDynamicStartupModel(failure, data))
		})
	}
	failed, data := startupModelRejection(t)
	require.NotNil(t, unavailableDynamicStartupModel(fmt.Errorf("start: %w", failed), data))
	require.Nil(t, unavailableDynamicStartupModel(errors.New("model unavailable"), data))
	var nilFailure *agentruntime.BootstrapFailure
	require.Nil(t, unavailableDynamicStartupModel(nilFailure, data))
}

type startupModelRoute struct {
	svc        *Service
	repo       *sqliterepo.Repository
	ctx        context.Context
	generation int64
	data       watcher.AgentEventData
	failure    *agentruntime.BootstrapFailure
}

func newStartupModelRoute(t *testing.T, action dynamicruntime.Action) startupModelRoute {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, startupModelTask, startupModelSession, models.TaskSessionStateStarting)
	seedExecutorRunning(t, repo, startupModelSession, startupModelTask, startupModelExecution)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, startupModelTask, v1.TaskStateInProgress)
	manager := &mockAgentManager{stopAgentWithReasonErr: errors.New("predecessor teardown unavailable")}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	t.Cleanup(svc.stopDynamicSuccessorWorkers)
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, startupModelProfile, []workflowDynamicCandidate{
		{executionProfileID: startupModelCandidate, enabled: true, rulesJSON: fmt.Sprintf(`{"model_unavailable":%q}`, action)},
		{executionProfileID: startupModelNext, enabled: true},
	}, dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(resolver)
	initial, err := resolver.Resolve(ctx, startupModelSession, startupModelProfile, 0, "")
	require.NoError(t, err)
	require.Equal(t, startupModelCandidate, initial.ExecutionProfileID)
	session, err := repo.GetTaskSession(ctx, startupModelSession)
	require.NoError(t, err)
	session.AgentProfileID = startupModelProfile
	session.ExecutionProfileID = startupModelCandidate
	session.RouteGeneration = initial.Generation
	session.RouteState = initial.Decision.Status
	session.AgentExecutionID = startupModelExecution
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	svc.lastTurnPrompt.Store(startupModelSession, capturedPrompt{text: "complete the task"})
	svc.beginPromptAttempt(startupModelSession, startupModelExecution, 1, true)
	ctx = withDynamicStartupAttempt(ctx, dynamicruntime.DownstreamLaunch{
		AttemptID: "startup-attempt", ExecutionProfileID: startupModelCandidate, Decision: initial.Decision,
	})
	failure, data := startupModelRejection(t)
	return startupModelRoute{svc: svc, repo: repo, ctx: ctx, generation: initial.Generation, failure: failure, data: data}
}

func (f startupModelRoute) handle() bool {
	var dispatch func(context.Context)
	return f.svc.handleDynamicAgentStartupFailure(
		f.ctx, f.failure, &f.data, startupModelTask, startupModelSession, f.data.AgentExecutionID, &dispatch,
	)
}

// @covers AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.5
func TestDynamicStartupModelUsesConfiguredCandidatePolicy(t *testing.T) {
	for _, action := range []dynamicruntime.Action{dynamicruntime.ActionTryNext, dynamicruntime.ActionStop} {
		t.Run(string(action), func(t *testing.T) {
			f := newStartupModelRoute(t, action)
			handled := f.handle()
			state, err := f.repo.LoadRouteState(f.ctx, startupModelSession)
			require.NoError(t, err)
			require.NotNil(t, state)
			if action == dynamicruntime.ActionTryNext {
				require.True(t, handled)
				require.Equal(t, startupModelNext, state.ExecutionProfileID)
				require.Equal(t, f.generation+1, state.Generation)
			} else {
				require.False(t, handled)
				require.Equal(t, startupModelCandidate, state.ExecutionProfileID)
				require.Equal(t, f.generation, state.Generation)
			}
		})
	}
}

// @covers AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.5
func TestDynamicStartupModelRejectsUnsafeOrStaleAttempt(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *startupModelRoute)
	}{
		{"resume", func(_ *testing.T, f *startupModelRoute) {
			f.data.Causes[0].Operation = models.AgentErrorCauseOperationResume
		}},
		{"unknown delivery", func(_ *testing.T, f *startupModelRoute) { f.failure.PromptNotSent = nil }},
		{"prompt sent", func(_ *testing.T, f *startupModelRoute) { *f.failure.PromptNotSent = false }},
		{"stale execution", func(_ *testing.T, f *startupModelRoute) { f.data.AgentExecutionID = "predecessor-execution" }},
		{"stale generation", func(_ *testing.T, f *startupModelRoute) {
			attempt, ok := dynamicStartupAttemptFromContext(f.ctx)
			require.True(t, ok)
			attempt.Generation++
			f.ctx = context.WithValue(f.ctx, dynamicStartupAttemptContextKey{}, attempt)
		}},
		{"fixed profile", func(t *testing.T, f *startupModelRoute) {
			f.updateSession(t, func(s *models.TaskSession) { s.RouteGeneration = 0; s.AgentProfileID = startupModelCandidate })
		}},
		{"changed logical profile", func(t *testing.T, f *startupModelRoute) {
			f.updateSession(t, func(s *models.TaskSession) { s.AgentProfileID = "replacement-logical-profile" })
		}},
		{"changed concrete profile", func(t *testing.T, f *startupModelRoute) {
			f.updateSession(t, func(s *models.TaskSession) { s.ExecutionProfileID = startupModelNext })
		}},
		{"output observed", func(_ *testing.T, f *startupModelRoute) {
			f.svc.observeDynamicAttempt(startupModelSession, startupModelExecution, true, false)
		}},
		{"effect observed", func(_ *testing.T, f *startupModelRoute) {
			f.svc.observeDynamicAttempt(startupModelSession, startupModelExecution, false, true)
		}},
		{"ambiguous observation", func(_ *testing.T, f *startupModelRoute) {
			f.svc.observeDynamicAttempt(startupModelSession, "", true, false)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStartupModelRoute(t, dynamicruntime.ActionTryNext)
			tt.change(t, &f)
			require.False(t, f.handle())
			state, err := f.repo.LoadRouteState(f.ctx, startupModelSession)
			require.NoError(t, err)
			require.NotNil(t, state)
			require.Equal(t, f.generation, state.Generation)
			require.Equal(t, startupModelCandidate, state.ExecutionProfileID)
		})
	}
}

func (f startupModelRoute) updateSession(t *testing.T, change func(*models.TaskSession)) {
	t.Helper()
	session, err := f.repo.GetTaskSession(f.ctx, startupModelSession)
	require.NoError(t, err)
	change(session)
	require.NoError(t, f.repo.UpdateTaskSession(f.ctx, session))
}
