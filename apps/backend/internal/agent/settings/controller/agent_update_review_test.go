package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type blockingCandidateUpdater struct {
	*recoveryRuntimeUpdater
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (u *blockingCandidateUpdater) RunUpdate(
	ctx context.Context,
	command agents.Command,
	onChunk func(string),
) error {
	u.once.Do(func() { close(u.started) })
	select {
	case <-u.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return u.recoveryRuntimeUpdater.RunUpdate(ctx, command, onChunk)
}

func newReviewedUpdateTestController(
	t *testing.T,
	updater RuntimeUpdater,
	selection *recoverySelectionStore,
) (*Controller, *updateTerminalBroadcaster) {
	t.Helper()
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec: agents.ManagedNPMRuntimeSpec{
			Package:        "@example/managed-acp",
			ACPArgs:        []string{"--acp"},
			DefaultVersion: "1.0.0",
		},
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	broadcaster := newUpdateTerminalBroadcaster()
	ctrl.SetJobBroadcaster(broadcaster)
	ctrl.SetRuntimeUpdater(updater)
	ctrl.SetManagedRuntimeSelectionStore(selection)
	ctrl.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(&autoMemorySettings{}))
	ctrl.updateJobStore.onRefresh = nil
	return ctrl, broadcaster
}

func reviewUpdater() *recoveryRuntimeUpdater {
	return &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"1.0.0", "1.1.0"},
			Latest:   "1.1.0",
		},
		current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"},
		currentFound: true,
		probeCaps:    hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.1.0"},
	}
}

func TestReviewedAgentUpdateAdmitsCurrentEvidenceAndPreservesOutcome(t *testing.T) {
	selection := newRecoverySelectionStore()
	ctrl, broadcaster := newReviewedUpdateTestController(t, reviewUpdater(), selection)
	ctx := context.Background()
	if err := ctrl.SetAgentAutomaticUpdates(ctx, "managed-acp", true); err != nil {
		t.Fatalf("SetAgentAutomaticUpdates: %v", err)
	}
	preview, err := ctrl.PreviewAgentUpdate(ctx, "managed-acp", "1.1.0")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	if preview.ReviewedRuntime == nil {
		t.Fatal("ordinary managed update preview did not issue review evidence")
	}
	job, err := ctrl.EnqueueReviewedAgentUpdate(ctx, "managed-acp", "1.1.0", *preview.ReviewedRuntime)
	if err != nil {
		t.Fatalf("EnqueueReviewedAgentUpdate: %v", err)
	}
	final := waitForUpdateStatus(t, broadcaster.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	if final.Operation != string(managedruntime.OperationUpdate) {
		t.Fatalf("operation = %q, want update", final.Operation)
	}
	selection.mu.Lock()
	saved := selection.values["managed-acp\x00@example/managed-acp"]
	selection.mu.Unlock()
	if saved.Version != "1.1.0" {
		t.Fatalf("saved selection = %#v, want 1.1.0", saved)
	}
	policy, err := ctrl.runtimeAutoUpdateStore.Get(ctx, "managed-acp", "npm:@example/managed-acp")
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if policy.Enabled {
		t.Fatal("admitted manual update retained automatic consent")
	}
}

func TestReviewedAgentUpdateRejectsChangedIdentityBeforePolicyWrite(t *testing.T) {
	selection := newRecoverySelectionStore()
	ctrl, _ := newReviewedUpdateTestController(t, reviewUpdater(), selection)
	ctx := context.Background()
	if err := ctrl.SetAgentAutomaticUpdates(ctx, "managed-acp", true); err != nil {
		t.Fatalf("SetAgentAutomaticUpdates: %v", err)
	}
	preview, err := ctrl.PreviewAgentUpdate(ctx, "managed-acp", "1.1.0")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	evidence := *preview.ReviewedRuntime
	evidence.CurrentVersion = "0.9.0"
	if _, err := ctrl.EnqueueReviewedAgentUpdate(ctx, "managed-acp", "1.1.0", evidence); !errors.Is(err, ErrRuntimeReviewChanged) {
		t.Fatalf("EnqueueReviewedAgentUpdate error = %v, want review changed", err)
	}
	if jobs := ctrl.ListAgentUpdateJobs(); len(jobs) != 0 {
		t.Fatalf("stale review created %d jobs", len(jobs))
	}
	policy, err := ctrl.runtimeAutoUpdateStore.Get(ctx, "managed-acp", "npm:@example/managed-acp")
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if !policy.Enabled {
		t.Fatal("stale review mutated automatic consent")
	}
}

func TestReviewedAgentUpdateRejectsChangedSourceAndFamily(t *testing.T) {
	selection := newRecoverySelectionStore()
	ctrl, _ := newReviewedUpdateTestController(t, reviewUpdater(), selection)
	ctx := context.Background()
	preview, err := ctrl.PreviewAgentUpdate(ctx, "managed-acp", "1.1.0")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	for name, mutate := range map[string]func(*dto.AgentRuntimeReviewDTO){
		"source":   func(evidence *dto.AgentRuntimeReviewDTO) { evidence.Source = "native" },
		"family":   func(evidence *dto.AgentRuntimeReviewDTO) { evidence.Family = "v2" },
		"revision": func(evidence *dto.AgentRuntimeReviewDTO) { evidence.RuntimeRevision = 9 },
		"package":  func(evidence *dto.AgentRuntimeReviewDTO) { evidence.Package = "other-package" },
	} {
		t.Run(name, func(t *testing.T) {
			evidence := *preview.ReviewedRuntime
			mutate(&evidence)
			if _, err := ctrl.EnqueueReviewedAgentUpdate(ctx, "managed-acp", "1.1.0", evidence); !errors.Is(err, ErrRuntimeReviewChanged) {
				t.Fatalf("error = %v, want review changed", err)
			}
		})
	}
}

func TestReviewedAgentUpdateDoesNotAdoptExistingMaintenanceJob(t *testing.T) {
	selection := newRecoverySelectionStore()
	updater := &blockingCandidateUpdater{
		recoveryRuntimeUpdater: reviewUpdater(),
		started:                make(chan struct{}),
		release:                make(chan struct{}),
	}
	ctrl, _ := newReviewedUpdateTestController(t, updater, selection)
	t.Cleanup(func() {
		select {
		case <-updater.release:
		default:
			close(updater.release)
		}
	})
	ctx := context.Background()
	preview, err := ctrl.PreviewAgentUpdate(ctx, "managed-acp", "1.1.0")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	legacy, err := ctrl.EnqueueAgentUpdate(ctx, "managed-acp", "1.1.0")
	if err != nil {
		t.Fatalf("legacy EnqueueAgentUpdate: %v", err)
	}
	_, err = ctrl.EnqueueReviewedAgentUpdate(ctx, "managed-acp", "1.1.0", *preview.ReviewedRuntime)
	var conflict *MaintenanceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want maintenance conflict", err)
	}
	if conflict.Active.JobID != legacy.JobID {
		t.Fatalf("conflict job = %q, want %q", conflict.Active.JobID, legacy.JobID)
	}
}

func TestReviewedAgentUpdateGuardedActivationPreservesNewerSelection(t *testing.T) {
	selection := newRecoverySelectionStore()
	selection.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@example/managed-acp",
		Version: "1.0.0",
	}
	updater := &blockingCandidateUpdater{
		recoveryRuntimeUpdater: reviewUpdater(),
		started:                make(chan struct{}),
		release:                make(chan struct{}),
	}
	ctrl, broadcaster := newReviewedUpdateTestController(t, updater, selection)
	ctx := context.Background()
	preview, err := ctrl.PreviewAgentUpdate(ctx, "managed-acp", "1.1.0")
	if err != nil {
		t.Fatalf("PreviewAgentUpdate: %v", err)
	}
	job, err := ctrl.EnqueueReviewedAgentUpdate(ctx, "managed-acp", "1.1.0", *preview.ReviewedRuntime)
	if err != nil {
		t.Fatalf("EnqueueReviewedAgentUpdate: %v", err)
	}
	select {
	case <-updater.started:
	case <-time.After(2 * time.Second):
		t.Fatal("guarded preparation did not start")
	}
	selection.mu.Lock()
	selection.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@example/managed-acp",
		Version: "1.2.0",
	}
	selection.mu.Unlock()
	close(updater.release)
	final := waitForUpdateStatus(t, broadcaster.completed, job.JobID, dto.AgentUpdateJobStatusFailed)
	if final.Error == "" {
		t.Fatal("stale guarded activation did not retain a failure cause")
	}
	selection.mu.Lock()
	saved := selection.values["managed-acp\x00@example/managed-acp"]
	selection.mu.Unlock()
	if saved.Version != "1.2.0" {
		t.Fatalf("stale activation replaced newer selection with %q", saved.Version)
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if len(updater.probe) == 0 || len(updater.prepare) == 0 {
		t.Fatalf("guarded candidate did not prepare/probe: prepare=%v probe=%v", updater.prepare, updater.probe)
	}
}
