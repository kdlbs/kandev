package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestNomadCreateCapturesOriginalEmptyWorkspaceInputBeforeResolvingRuntimePath(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.request.WorkspacePath = ""
	if _, err := fixture.runtime.CreateInstance(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !state.WorkspaceInputCaptured || state.WorkspaceInputPath != "" {
		t.Fatalf("initial Nomad intent captured the wrong original input: captured=%t path=%q", state.WorkspaceInputCaptured, state.WorkspaceInputPath)
	}
	fixture.runtime.mu.Lock()
	connection := fixture.runtime.sessions[fixture.request.InstanceID]
	fixture.runtime.mu.Unlock()
	if connection == nil {
		t.Fatal("executor did not retain the captured operation connection")
	}
	t.Cleanup(connection.closeHostResources)
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"workspace_input_path":""`) || !strings.Contains(string(encoded), `"workspace_input_captured":true`) {
		t.Fatalf("initial Nomad intent did not preserve captured empty input: %s", encoded)
	}
	if state.WorkspacePath == "" {
		t.Fatal("resolved Nomad workspace path is empty")
	}
}

func TestNomadCreateReconcilesCommittedRegistrationAfterResponseLoss(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.loseRegisterResponse = true

	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("CreateInstance() after committed registration response loss: %v", err)
	}
	if instance == nil || instance.RuntimeName != agentruntime.RuntimeNomad || instance.InstanceID != fixture.request.InstanceID {
		t.Fatalf("CreateInstance() = %+v, want original connected operation", instance)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil || state.Phase != "ready" || state.JobID == "" || state.AllocationID != "alloc-1" {
		t.Fatalf("reconciled state = %+v, err=%v", state, err)
	}
	if fixture.registerCalls != 1 || fixture.createCalls != 1 {
		t.Fatalf("lost registration response repeated side effects: registrations=%d instance_creates=%d", fixture.registerCalls, fixture.createCalls)
	}
	if got := len(fixture.checkpoints); got != 6 {
		t.Fatalf("durable checkpoints = %d, want intent + immutable job evidence + registered + allocation + connection + ready", got)
	}
	fixture.runtime.mu.Lock()
	connection := fixture.runtime.sessions[fixture.request.InstanceID]
	fixture.runtime.mu.Unlock()
	if connection == nil || connection.instance != instance {
		t.Fatal("reconciled registration did not retain the exact connected operation")
	}
	connection.closeHostResources()
}

func TestNomadCreateFailureRetainsDurableIntentWithoutClaimingAllocation(t *testing.T) {
	fixture := newNomadCreateFixture(t, true)
	fixture.jobHistoryMissing = true // the 5xx answer left no job behind
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err == nil || !strings.Contains(err.Error(), "did not reconcile") {
		t.Fatalf("CreateInstance() error = %v, want unreconciled registration failure", err)
	}
	if len(fixture.checkpoints) != 3 || fixture.checkpoints[0].Phase != "intent" || fixture.checkpoints[1].NativeJobSpecSHA256 == "" {
		t.Fatalf("failed registration did not retain durable intent and the cleanup record: %+v", fixture.checkpoints)
	}
	state, decodeErr := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if decodeErr != nil || state.Phase != "cleanup_pending" || state.AllocationID != "" {
		t.Fatalf("failed registration was reported as allocated or lost its cleanup record: %+v, err=%v", state, decodeErr)
	}
	fixture.runtime.mu.Lock()
	_, retained := fixture.runtime.sessions[fixture.request.InstanceID]
	fixture.runtime.mu.Unlock()
	if retained {
		t.Fatal("failed create retained an in-memory connected session")
	}
}

func TestNomadStagingBarrierRemainsUnknownAndDoesNotRegisterOnRetry(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.failBootstrapWrite = true
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err == nil || !strings.Contains(err.Error(), "stage private Nomad bootstrap environment") {
		t.Fatalf("CreateInstance() error = %v, want interrupted private staging", err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil || state.Phase != "intent" || state.NativeJobSpecSHA256 == "" {
		t.Fatalf("staging interruption lost its durable intent: %+v, err=%v", state, err)
	}
	if fixture.registerCalls != 0 {
		t.Fatalf("staging interruption registered %d jobs before the dispatch boundary", fixture.registerCalls)
	}

	// A later process cannot infer from a missing job response whether the
	// prior process crashed before dispatch or lost the registration response.
	// Retain the operation as unknown rather than silently registering again.
	fixture.failBootstrapWrite = false
	fixture.request.PreviousExecutionID = fixture.request.InstanceID
	_, err = fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err == nil || !strings.Contains(err.Error(), "retained Nomad job is missing") {
		t.Fatalf("retry of staged intent = %v, want fail-closed unknown operation", err)
	}
	if fixture.registerCalls != 0 || fixture.createCalls != 0 {
		t.Fatalf("unknown staging operation repeated dispatch: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
	}
}

func TestNomadRegisteredAllocationBarrierRetainsCleanupPending(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.allocationsPending = true
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := fixture.runtime.CreateInstance(ctx, fixture.request)
	if err == nil || !strings.Contains(err.Error(), "wait for exact running Nomad allocation") {
		t.Fatalf("CreateInstance() at pending allocation barrier = %v, want bounded wait", err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil || state.Phase != nomadPhaseCleanupPending || state.JobCreateIndex == 0 || state.JobDefinitionIndex == 0 || state.AllocationID != "" {
		t.Fatalf("allocation wait did not retain the exact registered operation for cleanup: %+v, err=%v", state, err)
	}
	if fixture.registerCalls != 1 || fixture.createCalls != 0 {
		t.Fatalf("pending allocation crossed extra launch boundaries: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
	}
}

func TestNomadBootstrapBarrierRetainsCleanupPendingAndRejectsRetry(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.rejectBootstrap = true
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err == nil || !strings.Contains(err.Error(), "bootstrap credential rejected") {
		t.Fatalf("CreateInstance() at bootstrap barrier = %v, want rejected one-use credential", err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil || state.Phase != "cleanup_pending" || state.AllocationID != "alloc-1" {
		t.Fatalf("bootstrap failure did not retain uncertain allocation: %+v, err=%v", state, err)
	}
	if fixture.registerCalls != 1 || fixture.createCalls != 0 {
		t.Fatalf("bootstrap barrier has unexpected side effects: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
	}

	fixture.rejectBootstrap = false
	fixture.request.PreviousExecutionID = fixture.request.InstanceID
	_, retryErr := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if retryErr == nil || !strings.Contains(retryErr.Error(), "does not match the exact retained operation") {
		t.Fatalf("bootstrap cleanup-pending retry = %v, want fail-closed non-resurrection", retryErr)
	}
	if fixture.registerCalls != 1 || fixture.createCalls != 0 {
		t.Fatalf("bootstrap retry duplicated a job or instance: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
	}
}
