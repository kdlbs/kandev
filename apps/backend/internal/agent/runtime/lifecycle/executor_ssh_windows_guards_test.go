package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/remoteauth"
)

func assertNoWindowsPreparation(t *testing.T, harness *windowsSSHLaunchHarness) {
	t.Helper()
	harness.mu.Lock()
	defer harness.mu.Unlock()
	if len(harness.prepareScripts) != 0 || len(harness.startScripts) != 0 {
		t.Fatalf("rejected launch ran preparation (%d) or launch (%d) scripts", len(harness.prepareScripts), len(harness.startScripts))
	}
}

// The PowerShell scripts exist only for Windows hosts, and Windows has no
// passthrough or POSIX-shell execution path.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.3
// @covers AC-EXECUTORS-SSH-WINDOWS-001.4
// @covers AC-EXECUTORS-SSH-WINDOWS-001.17
func TestSSHWindowsLaunchGuard(t *testing.T) {
	windows := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	linux := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	darwin := SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"}
	for _, tc := range []struct {
		platform    SSHRemotePlatform
		shell       string
		passthrough bool
		wantErr     error
		wantText    []string
	}{
		{platform: windows, shell: ""},
		{platform: windows, shell: "powershell"},
		{platform: windows, shell: "pwsh"},
		{platform: windows, shell: `C:\Program Files\PowerShell\7\pwsh.exe`},
		{platform: windows, shell: "bash", wantErr: errSSHWindowsLaunchUnsupported, wantText: []string{`"bash"`, "powershell", "pwsh"}},
		{platform: windows, shell: "cmd", wantErr: errSSHWindowsLaunchUnsupported, wantText: []string{`"cmd"`, "powershell", "pwsh"}},
		{platform: windows, passthrough: true, wantErr: errSSHWindowsLaunchUnsupported, wantText: []string{"passthrough"}},
		{platform: linux, shell: "bash", passthrough: true},
		{platform: linux, shell: "pwsh", wantErr: errSSHPowerShellRequiresWindows, wantText: []string{"pwsh"}},
		{platform: linux, shell: "/usr/bin/pwsh", wantErr: errSSHPowerShellRequiresWindows, wantText: []string{"/usr/bin/pwsh"}},
		{platform: darwin, shell: "powershell", wantErr: errSSHPowerShellRequiresWindows, wantText: []string{"powershell"}},
	} {
		req := &ExecutorCreateRequest{IsPassthrough: tc.passthrough, Metadata: map[string]interface{}{MetadataKeySSHShell: tc.shell}}
		err := validateSSHWindowsLaunch(req, tc.platform)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("validateSSHWindowsLaunch(%s, shell=%q, passthrough=%v) = %v, want %v", tc.platform.GOOS, tc.shell, tc.passthrough, err, tc.wantErr)
			continue
		}
		for _, want := range tc.wantText {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err.Error(), want)
			}
		}
	}
}

// The guard runs before any remote preparation.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.4
// @covers AC-EXECUTORS-SSH-WINDOWS-001.17
func TestSSHWindowsRejectedLaunchRunsNoPreparation(t *testing.T) {
	for name, configure := range map[string]func(*ExecutorCreateRequest){
		"bash shell":  func(req *ExecutorCreateRequest) { req.Metadata[MetadataKeySSHShell] = "bash" },
		"passthrough": func(req *ExecutorCreateRequest) { req.IsPassthrough = true },
	} {
		t.Run(name, func(t *testing.T) {
			harness := newWindowsSSHLaunchHarness(t)
			req := harness.request(t)
			configure(req)
			if _, err := harness.newExecutor(t).CreateInstance(context.Background(), req); !errors.Is(err, errSSHWindowsLaunchUnsupported) {
				t.Fatalf("CreateInstance error = %v, want errSSHWindowsLaunchUnsupported", err)
			}
			assertNoWindowsPreparation(t, harness)
		})
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

func TestGitCredentialHelperCommand(t *testing.T) {
	for bin, want := range map[string]string{
		"/home/kandev/.kandev/bin/agentctl":             "!/home/kandev/.kandev/bin/agentctl git-credential",
		"/home/kandev user/.kandev/bin/agentctl":        "!'/home/kandev user/.kandev/bin/agentctl' git-credential",
		`C:\Users\Kandev User\.kandev\bin\agentctl.exe`: `!'C:/Users/Kandev User/.kandev/bin/agentctl.exe' git-credential`,
	} {
		if got := gitCredentialHelperCommand(bin); got != want {
			t.Errorf("gitCredentialHelperCommand(%q) = %q, want %q", bin, got, want)
		}
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
