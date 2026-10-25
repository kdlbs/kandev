package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"testing"
)

func TestOpenCodeAutomaticUpdateKeepsSelectedFamily(t *testing.T) {
	for _, family := range []managedruntime.OpenCodeFamily{managedruntime.OpenCodeFamilyV1, managedruntime.OpenCodeFamilyV2} {
		t.Run(string(family), func(t *testing.T) {
			ag := agents.NewOpenCodeACP()
			spec, err := ag.ManagedNPMRuntimeForFamily(family)
			if err != nil {
				t.Fatal(err)
			}
			previous := spec.DefaultVersionOrPinned()
			target := "1.19.0"
			if family == managedruntime.OpenCodeFamilyV2 {
				target = "2.0.20"
			}
			selection := newMigrationTestState(&migrationEventLog{})
			selection.selection.Family, selection.selection.Package = family, spec.Package
			selection.selection.SelectedVersion, selection.selection.AppliedDefaultVersion = previous, previous
			updater := &recoveryRuntimeUpdater{
				metadata:     RuntimeVersionMetadata{Latest: "3.0.0", Versions: []string{previous, target, "3.0.0"}},
				currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous},
				probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: target},
			}
			c := newTestController(map[string]agents.Agent{ag.ID(): ag})
			c.SetRuntimeUpdater(updater)
			c.SetManagedRuntimeSelectionStore(selection)
			c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(&autoMemorySettings{}))
			hub := newUpdateTerminalBroadcaster()
			c.SetJobBroadcaster(hub)
			c.updateJobStore.onRefresh = nil
			t.Cleanup(c.updateJobStore.automaticWorkers.Wait)
			ctx := context.Background()
			if err := c.SetAgentAutomaticUpdates(ctx, ag.ID(), true); err != nil {
				t.Fatal(err)
			}
			statuses, err := c.ListAgentUpdateStatuses(ctx)
			if err != nil {
				t.Fatal(err)
			}
			status := statuses.Statuses[0]
			if status.RuntimeID != "npm:"+spec.Package || status.LatestVersion != target || !status.AutoUpdateSupported {
				t.Fatalf("selected runtime status = %+v", status)
			}
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			jobs := c.ListAgentUpdateJobs()
			if len(jobs) != 1 {
				t.Fatalf("jobs = %+v", jobs)
			}
			waitForUpdateStatus(t, hub.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
			got := selection.selectionSnapshot()
			if got.Family != family || got.Package != spec.Package || got.SelectedVersion != target {
				t.Fatalf("automatic update changed selected family: %+v", got)
			}
		})
	}
}
