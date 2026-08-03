package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/watchreset"
)

// eventSource is the `source` field on every bus.Event published by this
// package; consumed by event_handlers_gitlab.go-style subscribers.
const eventSource = "gitlab"

const (
	defaultWatchPollIntervalSec = 300
	minWatchPollIntervalSec     = 60
	watchDeleteTimeout          = 30 * time.Second
)

// --- MR Watch ---

// CreateMRWatch records a session→MR watch row. The poller (and topbar) use it
// to discover a freshly-pushed MR off the agent's source branch.
func (s *Service) CreateMRWatch(ctx context.Context, sessionID, taskID, repositoryID, projectPath string, iid int, branch string) (*MRWatch, error) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return nil, fmt.Errorf("gitlab store not configured")
	}
	w := &MRWatch{
		SessionID:    sessionID,
		TaskID:       taskID,
		RepositoryID: repositoryID,
		ProjectPath:  projectPath,
		MRIID:        iid,
		Branch:       branch,
	}
	if err := store.CreateMRWatch(ctx, w); err != nil {
		return nil, fmt.Errorf("create MR watch: %w", err)
	}
	s.publishWatchEvent(ctx, "mr_watch_created", w.ID, sessionID, taskID)
	return w, nil
}

// EnsureMRWatch is the idempotent get-or-create counterpart to CreateMRWatch,
// keyed by (sessionID, repositoryID, branch). Returns the existing row when
// one is already present, backfilling mr_iid in place if it was previously
// unknown (0) and a real iid is now available. Otherwise creates a new watch.
// Safe to call repeatedly for the same (session, repository, branch) triple —
// callers on the auto-link and on-demand-check paths call this on every push
// without checking for an existing row themselves.
func (s *Service) EnsureMRWatch(ctx context.Context, sessionID, taskID, repositoryID, projectPath string, iid int, branch string) (*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	existing, err := store.GetMRWatchBySessionRepoAndBranch(ctx, sessionID, repositoryID, branch)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		w, createErr := s.CreateMRWatch(ctx, sessionID, taskID, repositoryID, projectPath, iid, branch)
		if createErr == nil {
			return w, nil
		}
		// Lost the race with a concurrent EnsureMRWatch for the same triple
		// (push detection and the on-demand check can both run for one
		// session): the UNIQUE(session_id, repository_id, branch) constraint
		// rejected our INSERT because the row we were about to create now
		// exists. Re-read and return it — the caller's intent is satisfied.
		// Any other failure is still reported.
		if raced, getErr := store.GetMRWatchBySessionRepoAndBranch(ctx, sessionID, repositoryID, branch); getErr == nil && raced != nil {
			return raced, nil
		}
		return nil, createErr
	}
	// Replace whenever the caller supplies a different, known iid — not just
	// when the stored one was still unknown (<=0). A branch can be relinked
	// to a replacement MR (the prior one closed and a new one opened), and
	// without this the watch keeps polling the stale iid forever.
	if iid > 0 && existing.MRIID != iid {
		if err := store.UpdateMRWatchMRIID(ctx, existing.ID, iid); err != nil {
			return nil, fmt.Errorf("update MR watch iid: %w", err)
		}
		existing.MRIID = iid
	}
	return existing, nil
}

// GetMRWatchBySession fetches the legacy single-repo watch.
func (s *Service) GetMRWatchBySession(ctx context.Context, sessionID string) (*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetMRWatchBySession(ctx, sessionID)
}

// GetMRWatchBySessionAndRepo fetches a watch keyed by (session, repo).
func (s *Service) GetMRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetMRWatchBySessionAndRepo(ctx, sessionID, repositoryID)
}

// ListMRWatchesBySession lists every MR watch on a session.
func (s *Service) ListMRWatchesBySession(ctx context.Context, sessionID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListMRWatchesBySession(ctx, sessionID)
}

// ListMRWatchesByTask lists every MR watch on a task.
func (s *Service) ListMRWatchesByTask(ctx context.Context, taskID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListMRWatchesByTask(ctx, taskID)
}

// ListActiveMRWatches returns every MR watch (used by the poller).
func (s *Service) ListActiveMRWatches(ctx context.Context) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListActiveMRWatches(ctx)
}

// DeleteMRWatch removes a single MR watch.
func (s *Service) DeleteMRWatch(ctx context.Context, id string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.DeleteMRWatch(ctx, id)
}

func (s *Service) ListMRWatchesBySessionForWorkspace(ctx context.Context, workspaceID, sessionID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListMRWatchesBySessionForWorkspace(ctx, workspaceID, sessionID)
}

func (s *Service) ListMRWatchesByTaskForWorkspace(ctx context.Context, workspaceID, taskID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListMRWatchesByTaskForWorkspace(ctx, workspaceID, taskID)
}

func (s *Service) ListActiveMRWatchesForWorkspace(ctx context.Context, workspaceID string) ([]*MRWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListActiveMRWatchesForWorkspace(ctx, workspaceID)
}

func (s *Service) DeleteMRWatchForWorkspace(ctx context.Context, workspaceID, id string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	deleted, err := store.DeleteMRWatchForWorkspace(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrWatchNotFound
	}
	return nil
}

// errStoreUnavailable is returned by Service methods that require the
// SQLite store to be wired but the runtime didn't manage to create it
// (table migration failure on boot). Distinct error so callers can render
// a "GitLab unconfigured" UI instead of "500 internal error".
var errStoreUnavailable = fmt.Errorf("gitlab store not configured")

// ErrWatchNotFound is returned by Update/Trigger methods when the watch id
// doesn't exist. Sentinel so the HTTP controller can map it to 404 rather
// than 500.
var ErrWatchNotFound = fmt.Errorf("watch not found")

// ErrWatchOwnershipLost means an event belongs to a watch generation that was
// invalidated by reset or delete. Callers must not retain the created task.
var ErrWatchOwnershipLost = fmt.Errorf("watch ownership lost")

// CheckMRWatch polls a watch once: returns the latest MR status and whether
// the underlying MR moved into a state worth notifying about (new note,
// pipeline transition, approval transition).
func (s *Service) CheckMRWatch(ctx context.Context, watch *MRWatch) (*MRStatus, bool, error) {
	if watch == nil {
		return nil, false, fmt.Errorf("watch is nil")
	}
	client, err := s.clientForTask(ctx, watch.TaskID)
	if err != nil {
		return nil, false, err
	}
	store := s.requireStore()
	if store == nil {
		return nil, false, errStoreUnavailable
	}
	valid, err := s.watchMatchesTaskRepository(ctx, store, client, watch)
	if err != nil {
		return nil, false, err
	}
	if !valid {
		return nil, false, nil
	}
	// If we don't yet know an iid, try to find it from the branch.
	if watch.MRIID <= 0 {
		mr, err := client.FindMRByBranch(ctx, watch.ProjectPath, watch.Branch)
		if err != nil || mr == nil {
			now := time.Now().UTC()
			_ = store.UpdateMRWatchTimestamps(ctx, watch.ID, now, watch.LastNoteAt, watch.LastPipelineState, watch.LastApprovalState)
			return nil, false, err
		}
		if err := store.UpdateMRWatchMRIID(ctx, watch.ID, mr.IID); err != nil {
			return nil, false, fmt.Errorf("update MR watch iid: %w", err)
		}
		watch.MRIID = mr.IID
	}
	status, err := client.GetMRStatus(ctx, watch.ProjectPath, watch.MRIID)
	if err != nil {
		return nil, false, err
	}
	notable := watch.LastPipelineState != status.PipelineState ||
		watch.LastApprovalState != status.ApprovalState
	now := time.Now().UTC()
	if err := store.UpdateMRWatchTimestamps(ctx, watch.ID, now, watch.LastNoteAt, status.PipelineState, status.ApprovalState); err != nil {
		return nil, false, fmt.Errorf("record MR watch poll: %w", err)
	}
	if notable {
		s.publishMRFeedbackEvent(ctx, watch, status)
	}
	s.refreshTaskMRFromWatch(ctx, store, client, watch, status)
	return status, notable, nil
}

// watchMatchesTaskRepository confirms watch.RepositoryID/ProjectPath still
// identify the task's own GitLab repository before any GitLab request or
// gitlab_task_mrs write. refreshTaskMRFromWatch's identity check only
// validates the MR *returned by GitLab* against the watch's own (host,
// project, iid) — that alone still lets a stale or mis-scoped watch (one
// whose repository_id was never actually tied to this GitLab project) poll
// and self-consistently upsert data anyway, since GetMRStatus itself only
// cares about project path and iid, not repository_id.
//
// Returns (true, nil) when the watch is valid, or when the owning task is
// orphaned (workspaceID == "", e.g. the task was deleted) — refreshing an
// orphaned watch is already a benign no-op elsewhere in this file, so
// identity is not re-litigated here. Returns (false, nil) — not an error —
// only when ValidateTaskMRRepositoryIdentity returns the direct
// ErrTaskMRRepositoryMismatch sentinel, a durable identity mismatch; the
// watch is deleted rather than left to retry, since a mismatch is a
// permanent fact, not a transient failure, and would otherwise poll forever
// for nothing. Any other error — ErrTaskMRNotFound (the task/repository
// link row itself is missing, which can be a transient race rather than a
// proven mismatch) or a wrapped store/lookup failure — is propagated
// instead of deleting the watch, so the poller's existing per-watch error
// logging fires and retries on the next tick rather than destroying data on
// what may be a recoverable condition.
func (s *Service) watchMatchesTaskRepository(ctx context.Context, store *Store, client Client, watch *MRWatch) (bool, error) {
	workspaceID, err := store.WorkspaceIDForTask(ctx, watch.TaskID)
	if err != nil {
		return false, err
	}
	if workspaceID == "" {
		return true, nil
	}
	err = store.ValidateTaskMRRepositoryIdentity(
		ctx, workspaceID, watch.TaskID, watch.RepositoryID, client.Host(), watch.ProjectPath,
	)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, ErrTaskMRRepositoryMismatch) {
		return false, err
	}
	s.logger.Warn("dropping MR watch whose repository no longer matches the task",
		zap.String("watch_id", watch.ID), zap.String("task_id", watch.TaskID), zap.Error(err))
	if delErr := store.DeleteMRWatch(ctx, watch.ID); delErr != nil {
		s.logger.Warn("failed to delete mismatched MR watch",
			zap.String("watch_id", watch.ID), zap.Error(delErr))
	}
	return false, nil
}

// refreshTaskMRFromWatch keeps gitlab_task_mrs in sync with every poll, not
// just the ones the caller found "notable" — otherwise the topbar MR surface
// (state, pipeline_state, approval_state, merge_status) only updates on a
// pipeline/approval transition and misses everything else (title, merge
// state, review comments' side effects, etc). Best-effort: a failure here
// must not fail the poll cycle, since UpdateMRWatchTimestamps above already
// recorded a successful check.
func (s *Service) refreshTaskMRFromWatch(ctx context.Context, store *Store, client Client, watch *MRWatch, status *MRStatus) {
	// A response that doesn't actually match this watch's (host, project,
	// iid) — a client bug, or a mismatched mock in tests — must not create or
	// overwrite a task-MR association. Same check the manual-link and
	// auto-link paths already run before trusting a fetched MR.
	if err := validateReturnedMRIdentity(status, client.Host(), watch.ProjectPath, watch.MRIID); err != nil {
		s.logger.Warn("dropping MR watch refresh with mismatched identity",
			zap.String("watch_id", watch.ID), zap.String("task_id", watch.TaskID), zap.Error(err))
		return
	}
	workspaceID, err := store.WorkspaceIDForTask(ctx, watch.TaskID)
	if err != nil {
		// This is now the only path that refreshes gitlab_task_mrs on every
		// poll, so a transient DB error here must be observable — unlike an
		// empty workspaceID with no error (a benign orphaned-watch case,
		// e.g. the task was deleted), which is not logged.
		s.logger.Warn("failed to resolve workspace while refreshing task MR from watch",
			zap.String("watch_id", watch.ID), zap.String("task_id", watch.TaskID), zap.Error(err))
		return
	}
	if workspaceID == "" {
		return
	}
	association := taskMRFromStatus(watch.TaskID, watch.RepositoryID, client.Host(), watch.ProjectPath, status)
	// Loaded before the upsert so it reflects the row's state prior to this
	// poll; a lookup failure isn't fatal to the refresh itself, but without a
	// "previous" to compare against we can't tell whether anything visible
	// changed, so this poll's update is published rather than silently
	// dropped by a comparison against a stale nil.
	previous, prevErr := store.GetTaskMR(ctx, watch.TaskID, watch.RepositoryID, watch.ProjectPath, watch.MRIID)
	if prevErr != nil {
		s.logger.Warn("failed to load existing task MR before refresh",
			zap.String("watch_id", watch.ID), zap.String("task_id", watch.TaskID), zap.Error(prevErr))
	}
	if err := store.UpsertTaskMR(ctx, association); err != nil {
		s.logger.Warn("failed to refresh task MR from watch",
			zap.String("watch_id", watch.ID), zap.String("task_id", watch.TaskID), zap.Error(err))
		return
	}
	if prevErr != nil || taskMRVisibleFieldsChanged(previous, association) {
		s.publishTaskMRUpdated(ctx, workspaceID, association)
	}
}

// taskMRVisibleFieldsChanged reports whether any user-visible field differs
// between a task-MR association's previous and current state. Excludes
// bookkeeping-only columns (id, timestamps) so a poll that re-fetches
// identical MR data doesn't broadcast a gitlab.task_mr.updated event on every
// tick — refreshTaskMRFromWatch runs on every poll, not just "notable"
// pipeline/approval transitions, so without this filter the event fires far
// more often than anything actually changed. previous == nil (first-ever
// refresh for this association) always counts as changed.
func taskMRVisibleFieldsChanged(previous, current *TaskMR) bool {
	if previous == nil {
		return true
	}
	return taskMRIdentityFieldsChanged(previous, current) || taskMRStatusFieldsChanged(previous, current)
}

// taskMRIdentityFieldsChanged compares the descriptive fields a poll can
// revise (title, branches, author) — split out from taskMRStatusFieldsChanged
// purely to keep taskMRVisibleFieldsChanged under the cyclomatic-complexity
// budget; the two halves together are the full comparison.
func taskMRIdentityFieldsChanged(previous, current *TaskMR) bool {
	return previous.MRURL != current.MRURL ||
		previous.MRTitle != current.MRTitle ||
		previous.HeadBranch != current.HeadBranch ||
		previous.BaseBranch != current.BaseBranch ||
		previous.AuthorUsername != current.AuthorUsername
}

// taskMRStatusFieldsChanged compares the review/CI status fields a poll can
// revise (state, approvals, pipeline, merge).
func taskMRStatusFieldsChanged(previous, current *TaskMR) bool {
	return previous.State != current.State ||
		previous.ApprovalState != current.ApprovalState ||
		previous.PipelineState != current.PipelineState ||
		previous.MergeStatus != current.MergeStatus ||
		previous.Draft != current.Draft ||
		previous.ApprovalCount != current.ApprovalCount ||
		previous.RequiredApprovals != current.RequiredApprovals ||
		previous.PipelineJobsTotal != current.PipelineJobsTotal ||
		previous.PipelineJobsPass != current.PipelineJobsPass ||
		!timePtrEqual(previous.MergedAt, current.MergedAt) ||
		!timePtrEqual(previous.ClosedAt, current.ClosedAt)
}

// timePtrEqual compares two *time.Time for equal instants, nil-safe.
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// --- Review Watch ---

// CreateReviewWatch persists a new review watch.
func (s *Service) CreateReviewWatch(ctx context.Context, req *CreateReviewWatchRequest) (*ReviewWatch, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if !IsValidCleanupPolicy(req.CleanupPolicy) {
		return nil, fmt.Errorf("invalid cleanup_policy: %q", req.CleanupPolicy)
	}
	if err := s.validateWatchDependencies(ctx, req.WorkspaceID, req.WorkflowID, req.WorkflowStepID, req.AgentProfileID, req.ExecutorProfileID); err != nil {
		return nil, err
	}
	repositoryID, baseBranch, err := s.resolveWatchRepository(ctx, req.WorkspaceID, req.RepositoryID, req.BaseBranch)
	if err != nil {
		return nil, err
	}
	interval := req.PollIntervalSeconds
	if interval <= 0 {
		interval = defaultWatchPollIntervalSec
	}
	if interval < minWatchPollIntervalSec {
		interval = minWatchPollIntervalSec
	}
	scope := req.ReviewScope
	if scope == "" {
		scope = ReviewScopeUserAndTeams
	}
	rw := &ReviewWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		Projects:            normalizeProjectFilters(req.Projects),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		RepositoryID:        repositoryID,
		BaseBranch:          baseBranch,
		ReviewScope:         scope,
		CustomQuery:         req.CustomQuery,
		Enabled:             true,
		PollIntervalSeconds: interval,
		CleanupPolicy:       NormalizeCleanupPolicy(req.CleanupPolicy),
		MaxInflightTasks:    req.MaxInflightTasks,
	}
	if err := validateWatchMaxInflight(rw.MaxInflightTasks); err != nil {
		return nil, err
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	if err := store.CreateReviewWatch(ctx, rw); err != nil {
		return nil, fmt.Errorf("create review watch: %w", err)
	}
	go s.initialReviewCheck(context.Background(), rw)
	return rw, nil
}

func (s *Service) initialReviewCheck(ctx context.Context, watch *ReviewWatch) {
	mrs, err := s.CheckReviewWatch(ctx, watch)
	if err != nil {
		s.logger.Debug("initial gitlab review check failed",
			zap.String("watch_id", watch.ID), zap.Error(err))
		return
	}
	for _, mr := range mrs {
		s.publishNewReviewMREvent(ctx, watch, mr)
	}
}

// GetReviewWatch returns a review watch by id.
func (s *Service) GetReviewWatch(ctx context.Context, id string) (*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetReviewWatch(ctx, id)
}

func (s *Service) GetReviewWatchIncludingDeleting(ctx context.Context, id string) (*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetReviewWatchIncludingDeleting(ctx, id)
}

// ListReviewWatches lists review watches in a workspace.
func (s *Service) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListReviewWatches(ctx, workspaceID)
}

// ListAllReviewWatches returns every review watch.
func (s *Service) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListAllReviewWatches(ctx)
}

// UpdateReviewWatch applies a partial update to a review watch.
func (s *Service) UpdateReviewWatch(ctx context.Context, id string, req *UpdateReviewWatchRequest) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	rw, err := store.GetReviewWatch(ctx, id)
	if err != nil {
		return err
	}
	if rw == nil {
		return fmt.Errorf("%w: review watch %s", ErrWatchNotFound, id)
	}
	applyReviewWatchPatch(rw, req)
	if err := s.validateWatchDependencies(ctx, rw.WorkspaceID, rw.WorkflowID, rw.WorkflowStepID, rw.AgentProfileID, rw.ExecutorProfileID); err != nil {
		return err
	}
	if req.RepositoryID != nil || req.BaseBranch != nil {
		repositoryID, baseBranch, err := s.resolveWatchRepository(ctx, rw.WorkspaceID, rw.RepositoryID, rw.BaseBranch)
		if err != nil {
			return err
		}
		rw.RepositoryID, rw.BaseBranch = repositoryID, baseBranch
	}
	if req.CleanupPolicy != nil && !IsValidCleanupPolicy(*req.CleanupPolicy) {
		return fmt.Errorf("invalid cleanup_policy: %q", *req.CleanupPolicy)
	}
	if err := validateWatchMaxInflight(rw.MaxInflightTasks); err != nil {
		return err
	}
	return store.UpdateReviewWatch(ctx, rw)
}

// the ReviewWatch shape (with ReviewScope instead of Labels). The two are
// kept as separate functions so each domain's fields are explicit; merging
// them via generics would obscure the per-type validation that lives in
// CreateXxxWatch.
//
//nolint:dupl // structurally similar to applyIssueWatchPatch but operates on
func applyReviewWatchPatch(rw *ReviewWatch, req *UpdateReviewWatchRequest) {
	if req.WorkflowID != nil {
		rw.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		rw.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Projects != nil {
		rw.Projects = normalizeProjectFilters(*req.Projects)
	}
	if req.AgentProfileID != nil {
		rw.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		rw.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		rw.Prompt = *req.Prompt
	}
	if req.RepositoryID != nil {
		rw.RepositoryID = *req.RepositoryID
		if rw.RepositoryID == "" {
			rw.BaseBranch = ""
		}
	}
	if req.BaseBranch != nil && rw.RepositoryID != "" {
		rw.BaseBranch = *req.BaseBranch
	}
	if req.ReviewScope != nil {
		rw.ReviewScope = *req.ReviewScope
	}
	if req.CustomQuery != nil {
		rw.CustomQuery = *req.CustomQuery
	}
	if req.Enabled != nil {
		rw.Enabled = *req.Enabled
		if rw.Enabled {
			rw.LastError = ""
			rw.LastErrorAt = nil
		}
	}
	if req.PollIntervalSeconds != nil {
		rw.PollIntervalSeconds = clampPollInterval(*req.PollIntervalSeconds)
	}
	if req.CleanupPolicy != nil {
		rw.CleanupPolicy = NormalizeCleanupPolicy(*req.CleanupPolicy)
	}
	if req.MaxInflightTasks != nil {
		if *req.MaxInflightTasks == 0 {
			rw.MaxInflightTasks = nil
		} else {
			rw.MaxInflightTasks = req.MaxInflightTasks
		}
	}
}

// clampPollInterval enforces the same bounds the create path applies (0 → default,
// below minimum → minimum). Used by both review-watch and issue-watch update paths.
func clampPollInterval(seconds int) int {
	if seconds <= 0 {
		return defaultWatchPollIntervalSec
	}
	if seconds < minWatchPollIntervalSec {
		return minWatchPollIntervalSec
	}
	return seconds
}

func validateWatchMaxInflight(max *int) error {
	if max != nil && *max <= 0 {
		return fmt.Errorf("max_inflight_tasks must be positive")
	}
	return nil
}

// DeleteReviewWatch removes a review watch and best-effort reaps any tasks
// it owned (tasks survive when the dedup row dies, so pre-sweep first).
func (s *Service) DeleteReviewWatch(ctx context.Context, id string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), watchDeleteTimeout)
	defer cancel()
	s.mu.RLock()
	deleter := s.taskDeleter
	cascadeDeleter := s.cascadeTaskDeleter
	s.mu.RUnlock()
	invalidation, err := store.BeginReviewWatchDelete(deleteCtx, id)
	if err != nil {
		return err
	}
	if invalidation.Missing {
		return nil
	}
	s.cleanupInvalidatedWatchTasks(deleteCtx, id, invalidation.TaskIDs, cascadeDeleter, deleter)
	return store.DeleteReviewWatch(deleteCtx, id)
}

func (s *Service) cleanupInvalidatedWatchTasks(ctx context.Context, watchID string, ids []string, cascadeDeleter watchreset.TaskDeleter, deleter TaskDeleter) {
	if cascadeDeleter != nil {
		s.deleteWatchTaskTrees(ctx, watchID, ids, cascadeDeleter)
		return
	}
	if deleter == nil {
		return
	}
	for _, taskID := range ids {
		if taskID == "" {
			continue
		}
		if err := deleter.DeleteTask(ctx, taskID); err != nil {
			s.logger.Warn("failed to delete task during watch cleanup",
				zap.String("watch_id", watchID), zap.String("task_id", taskID), zap.Error(err))
		}
	}
}

// CheckReviewWatch polls a single review watch and returns newly observed MRs
// not yet tracked. Dedup happens against gitlab_review_mr_tasks.
//
// different domain type (MR vs Issue); extracting a generic helper would
// require type parameters across the dedup-check + store-poll API which
// gives up more clarity than it saves.
//
//nolint:dupl // structurally similar to CheckIssueWatch but operates on a
func (s *Service) CheckReviewWatch(ctx context.Context, watch *ReviewWatch) ([]*MR, error) {
	if watch == nil {
		return nil, fmt.Errorf("watch is nil")
	}
	if !watch.Enabled {
		return nil, nil
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	mrs, err := s.fetchReviewMRs(ctx, watch)
	if err != nil {
		return nil, err
	}
	out := make([]*MR, 0, len(mrs))
	for _, mr := range mrs {
		exists, err := store.HasReviewMRTask(ctx, watch.ID, mr.ProjectPath, mr.IID)
		if err != nil {
			s.logger.Error("check review MR dedup", zap.Error(err))
			continue
		}
		if !exists {
			out = append(out, mr)
		}
	}
	now := time.Now().UTC()
	if err := store.RecordReviewWatchPoll(ctx, watch.ID, now); err != nil {
		s.logger.Warn("record review watch poll", zap.String("watch_id", watch.ID), zap.Error(err))
	}
	return out, nil
}

func (s *Service) fetchReviewMRs(ctx context.Context, watch *ReviewWatch) ([]*MR, error) {
	client, err := s.ClientForWorkspace(ctx, watch.WorkspaceID)
	if err != nil {
		return nil, err
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gitlab username: %w", err)
	}
	if username == "" {
		return nil, fmt.Errorf("no authenticated gitlab user")
	}
	// SearchMRs's buildMRSearchQuery returns customQuery verbatim when
	// non-empty (ignoring filter), so only build the default filter when
	// the watch has no customQuery to pass through.
	filter := ""
	if watch.CustomQuery == "" {
		filter = "reviewer_username=" + url.QueryEscape(username)
	}
	mrs, err := client.SearchMRs(ctx, filter, watch.CustomQuery)
	if err != nil {
		return nil, fmt.Errorf("search MRs: %w", err)
	}
	if len(watch.Projects) == 0 {
		return mrs, nil
	}
	allowed := make(map[string]bool, len(watch.Projects))
	for _, p := range watch.Projects {
		allowed[strings.ToLower(strings.TrimSpace(p.Path))] = true
	}
	out := mrs[:0]
	for _, mr := range mrs {
		if allowed[strings.ToLower(mr.ProjectPath)] {
			out = append(out, mr)
		}
	}
	return out, nil
}

// TriggerReviewWatch runs the watch once on demand. Returns matching MRs.
func (s *Service) TriggerReviewWatch(ctx context.Context, id string) ([]*MR, error) {
	rw, err := s.GetReviewWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if rw == nil {
		return nil, fmt.Errorf("%w: review watch %s", ErrWatchNotFound, id)
	}
	mrs, err := s.CheckReviewWatch(ctx, rw)
	if err != nil {
		return nil, err
	}
	for _, mr := range mrs {
		s.publishNewReviewMREvent(ctx, rw, mr)
	}
	return mrs, nil
}

// TriggerReviewWatchAll runs every enabled watch and aggregates new MRs.
func (s *Service) TriggerReviewWatchAll(ctx context.Context) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	watches, err := store.ListEnabledReviewWatches(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, rw := range watches {
		found, err := s.CheckReviewWatch(ctx, rw)
		if err != nil {
			s.logger.Warn("trigger review watch all", zap.String("watch_id", rw.ID), zap.Error(err))
			continue
		}
		for _, mr := range found {
			s.publishNewReviewMREvent(ctx, rw, mr)
		}
		total += len(found)
	}
	return total, nil
}

// TriggerReviewWatchAllForWorkspace runs only watches owned by workspaceID.
func (s *Service) TriggerReviewWatchAllForWorkspace(ctx context.Context, workspaceID string) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	watches, err := store.ListEnabledReviewWatchesForWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, watch := range watches {
		found, checkErr := s.CheckReviewWatch(ctx, watch)
		if checkErr != nil {
			s.logger.Warn("trigger workspace review watches", zap.String("watch_id", watch.ID), zap.Error(checkErr))
			continue
		}
		for _, mr := range found {
			s.publishNewReviewMREvent(ctx, watch, mr)
		}
		total += len(found)
	}
	return total, nil
}

// --- Helpers ---

func (s *Service) requireStore() *Store {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	return store
}

// appendLabelsToQuery merges a label list into an existing customQuery string.
// If the query already has a `labels` key the caller's value is kept (we
// don't want to silently double-up). url.ParseQuery is used for an exact
// key match — strings.Contains("labels=") would false-positive on keys
// like `mylabels=` and silently drop the watch's labels.
func appendLabelsToQuery(customQuery string, labels []string) string {
	if parsed, err := url.ParseQuery(customQuery); err == nil && parsed.Has("labels") {
		return customQuery
	}
	encoded := url.QueryEscape(strings.Join(labels, ","))
	if customQuery == "" {
		return "labels=" + encoded
	}
	return customQuery + "&labels=" + encoded
}

func normalizeProjectFilters(in []ProjectFilter) []ProjectFilter {
	if in == nil {
		return []ProjectFilter{}
	}
	out := make([]ProjectFilter, 0, len(in))
	for _, p := range in {
		p.Path = strings.TrimSpace(p.Path)
		if p.Path == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
