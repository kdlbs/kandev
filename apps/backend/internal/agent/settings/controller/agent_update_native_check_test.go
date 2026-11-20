package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

const (
	ompCanaryAvailable = "Current version: 18.13.3\nCurrent channel: canary\nNew version available: 18.22.0-canary.3\n"
	ompUpToDate        = "Current version: 18.13.3\n✔ Already up to date\n"
	ompCheckFailed     = "Current version: 18.13.3\nFailed to check for updates: TypeError: ECONNREFUSED\n"
	ompAgentID         = "omp-acp"
	ompPackage         = "@oh-my-pi/pi-coding-agent"
)

// fakeNativeCheckRunner records runs and returns queued results. With a gate,
// Run blocks until the gate closes or the run context is cancelled.
type fakeNativeCheckRunner struct {
	mu           sync.Mutex
	path         string
	env          []string
	results      []nativeUpdateCheckRun
	fallback     nativeUpdateCheckRun
	calls        int
	resolveCalls int
	gate         chan struct{}
	started      chan struct{}
	envHook      func()
	runHook      func()
}

func (r *fakeNativeCheckRunner) Resolve(agents.Command) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolveCalls++
	if r.path == "" {
		return "", errors.New("executable not found")
	}
	return r.path, nil
}

func (r *fakeNativeCheckRunner) Environment() []string {
	r.mu.Lock()
	hook, env := r.envHook, slices.Clone(r.env)
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	return env
}

func (r *fakeNativeCheckRunner) setEnvironmentHook(hook func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.envHook = hook
}

func (r *fakeNativeCheckRunner) Run(ctx context.Context, _ agents.Command) nativeUpdateCheckRun {
	r.mu.Lock()
	hook := r.runHook
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	r.mu.Lock()
	r.calls++
	result := r.fallback
	if len(r.results) > 0 {
		result, r.results = r.results[0], r.results[1:]
	}
	result.Executable = r.path
	result.RealExecutable = r.path
	if real, err := filepath.EvalSymlinks(r.path); err == nil {
		result.RealExecutable = real
	}
	gate, started := r.gate, r.started
	r.mu.Unlock()
	if started != nil {
		started <- struct{}{}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nativeUpdateCheckRun{Reason: agents.NativeUpdateCheckReasonCancelled, Executable: result.Executable, RealExecutable: result.RealExecutable}
		}
	}
	return result
}

func (r *fakeNativeCheckRunner) setRunHook(hook func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runHook = hook
}

func (r *fakeNativeCheckRunner) setOutput(stdout string, exitCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = nativeUpdateCheckRun{Output: agents.NativeUpdateCheckOutput{Stdout: stdout, ExitCode: exitCode}}
}

func (r *fakeNativeCheckRunner) queue(stdout ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, out := range stdout {
		r.results = append(r.results, nativeUpdateCheckRun{Output: agents.NativeUpdateCheckOutput{Stdout: out}})
	}
}

func (r *fakeNativeCheckRunner) setReason(reason agents.NativeUpdateCheckReason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = nativeUpdateCheckRun{Reason: reason}
}

func (r *fakeNativeCheckRunner) setPath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = path
}

func (r *fakeNativeCheckRunner) setEnv(env ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.env = env
}

func (r *fakeNativeCheckRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *fakeNativeCheckRunner) resolveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resolveCalls
}

// gated makes later runs block; the returned func releases them.
func (r *fakeNativeCheckRunner) gated() (started <-chan struct{}, release func()) {
	gate, signal := make(chan struct{}), make(chan struct{}, 16)
	r.mu.Lock()
	r.gate, r.started = gate, signal
	r.mu.Unlock()
	var once sync.Once
	return signal, func() { once.Do(func() { close(gate) }) }
}

// countingHarnessUpdater records stable-reference lookups.
type countingHarnessUpdater struct {
	harnessTestUpdater
	stableLookups   int
	versionSequence []string
}

func (u *countingHarnessUpdater) ResolveHarnessLatest(ctx context.Context, pkg string) (string, error) {
	u.mu.Lock()
	u.stableLookups++
	u.mu.Unlock()
	return u.harnessTestUpdater.ResolveHarnessLatest(ctx, pkg)
}

func (u *countingHarnessUpdater) stableLookupCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stableLookups
}

func (u *countingHarnessUpdater) setACPVersion(version string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: version}
	u.currentFound = true
}

// sequenceACPVersions makes successive CurrentCapabilities calls report the
// given versions; the last one repeats.
func (u *countingHarnessUpdater) sequenceACPVersions(versions ...string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.versionSequence = versions
}

func (u *countingHarnessUpdater) CurrentCapabilities(agentName string) (hostutility.AgentCapabilities, bool) {
	u.mu.Lock()
	if len(u.versionSequence) > 0 {
		version := u.versionSequence[0]
		if len(u.versionSequence) > 1 {
			u.versionSequence = u.versionSequence[1:]
		}
		u.current = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: version}
		u.currentFound = version != ""
	}
	u.mu.Unlock()
	return u.harnessTestUpdater.CurrentCapabilities(agentName)
}

type nativeCheckHarness struct {
	ctrl    *Controller
	runner  *fakeNativeCheckRunner
	updater *countingHarnessUpdater
	home    string
	cwd     string
}

// newNativeCheckHarness wires a controller with a fake runner whose check
// environment points at a temporary home and working directory.
func newNativeCheckHarness(t *testing.T, acpVersion string, list ...agents.Agent) nativeCheckHarness {
	t.Helper()
	root := t.TempDir()
	h := nativeCheckHarness{home: filepath.Join(root, "home"), cwd: filepath.Join(root, "work")}
	mustMkdir(t, h.home)
	mustMkdir(t, h.cwd)
	t.Chdir(h.cwd)
	registered := map[string]agents.Agent{}
	for _, ag := range list {
		registered[ag.ID()] = ag
	}
	h.ctrl = newTestController(registered)
	h.updater = &countingHarnessUpdater{harnessTestUpdater: harnessTestUpdater{latest: "18.13.3"}}
	h.updater.setACPVersion(acpVersion)
	h.ctrl.SetRuntimeUpdater(h.updater)
	h.runner = &fakeNativeCheckRunner{path: filepath.Join(root, "install", "bin", "omp")}
	h.runner.setEnv(h.baseEnv()...)
	h.ctrl.SetNativeUpdateCheckRunner(h.runner)
	return h
}

func (h nativeCheckHarness) baseEnv() []string {
	return []string{"HOME=" + h.home, "USERPROFILE=" + h.home}
}

func (h nativeCheckHarness) status(t *testing.T, agentName string) dto.AgentUpdateStatusDTO {
	t.Helper()
	return listStatus(t, h.ctrl, agentName)
}

type nativeStatusResult struct {
	status dto.AgentUpdateStatusDTO
	err    error
}

func fetchNativeStatus(ctrl *Controller, agentName string) nativeStatusResult {
	result, err := ctrl.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		return nativeStatusResult{err: err}
	}
	for _, status := range result.Statuses {
		if status.AgentName == agentName {
			return nativeStatusResult{status: status}
		}
	}
	return nativeStatusResult{err: fmt.Errorf("%s missing from statuses", agentName)}
}

func fetchNativeStatusAsync(ctrl *Controller, agentName string) <-chan nativeStatusResult {
	result := make(chan nativeStatusResult, 1)
	go func() { result <- fetchNativeStatus(ctrl, agentName) }()
	return result
}

func awaitNativeStatus(t *testing.T, operation string, result <-chan nativeStatusResult) dto.AgentUpdateStatusDTO {
	t.Helper()
	select {
	case fetched := <-result:
		if fetched.err != nil {
			t.Fatalf("%s failed: %v", operation, fetched.err)
		}
		return fetched.status
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
		return dto.AgentUpdateStatusDTO{}
	}
}

func awaitNativeSignal(t *testing.T, operation string, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func listStatus(t *testing.T, ctrl *Controller, agentName string) dto.AgentUpdateStatusDTO {
	t.Helper()
	fetched := fetchNativeStatus(ctrl, agentName)
	if fetched.err != nil {
		t.Fatal(fetched.err)
	}
	return fetched.status
}

// harnessOnlyFixture is a harness-owned updater without a native check, which
// keeps the generic stable-reference flavor covered.
type harnessOnlyFixture struct{ agents.Agent }

func (a harnessOnlyFixture) HarnessUpdate() agents.HarnessUpdateSpec {
	return agents.NewOmpACP().HarnessUpdate()
}

func newHarnessOnlyFixture() agents.Agent { return harnessOnlyFixture{Agent: agents.NewOmpACP()} }

// nativeHarnessFixture is OMP with controllable enablement and discovery.
type nativeHarnessFixture struct {
	agents.Agent
	disabled    bool
	uninstalled bool
}

func (a *nativeHarnessFixture) HarnessUpdate() agents.HarnessUpdateSpec {
	return agents.NewOmpACP().HarnessUpdate()
}

func (a *nativeHarnessFixture) NativeUpdateCheck() agents.NativeUpdateCheckSpec {
	return agents.NewOmpACP().NativeUpdateCheck()
}

func (a *nativeHarnessFixture) Enabled() bool { return !a.disabled }

func (a *nativeHarnessFixture) IsInstalled(context.Context) (*agents.DiscoveryResult, error) {
	return &agents.DiscoveryResult{Available: !a.uninstalled}, nil
}

// checkerOnlyFixture declares a native check without a harness updater.
type checkerOnlyFixture struct {
	agents.Agent
	id string
}

func (a checkerOnlyFixture) ID() string { return a.id }

func (a checkerOnlyFixture) NativeUpdateCheck() agents.NativeUpdateCheckSpec {
	return agents.NewOmpACP().NativeUpdateCheck()
}

// manualTwinFixture is the same manual agent without a native check.
type manualTwinFixture struct {
	agents.Agent
	id string
}

func (a manualTwinFixture) ID() string { return a.id }

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.1, AC-AGENTS-RUNTIME-NATIVE-CHECK-001.5
func TestNativeCheckStatusBypassesRegistryLookup(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP(), agents.NewClaudeACP())
	var mu sync.Mutex
	var looked []string
	h.ctrl.SetRuntimeUpdateStatusResolver(func(_ context.Context, pkg string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		looked = append(looked, pkg)
		return "0.100.0", nil
	})
	h.runner.setOutput(ompCanaryAvailable, 0)
	omp := h.status(t, ompAgentID)
	if omp.CheckSource != dto.AgentUpdateCheckSourceNative || omp.CheckState != dto.AgentUpdateCheckStateUpdateAvailable ||
		omp.LatestVersion != "18.22.0-canary.3" || omp.UpdateChannel != "canary" || omp.CheckReason != "" {
		t.Errorf("native status = %+v", omp)
	}
	if omp.UpdateMode != dto.AgentUpdateModeSelfUpdate || omp.Package != ompPackage || omp.EffectiveVersion != "18.13.3" || omp.CheckedAt == nil {
		t.Errorf("native status identity = %+v", omp)
	}
	if claude := listStatus(t, h.ctrl, "claude-acp"); claude.CheckSource != "" || claude.CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("non-native status changed: %+v", claude)
	}
	mu.Lock()
	defer mu.Unlock()
	if slices.Contains(looked, ompPackage) || len(looked) == 0 {
		t.Errorf("registry lookups = %v, want only non-native packages", looked)
	}
	if h.updater.stableLookupCount() != 0 {
		t.Error("native status queried the stable reference")
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.7
func TestNativeCheckRuntimeMismatchIsUnknown(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.2", agents.NewOmpACP())
	h.runner.setOutput(ompCanaryAvailable, 0)
	status := h.status(t, ompAgentID)
	if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonRuntimeMismatch) ||
		status.LatestVersion != "" || status.UpdateChannel != "" || status.EffectiveVersion != "18.13.2" {
		t.Errorf("mismatched status = %+v", status)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.6
func TestNativeCheckUnknownReasonsNeverReportUpToDate(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompCheckFailed, 1)
	status := h.status(t, ompAgentID)
	if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != "check_failed" || status.LatestVersion != "" {
		t.Errorf("failed check status = %+v", status)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.3
func TestNativeCheckSkipsDisabledAndUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture *nativeHarnessFixture
	}{
		{name: "disabled", fixture: &nativeHarnessFixture{Agent: agents.NewOmpACP(), disabled: true}},
		{name: "discovery unavailable", fixture: &nativeHarnessFixture{Agent: agents.NewOmpACP(), uninstalled: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newNativeCheckHarness(t, "18.13.3", tc.fixture)
			discoveryRegistry, err := discovery.LoadRegistry(context.Background(), h.ctrl.agentRegistry, h.ctrl.logger)
			if err != nil {
				t.Fatal(err)
			}
			h.ctrl.discovery = discoveryRegistry
			h.runner.setOutput(ompCanaryAvailable, 0)
			status := h.status(t, ompAgentID)
			if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != "unavailable" || status.LatestVersion != "" {
				t.Errorf("ineligible status = %+v", status)
			}
			preview, err := h.ctrl.PreviewAgentUpdate(context.Background(), ompAgentID)
			if err != nil {
				t.Fatal(err)
			}
			if preview.CheckSource != dto.AgentUpdateCheckSourceNative || preview.CheckState != dto.AgentUpdateCheckStateUnknown ||
				preview.CheckReason != "unavailable" || preview.StableLatestVersion != "" {
				t.Errorf("ineligible preview = %+v", preview)
			}
			if h.runner.callCount() != 0 || h.runner.resolveCount() != 0 {
				t.Errorf("ineligible agent resolved %d and ran %d checks", h.runner.resolveCount(), h.runner.callCount())
			}
		})
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4, AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8
func TestNativeCheckPreviewSkipsStableReference(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompCanaryAvailable, 0)
	_ = h.status(t, ompAgentID)
	preview, err := h.ctrl.PreviewAgentUpdate(context.Background(), ompAgentID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CheckSource != dto.AgentUpdateCheckSourceNative || preview.CheckState != dto.AgentUpdateCheckStateUpdateAvailable ||
		preview.LatestVersion != "18.22.0-canary.3" || preview.UpdateChannel != "canary" || preview.StableLatestVersion != "" {
		t.Errorf("native preview = %+v", preview)
	}
	if preview.UpdateMode != dto.AgentUpdateModeSelfUpdate || preview.TargetVersion != "" || len(preview.AvailableVersions) != 0 ||
		preview.Operation != "update" || !reflect.DeepEqual(preview.Command, []string{"omp", "update"}) {
		t.Errorf("approval shape changed: %+v", preview)
	}
	if h.updater.stableLookupCount() != 0 || h.runner.callCount() != 1 {
		t.Errorf("stable lookups = %d, native runs = %d; preview must reuse the cached check", h.updater.stableLookupCount(), h.runner.callCount())
	}
	h.updater.refreshCaps = hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "18.22.0-canary.3"}
	hub := newUpdateTerminalBroadcaster()
	h.ctrl.SetJobBroadcaster(hub)
	h.ctrl.updateJobStore.onRefresh = nil
	job, err := h.ctrl.EnqueueAgentUpdate(context.Background(), ompAgentID, "")
	if err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	h.updater.mu.Lock()
	defer h.updater.mu.Unlock()
	if !reflect.DeepEqual(h.updater.runCommand, []string{"omp", "update"}) || !reflect.DeepEqual(h.updater.refreshCommand, []string{"omp", "acp"}) {
		t.Errorf("approval ran %v and probed %v", h.updater.runCommand, h.updater.refreshCommand)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.4
func TestNativeCheckReportsCheckCurrentWhenRuntimeVersionUnknown(t *testing.T) {
	h := newNativeCheckHarness(t, "", agents.NewOmpACP())
	h.runner.setOutput(ompCanaryAvailable, 0)
	status := h.status(t, ompAgentID)
	preview, err := h.ctrl.PreviewAgentUpdate(context.Background(), ompAgentID)
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentVersion != "18.13.3" || preview.CurrentVersion != "18.13.3" {
		t.Errorf("current version: status = %q, preview = %q; want the check-reported 18.13.3", status.CurrentVersion, preview.CurrentVersion)
	}
	if status.EffectiveVersion != "" || preview.EffectiveVersion != "" || preview.Operation != "repair" {
		t.Errorf("effective/operation must stay runtime-owned: status effective = %q, preview effective = %q, operation = %q",
			status.EffectiveVersion, preview.EffectiveVersion, preview.Operation)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8
func TestNativeCheckDoesNotEnableAutomaticUpdates(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompCanaryAvailable, 0)
	if status := h.status(t, ompAgentID); status.AutoUpdateSupported || status.Management != "manual" {
		t.Errorf("native status enabled automation: %+v", status)
	}
	if err := h.ctrl.RunRuntimeUpdatePass(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs := h.ctrl.ListAgentUpdateJobs(); len(jobs) != 0 {
		t.Errorf("background pass started %d jobs for a native result", len(jobs))
	}
	if h.runner.callCount() != 1 {
		t.Errorf("background pass reran a cached check: %d runs", h.runner.callCount())
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.1, AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8
func TestNativeCheckCheckerOnlyAgentKeepsManualCapability(t *testing.T) {
	checker := checkerOnlyFixture{Agent: agents.NewKimiACP(), id: "checker-only"}
	twin := manualTwinFixture{Agent: agents.NewKimiACP(), id: "manual-twin"}
	h := newNativeCheckHarness(t, "18.13.3", checker, twin)
	h.runner.setOutput(ompCanaryAvailable, 0)
	native, plain := h.status(t, checker.id), h.status(t, twin.id)
	if native.CheckSource != dto.AgentUpdateCheckSourceNative || native.CheckState != dto.AgentUpdateCheckStateUpdateAvailable || native.LatestVersion != "18.22.0-canary.3" {
		t.Errorf("checker-only status = %+v", native)
	}
	if native.UpdateMode != plain.UpdateMode || native.Management != plain.Management || native.Owner != plain.Owner ||
		native.Mechanism != plain.Mechanism || native.Source != plain.Source || native.GuidanceURL != plain.GuidanceURL ||
		native.Package != plain.Package || native.AutoUpdateSupported || plain.AutoUpdateSupported {
		t.Errorf("checker-only capability changed:\nnative %+v\nplain  %+v", native, plain)
	}
	if h.ctrl.IsHarnessUpdate(checker.id) {
		t.Error("checker-only agent became a harness update")
	}
	_, nativeErr := h.ctrl.PreviewAgentUpdate(context.Background(), checker.id)
	_, plainErr := h.ctrl.PreviewAgentUpdate(context.Background(), twin.id)
	if fmt.Sprint(nativeErr) != fmt.Sprint(plainErr) {
		t.Errorf("install action eligibility changed: native %v, plain %v", nativeErr, plainErr)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.3
func TestNativeCheckNotifiesOnlyAvailable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stdout string
		exit   int
		want   string
	}{
		{name: "update available", stdout: ompCanaryAvailable, want: "18.22.0-canary.3"},
		{name: "up to date", stdout: ompUpToDate},
		{name: "unknown", stdout: ompCheckFailed, exit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
				h.runner.setOutput(tc.stdout, tc.exit)
				notices := &runtimeNoticeCapture{}
				h.ctrl.SetRuntimeUpdateNotifier(notices)
				stop := h.ctrl.StartRuntimeUpdateBackground(context.Background())
				defer stop()
				synctest.Wait()
				time.Sleep(runtimeUpdateAvailabilityWindow)
				synctest.Wait()
				delivered, _ := notices.snapshot()
				if tc.want == "" {
					if len(delivered) != 0 {
						t.Fatalf("%s result notified: %+v", tc.name, delivered)
					}
					return
				}
				if len(delivered) != 1 || delivered[0].AgentID != ompAgentID || delivered[0].Version != tc.want || delivered[0].PreviousVersion != "18.13.3" {
					t.Fatalf("native availability notices = %+v", delivered)
				}
			})
		})
	}
}

// A subscription replay is requested from the WebSocket subscribe path, which
// acknowledges only after the request returns. A cold native check must not
// hold that acknowledgement; the replay still delivers its notice later.
func TestNativeCheckReplayRequestDoesNotWaitForCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		h.runner.setOutput(ompCanaryAvailable, 0)
		started, release := h.runner.gated()
		notices := &runtimeNoticeCapture{}
		h.ctrl.SetRuntimeUpdateNotifier(notices)
		stop := h.ctrl.StartRuntimeUpdateBackground(context.Background(), make(chan struct{}))
		defer stop()
		h.ctrl.RequestRuntimeUpdateReplay()
		h.ctrl.RequestRuntimeUpdateReplay()
		<-started
		synctest.Wait()
		requireRuns(t, h.runner, 1, "while the replayed check is gated")
		release()
		synctest.Wait()
		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		delivered, _ := notices.snapshot()
		if len(delivered) != 1 || delivered[0].Version != "18.22.0-canary.3" {
			t.Fatalf("replayed notices = %+v", delivered)
		}
	})
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
