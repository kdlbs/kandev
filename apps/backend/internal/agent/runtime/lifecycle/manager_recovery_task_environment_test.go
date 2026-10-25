package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
)

// fakeTaskEnvironmentProvider is a minimal WorkspaceInfoProvider double
// scoped to hydrateRecoveredTaskEnvironmentID's own tests.
type fakeTaskEnvironmentProvider struct {
	info *WorkspaceInfo
	err  error
}

func (p *fakeTaskEnvironmentProvider) GetWorkspaceInfoForSession(context.Context, string, string) (*WorkspaceInfo, error) {
	return p.info, p.err
}

func (p *fakeTaskEnvironmentProvider) GetWorkspaceInfoForEnvironment(context.Context, string) (*WorkspaceInfo, error) {
	return p.info, p.err
}

// TestHydrateRecoveredTaskEnvironmentIDNilProviderIsANoOp pins that an
// unwired provider (every production backend wires one; only bare-bones test
// doubles don't) is treated as a capability this deployment never had, not a
// failed read -- same convention as every other optional recovery capability.
func TestHydrateRecoveredTaskEnvironmentIDNilProviderIsANoOp(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{TaskID: "task-1", SessionID: "session-1"}

	if err := mgr.hydrateRecoveredTaskEnvironmentID(context.Background(), execution); err != nil {
		t.Fatalf("hydrateRecoveredTaskEnvironmentID: %v, want nil with no provider configured", err)
	}
	if execution.TaskEnvironmentID != "" {
		t.Fatalf("TaskEnvironmentID = %q, want empty", execution.TaskEnvironmentID)
	}
}

// TestHydrateRecoveredTaskEnvironmentIDReturnsErrorOnLookupFailure pins
// Review round 3, finding 4: a lookup failure must propagate as an error so
// the caller can refuse to re-track the instance (AC-EXECUTORS-SURVIVAL-002.13),
// rather than being logged and silently ignored as an earlier version of this
// method did.
func TestHydrateRecoveredTaskEnvironmentIDReturnsErrorOnLookupFailure(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetWorkspaceInfoProvider(&fakeTaskEnvironmentProvider{err: errors.New("db unavailable")})
	execution := &AgentExecution{TaskID: "task-1", SessionID: "session-1"}

	if err := mgr.hydrateRecoveredTaskEnvironmentID(context.Background(), execution); err == nil {
		t.Fatal("want an error when the lookup itself fails")
	}
}

// TestHydrateRecoveredTaskEnvironmentIDReturnsErrorWhenAnsweredEmpty pins the
// AC-EXECUTORS-SURVIVAL-002.4 half of finding 4: the durable store answering
// successfully with no task-environment identity at all is authoritatively
// absent, not "nothing to hydrate."
func TestHydrateRecoveredTaskEnvironmentIDReturnsErrorWhenAnsweredEmpty(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetWorkspaceInfoProvider(&fakeTaskEnvironmentProvider{info: &WorkspaceInfo{TaskEnvironmentID: ""}})
	execution := &AgentExecution{TaskID: "task-1", SessionID: "session-1"}

	if err := mgr.hydrateRecoveredTaskEnvironmentID(context.Background(), execution); err == nil {
		t.Fatal("want an error when the durable store answers with no task-environment identity")
	}
}

// TestHydrateRecoveredTaskEnvironmentIDReturnsErrorWhenInfoIsNil covers the
// same authoritatively-absent case for a nil *WorkspaceInfo, not just an
// empty field on a non-nil one.
func TestHydrateRecoveredTaskEnvironmentIDReturnsErrorWhenInfoIsNil(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetWorkspaceInfoProvider(&fakeTaskEnvironmentProvider{info: nil, err: nil})
	execution := &AgentExecution{TaskID: "task-1", SessionID: "session-1"}

	if err := mgr.hydrateRecoveredTaskEnvironmentID(context.Background(), execution); err == nil {
		t.Fatal("want an error when the durable store answers with a nil info and no error")
	}
}

// TestHydrateRecoveredTaskEnvironmentIDSetsValueOnSuccess pins the happy
// path still works after the fix: a populated answer sets the field and
// returns no error.
func TestHydrateRecoveredTaskEnvironmentIDSetsValueOnSuccess(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetWorkspaceInfoProvider(&fakeTaskEnvironmentProvider{info: &WorkspaceInfo{TaskEnvironmentID: "env-1"}})
	execution := &AgentExecution{TaskID: "task-1", SessionID: "session-1"}

	if err := mgr.hydrateRecoveredTaskEnvironmentID(context.Background(), execution); err != nil {
		t.Fatalf("hydrateRecoveredTaskEnvironmentID: %v, want nil", err)
	}
	if execution.TaskEnvironmentID != "env-1" {
		t.Fatalf("TaskEnvironmentID = %q, want env-1", execution.TaskEnvironmentID)
	}
}

// TestManagerStartRefusesRecoveredExecutionWhenTaskEnvironmentIdentityCannotBeReconstructed
// pins the full Manager.Start wiring: a recovered instance whose
// task-environment identity cannot be reconstructed (durable lookup fails)
// must never be published as tracked, and is instead routed to the
// AC-EXECUTORS-SURVIVAL-002.6 stop path, exactly like a failed agent-identity
// or turn-outcome reconstruction already is.
func TestManagerStartRefusesRecoveredExecutionWhenTaskEnvironmentIdentityCannotBeReconstructed(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	mockExec := &MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:  "exec-bad-env",
			TaskID:      "task-1",
			SessionID:   "session-bad-env",
			RuntimeName: executor.NameStandalone,
		}},
	}
	execRegistry.Register(mockExec)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = mgr.Stop() })
	mgr.SetWorkspaceInfoProvider(&fakeTaskEnvironmentProvider{err: errors.New("db unavailable")})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, ok := mgr.GetExecution("exec-bad-env"); ok {
		t.Fatal("an execution whose task-environment identity could not be reconstructed must not be tracked")
	}
	if len(mockExec.stopInstanceCalls) != 1 || mockExec.stopInstanceCalls[0].InstanceID != "exec-bad-env" {
		t.Fatalf("stopInstanceCalls = %+v, want the unreconstructable instance stopped", mockExec.stopInstanceCalls)
	}
}
