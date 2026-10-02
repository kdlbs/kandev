package models

import "sort"

// WorkspaceRecoverySelectionSnapshot identifies the selected session,
// environment, and complete active repository inventory at recovery preflight.
// It is compared again before inspection and at the durable error write.
type WorkspaceRecoverySelectionSnapshot struct {
	TaskID    string
	SessionID string
	// SessionPersisted is false only for a prepared replacement session whose
	// row must remain absent until recovery admission completes.
	SessionPersisted         bool
	SessionTaskEnvironmentID string
	TaskEnvironmentID        string
	EnvironmentOwnerTaskID   string
	OwnershipGeneration      int64
	ExecutorType             string
	ExecutorID               string
	ExecutorProfileID        string
	EnvironmentStatus        string
	TaskDirName              string
	WorkspacePath            string
	Slots                    []WorkspaceRecoveryInventorySlot
}

// WorkspaceRecoveryInventorySlot identifies one active repository slot,
// including its physical worktree and the registered source repository.
type WorkspaceRecoveryInventorySlot struct {
	EnvironmentRepoID        string
	RepositoryID             string
	BranchSlug               string
	WorktreeID               string
	WorktreePath             string
	WorktreeBranch           string
	WorktreeBranchOwner      string
	WorktreeIntegrationRef   string
	WorktreeRecoveryHeadSHA  string
	WorktreeSourceClonePath  string
	WorktreeSourceCommonDir  string
	Position                 int
	Status                   string
	RepositoryPresent        bool
	RepositoryDeleted        bool
	RepositoryWorkspaceID    string
	RepositoryName           string
	RepositorySourceType     string
	RepositoryLocalPath      string
	RepositoryProvider       string
	RepositoryProviderRepoID string
	RepositoryProviderHost   string
	RepositoryProviderScope  string
	RepositoryProviderOwner  string
	RepositoryProviderName   string
	RepositoryRemoteURL      string
}

// NewWorkspaceRecoverySelectionSnapshot captures an environment and every
// active repository slot in a stable order. Missing repository entities remain
// explicit in the snapshot so callers can fail closed instead of omitting a slot.
func NewWorkspaceRecoverySelectionSnapshot(
	session *TaskSession,
	environment *TaskEnvironment,
	repositories map[string]*Repository,
) WorkspaceRecoverySelectionSnapshot {
	if session == nil || environment == nil {
		return WorkspaceRecoverySelectionSnapshot{}
	}

	snapshot := WorkspaceRecoverySelectionSnapshot{
		TaskID: session.TaskID, SessionID: session.ID,
		SessionPersisted:         true,
		SessionTaskEnvironmentID: session.TaskEnvironmentID,
		TaskEnvironmentID:        environment.ID, EnvironmentOwnerTaskID: environment.TaskID,
		OwnershipGeneration: environment.OwnershipGeneration,
		ExecutorType:        environment.ExecutorType, ExecutorID: environment.ExecutorID,
		ExecutorProfileID: environment.ExecutorProfileID,
		EnvironmentStatus: string(environment.Status), TaskDirName: environment.TaskDirName,
		WorkspacePath: environment.WorkspacePath,
	}
	for _, row := range environment.Repos {
		if row == nil || row.DeletedAt != nil || (row.Status != "" && row.Status != "active") {
			continue
		}
		slot := WorkspaceRecoveryInventorySlot{
			EnvironmentRepoID: row.ID, RepositoryID: row.RepositoryID,
			BranchSlug: row.BranchSlug, WorktreeID: row.WorktreeID,
			WorktreePath: row.WorktreePath, WorktreeBranch: row.WorktreeBranch,
			WorktreeBranchOwner:     row.WorktreeBranchOwner,
			WorktreeIntegrationRef:  row.WorktreeIntegrationRef,
			WorktreeRecoveryHeadSHA: row.WorktreeRecoveryHeadSHA,
			WorktreeSourceClonePath: row.WorktreeSourceClonePath,
			WorktreeSourceCommonDir: row.WorktreeSourceCommonDir,
			Position:                row.Position, Status: "active",
		}
		if repository := repositories[row.RepositoryID]; repository != nil {
			slot.RepositoryPresent = true
			slot.RepositoryDeleted = repository.DeletedAt != nil
			slot.RepositoryWorkspaceID = repository.WorkspaceID
			slot.RepositoryName = repository.Name
			slot.RepositorySourceType = repository.SourceType
			slot.RepositoryLocalPath = repository.LocalPath
			slot.RepositoryProvider = repository.Provider
			slot.RepositoryProviderRepoID = repository.ProviderRepoID
			slot.RepositoryProviderHost = repository.ProviderHost
			slot.RepositoryProviderScope = repository.ProviderScope
			slot.RepositoryProviderOwner = repository.ProviderOwner
			slot.RepositoryProviderName = repository.ProviderName
			slot.RepositoryRemoteURL = repository.RemoteURL
		}
		snapshot.Slots = append(snapshot.Slots, slot)
	}
	return snapshot.Canonical()
}

// Canonical returns a copy whose slots are ordered independently of query or
// map iteration order.
func (s WorkspaceRecoverySelectionSnapshot) Canonical() WorkspaceRecoverySelectionSnapshot {
	s.Slots = append([]WorkspaceRecoveryInventorySlot(nil), s.Slots...)
	sort.Slice(s.Slots, func(i, j int) bool {
		left, right := s.Slots[i], s.Slots[j]
		if left.EnvironmentRepoID != right.EnvironmentRepoID {
			return left.EnvironmentRepoID < right.EnvironmentRepoID
		}
		if left.RepositoryID != right.RepositoryID {
			return left.RepositoryID < right.RepositoryID
		}
		if left.BranchSlug != right.BranchSlug {
			return left.BranchSlug < right.BranchSlug
		}
		return left.Position < right.Position
	})
	return s
}

// Equal compares complete canonical selection identities.
func (s WorkspaceRecoverySelectionSnapshot) Equal(other WorkspaceRecoverySelectionSnapshot) bool {
	a, b := s.Canonical(), other.Canonical()
	return sameRecoverySelectionIdentity(a, b) && sameRecoverySelectionInventory(a.Slots, b.Slots)
}

func sameRecoverySelectionIdentity(a, b WorkspaceRecoverySelectionSnapshot) bool {
	return a.TaskID == b.TaskID && a.SessionID == b.SessionID &&
		a.SessionPersisted == b.SessionPersisted &&
		a.SessionTaskEnvironmentID == b.SessionTaskEnvironmentID &&
		a.TaskEnvironmentID == b.TaskEnvironmentID &&
		a.EnvironmentOwnerTaskID == b.EnvironmentOwnerTaskID &&
		a.OwnershipGeneration == b.OwnershipGeneration &&
		a.ExecutorType == b.ExecutorType && a.ExecutorID == b.ExecutorID &&
		a.ExecutorProfileID == b.ExecutorProfileID &&
		a.EnvironmentStatus == b.EnvironmentStatus && a.TaskDirName == b.TaskDirName &&
		a.WorkspacePath == b.WorkspacePath
}

func sameRecoverySelectionInventory(a, b []WorkspaceRecoveryInventorySlot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Valid reports whether the snapshot has enough identity to fence a write.
func (s WorkspaceRecoverySelectionSnapshot) Valid() bool {
	return s.TaskID != "" && s.SessionID != "" && s.TaskEnvironmentID != "" &&
		s.EnvironmentOwnerTaskID != "" && s.OwnershipGeneration > 0
}

// Present reports whether a caller attempted to supply a selection snapshot.
func (s WorkspaceRecoverySelectionSnapshot) Present() bool {
	return s.TaskID != "" || s.SessionID != "" || s.SessionTaskEnvironmentID != "" ||
		s.TaskEnvironmentID != "" || s.EnvironmentOwnerTaskID != "" ||
		s.OwnershipGeneration != 0 || len(s.Slots) > 0
}

// Complete reports whether each selected environment slot has an identifiable
// repository row. An incomplete slot cannot authorize admission or projection.
func (s WorkspaceRecoverySelectionSnapshot) Complete() bool {
	if !s.Valid() {
		return false
	}
	for _, slot := range s.Slots {
		if slot.EnvironmentRepoID == "" || slot.RepositoryID == "" ||
			!slot.RepositoryPresent || slot.RepositoryDeleted {
			return false
		}
	}
	return true
}
