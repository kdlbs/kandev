package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
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

func (b *captureBroadcaster) waitForAction(t *testing.T, action string) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		actions := b.actions()
		for _, got := range actions {
			if got == action {
				return actions
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("broadcast action %s did not arrive in time; got %v", action, b.actions())
	return nil
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
	actions := hub.waitForAction(t, ws.ActionAgentInstallFinished)
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

func TestOpenCodeInstallPreparesSelectedManagedRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install fixture uses a POSIX shell")
	}
	for _, tc := range []struct {
		family managedruntime.OpenCodeFamily
		source managedruntime.OpenCodeSource
	}{
		{family: managedruntime.OpenCodeFamilyV2, source: managedruntime.OpenCodeSourceManaged},
		{family: managedruntime.OpenCodeFamilyV1, source: managedruntime.OpenCodeSourceManaged},
		{family: managedruntime.OpenCodeFamilyV1, source: managedruntime.OpenCodeSourceNative},
	} {
		t.Run(string(tc.family)+"-"+string(tc.source), func(t *testing.T) {
			settings, closeSettings := openInstallSettingsStore(t)
			t.Cleanup(closeSettings)
			selectionStore := managedruntime.NewStore(settings)
			agent := agents.NewOpenCodeACP()
			agent.SetOpenCodeSelectionReader(selectionStore)
			defaultsV1, err := agent.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV1)
			if err != nil {
				t.Fatalf("resolve v1 runtime spec: %v", err)
			}
			defaultsV2, err := agent.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
			if err != nil {
				t.Fatalf("resolve v2 runtime spec: %v", err)
			}
			defaults := managedruntime.OpenCodeRuntimeDefaults{
				V1Package: defaultsV1.Package, V1Version: defaultsV1.DefaultVersionOrPinned(),
				V2Package: defaultsV2.Package, V2Version: defaultsV2.DefaultVersionOrPinned(),
			}
			selection, err := selectionStore.BootstrapOpenCode(context.Background(), defaults, managedruntime.OpenCodeBootstrapEvidence{})
			if err != nil {
				t.Fatalf("bootstrap fresh selection: %v", err)
			}
			if tc.family == managedruntime.OpenCodeFamilyV1 {
				selection.Family = managedruntime.OpenCodeFamilyV1
				selection.Source = tc.source
				selection.Package = defaults.V1Package
				if tc.source == managedruntime.OpenCodeSourceManaged {
					selection.SelectedVersion = defaults.V1Version
				} else {
					selection.SelectedVersion = ""
				}
				selection.AppliedDefaultVersion = defaults.V1Version
				selection.Revision++
				if err := selectionStore.SaveOpenCodeSelection(context.Background(), 1, selection); err != nil {
					t.Fatalf("select v1 runtime: %v", err)
				}
			}
			spec, err := agent.ManagedNPMRuntimeForFamily(tc.family)
			if err != nil {
				t.Fatalf("resolve selected runtime spec: %v", err)
			}
			version := spec.DefaultVersionOrPinned()
			packageSpec := spec.PackageSpec(version)
			cacheRoot := t.TempDir()
			binDir := t.TempDir()
			homeDir := t.TempDir()
			managedTempRoot := t.TempDir()
			argsPath := filepath.Join(t.TempDir(), "npm-args")
			if _, err := os.Lstat(filepath.Join(homeDir, ".kandev")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("HOME/.kandev exists before install, lstat error = %v", err)
			}
			fakeNPM := fmt.Sprintf("#!/bin/sh\n" +
				"printf '%%s\\n' \"$@\" >> \"$NPM_ARGS_FILE\"\n" +
				"if [ \"$1\" = \"--prefix\" ] && [ \"$3\" = exec ] && [ ! -d \"$2\" ]; then echo \"npm prefix does not exist: $2\" >&2; exit 17; fi\n" +
				"if [ \"$1\" = config ]; then printf '%%s\\n' \"$NPM_CACHE_ROOT\"; exit 0; fi\n" +
				"for arg do if [ \"$arg\" = \"--package=$EXPECTED_PACKAGE_SPEC\" ]; then /bin/mkdir -p \"$NPM_CACHE_ROOT/_npx/$EXPECTED_CACHE_KEY\"; exit 0; fi; done\n" +
				"exit 0\n")
			if err := os.WriteFile(filepath.Join(binDir, "npm"), []byte(fakeNPM), 0o755); err != nil {
				t.Fatalf("write npm fixture: %v", err)
			}
			if tc.source == managedruntime.OpenCodeSourceNative {
				if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte("#!/bin/sh\nprintf '1.18.5\\n'\n"), 0o755); err != nil {
					t.Fatalf("write native OpenCode fixture: %v", err)
				}
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/bin")
			t.Setenv("HOME", homeDir)
			t.Setenv("TMPDIR", managedTempRoot)
			t.Setenv("NPM_CACHE_ROOT", cacheRoot)
			t.Setenv("NPM_ARGS_FILE", argsPath)
			t.Setenv("EXPECTED_PACKAGE_SPEC", packageSpec)
			t.Setenv("EXPECTED_CACHE_KEY", managedruntime.NpxExecutionCacheKey(packageSpec))

			controller, _ := newInstallController(t, agent)
			controller.SetManagedRuntimeSelectionStore(selectionStore)
			displayedScript, err := controller.installScriptForSettings(context.Background(), agent)
			if err != nil {
				t.Fatalf("resolve displayed install command: %v", err)
			}
			displayedAgent := controller.buildAvailableAgentDTO(context.Background(), agent, discovery.Availability{}, time.Now())
			if displayedAgent.InstallScript != displayedScript {
				t.Fatalf("agent DTO install command = %q, want selected command %q", displayedAgent.InstallScript, displayedScript)
			}
			var enqueuedScript string
			withStubStreamingRunner(t, func(ctx context.Context, script string, onChunk func(string)) error {
				enqueuedScript = script
				return defaultStreamingInstallRunner(ctx, script, onChunk)
			})
			job, err := controller.EnqueueInstall(agent.ID())
			if err != nil {
				t.Fatalf("EnqueueInstall: %v", err)
			}
			final := waitForStatus(t, controller, job.JobID, dto.InstallJobStatusSucceeded, dto.InstallJobStatusFailed)
			if final.Status != dto.InstallJobStatusSucceeded {
				t.Fatalf("install job failed: %s (output: %s)", final.Error, final.Output)
			}
			if tc.source == managedruntime.OpenCodeSourceManaged {
				if !strings.Contains(displayedScript, managedruntime.NPMProjectPrefix) {
					t.Fatalf("displayed install command %q omits the managed prefix marker", displayedScript)
				}
				if strings.Contains(enqueuedScript, managedruntime.NPMProjectPrefix) {
					t.Fatalf("queued install command %q retains the unprepared managed prefix", enqueuedScript)
				}
			} else if enqueuedScript != displayedScript {
				t.Fatalf("displayed install command %q differs from queued command %q", displayedScript, enqueuedScript)
			}
			installed, err := agent.IsInstalled(context.Background())
			if err != nil || !installed.Available {
				t.Fatalf("selected %s/%s install available = %v, err=%v", tc.family, tc.source, installed, err)
			}
			args, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("read npm argv: %v", err)
			}
			switch tc.source {
			case managedruntime.OpenCodeSourceNative:
				if !strings.Contains(string(args), "install\n-g\n"+packageSpec) {
					t.Fatalf("native install npm arguments = %q, want exact global package", args)
				}
			case managedruntime.OpenCodeSourceManaged:
				if !strings.Contains(string(args), "--package="+packageSpec) || strings.Contains(string(args), "\n-g\n") {
					t.Fatalf("managed install npm arguments = %q, want exact managed package without global install", args)
				}
				installArgs := strings.SplitN(string(args), "\n", 4)
				if len(installArgs) < 3 || installArgs[0] != "--prefix" || installArgs[2] != "exec" {
					t.Fatalf("managed install npm arguments = %q, want an isolated prefix before exec", args)
				}
				if !filepath.IsAbs(installArgs[1]) || installArgs[1] == managedruntime.NPMProjectPrefix {
					t.Fatalf("managed npm prefix = %q, want the prepared absolute directory", installArgs[1])
				}
				if info, err := os.Stat(installArgs[1]); err != nil || !info.IsDir() {
					t.Fatalf("prepared managed npm prefix %q is not a directory: %v", installArgs[1], err)
				}
				managedTempPrefix := filepath.Clean(managedTempRoot) + string(filepath.Separator)
				if !strings.HasPrefix(filepath.Clean(installArgs[1])+string(filepath.Separator), managedTempPrefix) {
					t.Fatalf("executed npm prefix %q is outside managed temp root %q", installArgs[1], managedTempRoot)
				}
				if _, err := os.Lstat(filepath.Join(homeDir, ".kandev")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("managed install touched HOME/.kandev, lstat error = %v", err)
				}
			}
		})
	}
}

func openInstallSettingsStore(t *testing.T) (*systemsettings.Store, func()) {
	t.Helper()
	database, err := db.OpenSQLite(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("open settings database: %v", err)
	}
	dbHandle := sqlx.NewDb(database, "sqlite3")
	settings, err := systemsettings.NewStore(db.NewPool(dbHandle, dbHandle))
	if err != nil {
		_ = dbHandle.Close()
		t.Fatalf("create settings store: %v", err)
	}
	return settings, func() { _ = dbHandle.Close() }
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
