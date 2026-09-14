package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/taskdependencies"
	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

type workspaceEnvironmentRepository interface {
	taskEnvironmentOwnerTransferer
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
}

type workspaceScopedTaskReparenter interface {
	ReparentDirectChildrenInWorkspace(ctx context.Context, oldParentID, newParentID, workspaceID string) error
}

type structuralChildLister interface {
	ListStructuralChildrenLimited(ctx context.Context, parentID string, limit int) ([]*models.Task, error)
}
type taskDependencySnapshotter interface {
	ListTaskBlockers(context.Context, string) ([]*orchmodels.TaskBlocker, error)
	ListTasksBlockedBy(context.Context, string) ([]string, error)
}
type taskParentCompensationRestorer interface {
	RestoreTaskParentIfUnchanged(
		context.Context, string, string, string, string,
	) error
}

type taskDependencySnapshot struct {
	taskID   string
	outgoing []*orchmodels.TaskBlocker
	incoming []string
}
type autoArchiveTaskRepository interface {
	ArchiveTaskIfAutoArchiveEligible(
		ctx context.Context, id string, expectedUpdatedAt time.Time, cascadeID string,
	) (bool, error)
}

type workspaceEnvironmentOwnershipTransfer struct {
	groupID             string
	environmentID       string
	oldOwnerTaskID      string
	newOwnerTaskID      string
	resultingGeneration int64
}

func (s *HandoffService) lockArchiveCascade(rootID string) func() {
	lock := s.archiveCascadeLock.lockFor(rootID)
	lock.Lock()
	return lock.Unlock
}

// publishUpdatedTask re-reads the task row and forwards it to the event
// publisher. Used by ArchiveTaskTree after stamping archived_at so the
// WS payload reflects the new column value (the frontend keys off
// archived_at to remove a card from the kanban).
func (s *HandoffService) publishUpdatedTask(ctx context.Context, taskID string) error {
	if s.eventPublisher == nil {
		return nil
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("reload task %s for lifecycle event: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("task %s disappeared before lifecycle event", taskID)
	}
	s.eventPublisher.PublishTaskUpdated(ctx, task)
	return nil
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
	// Cleanup must refuse to delete a shared workspace while any member
	// session still has an executor row that may be writing to it.
	// Confirm every member session is stopped before deleting its files.
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
	// Claim this ownership generation before invoking the cleaner. A stale
	// evaluator or a concurrent membership admission cannot pass this write.
	claimed, err := claimWorkspaceGroupCleanup(ctx, s.wsGroups, g)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	if s.cleaner == nil {
		// No cleaner wired — leave the group in cleanup_pending so the
		// operator can see it awaiting a cleaner upgrade. This matches
		// pre-wiring behaviour.
		return nil
	}
	if err := s.runWorkspaceGroupCleanup(ctx, g); err != nil {
		_ = completeWorkspaceGroupCleanup(ctx, s.wsGroups, g,
			orchmodels.WorkspaceCleanupStatusFailed, err.Error(), nil)
		return err
	}
	now := time.Now().UTC()
	return completeWorkspaceGroupCleanup(ctx, s.wsGroups, g,
		orchmodels.WorkspaceCleanupStatusCleaned, "", &now)
}

// hasActiveExecutionsForGroup walks every task that ever belonged to
// the group (active + released) and checks whether any of its sessions
// still has an executors_running row. Returns true on the first hit so
// cleanup state machine can short-circuit. Missing session evidence is an
// error, never proof of inactivity.
func (s *HandoffService) hasActiveExecutionsForGroup(ctx context.Context, groupID string) (bool, error) {
	if s.wsGroups == nil {
		return false, errors.New("workspace group repository unavailable for workspace cleanup")
	}
	all, err := s.wsGroups.ListWorkspaceGroupMembers(ctx, groupID)
	if err != nil {
		return false, err
	}
	if len(all) == 0 {
		return false, nil
	}
	if s.sessions == nil {
		return false, errors.New("session reader unavailable for workspace cleanup")
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

// CascadePostCommitError reports housekeeping failure after the lifecycle
// mutation has committed. Callers can acknowledge the durable task outcome
// while retaining the error for observability and retry handling.
type CascadePostCommitError struct {
	Err error
}

func (e *CascadePostCommitError) Error() string {
	return e.Err.Error()
}

func (e *CascadePostCommitError) Unwrap() error {
	return e.Err
}

func cascadePostCommitError(out *CascadeOutcome, err error) error {
	if err == nil || out == nil || len(out.ArchivedTaskIDs) == 0 {
		return err
	}
	return &CascadePostCommitError{Err: err}
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
//  2. Transfer a shared environment off a departing owner when another
//     active group member will remain, then snapshot cleanup handles.
//  3. Cancel active sessions / runs for every task in the set before
//     touching the task row, so the agent isn't writing to a workspace
//     we're about to release / clean.
//  4. CAS-archive each task with the cascade ID.
//  5. Release workspace-group membership for the tasks this cascade
//     archived, stamping the cascade ID on the released row.
//  6. Evaluate cleanup once per affected group.
func (s *HandoffService) ArchiveTaskTree(ctx context.Context, rootID string, cascade bool) (*CascadeOutcome, error) {
	return s.archiveTaskTree(ctx, rootID, cascade, nil)
}

// ArchiveAutoTask archives a candidate only if the task row and its current
// workflow step still satisfy the auto-archive policy.
func (s *HandoffService) ArchiveAutoTask(ctx context.Context, candidate *models.Task) (*CascadeOutcome, error) {
	if candidate == nil {
		return nil, errors.New("auto-archive candidate is required")
	}
	return s.archiveTaskTree(ctx, candidate.ID, false, candidate)
}

func (s *HandoffService) validateArchiveRoot(ctx context.Context, rootID string) error {
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return err
	}
	if root == nil {
		return taskrepo.ErrTaskNotFound
	}
	return nil
}

func (s *HandoffService) archiveTaskTree(
	ctx context.Context,
	rootID string,
	cascade bool,
	autoArchiveCandidate *models.Task,
) (*CascadeOutcome, error) {
	archiveDeadline := archivecascade.ArchiveDeadline(ctx)
	archiveCtx, cancelArchive := context.WithDeadline(ctx, archiveDeadline)
	defer cancelArchive()
	if err := s.authorizeTask(archiveCtx, rootID); err != nil {
		return nil, err
	}
	if rootID == "" {
		return nil, errors.New("rootID is required")
	}
	if s.tasks == nil {
		return nil, errors.New("task repo not configured")
	}
	// Validate the root before CAS can turn an unknown ID into a no-op.
	if err := s.validateArchiveRoot(archiveCtx, rootID); err != nil {
		return nil, err
	}
	cascadeLock := s.archiveCascadeLock.lockFor(rootID)
	cascadeLock.Lock()
	defer cascadeLock.Unlock()
	cascadeID, all, err := s.resolveArchiveCascade(archiveCtx, rootID, cascade)
	if err != nil {
		return nil, err
	}
	out := &CascadeOutcome{CascadeID: cascadeID}
	// Archive cleanup must not tear down a shared workspace while an active
	// group member remains. Transfer ownership before taking the cleanup
	// snapshot, using the same ownership handoff as delete cascades.
	transferCompensationCtx, cancelTransferCompensation := archivecascade.ContinuationContextUntil(ctx, archiveDeadline)
	defer cancelTransferCompensation()
	ownershipTransfers, err := s.transferSharedWorkspaceEnvironmentOwnership(
		archiveCtx, transferCompensationCtx, all)
	if err != nil {
		return out, err
	}
	cleanupOps, err := s.prepareCascadeResourceCleanupWithCompensation(
		archiveCtx, transferCompensationCtx, archiveDeadline, all, cascadeID,
		models.TaskResourceCleanupTriggerCascadeArchive, false)
	if err != nil {
		return out, s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(
			transferCompensationCtx, ownershipTransfers, err)
	}
	if cascade {
		s.rememberPartialArchiveCascade(rootID, cascadeID)
		if err := s.persistPartialArchiveCascadeMarker(archiveCtx, rootID, cascadeID); err != nil {
			s.forgetPartialArchiveCascade(rootID)
			return out, err
		}
	}
	archiveContinuationCtx, cancelArchiveContinuation := archivecascade.ContinuationContextUntil(ctx, archiveDeadline)
	defer cancelArchiveContinuation()
	postArchiveCtx := archiveContinuationCtx

	// Capture active session repositories before cancellation and archive
	// mutation. Snapshot failures are best effort, matching the legacy path.
	s.captureArchiveSnapshots(postArchiveCtx, all)
	s.cancelArchiveRunsForCandidate(postArchiveCtx, all, autoArchiveCandidate)
	// Archive deepest first so parent_id pointers stay valid through the walk;
	// not strictly required by the schema, but keeps the audit log readable.
	vacatedStepIDs := make(map[string]struct{})
	defer func() {
		s.pullTasksForVacatedSteps(postArchiveCtx, archiveDeadline, vacatedStepIDs)
	}()
	cleanupErrors, mutationErr := s.applyArchiveTaskMutations(
		postArchiveCtx, archiveDeadline, all, cascadeID, autoArchiveCandidate,
		cleanupOps, out, vacatedStepIDs,
	)
	if mutationErr != nil {
		return out, mutationErr
	}
	cleanupErrors, finishErr := s.finishArchiveTaskTree(
		postArchiveCtx, transferCompensationCtx, archiveDeadline,
		rootID, cascade, cascadeID, ownershipTransfers,
		autoArchiveCandidate, out, cleanupErrors,
	)
	if finishErr != nil {
		return out, finishErr
	}
	return out, cascadePostCommitError(out, errors.Join(cleanupErrors...))
}

func (s *HandoffService) finishArchiveTaskTree(
	postArchiveCtx, transferCompensationCtx context.Context,
	archiveDeadline time.Time,
	rootID string,
	cascade bool,
	cascadeID string,
	ownershipTransfers []workspaceEnvironmentOwnershipTransfer,
	autoArchiveCandidate *models.Task,
	out *CascadeOutcome,
	cleanupErrors []error,
) ([]error, error) {
	if cascade {
		if err := s.clearPartialArchiveCascadeMarker(postArchiveCtx, rootID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		s.forgetPartialArchiveCascade(rootID)
	}
	cleanupErrors = append(cleanupErrors, s.rollbackAutoArchiveCASLoss(
		transferCompensationCtx, ownershipTransfers, autoArchiveCandidate, out,
		cleanupErrors,
	))
	groupIDs, membershipErrors, membershipErr := s.releaseAndEvaluateMemberships(
		postArchiveCtx, out.ArchivedTaskIDs,
		orchmodels.WorkspaceReleaseReasonArchived, cascadeID,
	)
	out.ReleasedGroupIDs = groupIDs
	cleanupErrors = append(cleanupErrors, membershipErrors...)
	if membershipErr != nil {
		return cleanupErrors, cascadePostCommitError(
			out, errors.Join(membershipErr, errors.Join(cleanupErrors...)),
		)
	}
	return cleanupErrors, nil
}

func (s *HandoffService) applyArchiveTaskMutations(
	ctx context.Context,
	archiveDeadline time.Time,
	all []string,
	cascadeID string,
	autoArchiveCandidate *models.Task,
	cleanupOps map[string]string,
	out *CascadeOutcome,
	vacatedStepIDs map[string]struct{},
) ([]error, error) {
	var cleanupErrors []error
	for i := len(all) - 1; i >= 0; i-- {
		vacatedStepID, ok, err := s.archiveTaskWithVacatedStep(
			ctx, all[i], cascadeID, autoArchiveCandidate,
		)
		if err != nil {
			cancelErr := s.cancelCascadeResourceCleanupRange(ctx, all[:i+1], cleanupOps)
			return nil, errors.Join(fmt.Errorf("archive %s: %w", all[i], err), cancelErr)
		}
		if ok {
			out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, all[i])
			recordVacatedStep(vacatedStepIDs, vacatedStepID)
			if autoArchiveCandidate != nil {
				s.cancelActiveRuns(ctx, []string{all[i]}, models.SessionArchiveTreeCancelReason)
			}
			cleanupErrors = appendTaskCleanupError(
				cleanupErrors,
				s.finalizeActiveSessions(ctx, archiveDeadline, all[i], models.SessionArchiveTreeCancelReason),
			)
			if err := s.publishUpdatedTask(ctx, all[i]); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("publish archived task %s: %w", all[i], err))
			}
			s.markOrphanedInheritParentChildren(ctx, &models.Task{ID: all[i]})
			if operationID := cleanupOps[all[i]]; operationID != "" {
				if err := s.startCascadeResourceCleanup(ctx, operationID); err != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("start cleanup %s: %w", operationID, err))
				}
			} else if s.resourceCleaner != nil {
				s.resourceCleaner.CleanupTaskResources(ctx, all[i], false)
			}
		} else {
			if err := s.cancelSkippedCascadeResourceCleanup(ctx, all[i], cascadeID, cleanupOps[all[i]]); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("cancel cleanup %s: %w", cleanupOps[all[i]], err))
			}
			out.SkippedTaskIDs = append(out.SkippedTaskIDs, all[i])
		}
	}

	return cleanupErrors, nil
}

func (s *HandoffService) cancelSkippedCascadeResourceCleanup(
	ctx context.Context,
	taskID, cascadeID, operationID string,
) error {
	if operationID == "" {
		return nil
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task != nil && task.ArchivedByCascadeID == cascadeID {
		return nil
	}
	return s.cancelCascadeResourceCleanup(ctx, operationID)
}

func appendTaskCleanupError(cleanupErrors []error, err error) []error {
	if err == nil {
		return cleanupErrors
	}
	return append(cleanupErrors, err)
}
func (s *HandoffService) captureArchiveSnapshots(ctx context.Context, taskIDs []string) {
	if s.gitArchiveCapture == nil || s.sessions == nil {
		return
	}
	reader, ok := s.sessions.(activeTaskSessionReader)
	if !ok {
		return
	}
	for _, taskID := range taskIDs {
		activeSessions, err := reader.ListActiveTaskSessionsByTaskID(ctx, taskID)
		if err != nil {
			s.logf().Warn("failed to list active sessions for archive snapshot",
				zap.String("task_id", taskID), zap.Error(err))
			continue
		}
		for _, session := range activeSessions {
			if session == nil || session.ID == "" {
				continue
			}
			snapshotCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := s.gitArchiveCapture.CaptureArchiveSnapshot(snapshotCtx, session.ID)
			cancel()
			if err != nil {
				s.logf().Warn("failed to capture git archive snapshot",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.Error(err))
			}
		}
	}
}

func (s *HandoffService) releaseAndEvaluateMemberships(
	ctx context.Context,
	taskIDs []string,
	reason, cascadeID string,
) ([]string, []error, error) {
	groupIDs, releaseErr := s.releaseMembershipsForCascade(ctx, taskIDs, reason, cascadeID)
	cleanupErrors := s.evaluateWorkspaceGroups(ctx, groupIDs)
	return groupIDs, cleanupErrors, releaseErr
}

func (s *HandoffService) evaluateWorkspaceGroups(ctx context.Context, groupIDs []string) []error {
	var cleanupErrors []error
	for _, gid := range groupIDs {
		if err := s.evaluateWorkspaceGroupCleanup(ctx, gid); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("evaluate workspace group cleanup %s: %w", gid, err))
		}
	}
	return cleanupErrors
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
	return s.DeleteTaskTreeWithOptions(ctx, rootID, cascade, DeleteTaskOptions{})
}

// DeleteTaskTreeWithReason preserves the machine-readable reason on the
// task.deleted event while using the same cascade lifecycle.
func (s *HandoffService) DeleteTaskTreeWithReason(
	ctx context.Context, rootID string, cascade bool, reason string,
) (*CascadeOutcome, error) {
	return s.deleteTaskTreeWithReasonAndOptions(ctx, rootID, cascade, reason, DeleteTaskOptions{})
}

// DeleteTaskTreeWithOptions deletes a task tree after all owned worktrees have
// passed the dirty-worktree admission check. Consent is persisted in each
// cleanup snapshot before any task row is mutated.
func (s *HandoffService) DeleteTaskTreeWithOptions(
	ctx context.Context, rootID string, cascade bool, options DeleteTaskOptions,
) (*CascadeOutcome, error) {
	return s.deleteTaskTreeWithReasonAndOptions(ctx, rootID, cascade, "", options)
}

func (s *HandoffService) deleteTaskTreeWithReasonAndOptions(
	ctx context.Context, rootID string, cascade bool, reason string, options DeleteTaskOptions,
) (*CascadeOutcome, error) {
	deleteDeadline := archivecascade.ArchiveDeadline(ctx)
	deleteCtx, cancelDelete := context.WithDeadline(ctx, deleteDeadline)
	defer cancelDelete()
	if err := s.authorizeTask(deleteCtx, rootID); err != nil {
		return nil, err
	}
	if rootID == "" {
		return nil, errors.New("rootID is required")
	}
	if s.tasks == nil {
		return nil, errors.New("task repo not configured")
	}
	defer s.lockArchiveCascade(rootID)()
	cascadeID := uuid.New().String()
	out := &CascadeOutcome{CascadeID: cascadeID}
	all, err := s.resolveDeleteSet(deleteCtx, rootID, cascade)
	if err != nil {
		return nil, err
	}
	if checker, ok := s.resourceCleaner.(taskDeleteWorktreeAdmissionChecker); ok {
		if err := checker.ValidateTaskDeleteWorktrees(deleteCtx, all, options.DiscardWorktreeChanges); err != nil {
			return nil, err
		}
	}
	transferCompensationCtx, cancelTransferCompensation :=
		archivecascade.ContinuationContextUntil(ctx, deleteDeadline)
	defer cancelTransferCompensation()
	ownershipTransfers, cleanupOps, err := s.prepareDeleteCascade(
		deleteCtx, transferCompensationCtx, deleteDeadline, all, cascadeID,
		options.DiscardWorktreeChanges,
	)
	if err != nil {
		return out, err
	}
	deleteCompensationCtx, cancelDeleteCompensation := archivecascade.ContinuationContextUntil(ctx, deleteDeadline)
	defer cancelDeleteCompensation()
	postDeleteCtx := deleteCompensationCtx
	var noCascadeSnapshots []*models.Task
	if !cascade {
		noCascadeSnapshots, err = s.reparentNoCascadeChildren(postDeleteCtx, rootID)
		if err != nil {
			cancelErr := s.cancelCascadeResourceCleanupRange(deleteCompensationCtx, all, cleanupOps)
			rollbackErr := s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(
				deleteCompensationCtx, ownershipTransfers, err)
			return out, errors.Join(err, rollbackErr, cancelErr)
		}
	}

	s.cancelActiveRuns(postDeleteCtx, all, "task tree deleted")
	// Release memberships BEFORE deleting the task rows so the
	// membership cleanup evaluation sees the group's full audit
	// history. Once tasks(id) cascade-deletes member rows we can no
	// longer log who left.
	groupIDs, err := s.releaseMembershipsForCascade(postDeleteCtx, all, orchmodels.WorkspaceReleaseReasonDeleted, cascadeID)
	if err != nil {
		cancelErr := s.cancelCascadeResourceCleanupRange(deleteCompensationCtx, all, cleanupOps)
		rollbackErr := s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(
			deleteCompensationCtx, ownershipTransfers, err)
		restoreErr := s.restoreReleasedMemberships(deleteCompensationCtx, all, cascadeID, nil)
		childRestoreErr := s.restoreNoCascadeChildren(deleteCompensationCtx, noCascadeSnapshots)
		return out, errors.Join(err, rollbackErr, cancelErr, restoreErr, childRestoreErr)
	}
	out.ReleasedGroupIDs = groupIDs

	// Delete deepest first; failures abort the cascade and surface so the
	// caller can retry. We do NOT roll back partial deletions —
	// delete is destructive by design and re-running is idempotent.

	vacatedStepIDs := make(map[string]struct{})
	defer func() {
		s.pullTasksForVacatedSteps(postDeleteCtx, deleteDeadline, vacatedStepIDs)
	}()
	cleanupErrors, err := s.deleteTaskTreeRows(
		postDeleteCtx, deleteCompensationCtx, deleteDeadline, all, cleanupOps,
		out, ownershipTransfers, vacatedStepIDs, reason,
	)
	if err != nil {
		return out, errors.Join(err, s.compensateDeleteFailure(
			deleteCompensationCtx, all, cascadeID, out.ArchivedTaskIDs, noCascadeSnapshots,
		))
	}

	cleanupErrors = append(cleanupErrors, s.evaluateWorkspaceGroups(postDeleteCtx, groupIDs)...)
	return out, cascadePostCommitError(out, errors.Join(cleanupErrors...))
}
func (s *HandoffService) prepareDeleteCascade(
	deleteCtx, compensationCtx context.Context,
	deadline time.Time,
	taskIDs []string,
	cascadeID string,
	discardWorktreeChanges bool,
) ([]workspaceEnvironmentOwnershipTransfer, map[string]string, error) {
	ownershipTransfers, err := s.transferSharedWorkspaceEnvironmentOwnership(
		deleteCtx, compensationCtx, taskIDs,
	)
	if err != nil {
		return nil, nil, err
	}
	cleanupOps, err := s.prepareCascadeResourceCleanupWithCompensation(
		deleteCtx, compensationCtx, deadline, taskIDs, cascadeID,
		models.TaskResourceCleanupTriggerCascadeDelete, discardWorktreeChanges,
	)
	if err != nil {
		return nil, nil, s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(
			compensationCtx, ownershipTransfers, err,
		)
	}
	return ownershipTransfers, cleanupOps, nil
}

func (s *HandoffService) restoreReleasedMemberships(
	ctx context.Context,
	taskIDs []string,
	cascadeID string,
	deletedTaskIDs []string,

) error {
	if s.wsGroups == nil || cascadeID == "" {
		return nil
	}
	deleted := make(map[string]struct{}, len(deletedTaskIDs))
	for _, taskID := range deletedTaskIDs {
		deleted[taskID] = struct{}{}
	}
	var errs []error
	for _, taskID := range taskIDs {
		if _, ok := deleted[taskID]; ok {
			continue
		}
		if err := s.wsGroups.RestoreWorkspaceGroupMemberByCascade(ctx, taskID, cascadeID); err != nil {
			errs = append(errs, fmt.Errorf("restore membership for task %s: %w", taskID, err))
		}
	}
	return errors.Join(errs...)
}
func (s *HandoffService) compensateDeleteFailure(
	ctx context.Context,
	taskIDs []string,
	cascadeID string,
	deletedTaskIDs []string,
	noCascadeSnapshots []*models.Task,
) error {
	restoreErr := s.restoreReleasedMemberships(ctx, taskIDs, cascadeID, deletedTaskIDs)
	if len(deletedTaskIDs) == 0 {
		restoreErr = errors.Join(restoreErr, s.restoreNoCascadeChildren(ctx, noCascadeSnapshots))
	}
	return restoreErr
}

func (s *HandoffService) deleteTaskTreeRows(
	postDeleteCtx, deleteCompensationCtx context.Context,
	deleteDeadline time.Time,
	all []string,
	cleanupOps map[string]string,
	out *CascadeOutcome,
	ownershipTransfers []workspaceEnvironmentOwnershipTransfer,
	vacatedStepIDs map[string]struct{},
	reason string,
) ([]error, error) {
	var cleanupErrors []error
	for i := len(all) - 1; i >= 0; i-- {
		// Snapshot the task row BEFORE deletion so the published event
		// carries workflow_id / workspace_id. A reload failure is fatal:
		// deleting without the snapshot would silently lose the lifecycle event.
		var snapshot *models.Task
		if s.eventPublisher != nil {
			var snapshotErr error
			snapshot, snapshotErr = s.tasks.GetTask(postDeleteCtx, all[i])
			if snapshotErr != nil || snapshot == nil {
				cancelErr := s.cancelCascadeResourceCleanupRange(deleteCompensationCtx, all[:i+1], cleanupOps)
				if len(out.ArchivedTaskIDs) == 0 {
					snapshotErr = s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(
						deleteCompensationCtx, ownershipTransfers,
						errors.Join(snapshotErr, errors.New("task snapshot is missing before delete")),
					)
				}
				return nil, errors.Join(snapshotErr, cancelErr)
			}
		}
		// Remove dependency edges before the task row so a transient cleanup
		// failure cannot leave edges pointing at a deleted task. Keep the
		// process-wide mutation boundary through deletion and compensation.
		dependencyUnlock := taskdependencies.AcquireMutationLock()
		dependencySnapshot, err := s.deleteTaskDependencyEdgesLocked(postDeleteCtx, all[i])
		if err != nil {
			restoreErr := s.restoreTaskDependencyEdgesLocked(deleteCompensationCtx, dependencySnapshot)
			dependencyUnlock()
			cancelErr := s.cancelCascadeResourceCleanupRange(deleteCompensationCtx, all[:i+1], cleanupOps)
			deleteErr := fmt.Errorf("clean up task dependencies %s: %w", all[i], err)
			if len(out.ArchivedTaskIDs) == 0 {
				deleteErr = s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(deleteCompensationCtx, ownershipTransfers, deleteErr)
			}
			return nil, errors.Join(deleteErr, restoreErr, cancelErr)
		}
		// Tear down runtime resources BEFORE the DB delete so the env / worktree
		// rows are still queryable for the gather step. The actual destroy work
		// runs async after this returns. Delete cascade removes the env row.
		vacatedStepID, err := s.deleteTaskWithVacatedStep(postDeleteCtx, all[i])
		if err != nil {
			restoreErr := s.restoreTaskDependencyEdgesLocked(deleteCompensationCtx, dependencySnapshot)
			dependencyUnlock()
			cancelErr := s.cancelCascadeResourceCleanupRange(deleteCompensationCtx, all[:i+1], cleanupOps)
			deleteErr := fmt.Errorf("delete %s: %w", all[i], err)
			if len(out.ArchivedTaskIDs) == 0 {
				deleteErr = s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(deleteCompensationCtx, ownershipTransfers, deleteErr)
			}
			return nil, errors.Join(deleteErr, restoreErr, cancelErr)
		}
		dependencyUnlock()
		s.publishDeletedTaskDependencies(postDeleteCtx, dependencySnapshot)
		recordVacatedStep(vacatedStepIDs, vacatedStepID)
		out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, all[i])
		cleanupErrors = appendTaskCleanupError(
			cleanupErrors,
			s.finalizeActiveSessions(postDeleteCtx, deleteDeadline, all[i], "task tree deleted"),
		)
		if operationID := cleanupOps[all[i]]; operationID != "" {
			if err := s.startCascadeResourceCleanup(postDeleteCtx, operationID); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("start cleanup %s: %w", operationID, err))
			}
		} else if s.resourceCleaner != nil {
			s.resourceCleaner.CleanupTaskResources(postDeleteCtx, all[i], true)
		}
		if s.eventPublisher != nil && snapshot != nil {
			s.publishDeletedTaskEvent(postDeleteCtx, snapshot, reason)
		}
	}
	return cleanupErrors, nil
}

func (s *HandoffService) publishDeletedTaskEvent(
	ctx context.Context,
	task *models.Task,
	reason string,
) {
	if reason != "" {
		if publisher, ok := s.eventPublisher.(TaskDeletedEventPublisherWithExtra); ok {
			publisher.PublishTaskDeletedWithExtra(ctx, task, map[string]interface{}{"reason": reason})
			return
		}
	}
	s.eventPublisher.PublishTaskDeleted(ctx, task)
}
func (s *HandoffService) publishDeletedTaskDependencies(
	ctx context.Context,
	snapshot *taskDependencySnapshot,
) {
	if snapshot == nil {
		return
	}
	publisher, ok := s.eventPublisher.(dependencyChangePublisher)
	if !ok {
		return
	}
	affected := append([]string{}, snapshot.incoming...)
	for _, blocker := range snapshot.outgoing {
		if blocker != nil {
			affected = append(affected, blocker.BlockerTaskID)
		}
	}
	// Deleting this task changes both surviving dependents' blocked
	// projections and surviving predecessors' dependent projections.
	publisher.PublishDependencyChange(ctx, affected...)
}
func (s *HandoffService) archiveTaskWithVacatedStep(
	ctx context.Context,
	taskID string,
	cascadeID string,
	autoArchiveCandidate *models.Task,
) (string, bool, error) {
	if autoArchiveCandidate != nil {
		repo, ok := s.tasks.(autoArchiveTaskRepository)
		if !ok {
			return "", false, errors.New("task repo cannot validate auto-archive eligibility atomically")
		}
		changed, err := repo.ArchiveTaskIfAutoArchiveEligible(
			ctx, taskID, autoArchiveCandidate.UpdatedAt, cascadeID,
		)
		if !changed || err != nil {
			return "", changed, err
		}
		return autoArchiveCandidate.WorkflowStepID, true, nil
	}
	repo, ok := s.tasks.(cascadeArchiveTaskRepository)
	if !ok {
		return "", false, errors.New("task repo cannot capture archive vacancy atomically")
	}
	return repo.ArchiveTaskIfActiveWithVacatedStep(ctx, taskID, cascadeID)
}

func (s *HandoffService) deleteTaskDependencyEdgesLocked(
	ctx context.Context,
	taskID string,
) (*taskDependencySnapshot, error) {
	if s.blockers == nil {
		return nil, nil
	}
	cleaner, ok := s.blockers.(taskDependencyCleaner)
	if !ok {
		return nil, errors.New("blocker repository cannot delete task dependency edges")
	}
	var snapshot *taskDependencySnapshot
	if snapshotter, ok := s.blockers.(taskDependencySnapshotter); ok {
		outgoing, err := snapshotter.ListTaskBlockers(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("snapshot outgoing task dependencies: %w", err)
		}
		incoming, err := snapshotter.ListTasksBlockedBy(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("snapshot incoming task dependencies: %w", err)
		}
		snapshot = &taskDependencySnapshot{taskID: taskID, outgoing: outgoing, incoming: incoming}
	}
	if err := cleaner.DeleteTaskBlockersForTask(ctx, taskID); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (s *HandoffService) restoreTaskDependencyEdgesLocked(
	ctx context.Context,
	snapshot *taskDependencySnapshot,
) error {
	if snapshot == nil || s.blockers == nil {
		return nil
	}
	var errs []error
	for _, blocker := range snapshot.outgoing {
		if blocker == nil {
			continue
		}
		if err := s.blockers.CreateTaskBlocker(ctx, blocker); err != nil {
			errs = append(errs, err)
		}
	}
	for _, dependentID := range snapshot.incoming {
		if err := s.blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{
			TaskID: dependentID, BlockerTaskID: snapshot.taskID,
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *HandoffService) deleteTaskWithVacatedStep(ctx context.Context, taskID string) (string, error) {
	repo, ok := s.tasks.(cascadeDeleteTaskRepository)
	if !ok {
		return "", errors.New("task repo cannot capture delete vacancy atomically")
	}
	return repo.DeleteTaskWithVacatedStep(ctx, taskID)
}

func recordVacatedStep(stepIDs map[string]struct{}, stepID string) {
	if stepID == "" {
		return
	}
	stepIDs[stepID] = struct{}{}
}

func (s *HandoffService) pullTasksForVacatedSteps(
	ctx context.Context,
	deadline time.Time,
	stepIDs map[string]struct{},
) {
	if s.vacancyReconciler == nil || len(stepIDs) == 0 {
		return
	}
	orderedStepIDs := make([]string, 0, len(stepIDs))
	for stepID := range stepIDs {
		orderedStepIDs = append(orderedStepIDs, stepID)
	}
	sort.Strings(orderedStepIDs)
	pullCtx, cancel := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancel()
	for _, stepID := range orderedStepIDs {
		s.vacancyReconciler.ReconcileVacatedStep(pullCtx, stepID)
	}
}

// transferSharedWorkspaceEnvironmentOwnership moves a materialized environment
// off a task about to leave its workspace group when another active member will
// remain. Lifecycle cleanup is scoped by task owner, so leaving the environment
// attached to the departing member could destroy shared workspace state before
// group cleanup gets a chance to enforce its last-member rule.
func (s *HandoffService) transferSharedWorkspaceEnvironmentOwnership(
	ctx context.Context,
	compensationCtx context.Context,
	taskIDs []string,
) ([]workspaceEnvironmentOwnershipTransfer, error) {
	if s.wsGroups == nil || len(taskIDs) == 0 {
		return nil, nil
	}
	departing := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		departing[taskID] = struct{}{}
	}
	seenGroups := make(map[string]struct{})
	transfers := make([]workspaceEnvironmentOwnershipTransfer, 0)
	for _, taskID := range taskIDs {
		group, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, taskID)
		if err != nil {
			cause := fmt.Errorf("lookup workspace group for task %s: %w", taskID, err)
			return nil, s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(compensationCtx, transfers, cause)
		}
		if group == nil || group.MaterializedEnvironmentID == "" {
			continue
		}
		if _, seen := seenGroups[group.ID]; seen {
			continue
		}
		seenGroups[group.ID] = struct{}{}
		transfer, err := s.transferWorkspaceGroupEnvironmentOwnership(ctx, group, departing)
		if err != nil {
			return nil, s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(compensationCtx, transfers, err)
		}
		if transfer != nil {
			transfers = append(transfers, *transfer)
		}
	}
	return transfers, nil
}

func (s *HandoffService) transferWorkspaceGroupEnvironmentOwnership(
	ctx context.Context,
	group *orchmodels.WorkspaceGroup,
	departing map[string]struct{},
) (*workspaceEnvironmentOwnershipTransfer, error) {
	for taskID := range departing {
		task, taskErr := s.tasks.GetTask(ctx, taskID)
		if taskErr != nil {
			return nil, fmt.Errorf("load task %s for workspace group %s: %w", taskID, group.ID, taskErr)
		}
		if task == nil {
			return nil, fmt.Errorf("task %s for workspace group %s not found", taskID, group.ID)
		}
		if group.WorkspaceID != "" && task.WorkspaceID != group.WorkspaceID {
			return nil, fmt.Errorf("workspace group %s belongs to workspace %s, task %s belongs to workspace %s",
				group.ID, group.WorkspaceID, taskID, task.WorkspaceID)
		}
	}
	mu := s.workspaceGroupLock.lockFor(group.ID)
	mu.Lock()
	defer mu.Unlock()
	environments, ok := s.tasks.(workspaceEnvironmentRepository)
	if !ok {
		return nil, fmt.Errorf("preserve workspace group %s: task environment repository unavailable", group.ID)
	}
	env, err := environments.GetTaskEnvironment(ctx, group.MaterializedEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("load materialized environment %s for workspace group %s: %w",
			group.MaterializedEnvironmentID, group.ID, err)
	}
	if env == nil {
		return nil, fmt.Errorf("materialized environment %s for workspace group %s not found",
			group.MaterializedEnvironmentID, group.ID)
	}
	if _, leaving := departing[env.TaskID]; !leaving {
		return nil, nil
	}
	members, err := s.wsGroups.ListActiveWorkspaceGroupMembers(ctx, group.ID)
	if err != nil {
		return nil, fmt.Errorf("list active workspace group members for %s: %w", group.ID, err)
	}
	candidates := survivingWorkspaceMemberIDs(group.OwnerTaskID, members, departing)
	if len(candidates) == 0 {
		// Every active member is departing, so last-member cleanup owns the
		// environment teardown and no ownership transfer is needed.
		return nil, nil
	}
	newOwner, err := availableWorkspaceEnvironmentOwner(ctx, environments, env.ID, candidates)
	if err != nil {
		return nil, fmt.Errorf("select workspace environment owner for group %s: %w", group.ID, err)
	}
	if newOwner == "" {
		return nil, fmt.Errorf("preserve workspace group %s: no surviving member can own environment %s", group.ID, env.ID)
	}
	if err := environments.TransferTaskEnvironmentOwnership(
		ctx, env.ID, env.TaskID, env.OwnershipGeneration, newOwner,
	); err != nil {
		return nil, fmt.Errorf("transfer workspace environment %s to task %s: %w", env.ID, newOwner, err)
	}
	s.logf().Info("transferred shared workspace environment before task lifecycle cleanup",
		zap.String("group_id", group.ID),
		zap.String("environment_id", env.ID),
		zap.String("old_owner_task_id", env.TaskID),
		zap.String("new_owner_task_id", newOwner))
	return &workspaceEnvironmentOwnershipTransfer{
		groupID:             group.ID,
		environmentID:       env.ID,
		oldOwnerTaskID:      env.TaskID,
		newOwnerTaskID:      newOwner,
		resultingGeneration: env.OwnershipGeneration + 1,
	}, nil
}

func (s *HandoffService) rollbackWorkspaceEnvironmentOwnershipAfterFailure(
	ctx context.Context,
	transfers []workspaceEnvironmentOwnershipTransfer,
	cause error,
) error {

	rollbackErr := s.rollbackWorkspaceEnvironmentOwnershipTransfers(ctx, transfers)
	if rollbackErr == nil {
		return cause
	}
	return errors.Join(cause, fmt.Errorf("rollback shared workspace environment ownership: %w", rollbackErr))
}
func (s *HandoffService) rollbackAutoArchiveCASLoss(
	ctx context.Context,
	transfers []workspaceEnvironmentOwnershipTransfer,
	candidate *models.Task,
	out *CascadeOutcome,
	cleanupErrors []error,
) error {
	if candidate == nil || out == nil || len(out.ArchivedTaskIDs) > 0 {
		return nil
	}
	return s.rollbackWorkspaceEnvironmentOwnershipAfterFailure(
		ctx, transfers, errors.Join(cleanupErrors...),
	)
}

func (s *HandoffService) rollbackWorkspaceEnvironmentOwnershipTransfers(
	ctx context.Context,
	transfers []workspaceEnvironmentOwnershipTransfer,
) error {
	if len(transfers) == 0 {
		return nil
	}
	environments, ok := s.tasks.(workspaceEnvironmentRepository)
	if !ok {
		return errors.New("task environment repository unavailable")
	}
	var errs []error
	for i := len(transfers) - 1; i >= 0; i-- {
		transfer := transfers[i]
		mu := s.workspaceGroupLock.lockFor(transfer.groupID)
		mu.Lock()
		env, err := environments.GetTaskEnvironment(ctx, transfer.environmentID)
		if err == nil {
			switch {
			case env == nil:
				err = errors.New("environment not found")
			case env.TaskID == transfer.oldOwnerTaskID:
				// A concurrent retry already restored this transfer.
			case env.TaskID != transfer.newOwnerTaskID:
				err = fmt.Errorf("owner changed from rollback target %s to %s", transfer.newOwnerTaskID, env.TaskID)
			case env.OwnershipGeneration != transfer.resultingGeneration:
				err = fmt.Errorf("ownership generation changed from rollback target %d to %d", transfer.resultingGeneration, env.OwnershipGeneration)
			default:
				err = environments.TransferTaskEnvironmentOwnership(
					ctx, transfer.environmentID, transfer.newOwnerTaskID, transfer.resultingGeneration, transfer.oldOwnerTaskID,
				)
			}
		}
		mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("environment %s: %w", transfer.environmentID, err))
			continue
		}
		s.logf().Info("restored shared workspace environment ownership after aborted task lifecycle mutation",
			zap.String("group_id", transfer.groupID),
			zap.String("environment_id", transfer.environmentID),
			zap.String("owner_task_id", transfer.oldOwnerTaskID))
	}
	return errors.Join(errs...)
}

func survivingWorkspaceMemberIDs(
	ownerTaskID string,
	members []orchmodels.WorkspaceGroupMember,
	departing map[string]struct{},
) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		if member.TaskID == "" {
			continue
		}
		if _, leaving := departing[member.TaskID]; leaving {
			continue
		}
		ids = append(ids, member.TaskID)
	}
	sort.Strings(ids)
	for i, taskID := range ids {
		if taskID == ownerTaskID {
			ids[0], ids[i] = ids[i], ids[0]
			break
		}
	}
	return ids
}

func availableWorkspaceEnvironmentOwner(
	ctx context.Context,
	environments workspaceEnvironmentRepository,
	environmentID string,
	candidates []string,
) (string, error) {
	for _, taskID := range candidates {
		owned, err := environments.GetTaskEnvironmentByTaskID(ctx, taskID)
		if err != nil {
			return "", err
		}
		if owned == nil || owned.ID == environmentID {
			return taskID, nil
		}
	}
	return "", nil
}

func (s *HandoffService) prepareCascadeResourceCleanup(
	ctx context.Context,
	deadline time.Time,
	taskIDs []string,
	cascadeID string,
	trigger models.TaskResourceCleanupTrigger,
) (map[string]string, error) {
	return s.prepareCascadeResourceCleanupWithCompensation(ctx, ctx, deadline, taskIDs, cascadeID, trigger, false)
}

func (s *HandoffService) prepareCascadeResourceCleanupWithCompensation(
	ctx, compensationCtx context.Context,
	deadline time.Time,
	taskIDs []string,
	cascadeID string,
	trigger models.TaskResourceCleanupTrigger,
	discardWorktreeChanges bool,
) (map[string]string, error) {
	coordinator, ok := s.resourceCleaner.(taskResourceCleanupCoordinator)
	if !ok {
		return nil, nil
	}
	operations := make(map[string]string, len(taskIDs))
	for _, taskID := range taskIDs {
		operationID := string(trigger) + ":" + cascadeID + ":" + taskID
		operations[taskID] = operationID
		deleteEnvironmentRow := trigger == models.TaskResourceCleanupTriggerCascadeDelete
		var err error
		if withOptions, supportsOptions := s.resourceCleaner.(taskResourceCleanupCoordinatorWithOptions); supportsOptions {
			err = withOptions.PrepareTaskResourceCleanupWithOptions(
				ctx, taskID, trigger, operationID, deleteEnvironmentRow, discardWorktreeChanges,
			)
		} else {
			err = coordinator.PrepareTaskResourceCleanup(ctx, taskID, trigger, operationID, deleteEnvironmentRow)
		}
		if err != nil {
			cancelCtx, cancel := archivecascade.ContinuationContextUntil(compensationCtx, deadline)
			cancelErr := s.cancelCascadeResourceCleanupRange(cancelCtx, taskIDs, operations)
			cancel()
			return nil, errors.Join(fmt.Errorf("prepare cleanup %s: %w", taskID, err), cancelErr)
		}
	}
	return operations, nil
}
func (s *HandoffService) cancelCascadeResourceCleanupRange(
	ctx context.Context,
	taskIDs []string,
	operations map[string]string,
) error {
	var errs []error
	for _, taskID := range taskIDs {
		if err := s.cancelCascadeResourceCleanup(ctx, operations[taskID]); err != nil {
			errs = append(errs, fmt.Errorf("cancel cleanup %s: %w", operations[taskID], err))
		}
	}
	return errors.Join(errs...)
}

func (s *HandoffService) startCascadeResourceCleanup(ctx context.Context, operationID string) error {
	coordinator, ok := s.resourceCleaner.(taskResourceCleanupCoordinator)
	if !ok || operationID == "" {
		return nil
	}
	if err := coordinator.StartPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		s.logf().Error("start cascade resource cleanup", zap.String("operation_id", operationID), zap.Error(err))
		return err
	}
	return nil
}

func (s *HandoffService) cancelCascadeResourceCleanup(ctx context.Context, operationID string) error {
	coordinator, ok := s.resourceCleaner.(taskResourceCleanupCoordinator)
	if !ok || operationID == "" {
		return nil
	}
	if err := coordinator.CancelPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		s.logf().Error("cancel cascade resource cleanup", zap.String("operation_id", operationID), zap.Error(err))
		return err
	}
	return nil
}

// cancelActiveRuns invokes the configured RunCanceller for every task in the
// cascade set. Session rows are finalized after the matching archive/delete
// mutation commits, because finalizing them here could leave a task active in
// the database when the caller's lifecycle mutation is cancelled.
func (s *HandoffService) cancelActiveRuns(ctx context.Context, taskIDs []string, reason string) {
	synchronous, hasSynchronousStop := s.runCanceller.(SynchronousRunCanceller)
	for _, id := range taskIDs {
		if s.runCanceller != nil {
			var err error
			if hasSynchronousStop {
				err = synchronous.CancelTaskExecutionSynchronously(ctx, id, reason, false)
			} else {
				err = s.runCanceller.CancelTaskExecution(ctx, id, reason, false)
			}
			if err != nil {
				s.logf().Warn("cascade: cancel task execution failed",
					zap.String("task_id", id), zap.Error(err))
			}
		}
	}
}

func (s *HandoffService) cancelArchiveRunsForCandidate(
	ctx context.Context,
	taskIDs []string,
	candidate *models.Task,
) {
	if candidate != nil {
		return
	}
	s.cancelActiveRuns(ctx, taskIDs, models.SessionArchiveTreeCancelReason)
}

func (s *HandoffService) finalizeActiveSessions(
	ctx context.Context,
	deadline time.Time,
	taskID, reason string,
) error {
	canceller, ok := s.sessions.(activeTaskSessionCanceller)
	if !ok {
		return nil
	}
	finalizeCtx, cancel := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancel()
	cancelled, err := canceller.CancelActiveTaskSessionsByTaskID(finalizeCtx, taskID, reason)
	if err != nil {
		wrapped := fmt.Errorf("finalize active task sessions %s: %w", taskID, err)
		s.logf().Warn("cascade: finalize active task sessions failed",
			zap.String("task_id", taskID), zap.Error(err))
		return wrapped
	}
	if len(cancelled) == 0 {
		return nil
	}
	if publisher, ok := s.eventPublisher.(taskSessionCancellationPublisher); ok {
		publisher.PublishTaskSessionsCancelled(finalizeCtx, taskID, cancelled, reason)
	}
	// Bulk writer: every returned row is released unconditionally rather than
	// branched on state, since RETURNING reports post-update CANCELLED for all
	// of them (AC-51e). Releasing an id that held no reservation is a defined
	// no-op.
	if s.sessionCeilingReleaser != nil {
		for _, session := range cancelled {
			if session == nil || session.ID == "" {
				continue
			}
			s.sessionCeilingReleaser.ReleaseCeilingReservation(session.ID)
		}
	}
	return nil
}

// UnarchiveTaskTree restores only members archived by the root's cascade.
// Manual archives and members of earlier cascades remain archived.
// The cascade ID scope prevents resurrection of unrelated archive rows.
func (s *HandoffService) UnarchiveTaskTree(ctx context.Context, rootID string) (*CascadeOutcome, error) {
	deadline := archivecascade.ArchiveDeadline(ctx)
	unarchiveCtx, cancelUnarchive := context.WithDeadline(ctx, deadline)
	defer cancelUnarchive()
	if err := s.authorizeTask(unarchiveCtx, rootID); err != nil {
		return nil, err
	}
	if rootID == "" {
		return nil, errors.New("rootID is required")
	}
	unlockArchiveCascade := s.archiveCascadeLock.lockFor(rootID)
	unlockArchiveCascade.Lock()
	defer unlockArchiveCascade.Unlock()
	root, err := s.tasks.GetTask(unarchiveCtx, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	operationCtx, cancelOperation := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancelOperation()
	if root.ArchivedByCascadeID == "" {
		return s.unarchiveManualRoot(operationCtx, root)
	}
	cascadeID := root.ArchivedByCascadeID
	out := &CascadeOutcome{CascadeID: cascadeID}
	// The descendant walk filters archived rows by this cascade ID, so
	// manually archived descendants remain untouched during restoration.
	all, err := s.collectArchivedTreeByCascade(operationCtx, rootID, cascadeID)
	if err != nil {
		return nil, err
	}
	// Fence every cleanup before restoring any task. A later cancellation
	// failure must not leave a partially restored tree that cannot be retried
	// from its archived root.
	cancelledCleanupOperations := make([]string, 0, len(all))
	restoreCancelledCleanup := func() error {
		restorer, ok := s.resourceCleaner.(taskResourceCleanupRestorer)
		if !ok {
			return nil
		}
		recoveryCtx, cancel := archivecascade.ContinuationContext(context.WithoutCancel(ctx))
		defer cancel()
		var errs []error
		for _, operationID := range cancelledCleanupOperations {
			if err := restorer.RestoreCancelledTaskResourceCleanup(recoveryCtx, operationID); err != nil {
				errs = append(errs, fmt.Errorf("restore archive cleanup %s: %w", operationID, err))
			}
		}
		return errors.Join(errs...)
	}
	var restorationErrors []error
	for _, id := range all {
		operationID := string(models.TaskResourceCleanupTriggerCascadeArchive) + ":" + cascadeID + ":" + id
		cancelledCleanupOperations = append(cancelledCleanupOperations, operationID)
		if err := s.cancelArchiveResourceCleanup(operationCtx, id, operationID); err != nil {
			return out, errors.Join(fmt.Errorf("cancel archive cleanup %s: %w", id, err), restoreCancelledCleanup())
		}
	}
	// Unarchive deep→shallow so a partial failure leaves the root archived
	// with its cascade ID, allowing a retry to discover remaining members.
	var mutationErr error
	for i := len(all) - 1; i >= 0; i-- {
		id := all[i]
		ok, err := s.tasks.UnarchiveTaskByCascade(operationCtx, id, cascadeID)
		if err != nil {
			mutationErr = fmt.Errorf("unarchive %s: %w", id, err)
			break
		}
		if ok {
			out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, id)
			// Publish per restored task. The WS handler keys off
			// archived_at=null to put the card back on the kanban.
			if err := s.publishUpdatedTask(operationCtx, id); err != nil {
				restorationErrors = append(restorationErrors,
					fmt.Errorf("publish restored task %s: %w", id, err))
			}
			// This task may itself be a parent whose inherit_parent
			// children were marked orphaned by this same archive; the
			// marker's "parent_archived" claim is no longer true.
			s.clearOrphanedInheritParentChildren(operationCtx, id)
		} else {
			out.SkippedTaskIDs = append(out.SkippedTaskIDs, id)
		}
	}
	if mutationErr != nil {
		restorationErrors = append(restorationErrors, mutationErr, restoreCancelledCleanup())
	}
	groupRestoreCtx := operationCtx
	var cancelGroupRestore context.CancelFunc
	if mutationErr != nil {
		groupRestoreCtx, cancelGroupRestore = archivecascade.ContinuationContext(context.WithoutCancel(ctx))
		defer cancelGroupRestore()
	}
	// Restore group memberships scoped to the same cascade. Track the
	// set of affected groups so we can also re-evaluate cleanup state
	// (cleanup_status=cleaned → active + restored / restorable).
	membershipRestoreFailed := false
	groupIDs := map[string]bool{}
	if s.wsGroups != nil {
		for _, id := range out.ArchivedTaskIDs {
			g, err := s.wsGroups.GetWorkspaceGroupForTask(groupRestoreCtx, id)
			if err != nil {
				membershipRestoreFailed = true
				restorationErrors = append(restorationErrors,
					fmt.Errorf("lookup workspace group for task %s: %w", id, err))
				continue
			}
			restored := false
			if g == nil {
				mu := s.workspaceGroupLock.lockFor(id)
				mu.Lock()
				err := s.wsGroups.RestoreWorkspaceGroupMemberByCascade(groupRestoreCtx, id, cascadeID)
				mu.Unlock()
				if err != nil {
					membershipRestoreFailed = true
					restorationErrors = append(restorationErrors,
						fmt.Errorf("restore membership for task %s: %w", id, err))
					continue
				}
				restored = true
				g, err = s.wsGroups.GetWorkspaceGroupForTask(groupRestoreCtx, id)
				if err != nil {
					membershipRestoreFailed = true
					restorationErrors = append(restorationErrors,
						fmt.Errorf("lookup restored workspace group for task %s: %w", id, err))
					continue
				}
				if g == nil {
					// A task without historical group membership is a
					// valid no-op for the restore repository.
					continue
				}
			}
			if !restored {
				mu := s.workspaceGroupLock.lockFor(g.ID)
				mu.Lock()
				err = s.wsGroups.RestoreWorkspaceGroupMemberByCascade(groupRestoreCtx, id, cascadeID)
				mu.Unlock()
				if err != nil {
					membershipRestoreFailed = true
					restorationErrors = append(restorationErrors,
						fmt.Errorf("restore membership for task %s: %w", id, err))
					continue
				}
			}
			groupIDs[g.ID] = true
		}
	}
	if membershipRestoreFailed {
		restorationErrors = append(restorationErrors, s.preserveCascadeAfterMembershipFailure(
			groupRestoreCtx, rootID, cascadeID, out.ArchivedTaskIDs, restoreCancelledCleanup,
		))
	}
	if len(groupIDs) > 0 {
		ids := make([]string, 0, len(groupIDs))
		for id := range groupIDs {
			ids = append(ids, id)
		}
		out.ReleasedGroupIDs = ids
		restorationErrors = append(restorationErrors, s.restoreCleanedGroups(groupRestoreCtx, ids))
	}
	return out, cascadePostCommitError(out, errors.Join(restorationErrors...))
}

func (s *HandoffService) preserveCascadeAfterMembershipFailure(
	ctx context.Context,
	rootID, cascadeID string,
	restoredTaskIDs []string,
	restoreCleanup func() error,
) error {
	archiver, ok := s.tasks.(interface {
		ArchiveTaskIfActive(context.Context, string, string) (bool, error)
	})
	if !ok {
		return errors.New("task repository cannot preserve cascade provenance after membership failure")
	}
	preserveCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		preserveCtx, cancel = archivecascade.ContinuationContext(context.WithoutCancel(ctx))
		defer cancel()
	}
	restoreIDs := append([]string{rootID}, restoredTaskIDs...)
	var errs []error
	for _, id := range restoreIDs {
		changed, err := archiver.ArchiveTaskIfActive(preserveCtx, id, cascadeID)
		if err != nil {
			errs = append(errs, fmt.Errorf("preserve cascade restore for task %s: %w", id, err))
			continue
		}
		if changed {
			if err := s.publishUpdatedTask(preserveCtx, id); err != nil {
				errs = append(errs, fmt.Errorf("publish preserved cascade task %s: %w", id, err))
			}
		}
	}
	errs = append(errs, restoreCleanup())
	return errors.Join(errs...)
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
	var restorationErrors []error
	if err := s.cancelArchiveResourceCleanup(ctx, root.ID, ""); err != nil {
		return out, fmt.Errorf("cancel archive cleanup %s: %w", root.ID, err)
	}
	ok, err := s.tasks.UnarchiveTask(ctx, root.ID)
	if err != nil {
		return out, fmt.Errorf("unarchive %s: %w", root.ID, err)
	}

	if !ok {
		out.SkippedTaskIDs = append(out.SkippedTaskIDs, root.ID)
		return out, nil
	}
	out.ArchivedTaskIDs = append(out.ArchivedTaskIDs, root.ID)
	if err := s.publishUpdatedTask(ctx, root.ID); err != nil {
		restorationErrors = append(restorationErrors, fmt.Errorf("publish unarchived task %s: %w", root.ID, err))
	}
	s.clearOrphanedInheritParentChildren(ctx, root.ID)
	// Legacy archives never released group memberships, but the group may
	// have been cleaned since (e.g. by a later cascade on another member).
	// Restore the group's materialized workspace if it was cleaned. Best
	// effort, like the cascade path: restoreCleanedGroups marks failures as
	// restore_status=restore_failed so they surface via the context API.
	if s.wsGroups != nil {
		g, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, root.ID)
		if err != nil {
			restorationErrors = append(restorationErrors,
				fmt.Errorf("lookup workspace group for unarchived task: %w", err))
		} else if g != nil {
			out.ReleasedGroupIDs = []string{g.ID}
			if err := s.restoreCleanedGroups(ctx, []string{g.ID}); err != nil {
				restorationErrors = append(restorationErrors, err)
			}
		}
	}
	return out, cascadePostCommitError(out, errors.Join(restorationErrors...))
}

func (s *HandoffService) cancelArchiveResourceCleanup(ctx context.Context, taskID, operationID string) error {
	if coordinator, ok := s.resourceCleaner.(taskResourceCleanupCoordinator); ok && operationID != "" {
		return coordinator.CancelPreparedTaskResourceCleanup(ctx, operationID)
	}
	canceller, ok := s.resourceCleaner.(archiveTaskResourceCleanupCanceller)
	if !ok {
		return nil
	}
	return canceller.CancelArchiveTaskResourceCleanup(ctx, taskID)
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
		// Delete must walk archived descendants too so every descendant
		// is removed rather than only currently active children.
		return s.collectTaskTreeIncludingArchived(ctx, rootID)
	}
	return []string{rootID}, nil
}

func (s *HandoffService) reparentNoCascadeChildren(ctx context.Context, rootID string) ([]*models.Task, error) {
	root, children, err := s.loadNoCascadeChildren(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if err := validateNoCascadeChildWorkspaces(root, children); err != nil {
		return nil, err
	}
	snapshots := snapshotNoCascadeChildren(children)
	restore := func(operationErr error) ([]*models.Task, error) {
		return nil, errors.Join(operationErr, s.restoreNoCascadeChildren(ctx, snapshots))
	}
	if err := s.normalizeNoCascadeChildren(ctx, children); err != nil {
		return restore(err)
	}
	if err := s.reparentChildrenInWorkspace(ctx, root, rootID); err != nil {
		return restore(err)
	}
	for _, child := range children {
		if err := s.publishUpdatedTask(ctx, child.ID); err != nil {
			return restore(err)
		}
	}
	return snapshots, nil
}

func snapshotNoCascadeChildren(children []*models.Task) []*models.Task {
	snapshots := make([]*models.Task, 0, len(children))
	for _, child := range children {
		if child == nil {
			continue
		}
		snapshot := *child
		snapshot.Metadata = cloneTaskMetadata(child.Metadata)
		if workspace, ok := snapshot.Metadata["workspace"].(map[string]interface{}); ok {
			snapshot.Metadata["workspace"] = cloneTaskMetadata(workspace)
		}
		snapshots = append(snapshots, &snapshot)
	}
	return snapshots
}

func (s *HandoffService) restoreNoCascadeChildren(ctx context.Context, snapshots []*models.Task) error {
	var errs []error
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		if err := s.restoreNoCascadeChild(ctx, snapshot); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *HandoffService) restoreNoCascadeChild(ctx context.Context, snapshot *models.Task) error {
	restorer, ok := s.tasks.(taskParentCompensationRestorer)
	if !ok {
		return fmt.Errorf("task repo cannot restore child %s conditionally", snapshot.ID)
	}
	current, err := s.tasks.GetTask(ctx, snapshot.ID)
	if err != nil {
		return fmt.Errorf("load child task %s for compensation: %w", snapshot.ID, err)
	}
	if current == nil {
		return fmt.Errorf("child task %s disappeared during compensation", snapshot.ID)
	}
	if current.ParentID != "" && current.ParentID != snapshot.ParentID {
		return fmt.Errorf("child task %s changed parent during compensation", snapshot.ID)
	}
	if err := restorer.RestoreTaskParentIfUnchanged(
		ctx, snapshot.ID, current.ParentID, snapshot.ParentID,
		taskWorkspaceMode(snapshot.Metadata),
	); err != nil {
		return fmt.Errorf("restore child task %s: %w", snapshot.ID, err)
	}
	if err := s.publishUpdatedTask(ctx, snapshot.ID); err != nil {
		return fmt.Errorf("publish restored child %s: %w", snapshot.ID, err)
	}
	return nil
}

func (s *HandoffService) loadNoCascadeChildren(
	ctx context.Context,
	rootID string,
) (*models.Task, []*models.Task, error) {
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return nil, nil, fmt.Errorf("load root task %s: %w", rootID, err)
	}
	if root == nil {
		return nil, nil, fmt.Errorf("task %s not found", rootID)
	}
	structural, ok := s.tasks.(structuralChildLister)
	if !ok {
		return nil, nil, errors.New("task repo lacks unfiltered structural child query")
	}
	children, err := structural.ListStructuralChildrenLimited(ctx, rootID, archiveCascadeMaxMembers+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list direct children of %s: %w", rootID, err)
	}
	if len(children) > archiveCascadeMaxMembers {
		return nil, nil, &archivecascade.SizeExceededError{
			Dimension: cascadeMembersDimension, Limit: archiveCascadeMaxMembers, Submitted: len(children),
		}
	}
	return root, children, nil
}

func validateNoCascadeChildWorkspaces(root *models.Task, children []*models.Task) error {
	for _, child := range children {
		if child.WorkspaceID != root.WorkspaceID {
			return &archivecascade.CrossWorkspaceDescendantError{
				TaskID: child.ID, WorkspaceID: child.WorkspaceID, ExpectedWorkspaceID: root.WorkspaceID,
			}
		}
	}
	return nil
}

func (s *HandoffService) normalizeNoCascadeChildren(
	ctx context.Context,
	children []*models.Task,
) error {
	for _, child := range children {
		if taskWorkspaceMode(child.Metadata) != workspaceModeInheritParent {
			continue
		}
		workspace, _ := child.Metadata["workspace"].(map[string]interface{})
		if workspace == nil {
			continue
		}
		guard := models.ObservedWorkspaceGuard(workspace)
		guard.RequireParentID = child.ParentID
		guard.RequireTaskNotArchived = true
		updatedWorkspace := cloneTaskMetadata(workspace)
		updatedWorkspace["mode"] = workspaceModeSharedGroup
		clearOrphanedWorkspaceMetadata(updatedWorkspace)
		updatedTask := *child
		updatedTask.Metadata = cloneTaskMetadata(child.Metadata)
		updatedTask.Metadata["workspace"] = updatedWorkspace
		landed, err := s.updateWorkspaceMetadata(ctx, &updatedTask, guard)
		if err != nil {
			return fmt.Errorf("normalize workspace mode for child %s before delete: %w", child.ID, err)
		}
		if !landed {
			return fmt.Errorf("%w: normalize workspace mode for child %s", errWorkspaceMetadataChangedConcurrently, child.ID)
		}
	}
	return nil
}

func (s *HandoffService) reparentChildrenInWorkspace(
	ctx context.Context,
	root *models.Task,
	rootID string,
) error {
	scoped, ok := s.tasks.(workspaceScopedTaskReparenter)
	if !ok {
		return errors.New("task repo lacks workspace-scoped reparent operation")
	}
	if err := scoped.ReparentDirectChildrenInWorkspace(ctx, rootID, "", root.WorkspaceID); err != nil {
		return fmt.Errorf("reparent direct children of %s: %w", rootID, err)
	}
	return nil
}

// collectTaskTree returns rootID followed by every NON-ARCHIVED
// descendant in BFS order. Used by ArchiveTaskTree where the cascade
// only ever needs to archive currently-active rows.
func (s *HandoffService) collectTaskTree(ctx context.Context, rootID string) ([]string, error) {
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	return s.collectTreeBFS(ctx, rootID, root.WorkspaceID, s.listCascadeChildren)
}

// collectTaskTreeIncludingArchived returns rootID followed by every
// descendant including already-archived rows. Delete cascades must remove
// every descendant from the task tree.
func (s *HandoffService) collectTaskTreeIncludingArchived(ctx context.Context, rootID string) ([]string, error) {
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	return s.collectTreeBFS(ctx, rootID, root.WorkspaceID, s.listCascadeChildrenIncludingArchived)
}

func (s *HandoffService) resolveArchiveCascade(ctx context.Context, rootID string, cascade bool) (string, []string, error) {
	cascadeID := uuid.New().String()
	if !cascade {
		return cascadeID, []string{rootID}, nil
	}
	all, err := s.collectTaskTree(ctx, rootID)
	if err != nil {
		return "", nil, err

	}
	retryID, retryAll, err := s.findArchiveRetryCascade(ctx, rootID)
	if err != nil {
		return "", nil, err
	}
	if retryID != "" {
		return retryID, retryAll, nil
	}
	return cascadeID, all, nil
}

const partialArchiveCascadeMetadataKey = "partial_archive_cascade_id"

type taskMetadataMutationRepository interface {
	SetTaskMetadataKey(context.Context, string, string, interface{}) error
	RemoveTaskMetadataKey(context.Context, string, string) (bool, error)
}

func (s *HandoffService) persistPartialArchiveCascadeMarker(ctx context.Context, taskID, cascadeID string) error {
	repo, ok := s.tasks.(taskMetadataMutationRepository)
	if !ok {
		return nil
	}
	if err := repo.SetTaskMetadataKey(ctx, taskID, partialArchiveCascadeMetadataKey, cascadeID); err != nil {
		return fmt.Errorf("persist partial archive cascade %s: %w", cascadeID, err)
	}
	return nil
}

func (s *HandoffService) clearPartialArchiveCascadeMarker(ctx context.Context, taskID string) error {
	repo, ok := s.tasks.(taskMetadataMutationRepository)
	if !ok {
		return nil
	}
	if _, err := repo.RemoveTaskMetadataKey(ctx, taskID, partialArchiveCascadeMetadataKey); err != nil {
		return fmt.Errorf("clear partial archive cascade marker: %w", err)
	}
	return nil
}

func (s *HandoffService) partialArchiveCascadeID(rootID string) string {
	s.partialArchiveMu.Lock()
	defer s.partialArchiveMu.Unlock()
	return s.partialArchiveIDs[rootID]
}

func (s *HandoffService) rememberPartialArchiveCascade(rootID, cascadeID string) {
	s.partialArchiveMu.Lock()
	defer s.partialArchiveMu.Unlock()
	if s.partialArchiveIDs == nil {
		s.partialArchiveIDs = make(map[string]string)
	}
	s.partialArchiveIDs[rootID] = cascadeID
}

func (s *HandoffService) forgetPartialArchiveCascade(rootID string) {
	s.partialArchiveMu.Lock()
	defer s.partialArchiveMu.Unlock()
	delete(s.partialArchiveIDs, rootID)
}

// findArchiveRetryCascade discovers a prior cascade after a partial archive
// mutation. Reusing its identity keeps already-archived descendants in the
// same resumable unarchive scope.
func (s *HandoffService) findArchiveRetryCascade(ctx context.Context, rootID string) (string, []string, error) {
	if cascadeID := s.partialArchiveCascadeID(rootID); cascadeID != "" {
		all, err := s.collectArchiveRetryTree(ctx, rootID, cascadeID)
		if err != nil {
			return "", nil, err
		}
		return cascadeID, all, nil
	}
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return "", nil, err
	}
	if root != nil && root.Metadata != nil {
		if cascadeID, ok := root.Metadata[partialArchiveCascadeMetadataKey].(string); ok && cascadeID != "" {
			all, err := s.collectArchiveRetryTree(ctx, rootID, cascadeID)
			if err != nil {
				return "", nil, err
			}
			return cascadeID, all, nil
		}
	}
	if root == nil || root.ArchivedByCascadeID == "" {
		// An active root provides no provenance for archived descendants.
		// They may belong to independent manual or auto archives.
		return "", nil, nil
	}
	all, err := s.collectArchiveRetryTree(ctx, rootID, root.ArchivedByCascadeID)
	if err != nil {
		return "", nil, err
	}
	return root.ArchivedByCascadeID, all, nil
}

type childLister func(ctx context.Context, parentID string) ([]*models.Task, error)

type boundedCascadeChildRepository interface {
	ListChildrenLimited(ctx context.Context, parentID string, limit int) ([]*models.Task, error)
	ListChildrenIncludingArchivedLimited(ctx context.Context, parentID string, limit int) ([]*models.Task, error)
	ListChildrenIncludingArchivedByCascadeLimited(ctx context.Context, parentID, cascadeID string, limit int) ([]*models.Task, error)
}

func (s *HandoffService) listCascadeChildren(ctx context.Context, parentID string) ([]*models.Task, error) {
	if repo, ok := s.tasks.(boundedCascadeChildRepository); ok {
		return repo.ListChildrenLimited(ctx, parentID, archiveCascadeMaxMembers+1)
	}
	return nil, errors.New("task repo lacks bounded cascade child query")
}

func (s *HandoffService) listCascadeChildrenIncludingArchived(ctx context.Context, parentID string) ([]*models.Task, error) {
	if repo, ok := s.tasks.(boundedCascadeChildRepository); ok {
		return repo.ListChildrenIncludingArchivedLimited(ctx, parentID, archiveCascadeMaxMembers+1)
	}
	return nil, errors.New("task repo lacks bounded cascade child query")
}

func (s *HandoffService) listCascadeChildrenIncludingArchivedByCascade(
	ctx context.Context,
	parentID, cascadeID string,
) ([]*models.Task, error) {
	if repo, ok := s.tasks.(boundedCascadeChildRepository); ok {
		return repo.ListChildrenIncludingArchivedByCascadeLimited(
			ctx, parentID, cascadeID, archiveCascadeMaxMembers+1,
		)
	}
	return nil, errors.New("task repo lacks bounded archive cascade child query")
}

const (
	archiveCascadeMaxMembers = 10000
	archiveCascadeMaxDepth   = 256
	cascadeMembersDimension  = "members"
	cascadeDepthDimension    = "depth"
	cascadeStructureCycle    = "cycle_or_duplicate"
)

type cascadeTreeNode struct {
	id    string
	depth int
}

func (s *HandoffService) collectTreeBFS(
	ctx context.Context,
	rootID, workspaceID string,
	list childLister,
) ([]string, error) {
	out := make([]string, 1, archiveCascadeMaxMembers+1)
	out[0] = rootID
	queue := make([]cascadeTreeNode, 1, archiveCascadeMaxMembers+1)
	queue[0] = cascadeTreeNode{id: rootID}
	visited := make(map[string]struct{}, archiveCascadeMaxMembers+1)
	visited[rootID] = struct{}{}
	for cursor := 0; cursor < len(queue); cursor++ {
		node := queue[cursor]
		children, err := list(ctx, node.id)
		if err != nil {
			return nil, err
		}
		if node.depth >= archiveCascadeMaxDepth && len(children) > 0 {
			return nil, &archivecascade.SizeExceededError{
				Dimension: cascadeDepthDimension,
				Limit:     archiveCascadeMaxDepth,
				Submitted: node.depth + 1,
			}
		}
		for _, c := range children {
			if c.WorkspaceID != workspaceID {
				return nil, &archivecascade.CrossWorkspaceDescendantError{
					TaskID:              c.ID,
					WorkspaceID:         c.WorkspaceID,
					ExpectedWorkspaceID: workspaceID,
				}
			}
			if _, exists := visited[c.ID]; exists {
				return nil, &archivecascade.StructureError{Kind: cascadeStructureCycle, TaskID: c.ID}
			}
			visited[c.ID] = struct{}{}
			out = append(out, c.ID)
			if len(out) > archiveCascadeMaxMembers {
				return nil, &archivecascade.SizeExceededError{
					Dimension: cascadeMembersDimension,
					Limit:     archiveCascadeMaxMembers,
					Submitted: len(out),
				}
			}
			queue = append(queue, cascadeTreeNode{id: c.ID, depth: node.depth + 1})
		}
	}
	return out, nil
}

// collectArchiveRetryTree returns active descendants plus descendants already
// stamped by cascadeID. Independently archived branches are boundaries and
// are neither included nor traversed.
func (s *HandoffService) collectArchiveRetryTree(ctx context.Context, rootID, cascadeID string) ([]string, error) {
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	out := make([]string, 1, archiveCascadeMaxMembers+1)
	out[0] = rootID
	queue := make([]cascadeTreeNode, 1, archiveCascadeMaxMembers+1)
	queue[0] = cascadeTreeNode{id: rootID}
	visited := map[string]struct{}{rootID: {}}
	for cursor := 0; cursor < len(queue); cursor++ {
		node := queue[cursor]
		children, err := s.listCascadeChildrenIncludingArchived(ctx, node.id)
		if err != nil {
			return nil, err
		}
		if len(children) > archiveCascadeMaxMembers {
			return nil, &archivecascade.SizeExceededError{
				Dimension: cascadeMembersDimension,
				Limit:     archiveCascadeMaxMembers,
				Submitted: len(children),
			}
		}
		for _, child := range children {
			if child.ArchivedAt != nil && child.ArchivedByCascadeID != cascadeID {
				continue
			}
			if child.WorkspaceID != root.WorkspaceID {
				return nil, &archivecascade.CrossWorkspaceDescendantError{
					TaskID: child.ID, WorkspaceID: child.WorkspaceID,
					ExpectedWorkspaceID: root.WorkspaceID,
				}
			}
			if _, exists := visited[child.ID]; exists {
				return nil, &archivecascade.StructureError{Kind: cascadeStructureCycle, TaskID: child.ID}
			}
			visited[child.ID] = struct{}{}
			if node.depth >= archiveCascadeMaxDepth {
				return nil, &archivecascade.SizeExceededError{
					Dimension: cascadeDepthDimension,
					Limit:     archiveCascadeMaxDepth,
					Submitted: node.depth + 1,
				}
			}
			out = append(out, child.ID)
			if len(out) > archiveCascadeMaxMembers {
				return nil, &archivecascade.SizeExceededError{
					Dimension: cascadeMembersDimension,
					Limit:     archiveCascadeMaxMembers,
					Submitted: len(out),
				}
			}
			queue = append(queue, cascadeTreeNode{id: child.ID, depth: node.depth + 1})
		}
	}
	return out, nil
}

// task tagged with the named cascade ID. Visits archived rows too so the
// full descendant set is reachable; we filter by cascade id rather than
// archived state so manual mid-cascade archives don't leak in.
func (s *HandoffService) collectArchivedTreeByCascade(ctx context.Context, rootID, cascadeID string) ([]string, error) {
	root, err := s.tasks.GetTask(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("task %s not found", rootID)
	}
	out := make([]string, 1, archiveCascadeMaxMembers+1)
	out[0] = rootID
	queue := make([]cascadeTreeNode, 1, archiveCascadeMaxMembers+1)
	queue[0] = cascadeTreeNode{id: rootID}
	visited := make(map[string]struct{}, archiveCascadeMaxMembers+1)
	visited[rootID] = struct{}{}
	for cursor := 0; cursor < len(queue); cursor++ {
		node := queue[cursor]
		t, err := s.tasks.GetTask(ctx, node.id)
		if err != nil {
			return nil, fmt.Errorf("get task %s during archive recovery: %w", node.id, err)
		}
		if t == nil {
			return nil, fmt.Errorf("task %s not found during archive recovery", node.id)
		}
		children, err := s.allChildrenIncludingArchived(ctx, node.id, cascadeID)
		if err != nil {
			return nil, err
		}
		for _, c := range children {
			if c.ArchivedByCascadeID != cascadeID {
				continue
			}
			if c.WorkspaceID != root.WorkspaceID {
				return nil, &archivecascade.CrossWorkspaceDescendantError{
					TaskID:              c.ID,
					WorkspaceID:         c.WorkspaceID,
					ExpectedWorkspaceID: root.WorkspaceID,
				}
			}
			if _, exists := visited[c.ID]; exists {
				return nil, &archivecascade.StructureError{Kind: cascadeStructureCycle, TaskID: c.ID}
			}
			visited[c.ID] = struct{}{}
			if node.depth >= archiveCascadeMaxDepth && len(children) > 0 {
				return nil, &archivecascade.SizeExceededError{
					Dimension: cascadeDepthDimension,
					Limit:     archiveCascadeMaxDepth,
					Submitted: node.depth + 1,
				}
			}
			out = append(out, c.ID)
			if len(out) > archiveCascadeMaxMembers {
				return nil, &archivecascade.SizeExceededError{
					Dimension: cascadeMembersDimension,
					Limit:     archiveCascadeMaxMembers,
					Submitted: len(out),
				}
			}
			queue = append(queue, cascadeTreeNode{id: c.ID, depth: node.depth + 1})
		}
	}
	return out, nil
}

// allChildrenIncludingArchived returns every child of parentID, archived
// or not, so the unarchive walk can find tasks tagged with the matching
// cascade ID even after they were archived by ArchiveTaskTree.
func (s *HandoffService) allChildrenIncludingArchived(ctx context.Context, parentID, cascadeID string) ([]*models.Task, error) {
	return s.listCascadeChildrenIncludingArchivedByCascade(ctx, parentID, cascadeID)
}

// releaseMembershipsForCascade releases group membership for each task
// in the input slice and returns the unique set of group IDs that had
// at least one member released. It attempts every task so cleanup
// evaluation can run for all groups changed before an individual failure.
func (s *HandoffService) releaseMembershipsForCascade(ctx context.Context, taskIDs []string, reason, cascadeID string) ([]string, error) {
	if s.wsGroups == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	var groups []string
	var errs []error
	for _, id := range taskIDs {
		g, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("lookup group for task %s: %w", id, err))
			continue
		}
		if g == nil {
			continue
		}
		task, taskErr := s.tasks.GetTask(ctx, id)
		if taskErr != nil {
			errs = append(errs, fmt.Errorf("load task %s for workspace group %s: %w", id, g.ID, taskErr))
			continue
		}
		if task == nil {
			errs = append(errs, fmt.Errorf("task %s for workspace group %s not found", id, g.ID))
			continue
		}
		if g.WorkspaceID != "" && task.WorkspaceID != g.WorkspaceID {
			errs = append(errs, fmt.Errorf("workspace group %s belongs to workspace %s, task %s belongs to workspace %s",
				g.ID, g.WorkspaceID, id, task.WorkspaceID))
			continue
		}
		if err := s.wsGroups.ReleaseWorkspaceGroupMember(ctx, g.ID, id, reason, cascadeID); err != nil {
			// A transient repository failure must not be acknowledged as a
			// successful archive while the membership remains active.
			if retryErr := s.wsGroups.ReleaseWorkspaceGroupMember(ctx, g.ID, id, reason, cascadeID); retryErr != nil {
				errs = append(errs, fmt.Errorf("release membership for task %s from group %s: %w", id, g.ID, errors.Join(err, retryErr)))
				continue
			}
		}
		if !seen[g.ID] {
			seen[g.ID] = true
			groups = append(groups, g.ID)
		}
	}
	return groups, errors.Join(errs...)
}

func (s *HandoffService) logf() *logger.Logger {
	if s.logger == nil {
		return logger.Default()
	}
	return s.logger
}
