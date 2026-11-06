package process

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
)

func TestDeliveryStorageRuntimeStorageUsesJournalFilesystem(t *testing.T) {
	dir := t.TempDir()
	manager := &Manager{cfg: &config.InstanceConfig{
		WorkDir:            dir,
		DurableJournalPath: filepath.Join(dir, "delivery.bbolt"),
	}}
	snapshot := manager.RuntimeStorage(context.Background())
	if snapshot.StorageKind != streams.RuntimeStorageKindJournal {
		t.Fatalf("storage kind = %q", snapshot.StorageKind)
	}
	if snapshot.Filesystem == nil || snapshot.Filesystem.TotalBytes <= 0 {
		t.Fatalf("filesystem = %+v", snapshot.Filesystem)
	}
}

func TestDeliveryStorageRuntimeStorageFallsBackToWorkspace(t *testing.T) {
	dir := t.TempDir()
	manager := &Manager{cfg: &config.InstanceConfig{WorkDir: dir}}
	snapshot := manager.RuntimeStorage(context.Background())
	if snapshot.StorageKind != streams.RuntimeStorageKindWorkspace {
		t.Fatalf("storage kind = %q", snapshot.StorageKind)
	}
	if snapshot.Filesystem == nil {
		t.Fatal("workspace capacity is missing")
	}
	if snapshot.Filesystem.FilesystemID == "" {
		t.Fatal("source filesystem identity is missing")
	}
}

func TestDeliveryStorageRuntimeStorageStaysUnavailable(t *testing.T) {
	manager := &Manager{cfg: &config.InstanceConfig{WorkDir: filepath.Join(t.TempDir(), "missing")}}
	snapshot := manager.RuntimeStorage(context.Background())
	if !snapshot.Unavailable || snapshot.Filesystem != nil {
		t.Fatalf("unreadable path produced capacity: %+v", snapshot)
	}
}

func TestDeliveryStorageSharedSamplerServesMultipleInstances(t *testing.T) {
	dir := t.TempDir()
	sampler := systemmetrics.NewDiskCapacitySampler()
	sampler.SetTTL(time.Minute)
	first := &Manager{cfg: &config.InstanceConfig{WorkDir: dir}}
	second := &Manager{cfg: &config.InstanceConfig{WorkDir: dir}}
	first.SetDiskCapacitySampler(sampler)
	second.SetDiskCapacitySampler(sampler)

	firstSnapshot := first.RuntimeStorage(context.Background())
	secondSnapshot := second.RuntimeStorage(context.Background())
	if firstSnapshot.Filesystem == nil || secondSnapshot.Filesystem == nil {
		t.Fatalf("snapshots = %+v / %+v", firstSnapshot, secondSnapshot)
	}
	if firstSnapshot.Filesystem.FilesystemID != secondSnapshot.Filesystem.FilesystemID {
		t.Fatalf("filesystem identity differs: %q vs %q",
			firstSnapshot.Filesystem.FilesystemID, secondSnapshot.Filesystem.FilesystemID)
	}
	if !firstSnapshot.Filesystem.ObservedAt.Equal(secondSnapshot.Filesystem.ObservedAt) {
		t.Fatal("shared sampler did not reuse one sample")
	}
}
