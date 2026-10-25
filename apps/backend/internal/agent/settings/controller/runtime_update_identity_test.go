package controller

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.5, AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestAutomaticOutcomeRetainsOriginalRuntimeIdentityAfterHostChanges(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	ag := agents.NewOpenCodeACP()
	spec := ag.ManagedNPMRuntime()
	previous := spec.DefaultVersionOrPinned()
	updater := &blockedAutoUpdater{recoveryRuntimeUpdater: &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "1.19.0", Versions: []string{"1.19.0", previous}}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.19.0"}}, probed: make(chan struct{}), release: make(chan struct{})}
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeUpdater(updater)
	c.SetManagedRuntimeSelectionStore(newRecoverySelectionStore())
	c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(&autoMemorySettings{}))
	hub := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(hub)
	c.updateJobStore.onRefresh = nil
	n := &runtimeNoticeCapture{}
	c.SetRuntimeUpdateNotifier(n)
	var once sync.Once
	release := func() { once.Do(func() { close(updater.release) }) }
	t.Cleanup(func() { release(); c.updateJobStore.automaticWorkers.Wait() })
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, ag.ID(), true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updater.probed:
	case <-time.After(2 * time.Second):
		t.Fatal("candidate probe did not start")
	}
	if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
		t.Fatal(err)
	}
	release()
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, ag.ID(), "npm:"+spec.Package)
	if err != nil || policy.Outcome.Status != "failed" {
		t.Fatalf("identity change lost original outcome: %+v,%v", policy, err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	found := false
	for _, notice := range n.notices {
		if notice.Status == "failed" && notice.RuntimeID == "npm:"+spec.Package {
			found = true
		}
	}
	if !found {
		t.Fatalf("identity change lost recovery notice: %+v", n.notices)
	}
}
