package journal

import (
	"errors"
	"fmt"
	"math"
	"syscall"
	"time"
)

// StorageReason is the closed classification for a rejected storage operation.
// It never carries provider text or output content.
type StorageReason string

const (
	// StorageReasonDiskSpaceLow is the physical byte-capacity admission guard.
	StorageReasonDiskSpaceLow StorageReason = "disk_space_low"
	// StorageReasonFilesystemFull is an actual ENOSPC allocation failure.
	StorageReasonFilesystemFull StorageReason = "filesystem_full"
	// StorageReasonDiskQuotaExceeded is an actual EDQUOT quota failure.
	StorageReasonDiskQuotaExceeded StorageReason = "disk_quota_exceeded"
	// StorageReasonJournalLimitReached is an explicit logical journal limit.
	StorageReasonJournalLimitReached StorageReason = "journal_limit_reached"
	// StorageReasonStreamLimitReached is an explicit logical stream limit.
	StorageReasonStreamLimitReached StorageReason = "stream_limit_reached"
	// StorageReasonEventTooLarge is a serialized event or producer item bound.
	StorageReasonEventTooLarge StorageReason = "event_too_large"
	// StorageReasonWriteFailed is another source-owned storage write error.
	StorageReasonWriteFailed StorageReason = "storage_write_failed"
)

// Valid reports whether a reason belongs to the closed classification.
func (r StorageReason) Valid() bool {
	switch r {
	case StorageReasonDiskSpaceLow,
		StorageReasonFilesystemFull,
		StorageReasonDiskQuotaExceeded,
		StorageReasonJournalLimitReached,
		StorageReasonStreamLimitReached,
		StorageReasonEventTooLarge,
		StorageReasonWriteFailed:
		return true
	default:
		return false
	}
}

// StorageError is bounded storage evidence captured at the exact rejected
// operation. Absent measurements stay nil, which means unknown and is never
// the same as zero.
type StorageError struct {
	Reason             StorageReason `json:"reason"`
	ObservedAt         time.Time     `json:"observed_at"`
	DiskAvailableBytes *int64        `json:"disk_available_bytes,omitempty"`
	DiskRequiredBytes  *int64        `json:"disk_required_bytes,omitempty"`
	RetainedBytes      *int64        `json:"retained_bytes,omitempty"`
	LimitBytes         *int64        `json:"limit_bytes,omitempty"`
	EventBytes         *int64        `json:"event_bytes,omitempty"`
	cause              error
}

// NewStorageError builds typed storage evidence while retaining the legacy
// sentinel through errors.Is.
func NewStorageError(reason StorageReason, cause error, measurements ...func(*StorageError)) *StorageError {
	storageErr := &StorageError{
		Reason:     reason,
		ObservedAt: time.Now().UTC(),
		cause:      cause,
	}
	for _, measurement := range measurements {
		if measurement != nil {
			measurement(storageErr)
		}
	}
	return storageErr
}

// Error keeps the historical sentinel text for older log consumers.
func (e *StorageError) Error() string {
	if e == nil {
		return "storage error"
	}
	if e.cause != nil {
		return fmt.Sprintf("%s: %s", e.cause.Error(), e.Reason)
	}
	return string(e.Reason)
}

// Unwrap retains errors.Is compatibility with the sentinels callers already
// classify.
func (e *StorageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// StorageDiagnosticFromError finds typed storage evidence without inspecting
// provider prose.
func StorageDiagnosticFromError(err error) (*StorageError, bool) {
	var storageErr *StorageError
	if err == nil || !errors.As(err, &storageErr) || storageErr == nil || !storageErr.Reason.Valid() {
		return nil, false
	}
	copy := *storageErr
	copy.cause = nil
	return &copy, true
}

// WithDiskMeasurements records the physical bytes the rejected operation
// needed and the capacity available at that boundary.
func WithDiskMeasurements(available, required int64) func(*StorageError) {
	return func(e *StorageError) {
		if available >= 0 {
			value := available
			e.DiskAvailableBytes = &value
		}
		if required >= 0 && required < math.MaxInt64 {
			value := required
			e.DiskRequiredBytes = &value
		}
	}
}

// WithLogicalMeasurements records retained and configured-limit bytes.
func WithLogicalMeasurements(retained, limit int64) func(*StorageError) {
	return func(e *StorageError) {
		if retained >= 0 {
			value := retained
			e.RetainedBytes = &value
		}
		if limit > 0 {
			value := limit
			e.LimitBytes = &value
		}
	}
}

// WithEventBytes records the rejected serialized size.
func WithEventBytes(size int64) func(*StorageError) {
	return func(e *StorageError) {
		if size >= 0 {
			value := size
			e.EventBytes = &value
		}
	}
}

func withLimitBytes(limit int64) func(*StorageError) {
	return func(e *StorageError) {
		if limit > 0 {
			value := limit
			e.LimitBytes = &value
		}
	}
}

// classifyStorageCommitError converts an OS allocation failure into typed
// evidence. Other errors keep their identity and wrapping.
func classifyStorageCommitError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := StorageDiagnosticFromError(err); ok {
		return err
	}
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return NewStorageError(StorageReasonFilesystemFull, err, nil)
	case errors.Is(err, syscall.EDQUOT):
		return NewStorageError(StorageReasonDiskQuotaExceeded, err, nil)
	default:
		return err
	}
}
