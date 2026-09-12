package tempartifacts

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestProviderCountsOnlyRegisteredStaleArtifactsAndCleansExplicitly(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now:      func() time.Time { return now },
		NewID:    func() string { return "artifact-1" },
		NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle.zip"), []byte("bundle"), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt = old
	artifact.ClosedAt = &old
	artifacts.artifacts[lease.Path()] = artifact

	unregistered := filepath.Join(root, "kandev-improve-unregistered")
	if err := os.Mkdir(unregistered, 0o700); err != nil {
		t.Fatalf("mkdir unregistered: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unregistered, "ignored"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("write unregistered: %v", err)
	}

	quarantine := newFakeQuarantineStore()
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Retention: 7 * 24 * time.Hour, Now: func() time.Time { return now },
		NewID: func() string { return "quarantine-1" },
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.TotalCount != 1 || analysis.StaleCount != 1 || analysis.StaleBytes <= int64(len("bundle")) {
		t.Fatalf("analysis = %#v, want only one stale registered artifact", analysis)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("scheduled Cleanup: %v", err)
	}
	if result["skipped"] != true || result["quarantined"] != float64(0) {
		t.Fatalf("scheduled cleanup = %#v, want manual-only skip", result)
	}
	if _, err := os.Stat(unregistered); err != nil {
		t.Fatalf("unregistered directory changed: %v", err)
	}
	if _, err := os.Stat(lease.Path()); err != nil {
		t.Fatalf("registered artifact changed by scheduled cleanup: %v", err)
	}

	explicit, err := provider.cleanupExplicit(context.Background())
	if err != nil {
		t.Fatalf("explicit Cleanup: %v", err)
	}
	if explicit.Quarantined != 1 || explicit.ReclaimedBytes != analysis.StaleBytes {
		t.Fatalf("explicit cleanup = %#v", explicit)
	}
	if _, err := os.Stat(lease.Path()); !os.IsNotExist(err) {
		t.Fatalf("original artifact still exists: %v", err)
	}
	if len(quarantine.entries) != 1 || quarantine.entries["quarantine-1"].ResourceType != storage.ResourceTypeTemporaryArtifact {
		t.Fatalf("quarantine entries = %#v", quarantine.entries)
	}
}

func TestProviderProtectsRecentlyClosedArtifact(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234, RunID: "test-run",
		Now:   func() time.Time { return now },
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle.zip"), []byte("bundle"), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: newFakeQuarantineStore(), HomeDir: home,
		Now: func() time.Time { return now },
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.ProtectedCount != 1 || analysis.ProtectedBytes == 0 || analysis.StaleCount != 0 {
		t.Fatalf("analysis = %#v, want recently closed artifact protected", analysis)
	}
}

func TestProviderScheduledCleanupFollowsSavedTemporaryArtifactPolicy(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now: func() time.Time { return now }, NewID: func() string { return "artifact-1" },
		NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle.zip"), []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt, artifact.ClosedAt = old, &old
	artifacts.artifacts[lease.Path()] = artifact

	quarantine := newFakeQuarantineStore()
	settings := storage.DefaultSettings()
	settings.TemporaryArtifacts.Enabled = true
	settings.QuarantineRetentionHours = 48
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Settings: testSettingsReader{settings: settings},
		Now:      func() time.Time { return now }, NewID: func() string { return "quarantine-1" },
	})

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("scheduled Cleanup: %v", err)
	}
	if result["skipped"] != false || result["quarantined"] != float64(1) {
		t.Fatalf("scheduled cleanup = %#v, want one quarantined artifact", result)
	}
	quarantinedBytes, ok := result["quarantined_bytes"].(float64)
	if !ok || quarantinedBytes <= 0 || quarantinedBytes != result["reclaimed_bytes"] {
		t.Fatalf("scheduled cleanup bytes = %#v, want moved bytes in both result fields", result)
	}
	entry := quarantine.entries["quarantine-1"]
	if !entry.DeleteAfter.Equal(now.Add(48 * time.Hour)) {
		t.Fatalf("delete deadline = %s, want saved 48-hour retention", entry.DeleteAfter)
	}
}

func TestProviderScheduledCleanupReconcilesOwnerDeathWithoutRestart(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	owner := exec.Command("sh", "-c", "sleep 30")
	if err := owner.Start(); err != nil {
		t.Fatalf("start owner process: %v", err)
	}
	t.Cleanup(func() {
		if owner.ProcessState == nil {
			_ = owner.Process.Kill()
			_ = owner.Wait()
		}
	})

	oldRegistry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: int64(owner.Process.Pid), RunID: "owner-run",
		Now: func() time.Time { return now }, NewID: func() string { return "artifact-1" },
		NewToken: func() string { return "token-1" },
	})
	lease, err := oldRegistry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle"), []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt, artifact.LastHeartbeatAt = old, &old
	artifacts.artifacts[lease.Path()] = artifact

	settings := storage.DefaultSettings()
	settings.TemporaryArtifacts.Enabled = true
	quarantine := newFakeQuarantineStore()
	currentRegistry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: int64(os.Getpid()), RunID: "current-run",
		Now: func() time.Time { return now }, NewID: func() string { return "current-artifact" },
		NewToken: func() string { return "current-token" },
	})
	provider := NewProvider(ProviderConfig{
		Registry: currentRegistry, Store: quarantine, HomeDir: home,
		Settings: testSettingsReader{settings: settings}, Now: func() time.Time { return now },
		NewID: func() string { return "quarantine-1" },
	})

	first, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("cleanup while owner is alive: %v", err)
	}
	if first["quarantined"] != float64(0) {
		t.Fatalf("cleanup while owner is alive = %#v, want protected active artifact", first)
	}

	if err := owner.Process.Kill(); err != nil {
		t.Fatalf("kill owner process: %v", err)
	}
	_ = owner.Wait()

	second, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("cleanup after owner death: %v", err)
	}
	if second["quarantined"] != float64(1) {
		t.Fatalf("cleanup after owner death = %#v, want one quarantined artifact", second)
	}
	third, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("repeated cleanup after owner death: %v", err)
	}
	if third["quarantined"] != float64(0) {
		t.Fatalf("repeated cleanup = %#v, want no duplicate quarantine", third)
	}
	if artifacts.artifacts[lease.Path()].State != storage.TemporaryArtifactStateQuarantined {
		t.Fatalf("artifact state = %q, want quarantined", artifacts.artifacts[lease.Path()].State)
	}
}

func TestProviderExplicitCleanupBypassesDisabledTemporaryArtifactPolicy(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now: func() time.Time { return now }, NewID: func() string { return "artifact-1" },
		NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindHostUtility, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "cache"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt, artifact.ClosedAt = old, &old
	artifacts.artifacts[lease.Path()] = artifact

	settings := storage.DefaultSettings()
	settings.TemporaryArtifacts.Enabled = false
	quarantine := newFakeQuarantineStore()
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Settings: testSettingsReader{settings: settings},
		Now:      func() time.Time { return now }, NewID: func() string { return "quarantine-1" },
	})

	scheduled, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("scheduled Cleanup: %v", err)
	}
	if scheduled["skipped"] != true || scheduled["reason"] != "disabled" {
		t.Fatalf("scheduled cleanup = %#v, want disabled skip", scheduled)
	}

	explicit, err := provider.CleanupExplicit(context.Background())
	if err != nil {
		t.Fatalf("explicit Cleanup: %v", err)
	}
	if explicit["quarantined"] != float64(1) {
		t.Fatalf("explicit cleanup = %#v, want one quarantined artifact", explicit)
	}
	if _, err := os.Stat(lease.Path()); !os.IsNotExist(err) {
		t.Fatalf("original artifact still exists: %v", err)
	}
}

func TestProviderRevalidatesLifecycleBeforeQuarantineRename(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now: func() time.Time { return now }, NewID: func() string { return "artifact-1" },
		NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle"), []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt, artifact.ClosedAt = old, &old
	artifacts.artifacts[lease.Path()] = artifact

	quarantine := newFakeQuarantineStore()
	quarantine.onCreate = func() {
		current := artifacts.artifacts[lease.Path()]
		recent := now.Add(-time.Hour)
		current.ClosedAt = &recent
		artifacts.artifacts[lease.Path()] = current
	}
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Now: func() time.Time { return now }, NewID: func() string { return "quarantine-1" },
	})

	result, err := provider.cleanupExplicit(context.Background())
	if err == nil {
		t.Fatal("cleanup succeeded after lifecycle timestamp changed")
	}
	if result.Failed != 1 || len(result.Warnings) != 1 {
		t.Fatalf("cleanup result = %#v, want one revalidation failure", result)
	}
	if _, err := os.Stat(lease.Path()); err != nil {
		t.Fatalf("original artifact was changed: %v", err)
	}
	if quarantine.entries["quarantine-1"].State != storage.QuarantineStateFailed {
		t.Fatalf("quarantine entry = %#v, want failed intent", quarantine.entries["quarantine-1"])
	}
}

func TestProviderReconcilesRenameBeforeLifecycleUpdate(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now:   func() time.Time { return now },
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	quarantineID := "quarantine-1"
	quarantinePath := filepath.Join(home, "trash", "temporary-artifacts", quarantineID)
	if err := os.MkdirAll(filepath.Dir(quarantinePath), 0o700); err != nil {
		t.Fatalf("mkdir quarantine: %v", err)
	}
	if err := os.Rename(artifact.Path, quarantinePath); err != nil {
		t.Fatalf("rename artifact: %v", err)
	}
	quarantine := newFakeQuarantineStore()
	if err := quarantine.CreateQuarantineEntry(context.Background(), &storage.QuarantineEntry{
		ID: quarantineID, ResourceType: storage.ResourceTypeTemporaryArtifact,
		OriginalPath: artifact.Path, QuarantinePath: quarantinePath,
		State: storage.QuarantineStateQuarantined, QuarantinedAt: now,
		DeleteAfter: now.Add(7 * 24 * time.Hour), Metadata: json.RawMessage(`{"artifact_id":"artifact-1"}`),
	}); err != nil {
		t.Fatalf("CreateQuarantineEntry: %v", err)
	}

	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Now: func() time.Time { return now },
	})
	if err := provider.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	reconciled, err := artifacts.GetTemporaryArtifact(context.Background(), artifact.ID)
	if err != nil {
		t.Fatalf("GetTemporaryArtifact: %v", err)
	}
	if reconciled.State != storage.TemporaryArtifactStateQuarantined {
		t.Fatalf("artifact state = %q, want quarantined", reconciled.State)
	}
	reconciledEntry, err := quarantine.GetQuarantineEntry(context.Background(), quarantineID)
	if err != nil {
		t.Fatalf("GetQuarantineEntry: %v", err)
	}
	if reconciledEntry.State != storage.QuarantineStateQuarantined {
		t.Fatalf("quarantine state = %q, want quarantined", reconciledEntry.State)
	}
}

func TestProviderReleasesInterruptedRenameIntentWhenOriginalRemains(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now:   func() time.Time { return now },
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	quarantineID := "quarantine-1"
	quarantinePath := filepath.Join(home, "trash", "temporary-artifacts", quarantineID)
	quarantine := newFakeQuarantineStore()
	if err := quarantine.CreateQuarantineEntry(context.Background(), &storage.QuarantineEntry{
		ID: quarantineID, ResourceType: storage.ResourceTypeTemporaryArtifact,
		OriginalPath: artifact.Path, QuarantinePath: quarantinePath,
		State: storage.QuarantineStateQuarantined, QuarantinedAt: now,
		DeleteAfter: now.Add(7 * 24 * time.Hour), Metadata: json.RawMessage(`{"artifact_id":"artifact-1"}`),
	}); err != nil {
		t.Fatalf("CreateQuarantineEntry: %v", err)
	}

	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Now: func() time.Time { return now },
	})
	if err := provider.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	reconciledEntry, err := quarantine.GetQuarantineEntry(context.Background(), quarantineID)
	if err != nil {
		t.Fatalf("GetQuarantineEntry: %v", err)
	}
	if reconciledEntry.State != storage.QuarantineStateRestored {
		t.Fatalf("quarantine state = %q, want restored", reconciledEntry.State)
	}
	reconciled, err := artifacts.GetTemporaryArtifact(context.Background(), artifact.ID)
	if err != nil {
		t.Fatalf("GetTemporaryArtifact: %v", err)
	}
	if reconciled.State != storage.TemporaryArtifactStateClosed {
		t.Fatalf("artifact state = %q, want closed", reconciled.State)
	}
}

func TestProviderRestoresQuarantinedLifecycleWhenOriginalRemains(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now:   func() time.Time { return now },
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	artifact.State = storage.TemporaryArtifactStateQuarantined
	artifact.QuarantinedAt = &now
	artifacts.artifacts[lease.Path()] = artifact

	quarantineID := "quarantine-1"
	quarantinePath := filepath.Join(home, "trash", "temporary-artifacts", quarantineID)
	quarantine := newFakeQuarantineStore()
	if err := quarantine.CreateQuarantineEntry(context.Background(), &storage.QuarantineEntry{
		ID: quarantineID, ResourceType: storage.ResourceTypeTemporaryArtifact,
		OriginalPath: artifact.Path, QuarantinePath: quarantinePath,
		State: storage.QuarantineStateQuarantined, QuarantinedAt: now,
		DeleteAfter: now.Add(7 * 24 * time.Hour), Metadata: json.RawMessage(`{"artifact_id":"artifact-1"}`),
	}); err != nil {
		t.Fatalf("CreateQuarantineEntry: %v", err)
	}

	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Now: func() time.Time { return now },
	})
	if err := provider.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	reconciled, err := artifacts.GetTemporaryArtifact(context.Background(), artifact.ID)
	if err != nil {
		t.Fatalf("GetTemporaryArtifact: %v", err)
	}
	if reconciled.State != storage.TemporaryArtifactStateClosed {
		t.Fatalf("artifact state = %q, want closed", reconciled.State)
	}
}

func TestProviderCanRequarantineRestoredArtifact(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	clock := now
	artifacts := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		Now:   func() time.Time { return clock },
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt = old
	artifact.ClosedAt = &old
	artifacts.artifacts[lease.Path()] = artifact
	quarantine := newFakeQuarantineStore()
	ids := []string{"quarantine-1", "quarantine-2"}
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home, Now: func() time.Time { return clock },
		NewID: func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
	if _, err := provider.cleanupExplicit(context.Background()); err != nil {
		t.Fatalf("first cleanup: %v", err)
	}
	if _, err := provider.Restore(context.Background(), "quarantine-1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	clock = now.Add(25 * time.Hour)
	if _, err := provider.cleanupExplicit(context.Background()); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
	if len(quarantine.entries) != 2 || quarantine.entries["quarantine-2"].State != storage.QuarantineStateQuarantined {
		t.Fatalf("quarantine entries after restore/retry = %#v", quarantine.entries)
	}
}

type fakeQuarantineStore struct {
	entries  map[string]storage.QuarantineEntry
	onCreate func()
}

func newFakeQuarantineStore() *fakeQuarantineStore {
	return &fakeQuarantineStore{entries: make(map[string]storage.QuarantineEntry)}
}

func (s *fakeQuarantineStore) CreateQuarantineEntry(_ context.Context, entry *storage.QuarantineEntry) error {
	s.entries[entry.ID] = *entry
	if s.onCreate != nil {
		s.onCreate()
	}
	return nil
}

func (s *fakeQuarantineStore) GetQuarantineEntry(_ context.Context, id string) (storage.QuarantineEntry, error) {
	entry, ok := s.entries[id]
	if !ok {
		return storage.QuarantineEntry{}, storage.ErrNotFound
	}
	return entry, nil
}

func (s *fakeQuarantineStore) ListQuarantineEntries(_ context.Context, includeTerminal bool) ([]storage.QuarantineEntry, error) {
	entries := make([]storage.QuarantineEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if !includeTerminal && entry.State != storage.QuarantineStateQuarantined && entry.State != storage.QuarantineStateFailed {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *fakeQuarantineStore) TransitionQuarantineEntry(
	_ context.Context, id string, next storage.QuarantineState, lastError string,
) (storage.QuarantineEntry, error) {
	entry, ok := s.entries[id]
	if !ok {
		return storage.QuarantineEntry{}, storage.ErrNotFound
	}
	if !validFakeQuarantineTransition(entry.State, next) {
		return storage.QuarantineEntry{}, storage.ErrInvalidTransition
	}
	entry.State = next
	entry.LastError = lastError
	if next != storage.QuarantineStateFailed {
		entry.LastError = ""
	}
	completion := time.Now().UTC()
	switch next {
	case storage.QuarantineStateRestored:
		entry.RestoredAt = &completion
	case storage.QuarantineStateDeleted:
		entry.DeletedAt = &completion
	}
	s.entries[id] = entry
	return entry, nil
}

func validFakeQuarantineTransition(current, next storage.QuarantineState) bool {
	switch current {
	case storage.QuarantineStateQuarantined:
		return next == storage.QuarantineStateRestored || next == storage.QuarantineStateDeleted ||
			next == storage.QuarantineStateFailed
	case storage.QuarantineStateFailed:
		return next == storage.QuarantineStateQuarantined || next == storage.QuarantineStateRestored ||
			next == storage.QuarantineStateDeleted
	default:
		return false
	}
}

type testSettingsReader struct {
	settings storage.StorageMaintenanceSettings
}

func (s testSettingsReader) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	return s.settings, nil
}
