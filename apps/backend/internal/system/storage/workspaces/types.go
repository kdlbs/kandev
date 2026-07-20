// Package workspaces safely quarantines orphaned Kandev task workspaces.
package workspaces

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

const (
	LayoutVersionSemantic = 1
	LayoutVersionScratch  = 2
)

var (
	ErrInventoryIncomplete = errors.New("workspace inventory is incomplete")
	ErrRestoreConflict     = errors.New("workspace restore destination already exists")
	ErrDeleteConfirmation  = errors.New("workspace permanent deletion requires DELETE confirmation")
)

type OwnershipMarker struct {
	TaskID        string    `json:"task_id"`
	WorkspaceID   string    `json:"workspace_id"`
	TaskDirName   string    `json:"task_dir_name"`
	LayoutVersion int       `json:"layout_version"`
	CreatedAt     time.Time `json:"created_at"`
}

type ScratchRoot struct {
	TaskID      string
	WorkspaceID string
	Path        string
}

type Inventory struct {
	Complete         bool
	WorktreePaths    []string
	EnvironmentPaths []string
	ExecutionPaths   []string
	ScratchRoots     []ScratchRoot
}

type InventorySource interface {
	LoadWorkspaceInventory(ctx context.Context) (Inventory, error)
}

type QuarantineStore interface {
	CreateQuarantineEntry(ctx context.Context, entry *storage.QuarantineEntry) error
	GetQuarantineEntry(ctx context.Context, id string) (storage.QuarantineEntry, error)
	TransitionQuarantineEntry(ctx context.Context, id string, next storage.QuarantineState, lastError string) (storage.QuarantineEntry, error)
	ListQuarantineEntries(ctx context.Context, includeTerminal bool) ([]storage.QuarantineEntry, error)
}

type Config struct {
	TasksRoot   string
	TrashRoot   string
	Inventory   InventorySource
	Store       QuarantineStore
	GracePeriod time.Duration
	Retention   time.Duration
	Now         func() time.Time
	NewID       func() string
	Pruner      WorktreePruner
}

type WorktreePruner interface {
	PruneQuarantinedWorkspace(ctx context.Context, entry storage.QuarantineEntry) error
}

type CleanupResult struct {
	Candidates     int      `json:"candidates"`
	Quarantined    int      `json:"quarantined"`
	ReclaimedBytes int64    `json:"reclaimed_bytes"`
	Warnings       []string `json:"warnings,omitempty"`
}

type Analysis struct {
	TotalBytes     int64    `json:"total_bytes"`
	ActiveBytes    int64    `json:"active_bytes"`
	CandidateBytes int64    `json:"candidate_bytes"`
	Warnings       []string `json:"warnings,omitempty"`
}

type WorkspaceRecovery struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type ReconcileResult struct {
	Recovered int      `json:"recovered"`
	Failed    int      `json:"failed"`
	Warnings  []string `json:"warnings,omitempty"`
}

type quarantineManifest struct {
	Entry storage.QuarantineEntry `json:"entry"`
	Owner OwnershipMarker         `json:"owner"`
}
