package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/remoteauth"
)

func newWindowsGuardRequest(t *testing.T, harness *windowsSSHLaunchHarness) *ExecutorCreateRequest {
	t.Helper()
	metadata := sshConnectionMetadata(t, harness.server)
	metadata["repository_clone_url"] = "https://github.com/example/demo.git"
	return &ExecutorCreateRequest{
		InstanceID: "instance-win",
		TaskID:     "task-win",
		SessionID:  "session-win",
		Protocol:   "acp",
		Metadata:   metadata,
	}
}

func assertNoWindowsPreparation(t *testing.T, harness *windowsSSHLaunchHarness) {
	t.Helper()
	harness.mu.Lock()
	defer harness.mu.Unlock()
	if len(harness.prepareScripts) != 0 || len(harness.startScripts) != 0 {
		t.Fatalf("rejected launch ran preparation (%d) or launch (%d) scripts", len(harness.prepareScripts), len(harness.startScripts))
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.4
func TestSSHWindowsRejectsNonPowerShellSavedShell(t *testing.T) {
	for _, shell := range []string{"bash", "cmd"} {
		t.Run(shell, func(t *testing.T) {
			withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
			harness := newWindowsSSHLaunchHarness(t, 4242)
			exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
			t.Cleanup(func() { _ = exec.Close() })
			req := newWindowsGuardRequest(t, harness)
			req.Metadata[MetadataKeySSHShell] = shell

			_, err := exec.CreateInstance(context.Background(), req)
			if !errors.Is(err, errSSHWindowsLaunchUnsupported) {
				t.Fatalf("CreateInstance error = %v, want errSSHWindowsLaunchUnsupported", err)
			}
			for _, want := range []string{`"` + shell + `"`, "powershell", "pwsh"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err.Error(), want)
				}
			}
			assertNoWindowsPreparation(t, harness)
		})
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.17
func TestSSHWindowsRejectsPassthroughLaunch(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	harness := newWindowsSSHLaunchHarness(t, 4242)
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })
	req := newWindowsGuardRequest(t, harness)
	req.IsPassthrough = true

	_, err := exec.CreateInstance(context.Background(), req)
	if !errors.Is(err, errSSHWindowsLaunchUnsupported) || !strings.Contains(err.Error(), "passthrough") {
		t.Fatalf("CreateInstance error = %v, want passthrough rejection", err)
	}
	assertNoWindowsPreparation(t, harness)
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.3
func TestSSHWindowsLaunchAcceptsPowerShellShells(t *testing.T) {
	windows := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	for _, shell := range []string{"", "powershell", "pwsh", `C:\Program Files\PowerShell\7\pwsh.exe`} {
		req := &ExecutorCreateRequest{Metadata: map[string]interface{}{MetadataKeySSHShell: shell}}
		if err := validateSSHWindowsLaunch(req, windows); err != nil {
			t.Errorf("validateSSHWindowsLaunch(shell=%q) = %v, want nil", shell, err)
		}
	}
	posix := &ExecutorCreateRequest{IsPassthrough: true, Metadata: map[string]interface{}{MetadataKeySSHShell: "bash"}}
	if err := validateSSHWindowsLaunch(posix, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}); err != nil {
		t.Errorf("validateSSHWindowsLaunch on Linux = %v, want nil", err)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.2
func TestSSHWindowsRemoteDockerRejectsWindowsHost(t *testing.T) {
	err := SSHRequireSupportedRemotePlatform(SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"})
	if err == nil {
		t.Fatal("Remote Docker gate accepted windows/amd64")
	}
	_, listed, found := strings.Cut(err.Error(), "supported platforms are")
	if !found || strings.Contains(listed, "windows") || !strings.Contains(listed, "darwin/{amd64,arm64}") {
		t.Fatalf("Remote Docker error %q must list only Linux and macOS platforms", err.Error())
	}
	if err := SSHRequireSupportedExecutorPlatform(SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}); err != nil {
		t.Fatalf("SSH executor gate rejected windows/amd64: %v", err)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.1
// @covers AC-EXECUTORS-SSH-WINDOWS-001.2
func TestSSHWindowsRejectsARM64Host(t *testing.T) {
	err := SSHRequireSupportedExecutorPlatform(SSHRemotePlatform{GOOS: "windows", GOARCH: "arm64", UnameOS: "Windows_NT", UnameArch: "ARM64"})
	if err == nil {
		t.Fatal("SSH executor gate accepted windows/arm64")
	}
	if !strings.Contains(err.Error(), `"windows/arm64"`) || !strings.Contains(err.Error(), "windows/amd64") ||
		strings.Contains(err.Error(), "windows/{amd64,arm64}") {
		t.Fatalf("error %q must name windows/arm64 and list windows/amd64 as the only Windows platform", err.Error())
	}
}

func TestGitCredentialHelperCommandQuotesPOSIXPathsWithSpaces(t *testing.T) {
	if got, want := gitCredentialHelperCommand("/home/kandev/.kandev/bin/agentctl"), "!/home/kandev/.kandev/bin/agentctl git-credential"; got != want {
		t.Fatalf("plain POSIX helper = %q, want %q", got, want)
	}
	if got, want := gitCredentialHelperCommand("/home/kandev user/.kandev/bin/agentctl"), "!'/home/kandev user/.kandev/bin/agentctl' git-credential"; got != want {
		t.Fatalf("spaced POSIX helper = %q, want %q", got, want)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.18
func TestSSHWindowsReclaimPreservesWindowsTaskDirWithoutRemoteCommands(t *testing.T) {
	runner := &fakeSSHRunner{responses: map[string]fakeSSHResponse{}}
	reclaimer := newTestReclaimer(runner)

	outcome, verdict, err := reclaimer.Reclaim(context.Background(),
		`C:\Users\Kandev User\.kandev`, `C:\Users\Kandev User\.kandev\tasks\task-win`)
	if err != nil {
		t.Fatalf("Reclaim error = %v, want a skip", err)
	}
	if outcome != SSHReclaimOutcomeSkipped || verdict.Reason != SSHReclaimSkipUnsupportedPlatform {
		t.Fatalf("Reclaim = (%s, %s), want (skipped, %s)", outcome, verdict.Reason, SSHReclaimSkipUnsupportedPlatform)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("Reclaim ran remote commands on a Windows task dir: %v", runner.calls)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.19
func TestSSHWindowsSkipsPOSIXAuthSetupScripts(t *testing.T) {
	var commands []string
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		commands = append(commands, command)
		return sshOK
	})
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })
	method := remoteauth.Method{MethodID: "agent:claude:env:ANTHROPIC_API_KEY", Type: "env", SetupScript: `mkdir -p "${HOME}/.claude"`}
	exec.runOneAuthSetupScript(context.Background(), server.dial(t), &ExecutorCreateRequest{Metadata: map[string]interface{}{}},
		"Claude", method, SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"})
	if len(commands) != 0 {
		t.Fatalf("POSIX auth setup script ran on a Windows host: %v", commands)
	}
}
