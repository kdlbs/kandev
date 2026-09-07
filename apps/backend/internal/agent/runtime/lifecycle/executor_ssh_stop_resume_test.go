package lifecycle

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/githubauth"
)

// Tests for the SSH executor's status, stop, resume and per-launch preflight
// paths.

func TestSSHExecutorGetRemoteStatus(t *testing.T) {
	t.Run("nil instance is an error", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		if _, err := exec.GetRemoteStatus(context.Background(), nil); err == nil {
			t.Fatal("expected an error for a nil instance")
		}
	})

	t.Run("untracked instance reports unknown", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		status, err := exec.GetRemoteStatus(context.Background(), &ExecutorInstance{InstanceID: "gone"})
		if err != nil {
			t.Fatalf("GetRemoteStatus: %v", err)
		}
		if status.State != sshStatusUnknown {
			t.Fatalf("State = %q, want %q", status.State, sshStatusUnknown)
		}
		if status.ErrorMessage == "" {
			t.Fatal("unknown state must explain why")
		}
	})

	t.Run("tracked session with no client reports disconnected", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.sessions["i"] = &sshSessionState{
			target: &SSHTarget{Host: "build.example", User: "deploy", PinnedFingerprint: "SHA256:x"},
			pid:    77,
		}
		status, err := exec.GetRemoteStatus(context.Background(), &ExecutorInstance{InstanceID: "i"})
		if err != nil {
			t.Fatalf("GetRemoteStatus: %v", err)
		}
		if status.State != sshStatusDisconnected {
			t.Fatalf("State = %q", status.State)
		}
		if status.RemoteName != "build.example" {
			t.Fatalf("RemoteName = %q", status.RemoteName)
		}
		if status.Details["pid"] != 77 || status.Details["user"] != "deploy" ||
			status.Details["fingerprint"] != "SHA256:x" {
			t.Fatalf("Details = %+v", status.Details)
		}
	})

	t.Run("live agentctl reports running, dead one reports agentctl-down", func(t *testing.T) {
		alive := true
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			if alive {
				return sshOK
			}
			return sshFail("no such process")
		})
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.sessions["i"] = &sshSessionState{
			target: &SSHTarget{Host: "build.example"},
			client: server.dial(t),
			pid:    4242,
		}
		status, err := exec.GetRemoteStatus(context.Background(), &ExecutorInstance{InstanceID: "i"})
		if err != nil {
			t.Fatalf("GetRemoteStatus: %v", err)
		}
		if status.State != sshStatusRunning {
			t.Fatalf("State = %q, want %q", status.State, sshStatusRunning)
		}

		alive = false
		status, err = exec.GetRemoteStatus(context.Background(), &ExecutorInstance{InstanceID: "i"})
		if err != nil {
			t.Fatalf("GetRemoteStatus: %v", err)
		}
		if status.State != sshStatusAgentctlDown {
			t.Fatalf("State = %q, want %q", status.State, sshStatusAgentctlDown)
		}
	})
}

func TestSSHShouldStopRemoteAgentctl(t *testing.T) {
	tests := []struct {
		name     string
		instance *ExecutorInstance
		force    bool
		want     bool
	}{
		{name: "nil instance", instance: nil, want: false},
		{name: "backend shutdown preserves agentctl",
			instance: &ExecutorInstance{StopReason: StopReasonBackendShutdown}, want: false},
		{name: "forced backend shutdown still stops",
			instance: &ExecutorInstance{StopReason: StopReasonBackendShutdown}, force: true, want: true},
		{name: "failed agent stop escalates",
			instance: &ExecutorInstance{StopReason: StopReasonBackendShutdown, AgentStopFailed: true}, want: true},
		{name: "user stop kills agentctl",
			instance: &ExecutorInstance{StopReason: "stopped via API"}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sshShouldStopRemoteAgentctl(tc.instance, tc.force); got != tc.want {
				t.Fatalf("sshShouldStopRemoteAgentctl = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSSHExecutorStopInstancePreservesAgentctlOnBackendShutdown(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	stopCalls := 0
	closeCalls := 0
	exec.stopRemote = func(context.Context, *ssh.Client, string, int) error { stopCalls++; return nil }
	exec.closeClient = func(*ssh.Client) error { closeCalls++; return nil }
	exec.sessions["i"] = &sshSessionState{client: &ssh.Client{}, remoteDir: "/remote/session", pid: 4242}

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "i",
		StopReason: StopReasonBackendShutdown,
	}, false)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if stopCalls != 0 {
		t.Fatalf("remote stop calls = %d, want 0 so a restart can re-attach", stopCalls)
	}
	if closeCalls != 1 {
		t.Fatalf("client close calls = %d, want 1", closeCalls)
	}
}

func TestSSHExecutorStopInstanceIsSafeWithoutTrackedState(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	if err := exec.StopInstance(context.Background(), nil, false); err != nil {
		t.Fatalf("StopInstance(nil): %v", err)
	}
	if err := exec.StopInstance(context.Background(), &ExecutorInstance{InstanceID: "gone"}, false); err != nil {
		t.Fatalf("StopInstance(untracked): %v", err)
	}
}

func TestSSHExecutorStopInstanceFallsBackToTheRealStopRemote(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.stopRemote = nil
	exec.sessions["i"] = &sshSessionState{
		client:    server.dial(t),
		remoteDir: "/remote/session",
		pid:       4242,
	}
	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "i",
		StopReason: "stopped via API",
	}, false)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if _, ok := server.lastCommandContaining("kill 4242"); !ok {
		t.Fatalf("expected the default stopRemoteAgentctl to run, commands: %v", server.commands())
	}
}

func TestSSHExecutorResumeRemoteInstance(t *testing.T) {
	t.Run("incomplete metadata is not a resume", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{Metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "4242",
		}}
		if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
			t.Fatalf("ResumeRemoteInstance = %v, want nil so a fresh create proceeds", err)
		}
	})

	t.Run("missing auth token is rejected", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{Metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID:  "4242",
			MetadataKeySSHRemoteAgentctlPort: "41234",
			MetadataKeySSHRemoteSessionDir:   "/remote/session",
			MetadataKeySSHRemoteTaskDir:      "/remote/task",
		}}
		err := exec.ResumeRemoteInstance(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "missing agentctl auth token") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("already-tracked instance short-circuits", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.sessions["instance-1"] = &sshSessionState{authToken: "tok"}
		req := &ExecutorCreateRequest{
			InstanceID: "instance-1",
			AuthToken:  "tok",
			Metadata: map[string]interface{}{
				MetadataKeySSHRemoteAgentctlPID:  "4242",
				MetadataKeySSHRemoteAgentctlPort: "41234",
				MetadataKeySSHRemoteSessionDir:   "/remote/session",
				MetadataKeySSHRemoteTaskDir:      "/remote/task",
			},
		}
		if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
			t.Fatalf("ResumeRemoteInstance: %v", err)
		}
		if _, set := req.Metadata[MetadataKeySSHLocalForwardPort]; set {
			t.Fatal("an idempotent resume must not rewrite the forward port")
		}
	})

	t.Run("re-attaches to a live remote agentctl", func(t *testing.T) {
		harness := newSSHLaunchHarness(t, "4242")
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		t.Cleanup(func() { _ = exec.Close() })

		metadata := sshConnectionMetadata(t, harness.server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
		metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
		metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
		req := &ExecutorCreateRequest{
			InstanceID: "instance-1",
			SessionID:  "session-1",
			AuthToken:  "persisted-token",
			Metadata:   metadata,
		}

		if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
			t.Fatalf("ResumeRemoteInstance: %v", err)
		}
		exec.mu.Lock()
		state := exec.sessions["instance-1"]
		exec.mu.Unlock()
		if state == nil {
			t.Fatal("resume must track the session")
		}
		if state.pid != 4242 || state.remoteDir != "/remote/session" || state.remoteTaskDir != "/remote/task" {
			t.Fatalf("state = %+v", state)
		}
		if state.authToken != "persisted-token" {
			t.Fatalf("authToken = %q", state.authToken)
		}
		forwardPort := req.Metadata[MetadataKeySSHLocalForwardPort]
		if forwardPort != strconv.Itoa(state.forwarder.LocalPort()) {
			t.Fatalf("metadata forward port = %v, want the new local port %d",
				forwardPort, state.forwarder.LocalPort())
		}
	})

	// @covers AC-EXECUTORS-SSH-EXECUTOR-001.9
	t.Run("dead remote pid falls back to fresh create", func(t *testing.T) {
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			if strings.Contains(command, "kill -0") {
				return sshFail("no such process")
			}
			return sshOut(t.TempDir())
		})
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
		metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
		metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
		metadata[MetadataKeySSHLocalForwardPort] = "5000"
		metadata[MetadataKeySSHRemoteAgentctlURL] = "http://127.0.0.1:5000"

		req := &ExecutorCreateRequest{
			InstanceID:          "instance-1",
			PreviousExecutionID: "previous-execution",
			AuthToken:           "persisted-token",
			Metadata:            metadata,
		}
		if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
			t.Fatalf("ResumeRemoteInstance = %v, want nil so a fresh create proceeds", err)
		}
		if len(exec.sessions) != 0 {
			t.Fatal("a fresh-create fallback must not track resumed state")
		}
		for _, key := range []string{
			MetadataKeySSHRemoteSessionDir,
			MetadataKeySSHRemoteAgentctlPort,
			MetadataKeySSHRemoteAgentctlPID,
			MetadataKeySSHLocalForwardPort,
			MetadataKeySSHRemoteAgentctlURL,
		} {
			if _, present := req.Metadata[key]; present {
				t.Fatalf("stale runtime metadata key %q must be cleared", key)
			}
		}
		if req.Metadata[MetadataKeySSHRemoteTaskDir] != "/remote/task" {
			t.Fatalf("remote task directory = %v, want preserved", req.Metadata[MetadataKeySSHRemoteTaskDir])
		}
		if req.PreviousExecutionID != "previous-execution" {
			t.Fatalf("previous execution ID = %q, want preserved resume intent", req.PreviousExecutionID)
		}
	})

	// @covers AC-EXECUTORS-SSH-EXECUTOR-001.10
	t.Run("SSH connection failure remains fail closed", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
		metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
		metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
		server.Close()

		err := execSSHResumeWithMetadata(metadata)
		if err == nil || !strings.Contains(err.Error(), "ssh resume: connect") {
			t.Fatalf("error = %v, want SSH connection failure", err)
		}
		if metadata[MetadataKeySSHRemoteAgentctlPID] != "4242" {
			t.Fatal("indeterminate liveness must not clear runtime metadata")
		}
	})

	t.Run("malformed remote pid remains fail closed", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshOut(t.TempDir())
		})
		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "not-a-pid"
		metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
		metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
		metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

		err := execSSHResumeWithMetadata(metadata)
		if err == nil || !strings.Contains(err.Error(), "invalid remote agentctl pid") {
			t.Fatalf("error = %v, want invalid-pid failure", err)
		}
		if metadata[MetadataKeySSHRemoteAgentctlPID] != "not-a-pid" {
			t.Fatal("invalid runtime identity must not clear runtime metadata")
		}
	})
}

func execSSHResumeWithMetadata(metadata map[string]interface{}) error {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	return exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
		AuthToken:  "persisted-token",
		Metadata:   metadata,
	})
}

func TestSSHExecutorResetManagedBrokerResume(t *testing.T) {
	brokerEnv := map[string]string{
		githubauth.CredentialBrokerURLEnv: "http://127.0.0.1:9/broker",
		githubauth.CredentialLeaseEnv:     "lease-1",
	}
	baseMetadata := func() map[string]interface{} {
		return map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID:  "4242",
			MetadataKeySSHRemoteAgentctlPort: "41234",
			MetadataKeySSHRemoteSessionDir:   "/remote/session",
			MetadataKeySSHRemoteTaskDir:      "/remote/task",
			MetadataKeySSHLocalForwardPort:   "5000",
			MetadataKeySSHRemoteAgentctlURL:  "http://127.0.0.1:5000",
		}
	}

	t.Run("tracked session is stopped and its runtime metadata cleared", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		var preflightCalls, stopCalls, closeCalls int
		var stoppedDir string
		var stoppedPid int
		exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
			preflightCalls++
			return nil
		}
		exec.stopRemote = func(_ context.Context, _ *ssh.Client, dir string, pid int) error {
			stopCalls++
			stoppedDir, stoppedPid = dir, pid
			return nil
		}
		exec.closeClient = func(*ssh.Client) error { closeCalls++; return nil }
		exec.sessions["instance-1"] = &sshSessionState{
			client:    &ssh.Client{},
			remoteDir: "/remote/session",
			pid:       4242,
		}

		req := &ExecutorCreateRequest{InstanceID: "instance-1", Env: brokerEnv, Metadata: baseMetadata()}
		if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
			t.Fatalf("ResumeRemoteInstance: %v", err)
		}
		if preflightCalls != 1 || stopCalls != 1 || closeCalls != 1 {
			t.Fatalf("calls = preflight %d / stop %d / close %d", preflightCalls, stopCalls, closeCalls)
		}
		if stoppedDir != "/remote/session" || stoppedPid != 4242 {
			t.Fatalf("stopped %q pid %d", stoppedDir, stoppedPid)
		}
		if len(exec.sessions) != 0 {
			t.Fatal("the stale session must be dropped")
		}
		for _, key := range []string{
			MetadataKeySSHRemoteSessionDir, MetadataKeySSHRemoteAgentctlPort,
			MetadataKeySSHRemoteAgentctlPID, MetadataKeySSHLocalForwardPort,
			MetadataKeySSHRemoteAgentctlURL,
		} {
			if _, present := req.Metadata[key]; present {
				t.Fatalf("runtime metadata key %q must be cleared so a fresh create runs", key)
			}
		}
		if _, present := req.Metadata[MetadataKeySSHRemoteTaskDir]; !present {
			t.Fatal("the task dir must survive so the fresh create reuses the checkout")
		}
	})

	t.Run("preflight failure aborts before stopping anything", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		wantErr := errors.New("broker unreachable")
		exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
			return wantErr
		}
		stopCalls := 0
		exec.stopRemote = func(context.Context, *ssh.Client, string, int) error { stopCalls++; return nil }
		exec.sessions["instance-1"] = &sshSessionState{client: &ssh.Client{}}

		err := exec.ResumeRemoteInstance(context.Background(),
			&ExecutorCreateRequest{InstanceID: "instance-1", Env: brokerEnv, Metadata: baseMetadata()})
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if stopCalls != 0 {
			t.Fatalf("stop calls = %d, want 0", stopCalls)
		}
	})

	t.Run("stop failure closes the client and surfaces the error", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
			return nil
		}
		exec.stopRemote = func(context.Context, *ssh.Client, string, int) error {
			return errors.New("kill failed")
		}
		closeCalls := 0
		exec.closeClient = func(*ssh.Client) error { closeCalls++; return nil }
		exec.sessions["instance-1"] = &sshSessionState{client: &ssh.Client{}}

		err := exec.ResumeRemoteInstance(context.Background(),
			&ExecutorCreateRequest{InstanceID: "instance-1", Env: brokerEnv, Metadata: baseMetadata()})
		if err == nil || !strings.Contains(err.Error(), "stop stale broker-backed agentctl") {
			t.Fatalf("error = %v", err)
		}
		if closeCalls != 1 {
			t.Fatalf("close calls = %d, want the client released anyway", closeCalls)
		}
	})

	t.Run("untracked session dials the host to refresh the broker", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		preflightCalls := 0
		exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
			preflightCalls++
			return nil
		}
		metadata := sshConnectionMetadata(t, server)
		for key, value := range baseMetadata() {
			metadata[key] = value
		}

		req := &ExecutorCreateRequest{InstanceID: "instance-1", Env: brokerEnv, Metadata: metadata}
		if err := exec.ResumeRemoteInstance(context.Background(), req); err != nil {
			t.Fatalf("ResumeRemoteInstance: %v", err)
		}
		if preflightCalls != 1 {
			t.Fatalf("preflight calls = %d", preflightCalls)
		}
		if _, ok := server.lastCommandContaining("kill 4242"); !ok {
			t.Fatalf("expected the stale remote agentctl to be stopped, commands: %v", server.commands())
		}
	})
}

func TestSSHTaskDirName(t *testing.T) {
	tests := []struct {
		name     string
		req      *ExecutorCreateRequest
		wantName string
	}{
		{
			name:     "falls back to the task ID",
			req:      &ExecutorCreateRequest{TaskID: "abc"},
			wantName: "task-abc",
		},
		{
			name: "task_dir_name hint wins over the task ID",
			req: &ExecutorCreateRequest{TaskID: "abc",
				Metadata: map[string]interface{}{"task_dir_name": "kandev-widgets"}},
			wantName: "kandev-widgets",
		},
		{
			name: "ssh_task_dir_name wins over task_dir_name",
			req: &ExecutorCreateRequest{TaskID: "abc", Metadata: map[string]interface{}{
				"task_dir_name":     "kandev-widgets",
				"ssh_task_dir_name": "ssh-specific",
			}},
			wantName: "ssh-specific",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sshTaskDirName(tc.req); got != tc.wantName {
				t.Fatalf("sshTaskDirName = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func TestClearSSHResumeRuntimeMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		MetadataKeySSHRemoteSessionDir:   "/remote/session",
		MetadataKeySSHRemoteAgentctlPort: "41234",
		MetadataKeySSHRemoteAgentctlPID:  "4242",
		MetadataKeySSHLocalForwardPort:   "5000",
		MetadataKeySSHRemoteAgentctlURL:  "http://127.0.0.1:5000",
		MetadataKeySSHRemoteTaskDir:      "/remote/task",
		MetadataKeySSHHost:               "build.example",
	}
	clearSSHResumeRuntimeMetadata(metadata)
	if len(metadata) != 2 {
		t.Fatalf("remaining metadata = %+v, want only the durable keys", metadata)
	}
	if metadata[MetadataKeySSHRemoteTaskDir] != "/remote/task" || metadata[MetadataKeySSHHost] != "build.example" {
		t.Fatalf("durable metadata was cleared: %+v", metadata)
	}
	clearSSHResumeRuntimeMetadata(nil) // must not panic
}

func TestSSHExecutorReportIsANoOpWithoutACallback(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.report(nil, "Step", PrepareStepCompleted, "output") // must not panic

	var got PrepareStep
	exec.report(func(step PrepareStep, _, _ int) { got = step }, "Step", PrepareStepFailed, "boom")
	if got.Name != "Step" || got.Status != PrepareStepFailed || got.Output != "boom" {
		t.Fatalf("reported step = %+v", got)
	}
}

func TestSSHExecutorPreflightAgentBinary(t *testing.T) {
	t.Run("no agent config skips the probe", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		err := exec.preflightAgentBinary(context.Background(), server.dial(t),
			&ExecutorCreateRequest{}, SSHRemotePlatform{})
		if err != nil {
			t.Fatalf("preflightAgentBinary: %v", err)
		}
		if len(server.commands()) != 0 {
			t.Fatalf("no probe expected, commands: %v", server.commands())
		}
	})

	t.Run("missing binary surfaces the install hint", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("") })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		agent := newFakeIdentityAgent("phantom")
		agent.name = "Phantom Agent"
		agent.installScript = "npm i -g phantom"

		var failed []PrepareStep
		err := exec.preflightAgentBinary(context.Background(), server.dial(t), &ExecutorCreateRequest{
			AgentConfig: agent,
			OnProgress: func(step PrepareStep, _, _ int) {
				if step.Status == PrepareStepFailed {
					failed = append(failed, step)
				}
			},
		}, SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), "Phantom Agent") {
			t.Fatalf("error = %v, want the agent name", err)
		}
		if !strings.Contains(err.Error(), "npm i -g phantom") {
			t.Fatalf("error = %v, want the install hint", err)
		}
		if len(failed) != 1 || failed[0].Name != "Verifying agent binary" {
			t.Fatalf("failed steps = %+v", failed)
		}
	})

	t.Run("present binary passes and reports the resolved path", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("/usr/bin/mock\n") })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		var completed []PrepareStep
		err := exec.preflightAgentBinary(context.Background(), server.dial(t), &ExecutorCreateRequest{
			AgentConfig: newFakeIdentityAgent("mock"),
			OnProgress: func(step PrepareStep, _, _ int) {
				if step.Status == PrepareStepCompleted {
					completed = append(completed, step)
				}
			},
		}, SSHRemotePlatform{})
		if err != nil {
			t.Fatalf("preflightAgentBinary: %v", err)
		}
		if len(completed) != 1 || completed[0].Output != "/usr/bin/mock" {
			t.Fatalf("completed steps = %+v", completed)
		}
	})

	t.Run("probe transport failure fails the launch", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("closed") })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		err := exec.preflightAgentBinary(context.Background(), server.dial(t),
			&ExecutorCreateRequest{AgentConfig: newFakeIdentityAgent("mock")}, SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), "probe") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSSHExecutorProbeNativeBinary(t *testing.T) {
	t.Run("agent without a native binary is skipped", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{AgentConfig: newFakeIdentityAgent("mock")}
		if exec.probeNativeBinary(context.Background(), server.dial(t), "bash", req, "step") {
			t.Fatal("a non-NativeBinaryAgent must not report a hit")
		}
	})

	t.Run("empty native binary name is skipped", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{AgentConfig: &fakeNativeBinaryAgent{
			fakeIdentityAgent: newFakeIdentityAgent("copilot"),
		}}
		if exec.probeNativeBinary(context.Background(), server.dial(t), "bash", req, "step") {
			t.Fatal("an empty native binary name must disable the preference")
		}
		if len(server.commands()) != 0 {
			t.Fatalf("no probe expected, commands: %v", server.commands())
		}
	})

	t.Run("hit records the binary in metadata", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("/usr/bin/copilot\n") })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{AgentConfig: &fakeNativeBinaryAgent{
			fakeIdentityAgent: newFakeIdentityAgent("copilot"),
			nativeBinary:      "copilot",
		}}
		if !exec.probeNativeBinary(context.Background(), server.dial(t), "bash", req, "step") {
			t.Fatal("a resolved native binary must report a hit")
		}
		if req.Metadata[MetadataKeyNativeBinary] != "copilot" {
			t.Fatalf("metadata = %+v, want the native binary recorded", req.Metadata)
		}
	})

	t.Run("miss clears any stale metadata value", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("") })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{
			AgentConfig: &fakeNativeBinaryAgent{
				fakeIdentityAgent: newFakeIdentityAgent("copilot"),
				nativeBinary:      "copilot",
			},
			Metadata: map[string]interface{}{MetadataKeyNativeBinary: "copilot"},
		}
		if exec.probeNativeBinary(context.Background(), server.dial(t), "bash", req, "step") {
			t.Fatal("an absent binary must not report a hit")
		}
		if _, present := req.Metadata[MetadataKeyNativeBinary]; present {
			t.Fatalf("stale native binary must be cleared: %+v", req.Metadata)
		}
	})

	t.Run("transport error falls through without aborting", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("closed") })
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		req := &ExecutorCreateRequest{
			AgentConfig: &fakeNativeBinaryAgent{
				fakeIdentityAgent: newFakeIdentityAgent("copilot"),
				nativeBinary:      "copilot",
			},
			Metadata: map[string]interface{}{MetadataKeyNativeBinary: "copilot"},
		}
		if exec.probeNativeBinary(context.Background(), server.dial(t), "bash", req, "step") {
			t.Fatal("a transport error must not report a hit")
		}
		if _, present := req.Metadata[MetadataKeyNativeBinary]; present {
			t.Fatalf("a failed probe must not leave the preference set: %+v", req.Metadata)
		}
	})
}

func TestSSHExecutorMaybeUploadCredentialsSwallowsFailures(t *testing.T) {
	// A credential-upload failure is best-effort: the launch continues.
	server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("no home") })
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	req := &ExecutorCreateRequest{
		TaskID:    "task-1",
		SessionID: "session-1",
		Metadata:  map[string]interface{}{"remote_credentials": `["agent:mock:files:0"]`},
	}
	exec.maybeUploadCredentials(context.Background(), server.dial(t), req, SSHRemotePlatform{})
}

func sshContainsStep(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
