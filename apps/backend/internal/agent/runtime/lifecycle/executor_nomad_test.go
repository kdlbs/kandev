package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestNomadExecutorRequiresRuntimeDependenciesAndFailsClosed(t *testing.T) {
	runtime := NewNomadExecutor(nil, nil, nil)
	if runtime.Name() != agentruntime.RuntimeNomad {
		t.Fatalf("Name() = %q, want nomad", runtime.Name())
	}
	if runtime.IsAlwaysResumable() != true || runtime.RequiresCloneURL() != true || runtime.ShouldApplyPreferredShell() {
		t.Fatalf("Nomad runtime capabilities are inconsistent with remote host execution")
	}
	if err := runtime.HealthCheck(context.Background()); !errors.Is(err, ErrNomadExecutorNotReady) {
		t.Fatalf("HealthCheck() = %v, want not-ready", err)
	}
	if _, err := runtime.CreateInstance(context.Background(), &ExecutorCreateRequest{}); !errors.Is(err, ErrNomadExecutorNotReady) {
		t.Fatalf("CreateInstance() = %v, want not-ready without unmanaged fallback", err)
	}
	if _, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{{Runtime: agentruntime.RuntimeNomad}}); !errors.Is(err, ErrNomadExecutorNotReady) {
		t.Fatalf("RecoverInstances() = %v, want not-ready", err)
	}
}

func TestNomadExecutorOrdinaryStopClosesOnlyMatchingLocalTransport(t *testing.T) {
	listener := func(t *testing.T) *SSHPortForwarder {
		t.Helper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return &SSHPortForwarder{listener: ln, localPort: ln.Addr().(*net.TCPAddr).Port, closed: make(chan struct{})}
	}

	firstForward := listener(t)
	secondForward := listener(t)
	first := nomadStopTestInstance("operation-1", "task-1", "session-1", "allocation-1")
	first.StopReason = "stopped via API"
	second := nomadStopTestInstance("operation-1", "task-1", "session-1", "allocation-stale")
	connection := &nomadAgentctlConnection{instance: first, controlForwarder: firstForward}
	runtime := NewNomadExecutor(nil, nil, nil)
	runtime.sessions[first.InstanceID] = connection
	var nomadCalls atomic.Int32
	runtime.newNomadClient = func(nomad.ProfileConfig, string) (*nomad.Client, error) {
		nomadCalls.Add(1)
		return nil, errors.New("unexpected Nomad call")
	}

	if err := runtime.StopInstance(context.Background(), second, false); err == nil {
		t.Fatal("stale instance identity unexpectedly detached the current Nomad transport")
	}
	conn, err := net.DialTimeout("tcp", firstForward.listener.Addr().String(), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("stale stop closed the current listener: %v", err)
	}
	_ = conn.Close()

	if err := runtime.StopInstance(context.Background(), first, false); err != nil {
		t.Fatalf("ordinary StopInstance(): %v", err)
	}
	if _, err := net.DialTimeout("tcp", firstForward.listener.Addr().String(), 10*time.Millisecond); err == nil {
		t.Fatal("ordinary stop left its local transport listener open")
	}
	if got := len(runtime.sessions); got != 0 {
		t.Fatalf("session map still contains %d entries after stop", got)
	}
	if err := runtime.StopInstance(context.Background(), first, false); err != nil {
		t.Fatalf("duplicate ordinary StopInstance() should be idempotent: %v", err)
	}
	if got := nomadCalls.Load(); got != 0 {
		t.Fatalf("ordinary stop made %d Nomad API calls, want none", got)
	}
	_ = secondForward.Close()
}

func TestNomadExecutorForceStopFailsClosedAndCloseOnlyDetaches(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	forward := &SSHPortForwarder{listener: ln, localPort: ln.Addr().(*net.TCPAddr).Port, closed: make(chan struct{})}
	instance := nomadStopTestInstance("operation-2", "task-2", "session-2", "allocation-2")
	runtime := NewNomadExecutor(nil, nil, nil)
	runtime.sessions[instance.InstanceID] = &nomadAgentctlConnection{instance: instance, controlForwarder: forward}
	instance.StopReason = StopReasonSessionArchived

	if err := runtime.StopInstance(context.Background(), instance, true); !errors.Is(err, ErrNomadExecutorNotReady) {
		t.Fatalf("force StopInstance() = %v, want fail-closed error", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 10*time.Millisecond); err != nil {
		t.Fatalf("force stop closed transport despite unsupported destructive cleanup: %v", err)
	}
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("force stop unexpectedly detached the session: %v", err)
	}
	_ = conn.Close()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 10*time.Millisecond); err == nil {
		t.Fatal("backend Close() left the local transport listener open")
	}
	if got := len(runtime.sessions); got != 0 {
		t.Fatalf("session map still contains %d entries after Close(), want 0", got)
	}
}

func TestNomadExecutorDestructiveStopReasonFailsClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	forward := &SSHPortForwarder{listener: ln, localPort: ln.Addr().(*net.TCPAddr).Port, closed: make(chan struct{})}
	instance := nomadStopTestInstance("operation-delete", "task-delete", "session-delete", "allocation-delete")
	instance.StopReason = StopReasonSessionArchived
	runtime := NewNomadExecutor(nil, nil, nil)
	runtime.sessions[instance.InstanceID] = &nomadAgentctlConnection{instance: instance, controlForwarder: forward}

	if err := runtime.StopInstance(context.Background(), instance, false); !errors.Is(err, ErrNomadExecutorNotReady) {
		t.Fatalf("destructive StopInstance() = %v, want fail-closed error", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 10*time.Millisecond); err != nil {
		t.Fatalf("destructive stop closed transport before cleanup was supported: %v", err)
	}
	_ = forward.Close()
}

func nomadStopTestInstance(executionID, taskID, sessionID, allocationID string) *ExecutorInstance {
	metadata := map[string]interface{}{}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 7, EnvironmentID: "environment-" + executionID, Generation: 2,
		OperationID: executionID, InstallationID: "profile-1", ExecutionID: executionID,
		InputDigest: strings.Repeat("a", 64), JobID: "job-" + executionID, Namespace: "default", Region: "global",
		Datacenter: "dc1", NodeID: "node-1", Platform: "darwin/arm64", SSHHost: "worker.example.test",
		SSHUser: "runner", SSHPort: 22, SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot: "/workspace", WorkspacePath: "/workspace/environments/environment-" + executionID + "/g2",
		AllocationRoot: "/allocations", CPUMHz: 1500, MemoryMB: 4096, AllocationID: allocationID,
		AllocationCreateIndex: 11, ControlPort: 22341, InstancePort: 41234, ServerIdentity: "server-1", Phase: "ready",
	}
	if err := nomad.PutEnvironmentState(metadata, state); err != nil {
		panic(err)
	}
	metadata[MetadataKeyAuthTokenSecret] = "auth-secret-reference"
	metadata[MetadataKeyBootstrapNonceSecret] = "nonce-secret-reference"
	return &ExecutorInstance{InstanceID: executionID, TaskID: taskID, SessionID: sessionID, RuntimeName: agentruntime.RuntimeNomad, Metadata: metadata}
}

func TestConnectNomadAgentctlOverSSHOnlyUsesPinnedConnectionPath(t *testing.T) {
	var controlHealthAttempts atomic.Int32
	var handshakeBeforeControlReady atomic.Bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = io.WriteString(w, `{"server_identity":"server-1","capabilities":["runtime-seal-drain.v1"]}`)
		case "/auth/handshake":
			if controlHealthAttempts.Load() < 3 {
				handshakeBeforeControlReady.Store(true)
			}
			_, _ = io.WriteString(w, `{"token":"fixture-token"}`)
		case "/api/v1/instances":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"execution-1","port":41234}`)
		case "/health":
			if controlHealthAttempts.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer httpServer.Close()
	server := newFakeSSHServer(t, nil)
	server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))
	sshClient := server.dial(t)
	state := nomad.EnvironmentState{
		Version: 1, Revision: 2, EnvironmentID: "env-1", Generation: 3, OperationID: "op-1", InstallationID: "profile-1",
		ExecutionID: "execution-1", InputDigest: strings.Repeat("a", 64), JobID: "job-1", Namespace: "default", Region: "global",
		Datacenter: "dc1", NodeID: "node-b", Platform: "darwin/arm64", SSHHost: "worker-b.example", SSHUser: "runner", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK", WorkspaceRoot: "/workspaces",
		WorkspacePath:  "/workspaces/environments/env-1/g3",
		AllocationRoot: "/allocations/env-1", CPUMHz: 1500, MemoryMB: 4096, AllocationID: "alloc-1", AllocationCreateIndex: 11,
		ControlPort: 22341, InstancePort: 41234, Phase: "allocated",
	}
	var checkpointed []nomad.EnvironmentState
	request := &ExecutorCreateRequest{
		InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1", Protocol: "acp",
		Metadata: map[string]interface{}{}, CheckpointRuntimeInventory: func(_ context.Context, metadata map[string]interface{}) error {
			saved, err := nomad.EnvironmentStateFromMetadata(metadata)
			if err != nil {
				return err
			}
			checkpointed = append(checkpointed, saved)
			return nil
		},
	}
	request.PersistNomadRuntimeSecret = func(_ context.Context, key, value string) error {
		if key != MetadataKeyAuthTokenSecret || value != "fixture-token" {
			return errors.New("unexpected runtime secret persistence request")
		}
		request.Metadata[key] = "fixture-auth-ref"
		return nil
	}
	if err := nomad.PutEnvironmentState(request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	profile := nomad.ProfileConfig{NodeID: state.NodeID, WorkspaceRoot: state.WorkspaceRoot}
	connection, err := connectNomadAgentctlOverSSH(context.Background(), sshClient, profile, &state, request, "one-use-bootstrap-secret", newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if handshakeBeforeControlReady.Load() {
		t.Fatal("bootstrap handshake was sent before the control server passed health readiness")
	}
	if got := controlHealthAttempts.Load(); got < 3 {
		t.Fatalf("control health probes = %d, want retries until the listener is ready", got)
	}
	defer connection.closeHostResources()
	if connection.instance == nil || connection.instance.RuntimeName != agentruntime.RuntimeNomad || connection.instance.Client == nil {
		t.Fatalf("connection did not return a Nomad-backed agentctl instance: %+v", connection)
	}
	updated, err := nomad.EnvironmentStateFromMetadata(request.Metadata)
	if err != nil || updated.ServerIdentity != "server-1" || updated.Phase != "ready" || updated.InstancePort != 41234 {
		t.Fatalf("connection did not checkpoint exact control identity: %+v, err=%v", updated, err)
	}
	if len(checkpointed) != 2 || checkpointed[0].Phase != "allocated" || checkpointed[1].Phase != "ready" || checkpointed[1].ServerIdentity != "server-1" {
		t.Fatalf("ready state was not durably checkpointed after the authenticated connection: %+v", checkpointed)
	}
	if commands := server.commands(); len(commands) != 0 {
		t.Fatalf("Nomad connection path invoked SSH process launch commands: %#v", commands)
	}
}

func TestNomadRecoveryNeverCreatesAMissingAgentctlInstance(t *testing.T) {
	var createCalls atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/identity":
			_, _ = io.WriteString(w, `{"server_identity":"server-1","capabilities":["runtime-seal-drain.v1"]}`)
		case r.URL.Path == "/health":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case r.URL.Path == "/api/v1/instances/execution-1" && r.Method == http.MethodGet:
			http.NotFound(w, r)
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodPost:
			createCalls.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"execution-1","port":41234}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer httpServer.Close()
	server := newFakeSSHServer(t, nil)
	server.forwardTo(strings.TrimPrefix(httpServer.URL, "http://"))
	sshClient := server.dial(t)
	instance := nomadStopTestInstance("execution-1", "task-1", "session-1", "allocation-1")
	state, err := nomad.EnvironmentStateFromMetadata(instance.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	request := &ExecutorCreateRequest{
		InstanceID: instance.InstanceID, TaskID: instance.TaskID, SessionID: instance.SessionID,
		TaskEnvironmentID: state.EnvironmentID, Metadata: instance.Metadata, AuthToken: "saved-token",
		RequireExistingAgentctlInstance: true,
	}
	_, err = connectNomadAgentctlOverSSH(context.Background(), sshClient, nomad.ProfileConfig{NodeID: state.NodeID}, &state, request, "", newTestLogger())
	if err == nil || !strings.Contains(err.Error(), "refusing to recreate during recovery") {
		t.Fatalf("recovery error = %v, want fail-closed missing-instance error", err)
	}
	if got := createCalls.Load(); got != 0 {
		t.Fatalf("recovery issued %d agentctl create requests, want none", got)
	}
}

func TestCategorizeNomadBootstrapHandshakeErrorDoesNotExposeDetails(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "credential rejected", err: errors.New("handshake rejected: invalid nonce nonce-secret (status 403)"), want: "bootstrap credential rejected (HTTP 403)"},
		{name: "transport unavailable", err: errors.New("handshake failed: dial tcp 127.0.0.1:1234: connection refused"), want: "bootstrap handshake transport/unavailable failure"},
		{name: "other HTTP response", err: errors.New("handshake failed: status 503"), want: "bootstrap handshake HTTP response rejected (HTTP 503)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := categorizeNomadBootstrapHandshakeError(tc.err)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Fatalf("categorized error = %v, want category %q", got, tc.want)
			}
			if strings.Contains(got.Error(), "nonce-secret") || strings.Contains(got.Error(), "connection refused") {
				t.Fatalf("categorized error exposed sensitive/detail text: %v", got)
			}
		})
	}
}

func TestNomadWorkerPathsAreImmutableAndDigestScoped(t *testing.T) {
	profile := nomad.ProfileConfig{NodeID: "node-b", AllocationRoot: "/var/lib/kandev/allocations/env-1"}
	state := nomad.EnvironmentState{NodeID: "node-b", EnvironmentID: "env-1", JobID: "job-1"}
	paths, err := nomadWorkerPathsFor(profile, state, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if paths.ArtifactPath != "/var/lib/kandev/allocations/env-1/job-1/agentctl-"+strings.Repeat("a", 64) || paths.EnvironmentPath != "/var/lib/kandev/allocations/env-1/job-1/bootstrap-"+strings.Repeat("a", 64)+".sh.env" {
		t.Fatalf("worker paths = %+v", paths)
	}
	if _, err := nomadWorkerPathsFor(profile, state, strings.Repeat("A", 64)); err == nil {
		t.Fatal("worker paths accepted noncanonical artifact digest")
	}
	state.NodeID = "other-node"
	if _, err := nomadWorkerPathsFor(profile, state, strings.Repeat("a", 64)); err == nil {
		t.Fatal("worker paths accepted a profile/state NodeID mismatch")
	}
}

func TestNomadLaunchDigestBindsRoutingAndExcludesResolvedSecrets(t *testing.T) {
	profile := nomad.ProfileConfig{
		Address: "https://nomad.example.test", Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-1", Platform: "darwin/arm64", SSHUser: "runner", SSHHost: "worker.example.test",
		SSHPort: 22, SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot: "/workspaces", AllocationRoot: "/allocations", CPUMHz: 1500, MemoryMB: 4096,
		NomadTokenSecretID: "secret-reference-id",
	}
	request := &ExecutorCreateRequest{
		InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", WorkspaceID: "workspace-1",
		WorkspacePath: "/workspaces/project", AgentProfileID: "agent-profile-1", Protocol: "acp",
		RepositoryIDs: []string{"repo-1"}, Env: map[string]string{"ANTHROPIC_API_KEY": "first-secret"},
	}
	environmentPath, err := nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, request.TaskEnvironmentID, 3)
	if err != nil {
		t.Fatal(err)
	}
	first, err := nomadLaunchDigest(profile, request, request.WorkspacePath, environmentPath)
	if err != nil {
		t.Fatal(err)
	}
	request.Env["ANTHROPIC_API_KEY"] = "changed-secret"
	second, err := nomadLaunchDigest(profile, request, request.WorkspacePath, environmentPath)
	if err != nil || second != first {
		t.Fatalf("secret rotation changed nonsecret immutable job input digest: %q, %v", second, err)
	}
	request.WorkspacePath = "/workspaces/other-project"
	third, err := nomadLaunchDigest(profile, request, request.WorkspacePath, environmentPath)
	if err != nil || third == first {
		t.Fatalf("workspace routing change did not change launch digest: %q, %v", third, err)
	}
	request.WorkspacePath = "/workspaces/project"
	otherEnvironmentPath, _ := nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, "env-2", 3)
	otherEnvironmentDigest, err := nomadLaunchDigest(profile, request, request.WorkspacePath, otherEnvironmentPath)
	if err != nil || otherEnvironmentDigest == first {
		t.Fatalf("environment-specific workspace change did not change launch digest: %q, %v", otherEnvironmentDigest, err)
	}
	otherGenerationPath, _ := nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, request.TaskEnvironmentID, 4)
	otherGenerationDigest, err := nomadLaunchDigest(profile, request, request.WorkspacePath, otherGenerationPath)
	if err != nil || otherGenerationDigest == first {
		t.Fatalf("generation-specific workspace change did not change launch digest: %q, %v", otherGenerationDigest, err)
	}
}

func TestNomadLaunchDigestUsesOriginalInputNotProjectedWorkspacePath(t *testing.T) {
	profile := nomad.ProfileConfig{
		Address: "https://nomad.example.test", Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-1", Platform: "darwin/arm64", SSHUser: "runner", SSHHost: "worker.example.test",
		SSHPort: 22, SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot: "/workspaces", AllocationRoot: "/allocations", CPUMHz: 1500, MemoryMB: 4096,
	}
	request := &ExecutorCreateRequest{
		InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", WorkspaceID: "workspace-1",
		WorkspacePath: "/workspaces/environments/env-1/g3", AgentProfileID: "agent-profile-1", Protocol: "acp",
		RepositoryIDs: []string{"repo-1"},
	}
	const environmentPath = "/workspaces/environments/env-1/g3"
	first, err := nomadLaunchDigest(profile, request, "", environmentPath)
	if err != nil {
		t.Fatal(err)
	}
	request.WorkspacePath = "/workspaces/projected-runtime-path"
	projected, err := nomadLaunchDigest(profile, request, "", environmentPath)
	if err != nil || projected != first {
		t.Fatalf("runtime path projection changed digest for captured empty input: first=%q projected=%q err=%v", first, projected, err)
	}
	changedInput, err := nomadLaunchDigest(profile, request, "/workspaces/original-user-input", environmentPath)
	if err != nil || changedInput == first {
		t.Fatalf("changed original input did not change immutable digest: first=%q changed=%q err=%v", first, changedInput, err)
	}
}

func TestJobMatchesStateRequiresExactImmutableJobMetadata(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	profile := fixture.profile
	request := fixture.request
	digest, err := nomadLaunchDigest(profile, request, request.WorkspacePath, filepath.Join(profile.WorkspaceRoot, "environments", request.TaskEnvironmentID, "g3"))
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{EnvironmentID: request.TaskEnvironmentID, Generation: 3, OperationID: request.InstanceID, InstallationID: "profile-1"})
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, EnvironmentID: request.TaskEnvironmentID, Generation: 3, OperationID: request.InstanceID,
		ExecutionID: request.InstanceID, InstallationID: "profile-1", InputDigest: digest, JobID: jobID,
		Namespace: profile.Namespace, Region: profile.Region, Datacenter: profile.Datacenter, NodeID: profile.NodeID,
		Platform: profile.Platform, SSHHost: profile.SSHHost, SSHUser: profile.SSHUser, SSHPort: profile.SSHPort,
		SSHHostFingerprint: profile.SSHHostFingerprint, WorkspaceRoot: profile.WorkspaceRoot,
		WorkspacePath:  filepath.Join(profile.WorkspaceRoot, "environments", request.TaskEnvironmentID, "g3"),
		AllocationRoot: profile.AllocationRoot, CPUMHz: profile.CPUMHz, MemoryMB: profile.MemoryMB,
		JobCreateIndex: 5, JobDefinitionIndex: 7, Phase: "registered",
	}
	spec := addFixtureJobEvidence(t, profile, &state)
	raw, err := json.Marshal(func() nomad.Job { spec.CreateIndex = 5; spec.ModifyIndex = 12; spec.JobModifyIndex = 7; return spec }())
	if err != nil {
		t.Fatal(err)
	}
	job := &nomad.JobRecord{
		ID: state.JobID, Namespace: state.Namespace, Region: state.Region, ModifyIndex: 12,
		CreateIndex: 5, JobModifyIndex: 7, Meta: spec.Meta, Spec: spec, Raw: raw,
	}
	if !jobMatchesState(job, state) {
		t.Fatal("exact immutable Nomad job was not recognized during retry reconciliation")
	}
	job.Meta["kandev.input_digest"] = strings.Repeat("b", 64)
	if jobMatchesState(job, state) {
		t.Fatal("changed input digest was accepted as the same immutable job")
	}
}

func TestNomadCreateInstanceDrivesJobAllocationAndAgentctlConnection(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if instance == nil || instance.RuntimeName != agentruntime.RuntimeNomad || instance.InstanceID != fixture.request.InstanceID || instance.Client == nil {
		t.Fatalf("CreateInstance() = %+v, want connected Nomad agentctl instance", instance)
	}
	if len(fixture.checkpoints) != 6 || fixture.checkpoints[0].Phase != "intent" || fixture.checkpoints[1].NativeJobSpecSHA256 == "" || fixture.checkpoints[2].Phase != "registered" || fixture.checkpoints[3].Phase != "allocated" || fixture.checkpoints[4].Phase != "allocated" || fixture.checkpoints[5].Phase != "ready" {
		t.Fatalf("durable lifecycle checkpoints = %+v", fixture.checkpoints)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil || state.Phase != "ready" || state.AllocationID != "alloc-1" || state.ServerIdentity != "fixture-agentctl" {
		t.Fatalf("final environment state = %+v, err=%v", state, err)
	}
	wantWorkspace := filepath.Join(fixture.profile.WorkspaceRoot, "environments", fixture.request.TaskEnvironmentID, "g3")
	if state.WorkspaceRoot != fixture.profile.WorkspaceRoot || state.WorkspacePath != wantWorkspace || instance.WorkspacePath != wantWorkspace {
		t.Fatalf("workspace identity state=%q instance=%q, want %q under profile root %q", state.WorkspacePath, instance.WorkspacePath, wantWorkspace, state.WorkspaceRoot)
	}
	if fixture.job == nil || fixture.job.Job.ID != state.JobID || fixture.job.Job.TaskGroups[0].Tasks[0].Env["AGENTCTL_BOOTSTRAP_NONCE"] != "" {
		t.Fatalf("Nomad job missing or includes bootstrap credential: %+v", fixture.job)
	}
	paths, err := nomadWorkerPathsFor(fixture.profile, state, fixture.artifactDigest)
	if err != nil {
		t.Fatal(err)
	}
	privateEnv, err := os.ReadFile(paths.EnvironmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(privateEnv), "AGENTCTL_BOOTSTRAP_NONCE=") || strings.Contains(string(privateEnv), "fixture-token") {
		t.Fatalf("private bootstrap env lacks nonce or contains Nomad API credential")
	}
	nonceLine := strings.TrimPrefix(strings.Split(strings.TrimSpace(string(privateEnv)), "AGENTCTL_BOOTSTRAP_NONCE=")[1], "'")
	nonce := strings.TrimSuffix(nonceLine, "'")
	jobJSON, err := json.Marshal(fixture.job)
	if err != nil || strings.Contains(string(jobJSON), nonce) {
		t.Fatal("bootstrap nonce appeared in the Nomad job specification")
	}
	for _, call := range fixture.sshServer.execCalls() {
		if strings.Contains(call.Command, nonce) {
			t.Fatal("bootstrap nonce appeared in remote command arguments")
		}
	}
	info, err := os.Stat(paths.EnvironmentPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private bootstrap env permissions = %v, err=%v", info, err)
	}
	fixture.runtime.mu.Lock()
	connection := fixture.runtime.sessions[fixture.request.InstanceID]
	fixture.runtime.mu.Unlock()
	if connection == nil || connection.instance != instance {
		t.Fatal("executor did not retain the authenticated connection owner")
	}
	connection.closeHostResources()
}

func addFixtureJobEvidence(t *testing.T, profile nomad.ProfileConfig, state *nomad.EnvironmentState) nomad.Job {
	t.Helper()
	artifactDigest := strings.Repeat("a", 64)
	state.AgentctlPath = filepath.Join(profile.AllocationRoot, state.JobID, "agentctl-"+artifactDigest)
	state.BootstrapPath = filepath.Join(profile.AllocationRoot, state.JobID, "bootstrap-"+artifactDigest+".sh")
	pathsAgentctl, pathsBootstrap := state.AgentctlPath, state.BootstrapPath
	state.AgentctlPath, state.BootstrapPath = "", ""
	job, err := nomad.BuildNativeJob(nomad.NativeJobInput{Profile: profile, State: *state, AgentctlPath: pathsAgentctl, BootstrapPath: pathsBootstrap})
	if err != nil {
		t.Fatal(err)
	}
	state.NativeJobSpecSHA256, err = nomad.NativeJobSpecDigest(job)
	if err != nil {
		t.Fatal(err)
	}
	state.AgentctlPath, state.BootstrapPath = pathsAgentctl, pathsBootstrap
	state.NativeJobPolicyVersion = nomad.NativeJobPolicyVersion
	job, err = nomad.BuildNativeJob(nomad.NativeJobInput{Profile: profile, State: *state, AgentctlPath: state.AgentctlPath, BootstrapPath: state.BootstrapPath})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

type nomadCreateFixture struct {
	runtime              *NomadExecutor
	request              *ExecutorCreateRequest
	profile              nomad.ProfileConfig
	checkpoints          []nomad.EnvironmentState
	job                  *nomad.JobRegisterRequest
	artifactDigest       string
	sshServer            *fakeSSHServer
	registerCalls        int
	createCalls          int
	preexistingInstance  bool
	fenceOK              bool
	fenceAbsent          bool
	loseRegisterResponse bool
	failBootstrapWrite   bool
	allocationsPending   bool
	rejectBootstrap      bool
	multipleAllocations  bool
	jobHistoryMissing    bool
	fake                 *nomadAdmissionFake
	inventory            []nomad.EnvironmentState
	cleanup              nomadCleanupFixture
}

type nomadCreateSecretReader struct{}

func (nomadCreateSecretReader) Reveal(_ context.Context, id string) (string, error) {
	if id != "nomad-token-id" {
		return "", errors.New("unexpected secret reference")
	}
	return "fixture-token", nil
}

func newNomadCreateFixture(t *testing.T, failRegistration bool) *nomadCreateFixture {
	t.Helper()
	workerRoot := t.TempDir()
	artifact := []byte("fixture darwin arm64 agentctl payload")
	artifactPath := filepath.Join(workerRoot, "agentctl-darwin-arm64")
	if err := os.WriteFile(artifactPath, artifact, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_AGENTCTL_DARWIN_ARM64_BINARY", artifactPath)
	artifactHash := sha256.Sum256(artifact)
	artifactDigest := hex.EncodeToString(artifactHash[:])
	fixture := &nomadCreateFixture{artifactDigest: artifactDigest, fake: newNomadAdmissionFake()}

	agentctlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = io.WriteString(w, `{"server_identity":"fixture-agentctl","capabilities":["runtime-seal-drain.v1"]}`)
		case "/auth/handshake":
			if fixture.rejectBootstrap {
				http.Error(w, "bootstrap rejected", http.StatusForbidden)
				return
			}
			_, _ = io.WriteString(w, `{"token":"fixture-instance-token"}`)
		case "/api/v1/instances":
			fixture.createCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"execution-1","port":41231}`)
		case "/api/v1/instances/execution-1":
			if !fixture.preexistingInstance {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, `{"id":"execution-1","port":41231,"task_id":"task-1","session_id":"session-1","workspace_path":"`+filepath.Join(fixture.profile.WorkspaceRoot, "environments", "environment-1", "g3")+`","status":"running"}`)
		case "/health":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "/api/v1/runtime/seal-and-drain":
			fixture.cleanup.serveDrain(w, r, fixture)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(agentctlServer.Close)

	nomadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nomad-Token") != "fixture-token" {
			t.Errorf("Nomad API request omitted resolved secret token")
		}
		if fixture.fake.serve(w, r) || fixture.cleanup.serveNomad(w, r, fixture) {
			return
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/jobs":
			fixture.registerCalls++
			var request nomad.JobRegisterRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode job registration: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fixture.job = &request
			if fixture.loseRegisterResponse {
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					t.Error("HTTP server cannot simulate a committed response loss")
					return
				}
				connection, _, err := hijacker.Hijack()
				if err != nil {
					t.Errorf("hijack committed registration response: %v", err)
					return
				}
				_ = connection.Close()
				return
			}
			if failRegistration {
				http.Error(w, "fixture failure", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"EvalID":"eval-1","JobModifyIndex":7}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/job/") && !strings.HasSuffix(r.URL.Path, "/allocations"):
			if fixture.job == nil || fixture.jobHistoryMissing {
				http.NotFound(w, r)
				return
			}
			response := fixture.job.Job
			response.CreateIndex = 5
			response.ModifyIndex = 9 // volatile observations may advance independently
			response.JobModifyIndex = 7
			_ = json.NewEncoder(w).Encode(response)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/allocations"):
			if fixture.allocationsPending {
				_, _ = io.WriteString(w, `[]`)
				return
			}
			if fixture.multipleAllocations {
				_, _ = io.WriteString(w, `[{"ID":"alloc-1","NodeID":"node-1","JobID":"`+fixture.job.Job.ID+`","Namespace":"default","DesiredStatus":"run","ClientStatus":"running","CreateIndex":9,"TaskStates":{"agentctl":{"State":"running"}}},{"ID":"alloc-2","NodeID":"node-1","JobID":"`+fixture.job.Job.ID+`","Namespace":"default","DesiredStatus":"run","ClientStatus":"running","CreateIndex":10,"TaskStates":{"agentctl":{"State":"running"}}}]`)
				return
			}
			_, _ = io.WriteString(w, `[{"ID":"alloc-1","NodeID":"node-1","JobID":"`+fixture.job.Job.ID+`","Namespace":"default","DesiredStatus":"run","ClientStatus":"running","CreateIndex":9,"TaskStates":{"agentctl":{"State":"running"}}}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/allocation/alloc-1":
			if r.URL.Query().Get("namespace") != "default" {
				t.Errorf("full allocation namespace = %q", r.URL.Query().Get("namespace"))
			}
			allocationJob := fixture.job.Job
			allocationJob.CreateIndex = 5
			allocationJob.ModifyIndex = 9
			allocationJob.JobModifyIndex = 7
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ID": "alloc-1", "NodeID": "node-1", "JobID": fixture.job.Job.ID, "Namespace": "default",
				"DesiredStatus": "run", "ClientStatus": "running", "CreateIndex": 9,
				"TaskStates": map[string]any{"agentctl": map[string]any{"State": "running"}}, "Job": allocationJob,
				"AllocatedResources": map[string]any{"Shared": map[string]any{"Ports": []any{
					map[string]any{"Label": "agentctl_control", "Value": 41230},
					map[string]any{"Label": "agentctl_instance", "Value": 41231},
				}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(nomadServer.Close)

	sshServer := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		switch {
		case strings.HasPrefix(command, "f="):
			if fixture.fenceOK {
				return sshOut("verified\n")
			}
			if fixture.fenceAbsent {
				return sshOut("absent\n")
			}
			return sshFail("fence mismatch")
		case strings.HasPrefix(command, "mkdir -p "):
			directory := strings.Trim(strings.TrimPrefix(command, "mkdir -p "), "'")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				return sshFail("mkdir failed")
			}
			return sshOK
		case strings.HasPrefix(command, "umask 077; cat > "):
			if fixture.failBootstrapWrite {
				return sshFail("fixture interrupted private environment staging")
			}
			parts := strings.Split(command, "'")
			if len(parts) < 4 {
				return sshFail("malformed private environment write")
			}
			if err := os.WriteFile(parts[1], []byte(stdin), 0o600); err != nil {
				return sshFail("private environment write failed")
			}
			return sshOK
		case strings.HasPrefix(command, "shasum -a 256 "):
			return sshOut(artifactDigest + "  fixture\n")
		case strings.HasPrefix(command, "d=") || strings.HasPrefix(command, "rm -f -- "):
			return fixture.cleanup.serveSSH(command, fixture)
		default:
			return sshFail("unexpected remote command")
		}
	})
	sshServer.enableSFTP()
	sshServer.forwardTo(strings.TrimPrefix(agentctlServer.URL, "http://"))
	fixture.sshServer = sshServer

	fixture.profile = nomad.ProfileConfig{
		Address: nomadServer.URL, Namespace: "default", Region: "global", Datacenter: "dc1", NodeID: "node-1",
		Platform: "darwin/arm64", SSHUser: "fixture", SSHHost: "worker.test", SSHPort: 22,
		SSHHostFingerprint: sshServer.hostKeyFingerprint(), WorkspaceRoot: filepath.Join(workerRoot, "workspace"),
		AllocationRoot: filepath.Join(workerRoot, "allocations"), CPUMHz: 1000, MemoryMB: 2048,
		NomadTokenSecretID: "nomad-token-id",
	}
	fixture.runtime = NewNomadExecutor(NewAgentctlResolver(newTestLogger()), nomadCreateSecretReader{}, newTestLogger())
	fixture.runtime.dialSSH = func(_ context.Context, target *SSHTarget) (*ssh.Client, error) {
		if target.PinnedFingerprint != fixture.profile.SSHHostFingerprint {
			return nil, errors.New("test did not receive the pinned worker fingerprint")
		}
		return sshServer.dial(t), nil
	}
	fixture.request = &ExecutorCreateRequest{
		InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "environment-1",
		WorkspaceID: "workspace-1", WorkspacePath: fixture.profile.WorkspaceRoot, Protocol: "acp",
		Metadata: map[string]interface{}{
			MetadataKeyExecutorProfileID: "profile-1", "address": fixture.profile.Address, "namespace": fixture.profile.Namespace,
			"region": fixture.profile.Region, "datacenter": fixture.profile.Datacenter, "node_id": fixture.profile.NodeID,
			"platform": fixture.profile.Platform, "ssh_target": fixture.profile.SSHUser + "@" + fixture.profile.SSHHost,
			"ssh_port": "22", "ssh_host_fingerprint": fixture.profile.SSHHostFingerprint,
			"workspace_root": fixture.profile.WorkspaceRoot, "allocation_root": fixture.profile.AllocationRoot,
			"cpu_mhz": "1000", "memory_mb": "2048", "nomad_token": "$kandev-secret:nomad-token-id",
			"nomad_environment_generation": "3",
		},
		CheckpointRuntimeInventory: func(_ context.Context, metadata map[string]interface{}) error {
			state, err := nomad.EnvironmentStateFromMetadata(metadata)
			if err != nil {
				return err
			}
			fixture.checkpoints = append(fixture.checkpoints, state)
			fixture.fake.record("checkpoint:" + state.Phase + ":" + state.QuarantineState)
			return nil
		},
		ListNomadInventory: func(context.Context) ([]nomad.EnvironmentState, error) {
			return fixture.inventory, nil
		},
	}
	fixture.request.PersistNomadRuntimeSecret = func(_ context.Context, metadataKey, value string) error {
		if value == "" || strings.Contains(value, "fixture-token") {
			return errors.New("fixture received invalid runtime credential")
		}
		fixture.request.Metadata[metadataKey] = "secret-ref-" + metadataKey
		return nil
	}
	return fixture
}
