package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// publishUpdatedTask re-reads the task row and forwards it to the event
// publisher. Used by ArchiveTaskTree after stamping archived_at so the
// WS payload reflects the new column value (the frontend keys off
// archived_at to remove a card from the kanban).
func (s *HandoffService) publishUpdatedTask(ctx context.Context, taskID string) {
	if s.eventPublisher == nil {
		return
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	s.eventPublisher.PublishTaskUpdated(ctx, task)
}

// evaluateWorkspaceGroupCleanup runs the cleanup-pending state machine
// for a workspace group after a member release. User-owned groups
// (owned_by_kandev=false / cleanup policy never_delete) are a safety
// no-op. Kandev-owned groups whose last active member just left are
// transitioned to cleanup_pending and then to cleaned (or
// cleanup_failed) by dispatching to the configured WorkspaceCleaner.
//
// Disk operations are gated by THREE separate conditions:
//  1. group.OwnedByKandev=true (set only by MarkWorkspaceMaterialized)
//  2. group.CleanupPolicy=delete_when_last_member_archived_or_deleted
//  3. The cleaner's per-kind managed-root guard
//
// All three must hold before any file is touched.
func (s *HandoffService) evaluateWorkspaceGroupCleanup(ctx context.Context, groupID string) error {
	if s.wsGroups == nil {
		return nil
	}
	g, err := s.wsGroups.GetWorkspaceGroup(ctx, groupID)
	if err != nil || g == nil {
		return err
	}
	if !g.OwnedByKandev || g.CleanupPolicy != orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel {
		// User-owned or never-delete groups: stop. Cleanup_status is
		// left untouched so unarchive sees the prior state.
		return nil
	}
	members, err := s.wsGroups.ListActiveWorkspaceGroupMembers(ctx, groupID)
	if err != nil {
		return err
	}
	if len(members) > 0 {
		return nil
	}
	// SAFETY (post-review #5): even when no ACTIVE members remain in
	// the group, an executors_running row might still reference one
	// of the (now-released) member tasks because the cancel call
	// failed or hasn't propagated yet. Cleanup MUST refuse to delete
	// the materialized workspace until every member session is
	// confirmed stopped, otherwise we delete files an agent is still
	// writing to.
	hasActive, err := s.hasActiveExecutionsForGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if hasActive {
		// Leave the group in cleanup_pending with a clear reason so
		// the operator (and a follow-up evaluation after the executor
		// stops) can find it.
		return s.wsGroups.UpdateWorkspaceGroupCleanupStatus(ctx, groupID,
			orchmodels.WorkspaceCleanupStatusPending,
			"active executor still bound to group's member session", nil)
	}
	// Mark the group as cleanup_pending before invoking the cleaner so
	// any concurrent observer sees a consistent state machine.
	if err := s.wsGroups.UpdateWorkspaceGroupCleanupStatus(ctx, groupID,
		orchmodels.WorkspaceCleanupStatusPending, "", nil); err != nil {
		return err
	}
	if s.cleaner == nil {
		// No cleaner wired — leave the group in cleanup_pending so the
		// operator can see it awaiting a cleaner upgrade. This matches
		// pre-wiring behaviour.
		return nil
	}
	if err := s.runWorkspaceGroupCleanup(ctx, g); err != nil {
		_ = s.wsGroups.UpdateWorkspaceGroupCleanupStatus(ctx, groupID,
			orchmodels.WorkspaceCleanupStatusFailed, err.Error(), nil)
		return err
	}
	now := time.Now().UTC()
	return s.wsGroups.UpdateWorkspaceGroupCleanupStatus(ctx, groupID,
		orchmodels.WorkspaceCleanupStatusCleaned, "", &now)
}

// hasActiveExecutionsForGroup walks every task that ever belonged to
// the group (active + released) and checks whether any of its sessions
// still has an executors_running row. Returns true on the first hit so
// the cleanup state machine can short-circuit. When the
// SessionWorktreeReader isn't wired we conservatively report no
// activity (legacy / test path).
func (s *HandoffService) hasActiveExecutionsForGroup(ctx context.Context, groupID string) (bool, error) {
	if s.sessions == nil || s.wsGroups == nil {
		return false, nil
	}
	all, err := s.wsGroups.ListWorkspaceGroupMembers(ctx, groupID)
	if err != nil {
		return false, err
	}
	for _, m := range all {
		sessions, err := s.sessions.ListTaskSessions(ctx, m.TaskID)
		if err != nil {
			return false, err
		}
		for _, sess := range sessions {
			running, err := s.sessions.HasExecutorRunningRow(ctx, sess.ID)
			if err != nil {
				return false, err
			}
			if running {
				return true, nil
			}
		}
	}
	return false, nil
}

// CascadeOutcome summarises an ArchiveTaskTree / DeleteTaskTree run.
// Returned to callers (HTTP/MCP handlers) so they can render the right
// audit log + UI activity entries.
type CascadeOutcome struct {
	CascadeID        string
	ArchivedTaskIDs  []string // tasks whose archived_at was set by THIS cascade
	SkippedTaskIDs   []string // descendants already archived → left untouched
	ReleasedGroupIDs []string // workspace groups whose membership was released
}

// ArchiveTaskTree archives rootID and every non-archived descendant under
// a single cascade ID. Already-archived descendants are skipped so the
// later UnarchiveTaskTree restores exactly what this cascade owned.
//
// When cascade=false, only rootID is archived; descendants are left
// alone (used when subtasks might still be in progress).
//
// Steps (in order):
//  1. Collect the descendant set (BFS over parent_id).
//  2. Cancel active sessions / runs for every task in the set before
//     touching the task row, so the agent isn't writing to a workspace
//     we're about to release / clean.
//  3. CAS-archive each task with the cascade ID.
//  4. Release workspace-group membership for the tasks this cascade
//     archived, stamping the cascade ID on the released row.
//  5. Evaluate cleanup once per affected group.
func (s *HandoffService) ArchiveTaskTree(ctx context.Context, rootID string, cascade bool) (*CascadeOutcome, error) {
	if rootID == "" {
		return nil, errors.New("rootID is required")
	}
	if s.tasks == nil {
		return nil, errors.New("task repo not configured")
	}
	// Validate the root exists up front. The CAS archive below treats a
	// zero-row update as "skipped" (idempotent re-archive), which would
	// silently report success for a task ID that doesn't exist at all.
	if root, err := s.tasks.GetTask(ctx, rootID); err != nil {
		return nil, err
	} else if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	cascadeID := uuid.New().String()
	out := &CascadeOutcome{CascadeID: cascadeID}

	var all []string
	if cascade {
		descendants, err := s.collectTaskTree(ctx, rootID)
		if err != nil {
			return nil, err
		}
		all = descendants
	} else {
		all = []string{rootID}
	}

	// Cancel active runs first. Failures are logged and skipped — a
	// stuck cancel must not block the archive cascade; the orchestrator
	// will reconcile any orphan execution on its next tick.
	s.cancelActiveRuns(ctx, all, "task tree archived")

	// Archive deepest first so parent_id pointers stay valid through
	// the walk; not strictly required by the schema (no FK on parent_id)
	// but keeps the audit log readable.
	for i := len(all) - 1; i >= 0; i-- {
		ok, err := s.tasks.ArchiveTaskIfActive(ctx, all[i], cascadeID)
		if err != nil {
			return out, fmt.Errorf("archive %s: %w", all[i], err)
		}
		if ok {
			out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, all[i])
			// Re-read the row so the published event carries the freshly
			// stamped archived_at; the WS handler removes archived tasks
			// from the kanban board by checking that field. Service.ArchiveTask
			// does the same re-read before publishing — this matches that
			// path so the cascade looks identical to a single-task archive
			// from the frontend's perspective.
			s.publishUpdatedTask(ctx, all[i])
			// Tear down runtime resources (container/sandbox/worktree).
			// Cancellation above stopped the agent but does not remove the
			// container. Archive preserves the env row (deleteEnvRow=false).
			if s.resourceCleaner != nil {
				s.resourceCleaner.CleanupTaskResources(ctx, all[i], false)
			}
		} else {
			out.SkippedTaskIDs = append(out.SkippedTaskIDs, all[i])
		}
	}

	// Release group memberships for THIS cascade's tasks. Memberships
	// owned by an earlier cascade or manual archive are left alone.
	groupIDs, err := s.releaseMembershipsForCascade(ctx, out.ArchivedTaskIDs, orchmodels.WorkspaceReleaseReasonArchived, cascadeID)
	if err != nil {
		return out, err
	}
	out.ReleasedGroupIDs = groupIDs
	for _, gid := range groupIDs {
		if err := s.evaluateWorkspaceGroupCleanup(ctx, gid); err != nil {
			s.logf().Error("evaluate workspace group cleanup",
				zap.String("group_id", gid), zap.Error(err))
		}
	}
	return out, nil
}

// DeleteTaskTree is the inverse-of-archive operation: it walks rootID's
// descendants, cancels active runs, releases workspace-group
// memberships with reason=deleted, and removes every task row. Unlike
// archive, delete is permanent — there is no Undelete cascade.
//
// When cascade=false, only rootID is deleted; its direct children are
// reparented to root (parent_id="") before the row is removed so the
// orphaned subtasks stay queryable instead of holding a dangling
// pointer.
//
// Group memberships are released with reason=deleted so the cleanup
// evaluation runs the same path archive does (last active member gone
// → cleanup_pending → optionally cleaned). Tasks the user manually
// archived but not deleted before this cascade are still removed
// because deletion is unconditional; the cascade ID is stamped only
// for symmetry with archive.
func (s *HandoffService) DeleteTaskTree(ctx context.Context, rootID string, cascade bool) (*CascadeOutcome, error) {
	if rootID == "" {
		return nil, errors.New("rootID is required")
	}
	if s.tasks == nil {
		return nil, errors.New("task repo not configured")
	}
	cascadeID := uuid.New().String()
	out := &CascadeOutcome{CascadeID: cascadeID}

	all, err := s.resolveDeleteSet(ctx, rootID, cascade)
	if err != nil {
		return nil, err
	}

	s.cancelActiveRuns(ctx, all, "task tree deleted")

	// Release memberships BEFORE deleting the task rows so the
	// membership cleanup evaluation sees the group's full audit
	// history. Once tasks(id) cascade-deletes member rows we can no
	// longer log who left.
	groupIDs, err := s.releaseMembershipsForCascade(ctx, all, orchmodels.WorkspaceReleaseReasonDeleted, cascadeID)
	if err != nil {
		return out, err
	}
	out.ReleasedGroupIDs = groupIDs

	// Delete deepest first; failures abort the cascade and surface so
	// the caller can retry. We do NOT roll back partial deletions —
	// delete is destructive by design and re-running is idempotent.
	for i := len(all) - 1; i >= 0; i-- {
		// Snapshot the task row BEFORE deletion so the published event
		// carries workflow_id / workspace_id — the kanban WS handler keys
		// off workflow_id to remove the card from the right swimlane.
		// Fetch is best-effort: if the row is already gone (rare race) we
		// fall back to a minimal payload below.
		var snapshot *models.Task
		if s.eventPublisher != nil {
			snapshot, _ = s.tasks.GetTask(ctx, all[i])
		}
		// Tear down runtime resources BEFORE the DB delete so the env / worktree
		// rows are still queryable for the gather step. The actual destroy work
		// runs async after this returns. Delete cascade removes the env row.
		if s.resourceCleaner != nil {
			s.resourceCleaner.CleanupTaskResources(ctx, all[i], true)
		}
		if err := s.tasks.DeleteTask(ctx, all[i]); err != nil {
			return out, fmt.Errorf("delete %s: %w", all[i], err)
		}
		out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, all[i])
		if s.eventPublisher != nil && snapshot != nil {
			s.eventPublisher.PublishTaskDeleted(ctx, snapshot)
		}
	}

	for _, gid := range groupIDs {
		if err := s.evaluateWorkspaceGroupCleanup(ctx, gid); err != nil {
			s.logf().Error("evaluate workspace group cleanup",
				zap.String("group_id", gid), zap.Error(err))
		}
	}
	return out, nil
}

// cancelActiveRuns invokes the configured RunCanceller for every task
// in the cascade set. Failures are logged and skipped — the cascade
// proceeds even if a single cancellation fails so the user's archive /
// delete intent is honoured.
func (s *HandoffService) cancelActiveRuns(ctx context.Context, taskIDs []string, reason string) {
	if s.runCanceller == nil {
		return
	}
	for _, id := range taskIDs {
		if err := s.runCanceller.CancelTaskExecution(ctx, id, reason, false); err != nil {
			s.logf().Warn("cascade: cancel task execution failed",
				zap.String("task_id", id), zap.Error(err))
		}
	}
}

// UnarchiveTaskTree is the inverse of ArchiveTaskTree. It walks the
// archived descendants of rootID and restores only the tasks that were
// archived by the same cascade as the root. Tasks the user manually
// archived (empty cascade id) or that belong to an earlier cascade are
// left archived — fixing the resurrection bug from the original review.
func (s *HandoffService) UnarchiveTaskTree(ctx context.Context, rootID string) (*CascadeOutcome, error) {
	if rootID == "" {
		return nil, errors.New("rootID is required")
	}
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	cascadeID := root.ArchivedByCascadeID
	if cascadeID == "" {
		return s.unarchiveManualRoot(ctx, root)
	}
	out := &CascadeOutcome{CascadeID: cascadeID}
	// The descendant walk below uses metadata.parent_id only; archived
	// rows are still queryable so collectTaskTree (which reads tasks
	// via GetTask) sees them.
	all, err := s.collectArchivedTreeByCascade(ctx, rootID, cascadeID)
	if err != nil {
		return nil, err
	}
	// Unarchive shallow→deep so the root's restored state is visible
	// before children are queried by anyone watching the bus.
	for _, id := range all {
		ok, err := s.tasks.UnarchiveTaskByCascade(ctx, id, cascadeID)
		if err != nil {
			return out, fmt.Errorf("unarchive %s: %w", id, err)
		}
		if ok {
			out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, id)
			// Publish per restored task — the WS handler keys off
			// archived_at=null to put the card back on the kanban, same
			// as ArchiveTaskTree publishes per archived task.
			s.publishUpdatedTask(ctx, id)
		} else {
			out.SkippedTaskIDs = append(out.SkippedTaskIDs, id)
		}
	}
	// Restore group memberships scoped to the same cascade. Track the
	// set of affected groups so we can also re-evaluate cleanup state
	// (cleanup_status=cleaned → active + restored / restorable).
	groupIDs := map[string]bool{}
	if s.wsGroups != nil {
		for _, id := range out.ArchivedTaskIDs {
			if err := s.wsGroups.RestoreWorkspaceGroupMemberByCascade(ctx, id, cascadeID); err != nil {
				s.logf().Error("restore membership", zap.String("task_id", id), zap.Error(err))
				continue
			}
			if g, _ := s.wsGroups.GetWorkspaceGroupForTask(ctx, id); g != nil {
				groupIDs[g.ID] = true
			}
		}
	}
	if len(groupIDs) > 0 {
		ids := make([]string, 0, len(groupIDs))
		for id := range groupIDs {
			ids = append(ids, id)
		}
		out.ReleasedGroupIDs = ids
		s.restoreCleanedGroups(ctx, ids)
	}
	return out, nil
}

// unarchiveManualRoot restores a single task that was archived without a
// cascade stamp (legacy Service.ArchiveTask path or rows predating the
// cascade infrastructure). Only the root is restored — its descendants
// were archived independently, so resurrecting them here would reintroduce
// the resurrection bug the cascade scoping fixed.
func (s *HandoffService) unarchiveManualRoot(ctx context.Context, root *models.Task) (*CascadeOutcome, error) {
	if root.ArchivedAt == nil {
		return nil, errors.New("task is not archived")
	}
	out := &CascadeOutcome{}
	ok, err := s.tasks.UnarchiveTask(ctx, root.ID)
	if err != nil {
		return out, fmt.Errorf("unarchive %s: %w", root.ID, err)
	}
	if !ok {
		out.SkippedTaskIDs = append(out.SkippedTaskIDs, root.ID)
		return out, nil
	}
	out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, root.ID)
	s.publishUpdatedTask(ctx, root.ID)
	// Legacy archives never released group memberships, but the group may
	// have been cleaned since (e.g. by a later cascade on another member).
	// Restore the group's materialized workspace if it was cleaned. Best
	// effort, like the cascade path: restoreCleanedGroups marks failures as
	// restore_status=restore_failed so they surface via the context API.
	if s.wsGroups != nil {
		g, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, root.ID)
		if err != nil {
			s.logf().Error("lookup workspace group for unarchived task",
				zap.String("task_id", root.ID), zap.Error(err))
		} else if g != nil {
			out.ReleasedGroupIDs = []string{g.ID}
			s.restoreCleanedGroups(ctx, []string{g.ID})
		}
	}
	return out, nil
}

// resolveDeleteSet returns the set of task IDs DeleteTaskTree should
// remove. When cascade is true that's the full descendant tree
// (including archived rows). When cascade is false it's just rootID,
// and the helper first reparents direct children to root so the
// soon-deleted parent_id pointer doesn't dangle, then publishes a
// task.updated event for each reparented child so WS-driven clients
// refresh their cached parent_id.
func (s *HandoffService) resolveDeleteSet(ctx context.Context, rootID string, cascade bool) ([]string, error) {
	if cascade {
		// Delete must walk archived descendants too: a parent with
		// already-archived children must remove every row, not just
		// the non-archived ones (post-review #4).
		return s.collectTaskTreeIncludingArchived(ctx, rootID)
	}
	// Capture the affected children BEFORE the update so we can
	// publish task.updated for each one after the row change —
	// clients (kanban, sidebar) cache parent_id and would otherwise
	// keep displaying the children nested under the deleted parent
	// until a full reload.
	children, err := s.tasks.ListChildrenIncludingArchived(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("list direct children of %s: %w", rootID, err)
	}
	// Reparent MUST succeed before we touch the parent row —
	// continuing past a reparent error would leave children pointing
	// at a row we're about to delete, exactly the dangling-pointer
	// state the no-cascade path is designed to avoid.
	if err := s.tasks.ReparentDirectChildren(ctx, rootID, ""); err != nil {
		return nil, fmt.Errorf("reparent direct children of %s: %w", rootID, err)
	}
	for _, c := range children {
		s.publishUpdatedTask(ctx, c.ID)
	}
	return []string{rootID}, nil
}

// collectTaskTree returns rootID followed by every NON-ARCHIVED
// descendant in BFS order. Used by ArchiveTaskTree where the cascade
// only ever needs to archive currently-active rows.
func (s *HandoffService) collectTaskTree(ctx context.Context, rootID string) ([]string, error) {
	return s.collectTreeBFS(ctx, rootID, s.tasks.ListChildren)
}

// collectTaskTreeIncludingArchived returns rootID followed by every
// descendant including already-archived rows. Used by DeleteTaskTree
// where the cascade must remove archived descendants too (regression
// fix for post-review #4: a parent with archived children was leaving
// the children behind after delete).
func (s *HandoffService) collectTaskTreeIncludingArchived(ctx context.Context, rootID string) ([]string, error) {
	return s.collectTreeBFS(ctx, rootID, s.tasks.ListChildrenIncludingArchived)
}

type childLister func(ctx context.Context, parentID string) ([]*models.Task, error)

func (s *HandoffService) collectTreeBFS(ctx context.Context, rootID string, list childLister) ([]string, error) {
	out := []string{rootID}
	queue := []string{rootID}
	for len(queue) > 0 {
		batch := queue
		queue = nil
		for _, id := range batch {
			children, err := list(ctx, id)
			if err != nil {
				return nil, err
			}
			for _, c := range children {
				out = append(out, c.ID)
				queue = append(queue, c.ID)
			}
		}
	}
	return out, nil
}

// collectArchivedTreeByCascade walks rootID's subtree and returns every
// task tagged with the named cascade ID. Visits archived rows too so the
// full descendant set is reachable; we filter by cascade id rather than
// archived state so manual mid-cascade archives don't leak in.
func (s *HandoffService) collectArchivedTreeByCascade(ctx context.Context, rootID, cascadeID string) ([]string, error) {
	out := []string{rootID}
	queue := []string{rootID}
	for len(queue) > 0 {
		batch := queue
		queue = nil
		for _, id := range batch {
			t, _ := s.tasks.GetTask(ctx, id)
			if t == nil {
				continue
			}
			children, err := s.allChildrenIncludingArchived(ctx, id)
			if err != nil {
				return nil, err
			}
			for _, c := range children {
				if c.ArchivedByCascadeID == cascadeID {
					out = append(out, c.ID)
					queue = append(queue, c.ID)
				}
			}
		}
	}
	return out, nil
}

// allChildrenIncludingArchived returns every child of parentID, archived
// or not, so the unarchive walk can find tasks tagged with the matching
// cascade ID even after they were archived by ArchiveTaskTree.
func (s *HandoffService) allChildrenIncludingArchived(ctx context.Context, parentID string) ([]*models.Task, error) {
	return s.tasks.ListChildrenIncludingArchived(ctx, parentID)
}

// releaseMembershipsForCascade releases group membership for each task
// in the input slice and returns the unique set of group IDs that had
// at least one member released. Failures are logged and skipped — a
// failed release does not abort the cascade since the row stays in the
// audit log either way.
func (s *HandoffService) releaseMembershipsForCascade(ctx context.Context, taskIDs []string, reason, cascadeID string) ([]string, error) {
	if s.wsGroups == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	var groups []string
	for _, id := range taskIDs {
		g, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, id)
		if err != nil {
			s.logf().Error("lookup group for task", zap.String("task_id", id), zap.Error(err))
			continue
		}
		if g == nil {
			continue
		}
		if err := s.wsGroups.ReleaseWorkspaceGroupMember(ctx, g.ID, id, reason, cascadeID); err != nil {
			s.logf().Error("release membership",
				zap.String("group_id", g.ID), zap.String("task_id", id), zap.Error(err))
			continue
		}
		if !seen[g.ID] {
			seen[g.ID] = true
			groups = append(groups, g.ID)
		}
	}
	return groups, nil
}

func (s *HandoffService) logf() *logger.Logger {
	if s.logger == nil {
		return logger.Default()
	}
	return s.logger
}
