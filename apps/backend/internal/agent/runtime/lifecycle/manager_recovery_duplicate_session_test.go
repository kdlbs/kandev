package lifecycle

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestManagerStartSkipsDuplicateSessionAcrossRuntimesDuringRecovery pins
// AC-EXECUTORS-SURVIVAL-002.9: if two different runtime backends each report
// a live, otherwise-reconstructable instance for the very same session ID in
// one Manager.Start recovery pass (the "should not happen... DB consistency
// issue" case manager_lifecycle.go's executionStore.Add call already guards),
// exactly one of them lands in the store -- never two, and never a panic or
// corrupted index. CorrelateRecoveryInstances' per-runtime Winners map cannot
// itself produce this (it is keyed by session ID within one runtime), but
// RecoverAll concatenates every registered runtime's winners without any
// cross-runtime session dedup, so this collision is reachable in practice
// when the same session is (incorrectly) live on two runtimes at once.
func TestManagerStartSkipsDuplicateSessionAcrossRuntimesDuringRecovery(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}

	registry := NewExecutorRegistry(log)
	standaloneBackend := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:           "exec-standalone",
			TaskID:               "task-1",
			SessionID:            "session-1",
			AgentProfileID:       recoveryTestAgentProfileID,
			RuntimeName:          executor.NameStandalone,
			StandaloneInstanceID: "std-inst-1",
		}},
	}
	dockerBackend := &MockExecutor{
		name: executor.NameDocker,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:     "exec-docker",
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: recoveryTestAgentProfileID,
			RuntimeName:    executor.NameDocker,
		}},
	}
	registry.Register(standaloneBackend)
	registry.Register(dockerBackend)

	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	if !ok {
		t.Fatal("expected exactly one of the two colliding instances to be tracked for session-1")
	}
	if execution.ID != "exec-standalone" && execution.ID != "exec-docker" {
		t.Fatalf("tracked execution id = %q, want one of exec-standalone/exec-docker", execution.ID)
	}

	// The loser must never be independently reachable: it was rejected by
	// ExecutionStore.Add before insertion, not raced into the map afterward.
	loserID := "exec-docker"
	if execution.ID == loserID {
		loserID = "exec-standalone"
	}
	if _, ok := mgr.GetExecution(loserID); ok {
		t.Fatalf("loser execution %q was unexpectedly tracked alongside the winner", loserID)
	}

	entries := logs.FilterMessage("skipping duplicate execution during recovery").All()
	if len(entries) != 1 {
		t.Fatalf("duplicate-skip log entries = %d, want exactly 1", len(entries))
	}
}
