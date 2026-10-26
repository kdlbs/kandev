package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func TestNomadRetainedOperationFailsClosedOnChangedInputOrMissingHistory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*nomadCreateFixture)
	}{
		{name: "changed worker identity", mutate: func(f *nomadCreateFixture) { f.request.Metadata["node_id"] = "different-node" }},
		{name: "changed workspace input", mutate: func(f *nomadCreateFixture) {
			state, err := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
			if err != nil {
				t.Fatal(err)
			}
			state.WorkspaceInputPath = "/different/original/input"
			if err := nomad.PutEnvironmentState(f.request.Metadata, state); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing workspace input capture", mutate: func(f *nomadCreateFixture) {
			state, err := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
			if err != nil {
				t.Fatal(err)
			}
			state.WorkspaceInputCaptured = false
			state.WorkspaceInputPath = ""
			if err := nomad.PutEnvironmentState(f.request.Metadata, state); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "changed environment generation", mutate: func(f *nomadCreateFixture) { f.request.Metadata["nomad_environment_generation"] = "4" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newNomadCreateFixture(t, false)
			prepareRetainedNomadFixture(t, fixture, "intent")
			tc.mutate(fixture)
			fixture.request.PreviousExecutionID = fixture.request.InstanceID
			_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
			if err == nil || !strings.Contains(err.Error(), "does not match the exact retained operation") {
				t.Fatalf("changed-identity continuation = %v, want exact-owner rejection", err)
			}
			if fixture.registerCalls != 0 || fixture.createCalls != 0 {
				t.Fatalf("changed input caused side effects: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
			}
		})
	}

	t.Run("missing job history", func(t *testing.T) {
		fixture := newNomadCreateFixture(t, false)
		prepareRetainedNomadFixture(t, fixture, "registered")
		fixture.jobHistoryMissing = true
		fixture.request.PreviousExecutionID = fixture.request.InstanceID
		_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
		if err == nil || !strings.Contains(err.Error(), "retained Nomad job is missing") {
			t.Fatalf("missing-history continuation = %v, want unknown retained operation", err)
		}
		if fixture.registerCalls != 0 || fixture.createCalls != 0 {
			t.Fatalf("missing history caused replacement side effects: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
		}
	})
}

func TestNomadRetainedOperationRejectsUnexpectedAllocationSet(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	prepareRetainedNomadFixture(t, fixture, "registered")
	fixture.multipleAllocations = true
	fixture.request.PreviousExecutionID = fixture.request.InstanceID
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err == nil || !strings.Contains(err.Error(), "multiple allocations") {
		t.Fatalf("unexpected allocation set = instance:%+v err:%v, want fail-closed ambiguity", instance, err)
	}
	state, stateErr := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if stateErr != nil || state.Phase != "registered" || state.AllocationID != "" {
		t.Fatalf("unexpected allocation set changed durable ownership: %+v, err=%v", state, stateErr)
	}
	if fixture.registerCalls != 0 || fixture.createCalls != 0 {
		t.Fatalf("unexpected allocation set caused replacement side effects: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
	}
}

func prepareRetainedNomadFixture(t *testing.T, fixture *nomadCreateFixture, phase string) {
	t.Helper()
	fixture.preexistingInstance = true
	fixture.fenceOK = true
	fixture.request.AuthToken = "fixture-instance-token"
	fixture.request.BootstrapNonce = "original-persisted-nonce"
	profile, err := nomadProfileFromMetadata(fixture.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath, err := nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, fixture.request.TaskEnvironmentID, 3)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := nomadLaunchDigest(profile, fixture.request, fixture.request.WorkspacePath, workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{
		EnvironmentID: fixture.request.TaskEnvironmentID, Generation: 3,
		OperationID: fixture.request.InstanceID, InstallationID: "profile-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 2, EnvironmentID: fixture.request.TaskEnvironmentID, Generation: 3,
		OperationID: fixture.request.InstanceID, InstallationID: "profile-1", ExecutionID: fixture.request.InstanceID,
		InputDigest: digest, JobID: jobID, Namespace: profile.Namespace, Region: profile.Region,
		Datacenter: profile.Datacenter, NodeID: profile.NodeID, Platform: profile.Platform,
		SSHHost: profile.SSHHost, SSHUser: profile.SSHUser, SSHPort: profile.SSHPort,
		SSHHostFingerprint: profile.SSHHostFingerprint, WorkspaceRoot: profile.WorkspaceRoot,
		WorkspacePath: workspacePath, WorkspaceInputPath: fixture.request.WorkspacePath, WorkspaceInputCaptured: true,
		AllocationRoot: profile.AllocationRoot,
		CPUMHz:         profile.CPUMHz, MemoryMB: profile.MemoryMB, Phase: phase,
	}
	job := addFixtureJobEvidence(t, profile, &state)
	if phase == "registered" {
		state.JobCreateIndex = 5
		state.JobDefinitionIndex = 7
	}
	if err := nomad.PutEnvironmentState(fixture.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	fixture.job = &nomad.JobRegisterRequest{Job: job}
}

func TestNomadCreateInstanceReattachesRetainedOperationWithoutRegisterOrRestage(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.preexistingInstance = true
	fixture.fenceOK = true
	request := fixture.request
	request.PreviousExecutionID = request.InstanceID
	request.AuthToken = "fixture-instance-token"
	request.BootstrapNonce = "original-persisted-nonce"
	profile, err := nomadProfileFromMetadata(request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := nomadLaunchDigest(profile, request, request.WorkspacePath, filepath.Join(profile.WorkspaceRoot, "environments", request.TaskEnvironmentID, "g3"))
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{
		EnvironmentID: request.TaskEnvironmentID, Generation: 3,
		OperationID: request.InstanceID, InstallationID: "profile-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 1, EnvironmentID: request.TaskEnvironmentID, Generation: 3,
		OperationID: request.InstanceID, InstallationID: "profile-1", ExecutionID: request.InstanceID,
		InputDigest: digest, JobID: jobID, Namespace: profile.Namespace, Region: profile.Region,
		Datacenter: profile.Datacenter, NodeID: profile.NodeID, Platform: profile.Platform,
		SSHHost: profile.SSHHost, SSHUser: profile.SSHUser, SSHPort: profile.SSHPort,
		SSHHostFingerprint: profile.SSHHostFingerprint, WorkspaceRoot: profile.WorkspaceRoot,
		WorkspacePath:      filepath.Join(profile.WorkspaceRoot, "environments", request.TaskEnvironmentID, "g3"),
		WorkspaceInputPath: request.WorkspacePath, WorkspaceInputCaptured: true,
		AllocationRoot: profile.AllocationRoot, CPUMHz: profile.CPUMHz, MemoryMB: profile.MemoryMB,
		Phase: "registered", JobCreateIndex: 5, JobDefinitionIndex: 7,
	}
	job := addFixtureJobEvidence(t, profile, &state)
	if err := nomad.PutEnvironmentState(request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	fixture.job = &nomad.JobRegisterRequest{Job: job}
	instance, err := fixture.runtime.CreateInstance(context.Background(), request)
	if err != nil {
		t.Fatalf("reattach exact retained operation: %v", err)
	}
	if instance == nil || instance.InstanceID != request.InstanceID || instance.AuthToken != request.AuthToken {
		t.Fatalf("reattached instance = %+v", instance)
	}
	fixture.runtime.mu.Lock()
	connection := fixture.runtime.sessions[request.InstanceID]
	fixture.runtime.mu.Unlock()
	if connection == nil || connection.instance != instance {
		t.Fatal("retained operation did not preserve the exact connection owner")
	}
	connection.closeHostResources()
	if fixture.registerCalls != 0 {
		t.Fatalf("retained operation registered %d new Nomad jobs", fixture.registerCalls)
	}
	if fixture.createCalls != 0 {
		t.Fatalf("retained operation created %d duplicate agentctl instances", fixture.createCalls)
	}
	if calls := fixture.sshServer.execCalls(); len(calls) != 1 || !strings.HasPrefix(calls[0].Command, "f=") {
		t.Fatalf("retained operation did more than read its exact owner fence: %#v", calls)
	}
}

func TestNomadReadyContinuationAttachesExistingInstanceWithoutCreatingReplacement(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	prepareRetainedNomadFixture(t, fixture, "ready")
	fixture.request.PreviousExecutionID = fixture.request.InstanceID
	fixture.request.AuthToken = "fixture-instance-token"
	fixture.request.BootstrapNonce = "original-persisted-nonce"
	fixture.request.RequireExistingAgentctlInstance = true
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.JobCreateIndex = 5
	state.JobDefinitionIndex = 7
	state.AllocationID = "alloc-1"
	state.AllocationCreateIndex = 9
	state.ControlPort = 41230
	state.InstancePort = 41231
	state.ServerIdentity = "fixture-agentctl"
	if err := nomad.PutEnvironmentState(fixture.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("attach existing instance for ready-before-ACP continuation: %v", err)
	}
	if instance == nil || instance.InstanceID != fixture.request.InstanceID || instance.WorkspacePath != state.WorkspacePath {
		t.Fatalf("ready continuation returned a different instance: %+v", instance)
	}
	if fixture.registerCalls != 0 || fixture.createCalls != 0 {
		t.Fatalf("ready continuation replaced owned resources: registrations=%d agentctl_creates=%d", fixture.registerCalls, fixture.createCalls)
	}
	fixture.runtime.mu.Lock()
	connection := fixture.runtime.sessions[fixture.request.InstanceID]
	fixture.runtime.mu.Unlock()
	if connection == nil || connection.instance != instance {
		t.Fatal("ready continuation did not retain the exact attached runtime connection")
	}
	connection.closeHostResources()
}

func TestNomadReadyContinuationRejectsMissingInstanceWithoutCreatingReplacement(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	prepareRetainedNomadFixture(t, fixture, "ready")
	fixture.preexistingInstance = false
	fixture.request.PreviousExecutionID = fixture.request.InstanceID
	fixture.request.AuthToken = "fixture-instance-token"
	fixture.request.BootstrapNonce = "original-persisted-nonce"
	fixture.request.RequireExistingAgentctlInstance = true
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.JobCreateIndex = 5
	state.JobDefinitionIndex = 7
	state.AllocationID = "alloc-1"
	state.AllocationCreateIndex = 9
	state.ControlPort = 41230
	state.InstancePort = 41231
	state.ServerIdentity = "fixture-agentctl"
	if err := nomad.PutEnvironmentState(fixture.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.runtime.CreateInstance(context.Background(), fixture.request); err == nil || !strings.Contains(err.Error(), "refusing to recreate during recovery") {
		t.Fatalf("ready continuation with missing agentctl instance = %v, want fail-closed error", err)
	}
	if fixture.registerCalls != 0 || fixture.createCalls != 0 {
		t.Fatalf("missing retained instance caused replacement side effects: registrations=%d agentctl_creates=%d", fixture.registerCalls, fixture.createCalls)
	}
}

func TestNomadReadyContinuationRequiresAttachOnlyFlag(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	prepareRetainedNomadFixture(t, fixture, "ready")
	fixture.request.PreviousExecutionID = fixture.request.InstanceID
	fixture.request.AuthToken = "fixture-instance-token"
	fixture.request.BootstrapNonce = "original-persisted-nonce"
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.JobCreateIndex = 5
	state.JobDefinitionIndex = 7
	state.AllocationID = "alloc-1"
	state.AllocationCreateIndex = 9
	state.ControlPort = 41230
	state.InstancePort = 41231
	state.ServerIdentity = "fixture-agentctl"
	if err := nomad.PutEnvironmentState(fixture.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.runtime.CreateInstance(context.Background(), fixture.request); err == nil || !strings.Contains(err.Error(), "retained operation, owner, input, or phase") {
		t.Fatalf("ready continuation without attach-only flag = %v, want fail-closed phase error", err)
	}
	if fixture.registerCalls != 0 || fixture.createCalls != 0 {
		t.Fatalf("unflagged ready continuation caused replacement side effects: registrations=%d agentctl_creates=%d", fixture.registerCalls, fixture.createCalls)
	}
}

// A crash can happen after Nomad commits the stable job but before Kandev
// checkpoints the registered phase. In that case intent is ambiguous: reconcile
// the exact immutable job and continue it, but never register or stage again.
func TestNomadIntentRetryReconcilesCommittedJobWithoutRegisteringAgain(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.preexistingInstance = true
	fixture.fenceOK = true
	request := fixture.request
	request.PreviousExecutionID = request.InstanceID
	request.AuthToken = "fixture-instance-token"
	request.BootstrapNonce = "original-persisted-nonce"
	profile, err := nomadProfileFromMetadata(request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath, err := nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, request.TaskEnvironmentID, 3)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := nomadLaunchDigest(profile, request, request.WorkspacePath, workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{
		EnvironmentID: request.TaskEnvironmentID, Generation: 3,
		OperationID: request.InstanceID, InstallationID: "profile-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 1, EnvironmentID: request.TaskEnvironmentID, Generation: 3,
		OperationID: request.InstanceID, InstallationID: "profile-1", ExecutionID: request.InstanceID,
		InputDigest: digest, JobID: jobID, Namespace: profile.Namespace, Region: profile.Region,
		Datacenter: profile.Datacenter, NodeID: profile.NodeID, Platform: profile.Platform,
		SSHHost: profile.SSHHost, SSHUser: profile.SSHUser, SSHPort: profile.SSHPort,
		SSHHostFingerprint: profile.SSHHostFingerprint, WorkspaceRoot: profile.WorkspaceRoot,
		WorkspacePath: workspacePath, WorkspaceInputPath: request.WorkspacePath,
		WorkspaceInputCaptured: true, AllocationRoot: profile.AllocationRoot,
		CPUMHz: profile.CPUMHz, MemoryMB: profile.MemoryMB, Phase: "intent",
	}
	job := addFixtureJobEvidence(t, profile, &state)
	if err := nomad.PutEnvironmentState(request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	fixture.job = &nomad.JobRegisterRequest{Job: job}

	instance, err := fixture.runtime.CreateInstance(context.Background(), request)
	if err != nil {
		t.Fatalf("reconcile the original committed job from intent: %v", err)
	}
	if instance == nil || instance.InstanceID != request.InstanceID || instance.AuthToken != request.AuthToken {
		t.Fatalf("continued instance = %+v", instance)
	}
	continued, err := nomad.EnvironmentStateFromMetadata(request.Metadata)
	if err != nil || continued.Phase != "ready" || continued.JobID != jobID || continued.AllocationID != "alloc-1" {
		t.Fatalf("continued state = %+v, err=%v", continued, err)
	}
	if fixture.registerCalls != 0 || fixture.createCalls != 0 {
		t.Fatalf("ambiguous intent retry repeated side effects: registrations=%d instances=%d", fixture.registerCalls, fixture.createCalls)
	}
	if got := len(fixture.sshServer.execCalls()); got != 1 {
		t.Fatalf("intent retry performed %d SSH operations, want exact fence verification only", got)
	}
	fixture.runtime.mu.Lock()
	connection := fixture.runtime.sessions[request.InstanceID]
	fixture.runtime.mu.Unlock()
	if connection == nil {
		t.Fatal("continued original operation did not retain its connection")
	}
	connection.closeHostResources()
}
