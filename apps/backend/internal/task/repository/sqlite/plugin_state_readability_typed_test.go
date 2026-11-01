package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// typedStateVersionFixture carries the envelope the way the lifecycle manager
// does: as a typed value rather than a decoded map.
func typedStateVersionFixture(t *testing.T, taskID, executionID string, version uint32, phase string) *models.ExecutorRunning {
	t.Helper()
	running := stateVersionFixture(taskID, executionID, version, phase)
	encoded, err := json.Marshal(running.Metadata["plugin_executor"])
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	running.Metadata["plugin_executor"] = rawEnvelope(encoded)
	return running
}

// rawEnvelope marshals to the stored JSON verbatim and is not a map, which is
// the shape the production checkpoint path hands to the repository.
type rawEnvelope []byte

func (r rawEnvelope) MarshalJSON() ([]byte, error) { return r, nil }

func TestPluginStateWritableCheckerGatesATypedEnvelope(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedPluginExecutorInventory(t, repo, 7)
	ctx := context.Background()
	taskID := "task-plugin-executor-inventory"
	var seen []uint32
	repo.SetPluginStateWritableChecker(func(_ context.Context, installationID, providerKey string, version uint32) error {
		seen = append(seen, version)
		if installationID != "installation-readability" || providerKey != "remote" {
			t.Errorf("checker identity = %q/%q", installationID, providerKey)
		}
		if version == 2 {
			return models.ErrExecutorCapabilityMismatch
		}
		return nil
	})
	if err := repo.CheckpointPluginExecutorInventory(ctx, typedStateVersionFixture(t, taskID, "execution-state", 1, "allocating")); err != nil {
		t.Fatalf("readable version: %v", err)
	}
	refused := typedStateVersionFixture(t, taskID, "execution-state", 2, "ready")
	refused.ExpectedPluginExecutorRevision = 1
	if err := repo.CheckpointPluginExecutorInventory(ctx, refused); !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
		t.Fatalf("unreadable version in a typed envelope: err = %v (checker saw %v), want capability_mismatch", err, seen)
	}
}
