package storage

import (
	"context"
	"time"
)

type AnalysisStateName string

const (
	AnalysisStateScanning AnalysisStateName = "scanning"
	AnalysisStateReady    AnalysisStateName = "ready"
	AnalysisStateFailed   AnalysisStateName = "failed"
)

type SourceStateName string

const (
	SourceStatePending  SourceStateName = "pending"
	SourceStateScanning SourceStateName = "scanning"
	SourceStateReady    SourceStateName = "ready"
	SourceStateFailed   SourceStateName = "failed"
)

const (
	StorageSourceWorkspaces         = "workspaces"
	StorageSourceGoCache            = "go_cache"
	StorageSourceQuarantine         = "quarantine"
	StorageSourceTemporaryArtifacts = "temporary_artifacts"
	StorageSourceDocker             = "docker"
)

var storageAnalysisSources = [...]string{
	StorageSourceWorkspaces,
	StorageSourceGoCache,
	StorageSourceQuarantine,
	StorageSourceTemporaryArtifacts,
	StorageSourceDocker,
}

type StorageSourceProgress struct {
	State          SourceStateName `json:"state"`
	CompletedItems int             `json:"completed_items"`
	TotalItems     *int            `json:"total_items,omitempty"`
	BytesScanned   int64           `json:"bytes_scanned"`
	Error          *string         `json:"error,omitempty"`
}

type StorageAnalysisProgress struct {
	CompletedSources int                              `json:"completed_sources"`
	TotalSources     int                              `json:"total_sources"`
	Sources          map[string]StorageSourceProgress `json:"sources"`
}

type StorageAnalysisState struct {
	Generation      uint64                  `json:"generation"`
	State           AnalysisStateName       `json:"state"`
	StartedAt       *time.Time              `json:"started_at"`
	CompletedAt     *time.Time              `json:"completed_at"`
	DurationMS      *int64                  `json:"duration_ms"`
	CacheTTLSeconds int64                   `json:"cache_ttl_seconds"`
	RefreshDueAt    *time.Time              `json:"refresh_due_at"`
	Stale           bool                    `json:"stale"`
	Error           *string                 `json:"error"`
	Progress        StorageAnalysisProgress `json:"progress"`
	PartialSummary  *Summary                `json:"partial_summary"`
}

type OverviewRead struct {
	Snapshot *OverviewSnapshot
	Analysis StorageAnalysisState
}

type StorageAnalysisUpdated struct {
	Generation uint64            `json:"generation"`
	State      AnalysisStateName `json:"state"`
}

type StorageAnalysisCompletion struct {
	Generation       uint64
	Duration         time.Duration
	SourceDurations  map[string]time.Duration
	SourceStates     map[string]SourceStateName
	PartitionCount   int
	MaxActiveWalkers int
	Succeeded        bool
}

type OverviewCacheOptions struct {
	Publisher func(StorageAnalysisUpdated)
	Observer  func(StorageAnalysisCompletion)
}

type OverviewProgress struct {
	Source           string
	State            SourceStateName
	CompletedItems   int
	TotalItems       *int
	BytesScanned     int64
	Value            any
	Err              error
	Duration         time.Duration
	PartitionCount   int
	MaxActiveWalkers int
}

type OverviewProgressCallback func(OverviewProgress)

type ProgressiveOverviewProvider interface {
	SummaryWithProgress(context.Context, OverviewProgressCallback) (Summary, error)
}
