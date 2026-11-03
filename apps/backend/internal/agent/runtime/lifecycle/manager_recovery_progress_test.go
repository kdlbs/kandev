package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

func newRecoveryProgressReporter(t *testing.T) (*startup.Reporter, *observer.ObservedLogs) {
	t.Helper()
	reporter, logs := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)
	return reporter, logs
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-001.21
// @covers AC-PLATFORM-STARTUP-PROGRESS-001.22
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7
func TestRecoveryProgressCountsConfirmedAbsence(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{}

	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(control.executor(t))
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID},
	}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	reporter, stepLogs := newRecoveryProgressReporter(t)
	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, fields, "done", 1)
	assertRecoveryCount(t, fields, "total", 1)
	if got := fields["measure"]; got != "counted" {
		t.Fatalf("measure = %v, want counted", got)
	}
	summary := recoveryOutcomeFields(t, logs)
	assertRecoveryCount(t, summary, "retracked_count", 0)
	assertRecoveryCount(t, summary, "not_retracked_no_matching_instance", 1)
	for _, entry := range stepLogs.All() {
		if entry.Message == "Startup step ended below its total" &&
			entry.ContextMap()["step"] == string(startup.StepSessionsRecovery) {
			t.Fatalf("confirmed absence must not end below total: %+v", entry.ContextMap())
		}
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-001.21
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7
func TestRecoveryProgressMixedResolvedCandidates(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	instance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"capabilities":["launch-wrapper.v1"]}`))
		case "/api/v1/agent/launch-wrapper":
			_, _ = w.Write([]byte(`{"version":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(instance.Close)

	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "inst-1", SessionID: "session-1", Port: instance.Listener.Addr().(*net.TCPAddr).Port},
	}

	standalone := control.executor(t)
	refusing := &MockExecutor{
		name: executor.NameDocker,
		recoverInstances: []*ExecutorInstance{
			{InstanceID: "inst-3", SessionID: "session-3", RuntimeName: executor.NameDocker},
			{InstanceID: "inst-1", SessionID: "session-1", RuntimeName: executor.NameDocker},
		},
	}
	registry := NewExecutorRegistry(log)
	registry.Register(standalone)
	registry.Register(refusing)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID},
		{TaskID: "task-2", SessionID: "session-2", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID},
		{TaskID: "task-3", SessionID: "session-3", Runtime: agentruntime.RuntimeDocker},
	}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	reporter, stepLogs := newRecoveryProgressReporter(t)
	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, fields, "done", 3)
	assertRecoveryCount(t, fields, "total", 3)
	summary := recoveryOutcomeFields(t, logs)
	assertRecoveryCount(t, summary, "candidate_count", 3)
	assertRecoveryCount(t, summary, "retracked_count", 1)
	assertRecoveryCount(t, summary, "not_retracked_no_matching_instance", 1)
	assertRecoveryCount(t, summary, "not_retracked_task_identity", 1)
	assertRecoveryCount(t, summary, "not_retracked_unknown", 0)
	for _, entry := range stepLogs.All() {
		if entry.Message == "Startup step ended below its total" &&
			entry.ContextMap()["step"] == string(startup.StepSessionsRecovery) {
			t.Fatalf("mixed conclusive candidates must not end below total: %+v", entry.ContextMap())
		}
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-001.22
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7
func TestRecoveryProgressRetainsIncompleteOutcomes(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	failing := &MockExecutor{name: executor.NameDocker, recoverErr: context.DeadlineExceeded}
	expired := &MockExecutor{
		name: executor.NameLocal,
		recoverInstances: []*ExecutorInstance{
			{InstanceID: "inst-2", TaskID: "task-2", SessionID: "session-2", RuntimeName: executor.NameLocal},
		},
	}
	omitted := &MockExecutor{name: executor.NameKubernetes}

	registry := NewExecutorRegistry(log)
	registry.Register(failing)
	registry.Register(expired)
	registry.Register(omitted)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeDocker},
		{TaskID: "task-2", SessionID: "session-2", Runtime: agentruntime.Runtime("local")},
		{TaskID: "task-3", SessionID: "session-3", Runtime: agentruntime.RuntimeKubernetes},
	}})
	mgr.SetRecoveryDeadlineStart(time.Now().Add(-time.Hour))
	mgr.SetRecoveryDeadline(time.Millisecond)

	reporter, stepLogs := newRecoveryProgressReporter(t)
	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, fields, "done", 0)
	assertRecoveryCount(t, fields, "total", 3)
	summary := recoveryOutcomeFields(t, logs)
	assertRecoveryCount(t, summary, "not_retracked_enumeration_failed", 1)
	assertRecoveryCount(t, summary, "not_retracked_deadline", 1)
	assertRecoveryCount(t, summary, "not_retracked_unknown", 1)
	var sawBelowTotal bool
	for _, entry := range stepLogs.All() {
		if entry.Message == "Startup step ended below its total" &&
			entry.ContextMap()["step"] == string(startup.StepSessionsRecovery) {
			sawBelowTotal = true
		}
	}
	if !sawBelowTotal {
		t.Fatalf("incomplete recovery must keep the below-total warning: fields=%+v", fields)
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-001.21
func TestRecoveryProgressDuplicateReturnedInstanceCountsOnce(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	duplicate := &ExecutorInstance{
		InstanceID:  "inst-1",
		SessionID:   "session-1",
		RuntimeName: executor.NameStandalone,
	}
	backend := &MockExecutor{
		name:             executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{duplicate, duplicate},
	}
	mgr := newRecoverySummaryManager(t, log, backend)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1"},
	}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	reporter, stepLogs := newRecoveryProgressReporter(t)
	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, fields, "done", 1)
	assertRecoveryCount(t, fields, "total", 1)
	assertRecoveryCount(t, recoveryOutcomeFields(t, logs), "candidate_count", 1)
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-001.21
func TestRecoveryProgressPublishesBeforeStopWorkersFinish(t *testing.T) {
	log, _ := newObservedRecoveryManagerLogger(t)
	control := newStandaloneControlServer(t, true)
	control.listInstances = nil
	blocking := &deadlineRecoveringExecutor{
		name:        executor.NameLocal,
		stopStarted: make(chan struct{}),
		allowStop:   make(chan struct{}),
		recovered: []*ExecutorInstance{{
			InstanceID:  "inst-blocked",
			SessionID:   "session-blocked",
			RuntimeName: executor.NameLocal,
		}},
	}
	registry := NewExecutorRegistry(log)
	registry.Register(control.executor(t))
	registry.Register(blocking)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-absent", SessionID: "session-absent", Runtime: agentruntime.RuntimeStandalone},
		{TaskID: "task-blocked", SessionID: "session-blocked", Runtime: agentruntime.Runtime("local")},
	}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	reporter, stepLogs := newRecoveryProgressReporter(t)
	done := make(chan error, 1)
	go func() { done <- mgr.Start(startup.WithReporter(context.Background(), reporter)) }()

	select {
	case <-blocking.stopStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked stop worker did not start")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		snapshot := reporter.Snapshot()
		if snapshot.Step != nil && snapshot.Step.Done != nil && *snapshot.Step.Done >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("completed candidate was not published while the stop worker was blocked: %+v", snapshot.Step)
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(blocking.allowStop)
	if err := <-done; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, fields, "done", 2)
	assertRecoveryCount(t, fields, "total", 2)
}

type failingSettingsReader struct{ ExecutorProfileReader }

func (failingSettingsReader) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, context.DeadlineExceeded
}

type failingRecoveryProfileResolver struct{}

func (failingRecoveryProfileResolver) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	return nil, context.DeadlineExceeded
}

type noopSettingsWriter struct{}

func (noopSettingsWriter) SetSessionMetadataKey(context.Context, string, string, interface{}) error {
	return nil
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7
func TestRecoveryProgressRetainsOperationalReconstructionFailures(t *testing.T) {
	t.Run("task environment read failure", func(t *testing.T) {
		log, _ := newObservedRecoveryManagerLogger(t)
		backend := &MockExecutor{
			name: executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{{
				InstanceID:     "inst-env",
				TaskID:         "task-env",
				SessionID:      "session-env",
				AgentProfileID: recoveryTestAgentProfileID,
				RuntimeName:    executor.NameStandalone,
			}},
		}
		mgr := newRecoverySummaryManager(t, log, backend)
		registerRecoveryTestAgentProfile(t, mgr)
		mgr.SetWorkspaceInfoProvider(&fakeTaskEnvironmentProvider{err: context.DeadlineExceeded})
		mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{
			TaskID: "task-env", SessionID: "session-env", ExecutionProfileID: recoveryTestAgentProfileID,
		}}})
		mgr.SetRecoveryDeadline(5 * time.Second)

		reporter, stepLogs := newRecoveryProgressReporter(t)
		if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
		assertRecoveryCount(t, fields, "done", 0)
		assertRecoveryCount(t, fields, "total", 1)
	})

	t.Run("durable delivery evidence failure", func(t *testing.T) {
		log, _ := newObservedRecoveryManagerLogger(t)
		backend := &MockExecutor{
			name: executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{{
				InstanceID:            "inst-delivery",
				TaskID:                "task-delivery",
				SessionID:             "session-delivery",
				AgentProfileID:        recoveryTestAgentProfileID,
				RuntimeName:           executor.NameStandalone,
				DeliveryRecoveryError: context.Canceled,
			}},
		}
		mgr := newRecoverySummaryManager(t, log, backend)
		registerRecoveryTestAgentProfile(t, mgr)
		mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{
			TaskID: "task-delivery", SessionID: "session-delivery", ExecutionProfileID: recoveryTestAgentProfileID,
		}}})
		mgr.SetRecoveryDeadline(5 * time.Second)

		reporter, stepLogs := newRecoveryProgressReporter(t)
		if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
		assertRecoveryCount(t, fields, "done", 0)
		assertRecoveryCount(t, fields, "total", 1)
	})

	t.Run("agent profile read failure", func(t *testing.T) {
		log, _ := newObservedRecoveryManagerLogger(t)
		backend := &MockExecutor{
			name: executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{{
				InstanceID:     "inst-agent",
				TaskID:         "task-agent",
				SessionID:      "session-agent",
				AgentProfileID: "missing-profile",
				RuntimeName:    executor.NameStandalone,
			}},
		}
		mgr := newRecoverySummaryManager(t, log, backend)
		mgr.profileResolver = failingRecoveryProfileResolver{}
		mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{
			TaskID: "task-agent", SessionID: "session-agent", ExecutionProfileID: "missing-profile",
		}}})
		mgr.SetRecoveryDeadline(5 * time.Second)

		reporter, stepLogs := newRecoveryProgressReporter(t)
		if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
		assertRecoveryCount(t, fields, "done", 0)
		assertRecoveryCount(t, fields, "total", 1)
	})

	t.Run("settings snapshot read failure", func(t *testing.T) {
		log, _ := newObservedRecoveryManagerLogger(t)
		backend := &MockExecutor{
			name: executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{{
				InstanceID:     "inst-settings",
				TaskID:         "task-settings",
				SessionID:      "session-settings",
				AgentProfileID: recoveryTestAgentProfileID,
				RuntimeName:    executor.NameStandalone,
			}},
		}
		mgr := newRecoverySummaryManager(t, log, backend)
		registerRecoveryTestAgentProfile(t, mgr)
		mgr.executorProfileReader = failingSettingsReader{}
		mgr.SetSessionSettingsSnapshotWriter(noopSettingsWriter{})
		mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{
			TaskID: "task-settings", SessionID: "session-settings", ExecutionProfileID: recoveryTestAgentProfileID,
		}}})
		mgr.SetRecoveryDeadline(5 * time.Second)

		reporter, stepLogs := newRecoveryProgressReporter(t)
		if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
		assertRecoveryCount(t, fields, "done", 0)
		assertRecoveryCount(t, fields, "total", 1)
	})
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7
func TestRecoveryProgressRetainsRemoteTransportFailure(t *testing.T) {
	const (
		sessionID   = "session-remote-transport"
		taskID      = "task-remote-transport"
		executionID = "execution-remote-transport"
	)
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusReady,
	}
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NameSSH},
		recoveryResult: func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			return []*ExecutorInstance{{
				InstanceID: executionID, SessionID: sessionID, TaskID: taskID, RuntimeName: executor.NameSSH,
			}}, nil, nil
		},
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})
	task08SetSnapshots(mgr, []*models.ExecutorRunning{row})
	mgr.SetPassthroughLookup(alwaysNotPassthrough)
	mgr.SetRecoveryDeadline(5 * time.Second)
	t.Cleanup(func() { _ = mgr.Stop() })

	reporter, stepLogs := newRecoveryProgressReporter(t)
	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, fields, "done", 0)
	assertRecoveryCount(t, fields, "total", 1)
	if err := mgr.recoveryGuard.CheckLaunchAllowed(sessionID); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("remote transport failure released its recovery guard: %v", err)
	}
}
