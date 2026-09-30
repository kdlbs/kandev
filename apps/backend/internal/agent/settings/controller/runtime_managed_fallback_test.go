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

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.2, AC-AGENTS-RUNTIME-NOTIFY-002.4
func TestNativeHostPreservesManagedFallbackSelectionAndRecovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ag := agents.NewOpenCodeACP()
	spec := ag.ManagedNPMRuntime()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", "8.0.0", spec.DefaultVersionOrPinned()}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeUpdater(updater)
	selection := newRecoverySelectionStore()
	c.SetManagedRuntimeSelectionStore(selection)
	hub := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(hub)
	c.updateJobStore.onRefresh = nil
	ctx := context.Background()
	if err := selection.Save(ctx, ag.ID(), spec.Package, "8.0.0"); err != nil {
		t.Fatal(err)
	}
	if item := c.buildRuntimeUpdateDTO(ctx, ag, true); item == nil || item.CurrentVersion != "" || item.EffectiveVersion != "8.0.0" {
		t.Fatalf("native host hid or misidentified managed fallback: %+v", item)
	}
	preview, err := c.PreviewAgentUpdate(ctx, ag.ID(), "9.0.0")
	if err != nil || preview.CurrentVersion != "" || !strings.Contains(preview.CommandString, "--package=opencode-ai@9.0.0") {
		t.Fatalf("fallback preview: %+v,%v", preview, err)
	}
	for _, target := range []string{"9.0.0", "8.0.0", ""} {
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
