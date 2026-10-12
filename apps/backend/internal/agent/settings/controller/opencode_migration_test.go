package controller

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"go.uber.org/zap"
)

type migrationEventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *migrationEventLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *migrationEventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

type migrationSelectionStore struct {
	mu        sync.Mutex
	selection managedruntime.OpenCodeSelection
	found     bool
	saveErr   error
	saves     int
	events    *migrationEventLog
}

func (s *migrationSelectionStore) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selection, s.found, nil
}

func (s *migrationSelectionStore) SaveOpenCodeSelection(_ context.Context, expectedRevision uint64, selection managedruntime.OpenCodeSelection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	if !s.found || s.selection.Revision != expectedRevision {
		return managedruntime.ErrOpenCodeSelectionRevisionConflict
	}
	s.selection = selection
	s.found = true
	s.events.add("save")
	return nil
}

func (s *migrationSelectionStore) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, nil
}

func (*migrationSelectionStore) Save(context.Context, string, string, string) error { return nil }
func (*migrationSelectionStore) Delete(context.Context, string, string) error       { return nil }

func (s *migrationSelectionStore) selectionSnapshot() managedruntime.OpenCodeSelection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selection
}

type migrationJobUpdater struct {
	mu               sync.Mutex
	selectionEvents  *migrationEventLog
	stageErr         error
	probeErr         error
	probeCaps        hostutility.AgentCapabilities
	refreshErr       error
	refreshCaps      hostutility.AgentCapabilities
	runCommand       []string
	probeCommand     []string
	refreshCommand   []string
	probeCalls       int
	refreshCalls     int
	invalidateCalls  int
	stageCalls       int
	stageStarted     chan struct{}
	allowStageFinish chan struct{}
}

func (u *migrationJobUpdater) CurrentCapabilities(string) (hostutility.AgentCapabilities, bool) {
	return hostutility.AgentCapabilities{}, false
}

func (*migrationJobUpdater) ResolveTarget(context.Context, string) (string, error) {
	return "2.0.18", nil
}

func (u *migrationJobUpdater) ResolveVersions(_ context.Context, packageName string) (RuntimeVersionMetadata, error) {
	if packageName == "opencode-ai" {
		return RuntimeVersionMetadata{Versions: []string{"1.18.32", "1.18.33"}, Latest: "1.18.33"}, nil
	}
	return RuntimeVersionMetadata{Versions: []string{"2.0.18"}, Latest: "2.0.18"}, nil
}

func (u *migrationJobUpdater) RunUpdate(ctx context.Context, command agents.Command, onChunk func(string)) error {
	u.mu.Lock()
	u.stageCalls++
	u.runCommand = append([]string(nil), command.Args()...)
	started, release, err := u.stageStarted, u.allowStageFinish, u.stageErr
	u.mu.Unlock()
	u.selectionEvents.add("stage")
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
	onChunk("staged runtime\n")
	return err
}

func (*migrationJobUpdater) InvalidateExecutionCache(context.Context, string) error { return nil }

func (*migrationJobUpdater) InvalidateExecutionCacheVersion(context.Context, string, string) error {
	return nil
}

func (u *migrationJobUpdater) Refresh(_ context.Context, _ string, command agents.Command) (hostutility.AgentCapabilities, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.refreshCalls++
	u.refreshCommand = append([]string(nil), command.Args()...)
	u.selectionEvents.add("refresh")
	return u.refreshCaps, u.refreshErr
}

func (u *migrationJobUpdater) ProbeIsolated(_ context.Context, _ string, command agents.Command) (hostutility.AgentCapabilities, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.probeCalls++
	u.probeCommand = append([]string(nil), command.Args()...)
	u.selectionEvents.add("probe")
	return u.probeCaps, u.probeErr
}

func (u *migrationJobUpdater) InvalidateCapabilities(string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.invalidateCalls++
	u.selectionEvents.add("invalidate")
}

func (u *migrationJobUpdater) calls() (stage, probe, refresh, invalidate int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stageCalls, u.probeCalls, u.refreshCalls, u.invalidateCalls
}

func (u *migrationJobUpdater) commands() (stage, probe, refresh []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.runCommand...), append([]string(nil), u.probeCommand...), append([]string(nil), u.refreshCommand...)
}

func newOpenCodeMigrationJobStore(
	updater *migrationJobUpdater,
	selectionStore *migrationSelectionStore,
	guardErr error,
) (*AgentUpdateJobStore, <-chan dto.AgentUpdateJobDTO, *int, *int) {
	hub := newUpdateTerminalBroadcaster()
	store := NewAgentUpdateJobStore(hub, zap.NewNop(), updater, nil, nil, selectionStore)
	store.SetOpenCodeSelectionReader(selectionStore)
	store.SetOpenCodeSelections(selectionStore)
	guardCalls := 0
	releases := 0
	store.SetOpenCodeMigrationGuard(func(ctx context.Context) (context.Context, func(), error) {
		guardCalls++
		updater.selectionEvents.add("guard")
		if guardErr != nil {
			return nil, nil, guardErr
		}
		return ctx, func() {
			releases++
			updater.selectionEvents.add("release")
		}, nil
	})
	return store, hub.completed, &guardCalls, &releases
}

func newMigrationTestState(events *migrationEventLog) *migrationSelectionStore {
	return &migrationSelectionStore{
		selection: managedruntime.OpenCodeSelection{
			SchemaVersion:         1,
			Family:                managedruntime.OpenCodeFamilyV1,
			Source:                managedruntime.OpenCodeSourceManaged,
			Package:               "opencode-ai",
			SelectedVersion:       "1.18.32",
			AppliedDefaultVersion: "1.18.32",
			Revision:              4,
		},
		found:  true,
		events: events,
	}
}

func TestOpenCodeMigrationActivationBoundaries(t *testing.T) {
	stageErr := errors.New("npm staging failed")
	probeErr := errors.New("candidate handshake failed")
	guardErr := errors.New("an OpenCode session is active")
	saveErr := errors.New("settings write failed")
	refreshErr := errors.New("live capability refresh failed")
	tests := []struct {
		name          string
		stageErr      error
		probeErr      error
		probeStatus   hostutility.Status
		guardErr      error
		saveErr       error
		refreshErr    error
		wantStatus    dto.AgentUpdateJobStatus
		wantActivated bool
		wantRefresh   bool
	}{
		{name: "stage failure retains v1", stageErr: stageErr, wantStatus: dto.AgentUpdateJobStatusFailed},
		{name: "probe failure retains v1", probeErr: probeErr, probeStatus: hostutility.StatusOK, wantStatus: dto.AgentUpdateJobStatusFailed},
		{name: "unhealthy candidate retains v1", probeStatus: hostutility.StatusFailed, wantStatus: dto.AgentUpdateJobStatusFailed},
		{name: "active work blocks activation", guardErr: guardErr, wantStatus: dto.AgentUpdateJobStatusFailed},
		{name: "persistence failure retains v1", saveErr: saveErr, wantStatus: dto.AgentUpdateJobStatusFailed},
		{name: "discovery failure leaves v2 selected", refreshErr: refreshErr, wantStatus: dto.AgentUpdateJobStatusSucceeded, wantActivated: true, wantRefresh: true},
		{name: "successful activation selects v2", wantStatus: dto.AgentUpdateJobStatusSucceeded, wantActivated: true, wantRefresh: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := &migrationEventLog{}
			state := newMigrationTestState(events)
			state.saveErr = tt.saveErr
			probeCaps := hostutility.AgentCapabilities{Status: tt.probeStatus, AgentVersion: "2.0.18"}
			if tt.probeStatus == "" {
				probeCaps.Status = hostutility.StatusOK
			}
			updater := &migrationJobUpdater{
				selectionEvents: events,
				stageErr:        tt.stageErr,
				probeErr:        tt.probeErr,
				probeCaps:       probeCaps,
				refreshErr:      tt.refreshErr,
				refreshCaps:     hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "2.0.18"},
			}
			store, completed, guardCalls, releases := newOpenCodeMigrationJobStore(updater, state, tt.guardErr)
			job, err := store.EnqueueOpenCodeMigration(
				"opencode-acp",
				opencodeV2Spec(t),
				"2.0.18",
				4,
			)
			if err != nil {
				t.Fatalf("EnqueueOpenCodeMigration: %v", err)
			}
			finished := waitForUpdateStatus(t, completed, job.ID, tt.wantStatus)
			selection := state.selectionSnapshot()
			if tt.wantActivated {
				if selection.Family != managedruntime.OpenCodeFamilyV2 || selection.Source != managedruntime.OpenCodeSourceManaged || selection.Package != "@opencode/cli" || selection.SelectedVersion != "2.0.18" || selection.Revision != 5 {
					t.Fatalf("activated selection = %+v", selection)
				}
				if finished.TargetFamily != "v2" || finished.RuntimeRevision != 5 || finished.Operation != string(managedruntime.OperationMigrate) {
					t.Fatalf("completed job migration fields = %+v", finished)
				}
			} else if selection.Family != managedruntime.OpenCodeFamilyV1 || selection.Revision != 4 {
				t.Fatalf("failed activation changed saved selection: %+v", selection)
			}
			stageCalls, probeCalls, refreshCalls, invalidations := updater.calls()
			wantProbes, wantRefreshes := 1, 0
			if tt.stageErr != nil {
				wantProbes = 0
			}
			if tt.probeErr == nil && tt.probeStatus == "" || tt.probeErr == nil && tt.probeStatus == hostutility.StatusOK {
				if tt.stageErr == nil {
					wantProbes = 1
				}
			}
			if tt.wantRefresh {
				wantRefreshes = 1
			}
			wantStageCalls := 1
			if tt.stageErr != nil {
				wantStageCalls = 2 // The job retries once after repairing the exact npm execution tree.
			}
			if stageCalls != wantStageCalls || probeCalls != wantProbes || refreshCalls != wantRefreshes {
				t.Fatalf("stage/probe/refresh calls = %d/%d/%d, want %d/%d/%d", stageCalls, probeCalls, refreshCalls, wantStageCalls, wantProbes, wantRefreshes)
			}
			wantGuardCalls := 0
			if probeCalls == 1 && updater.probeErr == nil && updater.probeCaps.Status == hostutility.StatusOK {
				wantGuardCalls = 1
			}
			if *guardCalls != wantGuardCalls {
				t.Fatalf("guard calls = %d, want %d", *guardCalls, wantGuardCalls)
			}
			wantReleases := wantGuardCalls
			if tt.guardErr != nil {
				wantReleases = 0
			}
			if *releases != wantReleases {
				t.Fatalf("guard releases = %d, want %d", *releases, wantReleases)
			}
			wantInvalidations := 0
			if tt.wantRefresh {
				wantInvalidations = 1
			}
			if invalidations != wantInvalidations {
				t.Fatalf("capability invalidations = %d, want %d", invalidations, wantInvalidations)
			}
			if tt.refreshErr != nil && finished.RefreshError == "" {
				t.Fatal("post-activation discovery error was not surfaced")
			}
			wantSaveAttempts := 0
			if tt.wantActivated || tt.saveErr != nil {
				wantSaveAttempts = 1
			}
			if state.saves != wantSaveAttempts {
				t.Fatalf("selection writes = %d, want %d", state.saves, wantSaveAttempts)
			}
			stage, probe, refresh := updater.commands()
			if !reflect.DeepEqual(stage, opencodeV2Spec(t).CacheUpdateCommand("2.0.18").Args()) {
				t.Fatalf("staging argv = %#v", stage)
			}
			if probeCalls == 1 && !reflect.DeepEqual(probe, opencodeV2Spec(t).ACPCommand("2.0.18").Args()) {
				t.Fatalf("isolated probe argv = %#v", probe)
			}
			if tt.wantRefresh && !reflect.DeepEqual(refresh, opencodeV2Spec(t).ACPCommand("2.0.18").Args()) {
				t.Fatalf("live refresh argv = %#v", refresh)
			}
			if tt.wantActivated {
				gotEvents := events.snapshot()
				wantEvents := []string{"stage", "probe", "guard", "save", "invalidate", "release", "refresh"}
				if !reflect.DeepEqual(gotEvents, wantEvents) {
					t.Fatalf("migration event order = %v", gotEvents)
				}
			}
		})
	}
}

func TestOpenCodeMigrationRejectsStaleRevisionBeforeStaging(t *testing.T) {
	events := &migrationEventLog{}
	state := newMigrationTestState(events)
	updater := &migrationJobUpdater{
		selectionEvents: events,
		probeCaps:       hostutility.AgentCapabilities{Status: hostutility.StatusOK},
	}
	store, completed, _, _ := newOpenCodeMigrationJobStore(updater, state, nil)
	job, err := store.EnqueueOpenCodeMigration("opencode-acp", opencodeV2Spec(t), "2.0.18", 3)
	if err != nil {
		t.Fatalf("EnqueueOpenCodeMigration: %v", err)
	}
	finished := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusFailed)
	if finished.Error == "" || len(events.snapshot()) != 0 {
		t.Fatalf("stale revision proceeded into migration: error %q, events %v", finished.Error, events.snapshot())
	}
	if state.saves != 0 {
		t.Fatalf("stale revision wrote selection %d times", state.saves)
	}
}

func TestOpenCodeMigrationCoalescesDuplicateActiveJobs(t *testing.T) {
	events := &migrationEventLog{}
	state := newMigrationTestState(events)
	updater := &migrationJobUpdater{
		selectionEvents:  events,
		probeCaps:        hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "2.0.18"},
		refreshCaps:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "2.0.18"},
		stageStarted:     make(chan struct{}),
		allowStageFinish: make(chan struct{}),
	}
	store, completed, _, _ := newOpenCodeMigrationJobStore(updater, state, nil)
	first, err := store.EnqueueOpenCodeMigration("opencode-acp", opencodeV2Spec(t), "2.0.18", 4)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	select {
	case <-updater.stageStarted:
	case <-time.After(time.Second):
		t.Fatal("migration did not enter staging")
	}
	second, err := store.EnqueueOpenCodeMigration("opencode-acp", opencodeV2Spec(t), "2.0.18", 4)
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate job ID = %s, want existing %s", second.ID, first.ID)
	}
	close(updater.allowStageFinish)
	waitForUpdateStatus(t, completed, first.ID, dto.AgentUpdateJobStatusSucceeded)
	stageCalls, _, _, _ := updater.calls()
	if stageCalls != 1 {
		t.Fatalf("duplicate migration staged runtime %d times", stageCalls)
	}
}

func TestOpenCodeMigrationPreviewAndEnqueueBindFamilyAndRevision(t *testing.T) {
	selection := newMigrationTestState(&migrationEventLog{})
	updater := &migrationJobUpdater{
		selectionEvents:  selection.events,
		probeCaps:        hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "2.0.18"},
		refreshCaps:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "2.0.18"},
		stageStarted:     make(chan struct{}),
		allowStageFinish: make(chan struct{}),
	}
	defer func() {
		select {
		case <-updater.allowStageFinish:
		default:
			close(updater.allowStageFinish)
		}
	}()
	provider := agents.NewOpenCodeACP()
	ctrl := newTestController(map[string]agents.Agent{provider.ID(): provider})
	ctrl.SetRuntimeUpdater(updater)
	ctrl.SetManagedRuntimeSelectionStore(selection)
	hub := newUpdateTerminalBroadcaster()
	ctrl.SetJobBroadcaster(hub)
	ctrl.updateJobStore.onRefresh = nil
	ctrl.SetOpenCodeMigrationGuard(func(ctx context.Context) (context.Context, func(), error) {
		return ctx, func() {}, nil
	})

	current, err := ctrl.PreviewAgentUpdate(context.Background(), provider.ID())
	if err != nil {
		t.Fatalf("v1 preview: %v", err)
	}
	if current.Family != "v1" || !current.MigrationAvailable || current.RuntimeRevision != 4 {
		t.Fatalf("current runtime preview = %+v", current)
	}
	preview, err := ctrl.PreviewAgentUpdateFamily(context.Background(), provider.ID(), "2.0.18", "v2")
	if err != nil {
		t.Fatalf("v2 preview: %v", err)
	}
	if preview.Package != "@opencode/cli" || preview.TargetFamily != "v2" || preview.RuntimeRevision != 4 || preview.Operation != string(managedruntime.OperationMigrate) {
		t.Fatalf("migration preview = %+v", preview)
	}
	if _, err := ctrl.EnqueueOpenCodeMigration(context.Background(), provider.ID(), "2.0.18", 3); !errors.Is(err, managedruntime.ErrOpenCodeSelectionRevisionConflict) {
		t.Fatalf("stale migration request error = %v", err)
	}
	job, err := ctrl.EnqueueOpenCodeMigration(context.Background(), provider.ID(), "2.0.18", 4)
	if err != nil {
		t.Fatalf("enqueue migration: %v", err)
	}
	select {
	case <-updater.stageStarted:
	case <-time.After(time.Second):
		t.Fatal("migration did not enter staging")
	}
	if job.TargetFamily != "v2" || job.RuntimeRevision != 4 || !job.Migration {
		t.Fatalf("queued migration job = %+v", job)
	}
	close(updater.allowStageFinish)
	waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
}

func opencodeV2Spec(t *testing.T) agents.ManagedNPMRuntimeSpec {
	t.Helper()
	spec, err := agents.NewOpenCodeACP().ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		t.Fatalf("ManagedNPMRuntimeForFamily(v2): %v", err)
	}
	return spec
}
