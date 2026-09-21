package executor

import (
	"context"
	"errors"
	"expvar"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newSessionCoresidencyTestExecutor is newTestExecutor plus an in-memory log
// sink and direct access to the mock repository, so tests can seed sibling
// sessions and assert on the AC-004 log fields observeSessionCoresidency
// emits.
func newSessionCoresidencyTestExecutor(t *testing.T) (*Executor, *mockRepository, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	repo := newMockRepository()
	exec := NewExecutor(&mockAgentManager{}, repo, log, ExecutorConfig{
		ShellPrefs: &mockShellPrefs{},
	})
	exec.SetCapabilities(&mockCapabilities{})
	return exec, repo, logs
}

// counterValue reads the current int64 stored under key in an expvar.Map, or
// 0 if the key has never been touched. Tests compare before/after deltas
// rather than absolute values because the map is process-global state shared
// across the whole test binary.
func counterValue(m *expvar.Map, key string) int64 {
	v := m.Get(key)
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// TestObserveSessionCoresidency_SolitarySessionRecordsNothing pins
// AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.2: a session starting alone
// on its task leaves both counters untouched and logs nothing.
func TestObserveSessionCoresidency_SolitarySessionRecordsNothing(t *testing.T) {
	exec, repo, logs := newSessionCoresidencyTestExecutor(t)
	repo.sessions["session-123"] = &models.TaskSession{
		ID: "session-123", TaskID: "task-123", State: models.TaskSessionStateStarting,
	}
	before := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch)

	exec.observeSessionCoresidency(context.Background(), sessionCoresidencySiteLaunch, "task-123", "session-123")

	if logs.Len() != 0 {
		t.Fatalf("expected no log entries for a solitary session, got %d: %v", logs.Len(), logs.All())
	}
	if after := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch); after != before {
		t.Fatalf("admitted[launch] counter changed for a solitary session: before=%d after=%d", before, after)
	}
}

// TestObserveSessionCoresidency_WorkingSiblingLogsWarningAndIncrementsCounter
// pins AC-004.1: a session starting while another session of the same task is
// already in a working runtime state records exactly one structured warning
// naming the site, the starting session, and the sibling session IDs, and
// increments the admitted counter once for the admitting site.
func TestObserveSessionCoresidency_WorkingSiblingLogsWarningAndIncrementsCounter(t *testing.T) {
	exec, repo, logs := newSessionCoresidencyTestExecutor(t)
	repo.sessions["session-123"] = &models.TaskSession{
		ID: "session-123", TaskID: "task-123", State: models.TaskSessionStateStarting,
	}
	repo.sessions["session-sibling"] = &models.TaskSession{
		ID: "session-sibling", TaskID: "task-123", State: models.TaskSessionStateRunning,
	}
	before := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch)

	exec.observeSessionCoresidency(context.Background(), sessionCoresidencySiteLaunch, "task-123", "session-123")

	warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warnings) != 1 {
		t.Fatalf("warning entries = %d, want 1; all=%v", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["site"] != sessionCoresidencySiteLaunch {
		t.Fatalf("site field = %v, want %q", fields["site"], sessionCoresidencySiteLaunch)
	}
	if fields["task_id"] != "task-123" || fields["session_id"] != "session-123" {
		t.Fatalf("unexpected identity fields: %+v", fields)
	}
	siblingIDs, ok := fields["sibling_session_ids"].([]interface{})
	if !ok || len(siblingIDs) != 1 || siblingIDs[0] != "session-sibling" {
		t.Fatalf("sibling_session_ids field = %#v, want [session-sibling]", fields["sibling_session_ids"])
	}
	lowerMsg := strings.ToLower(warnings[0].Message)
	if !strings.Contains(lowerMsg, "permit") {
		t.Fatalf("warning message = %q, want wording that reads as permitted rather than a failure to prevent", warnings[0].Message)
	}
	if strings.Contains(lowerMsg, "fail") {
		t.Fatalf("warning message = %q, must not read as Kandev failing to prevent something it permits", warnings[0].Message)
	}
	if after := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch); after != before+1 {
		t.Fatalf("admitted[launch] counter = %d, want %d", after, before+1)
	}
}

// TestObserveSessionCoresidency_SiblingReadFailureRecordsSkipNotAbsence pins
// AC-004.3: a failed sibling-session read records a skip with its reason
// rather than reporting an absence of co-residency, and never blocks the
// caller — observeSessionCoresidency itself has no error return.
func TestObserveSessionCoresidency_SiblingReadFailureRecordsSkipNotAbsence(t *testing.T) {
	exec, repo, logs := newSessionCoresidencyTestExecutor(t)
	readErr := errors.New("transient sibling-session read failure")
	repo.listTaskSessionsFunc = func(context.Context, string) ([]*models.TaskSession, error) {
		return nil, readErr
	}
	admittedBefore := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch)
	skippedBefore := counterValue(sessionCoresidencyObservationSkippedTotalVar, sessionCoresidencySkipReadFailed)

	exec.observeSessionCoresidency(context.Background(), sessionCoresidencySiteLaunch, "task-123", "session-123")

	if after := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch); after != admittedBefore {
		t.Fatalf("admitted[launch] counter changed on a read failure: before=%d after=%d", admittedBefore, after)
	}
	if after := counterValue(sessionCoresidencyObservationSkippedTotalVar, sessionCoresidencySkipReadFailed); after != skippedBefore+1 {
		t.Fatalf("skipped[%s] counter = %d, want %d", sessionCoresidencySkipReadFailed, after, skippedBefore+1)
	}
	warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warnings) != 1 {
		t.Fatalf("warning entries = %d, want 1; all=%v", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["site"] != sessionCoresidencySiteLaunch || fields["task_id"] != "task-123" || fields["session_id"] != "session-123" {
		t.Fatalf("unexpected identity fields: %+v", fields)
	}
}

// TestLaunchPreparedSession_ObservesWorkingSiblingBeforeStartingAgent pins
// the wiring half of AC-004.1: LaunchPreparedSession calls the observation
// before the agent process starts, for a real launch that otherwise
// succeeds, not just the extracted helper.
func TestLaunchPreparedSession_ObservesWorkingSiblingBeforeStartingAgent(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-123"] = &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
	}
	repo.sessions["session-sibling"] = &models.TaskSession{
		ID: "session-sibling", TaskID: "task-123", State: models.TaskSessionStateRunning,
	}

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
			}, nil
		},
	}
	executor := NewExecutor(agentManager, repo, log, ExecutorConfig{ShellPrefs: &mockShellPrefs{}})
	executor.SetCapabilities(&mockCapabilities{})

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}
	before := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch)

	if _, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", LaunchOptions{
		AgentProfileID: "profile-123",
		Prompt:         "test prompt",
		StartAgent:     true,
	}); err != nil {
		t.Fatalf("LaunchPreparedSession failed: %v", err)
	}

	if after := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteLaunch); after != before+1 {
		t.Fatalf("admitted[launch] counter = %d, want %d", after, before+1)
	}
	warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warnings) != 1 {
		t.Fatalf("warning entries = %d, want 1; all=%v", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["site"] != sessionCoresidencySiteLaunch {
		t.Fatalf("site field = %v, want %q", fields["site"], sessionCoresidencySiteLaunch)
	}
}

// TestResumeSession_ObservesWorkingSiblingBeforeStartingAgent pins the
// wiring half of AC-004.1 for the resume seam.
func TestResumeSession_ObservesWorkingSiblingBeforeStartingAgent(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed
	repo.sessions["sess-sibling"] = &models.TaskSession{
		ID: "sess-sibling", TaskID: "task-1", State: models.TaskSessionStateRunning,
	}

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{AgentExecutionID: "exec-new"}, nil
		},
	}
	exec := NewExecutor(agentMgr, repo, log, ExecutorConfig{ShellPrefs: &mockShellPrefs{}})
	exec.SetCapabilities(&mockCapabilities{})
	before := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteResume)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}

	if after := counterValue(sessionCoresidencyAdmittedTotalVar, sessionCoresidencySiteResume); after != before+1 {
		t.Fatalf("admitted[resume] counter = %d, want %d", after, before+1)
	}
	warnings := logs.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warnings) != 1 {
		t.Fatalf("warning entries = %d, want 1; all=%v", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["site"] != sessionCoresidencySiteResume {
		t.Fatalf("site field = %v, want %q", fields["site"], sessionCoresidencySiteResume)
	}
}
