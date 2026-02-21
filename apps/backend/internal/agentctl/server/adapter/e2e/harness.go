//go:build e2e

// Package e2e provides end-to-end tests that exercise the full adapter lifecycle
// against real agent binaries (Claude Code, Amp, etc.).
//
// These tests cost money (real API subscriptions) and are gated behind the "e2e"
// build tag so they never run in CI. Run them manually:
//
//	go test -tags e2e -v -timeout 10m ./internal/agentctl/server/adapter/e2e/
//
// Tests skip gracefully when an agent binary is not installed on PATH.
//
// Debug logging (set externally — read at package init time):
//
//	KANDEV_DEBUG_AGENT_MESSAGES=true KANDEV_DEBUG_LOG_DIR=/tmp/e2e-debug ...
//
// OTel tracing:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 ...
package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// AgentSpec defines the configuration for an E2E agent test.
type AgentSpec struct {
	// Name is a human-readable name for logging (e.g., "claude-code").
	Name string

	// Command is the full CLI command to run the agent.
	// The first token must be a binary on PATH (or an absolute path).
	// Tests skip if the binary is not found.
	Command string

	// Protocol is the agent protocol constant.
	Protocol agent.Protocol

	// DefaultPrompt is the prompt to send.
	DefaultPrompt string

	// Timeout is the maximum time to wait for the full test lifecycle.
	// Defaults to 2 minutes if zero.
	Timeout time.Duration

	// AutoApprove enables auto-approval of permission requests.
	AutoApprove bool
}

// TestResult holds collected events and metadata from a test run.
type TestResult struct {
	Events      []adapter.AgentEvent
	SessionID   string
	OperationID string
	Duration    time.Duration
}

// RunAgent executes the full adapter lifecycle against an agent binary
// and returns collected events. Skips if the binary is not on PATH.
func RunAgent(t *testing.T, spec AgentSpec) *TestResult {
	t.Helper()
	requireBinary(t, spec.Command)
	return runAgentLifecycle(t, spec, spec.Command)
}

// requireBinary checks that the first token of the command is a binary
// on PATH. Skips the test if not found.
func requireBinary(t *testing.T, command string) {
	t.Helper()
	fields := strings.Fields(command)
	if len(fields) == 0 {
		t.Fatal("empty command")
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		t.Skipf("skipping: %s not found on PATH", fields[0])
	}
}

// runAgentLifecycle implements the full lifecycle:
// workspace setup → process manager → Start → Initialize → NewSession → Prompt → collect events.
func runAgentLifecycle(t *testing.T, spec AgentSpec, command string) *TestResult {
	t.Helper()

	timeout := spec.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	if envTimeout := os.Getenv("E2E_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Set up workspace
	workDir := setupWorkspace(t)

	// Build config and create process manager
	cfg := buildInstanceConfig(command, spec.Protocol, workDir, spec.AutoApprove)
	log := newTestLogger(t)
	mgr := process.NewManager(cfg, log)

	// Start the subprocess
	start := time.Now()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = mgr.Stop(stopCtx)
	})

	// Initialize adapter
	adpt := mgr.GetAdapter()
	if adpt == nil {
		t.Fatal("adapter is nil after Start")
	}
	if err := adpt.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	// Create session
	if _, err := adpt.NewSession(ctx, nil); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Collect events while prompting
	events := collectEventsUntilPromptDone(ctx, t, mgr, adpt, spec.DefaultPrompt)
	duration := time.Since(start)

	return &TestResult{
		Events:      events,
		SessionID:   adpt.GetSessionID(),
		OperationID: adpt.GetOperationID(),
		Duration:    duration,
	}
}

// collectEventsUntilPromptDone drains the manager's updates channel while
// the adapter processes a prompt. Prompt() blocks until the turn completes.
func collectEventsUntilPromptDone(
	ctx context.Context,
	t *testing.T,
	mgr *process.Manager,
	adpt adapter.AgentAdapter,
	prompt string,
) []adapter.AgentEvent {
	t.Helper()

	var events []adapter.AgentEvent
	var mu sync.Mutex
	done := make(chan struct{})

	// Start event collector goroutine
	go func() {
		defer close(done)
		ch := mgr.GetUpdates()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				mu.Lock()
				events = append(events, ev)
				mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Send prompt (blocks until turn completes or context cancels)
	if err := adpt.Prompt(ctx, prompt, nil); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	// Grace period for remaining events to propagate through the channel
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	result := make([]adapter.AgentEvent, len(events))
	copy(result, events)
	return result
}

// setupWorkspace creates a temp directory with git init for agents that require a repo.
func setupWorkspace(t *testing.T) string {
	t.Helper()

	if envDir := os.Getenv("E2E_WORKDIR"); envDir != "" {
		return envDir
	}

	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed in %s: %v\n%s", dir, err, out)
	}

	// Create a minimal file so the repo isn't completely empty
	if err := os.WriteFile(dir+"/README.md", []byte("# E2E Test Workspace\n"), 0644); err != nil {
		t.Fatalf("failed to create README.md: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	_ = cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "init", "--allow-empty")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	_ = cmd.Run()

	return dir
}

// buildInstanceConfig creates a config.InstanceConfig from the test parameters.
func buildInstanceConfig(command string, protocol agent.Protocol, workDir string, autoApprove bool) *config.InstanceConfig {
	args := config.ParseCommand(command)
	env := config.CollectAgentEnv(nil)

	return &config.InstanceConfig{
		Protocol:               protocol,
		AgentCommand:           command,
		AgentArgs:              args,
		WorkDir:                workDir,
		AgentEnv:               env,
		AutoApprovePermissions: autoApprove,
		ApprovalPolicy:         "never",
		ShellEnabled:           false,
		LogLevel:               "debug",
		LogFormat:              "console",
		AgentType:              string(protocol),
	}
}

// newTestLogger creates a logger for e2e tests. Uses debug level so adapter
// internals are visible in test output via -v.
func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	level := "info"
	if testing.Verbose() {
		level = "debug"
	}
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      level,
		Format:     "console",
		OutputPath: "stderr",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

// buildMockAgent compiles the mock-agent binary and returns its path.
func buildMockAgent(t *testing.T) string {
	t.Helper()

	// Find the backend root relative to this test file
	backendRoot := findBackendRoot(t)

	binary := t.TempDir() + "/mock-agent"
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/mock-agent")
	cmd.Dir = backendRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build mock-agent: %v\n%s", err, out)
	}
	return binary
}

// findBackendRoot walks up from CWD to find apps/backend.
func findBackendRoot(t *testing.T) string {
	t.Helper()

	// Try common relative paths from the e2e test directory
	candidates := []string{
		"../../../../../..", // from internal/agentctl/server/adapter/e2e/ back to apps/backend
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	for _, rel := range candidates {
		abs := cwd + "/" + rel
		if _, err := os.Stat(abs + "/cmd/mock-agent"); err == nil {
			return abs
		}
	}

	// Fallback: walk up from cwd looking for cmd/mock-agent
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(dir + "/cmd/mock-agent"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("could not find backend root (cmd/mock-agent) from %s", cwd)
	return ""
}
