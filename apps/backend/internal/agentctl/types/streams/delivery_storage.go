package streams

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

// DeliveryStorageDiagnosticVersion is the only accepted source wire version.
const DeliveryStorageDiagnosticVersion = 1

// maxDeliveryStorageDiagnosticJSONBytes bounds one serialized diagnostic.
const maxDeliveryStorageDiagnosticJSONBytes = 2 << 10

// Closed storage reasons shared with the journal classification.
const (
	DeliveryStorageReasonDiskSpaceLow        = string(journal.StorageReasonDiskSpaceLow)
	DeliveryStorageReasonFilesystemFull      = string(journal.StorageReasonFilesystemFull)
	DeliveryStorageReasonDiskQuotaExceeded   = string(journal.StorageReasonDiskQuotaExceeded)
	DeliveryStorageReasonJournalLimitReached = string(journal.StorageReasonJournalLimitReached)
	DeliveryStorageReasonStreamLimitReached  = string(journal.StorageReasonStreamLimitReached)
	DeliveryStorageReasonEventTooLarge       = string(journal.StorageReasonEventTooLarge)
	DeliveryStorageReasonWriteFailed         = string(journal.StorageReasonWriteFailed)
)

// DeliveryStorageDiagnostic is the bounded wire evidence attached to a
// host-generated terminal error. Absent measurements are unknown, never zero.
// It carries no raw error, path, prompt, or output content.
type DeliveryStorageDiagnostic struct {
	Version            int       `json:"version"`
	Reason             string    `json:"reason"`
	ObservedAt         time.Time `json:"observed_at"`
	DiskAvailableBytes *int64    `json:"disk_available_bytes,omitempty"`
	DiskRequiredBytes  *int64    `json:"disk_required_bytes,omitempty"`
	RetainedBytes      *int64    `json:"retained_bytes,omitempty"`
	LimitBytes         *int64    `json:"limit_bytes,omitempty"`
	EventBytes         *int64    `json:"event_bytes,omitempty"`
}

// KnownDeliveryStorageReason reports whether the closed reason is recognized.
func KnownDeliveryStorageReason(reason string) bool {
	return journal.StorageReason(reason).Valid()
}

// NewDeliveryStorageDiagnostic converts source-owned journal evidence into the
// bounded wire type. It never classifies provider prose.
func NewDeliveryStorageDiagnostic(storageErr *journal.StorageError) *DeliveryStorageDiagnostic {
	if storageErr == nil || !storageErr.Reason.Valid() {
		return nil
	}
	diagnostic := &DeliveryStorageDiagnostic{
		Version:            DeliveryStorageDiagnosticVersion,
		Reason:             string(storageErr.Reason),
		ObservedAt:         storageErr.ObservedAt.UTC(),
		DiskAvailableBytes: copyNonNegative(storageErr.DiskAvailableBytes),
		DiskRequiredBytes:  copyNonNegative(storageErr.DiskRequiredBytes),
		RetainedBytes:      copyNonNegative(storageErr.RetainedBytes),
		LimitBytes:         copyPositive(storageErr.LimitBytes),
		EventBytes:         copyNonNegative(storageErr.EventBytes),
	}
	if !diagnostic.Valid() {
		return nil
	}
	return diagnostic
}

// DeliveryStorageDiagnosticFromError converts typed storage errors only.
func DeliveryStorageDiagnosticFromError(err error) *DeliveryStorageDiagnostic {
	storageErr, ok := journal.StorageDiagnosticFromError(err)
	if !ok {
		return nil
	}
	return NewDeliveryStorageDiagnostic(storageErr)
}

// Normalize validates and bounds an incoming diagnostic. It returns nil for an
// unsupported version, an unknown reason, or malformed measurements so older
// and foreign payloads never establish a storage claim.
func (d DeliveryStorageDiagnostic) Normalize() *DeliveryStorageDiagnostic {
	if d.Version != DeliveryStorageDiagnosticVersion || !KnownDeliveryStorageReason(d.Reason) {
		return nil
	}
	normalized := DeliveryStorageDiagnostic{
		Version:            DeliveryStorageDiagnosticVersion,
		Reason:             d.Reason,
		ObservedAt:         d.ObservedAt.UTC(),
		DiskAvailableBytes: copyNonNegative(d.DiskAvailableBytes),
		DiskRequiredBytes:  copyNonNegative(d.DiskRequiredBytes),
		RetainedBytes:      copyNonNegative(d.RetainedBytes),
		LimitBytes:         copyPositive(d.LimitBytes),
		EventBytes:         copyNonNegative(d.EventBytes),
	}
	if normalized.ObservedAt.IsZero() {
		normalized.ObservedAt = time.Now().UTC()
	}
	if !normalized.Valid() {
		return nil
	}
	return &normalized
}

// Valid reports whether the diagnostic is well formed and within its bound.
func (d *DeliveryStorageDiagnostic) Valid() bool {
	if d == nil || d.Version != DeliveryStorageDiagnosticVersion || !KnownDeliveryStorageReason(d.Reason) {
		return false
	}
	encoded, err := json.Marshal(d)
	if err != nil || len(encoded) > maxDeliveryStorageDiagnosticJSONBytes {
		return false
	}
	return true
}

func copyNonNegative(value *int64) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	copied := *value
	return &copied
}

func copyPositive(value *int64) *int64 {
	if value == nil || *value <= 0 {
		return nil
	}
	copied := *value
	return &copied
}
