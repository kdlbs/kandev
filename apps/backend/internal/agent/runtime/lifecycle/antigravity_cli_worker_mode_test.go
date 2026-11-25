package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/pkg/agent"
)

func TestAntigravityCLIWorkerModeDeliversCoreMCP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ACP CLI uses a POSIX shell")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_HOME_DIR", filepath.Join(t.TempDir(), "kandev"))
	bin := filepath.Join(t.TempDir(), "Antigravity CLI")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "acp-wire.jsonl")
	cliPath := filepath.Join(bin, "agy")
	if err := os.WriteFile(cliPath, []byte(antigravityWorkerModeACPFixture), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	for _, model := range []string{"model-a", "model-b"} {
		request := &LaunchRequest{ExecutorType: "local"}
		commands, err := manager.buildAgentCommandWithContext(
			context.Background(), request, &AgentProfileInfo{Model: model}, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
		)
		if err != nil {
			t.Fatalf("build command for %s: %v", model, err)
		}
		if !slices.Contains(commands.args, "--process-per-session") {
			t.Fatalf("model %s command lacks worker mode: %v", model, commands.args)
		}
		runAntigravityWorkerModeACPFixture(t, commands.args, request.AuthPolicy, home, bin, marker)
	}

	wire, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read ACP fixture evidence: %v", err)
	}
	if strings.Count(string(wire), "http_advertised=true") != 2 ||
		strings.Count(string(wire), `"name":"kandev"`) != 2 ||
		strings.Count(string(wire), `"http"`) < 2 {
		t.Fatalf("worker HTTP capability or required Core server was not delivered for both models: %s", wire)
	}
}

func TestAntigravityCLIWorkerModeReapsOwnedWorkersAndResumesNativeSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ACP CLI uses a POSIX shell")
	}
	home, bin, _, marker := installAntigravityWorkerFixture(t)
	command, authPolicy := buildAntigravityWorkerCommand(t)
	if !slices.Contains(command, "--process-per-session") {
		t.Fatalf("worker-mode command lacks process isolation: %v", command)
	}

	first := newAntigravityWorkerFixtureProcess(t, command, authPolicy, home, bin, marker)
	startAntigravityWorkerFixture(t, first)
	nativeSessionID := createAntigravityWorkerFixtureSession(t, first)
	firstChild := waitForAntigravityWorkerChild(t, first.childPIDFile)

	independent := newAntigravityWorkerFixtureProcess(t, command, authPolicy, home, bin, marker)
	startAntigravityWorkerFixture(t, independent)
	if sessionID := createAntigravityWorkerFixtureSession(t, independent); sessionID == "" {
		t.Fatal("independent worker returned an empty session ID")
	}
	independentChild := waitForAntigravityWorkerChild(t, independent.childPIDFile)
	assertAntigravityChildAlive(t, firstChild, true)
	assertAntigravityChildAlive(t, independentChild, true)

	stopAntigravityWorkerFixture(t, first)
	assertAntigravityChildAlive(t, firstChild, false)
	assertAntigravityChildAlive(t, independentChild, true)

	resumeCommand, resumeAuthPolicy := buildAntigravityWorkerResumeCommand(t, nativeSessionID)
	if !slices.Contains(resumeCommand, "--process-per-session") || resumeAuthPolicy == nil || resumeAuthPolicy.RuntimeVariant != acpcompat.AntigravityRuntimeCLI {
		t.Fatalf("saved CLI resume command = %v, policy %+v", resumeCommand, resumeAuthPolicy)
	}
	restarted := newAntigravityWorkerFixtureProcess(t, resumeCommand, resumeAuthPolicy, home, bin, marker)
	startAntigravityWorkerFixture(t, restarted)
	resumeAntigravityWorkerFixtureSession(t, restarted, nativeSessionID)
	if got := restarted.manager.GetAdapter().GetSessionID(); got != nativeSessionID {
		t.Fatalf("resumed native session ID = %q, want stored ID %q", got, nativeSessionID)
	}
	restartedChild := waitForAntigravityWorkerChild(t, restarted.childPIDFile)
	wire, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read ACP resume evidence: %v", err)
	}
	if !strings.Contains(string(wire), `"session/resume"`) || !strings.Contains(string(wire), nativeSessionID) {
		t.Fatalf("restart did not send stored native ID through session/resume: %s", wire)
	}

	stopAntigravityWorkerFixture(t, restarted)
	assertAntigravityChildAlive(t, restartedChild, false)
	assertAntigravityChildAlive(t, independentChild, true)
	stopAntigravityWorkerFixture(t, independent)
	assertAntigravityChildAlive(t, independentChild, false)
}

func runAntigravityWorkerModeACPFixture(
	t *testing.T,
	command []string,
	authPolicy *acpcompat.AuthPolicy,
	home, bin, marker string,
) {
	t.Helper()
	workDir := t.TempDir()
	childEnv := []string{
		"HOME=" + home,
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"ANTIGRAVITY_MCP_MARKER=" + marker,
	}
	processManager := process.NewManager(&config.InstanceConfig{
		AgentType:  agents.AntigravityACPAgentID,
		AgentArgs:  command,
		AgentEnv:   childEnv,
		AuthPolicy: authPolicy,
		Protocol:   agent.ProtocolACP,
		WorkDir:    workDir,
		Port:       43210,
		McpServers: []config.McpServerConfig{
			{Name: "kandev", Type: "http", URL: "http://127.0.0.1:43210/mcp"},
			{Name: "kandev", Type: "sse", URL: "http://127.0.0.1:43210/sse"},
		},
	}, newTestLogger())
	registerAntigravityWorkerFixtureCleanup(t, processManager)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := processManager.Start(ctx); err != nil {
		t.Fatalf("start worker-mode CLI: %v", err)
	}
	if err := processManager.GetAdapter().Initialize(ctx); err != nil {
		t.Fatalf("initialize worker-mode CLI: %v", err)
	}
	coreServers := []types.McpServer{
		{Name: "kandev", Type: "http", URL: "http://127.0.0.1:43210/mcp", BuiltinPackage: mcpprofile.PackageCore},
		{Name: "kandev", Type: "sse", URL: "http://127.0.0.1:43210/sse", BuiltinPackage: mcpprofile.PackageCore},
	}
	if _, err := processManager.GetAdapter().NewSession(ctx, coreServers); err != nil {
		t.Fatalf("create session with required Core MCP: %v", err)
	}
}

type antigravityWorkerFixtureProcess struct {
	manager      *process.Manager
	childPIDFile string
	stopped      *bool
}

func installAntigravityWorkerFixture(t *testing.T) (home, bin, cliPath, marker string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_HOME_DIR", filepath.Join(t.TempDir(), "kandev"))
	bin = filepath.Join(t.TempDir(), "Antigravity CLI")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	cliPath = filepath.Join(bin, "agy")
	if err := os.WriteFile(cliPath, []byte(antigravityWorkerModeACPFixture), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	marker = filepath.Join(t.TempDir(), "acp-wire.jsonl")
	return home, bin, cliPath, marker
}

func buildAntigravityWorkerCommand(t *testing.T) ([]string, *acpcompat.AuthPolicy) {
	t.Helper()
	request := &LaunchRequest{ExecutorType: "local"}
	commands, err := (&Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}).buildAgentCommandWithContext(
		context.Background(), request, &AgentProfileInfo{Model: "model-a"}, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
	)
	if err != nil {
		t.Fatalf("build Antigravity worker command: %v", err)
	}
	return commands.args, request.AuthPolicy
}

func buildAntigravityWorkerResumeCommand(t *testing.T, nativeSessionID string) ([]string, *acpcompat.AuthPolicy) {
	t.Helper()
	request := &LaunchRequest{
		ExecutorType:                 "local",
		RuntimeAdapterVersion:        "antigravity-cli/1.5.15",
		RequiredNativeConversationID: nativeSessionID,
	}
	commands, err := (&Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}).buildAgentCommandWithContext(
		context.Background(), request, &AgentProfileInfo{Model: "model-a"}, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
	)
	if err != nil {
		t.Fatalf("build saved Antigravity resume command: %v", err)
	}
	return commands.args, request.AuthPolicy
}

func newAntigravityWorkerFixtureProcess(
	t *testing.T,
	command []string,
	authPolicy *acpcompat.AuthPolicy,
	home, bin, marker string,
) *antigravityWorkerFixtureProcess {
	t.Helper()
	childPIDFile := filepath.Join(t.TempDir(), "worker-child.pid")
	childEnv := []string{
		"HOME=" + home,
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"ANTIGRAVITY_MCP_MARKER=" + marker,
		"ANTIGRAVITY_CHILD_PID_FILE=" + childPIDFile,
	}
	manager := process.NewManager(&config.InstanceConfig{
		AgentType:  agents.AntigravityACPAgentID,
		AgentArgs:  command,
		AgentEnv:   childEnv,
		AuthPolicy: authPolicy,
		Protocol:   agent.ProtocolACP,
		WorkDir:    t.TempDir(),
		Port:       43210,
		McpServers: []config.McpServerConfig{
			{Name: "kandev", Type: "http", URL: "http://127.0.0.1:43210/mcp"},
			{Name: "kandev", Type: "sse", URL: "http://127.0.0.1:43210/sse"},
		},
	}, newTestLogger())
	stopped := registerAntigravityWorkerFixtureCleanup(t, manager)
	return &antigravityWorkerFixtureProcess{manager: manager, childPIDFile: childPIDFile, stopped: stopped}
}

func registerAntigravityWorkerFixtureCleanup(t *testing.T, manager *process.Manager) *bool {
	t.Helper()
	stopped := new(bool)
	t.Cleanup(func() {
		if *stopped {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := manager.StopForTeardown(ctx); err != nil {
			t.Errorf("stop Antigravity worker fixture: %v", err)
			return
		}
		*stopped = true
	})
	return stopped
}

func startAntigravityWorkerFixture(t *testing.T, fixture *antigravityWorkerFixtureProcess) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := fixture.manager.Start(ctx); err != nil {
		t.Fatalf("start worker-mode CLI: %v", err)
	}
	if err := fixture.manager.GetAdapter().Initialize(ctx); err != nil {
		t.Fatalf("initialize worker-mode CLI: %v", err)
	}
}

func createAntigravityWorkerFixtureSession(t *testing.T, fixture *antigravityWorkerFixtureProcess) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	sessionID, err := fixture.manager.GetAdapter().NewSession(ctx, antigravityWorkerCoreServers())
	if err != nil {
		t.Fatalf("create worker-mode session: %v", err)
	}
	return sessionID
}

func resumeAntigravityWorkerFixtureSession(t *testing.T, fixture *antigravityWorkerFixtureProcess, nativeSessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := fixture.manager.GetAdapter().LoadSession(ctx, nativeSessionID, antigravityWorkerCoreServers()); err != nil {
		t.Fatalf("resume worker-mode native session %q: %v", nativeSessionID, err)
	}
}

func antigravityWorkerCoreServers() []types.McpServer {
	return []types.McpServer{
		{Name: "kandev", Type: "http", URL: "http://127.0.0.1:43210/mcp", BuiltinPackage: mcpprofile.PackageCore},
		{Name: "kandev", Type: "sse", URL: "http://127.0.0.1:43210/sse", BuiltinPackage: mcpprofile.PackageCore},
	}
}

func waitForAntigravityWorkerChild(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pidText, err := os.ReadFile(pidFile)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidText)))
			if parseErr == nil && pid > 0 {
				assertAntigravityChildAlive(t, pid, true)
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("worker child PID was not written to %s", pidFile)
	return 0
}

func stopAntigravityWorkerFixture(t *testing.T, fixture *antigravityWorkerFixtureProcess) {
	t.Helper()
	if *fixture.stopped {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := fixture.manager.StopForTeardown(ctx); err != nil {
		t.Fatalf("stop worker-mode CLI: %v", err)
	}
	*fixture.stopped = true
}

func assertAntigravityChildAlive(t *testing.T, pid int, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		state := strings.TrimSpace(string(output))
		alive := err == nil && state != "" && !strings.HasPrefix(state, "Z") && !strings.HasPrefix(state, "X")
		if alive == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("worker child %d alive = %t, want %t", pid, workerChildIsAlive(pid), want)
}

func workerChildIsAlive(pid int) bool {
	output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(output))
	return err == nil && state != "" && !strings.HasPrefix(state, "Z") && !strings.HasPrefix(state, "X")
}

const antigravityWorkerModeACPFixture = `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'agy 1.5.15'; exit 0; fi
if [ "$1" = "acp" ] && [ "$2" = "--help" ]; then printf 'Usage: agy acp [flags]\n\nFlags:\n  --process-per-session  Serve each session from an isolated worker process\n'; exit 0; fi
worker=false
for argument in "$@"; do
  if [ "$argument" = "--process-per-session" ]; then worker=true; fi
done
if [ "$worker" = "true" ]; then
  printf 'http_advertised=true\n' >> "$ANTIGRAVITY_MCP_MARKER"
  capabilities='{"mcpCapabilities":{"http":true},"sessionCapabilities":{"resume":{}}}'
else
  printf 'http_advertised=false\n' >> "$ANTIGRAVITY_MCP_MARKER"
  capabilities='{"mcpCapabilities":{},"sessionCapabilities":{"resume":{}}}'
fi
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
  case "$line" in
    *'"initialize"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":%s,"agentInfo":{"name":"agy-fixture","version":"1.5.15"}}}\n' "$id" "$capabilities" ;;
    *'"session/new"'*) printf '%s\n' "$line" >> "$ANTIGRAVITY_MCP_MARKER"; if [ -n "$ANTIGRAVITY_CHILD_PID_FILE" ]; then sleep 60 & echo "$!" > "$ANTIGRAVITY_CHILD_PID_FILE"; fi; printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"fixture-session","configOptions":[]}}\n' "$id" ;;
    *'"session/resume"'*) printf '%s\n' "$line" >> "$ANTIGRAVITY_MCP_MARKER"; if [ -n "$ANTIGRAVITY_CHILD_PID_FILE" ]; then sleep 60 & echo "$!" > "$ANTIGRAVITY_CHILD_PID_FILE"; fi; printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id" ;;
    *'"session/close"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id" ;;
  esac
done
`
