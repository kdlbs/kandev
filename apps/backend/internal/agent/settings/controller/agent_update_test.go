package controller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"go.uber.org/zap"
)

type fakeRuntimeUpdater struct {
	mu              sync.Mutex
	current         hostutility.AgentCapabilities
	currentFound    bool
	target          string
	resolveErr      error
	runErr          error
	refreshCaps     hostutility.AgentCapabilities
	refreshErr      error
	runCommand      []string
	refreshCommand  []string
	updateOutput    string
	runStarted      chan struct{}
	releaseRun      chan struct{}
	runCalls        int
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

func waitForUpdateStatus(
	t *testing.T,
	store *AgentUpdateJobStore,
	jobID string,
	statuses ...dto.AgentUpdateJobStatus,
) *dto.AgentUpdateJobDTO {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, ok := store.Get(jobID)
		if ok {
			for _, status := range statuses {
				if snapshot.Status == status {
					return snapshot
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %v", jobID, statuses)
	return nil
}

func managedRuntimeSpec() agents.ManagedNPMRuntimeSpec {
	return agents.ManagedNPMRuntimeSpec{
		Package: "@example/managed-acp",
		ACPArgs: []string{"--acp"},
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
	hub := &captureBroadcaster{}
	refreshed := make(chan struct{}, 1)
	store := NewAgentUpdateJobStore(
		hub,
		zap.NewNop(),
		updater,
		newMaintenanceCoordinator(),
		func() { refreshed <- struct{}{} },
	)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, store, job.ID, dto.AgentUpdateJobStatusSucceeded)
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
	store := NewAgentUpdateJobStore(
		&captureBroadcaster{},
		zap.NewNop(),
		updater,
		newMaintenanceCoordinator(),
		func() { refreshed = true },
	)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, store, job.ID, dto.AgentUpdateJobStatusSucceeded)
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
	store := NewAgentUpdateJobStore(
		&captureBroadcaster{},
		zap.NewNop(),
		updater,
		newMaintenanceCoordinator(),
		nil,
	)
	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, store, job.ID, dto.AgentUpdateJobStatusFailed)
	if !strings.Contains(final.Error, "registry unavailable") {
		t.Fatalf("Error = %q", final.Error)
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if updater.runCalls != 0 || updater.refreshCalls != 0 {
		t.Fatalf("calls after registry failure: update=%d refresh=%d", updater.runCalls, updater.refreshCalls)
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
			store := NewAgentUpdateJobStore(
				&captureBroadcaster{},
				zap.NewNop(),
				test.updater,
				newMaintenanceCoordinator(),
				func() { refreshed = true },
			)
			job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
			if err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			final := waitForUpdateStatus(t, store, job.ID, dto.AgentUpdateJobStatusFailed)
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
	store := NewAgentUpdateJobStore(
		&captureBroadcaster{},
		zap.NewNop(),
		updater,
		coordinator,
		nil,
	)
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
	waitForUpdateStatus(t, store, first.ID, dto.AgentUpdateJobStatusSucceeded)

	retry, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("retry enqueue: %v", err)
	}
	if retry.ID == first.ID {
		t.Fatal("retry reused a completed update job")
	}
	waitForUpdateStatus(t, store, retry.ID, dto.AgentUpdateJobStatusSucceeded)
}

func TestAgentUpdateOutputIsBounded(t *testing.T) {
	updater := &fakeRuntimeUpdater{
		target:       "1.1.0",
		refreshCaps:  hostutility.AgentCapabilities{Status: hostutility.StatusOK},
		updateOutput: strings.Repeat("line contents\n", 7000),
	}
	store := NewAgentUpdateJobStore(
		&captureBroadcaster{},
		zap.NewNop(),
		updater,
		newMaintenanceCoordinator(),
		nil,
	)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, store, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if len(final.Output) > jobOutputRingSize {
		t.Fatalf("output bytes = %d, limit = %d", len(final.Output), jobOutputRingSize)
	}
}
