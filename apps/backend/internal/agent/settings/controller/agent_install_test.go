package controller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// installScriptAgent extends testAgent so we can set a non-empty install script.
type installScriptAgent struct {
	testAgent
	script string
}

func (a *installScriptAgent) InstallScript() string { return a.script }

// captureBroadcaster captures all WS messages emitted during the test.
type captureBroadcaster struct {
	mu  sync.Mutex
	msg []*ws.Message
}

func (b *captureBroadcaster) Broadcast(m *ws.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msg = append(b.msg, m)
}

func (b *captureBroadcaster) actions() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.msg))
	for i, m := range b.msg {
		out[i] = m.Action
	}
	return out
}

// withStubStreamingRunner swaps streamingInstallRunner for the duration of
// the test. The stub invokes onChunk synchronously to make ordering deterministic.
func withStubStreamingRunner(t *testing.T, fn func(ctx context.Context, script string, onChunk func(string)) error) {
	t.Helper()
	prev := streamingInstallRunner
	streamingInstallRunner = fn
	t.Cleanup(func() { streamingInstallRunner = prev })
}

func newInstallController(t *testing.T, ag agents.Agent) (*Controller, *captureBroadcaster) {
	t.Helper()
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	hub := &captureBroadcaster{}
	ctrl.SetJobBroadcaster(hub)
	return ctrl, hub
}

// waitForStatus polls until the job hits one of the terminal statuses or the
// deadline expires.
func waitForStatus(t *testing.T, ctrl *Controller, jobID string, want ...dto.InstallJobStatus) *dto.InstallJobDTO {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, ok := ctrl.GetInstallJob(jobID)
		if ok {
			for _, w := range want {
				if snap.Status == w {
					return snap
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %v in time", jobID, want)
	return nil
}

func TestEnqueueInstall_StreamsAndSucceeds(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "echo ok",
	}
	ctrl, hub := newInstallController(t, ag)

	withStubStreamingRunner(t, func(_ context.Context, _ string, onChunk func(string)) error {
		onChunk("installing...\n")
		onChunk("done\n")
		return nil
	})

	snap, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("EnqueueInstall() error = %v", err)
	}
	if snap.JobID == "" {
		t.Fatal("expected job_id")
	}

	final := waitForStatus(t, ctrl, snap.JobID, dto.InstallJobStatusSucceeded)
	if !strings.Contains(final.Output, "installing...") || !strings.Contains(final.Output, "done") {
		t.Errorf("output missing stream chunks, got %q", final.Output)
	}
	if final.ExitCode == nil || *final.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", final.ExitCode)
	}

	// Must have broadcast a started and a finished message; output messages
	// land in between but exact count depends on the flush timing.
	actions := hub.actions()
	if len(actions) < 2 {
		t.Fatalf("expected ≥2 broadcasts, got %v", actions)
	}
	if actions[0] != ws.ActionAgentInstallStarted {
		t.Errorf("first action = %s, want %s", actions[0], ws.ActionAgentInstallStarted)
	}
	if actions[len(actions)-1] != ws.ActionAgentInstallFinished {
		t.Errorf("last action = %s, want %s", actions[len(actions)-1], ws.ActionAgentInstallFinished)
	}
}

func TestEnqueueInstall_Failure(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "exit 1",
	}
	ctrl, _ := newInstallController(t, ag)

	withStubStreamingRunner(t, func(_ context.Context, _ string, onChunk func(string)) error {
		onChunk("npm ERR! boom\n")
		return errors.New("exit status 1")
	})

	snap, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("EnqueueInstall() error = %v", err)
	}

	final := waitForStatus(t, ctrl, snap.JobID, dto.InstallJobStatusFailed)
	if final.Error == "" {
		t.Error("Error empty on failed install")
	}
	if !strings.Contains(final.Output, "npm ERR!") {
		t.Errorf("Output missing stderr, got %q", final.Output)
	}
}

func TestEnqueueInstall_IdempotentWhileRunning(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "sleep",
	}
	ctrl, _ := newInstallController(t, ag)

	// Block the runner so the first job stays in 'running' while we call
	// EnqueueInstall a second time.
	release := make(chan struct{})
	withStubStreamingRunner(t, func(ctx context.Context, _ string, _ func(string)) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	first, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if first.JobID != second.JobID {
		t.Errorf("expected same job_id, got %s and %s", first.JobID, second.JobID)
	}

	// Release the runner and wait for the goroutine to finish before the test
	// returns. Otherwise withStubStreamingRunner's restore cleanup races with
	// the still-running goroutine's read of streamingInstallRunner.
	close(release)
	waitForStatus(t, ctrl, first.JobID, dto.InstallJobStatusSucceeded, dto.InstallJobStatusFailed)
}

func TestEnqueueInstall_AgentNotFound(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "echo ok",
	}
	ctrl, _ := newInstallController(t, ag)

	_, err := ctrl.EnqueueInstall("missing")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
}

func TestEnqueueInstall_EmptyScript(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "   ",
	}
	ctrl, _ := newInstallController(t, ag)

	_, err := ctrl.EnqueueInstall("test-agent")
	if !errors.Is(err, ErrInstallScriptEmpty) {
		t.Fatalf("err = %v, want ErrInstallScriptEmpty", err)
	}
}

func TestEnqueueInstall_NoJobStore(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "echo ok",
	}
	// Construct without calling SetJobBroadcaster.
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})

	_, err := ctrl.EnqueueInstall("test-agent")
	if !errors.Is(err, ErrJobStoreUnavailable) {
		t.Fatalf("err = %v, want ErrJobStoreUnavailable", err)
	}
}

func TestRingBuffer_DropsOldestOnLineBoundary(t *testing.T) {
	rb := newRingBuffer(20)
	_, _ = rb.Write([]byte("first line\n"))
	_, _ = rb.Write([]byte("second line\n"))
	_, _ = rb.Write([]byte("third\n"))
	got := rb.String()
	// "first line\n" must have been evicted; the buffer holds the tail starting
	// after the next newline boundary.
	if strings.Contains(got, "first") {
		t.Errorf("ring buffer should have evicted 'first', got %q", got)
	}
	if !strings.Contains(got, "third") {
		t.Errorf("ring buffer missing newest write, got %q", got)
	}
}

// TestIsInstallNpmEnvVar mirrors TestIsNpmEnvVar in agentctl/server/process,
// guarding against the two filters drifting apart and verifying that
// legitimate npm config (registry, proxy, auth, custom .npmrc) survives so
// install scripts behind corporate registries still work.
func TestIsInstallNpmEnvVar(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Poison: pnpm-injected workspace dir.
		{"npm_config_prefix", true},
		{"npm_config_dir", true},
		{"npm_config_user_agent", true},
		{"npm_execpath", true},
		{"npm_node_execpath", true},
		// Per-script context, never user config.
		{"npm_package_name", true},
		{"npm_package_version", true},
		{"npm_lifecycle_event", true},

		// Legitimate npm config that must survive (would break installs
		// behind corporate registries / proxies / private auth otherwise).
		{"npm_config_registry", false},
		{"npm_config_proxy", false},
		{"npm_config_https-proxy", false},
		{"npm_config_userconfig", false},
		{"npm_config_globalconfig", false},
		{"npm_config_//registry.npmjs.org/:_authToken", false},
		{"npm_config_strict-ssl", false},
		{"npm_config_cafile", false},

		// Unrelated env.
		{"PATH", false},
		{"HOME", false},
		{"NPM_TOKEN", false},
		{"NPMRC", false},
		{"npm_not_a_config", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isInstallNpmEnvVar(tt.key); got != tt.expected {
				t.Errorf("isInstallNpmEnvVar(%q) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestFilteredInstallEnv(t *testing.T) {
	// Poison vars pnpm injects.
	t.Setenv("npm_config_prefix", "/workspace/apps/cli")
	t.Setenv("npm_config_user_agent", "pnpm/9.15.9")
	t.Setenv("npm_package_name", "kandev")
	t.Setenv("npm_lifecycle_event", "dev")
	// Legitimate config a user might have in their shell.
	t.Setenv("npm_config_registry", "https://registry.corp.example.com/")
	t.Setenv("npm_config_https-proxy", "http://proxy.corp.example.com:8080")
	// Unrelated env.
	t.Setenv("KANDEV_TEST_KEEP", "yes")

	got := make(map[string]string)
	for _, entry := range filteredInstallEnv() {
		if eq := strings.IndexByte(entry, '='); eq > 0 {
			got[entry[:eq]] = entry[eq+1:]
		}
	}

	for _, k := range []string{"npm_config_prefix", "npm_config_user_agent", "npm_package_name", "npm_lifecycle_event"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s should have been filtered", k)
		}
	}
	for _, k := range []string{"npm_config_registry", "npm_config_https-proxy", "KANDEV_TEST_KEEP"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%s should have been kept", k)
		}
	}
}
