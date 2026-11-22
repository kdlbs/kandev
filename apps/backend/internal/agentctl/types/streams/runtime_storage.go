package streams

import "time"

// RuntimeStorageSnapshotVersion is the only accepted advisory wire version.
const RuntimeStorageSnapshotVersion = 1

// Runtime storage kinds identify what the executor measured.
const (
	RuntimeStorageKindJournal   = "journal"
	RuntimeStorageKindWorkspace = "workspace"
)

// RuntimeStorageSnapshot is the read-only advisory reported by the agentctl
// that owns the executor filesystem. It is independent of durable delivery.
type RuntimeStorageSnapshot struct {
	Version      int                       `json:"version"`
	StorageKind  string                    `json:"storage_kind"`
	Filesystem   *RuntimeStorageFilesystem `json:"filesystem_capacity,omitempty"`
	ObservedAt   time.Time                 `json:"observed_at"`
	Unavailable  bool                      `json:"unavailable,omitempty"`
	DetailReason string                    `json:"detail_reason,omitempty"`
}

// RuntimeStorageFilesystem is one raw filesystem capacity sample. The opaque
// identity is private to source-side grouping and is never sent to browsers.
type RuntimeStorageFilesystem struct {
	TotalBytes     int64     `json:"total_bytes"`
	AvailableBytes int64     `json:"available_bytes"`
	ObservedAt     time.Time `json:"observed_at"`
	FilesystemID   string    `json:"filesystem_id,omitempty"`
}

// UsagePercent returns the raw usage percentage derived from the same total
// and available values the source reported. False means unknown.
func (s RuntimeStorageSnapshot) UsagePercent() (float64, bool) {
	if s.Filesystem == nil || s.Filesystem.TotalBytes <= 0 {
		return 0, false
	}
	available := s.Filesystem.AvailableBytes
	if available < 0 {
		available = 0
	}
	if available > s.Filesystem.TotalBytes {
		available = s.Filesystem.TotalBytes
	}
	used := s.Filesystem.TotalBytes - available
	return float64(used) / float64(s.Filesystem.TotalBytes) * 100, true
}

// Normalize rejects unsupported versions and malformed capacity so a foreign
// or old peer cannot establish a usage claim.
func (s RuntimeStorageSnapshot) Normalize() *RuntimeStorageSnapshot {
	if s.Version != RuntimeStorageSnapshotVersion {
		return nil
	}
	if s.StorageKind != RuntimeStorageKindJournal && s.StorageKind != RuntimeStorageKindWorkspace {
		return nil
	}
	normalized := s
	if s.Filesystem != nil {
		if s.Filesystem.TotalBytes <= 0 || s.Filesystem.AvailableBytes < 0 ||
			s.Filesystem.AvailableBytes > s.Filesystem.TotalBytes {
			normalized.Filesystem = nil
			normalized.Unavailable = true
		} else {
			filesystem := *s.Filesystem
			filesystem.FilesystemID = ""
			if filesystem.ObservedAt.IsZero() {
				filesystem.ObservedAt = s.ObservedAt
			}
			normalized.Filesystem = &filesystem
		}
	}
	if normalized.ObservedAt.IsZero() && normalized.Filesystem != nil {
		normalized.ObservedAt = normalized.Filesystem.ObservedAt
	}
	return &normalized
}
