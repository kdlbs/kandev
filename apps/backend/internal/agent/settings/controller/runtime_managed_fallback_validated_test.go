package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type validatingSelectionStore struct {
	*recoverySelectionStore
	mu        sync.Mutex
	validated map[string]managedruntime.Selection
	saveErr   error
}

func newValidatingSelectionStore() *validatingSelectionStore {
	return &validatingSelectionStore{
		recoverySelectionStore: newRecoverySelectionStore(),
		validated:              make(map[string]managedruntime.Selection),
	}
}

func (s *validatingSelectionStore) GetValidated(
	_ context.Context,
	agentName string,
	packageName string,
) (managedruntime.Selection, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.validated[agentName]
	if !ok || record.Package != packageName {
		return managedruntime.Selection{}, false, nil
	}
	return record, true, nil
}

func (s *validatingSelectionStore) SaveValidated(
	_ context.Context,
	agentName string,
	packageName string,
	version string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.validated[agentName] = managedruntime.Selection{Package: packageName, Version: version}
	return nil
}

type fallbackHarness struct {
	agent     *agents.OpenCodeACP
	spec      agents.ManagedNPMRuntimeSpec
	updater   *recoveryRuntimeUpdater
	selection *validatingSelectionStore
}

func newFallbackHarness(t *testing.T) *fallbackHarness {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ag := agents.NewOpenCodeACP()
	spec := ag.ManagedNPMRuntime()
	defaultVersion := spec.DefaultVersionOrPinned()
	return &fallbackHarness{
		agent: ag,
		spec:  spec,
		updater: &recoveryRuntimeUpdater{
			metadata:     RuntimeVersionMetadata{Latest: "1.19.0", Versions: []string{"1.19.0", "1.18.30", defaultVersion}},
			currentFound: true,
			current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"},
			probeCaps:    hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: defaultVersion},
		},
		selection: newValidatingSelectionStore(),
	}
}

// controller builds a controller over the harness's persistent stores, so a
// second call simulates a backend restart.
func (h *fallbackHarness) controller() (*Controller, *updateTerminalBroadcaster) {
	c := newTestController(map[string]agents.Agent{h.agent.ID(): h.agent})
	c.SetRuntimeUpdater(h.updater)
	c.SetManagedRuntimeSelectionStore(h.selection)
	hub := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(hub)
	c.updateJobStore.onRefresh = nil
	return c, hub
}

func assertFallbackPreview(t *testing.T, c *Controller, target, current, operation string) {
	t.Helper()
	preview, err := c.PreviewAgentUpdate(context.Background(), "opencode-acp", target)
	if err != nil || preview.CurrentVersion != current || preview.Operation != operation {
		t.Fatalf("preview %s: want current %q and %s, got %+v,%v", target, current, operation, preview, err)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.8, AC-AGENTS-RUNTIME-UPDATES-001.6
func TestManagedFallbackPreviewReportsValidatedDefaultAsCurrent(t *testing.T) {
	h := newFallbackHarness(t)
	ctx := context.Background()
	defaultVersion := h.spec.DefaultVersionOrPinned()
	if err := h.selection.Save(ctx, h.agent.ID(), h.spec.Package, "1.18.30"); err != nil {
		t.Fatal(err)
	}
	c, hub := h.controller()

	job, err := c.EnqueueAgentUpdateUseDefault(ctx, h.agent.ID())
	if err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	if _, found, _ := h.selection.Get(ctx, h.agent.ID(), h.spec.Package); found {
		t.Fatal("return to default persisted an operator selection")
	}
	assertFallbackPreview(t, c, "1.19.0", defaultVersion, "update")
	assertFallbackPreview(t, c, defaultVersion, defaultVersion, "up_to_date")

	restarted, _ := h.controller()
	assertFallbackPreview(t, restarted, "1.19.0", defaultVersion, "update")
	if caps, _ := h.updater.CurrentCapabilities(h.agent.ID()); caps.AgentVersion != "1.0.0" {
		t.Fatalf("fallback published candidate as native host capabilities: %+v", caps)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.8
func TestManagedFallbackIgnoresValidatedVersionThatIsNoLongerEffective(t *testing.T) {
	h := newFallbackHarness(t)
	ctx := context.Background()
	if err := h.selection.SaveValidated(ctx, h.agent.ID(), h.spec.Package, "1.18.30"); err != nil {
		t.Fatal(err)
	}
	c, _ := h.controller()
	assertFallbackPreview(t, c, "1.19.0", "", "repair")

	if err := h.selection.SaveValidated(ctx, h.agent.ID(), "other-package", h.spec.DefaultVersionOrPinned()); err != nil {
		t.Fatal(err)
	}
	assertFallbackPreview(t, c, "1.19.0", "", "repair")
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.8
func TestManagedFallbackRecordFailureKeepsActivationSucceeded(t *testing.T) {
	h := newFallbackHarness(t)
	ctx := context.Background()
	h.selection.saveErr = errors.New("settings unavailable")
	if err := h.selection.Save(ctx, h.agent.ID(), h.spec.Package, "1.18.30"); err != nil {
		t.Fatal(err)
	}
	c, hub := h.controller()

	job, err := c.EnqueueAgentUpdateUseDefault(ctx, h.agent.ID())
	if err != nil {
		t.Fatal(err)
	}
	finished := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	if finished.Error != "" {
		t.Fatalf("record failure surfaced as job error: %+v", finished)
	}
	if _, found, _ := h.selection.Get(ctx, h.agent.ID(), h.spec.Package); found {
		t.Fatal("return to default was not committed")
	}
	assertFallbackPreview(t, c, "1.19.0", "", "repair")
}
