package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func stateVersionFixture(executionID string, version uint32, phase string) *models.ExecutorRunning {
	running := pluginExecutorRunningFixture(executionID, 7, phase)
	envelope := running.Metadata["plugin_executor"].(map[string]interface{})
	envelope["installation_id"] = "installation-readability"
	envelope["provider_key"] = "remote"
	envelope["state_version"] = version
	return running
}

func TestPluginStateWritableCheckerRefusesAnUnreadableVersion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedPluginExecutorInventory(t, repo, 7)
	exerciseWritableChecker(t, repo)
}

func exerciseWritableChecker(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
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
	if err := repo.CheckpointPluginExecutorInventory(ctx, stateVersionFixture("execution-state", 1, "allocating")); err != nil {
		t.Fatalf("readable version: %v", err)
	}
	refused := stateVersionFixture("execution-state", 2, "ready")
	refused.ExpectedPluginExecutorRevision = 1
	if err := repo.CheckpointPluginExecutorInventory(ctx, refused); !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
		t.Fatalf("unreadable version: err = %v, want capability_mismatch", err)
	}
	stored, err := repo.GetExecutorRunningBySessionID(ctx, "session-plugin-executor-inventory")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := stored.Metadata["plugin_executor"].(map[string]interface{})["phase"]; got != "allocating" {
		t.Fatalf("a refused write changed the stored envelope: phase = %v", got)
	}
	cleanup := stateVersionFixture("execution-state", 2, "absent")
	cleanup.ExpectedPluginExecutorRevision = 1
	if err := repo.CheckpointPluginExecutorInventory(ctx, cleanup); err != nil {
		t.Fatalf("cleanup of an unreadable record must still be recordable: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("checker saw versions %v; the terminal cleanup write is not gated", seen)
	}
}

func TestPluginStateReadabilityLockExcludesAnAdmissionWrite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedPluginExecutorInventory(t, repo, 7)
	exerciseReadabilityLockExclusion(t, repo, repo)
}

func exerciseReadabilityLockExclusion(t *testing.T, holder, writer *Repository) {
	t.Helper()
	ctx := context.Background()
	held, release := make(chan struct{}), make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- holder.WithPluginStateReadabilityLock(ctx, "installation-readability", func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- writer.CheckpointPluginExecutorInventory(ctx, stateVersionFixture("execution-lock", 1, "allocating"))
	}()
	select {
	case err := <-writeDone:
		t.Fatalf("admission write completed while the activation held the installation lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-lockDone; err != nil {
		t.Fatalf("lock holder: %v", err)
	}
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("admission write after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("admission write did not proceed after the lock was released")
	}
}

func TestPluginStateReadabilityLockIsPerInstallation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.WithPluginStateReadabilityLock(ctx, "", func(context.Context) error { return nil }); err == nil {
		t.Fatal("an empty installation id must fail closed")
	}
	wantErr := errors.New("callback failed")
	if err := repo.WithPluginStateReadabilityLock(ctx, "installation-a", func(context.Context) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("callback error = %v", err)
	}
}
