package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.5
func TestRecoverySummaryDistinguishesEmptyEnumerationFromFailure(t *testing.T) {
	t.Run("empty enumeration reports no matching instance", func(t *testing.T) {
		log, logs := newObservedRecoveryManagerLogger(t)
		control := newStandaloneControlServer(t, true)
		control.listInstances = []*agentctlclient.InstanceInfo{} // successful empty inventory

		execRegistry := NewExecutorRegistry(log)
		execRegistry.Register(control.executor(t))
		mgr := NewManager(
			newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
			ExecutorFallbackWarn, "", log,
		)
		mgr.SetAgentSurvivalEnabled(true)
		cleanupManagerStopCh(t, mgr)
		t.Cleanup(func() { _ = mgr.Stop() })
		registerRecoveryTestAgentProfile(t, mgr)
		mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
			{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone},
		}})
		mgr.SetRecoveryDeadline(5 * time.Second)

		err := mgr.Start(context.Background())
		require.NoError(t, err)

		fields := recoveryOutcomeFields(t, logs)
		assertRecoveryCount(t, fields, "candidate_count", 1)
		assertRecoveryCount(t, fields, "retracked_count", 0)
		assertRecoveryCount(t, fields, "not_retracked_count", 1)
		assertRecoveryCount(t, fields, "not_retracked_no_matching_instance", 1)
		assertRecoveryCount(t, fields, "not_retracked_enumeration_failed", 0)
		assertRecoveryCount(t, fields, "not_retracked_unknown", 0)
	})

	t.Run("failed enumeration reports enumeration failed", func(t *testing.T) {
		log, logs := newObservedRecoveryManagerLogger(t)
		control := newStandaloneControlServer(t, false)
		control.listInstancesFailures = 10

		execRegistry := NewExecutorRegistry(log)
		execRegistry.Register(control.executor(t))
		mgr := NewManager(
			newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
			ExecutorFallbackWarn, "", log,
		)
		mgr.SetAgentSurvivalEnabled(true)
		cleanupManagerStopCh(t, mgr)
		t.Cleanup(func() { _ = mgr.Stop() })
		registerRecoveryTestAgentProfile(t, mgr)
		mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
			{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone},
		}})
		mgr.SetRecoveryDeadline(5 * time.Second)

		err := mgr.Start(context.Background())
		require.NoError(t, err)

		fields := recoveryOutcomeFields(t, logs)
		assertRecoveryCount(t, fields, "candidate_count", 1)
		assertRecoveryCount(t, fields, "retracked_count", 0)
		assertRecoveryCount(t, fields, "not_retracked_count", 1)
		assertRecoveryCount(t, fields, "not_retracked_no_matching_instance", 0)
		assertRecoveryCount(t, fields, "not_retracked_enumeration_failed", 1)
		assertRecoveryCount(t, fields, "not_retracked_unknown", 0)
	})
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.5
func TestRecoverySummaryMixedBackendsAccountsEachRecordOnce(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "inst-1", SessionID: "session-1", Port: 4001},
	}

	standaloneExec := control.executor(t)
	otherMock := &MockExecutor{
		name: executor.NameDocker,
	}

	registry := NewExecutorRegistry(log)
	registry.Register(standaloneExec)
	registry.Register(otherMock)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID},
		{TaskID: "task-2", SessionID: "session-2", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID},
		{TaskID: "task-3", SessionID: "session-3", Runtime: agentruntime.RuntimeDocker, ExecutionProfileID: recoveryTestAgentProfileID},
	}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	err := mgr.Start(context.Background())
	require.NoError(t, err)

	fields := recoveryOutcomeFields(t, logs)
	assertRecoveryCount(t, fields, "candidate_count", 3)
	assertRecoveryCount(t, fields, "retracked_count", 1)
	assertRecoveryCount(t, fields, "not_retracked_count", 2)
	assertRecoveryCount(t, fields, "not_retracked_no_matching_instance", 1)
	assertRecoveryCount(t, fields, "not_retracked_enumeration_failed", 0)
	assertRecoveryCount(t, fields, "not_retracked_unknown", 1)
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.5
func TestDuplicateRecoveryResultCannotOverwriteRetrackedOutcome(t *testing.T) {
	log, logs := newObservedRecoveryManagerLogger(t)
	mockBackend := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{
			{
				InstanceID:           "inst-1",
				TaskID:               "task-1",
				SessionID:            "session-1",
				AgentProfileID:       recoveryTestAgentProfileID,
				RuntimeName:          executor.NameStandalone,
				StandaloneInstanceID: "std-1",
			},
			{
				InstanceID:           "inst-2",
				TaskID:               "task-1",
				SessionID:            "session-1",
				AgentProfileID:       recoveryTestAgentProfileID,
				RuntimeName:          executor.NameStandalone,
				StandaloneInstanceID: "std-2",
			},
		},
	}

	registry := NewExecutorRegistry(log)
	registry.Register(mockBackend)

	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{
		{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID},
	}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	err := mgr.Start(context.Background())
	require.NoError(t, err)

	if _, ok := mgr.GetExecutionBySessionID("session-1"); !ok {
		t.Fatal("expected session-1 to be re-tracked in store")
	}

	fields := recoveryOutcomeFields(t, logs)
	assertRecoveryCount(t, fields, "candidate_count", 1)
	assertRecoveryCount(t, fields, "retracked_count", 1)
	assertRecoveryCount(t, fields, "not_retracked_count", 0)
	assertRecoveryCount(t, fields, "not_retracked_duplicate_execution", 0)
	assertRecoveryCount(t, fields, "not_retracked_unknown", 0)
}
