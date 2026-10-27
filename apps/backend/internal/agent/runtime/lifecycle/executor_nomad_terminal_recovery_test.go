package lifecycle

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
)

type resumeRecordingCoordinator struct {
	recordingCleanupCoordinator
	mu      sync.Mutex
	resumed []string
}

func (c *resumeRecordingCoordinator) ResumeCleanup(_ context.Context, record *models.ExecutorRunning) (bool, error) {
	state, err := nomad.EnvironmentStateFromMetadata(record.Metadata)
	if err != nil || !nomadCleanupResumable(state) {
		return false, nil
	}
	c.mu.Lock()
	c.resumed = append(c.resumed, record.SessionID)
	c.mu.Unlock()
	return true, nil
}

func TestNomadRecoverInstancesResumesCleanupOfTerminalRows(t *testing.T) {
	for _, status := range []string{models.ExecutorRunningStatusStopped, models.ExecutorRunningStatusFailed, models.ExecutorRunningStatusComplete} {
		f := prepareNomadCleanupFixture(t)
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) { s.Phase = nomadPhaseCleanupPending })
		coordinator := &resumeRecordingCoordinator{}
		f.runtime.setCleanupCoordinator(coordinator)
		record := nomadFixtureRecord(f)
		record.Status = status
		if _, err := f.runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{record}); err != nil {
			t.Fatalf("%s: RecoverInstances: %v", status, err)
		}
		if len(coordinator.resumed) != 1 {
			t.Fatalf("%s: cleanup resumed %d times, want 1 for a retained terminal row", status, len(coordinator.resumed))
		}
	}
}

func TestNomadRecoverInstancesNeverAttachesTerminalRows(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) { s.Phase = nomadPhaseReady })
	f.runtime.setCleanupCoordinator(&resumeRecordingCoordinator{})
	record := nomadFixtureRecord(f)
	record.Status = models.ExecutorRunningStatusStopped
	instances, err := f.runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{record})
	if err != nil || len(instances) != 0 {
		t.Fatalf("RecoverInstances = %v, %v; a terminal row must not be attached", instances, err)
	}
}
