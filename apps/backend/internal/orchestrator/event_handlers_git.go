package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// gitSnapshotPersistInterval is the minimum time between persisted live git
// status snapshots for a single session when the underlying status hasn't
// changed. Writes still happen immediately when the status hash changes.
const gitSnapshotPersistInterval = 30 * time.Second

// gitSnapshotCacheMaxEntries bounds the in-memory throttle map so a long-lived
// backend with many sessions can't grow it without limit. When the cache is
// full and a new session arrives, the oldest entry by lastWrite is evicted.
const gitSnapshotCacheMaxEntries = 4096

type gitSnapshotCacheEntry struct {
	hash      string
	lastWrite time.Time
}

// gitSnapshotCache throttles per-session writes to the live git snapshot cache
// table. It is process-local — first event after a restart will rewrite the
// row, which is fine because UpsertLatestLiveGitSnapshot is idempotent.
type gitSnapshotCache struct {
	mu      sync.Mutex
	byID    map[string]gitSnapshotCacheEntry
	maxSize int
}

func newGitSnapshotCache() *gitSnapshotCache {
	return &gitSnapshotCache{
		byID:    make(map[string]gitSnapshotCacheEntry),
		maxSize: gitSnapshotCacheMaxEntries,
	}
}

// shouldWrite returns true if the new hash should be persisted now. Writes
// happen on hash change, or when the previous write is older than
// gitSnapshotPersistInterval (defensive: makes the cache eventually consistent
// even if hashing misses something).
func (c *gitSnapshotCache) shouldWrite(sessionID, hash string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, ok := c.byID[sessionID]
	if ok && prev.hash == hash && now.Sub(prev.lastWrite) < gitSnapshotPersistInterval {
		return false
	}
	if !ok && c.maxSize > 0 && len(c.byID) >= c.maxSize {
		c.evictOldestLocked()
	}
	c.byID[sessionID] = gitSnapshotCacheEntry{hash: hash, lastWrite: now}
	return true
}

// evictOldestLocked drops the entry with the oldest lastWrite. Caller must
// hold c.mu. O(n) over the cache; only invoked when the cache is full, which
// is rare in practice.
func (c *gitSnapshotCache) evictOldestLocked() {
	var oldestID string
	var oldestAt time.Time
	for id, entry := range c.byID {
		if oldestID == "" || entry.lastWrite.Before(oldestAt) {
			oldestID = id
			oldestAt = entry.lastWrite
		}
	}
	if oldestID != "" {
		delete(c.byID, oldestID)
	}
}

// forget removes a session's cached entry. Called when a session is deleted
// so the cache doesn't retain stale state for sessions that will never
// receive another git event.
func (c *gitSnapshotCache) forget(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byID, sessionID)
}

func gitStatusHash(s *lifecycle.GitStatusData) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%s|%s|%d|%d|%d|%d",
		s.Branch, s.RemoteBranch, s.HeadCommit, s.BaseCommit,
		s.Ahead, s.Behind, s.BranchAdditions, s.BranchDeletions)
	return hex.EncodeToString(h.Sum(nil))
}

// handleGitEvent handles unified git events and dispatches to appropriate handler
func (s *Service) handleGitEvent(ctx context.Context, data watcher.GitEventData) {
	s.logger.Debug("handling git event",
		zap.String("type", string(data.Type)),
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))

	if data.SessionID == "" {
		s.logger.Debug("missing session_id for git event",
			zap.String("task_id", data.TaskID),
			zap.String("type", string(data.Type)))
		return
	}

	switch data.Type {
	case lifecycle.GitEventTypeStatusUpdate:
		s.handleGitStatusUpdate(ctx, data)
	case lifecycle.GitEventTypeCommitCreated:
		s.handleGitCommitCreated(ctx, data)
	case lifecycle.GitEventTypeCommitsReset:
		s.handleGitCommitsReset(ctx, data)
	case lifecycle.GitEventTypeBranchSwitched:
		s.handleBranchSwitched(ctx, data)
	case lifecycle.GitEventTypeSnapshotCreated:
		// Snapshot events are published from orchestrator, no need to handle here
		s.logger.Debug("received snapshot_created event, no action needed",
			zap.String("session_id", data.SessionID))
	default:
		s.logger.Warn("unknown git event type",
			zap.String("type", string(data.Type)),
			zap.String("session_id", data.SessionID))
	}
}

// handleGitStatusUpdate handles git status updates by forwarding them to the frontend.
// In the live model, git status is not persisted to DB - the frontend queries agentctl directly.
func (s *Service) handleGitStatusUpdate(ctx context.Context, data watcher.GitEventData) {
	if data.Status == nil {
		s.logger.Debug("missing status data for git status update",
			zap.String("task_id", data.TaskID))
		return
	}

	// Forward status_update event to WebSocket subject for frontend
	// The frontend uses this for real-time updates during active sessions
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitWSEvent, "orchestrator", &data)
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}

	// Update PR watch branch if the user changed branches (e.g. renamed)
	s.syncPRWatchBranch(ctx, data.SessionID, data.Status.Branch)

	// Push detection: when ahead goes from >0 to 0, a push happened
	s.trackPushAndAssociatePR(ctx, data)

	// Persist a throttled cache of the live status so the sidebar diff badge
	// works for tasks whose executor isn't currently running (and across
	// backend restarts). Best-effort: errors are logged and swallowed.
	s.persistGitStatusSnapshot(ctx, data)
}

// persistGitStatusSnapshot writes a single cached "live monitor" snapshot per
// session, throttled by gitSnapshotCache. The cached row is read by
// appendDBSnapshotGitStatus when no live execution is available.
func (s *Service) persistGitStatusSnapshot(ctx context.Context, data watcher.GitEventData) {
	if s.repo == nil || data.SessionID == "" || data.Status == nil {
		return
	}
	if s.gitSnapshotCache == nil {
		return
	}
	hash := gitStatusHash(data.Status)
	if !s.gitSnapshotCache.shouldWrite(data.SessionID, hash, time.Now()) {
		return
	}

	st := data.Status
	snapshot := &models.GitSnapshot{
		SessionID:    data.SessionID,
		Branch:       st.Branch,
		RemoteBranch: st.RemoteBranch,
		HeadCommit:   st.HeadCommit,
		BaseCommit:   st.BaseCommit,
		Ahead:        st.Ahead,
		Behind:       st.Behind,
		Files:        nil, // intentional: badge only needs totals
		Metadata: map[string]interface{}{
			"branch_additions": st.BranchAdditions,
			"branch_deletions": st.BranchDeletions,
			"modified":         st.Modified,
			"added":            st.Added,
			"deleted":          st.Deleted,
			"untracked":        st.Untracked,
			"renamed":          st.Renamed,
			"timestamp":        data.Timestamp,
		},
	}
	if err := s.repo.UpsertLatestLiveGitSnapshot(ctx, snapshot); err != nil {
		s.logger.Debug("failed to persist live git snapshot",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
	}
}

// trackPushAndAssociatePR detects git pushes by tracking the "ahead" count.
// Two cases trigger detection:
//
//  1. Transition: ahead went from >0 to 0 with a remote branch set — the
//     normal in-session push, observed across two status events.
//  2. First-observation sync: the very first status event for this
//     (session, repo) already shows ahead=0 with a remote branch. This means
//     a push happened before agentctl's poller saw the ahead>0 phase (the
//     poll cadence missed it, or the session resumed after a restart). For a
//     fresh task branch, RemoteBranch is only populated after `git push -u`,
//     so seeing it pre-synced is itself a push signal.
//
// Without (2), multi-repo tasks routinely lose PR associations for any repo
// whose first poll happens to land after the push completes — the
// transition never gets observed and the watch never gets created.
//
// Multi-repo: keyed per (session, repository_name) so each repo's
// transitions are tracked independently. Without this, agentctl's per-repo
// status events race-overwrote each other's ahead counts and only one
// repo's push got detected.
func (s *Service) trackPushAndAssociatePR(ctx context.Context, data watcher.GitEventData) {
	key := pushTrackerKey(data.SessionID, data.Status.RepositoryName)
	prevAheadVal, loaded := s.pushTracker.Swap(key, data.Status.Ahead)
	prevAhead, _ := prevAheadVal.(int)
	if !shouldFirePushDetection(loaded, prevAhead, data.Status) {
		return
	}
	s.logger.Info("git push detected, starting PR association",
		zap.String("session_id", data.SessionID),
		zap.String("task_id", data.TaskID),
		zap.String("repository_name", data.Status.RepositoryName),
		zap.String("branch", data.Status.Branch),
		zap.Bool("first_observation", !loaded))
	go s.detectPushAndAssociatePR(
		context.Background(),
		data.SessionID,
		data.TaskID,
		data.Status.RepositoryName,
		data.Status.Branch,
	)
}

// shouldFirePushDetection decides whether to kick off PR association for one
// status event. It fires in two cases (see trackPushAndAssociatePR doc):
//
//   - Transition: the previous observation had ahead>0 and this one has ahead=0
//     with a remote branch set.
//   - First observation: no previous entry, this one has ahead=0 with a
//     remote branch set — meaning a push happened before agentctl's poller
//     observed the ahead>0 phase.
//
// Pulled out as a pure function so the decision logic can be tested without
// spawning the goroutine that calls the GitHub API.
func shouldFirePushDetection(loaded bool, prevAhead int, status *lifecycle.GitStatusData) bool {
	if status == nil {
		return false
	}
	if status.RemoteBranch == "" || status.Ahead != 0 {
		return false
	}
	if !loaded {
		return true
	}
	return prevAhead > 0
}

// pushTrackerKey builds the per-(session, repo) key used by pushTracker.
// Empty repository_name (single-repo / repo-less sessions) keeps the legacy
// single-key behaviour.
func pushTrackerKey(sessionID, repositoryName string) string {
	return sessionID + "|" + repositoryName
}

// pushTrackerForget drops every pushTracker entry belonging to a session. The
// tracker is keyed (session|repo); a single session can have N entries (one
// per repo in a multi-repo task). Called when the session is deleted so its
// tracker rows don't linger for the lifetime of the process.
func (s *Service) pushTrackerForget(sessionID string) {
	prefix := sessionID + "|"
	s.pushTracker.Range(func(k, _ any) bool {
		key, ok := k.(string)
		if ok && strings.HasPrefix(key, prefix) {
			s.pushTracker.Delete(key)
		}
		return true
	})
}

// syncPRWatchBranch updates the PR watch branch if the live git branch
// differs from what's stored (e.g. user renamed the branch).
// Only updates watches that haven't found a PR yet (pr_number=0).
func (s *Service) syncPRWatchBranch(ctx context.Context, sessionID, liveBranch string) {
	if s.githubService == nil || liveBranch == "" {
		return
	}
	watch, err := s.githubService.GetPRWatchBySession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get PR watch for branch sync",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if watch == nil || watch.PRNumber != 0 {
		return
	}
	if watch.Branch == liveBranch {
		return
	}
	s.logger.Info("PR watch branch changed, updating from git status",
		zap.String("session_id", sessionID),
		zap.String("old_branch", watch.Branch),
		zap.String("new_branch", liveBranch))
	if updateErr := s.githubService.UpdatePRWatchBranchIfSearching(ctx, watch.ID, liveBranch); updateErr != nil {
		s.logger.Error("failed to update PR watch branch",
			zap.String("session_id", sessionID), zap.Error(updateErr))
	}
}

// handleContextWindowUpdated handles context window updates and persists them to session metadata
func (s *Service) handleContextWindowUpdated(ctx context.Context, data watcher.ContextWindowData) {
	s.logger.Debug("handling context window update",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.TaskSessionID),
		zap.Int64("size", data.ContextWindowSize),
		zap.Int64("used", data.ContextWindowUsed))

	if data.TaskSessionID == "" {
		s.logger.Debug("missing session_id for context window update",
			zap.String("task_id", data.TaskID))
		return
	}

	contextWindowData := map[string]interface{}{
		"size":       data.ContextWindowSize,
		"used":       data.ContextWindowUsed,
		"remaining":  data.ContextWindowRemaining,
		"efficiency": data.ContextEfficiency,
	}

	// Persist to database asynchronously using json_set to atomically set one
	// key without clobbering other metadata keys (plan_mode, prepare_result).
	go func() {
		if err := s.repo.SetSessionMetadataKey(context.Background(), data.TaskSessionID, "context_window", contextWindowData); err != nil {
			s.logger.Error("failed to update session with context window",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.TaskSessionID),
				zap.Error(err))
		} else {
			s.logger.Debug("persisted context window to session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.TaskSessionID))
		}
	}()

	// Broadcast context window update so the frontend can update in real-time.
	// This uses the existing session.state_changed event with metadata included.
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			map[string]interface{}{
				"task_id":    data.TaskID,
				"session_id": data.TaskSessionID,
				"metadata": map[string]interface{}{
					"context_window": contextWindowData,
				},
			},
		))
	}
}

// handlePermissionRequest handles permission request events and saves as message
func (s *Service) handlePermissionRequest(ctx context.Context, data watcher.PermissionRequestData) {
	s.logger.Debug("handling permission request",
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.String("title", data.Title))

	if data.TaskSessionID == "" {
		s.logger.Warn("missing session_id for permission_request",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return
	}

	s.setSessionWaitingForInput(ctx, data.TaskID, data.TaskSessionID)

	if s.messageCreator != nil {
		_, err := s.messageCreator.CreatePermissionRequestMessage(
			ctx,
			data.TaskID,
			data.TaskSessionID,
			data.PendingID,
			data.ToolCallID,
			data.Title,
			s.getActiveTurnID(data.TaskSessionID),
			data.Options,
			data.ActionType,
			data.ActionDetails,
		)
		if err != nil {
			s.logger.Error("failed to create permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		} else {
			s.logger.Debug("created permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID))
		}
	}
}

// handleGitCommitCreated handles git commit events by forwarding them to the frontend.
// In the live model, commits are not persisted to DB - they're only captured at archive time.
func (s *Service) handleGitCommitCreated(ctx context.Context, data watcher.GitEventData) {
	if data.Commit == nil {
		s.logger.Debug("missing commit data for git commit event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commit created",
		zap.String("task_id", data.TaskID),
		zap.String("commit_sha", data.Commit.CommitSHA))

	// Forward commit_created event to WebSocket subject for frontend real-time updates
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
			Type:      lifecycle.GitEventTypeCommitCreated,
			SessionID: data.SessionID,
			TaskID:    data.TaskID,
			Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
			Commit: &lifecycle.GitCommitData{
				CommitSHA:      data.Commit.CommitSHA,
				ParentSHA:      data.Commit.ParentSHA,
				Message:        data.Commit.Message,
				AuthorName:     data.Commit.AuthorName,
				AuthorEmail:    data.Commit.AuthorEmail,
				FilesChanged:   data.Commit.FilesChanged,
				Insertions:     data.Commit.Insertions,
				Deletions:      data.Commit.Deletions,
				CommittedAt:    data.Commit.CommittedAt,
				RepositoryName: data.Commit.RepositoryName,
			},
		})
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}
}

// handleGitCommitsReset handles git reset events by forwarding them to the frontend.
// In the live model, no DB cleanup is needed - the frontend queries agentctl directly.
func (s *Service) handleGitCommitsReset(ctx context.Context, data watcher.GitEventData) {
	if data.Reset == nil {
		s.logger.Debug("missing reset data for git reset event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commits reset",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("previous_head", data.Reset.PreviousHead),
		zap.String("current_head", data.Reset.CurrentHead))

	// Forward commits_reset event to WebSocket subject for frontend real-time updates
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
			Type:      lifecycle.GitEventTypeCommitsReset,
			SessionID: data.SessionID,
			TaskID:    data.TaskID,
			Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
			Reset: &lifecycle.GitResetData{
				PreviousHead:   data.Reset.PreviousHead,
				CurrentHead:    data.Reset.CurrentHead,
				RepositoryName: data.Reset.RepositoryName,
			},
		})
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}
}

// handleBranchSwitched handles branch switch events by updating the session's base commit
// and forwarding the event to the frontend for real-time updates.
func (s *Service) handleBranchSwitched(ctx context.Context, data watcher.GitEventData) {
	if data.BranchSwitch == nil {
		s.logger.Debug("missing branch switch data for branch switch event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Info("handling branch switch",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("previous_branch", data.BranchSwitch.PreviousBranch),
		zap.String("current_branch", data.BranchSwitch.CurrentBranch),
		zap.String("new_base_commit", data.BranchSwitch.BaseCommit))

	// Update the session's base commit SHA to reflect the new branch's merge-base
	if data.BranchSwitch.BaseCommit != "" {
		if err := s.repo.UpdateTaskSessionBaseCommit(ctx, data.SessionID, data.BranchSwitch.BaseCommit); err != nil {
			s.logger.Error("failed to update session base commit after branch switch",
				zap.String("session_id", data.SessionID),
				zap.String("base_commit", data.BranchSwitch.BaseCommit),
				zap.Error(err))
		} else {
			s.logger.Info("updated session base commit after branch switch",
				zap.String("session_id", data.SessionID),
				zap.String("base_commit", data.BranchSwitch.BaseCommit))
		}
	}

	// Persist the new branch name to the session's worktree record so downstream
	// consumers (PR watch reconciliation, branch listings) observe the current
	// branch rather than the value captured at worktree creation. Without this,
	// renaming or switching branches (e.g. `git branch -m`, `git checkout`)
	// leaves PR auto-association stuck on the original branch.
	if data.BranchSwitch.CurrentBranch != "" {
		if err := s.repo.UpdateTaskSessionWorktreeBranch(ctx, data.SessionID, data.BranchSwitch.CurrentBranch); err != nil {
			s.logger.Error("failed to update session worktree branch after branch switch",
				zap.String("session_id", data.SessionID),
				zap.String("current_branch", data.BranchSwitch.CurrentBranch),
				zap.Error(err))
		}

		// Reset the PR watch for this session so the poller re-searches for a PR
		// on the new branch. This handles both rename (same PR, new branch name)
		// and stacked-PR workflows (switching to a different branch with its own
		// open PR).
		s.resetPRWatchForBranchSwitch(ctx, data.SessionID, data.BranchSwitch.CurrentBranch)
	}

	// Forward branch_switched event to WebSocket subject for frontend real-time updates
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
			Type:      lifecycle.GitEventTypeBranchSwitched,
			SessionID: data.SessionID,
			TaskID:    data.TaskID,
			Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
			BranchSwitch: &lifecycle.GitBranchSwitchData{
				PreviousBranch: data.BranchSwitch.PreviousBranch,
				CurrentBranch:  data.BranchSwitch.CurrentBranch,
				CurrentHead:    data.BranchSwitch.CurrentHead,
				BaseCommit:     data.BranchSwitch.BaseCommit,
				RepositoryName: data.BranchSwitch.RepositoryName,
			},
		})
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}
}

// resetPRWatchForBranchSwitch re-points the session's existing PR watch to the
// new branch and marks it as searching (pr_number=0) so the poller will
// discover the PR for the new branch on its next tick. If no watch exists this
// is a no-op — a watch will be created on the next push.
func (s *Service) resetPRWatchForBranchSwitch(ctx context.Context, sessionID, newBranch string) {
	if s.githubService == nil {
		return
	}
	watch, err := s.githubService.GetPRWatchBySession(ctx, sessionID)
	if err != nil {
		s.logger.Debug("failed to look up PR watch for branch switch",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	if watch == nil {
		return
	}
	if watch.Branch == newBranch && watch.PRNumber == 0 {
		return
	}
	if err := s.githubService.ResetPRWatch(ctx, watch.ID, newBranch); err != nil {
		s.logger.Error("failed to reset PR watch after branch switch",
			zap.String("session_id", sessionID), zap.String("new_branch", newBranch),
			zap.Error(err))
		return
	}
	s.logger.Info("reset PR watch after branch switch",
		zap.String("session_id", sessionID),
		zap.String("new_branch", newBranch))
}
