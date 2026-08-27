package lifecycle

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/githubauth"
)

// writeTestIdentityFile writes an unencrypted ed25519 private key so the SSH
// executor's "file" identity source has something real to parse. The fake
// server accepts any auth, so the key only has to load.
func writeTestIdentityFile(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal identity key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write identity key: %v", err)
	}
	return path
}

// sshConnectionMetadata builds the executor-config metadata that points the
// SSH executor at the in-process fake server.
func sshConnectionMetadata(t *testing.T, server *fakeSSHServer) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		MetadataKeySSHHost:            "127.0.0.1",
		MetadataKeySSHPort:            strconv.Itoa(server.port()),
		MetadataKeySSHUser:            "kandev",
		MetadataKeySSHHostFingerprint: server.hostKeyFingerprint(),
		MetadataKeySSHIdentitySource:  string(SSHIdentitySourceFile),
		MetadataKeySSHIdentityFile:    writeTestIdentityFile(t),
	}
}

func TestSSHExecutorStaticSurface(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	if exec.Name() != executor.NameSSH {
		t.Fatalf("Name() = %q", exec.Name())
	}
	if err := exec.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() = %v, want nil (per-host reachability is checked elsewhere)", err)
	}
	if !exec.RequiresCloneURL() {
		t.Fatal("SSH remotes must clone the repository themselves")
	}
	if exec.ShouldApplyPreferredShell() {
		t.Fatal("the host's preferred shell must not be applied to a remote")
	}
	if !exec.IsAlwaysResumable() {
		t.Fatal("SSH executions are always resumable")
	}
	if exec.GetInteractiveRunner() != nil {
		t.Fatal("SSH has no host-side interactive runner")
	}
	instances, err := exec.RecoverInstances(context.Background())
	if err != nil || instances != nil {
		t.Fatalf("RecoverInstances() = %v, %v; want nil, nil", instances, err)
	}
}

func TestSSHExecutorTargetFromMetadata(t *testing.T) {
	t.Run("maps every connection field", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		target, err := exec.targetFromMetadata(map[string]interface{}{
			MetadataKeySSHHost:            "build.example",
			MetadataKeySSHPort:            "2222",
			MetadataKeySSHUser:            "deploy",
			MetadataKeySSHHostFingerprint: "SHA256:pinned",
			MetadataKeySSHIdentitySource:  string(SSHIdentitySourceFile),
			MetadataKeySSHIdentityFile:    "/keys/id_ed25519",
			MetadataKeySSHProxyJump:       "bastion.example",
		})
		if err != nil {
			t.Fatalf("targetFromMetadata: %v", err)
		}
		if target.Host != "build.example" || target.Port != 2222 || target.User != "deploy" {
			t.Fatalf("target = %+v", target)
		}
		if target.PinnedFingerprint != "SHA256:pinned" {
			t.Fatalf("PinnedFingerprint = %q", target.PinnedFingerprint)
		}
		if target.IdentitySource != SSHIdentitySourceFile || target.IdentityFile != "/keys/id_ed25519" {
			t.Fatalf("identity = %q / %q", target.IdentitySource, target.IdentityFile)
		}
		if target.ProxyJump != "bastion.example" {
			t.Fatalf("ProxyJump = %q", target.ProxyJump)
		}
	})

	t.Run("host alias alone is accepted", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		target, err := exec.targetFromMetadata(map[string]interface{}{
			MetadataKeySSHHostAlias:       "prod-box",
			MetadataKeySSHUser:            "deploy",
			MetadataKeySSHHostFingerprint: "SHA256:pinned",
			MetadataKeySSHIdentitySource:  string(SSHIdentitySourceAgent),
		})
		if err != nil {
			t.Fatalf("targetFromMetadata: %v", err)
		}
		if target.Host != "prod-box" {
			t.Fatalf("Host = %q, want the alias used as hostname", target.Host)
		}
	})

	tests := []struct {
		name     string
		metadata map[string]interface{}
		wantErr  string
	}{
		{
			name:     "host is required",
			metadata: map[string]interface{}{MetadataKeySSHHostFingerprint: "SHA256:x"},
			wantErr:  "host (or host_alias) is required",
		},
		{
			name: "fingerprint is required",
			metadata: map[string]interface{}{
				MetadataKeySSHHost: "build.example",
			},
			wantErr: "host_fingerprint is required",
		},
		{
			name: "non-numeric port is rejected loudly",
			metadata: map[string]interface{}{
				MetadataKeySSHHost:            "build.example",
				MetadataKeySSHPort:            "abc",
				MetadataKeySSHHostFingerprint: "SHA256:x",
			},
			wantErr: `invalid ssh_port "abc"`,
		},
		{
			name: "port zero is rejected rather than defaulted to 22",
			metadata: map[string]interface{}{
				MetadataKeySSHHost:            "build.example",
				MetadataKeySSHPort:            "0",
				MetadataKeySSHHostFingerprint: "SHA256:x",
			},
			wantErr: `invalid ssh_port "0"`,
		},
		{
			name: "out-of-range port is rejected",
			metadata: map[string]interface{}{
				MetadataKeySSHHost:            "build.example",
				MetadataKeySSHPort:            "70000",
				MetadataKeySSHHostFingerprint: "SHA256:x",
			},
			wantErr: `invalid ssh_port "70000"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
			_, err := exec.targetFromMetadata(tc.metadata)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSSHExecutorWorkdirRoot(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	if got := exec.workdirRoot(nil); got != sshDefaultWorkdir {
		t.Fatalf("default workdir = %q, want %q", got, sshDefaultWorkdir)
	}
	got := exec.workdirRoot(map[string]interface{}{MetadataKeySSHWorkdirRoot: "/srv/kandev"})
	if got != "/srv/kandev" {
		t.Fatalf("override workdir = %q", got)
	}
}

func TestSSHExecutorCloseTearsDownEveryTrackedSession(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	var closed int
	var mu sync.Mutex
	exec.closeClient = func(*ssh.Client) error {
		mu.Lock()
		defer mu.Unlock()
		closed++
		return nil
	}
	forwardServer := newFakeSSHServer(t, nil)
	fwd, err := StartPortForward(forwardServer.dial(t), 41234, newTestLogger())
	if err != nil {
		t.Fatalf("StartPortForward: %v", err)
	}
	exec.sessions["a"] = &sshSessionState{client: &ssh.Client{}, forwarder: fwd}
	exec.sessions["b"] = &sshSessionState{client: &ssh.Client{}}

	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if closed != 2 {
		t.Fatalf("closed clients = %d, want 2", closed)
	}
	if len(exec.sessions) != 0 {
		t.Fatalf("sessions still tracked after Close: %+v", exec.sessions)
	}
	// The forwarder must be closed: a second Close is a no-op error-free call,
	// and its listener must no longer accept.
	if err := fwd.Close(); err != nil {
		t.Fatalf("forwarder Close after executor Close: %v", err)
	}
}

func TestSSHExecutorCloseSSHClientFallsBackToTheRealClose(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	if err := exec.closeSSHClient(nil); err != nil {
		t.Fatalf("closing a nil client must be a no-op, got %v", err)
	}
	exec.closeClient = nil
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	if err := exec.closeSSHClient(client); err != nil {
		t.Fatalf("closeSSHClient: %v", err)
	}
	if _, err := client.NewSession(); err == nil {
		t.Fatal("client must actually be closed when no closeClient seam is set")
	}
}

// sshLaunchHarness wires a fake SSH server plus an in-process agentctl control
// server so CreateInstance / ResumeRemoteInstance can run end to end.
type sshLaunchHarness struct {
	server     *fakeSSHServer
	remoteHome string
	// createRequests captures each create-instance body the control server saw.
	createRequests []string
	handshakes     int
	mu             sync.Mutex
}

func newSSHLaunchHarness(t *testing.T, pid string) *sshLaunchHarness {
	t.Helper()
	h := &sshLaunchHarness{remoteHome: t.TempDir()}

	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		switch r.URL.Path {
		case "/auth/handshake":
			h.handshakes++
			_, _ = io.WriteString(w, `{"token":"launch-token"}`)
		case "/api/v1/instances":
			body, _ := io.ReadAll(r.Body)
			h.createRequests = append(h.createRequests, string(body))
			_, _ = io.WriteString(w, `{"id":"remote-instance","port":41234}`)
		case "/health":
			_, _ = io.WriteString(w, "ok")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(control.Close)

	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	_, sha := writeFakeAgentctl(t, platform, "agentctl-bytes")
	if err := os.MkdirAll(filepath.Join(h.remoteHome, ".kandev", "bin"), 0o755); err != nil {
		t.Fatalf("seed remote bin dir: %v", err)
	}

	h.server = newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case command == "uname -a":
			return sshOut("Linux remote 6.1.0\n")
		case command == "uname -m":
			return sshOut("x86_64\n")
		case command == "uname -s":
			return sshOut("Linux\n")
		case command == "git --version":
			return sshOut("git version 2.44.0\n")
		case strings.Contains(command, remoteHomeCommand):
			return sshOut(h.remoteHome)
		case strings.Contains(command, "agentctl.sha256"):
			return sshOut(sha + "\n")
		case strings.Contains(command, "test -x"):
			return sshOK
		case strings.Contains(command, "nohup"):
			return sshOut(pid + "\n")
		case strings.Contains(command, "agentctl.log"):
			return sshOut("HTTP server bound successfully\n")
		case strings.Contains(command, "kill -0"):
			return sshOK
		default:
			// mkdir -p, prepare script, stop script, …
			return sshOK
		}
	})
	h.server.enableSFTP()
	h.server.forwardTo(strings.TrimPrefix(control.URL, "http://"))
	return h
}

func (h *sshLaunchHarness) createInstanceBodies() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.createRequests...)
}

func TestSSHExecutorCreateInstanceProvisionsAndTracksTheSession(t *testing.T) {
	harness := newSSHLaunchHarness(t, "4242")
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	metadata := sshConnectionMetadata(t, harness.server)
	metadata[MetadataKeySetupScript] = "echo prepared"
	metadata[MetadataKeyBaseBranch] = "main"
	metadata[MetadataKeyWorktreeBranch] = "feature/task-1"

	var steps []string
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		SessionID:  "session-1",
		Protocol:   "acp",
		Metadata:   metadata,
		OnProgress: func(step PrepareStep, _, _ int) {
			steps = append(steps, step.Name+":"+string(step.Status))
		},
	}

	instance, err := exec.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	wantTaskDir := harness.remoteHome + "/.kandev/tasks/task-task-1"
	wantSessionDir := wantTaskDir + "/.kandev/sessions/session-1"
	if instance.InstanceID != "instance-1" || instance.TaskID != "task-1" || instance.SessionID != "session-1" {
		t.Fatalf("instance identity = %+v", instance)
	}
	if instance.RuntimeName != executor.NameSSH {
		t.Fatalf("RuntimeName = %q", instance.RuntimeName)
	}
	if instance.WorkspacePath != wantTaskDir {
		t.Fatalf("WorkspacePath = %q, want the remote task dir %q", instance.WorkspacePath, wantTaskDir)
	}
	if instance.AuthToken != "launch-token" {
		t.Fatalf("AuthToken = %q, want the handshake token", instance.AuthToken)
	}
	if instance.Client == nil {
		t.Fatal("instance must carry an agentctl client bound to the local forward")
	}
	if got := instance.Metadata[MetadataKeySSHRemoteTaskDir]; got != wantTaskDir {
		t.Fatalf("remote task dir metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeySSHRemoteSessionDir]; got != wantSessionDir {
		t.Fatalf("remote session dir metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeySSHRemoteAgentctlPort]; got != "41234" {
		t.Fatalf("remote agentctl port metadata = %v, want the per-instance port", got)
	}
	if got := instance.Metadata[MetadataKeySSHRemoteAgentctlPID]; got != "4242" {
		t.Fatalf("remote pid metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeySSHHost]; got != "127.0.0.1" {
		t.Fatalf("host metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeySSHHostFingerprint]; got != harness.server.hostKeyFingerprint() {
		t.Fatalf("fingerprint metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeySSHWorkdirRoot]; got != sshDefaultWorkdir {
		t.Fatalf("workdir root metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeyIsRemote]; got != true {
		t.Fatalf("is_remote metadata = %v", got)
	}
	forwardPort, _ := strconv.Atoi(instance.Metadata[MetadataKeySSHLocalForwardPort].(string))
	if forwardPort == 0 {
		t.Fatal("local forward port metadata must be a real bound port")
	}

	if len(harness.createInstanceBodies()) != 1 {
		t.Fatalf("create-instance calls = %d, want exactly one", len(harness.createInstanceBodies()))
	}
	if !strings.Contains(harness.createInstanceBodies()[0], `"workspace_path":"`+wantTaskDir+`"`) {
		t.Fatalf("create-instance body = %s", harness.createInstanceBodies()[0])
	}

	for _, want := range []string{
		"Connecting to SSH host:completed",
		"Detecting remote OS:completed",
		"Uploading agent controller:completed",
		"Preparing task directory:completed",
		"Running prepare script:completed",
		"Preparing session directory:completed",
		"Starting agent controller:completed",
		"Creating agent instance:completed",
		"Connecting to agent controller:completed",
	} {
		if !sshContainsStep(steps, want) {
			t.Fatalf("progress steps %v missing %q", steps, want)
		}
	}

	exec.mu.Lock()
	state := exec.sessions["instance-1"]
	exec.mu.Unlock()
	if state == nil {
		t.Fatal("CreateInstance must track the session state for teardown")
	}
	if state.pid != 4242 || state.remoteDir != wantSessionDir || state.remoteTaskDir != wantTaskDir {
		t.Fatalf("session state = %+v", state)
	}
	if state.authToken != "launch-token" {
		t.Fatalf("session auth token = %q", state.authToken)
	}
	if state.platform.GOOS != "linux" || state.platform.GOARCH != "amd64" {
		t.Fatalf("session platform = %+v", state.platform)
	}

	// StopInstance releases the forwarder and SSH client and forgets the state.
	if err := exec.StopInstance(context.Background(), instance, true); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	exec.mu.Lock()
	_, stillTracked := exec.sessions["instance-1"]
	exec.mu.Unlock()
	if stillTracked {
		t.Fatal("StopInstance must drop the tracked session")
	}
	if _, ok := harness.server.lastCommandContaining("kill 4242"); !ok {
		t.Fatalf("expected the remote agentctl to be killed, commands: %v", harness.server.commands())
	}
}

func TestSSHExecutorCreateInstanceReusesResumedState(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	fwd, err := StartPortForward(server.dial(t), 41234, newTestLogger())
	if err != nil {
		t.Fatalf("StartPortForward: %v", err)
	}
	t.Cleanup(func() { _ = fwd.Close() })

	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.sessions["instance-1"] = &sshSessionState{
		target:         &SSHTarget{Host: "build.example", Port: 2222, User: "deploy", PinnedFingerprint: "SHA256:pinned"},
		forwarder:      fwd,
		pid:            999,
		remoteDir:      "/remote/session",
		authToken:      "resumed-token",
		reusingProcess: true,
	}

	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		SessionID:  "session-1",
		Metadata: map[string]interface{}{
			MetadataKeySSHRemoteTaskDir:      "/remote/task",
			MetadataKeySSHRemoteAgentctlPort: "41234",
		},
	}
	instance, err := exec.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if instance.AuthToken != "resumed-token" {
		t.Fatalf("AuthToken = %q, want the resumed token", instance.AuthToken)
	}
	if instance.WorkspacePath != "/remote/task" {
		t.Fatalf("WorkspacePath = %q", instance.WorkspacePath)
	}
	if got := instance.Metadata["reuse_existing_process"]; got != true {
		t.Fatalf("reuse_existing_process = %v, want true", got)
	}
	if got := instance.Metadata[MetadataKeySSHRemoteAgentctlPID]; got != "999" {
		t.Fatalf("pid metadata = %v", got)
	}
	if got := instance.Metadata[MetadataKeySSHHost]; got != "build.example" {
		t.Fatalf("host metadata = %v, want it mirrored from the resumed target", got)
	}
	if got := instance.Metadata[MetadataKeySSHLocalForwardPort]; got != strconv.Itoa(fwd.LocalPort()) {
		t.Fatalf("local forward port = %v, want the live forwarder's port", got)
	}
	if len(server.commands()) != 0 {
		t.Fatalf("resumed create must not re-run remote setup, commands: %v", server.commands())
	}
}

func TestSSHExecutorCreateInstanceIgnoresResumedStateForBrokerRefresh(t *testing.T) {
	// A managed-broker launch must not silently reuse the previous session's
	// agentctl: the broker lease in its environment is stale.
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.sessions["instance-1"] = &sshSessionState{authToken: "old"}
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		Env: map[string]string{
			githubauth.CredentialBrokerURLEnv: "http://127.0.0.1:9/broker",
			githubauth.CredentialLeaseEnv:     "lease-1",
		},
	}
	if _, ok := exec.resumedStateForCreate(req); ok {
		t.Fatal("managed broker env must force a fresh create")
	}
	if _, ok := exec.resumedStateForCreate(nil); ok {
		t.Fatal("nil request must not resume")
	}
}

func TestSSHExecutorCreateInstanceRejectsUnsupportedRemotePlatform(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch command {
		case "uname -m":
			return sshOut("riscv64\n")
		case "uname -s":
			return sshOut("FreeBSD\n")
		default:
			return sshOK
		}
	})
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())

	var failedSteps []PrepareStep
	_, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		SessionID:  "session-1",
		Metadata:   sshConnectionMetadata(t, server),
		OnProgress: func(step PrepareStep, _, _ int) {
			if step.Status == PrepareStepFailed {
				failedSteps = append(failedSteps, step)
			}
		},
	})
	if err == nil {
		t.Fatal("expected an unsupported-platform failure")
	}
	if !strings.Contains(err.Error(), "FreeBSD/riscv64") {
		t.Fatalf("error = %v, want the raw remote platform preserved", err)
	}
	if len(failedSteps) != 1 || failedSteps[0].Name != "Detecting remote OS" {
		t.Fatalf("failed steps = %+v, want a failed OS-detection step", failedSteps)
	}
	if len(exec.sessions) != 0 {
		t.Fatal("a failed launch must leave no tracked session")
	}
}

func TestSSHExecutorCreateInstanceRejectsBadConnectionMetadata(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	_, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
		Metadata:   map[string]interface{}{MetadataKeySSHHost: "build.example"},
	})
	if err == nil || !strings.Contains(err.Error(), "host_fingerprint is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestSSHExecutorPrepareRemoteHostReportsUploadFailure(t *testing.T) {
	// The agentctl resolver cannot find a helper for this platform, so the
	// upload step fails and must be reported as such.
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch command {
		case "uname -m":
			return sshOut("arm64\n")
		case "uname -s":
			return sshOut("Darwin\n")
		default:
			return sshOK
		}
	})
	t.Setenv("KANDEV_AGENTCTL_DARWIN_ARM64_BINARY", filepath.Join(t.TempDir(), "absent"))
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())

	var failed []string
	_, _, err := exec.prepareRemoteHost(context.Background(), server.dial(t), &ExecutorCreateRequest{
		OnProgress: func(step PrepareStep, _, _ int) {
			if step.Status == PrepareStepFailed {
				failed = append(failed, step.Name)
			}
		},
	})
	if err == nil {
		t.Fatal("expected the missing agentctl helper to fail the launch")
	}
	if !sshContainsStep(failed, "Uploading agent controller") {
		t.Fatalf("failed steps = %v, want the upload step", failed)
	}
}
