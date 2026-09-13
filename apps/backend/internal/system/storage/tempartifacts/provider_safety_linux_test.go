//go:build linux

package tempartifacts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestProviderRejectsArtifactReplacementAfterValidation(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle"), []byte("original"), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt, artifact.ClosedAt = old, &old
	artifacts.artifacts[lease.Path()] = artifact

	quarantine := newFakeQuarantineStore()
	replacementBackup := lease.Path() + ".original"
	quarantine.onCreate = func() {
		if err := os.Rename(lease.Path(), replacementBackup); err != nil {
			t.Fatalf("replace original artifact: %v", err)
		}
		if err := os.Mkdir(lease.Path(), 0o700); err != nil {
			t.Fatalf("create replacement artifact: %v", err)
		}
		if err := writeMarker(filepath.Join(lease.Path(), MarkerName), markerData{
			ID: artifact.ID, Kind: artifact.Kind, Token: artifact.MarkerToken,
		}); err != nil {
			t.Fatalf("write replacement marker: %v", err)
		}
	}
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Now: func() time.Time { return now }, NewID: func() string { return "quarantine-1" },
	})

	result, err := provider.cleanupExplicit(context.Background())
	if err == nil {
		t.Fatal("cleanup succeeded after the artifact directory was replaced")
	}
	if result.Failed != 1 || quarantine.entries["quarantine-1"].State != storage.QuarantineStateFailed {
		t.Fatalf("cleanup result = %#v, quarantine = %#v", result, quarantine.entries["quarantine-1"])
	}
	if _, err := os.Stat(lease.Path()); err != nil {
		t.Fatalf("replacement artifact was removed: %v", err)
	}
	if _, err := os.Stat(replacementBackup); err != nil {
		t.Fatalf("original artifact backup missing: %v", err)
	}
}

func TestProviderQuarantinesAcrossFilesystemsWithValidatedCopy(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(lease.Path(), "nested"), 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "nested", "cache"), []byte("copied"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact := artifacts.artifacts[lease.Path()]
	old := now.Add(-25 * time.Hour)
	artifact.CreatedAt, artifact.ClosedAt = old, &old
	artifacts.artifacts[lease.Path()] = artifact

	quarantine := newFakeQuarantineStore()
	renameCalls := 0
	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: quarantine, HomeDir: home,
		Now: func() time.Time { return now }, NewID: func() string { return "quarantine-1" },
		Rename: func(source, destination string) error {
			renameCalls++
			if renameCalls == 1 {
				return syscall.EXDEV
			}
			return os.Rename(source, destination)
		},
	})

	result, err := provider.cleanupExplicit(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Quarantined != 1 || renameCalls != 3 {
		t.Fatalf("cleanup result = %#v, rename calls = %d", result, renameCalls)
	}
	entry := quarantine.entries["quarantine-1"]
	content, err := os.ReadFile(filepath.Join(entry.QuarantinePath, "nested", "cache"))
	if err != nil {
		t.Fatalf("read quarantined copy: %v", err)
	}
	if string(content) != "copied" {
		t.Fatalf("quarantined content = %q, want copied content", content)
	}
	if _, err := os.Stat(lease.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original artifact still exists: %v", err)
	}
	if artifacts.artifacts[lease.Path()].State != storage.TemporaryArtifactStateQuarantined {
		t.Fatalf("artifact state = %q, want quarantined", artifacts.artifacts[lease.Path()].State)
	}
}

func TestProviderDeletionRejectsReplacedValidatedPath(t *testing.T) {
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
	artifact := lease.Artifact()
	validatedInfo, err := os.Lstat(lease.Path())
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	backup := lease.Path() + ".validated"
	if err := os.Rename(lease.Path(), backup); err != nil {
		t.Fatalf("rename original: %v", err)
	}
	if err := os.Mkdir(lease.Path(), 0o700); err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	if err := writeMarker(filepath.Join(lease.Path(), MarkerName), markerData{
		ID: artifact.ID, Kind: artifact.Kind, Token: artifact.MarkerToken,
	}); err != nil {
		t.Fatalf("write replacement marker: %v", err)
	}

	provider := NewProvider(ProviderConfig{
		Registry: registry, Store: newFakeQuarantineStore(), HomeDir: home,
		Now: func() time.Time { return now },
	})
	if err := provider.removeValidatedArtifact(lease.Path(), validatedInfo, artifact); err == nil {
		t.Fatal("deletion accepted a replacement directory")
	}
	if _, err := os.Stat(lease.Path()); err != nil {
		t.Fatalf("replacement directory was removed: %v", err)
	}
}
