package journal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
)

func TestDeliveryStorageDiskGuardReportsMeasurements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{
		Path:                 path,
		DiskCapacityCacheTTL: time.Hour,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 30, AvailableBytes: 1 << 10}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	_, err = j.Append(context.Background(), Event{
		SessionID: "session", IncarnationID: "incarnation", StreamID: "stream",
		Type: "message", Payload: make([]byte, 64<<10),
	})
	if !errors.Is(err, ErrJournalFull) {
		t.Fatalf("append error = %v, want ErrJournalFull compatibility", err)
	}
	storageErr, ok := StorageDiagnosticFromError(err)
	if !ok {
		t.Fatalf("append error %v has no storage diagnostic", err)
	}
	if storageErr.Reason != StorageReasonDiskSpaceLow {
		t.Fatalf("reason = %q, want %q", storageErr.Reason, StorageReasonDiskSpaceLow)
	}
	if storageErr.DiskAvailableBytes == nil || *storageErr.DiskAvailableBytes <= 0 {
		t.Fatalf("available measurement = %v", storageErr.DiskAvailableBytes)
	}
	if storageErr.DiskRequiredBytes == nil || *storageErr.DiskRequiredBytes <= *storageErr.DiskAvailableBytes {
		t.Fatalf("required measurement = %v", storageErr.DiskRequiredBytes)
	}
}

func TestDeliveryStorageLogicalLimitReportsRetainedAndLimit(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt"), MaxJournalBytes: 300, ReserveBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	var storageErr *StorageError
	for sequence := uint64(1); sequence < 100 && storageErr == nil; sequence++ {
		_, appendErr := j.Append(context.Background(), Event{
			SessionID: "session", IncarnationID: "incarnation", StreamID: "stream",
			Sequence: sequence, Type: "message", Payload: []byte("payload"),
		})
		if appendErr == nil {
			continue
		}
		var found bool
		storageErr, found = StorageDiagnosticFromError(appendErr)
		if !found {
			t.Fatalf("append error %v has no storage diagnostic", appendErr)
		}
	}
	if storageErr == nil {
		t.Fatal("journal never enforced its logical limit")
	}
	if storageErr.Reason != StorageReasonJournalLimitReached {
		t.Fatalf("reason = %q", storageErr.Reason)
	}
	if storageErr.RetainedBytes == nil || storageErr.LimitBytes == nil {
		t.Fatalf("logical measurements = retained %v limit %v", storageErr.RetainedBytes, storageErr.LimitBytes)
	}
}

func TestDeliveryStorageOversizedEventKeepsStreamCompatibility(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt"), MaxEventBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	_, err = j.Append(context.Background(), Event{
		SessionID: "session", IncarnationID: "incarnation", StreamID: "stream",
		Type: "message", Payload: make([]byte, 17),
	})
	if !errors.Is(err, ErrStreamFull) {
		t.Fatalf("error = %v, want ErrStreamFull compatibility", err)
	}
	storageErr, ok := StorageDiagnosticFromError(err)
	if !ok || storageErr.Reason != StorageReasonEventTooLarge {
		t.Fatalf("storage diagnostic = %+v, ok = %v", storageErr, ok)
	}
	if storageErr.EventBytes == nil || *storageErr.EventBytes != 17 {
		t.Fatalf("event bytes = %v", storageErr.EventBytes)
	}
}

func TestDeliveryStorageCommitAllocationErrorIsClassified(t *testing.T) {
	storageErr, ok := StorageDiagnosticFromError(classifyStorageCommitError(fmt.Errorf("commit: %w", syscall.ENOSPC)))
	if !ok || storageErr.Reason != StorageReasonFilesystemFull {
		t.Fatalf("classified error = %+v, ok = %v", storageErr, ok)
	}
	if storageErr.DiskAvailableBytes != nil {
		t.Fatalf("ENOSPC must not invent byte capacity: %v", *storageErr.DiskAvailableBytes)
	}
	quotaErr, ok := StorageDiagnosticFromError(classifyStorageCommitError(fmt.Errorf("commit: %w", syscall.EDQUOT)))
	if !ok || quotaErr.Reason != StorageReasonDiskQuotaExceeded {
		t.Fatalf("quota classification = %+v, ok = %v", quotaErr, ok)
	}
}

func TestDeliveryStorageUnknownMeasurementsStayUnknown(t *testing.T) {
	storageErr := NewStorageError(StorageReasonDiskSpaceLow, ErrJournalFull, WithDiskMeasurements(-1, math.MaxInt64))
	if storageErr.DiskAvailableBytes != nil || storageErr.DiskRequiredBytes != nil {
		t.Fatalf("unknown measurements became known: %+v", storageErr)
	}
}
