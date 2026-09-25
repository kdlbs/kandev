package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

// newObservedSessionsRecoveryReporter builds a startup.Reporter whose own
// logger is independent of the Manager's -- Manager.Start's
// startup.BeginStep/Advance/Degrade/EndStep calls all go through the
// context-carried Reporter, not m.logger, so observing it needs no changes
// to how the Manager under test is constructed.
func newObservedSessionsRecoveryReporter(t *testing.T) (*startup.Reporter, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	return startup.New(log), logs
}

// completedStepFields returns the ContextMap of the last "Startup step
// completed" entry for wantStep, or fails the test if there is none.
func completedStepFields(t *testing.T, logs *observer.ObservedLogs, wantStep startup.StepID) map[string]interface{} {
	t.Helper()
	var fields map[string]interface{}
	for _, entry := range logs.All() {
		if entry.Message != "Startup step completed" {
			continue
		}
		if entry.ContextMap()["step"] != string(wantStep) {
			continue
		}
		fields = entry.ContextMap()
	}
	if fields == nil {
		t.Fatalf("no \"Startup step completed\" entry for step %q", wantStep)
	}
	return fields
}

func newObservedRecoveryManagerLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	return log, logs
}

func recoveryOutcomeFields(t *testing.T, logs *observer.ObservedLogs) map[string]interface{} {
	t.Helper()
	entries := logs.FilterMessage("startup session recovery summary").All()
	if len(entries) != 1 {
		t.Fatalf("recovery summary entries = %d, want one", len(entries))
	}
	return entries[0].ContextMap()
}

func assertRecoveryCount(t *testing.T, fields map[string]interface{}, name string, want int) {
	t.Helper()
	if got := fmt.Sprint(fields[name]); got != fmt.Sprint(want) {
		t.Fatalf("%s = %v, want %d", name, fields[name], want)
	}
}

func newRecoverySummaryManager(t *testing.T, log *logger.Logger, backend *MockExecutor) *Manager {
	t.Helper()
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	return mgr
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
func TestManagerStartLogsRecoveryOutcomeSummaryForZeroCandidates(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	mgr := newRecoverySummaryManager(t, log, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{}})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fields := recoveryOutcomeFields(t, logs)
	if fields["candidate_count_known"] != true {
		t.Fatalf("candidate_count_known = %v, want true for a successful empty inventory", fields["candidate_count_known"])
	}
	for name, want := range map[string]int{
		"candidate_count": 0, "retracked_count": 0, "not_retracked_count": 0,
		"not_retracked_deadline": 0, "not_retracked_task_identity": 0,
		"not_retracked_task_environment": 0, "not_retracked_agent_identity": 0,
		"not_retracked_turn_status": 0, "not_retracked_duplicate_execution": 0,
		"not_retracked_unknown": 0,
	} {
		assertRecoveryCount(t, fields, name, want)
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
func TestManagerStartMarksCandidateCountUnknownWhenInventoryReadFails(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	mgr := newRecoverySummaryManager(t, log, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&listingWriter{err: errors.New("inventory read failed")})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fields := recoveryOutcomeFields(t, logs)
	if fields["candidate_count_known"] != false {
		t.Fatalf("candidate_count_known = %v, want false after inventory read failure", fields["candidate_count_known"])
	}
	assertRecoveryCount(t, fields, "candidate_count", 0)
	assertRecoveryCount(t, fields, "not_retracked_count", 0)
	assertRecoveryCount(t, fields, "not_retracked_unknown", 0)
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
func TestManagerStartCountsBackendOmissionsAsUnknownRecoveryOutcomes(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	mgr := newRecoverySummaryManager(t, log, &MockExecutor{name: executor.NameStandalone})
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-1"}}})
	reporter, stepLogs := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)

	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fields := recoveryOutcomeFields(t, logs)
	if fields["candidate_count_known"] != true {
		t.Fatalf("candidate_count_known = %v, want true", fields["candidate_count_known"])
	}
	assertRecoveryCount(t, fields, "candidate_count", 1)
	assertRecoveryCount(t, fields, "retracked_count", 0)
	assertRecoveryCount(t, fields, "not_retracked_count", 1)
	assertRecoveryCount(t, fields, "not_retracked_unknown", 1)
	stepFields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, stepFields, "done", 0)
	assertRecoveryCount(t, stepFields, "total", 1)
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
func TestManagerStartSeparatesRetrackedAndKnownRefusalOutcomes(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	backend := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{
			newRecoveryTurnOutcomeExecutorInstance(),
			{InstanceID: "instance-missing-task", SessionID: "session-2", RuntimeName: executor.NameStandalone},
		},
	}
	mgr := newRecoverySummaryManager(t, log, backend)
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1"},
		{TaskID: "task-2", SessionID: "session-2"},
	}})
	reporter, stepLogs := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)

	if err := mgr.Start(startup.WithReporter(context.Background(), reporter)); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fields := recoveryOutcomeFields(t, logs)
	assertRecoveryCount(t, fields, "candidate_count", 2)
	assertRecoveryCount(t, fields, "retracked_count", 1)
	assertRecoveryCount(t, fields, "not_retracked_count", 1)
	assertRecoveryCount(t, fields, "not_retracked_task_identity", 1)
	assertRecoveryCount(t, fields, "not_retracked_unknown", 0)
	if _, ok := mgr.GetExecutionBySessionID("session-1"); !ok {
		t.Fatal("the valid recovered session was not re-tracked")
	}
	stepFields := completedStepFields(t, stepLogs, startup.StepSessionsRecovery)
	assertRecoveryCount(t, stepFields, "done", 2)
	assertRecoveryCount(t, stepFields, "total", 2)
}

// TestManagerStartRecoverySessionsStepReportsCountedProgress covers the
// sessions.recovery half of AC-PLATFORM-STARTUP-PROGRESS-005.2/.8 on the
// ordinary (successful inventory read) path: the step totals from the
// post-recoverableRecords record count, and the consumer loop's Advance(1)
// call fires even on a record that is immediately refused (empty TaskID),
// since that record was still consumed from the corpus that SetTotal
// counted.
func TestManagerStartRecoverySessionsStepReportsCountedProgress(t *testing.T) {
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	backend := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:  "instance-1",
			SessionID:   "session-1",
			TaskID:      "", // refused immediately (AC-EXECUTORS-SURVIVAL-002.4), but still one unit of progress
			RuntimeName: executor.NameStandalone,
		}},
	}
	registry.Register(backend)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-1"}}})

	reporter, logs := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)
	ctx := startup.WithReporter(context.Background(), reporter)

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if snap := reporter.Snapshot(); snap.Step != nil {
		t.Fatalf("Step after Start = %+v, want nil (step closed)", snap.Step)
	}

	fields := completedStepFields(t, logs, startup.StepSessionsRecovery)
	if got := fields["measure"]; got != "counted" {
		t.Fatalf("measure = %v, want \"counted\": a successful non-empty inventory read must promote past opaque", got)
	}
	if got, ok := fields["done"].(int64); !ok || got != 1 {
		t.Fatalf("done = %v, want 1", fields["done"])
	}
	if got, ok := fields["total"].(int64); !ok || got != 1 {
		t.Fatalf("total = %v, want 1", fields["total"])
	}
}

// TestManagerStartRecoverySessionsStepReportsZeroTotalOnEmptyInventory
// covers the successful-but-empty boundary: a SUCCESSFUL read returning no
// records is authoritative (TestManagerStartEmptyRecoveryInventoryStillStopsOrphans
// pins the orphan-stop side of this same scenario), so SetTotal(0) reports a
// known-empty corpus rather than the unreadable path's Degrade.
func TestManagerStartRecoverySessionsStepReportsZeroTotalOnEmptyInventory(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", SessionID: "session-1"},
	}

	mgr := newLivenessTestManager(t, control)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	reporter, logs := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)
	ctx := startup.WithReporter(context.Background(), reporter)

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	for _, entry := range logs.All() {
		if entry.Message == "Startup step warning" && entry.ContextMap()["step"] == string(startup.StepSessionsRecovery) && entry.ContextMap()["condition"] == "degrade" {
			t.Fatalf("unexpected degrade warning on an authoritative empty inventory: %+v", entry.ContextMap())
		}
	}
	fields := completedStepFields(t, logs, startup.StepSessionsRecovery)
	if got := fields["measure"]; got != "opaque" {
		t.Fatalf("measure = %v, want \"opaque\": SetTotal(0) seals the activation opaque", got)
	}
	if got, ok := fields["total"].(int64); !ok || got != 0 {
		t.Fatalf("total = %v, want 0", fields["total"])
	}
}

func TestManagerStartKeepsRecoveryStepUntilStopWorkersFinish(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	backend := &deadlineRecoveringExecutor{
		name:        executor.NameStandalone,
		stopStarted: make(chan struct{}),
		allowStop:   make(chan struct{}),
		recovered: []*ExecutorInstance{{
			InstanceID:           "exec-1",
			TaskID:               "task-1",
			SessionID:            "session-1",
			RuntimeName:          executor.NameStandalone,
			StandaloneInstanceID: "standalone-1",
		}},
	}
	execRegistry.Register(backend)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{SessionID: "session-1"}}})
	mgr.SetRecoveryDeadlineStart(time.Now().Add(-time.Hour))
	mgr.SetRecoveryDeadline(time.Millisecond)

	reporter, _ := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)
	ctx := startup.WithReporter(context.Background(), reporter)
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()

	select {
	case <-backend.stopStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery stop worker did not start")
	}

	if snap := reporter.Snapshot(); snap.Step == nil {
		t.Fatal("recovery step ended before the stop worker finished")
	}

	close(backend.allowStop)
	if err := <-done; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if snap := reporter.Snapshot(); snap.Step != nil {
		t.Fatalf("Step after Start = %+v, want nil (step closed)", snap.Step)
	}
}

// TestManagerStartRecoverySessionsStepLocksOpaqueOnUnreadableInventory
// covers the AC-EXECUTORS-SURVIVAL-002.6/002.12 unreadable-inventory path
// this same file's sibling manager_recovery_inventory_unknown_test.go pins
// the orphan-stop behavior for: the corpus is unknown, not empty, so the
// step never promotes past opaque and Degrade logs a "degrade" warning
// distinguishing this boot from a genuinely empty one.
func TestManagerStartRecoverySessionsStepLocksOpaqueOnUnreadableInventory(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", SessionID: "session-1"},
	}

	mgr := newLivenessTestManager(t, control)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{err: errors.New("database unavailable")})
	mgr.SetRecoveryDeadline(5 * time.Second)

	reporter, logs := newObservedSessionsRecoveryReporter(t)
	reporter.Set(startup.RecoveringSessions)
	ctx := startup.WithReporter(context.Background(), reporter)

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var sawDegradeWarning bool
	for _, entry := range logs.All() {
		if entry.Message != "Startup step warning" {
			continue
		}
		if entry.ContextMap()["step"] == string(startup.StepSessionsRecovery) && entry.ContextMap()["condition"] == "degrade" {
			sawDegradeWarning = true
		}
	}
	if !sawDegradeWarning {
		t.Fatal("expected a \"Startup step warning\" entry with condition=degrade for an unreadable inventory")
	}

	fields := completedStepFields(t, logs, startup.StepSessionsRecovery)
	if got := fields["measure"]; got != "opaque" {
		t.Fatalf("measure = %v, want \"opaque\": an unreadable inventory must never promote past opaque", got)
	}
	if got, ok := fields["total"].(int64); !ok || got != 0 {
		t.Fatalf("total = %v, want 0 (SetTotal is never called on this path)", fields["total"])
	}
}
