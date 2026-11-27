package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestAgentUpdateJobKeepsHostCurrentVersionWhenSelectionReadFails(t *testing.T) {
	selectionStore := newRecoverySelectionStore()
	selectionStore.err = errors.New("selection store locked")
	updater := &recoveryRuntimeUpdater{
		metadata:     RuntimeVersionMetadata{Versions: []string{"1.1.0", "1.0.0"}, Latest: "1.1.0"},
		current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.0"},
		currentFound: true,
	}
	ag := &managedTestAgent{
		testAgent: testAgent{id: "managed-acp", name: "Managed", enabled: true},
		spec:      managedRuntimeSpec(),
	}
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	ctrl.SetManagedRuntimeSelectionStore(selectionStore)
	hub := newUpdateTerminalBroadcaster()
	ctrl.SetJobBroadcaster(hub)
	ctrl.SetRuntimeUpdater(updater)

	job, err := ctrl.EnqueueAgentUpdate(context.Background(), ag.ID(), "1.1.0")
	if err != nil {
		t.Fatalf("EnqueueAgentUpdate: %v", err)
	}
	finished := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusFailed)
	if finished.CurrentVersion != "1.0.0" {
		t.Fatalf("failed job current version = %q, want host version 1.0.0", finished.CurrentVersion)
	}
}
