package streams

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func int64Pointer(value int64) *int64 { return &value }

func TestDeliveryStorageDiagnosticNormalizesKnownEvidence(t *testing.T) {
	source := &journal.StorageError{
		Reason:             journal.StorageReasonDiskSpaceLow,
		ObservedAt:         time.Now().UTC(),
		DiskAvailableBytes: int64Pointer(1024),
		DiskRequiredBytes:  int64Pointer(2048),
	}
	diagnostic := NewDeliveryStorageDiagnostic(source)
	if diagnostic == nil || !diagnostic.Valid() {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
	if diagnostic.Version != DeliveryStorageDiagnosticVersion || diagnostic.Reason != DeliveryStorageReasonDiskSpaceLow {
		t.Fatalf("diagnostic identity = %+v", diagnostic)
	}
}

func TestDeliveryStorageDiagnosticRejectsUnsupportedEvidence(t *testing.T) {
	cases := map[string]DeliveryStorageDiagnostic{
		"unknown version": {Version: 2, Reason: DeliveryStorageReasonDiskSpaceLow},
		"unknown reason":  {Version: 1, Reason: "provider_said_disk"},
		"empty reason":    {Version: 1},
	}
	for name, diagnostic := range cases {
		if diagnostic.Normalize() != nil {
			t.Fatalf("%s: unsupported diagnostic was accepted", name)
		}
	}
	negative := DeliveryStorageDiagnostic{
		Version:            1,
		Reason:             DeliveryStorageReasonFilesystemFull,
		DiskAvailableBytes: int64Pointer(-5),
		LimitBytes:         int64Pointer(-1),
	}
	normalized := negative.Normalize()
	if normalized == nil {
		t.Fatal("supported reason was rejected")
	}
	if normalized.DiskAvailableBytes != nil || normalized.LimitBytes != nil {
		t.Fatalf("malformed measurements survived normalization: %+v", normalized)
	}
}

func TestDeliveryStorageDiagnosticDoesNotClassifyPlainErrors(t *testing.T) {
	if diagnostic := DeliveryStorageDiagnosticFromError(journal.ErrJournalFull); diagnostic != nil {
		t.Fatalf("plain sentinel produced a storage claim: %+v", diagnostic)
	}
	if diagnostic := DeliveryStorageDiagnosticFromError(nil); diagnostic != nil {
		t.Fatalf("nil error produced a storage claim: %+v", diagnostic)
	}
}

func TestRuntimeStorageSnapshotUsageAndNormalization(t *testing.T) {
	snapshot := RuntimeStorageSnapshot{
		Version:     RuntimeStorageSnapshotVersion,
		StorageKind: RuntimeStorageKindWorkspace,
		ObservedAt:  time.Now().UTC(),
		Filesystem:  &RuntimeStorageFilesystem{TotalBytes: 1000, AvailableBytes: 100, FilesystemID: "device:1"},
	}
	usage, ok := snapshot.UsagePercent()
	if !ok || usage != 90 {
		t.Fatalf("usage = %v, ok = %v", usage, ok)
	}
	normalized := snapshot.Normalize()
	if normalized == nil || normalized.Filesystem == nil {
		t.Fatalf("snapshot was rejected: %+v", normalized)
	}
	if normalized.Filesystem.FilesystemID != "" {
		t.Fatal("filesystem identity leaked into browser normalization")
	}
	malformed := snapshot
	malformed.Filesystem = &RuntimeStorageFilesystem{TotalBytes: 1000, AvailableBytes: 1001}
	if malformed.Normalize().Filesystem != nil {
		t.Fatal("available bytes above total survived normalization")
	}
	unsupported := snapshot
	unsupported.Version = 7
	if unsupported.Normalize() != nil {
		t.Fatal("unsupported snapshot version was accepted")
	}
}
