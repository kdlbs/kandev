package controller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type fakeRuntimeUpdater struct {
	mu              sync.Mutex
	current         hostutility.AgentCapabilities
	currentFound    bool
	target          string
	resolveErr      error
	runErr          error
	runErrs         []error
	invalidateErr   error
	refreshCaps     hostutility.AgentCapabilities
	refreshErr      error
	runCommand      []string
	refreshCommand  []string
	updateOutput    string
	runStarted      chan struct{}
	releaseRun      chan struct{}
	runCalls        int
	invalidateCalls int
	invalidatePkg   string
	refreshCalls    int
	resolvedPackage string
}

type recordingCommandExecutor struct {
	outputCommand []string
	output        string
}

func (e *recordingCommandExecutor) Output(
	_ context.Context,
	command agents.Command,
) (string, error) {
	e.outputCommand = append([]string(nil), command.Args()...)
	return e.output, nil
}

func (e *recordingCommandExecutor) Stream(
	context.Context,
	agents.Command,
	func(string),
) error {
	return nil
}

func TestHostRuntimeUpdaterResolvesTargetWithDirectNPMArgv(t *testing.T) {
	executor := &recordingCommandExecutor{output: "\"1.2.3\"\n"}
	updater := &hostRuntimeUpdater{executor: executor}

	target, err := updater.ResolveTarget(context.Background(), "@example/managed-acp")
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target != "1.2.3" {
		t.Fatalf("target = %q, want 1.2.3", target)
	}
	want := []string{"npm", "view", "@example/managed-acp", "dist-tags.latest", "--json"}
	if got := strings.Join(executor.outputCommand, "\x00"); got != strings.Join(want, "\x00") {
		t.Fatalf("command = %v, want %v", executor.outputCommand, want)
	}
}

func TestHostRuntimeUpdaterInvalidatesOnlyManagedNPMExecutionTree(t *testing.T) {
	cacheRoot := t.TempDir()
	spec := agents.ManagedNPMRuntimeSpec{Package: "opencode-ai"}
	target := filepath.Join(cacheRoot, "_npx", spec.ExecutionCacheKey())
	other := filepath.Join(cacheRoot, "_npx", "keep-me")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}

	executor := &recordingCommandExecutor{output: cacheRoot + "\n"}
	updater := &hostRuntimeUpdater{executor: executor}
	if err := updater.InvalidateExecutionCache(context.Background(), spec.Package); err != nil {
		t.Fatalf("InvalidateExecutionCache: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target stat error = %v, want not exists", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated cache entry was removed: %v", err)
	}
	want := []string{"npm", "config", "get", "cache"}
	if got := strings.Join(executor.outputCommand, "\x00"); got != strings.Join(want, "\x00") {
		t.Fatalf("command = %v, want %v", executor.outputCommand, want)
	}
}

func (f *fakeRuntimeUpdater) CurrentCapabilities(string) (hostutility.AgentCapabilities, bool) {
	return f.current, f.currentFound
}

func (f *fakeRuntimeUpdater) ResolveTarget(_ context.Context, packageName string) (string, error) {
	f.mu.Lock()
	f.resolvedPackage = packageName
	f.mu.Unlock()
	return f.target, f.resolveErr
}

func (f *fakeRuntimeUpdater) RunUpdate(
	ctx context.Context,
	command agents.Command,
	onChunk func(string),
) error {
	f.mu.Lock()
	f.runCalls++
	f.runCommand = append([]string(nil), command.Args()...)
	started := f.runStarted
	release := f.releaseRun
	output := f.updateOutput
	err := f.runErr
	if f.runCalls-1 < len(f.runErrs) {
		err = f.runErrs[f.runCalls-1]
	}
	f.mu.Unlock()
	if started != nil {
		select {
		case <-started:
		default:
			close(started)
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	onChunk(output)
	return err
}

func (f *fakeRuntimeUpdater) InvalidateExecutionCache(_ context.Context, packageName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidateCalls++
	f.invalidatePkg = packageName
	return f.invalidateErr
}

func (f *fakeRuntimeUpdater) Refresh(
	_ context.Context,
	_ string,
	command agents.Command,
) (hostutility.AgentCapabilities, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshCalls++
	f.refreshCommand = append([]string(nil), command.Args()...)
	return f.refreshCaps, f.refreshErr
}

type updateTerminalBroadcaster struct {
	completed chan dto.AgentUpdateJobDTO
}

func newUpdateTerminalBroadcaster() *updateTerminalBroadcaster {
	return &updateTerminalBroadcaster{completed: make(chan dto.AgentUpdateJobDTO, 2)}
}

func (b *updateTerminalBroadcaster) Broadcast(message *ws.Message) {
	if message.Action != ws.ActionAgentUpdateFinished {
		return
	}
	var job dto.AgentUpdateJobDTO
	if json.Unmarshal(message.Payload, &job) != nil {
		return
	}
	select {
	case b.completed <- job:
	default:
	}
}

func newUpdateTestStore(
	updater RuntimeUpdater,
	maintenance *maintenanceCoordinator,
	onRefresh func(),
) (*AgentUpdateJobStore, <-chan dto.AgentUpdateJobDTO) {
	hub := newUpdateTerminalBroadcaster()
	return NewAgentUpdateJobStore(hub, zap.NewNop(), updater, maintenance, onRefresh), hub.completed
}

func waitForUpdateStatus(
	t *testing.T,
	completed <-chan dto.AgentUpdateJobDTO,
	jobID string,
	statuses ...dto.AgentUpdateJobStatus,
) *dto.AgentUpdateJobDTO {
	t.Helper()
	var snapshot dto.AgentUpdateJobDTO
	select {
	case snapshot = <-completed:
	case <-time.After(2 * time.Second):
		t.Fatalf("job %s did not finish in time", jobID)
	}
	if snapshot.JobID != jobID {
		t.Fatalf("finished job = %s, want %s", snapshot.JobID, jobID)
	}
	for _, status := range statuses {
		if snapshot.Status == status {
			return &snapshot
		}
	}
	t.Fatalf("job %s finished with status %s, want %v", jobID, snapshot.Status, statuses)
	return nil
}

func managedRuntimeSpec() agents.ManagedNPMRuntimeSpec {
	return agents.ManagedNPMRuntimeSpec{
		Package: "@example/managed-acp",
		ACPArgs: []string{"--acp"},
	}
}

func TestAgentUpdatePreviewResolvesTrustedCommandWithoutStartingAJob(t *testing.T) {
	updater := &fakeRuntimeUpdater{
		current:      hostutility.AgentCapabilities{AgentVersion: "1.0.0"},
		currentFound: true,
		target:       "1.1.0",
	}
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec:      managedRuntimeSpec(),
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	ctrl.SetRuntimeUpdater(updater)

	preview, err := ctrl.PreviewAgentUpdate(context.Background(), ag.ID())
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	if preview.CurrentVersion != "1.0.0" || preview.TargetVersion != "1.1.0" {
		t.Fatalf("versions = %q -> %q", preview.CurrentVersion, preview.TargetVersion)
	}
	wantCommand := []string{
		"npm", "exec", "--yes", "--prefer-online", "--package=@example/managed-acp", "--", "node", "-e", "",
	}
	if got := strings.Join(preview.Command, "\x00"); got != strings.Join(wantCommand, "\x00") {
		t.Fatalf("command = %q, want %q", got, strings.Join(wantCommand, "\x00"))
	}
	if preview.CommandString != `npm exec --yes --prefer-online --package=@example/managed-acp -- node -e ""` {
		t.Fatalf("command string = %q", preview.CommandString)
	}

	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.resolvedPackage != "@example/managed-acp" {
		t.Fatalf("resolved package = %q", updater.resolvedPackage)
	}
	if updater.runCalls != 0 || updater.refreshCalls != 0 {
		t.Fatalf("preview mutated runtime: update=%d refresh=%d", updater.runCalls, updater.refreshCalls)
	}
}

func TestAgentUpdatePreviewRejectsUnsupportedAndResolutionFailure(t *testing.T) {
	tests := []struct {
		name    string
		agent   agents.Agent
		updater *fakeRuntimeUpdater
		wantErr error
	}{
		{
			name:    "unmanaged",
			agent:   &testAgent{id: "native", name: "Native", enabled: true},
			updater: &fakeRuntimeUpdater{},
			wantErr: ErrRuntimeUpdateUnsupported,
		},
		{
			name: "registry failure",
			agent: &managedTestAgent{
				testAgent: testAgent{id: "managed", name: "Managed", enabled: true},
				spec:      managedRuntimeSpec(),
			},
			updater: &fakeRuntimeUpdater{resolveErr: errors.New("registry unavailable")},
			wantErr: ErrRuntimeUpdatePreviewFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := newTestController(map[string]agents.Agent{test.agent.ID(): test.agent})
			ctrl.SetRuntimeUpdater(test.updater)
			_, err := ctrl.PreviewAgentUpdate(context.Background(), test.agent.ID())
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("PreviewAgentUpdate error = %v, want %v", err, test.wantErr)
			}
			test.updater.mu.Lock()
			defer test.updater.mu.Unlock()
			if test.updater.runCalls != 0 || test.updater.refreshCalls != 0 {
				t.Fatalf("failed preview mutated runtime: update=%d refresh=%d", test.updater.runCalls, test.updater.refreshCalls)
			}
		})
	}
}

func TestAgentUpdateJobResolvesUpdatesRefreshesAndStreams(t *testing.T) {
	updater := &fakeRuntimeUpdater{
		current:      hostutility.AgentCapabilities{AgentVersion: "1.0.0"},
		currentFound: true,
		target:       "1.1.0",
		refreshCaps: hostutility.AgentCapabilities{
			Status:       hostutility.StatusOK,
			AgentVersion: "1.1.0",
		},
		updateOutput: "npm prepared runtime\n",
	}
	refreshed := make(chan struct{}, 1)
	store, completed := newUpdateTestStore(
		updater, newMaintenanceCoordinator(), func() { refreshed <- struct{}{} },
	)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if final.CurrentVersion != "1.0.0" || final.TargetVersion != "1.1.0" {
		t.Fatalf("versions = %q -> %q, want 1.0.0 -> 1.1.0", final.CurrentVersion, final.TargetVersion)
	}
	if final.Output != "npm prepared runtime\n" {
		t.Fatalf("Output = %q", final.Output)
	}
	select {
	case <-refreshed:
	default:
		t.Fatal("successful refresh did not invoke catalogue callback")
	}

	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.resolvedPackage != "@example/managed-acp" {
		t.Fatalf("resolved package = %q", updater.resolvedPackage)
	}
	wantUpdate := "npm exec --yes --prefer-online --package=@example/managed-acp -- node -e "
	if got := strings.Join(updater.runCommand, " "); got != wantUpdate {
		t.Fatalf("update command = %q, want %q", got, wantUpdate)
	}
	wantRefresh := "npx --yes --prefer-offline @example/managed-acp --acp"
	if got := strings.Join(updater.refreshCommand, " "); got != wantRefresh {
		t.Fatalf("refresh command = %q, want %q", got, wantRefresh)
	}
}

func TestAgentUpdateAuthRequiredIsPackageSuccessWithRefreshError(t *testing.T) {
	updater := &fakeRuntimeUpdater{
		target: "1.1.0",
		refreshCaps: hostutility.AgentCapabilities{
			Status: hostutility.StatusAuthRequired,
			Error:  "login required",
		},
	}
	refreshed := false
	store, completed := newUpdateTestStore(
		updater, newMaintenanceCoordinator(), func() { refreshed = true },
	)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if final.RefreshError != "login required" {
		t.Fatalf("RefreshError = %q, want login required", final.RefreshError)
	}
	if final.Error != "" {
		t.Fatalf("Error = %q, want empty", final.Error)
	}
	if refreshed {
		t.Fatal("auth-required refresh must not broadcast a replacement catalogue")
	}
}

func TestAgentUpdateRegistryFailureStopsBeforeMutation(t *testing.T) {
	updater := &fakeRuntimeUpdater{resolveErr: errors.New("registry unavailable")}
	store, completed := newUpdateTestStore(updater, newMaintenanceCoordinator(), nil)
	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusFailed)
	if !strings.Contains(final.Error, "registry unavailable") {
		t.Fatalf("Error = %q", final.Error)
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.runCalls != 0 || updater.refreshCalls != 0 {
		t.Fatalf("calls after registry failure: update=%d refresh=%d", updater.runCalls, updater.refreshCalls)
	}
}

func TestAgentUpdateRepairsExecutionCacheAndRetriesOnce(t *testing.T) {
	updater := &fakeRuntimeUpdater{
		target:  "1.1.0",
		runErrs: []error{errors.New("truncated npm execution tree"), nil},
		refreshCaps: hostutility.AgentCapabilities{
			Status:       hostutility.StatusOK,
			AgentVersion: "1.1.0",
		},
	}
	store, completed := newUpdateTestStore(updater, newMaintenanceCoordinator(), nil)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if !strings.Contains(final.Output, "repairing execution cache") ||
		!strings.Contains(final.Output, "retrying managed runtime update") {
		t.Fatalf("recovery output = %q", final.Output)
	}

	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.runCalls != 2 {
		t.Fatalf("update calls = %d, want 2", updater.runCalls)
	}
	if updater.invalidateCalls != 1 || updater.invalidatePkg != managedRuntimeSpec().Package {
		t.Fatalf("cache repair = %d calls for %q", updater.invalidateCalls, updater.invalidatePkg)
	}
}

func TestAgentUpdateHardFailuresRemainFailed(t *testing.T) {
	tests := []struct {
		name        string
		updater     *fakeRuntimeUpdater
		wantMessage string
	}{
		{
			name: "package update",
			updater: &fakeRuntimeUpdater{
				target: "1.1.0",
				runErr: errors.New("npm exec failed"),
			},
			wantMessage: "npm exec failed",
		},
		{
			name: "ACP initialization",
			updater: &fakeRuntimeUpdater{
				target: "1.1.0",
				refreshCaps: hostutility.AgentCapabilities{
					Status: hostutility.StatusFailed,
					Error:  "unsupported protocol version",
				},
			},
			wantMessage: "unsupported protocol version",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			refreshed := false
			store, completed := newUpdateTestStore(
				test.updater, newMaintenanceCoordinator(), func() { refreshed = true },
			)
			job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
			if err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusFailed)
			if !strings.Contains(final.Error, test.wantMessage) {
				t.Fatalf("Error = %q, want %q", final.Error, test.wantMessage)
			}
			if refreshed {
				t.Fatal("hard failure invoked catalogue refresh callback")
			}
		})
	}
}

func TestAgentUpdateDeduplicatesAndConflictsWithInstall(t *testing.T) {
	coordinator := newMaintenanceCoordinator()
	updater := &fakeRuntimeUpdater{
		target:      "1.1.0",
		refreshCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK},
		runStarted:  make(chan struct{}),
		releaseRun:  make(chan struct{}),
	}
	store, completed := newUpdateTestStore(updater, coordinator, nil)
	t.Cleanup(func() {
		select {
		case <-updater.releaseRun:
		default:
			close(updater.releaseRun)
		}
	})

	first, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	<-updater.runStarted
	second, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("job IDs differ: %s != %s", first.ID, second.ID)
	}

	installStore := NewJobStore(&captureBroadcaster{}, zap.NewNop(), nil, coordinator)
	_, err = installStore.Enqueue("managed-acp", "echo install")
	var conflict *MaintenanceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("install conflict error = %v", err)
	}
	if conflict.Active.JobID != first.ID || conflict.Active.Kind != MaintenanceKindUpdate {
		t.Fatalf("active conflict = %#v", conflict.Active)
	}

	close(updater.releaseRun)
	waitForUpdateStatus(t, completed, first.ID, dto.AgentUpdateJobStatusSucceeded)

	retry, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("retry enqueue: %v", err)
	}
	if retry.ID == first.ID {
		t.Fatal("retry reused a completed update job")
	}
	waitForUpdateStatus(t, completed, retry.ID, dto.AgentUpdateJobStatusSucceeded)
}

func TestAgentUpdateOutputIsBounded(t *testing.T) {
	updater := &fakeRuntimeUpdater{
		target:       "1.1.0",
		refreshCaps:  hostutility.AgentCapabilities{Status: hostutility.StatusOK},
		updateOutput: strings.Repeat("line contents\n", 7000),
	}
	store, completed := newUpdateTestStore(updater, newMaintenanceCoordinator(), nil)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if len(final.Output) > jobOutputRingSize {
		t.Fatalf("output bytes = %d, limit = %d", len(final.Output), jobOutputRingSize)
	}
}
