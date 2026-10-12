package controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.8, AC-AGENTS-RUNTIME-NOTIFY-002.2, AC-AGENTS-RUNTIME-NOTIFY-002.4
func TestNativeHostPreservesManagedFallbackSelectionAndRecovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ag := agents.NewOpenCodeACP()
	spec := ag.ManagedNPMRuntime()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "1.19.0", Versions: []string{"1.19.0", "1.18.32", spec.DefaultVersionOrPinned()}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.19.0"}}
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeUpdater(updater)
	selection := newRecoverySelectionStore()
	c.SetManagedRuntimeSelectionStore(selection)
	hub := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(hub)
	c.updateJobStore.onRefresh = nil
	ctx := context.Background()
	if err := selection.Save(ctx, ag.ID(), spec.Package, "1.18.32"); err != nil {
		t.Fatal(err)
	}
	if item := c.buildRuntimeUpdateDTO(ctx, ag, true); item == nil || item.CurrentVersion != "" || item.EffectiveVersion != "1.18.32" {
		t.Fatalf("native host hid or misidentified managed fallback: %+v", item)
	}
	preview, err := c.PreviewAgentUpdate(ctx, ag.ID(), "1.19.0")
	if err != nil || preview.CurrentVersion != "1.18.32" || !strings.Contains(preview.CommandString, "--package=opencode-ai@1.19.0") {
		t.Fatalf("fallback preview: %+v,%v", preview, err)
	}
	for _, target := range []string{"1.19.0", "1.18.32", ""} {
		var job *dto.AgentUpdateJobDTO
		if target == "" {
			job, err = c.EnqueueAgentUpdateUseDefault(ctx, ag.ID())
		} else {
			job, err = c.EnqueueAgentUpdate(ctx, ag.ID(), target)
		}
		if err != nil {
			t.Fatalf("fallback target %q: %v", target, err)
		}
		waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
		caps, _ := updater.CurrentCapabilities(ag.ID())
		if caps.AgentVersion != "1.0.0" {
			t.Fatalf("fallback published candidate as native host capabilities: %+v", caps)
		}
	}
	if _, found, err := selection.Get(ctx, ag.ID(), spec.Package); err != nil || found {
		t.Fatalf("fallback return-to-default: found=%v,err=%v", found, err)
	}
	for _, command := range updater.prepare {
		if !strings.HasPrefix(command, "npm --prefix ") || strings.Contains(command, "install -g") {
			t.Fatalf("fallback mutated external installation: %s", command)
		}
	}
	for _, command := range updater.probe {
		if !strings.HasPrefix(command, "npx --yes ") {
			t.Fatalf("fallback probed external native: %s", command)
		}
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.8
func TestManagedFallbackPreviewReportsValidatedSelectionAsCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ag := agents.NewOpenCodeACP()
	spec := ag.ManagedNPMRuntime()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "1.19.0", Versions: []string{"1.19.0", "1.18.30", spec.DefaultVersionOrPinned()}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.19.0"}}
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeUpdater(updater)
	selection := newRecoverySelectionStore()
	c.SetManagedRuntimeSelectionStore(selection)
	hub := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(hub)
	c.updateJobStore.onRefresh = nil
	ctx := context.Background()

	preview, err := c.PreviewAgentUpdate(ctx, ag.ID(), "1.19.0")
	if err != nil || preview.CurrentVersion != "" || preview.Operation != "repair" {
		t.Fatalf("unselected fallback must stay unknown and repair: %+v,%v", preview, err)
	}

	if err := selection.Save(ctx, ag.ID(), spec.Package, "1.18.30"); err != nil {
		t.Fatal(err)
	}
	for target, operation := range map[string]string{"1.19.0": "update", "1.18.30": "up_to_date"} {
		preview, err = c.PreviewAgentUpdate(ctx, ag.ID(), target)
		if err != nil || preview.CurrentVersion != "1.18.30" || preview.Operation != operation {
			t.Fatalf("fallback preview %s: want current 1.18.30 and %s, got %+v,%v", target, operation, preview, err)
		}
	}

	job, err := c.EnqueueAgentUpdate(ctx, ag.ID(), "1.19.0")
	if err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	preview, err = c.PreviewAgentUpdate(ctx, ag.ID(), "1.19.0")
	if err != nil || preview.CurrentVersion != "1.19.0" || preview.Operation != "up_to_date" {
		t.Fatalf("reopened fallback preview after activation: %+v,%v", preview, err)
	}
	if caps, _ := updater.CurrentCapabilities(ag.ID()); caps.AgentVersion != "1.0.0" {
		t.Fatalf("fallback published candidate as native host capabilities: %+v", caps)
	}

	job, err = c.EnqueueAgentUpdate(ctx, ag.ID(), "1.19.0")
	if err != nil {
		t.Fatal(err)
	}
	finished := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
	if finished.Operation != "up_to_date" || finished.CurrentVersion != "1.19.0" {
		t.Fatalf("selected fallback version must be up to date: %+v", finished)
	}
}
