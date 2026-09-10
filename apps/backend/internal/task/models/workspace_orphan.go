package models

// Keys and values used under a task's metadata.workspace sub-map. The
// producer (internal/task/service/handoff_workspace_orphan.go) writes the
// orphan marker; this file owns the single read-side derivation of it.
const (
	workspaceMetadataKey       = "workspace"
	WorkspaceOrphanedKey       = "orphaned"
	WorkspaceModeKey           = "mode"
	WorkspaceModeInheritParent = "inherit_parent"
)

// WorkspaceOrphaned reports whether task metadata carries a live
// orphaned-workspace marker: metadata.workspace.orphaned strictly boolean
// true, conjoined with metadata.workspace.mode == "inherit_parent". This is
// the single derivation site — ToAPI, dto.FromTaskWithSessionInfo,
// publishTaskEventNow, and the startup repair's SQL mirror all agree with it
// rather than re-testing the key independently.
//
// A presence test (as interrupted/auto_start_failed use) is wrong here
// because the clear path for this marker leaves the key removed only on the
// happy path; a value test also correctly reports false for a
// hand-written `"orphaned": false` (metadata is user-writable through the
// generic PATCH surface). The mode conjunct keeps the boolean honest against
// writers that move a task off inherit_parent without retracting the marker
// (reparent, detach, non-cascade delete) — none of which the marker's own
// clearing path can reach.
func WorkspaceOrphaned(metadata map[string]interface{}) bool {
	ws, ok := metadata[workspaceMetadataKey].(map[string]interface{})
	if !ok {
		return false
	}
	orphaned, _ := ws[WorkspaceOrphanedKey].(bool)
	mode, _ := ws[WorkspaceModeKey].(string)
	return orphaned && mode == WorkspaceModeInheritParent
}

// OrphanWriteGuard is the state a caller observed before mutating a task's
// metadata.workspace sub-map, plus which write-time preconditions must still
// hold. The two CAS fields are ALWAYS compared, including when empty: ""
// means "no string was stored", never an omission. Each Require* field adds
// one AND clause only when it is non-zero.
//
// Build the CAS fields from the workspace map as read, BEFORE stamp / clear /
// normalize mutates it in place — a wrapper that reads the map after
// mutation compares a just-written value against itself and matches
// nothing. Hosted here (rather than in service or sqlite) because
// SetTaskWorkspaceMetadataIfUnchanged (sqlite) and its callers (service) both
// already import models with no cycle risk in either direction.
type OrphanWriteGuard struct {
	// ExpectedOrphanedParentID and ExpectedMode are CAS fields, always
	// compared. A stored value that is absent, null, numeric, or an object
	// is read as "" by the same comma-ok string assertion the SQL mirrors.
	ExpectedOrphanedParentID string
	ExpectedMode             string

	// RequireParentArchivedID and RequireParentUnarchivedID require the
	// named task to exist and be archived / not archived respectively.
	// "" omits the clause.
	RequireParentArchivedID   string
	RequireParentUnarchivedID string
	// RequireParentID requires tasks.parent_id still equals this value.
	// "" omits the clause.
	RequireParentID string
	// RequireNoOwnEnvironment requires no task_environments row for this
	// task. false omits the clause.
	RequireNoOwnEnvironment bool
	// RequireTaskNotArchived requires the task's own archived_at IS NULL.
	// Stamping paths only; never set on a clearing path.
	RequireTaskNotArchived bool
}

// ObservedWorkspaceGuard builds the CAS half of a guard from a workspace map
// as read, before a caller mutates it. A nil map is safe to pass: Go reads
// from a nil map return zero values, so this is the correct observation of
// "nothing was stored".
func ObservedWorkspaceGuard(workspace map[string]interface{}) OrphanWriteGuard {
	claim, _ := workspace["orphaned_parent_id"].(string)
	mode, _ := workspace[WorkspaceModeKey].(string)
	return OrphanWriteGuard{ExpectedOrphanedParentID: claim, ExpectedMode: mode}
}

// OrphanRepairCandidate is one row eligible for the startup repair's
// stamping pass: a live, non-ephemeral, non-automation-origin inherit_parent
// child of an archived parent with no task_environments row of its own.
type OrphanRepairCandidate struct {
	TaskID    string
	ParentID  string
	Workspace map[string]interface{}
}

// StaleOrphanMarker is one row eligible for the startup repair's clearing
// pass: a task (possibly archived) carrying workspace.orphaned == true whose
// orphaned_parent_id names a task that exists and is not archived.
type StaleOrphanMarker struct {
	TaskID    string
	Workspace map[string]interface{}
}
