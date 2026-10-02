package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.5
func TestRejectedManualRequestPreservesAutomaticConsent(t *testing.T) {
	spec := agents.NewGemini().ManagedNPMRuntime()
	previous := spec.DefaultVersionOrPinned()
	updater := &blockedAutoUpdater{recoveryRuntimeUpdater: &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}, probed: make(chan struct{}), release: make(chan struct{})}
	c, hub := autoController(t, updater, newRecoverySelectionStore(), &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnqueueAgentUpdate(ctx, "gemini", "invalid"); !errors.Is(err, ErrRuntimeUpdateTargetInvalid) {
		t.Fatalf("invalid target: %v", err)
	}
	policy, _ := c.runtimeAutoUpdateStore.Get(ctx, "gemini", "npm:"+spec.Package)
	if !policy.Enabled {
		t.Fatal("invalid manual target withdrew consent")
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeSignal(t, updater.probed, "candidate probe")
	var once sync.Once
	release := func() { once.Do(func() { close(updater.release) }) }
	t.Cleanup(func() { release(); c.updateJobStore.automaticWorkers.Wait() })
	_, err := c.EnqueueAgentUpdateUseDefault(ctx, "gemini")
	var conflict *MaintenanceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("busy automatic request must report conflict: %v", err)
	}
	policy, _ = c.runtimeAutoUpdateStore.Get(ctx, "gemini", "npm:"+spec.Package)
	if !policy.Enabled {
		t.Fatal("busy manual request withdrew consent")
	}
	release()
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.5, AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestManualSelectionAcceptedAfterOutcomeBeforeRefreshCallback(t *testing.T) {
	previous := agents.NewGemini().ManagedNPMRuntime().DefaultVersionOrPinned()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}
	c, hub := autoController(t, updater, newRecoverySelectionStore(), &autoMemorySettings{})
	refreshing, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c.updateJobStore.onRefresh = func() { once.Do(func() { close(refreshing); <-release }) }
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() { releaseCallback(); c.updateJobStore.automaticWorkers.Wait() })
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeSignal(t, refreshing, "capability refresh")
	old := c.ListAgentUpdateJobs()[0]
	job, err := c.EnqueueAgentUpdateUseDefault(ctx, "gemini")
	if err != nil || job.JobID == old.JobID || job.Automatic {
		t.Fatalf("manual default request dropped during refresh: %+v,%v", job, err)
	}
	policy, _ := c.runtimeAutoUpdateStore.Get(ctx, "gemini", "npm:"+agents.NewGemini().ManagedNPMRuntime().Package)
	if policy.Enabled || policy.Outcome.Status != "succeeded" {
		t.Fatalf("outcome must be retained before admitting manual selection: %+v", policy)
	}
	releaseCallback()
	completed := map[string]bool{}
	for range 2 {
		var result dto.AgentUpdateJobDTO
		select {
		case result = <-hub.completed:
		case <-time.After(2 * time.Second):
			t.Fatal("terminal update delivery timed out")
		}
		if result.Status != dto.AgentUpdateJobStatusSucceeded {
			t.Fatalf("terminal activation: %+v", result)
		}
		completed[result.JobID] = true
	}
	if !completed[job.JobID] || !completed[old.JobID] {
		t.Fatalf("terminal delivery dropped an activation: %+v", completed)
	}
}

func waitForRuntimeSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not start", name)
	}
}
