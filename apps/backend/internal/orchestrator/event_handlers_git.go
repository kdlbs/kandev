package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// gitSnapshotPersistInterval is the minimum time between persisted live git
// status snapshots for a single session when the underlying status hasn't
// changed. Writes still happen immediately when the status hash changes.
const gitSnapshotPersistInterval = 30 * time.Second

const gitSnapshotTriggeredByAgentCompleted = "agent_completed"

// gitSnapshotCacheMaxEntries bounds the in-memory throttle map so a long-lived
// backend with many sessions can't grow it without limit. When the cache is
// full and a new session arrives, the oldest entry by lastWrite is evicted.
const gitSnapshotCacheMaxEntries = 4096

// gitlabProviderName is the repositories.provider value used for GitLab,
// mirrored locally the same way githubPRStateOpen mirrors the github
// package's vocabulary rather than importing task/service's unexported
// equivalent.
const gitlabProviderName = "gitlab"

type gitSnapshotCacheEntry struct {
	hash      string
	lastWrite time.Time
}

// gitSnapshotCache throttles writes to the live git snapshot cache per
// environment and repository. It is process-local — first event after a
// restart will rewrite the row, which is fine because
// UpsertLatestLiveGitSnapshot is idempotent.
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
func (c *gitSnapshotCache) shouldWrite(taskEnvironmentID, repositoryName, hash string, now time.Time) bool {
	key := gitSnapshotCacheKey(taskEnvironmentID, repositoryName)
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, ok := c.byID[key]
	if ok && prev.hash == hash && now.Sub(prev.lastWrite) < gitSnapshotPersistInterval {
		return false
	}
	if !ok && c.maxSize > 0 && len(c.byID) >= c.maxSize {
		c.evictOldestLocked()
	}
	c.byID[key] = gitSnapshotCacheEntry{hash: hash, lastWrite: now}
	return true
}

func gitSnapshotCacheKey(taskEnvironmentID, repositoryName string) string {
	return taskEnvironmentID + "\x00" + repositoryName
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

// forget removes every cached repository entry for an environment. Called
// when an environment is deleted so the cache doesn't retain stale state for
// workspaces that will never receive another git event.
func (c *gitSnapshotCache) forget(taskEnvironmentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := taskEnvironmentID + "\x00"
	for key := range c.byID {
		if strings.HasPrefix(key, prefix) {
			delete(c.byID, key)
		}
	}
}

func gitStatusHash(s *lifecycle.GitStatusData) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|%s|%d|%d|%d|%d",
		s.RepositoryName, s.Branch, s.RemoteBranch, s.HeadCommit, s.BaseCommit,
		s.ComparisonTarget, s.ComparisonStatus, s.ComparisonErrorCode,
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
	if data.TaskEnvironmentID == "" {
		data.TaskEnvironmentID, _ = s.resolveGitSnapshotEnvironmentID(ctx, data.SessionID)
	}

	// Forward status_update event to WebSocket subject for frontend
	// The frontend uses this for real-time updates during active sessions
	if s.eventBus != nil && data.TaskEnvironmentID != "" {
		event := bus.NewEvent(events.GitWSEvent, "orchestrator", &data)
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}

	// Update PR watch branch if the user changed branches (e.g. renamed)
	s.syncPRWatchBranch(ctx, data.TaskID, data.SessionID, data.Status.RepositoryName, data.Status.Branch)

	// Push detection: when ahead goes from >0 to 0, a push happened
	s.trackPushAndAssociatePR(ctx, data)

	// Persist a throttled cache of the live status so the sidebar diff badge
	// works for tasks whose executor isn't currently running (and across
	// backend restarts). Best-effort: errors are logged and swallowed.
	s.persistGitStatusSnapshot(ctx, data)
}

// persistGitStatusSnapshot writes a single cached "live monitor" snapshot per
// environment, throttled by gitSnapshotCache. The cached row is read by
// appendDBSnapshotGitStatus when no live execution is available.
func (s *Service) persistGitStatusSnapshot(ctx context.Context, data watcher.GitEventData) {
	if s.repo == nil || data.SessionID == "" || data.Status == nil {
		return
	}
	if s.gitSnapshotCache == nil {
		return
	}
	taskEnvironmentID := data.TaskEnvironmentID
	if taskEnvironmentID == "" {
		var ok bool
		taskEnvironmentID, ok = s.resolveGitSnapshotEnvironmentID(ctx, data.SessionID)
		if !ok {
			return
		}
	}
	hash := gitStatusHash(data.Status)
	if !s.gitSnapshotCache.shouldWrite(taskEnvironmentID, data.Status.RepositoryName, hash, time.Now()) {
		return
	}

	st := data.Status
	snapshot := &models.GitSnapshot{
		TaskEnvironmentID: taskEnvironmentID,
		SessionID:         data.SessionID,
		Branch:            st.Branch,
		RemoteBranch:      st.RemoteBranch,
		HeadCommit:        st.HeadCommit,
		BaseCommit:        st.BaseCommit,
		Ahead:             st.Ahead,
		Behind:            st.Behind,
		Files:             nil, // intentional: badge only needs totals
		Metadata: map[string]interface{}{
			"repository_name":       st.RepositoryName,
			"branch_additions":      st.BranchAdditions,
			"branch_deletions":      st.BranchDeletions,
			"comparison_target":     st.ComparisonTarget,
			"comparison_status":     st.ComparisonStatus,
			"comparison_error_code": st.ComparisonErrorCode,
			"modified":              st.Modified,
			"added":                 st.Added,
			"deleted":               st.Deleted,
			"untracked":             st.Untracked,
			"renamed":               st.Renamed,
			"timestamp":             data.Timestamp,
		},
	}
	if err := s.repo.UpsertLatestLiveGitSnapshot(ctx, snapshot); err != nil {
		s.logger.Debug("failed to persist live git snapshot",
			zap.String("task_environment_id", taskEnvironmentID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
	}
}

func (s *Service) resolveGitSnapshotEnvironmentID(ctx context.Context, sessionID string) (string, bool) {
	if s.repo == nil || sessionID == "" {
		return "", false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Debug("failed to resolve git snapshot environment",
			zap.String("session_id", sessionID), zap.Error(err))
		return "", false
	}
	if session == nil || session.TaskEnvironmentID == "" {
		s.logger.Debug("skipping git snapshot without task environment",
			zap.String("session_id", sessionID))
		return "", false
	}
	return session.TaskEnvironmentID, true
}

// pushTrackerUnsynced is the pushTracker sentinel for "no upstream
// configured yet" (RemoteBranch == ""). Deliberately distinct from 0 (a real
// RemoteAhead of zero, meaning "has an upstream, nothing left to push") —
// collapsing both into 0 would make "never pushed" and "just pushed"
// indistinguishable to the transition check below, since a task's very
// first status observation (no upstream, no commits yet) would itself read
// as "already synced" and silently consume the one-time first-observation
// fast path before any real push ever happens.
const pushTrackerUnsynced = -1

// trackPushAndAssociatePR detects git pushes by tracking the "remote ahead"
// count — commits not yet present on the branch's own upstream (see
// lifecycle.GitStatusData.RemoteAhead). Two cases trigger detection:
//
//  1. Transition: the branch was unsynced (no upstream yet, or remote-ahead
//     >0 with a remote branch set) on the previous observation, and this one
//     shows remote-ahead=0 with a remote branch — the normal in-session
//     push, observed across two status events.
//  2. First-observation sync: the very first status event for this
//     (session, repo) already shows remote-ahead=0 with a remote branch.
//     This means a push happened before agentctl's poller saw the unsynced
//     phase (the poll cadence missed it, or the session resumed after a
//     restart). For a fresh task branch, RemoteBranch is only populated
//     after `git push -u`, so seeing it pre-synced is itself a push signal.
//
// Without (2), multi-repo tasks routinely lose PR associations for any repo
// whose first poll happens to land after the push completes — the
// transition never gets observed and the watch never gets created.
//
// Multi-repo: keyed per (session, repository_name) so each repo's
// transitions are tracked independently. Without this, agentctl's per-repo
// status events race-overwrote each other's remote-ahead counts and only one
// repo's push got detected.
//
// Deliberately keys off RemoteAhead, not the base-branch-relative Ahead:
// Ahead reflects "commits ahead of origin/main" for the Push/Pull UI badge
// and stays > 0 for the entire life of a normal feature branch — it never
// drops to 0 just because the branch itself was pushed, so it could never
// signal "a push just happened" for real feature-branch work.
func (s *Service) trackPushAndAssociatePR(ctx context.Context, data watcher.GitEventData) {
	key := pushTrackerKey(data.SessionID, data.Status.RepositoryName)
	trackedValue := data.Status.RemoteAhead
	if data.Status.RemoteBranch == "" {
		trackedValue = pushTrackerUnsynced
	}
	prevValueVal, loaded := s.pushTracker.Swap(key, trackedValue)
	prevValue, _ := prevValueVal.(int)
	if !shouldFirePushDetection(loaded, prevValue, data.Status) {
		return
	}
	s.logger.Info("git push detected, starting PR association",
		zap.String("session_id", data.SessionID),
		zap.String("task_id", data.TaskID),
		zap.String("repository_name", data.Status.RepositoryName),
		zap.String("branch", data.Status.Branch),
		zap.Bool("first_observation", !loaded))
	go s.dispatchPushDetection(
		context.Background(),
		data.SessionID,
		data.TaskID,
		data.Status.RepositoryName,
		data.Status.Branch,
	)
}

// dispatchPushDetection routes one push-detection run to the right
// provider's association logic, so the two providers' code paths issue zero
// calls into each other's client. GitHub's proven detectPushAndAssociatePR is
// called verbatim for every non-GitLab (including unknown/legacy-empty
// provider) repository — this passes the shared repository identity into the
// existing provider-specific path. Extracted from trackPushAndAssociatePR to
// keep that function inside the statement budget. The taskID the write lands
// under may be redirected away from the observing task; see
// resolveEffectivePushTaskID.
func (s *Service) dispatchPushDetection(ctx context.Context, sessionID, taskID, repositoryName, branch string) {
	// Identity (owner/name/repositoryID) MUST resolve against the observing
	// task, not the redirected one: the physical checkout, session, and
	// task_repositories rows belong to whichever task's session actually saw
	// the push (a subtask sharing a parent's worktree still has its own
	// session row). Only the write destination below is redirected.
	identity := s.resolvePushRepositoryIdentity(ctx, sessionID, taskID, repositoryName)
	effectiveTaskID := s.resolveEffectivePushTaskIDForSession(ctx, sessionID, taskID, identity.repositoryID)
	if identity.provider == gitlabProviderName {
		s.detectPushAndAssociateMRWithIdentity(ctx, sessionID, effectiveTaskID, repositoryName, branch, identity)
		return
	}
	if s.githubService == nil {
		return
	}
	s.detectPushAndAssociatePRWithIdentity(ctx, sessionID, effectiveTaskID, repositoryName, branch, identity)
}

// resolveEffectivePushTaskID redirects a push-detected PR/MR association from
// the observing task to its workspace-group owner task. A subtask sharing its
// parent's worktree via inherit_parent/shared_group workspace modes observes
// the SAME branch pushes as every other member of the group, so writing the
// association under the observing task's own ID binds one PR/MR to several
// unrelated tasks (the group owner plus whichever subtasks happened to be
// running when the push landed). Falls back to the original taskID — leaving
// the pre-fix behavior for that call — when there is no active group, the
// repo doesn't support group lookups, the lookup fails, the resolved owner
// task can't be loaded or is archived (an archived owner is not a valid
// association target), or the owner task doesn't hold repositoryID itself
// (redirecting would make the downstream write get rejected by
// validateTaskRepositoryID and dropped — see associatePRWithTask — which is
// worse than leaving the association under the observing task). repositoryID
// may be "" (identity couldn't be resolved yet); the ownership check is
// skipped in that case, matching validateTaskRepositoryID's own no-op on an
// empty repositoryID.
//
// This is the single boundary every watch-creation and association call site
// routes through — dispatchPushDetection, ensureSessionPRWatch,
// CheckSessionPR, buildTaskBranchList (feeding the poller's
// reconcileWatches), and resetPRWatchForBranchSwitch — so no
// member-attributed github_pr_watches row is ever created and the poller's
// own AssociatePRWithTask(watch.TaskID, ...) writes under the
// already-redirected task.
type sessionWorkspaceGroupOwnerResolver interface {
	GetWorkspaceGroupOwnerTaskIDForSession(ctx context.Context, taskID, sessionID string) (string, error)
}

func (s *Service) resolveEffectivePushTaskIDForSession(
	ctx context.Context, sessionID, taskID, repositoryID string,
) string {
	if taskID == "" {
		return taskID
	}
	store, ok := s.repo.(repoStore)
	if !ok {
		return taskID
	}
	var ownerTaskID string
	var err error
	if sessionID != "" {
		if sessionResolver, ok := any(store).(sessionWorkspaceGroupOwnerResolver); ok {
			ownerTaskID, err = sessionResolver.GetWorkspaceGroupOwnerTaskIDForSession(ctx, taskID, sessionID)
		} else {
			// Keep compatibility with repository test doubles and older adapters.
			ownerTaskID, err = store.GetWorkspaceGroupOwnerTaskID(ctx, taskID)
		}
	} else {
		ownerTaskID, err = store.GetWorkspaceGroupOwnerTaskID(ctx, taskID)
	}
	if err != nil || ownerTaskID == "" || ownerTaskID == taskID {
		return taskID
	}
	ownerTask, err := s.repo.GetTask(ctx, ownerTaskID)
	if err != nil || ownerTask == nil || ownerTask.ArchivedAt != nil {
		return taskID
	}
	if repositoryID != "" && !s.taskHoldsRepository(ctx, store, ownerTaskID, repositoryID) {
		return taskID
	}
	s.logger.Info("redirecting push-detected PR/MR association to workspace group owner",
		zap.String("from_task_id", taskID),
		zap.String("to_task_id", ownerTaskID))
	return ownerTaskID
}

// taskHoldsRepository reports whether taskID has a task_repositories row for
// repositoryID. Used by resolveEffectivePushTaskID to avoid redirecting into
// a write that validateTaskRepositoryID would reject and associatePRWithTask
// would then silently drop.
func (s *Service) taskHoldsRepository(ctx context.Context, store repoStore, taskID, repositoryID string) bool {
	taskRepos, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return false
	}
	for _, tr := range taskRepos {
		if tr != nil && tr.RepositoryID == repositoryID {
			return true
		}
	}
	return false
}

type pushRepositoryIdentity struct {
	owner        string
	name         string
	repositoryID string
	provider     string
	projectPath  string
}

// resolvePushRepositoryIdentity resolves the repository, provider, and full
// project path from one repository snapshot. Local checkout fallbacks are
// shared by routing and association so a legacy row cannot be routed from one
// identity while its provider-specific lookup uses another.
func (s *Service) resolvePushRepositoryIdentity(
	ctx context.Context, sessionID, taskID, repositoryName string,
) pushRepositoryIdentity {
	owner, name, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, repositoryName)
	identity := pushRepositoryIdentity{
		owner:        owner,
		name:         name,
		repositoryID: repositoryID,
	}
	if owner != "" && name != "" {
		identity.projectPath = owner + "/" + name
	}
	if repositoryID == "" {
		return identity
	}
	repoObj := s.getPushRepository(ctx, repositoryID)
	if repoObj == nil {
		return identity
	}
	remoteURL := s.enrichPushRepositoryIdentity(&identity, repoObj)
	if identity.projectPath == "" {
		identity.projectPath = gitLabProjectPathFromRemoteURL(remoteURL)
	}
	if identity.provider == "" {
		identity.provider = s.resolveConfiguredGitLabProvider(ctx, taskID, remoteURL)
	}
	return identity
}

func (s *Service) getPushRepository(ctx context.Context, repositoryID string) *models.Repository {
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil
	}
	repoObj, err := store.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil
	}
	return repoObj
}

func (s *Service) enrichPushRepositoryIdentity(
	identity *pushRepositoryIdentity, repoObj *models.Repository,
) string {
	identity.provider = repoObj.Provider
	if identity.provider == "" && repoObj.LocalPath != "" {
		if provider, _, localOwner, localName := service.ResolveGitRemoteProviderIdentity(repoObj.LocalPath); provider != "" && localOwner != "" {
			identity.provider = provider
			if identity.owner == "" && localName != "" {
				identity.owner = localOwner
				identity.name = localName
				identity.projectPath = localOwner + "/" + localName
			}
		}
	}

	remoteURL := repoObj.RemoteURL
	if remoteURL == "" && repoObj.LocalPath != "" {
		if origin, localOwner, localName := service.ResolveGitRemoteIdentity(repoObj.LocalPath); origin != "" && localOwner != "" && localName != "" {
			remoteURL = origin + "/" + localOwner + "/" + localName
			if identity.projectPath == "" {
				identity.projectPath = localOwner + "/" + localName
			}
		}
	}
	return remoteURL
}

func (s *Service) resolveConfiguredGitLabProvider(ctx context.Context, taskID, remoteURL string) string {
	if s.gitlabMRLinkService == nil || remoteURL == "" {
		return ""
	}
	workspaceID := s.taskWorkspaceID(ctx, taskID)
	if workspaceID == "" {
		return ""
	}
	if s.gitlabMRLinkService.IsConfiguredGitLabHost(ctx, workspaceID, remoteURL) {
		return gitlabProviderName
	}
	return ""
}

// resolvePushRepositoryProvider looks up the provider ("github", "gitlab", or
// "" when unknown/unresolvable) of the repository backing this push, reusing
// resolvePushRepo's owner/name matching rather than re-deriving it, so
// dispatchPushDetection can route without duplicating that logic.
func (s *Service) resolvePushRepositoryProvider(ctx context.Context, sessionID, taskID, repositoryName string) string {
	return s.resolvePushRepositoryIdentity(ctx, sessionID, taskID, repositoryName).provider
}

// shouldFirePushDetection decides whether to kick off PR association for one
// status event. It fires in two cases (see trackPushAndAssociatePR doc):
//
//   - Transition: the previous observation was unsynced — either no upstream
//     yet (pushTrackerUnsynced) or a remote branch with remote-ahead>0 — and
//     this one has remote-ahead=0 with a remote branch set.
//   - First observation: no previous entry, this one has remote-ahead=0 with
//     a remote branch set — meaning a push happened before agentctl's poller
//     observed the unsynced phase.
//
// Reads RemoteAhead (commits not yet on this branch's own upstream), not the
// base-branch-relative Ahead — see lifecycle.GitStatusData.RemoteAhead and
// trackPushAndAssociatePR's doc comment for why Ahead can never signal this.
//
// prevValue must come from pushTracker's stored value, not a raw prior
// RemoteAhead: comparing against pushTrackerUnsynced (not 0) for "was this
// unsynced before" is what lets a task's first-ever observation (no
// upstream, no commits) correctly NOT count as "already synced" — otherwise
// it would consume the first-observation fast path before any real push
// happens, and the genuine no-upstream-to-synced transition on the next
// observation would never fire (prevValue would already read 0 == "was
// already synced", identical to "had nothing to push", not ">0" or
// "unsynced").
//
// Pulled out as a pure function so the decision logic can be tested without
// spawning the goroutine that calls the GitHub API.
func shouldFirePushDetection(loaded bool, prevValue int, status *lifecycle.GitStatusData) bool {
	if status == nil {
		return false
	}
	if status.RemoteBranch == "" || status.RemoteAhead != 0 {
		return false
	}
	if !loaded {
		return true
	}
	return prevValue != 0
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
// Only updates watches that haven't found a PR yet (pr_number=0), and only
// within the repository the status belongs to — a session holds one watch per
// (repository, branch), so a session-wide lookup would re-point whichever row
// the query happened to return first.
//
// This runs on every git-status tick, so the single watch listing is also the
// early-out: once every watch has found its PR there is nothing to re-point
// and we return before resolving the repository.
func (s *Service) syncPRWatchBranch(ctx context.Context, taskID, sessionID, repositoryName, liveBranch string) {
	if s.githubService == nil || liveBranch == "" {
		return
	}
	watches, err := s.githubService.ListPRWatchesBySession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get PR watch for branch sync",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if !anySearchingPRWatch(watches) {
		return
	}
	_, _, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, repositoryName)
	if watchForRepoBranch(watches, repositoryID, liveBranch) != nil {
		return
	}
	if !s.repointSearchingPRWatch(ctx, watches, sessionID, repositoryID, liveBranch, "git status") {
		// Reached only when the session has a searching watch but none for
		// this repository — most often because resolvePushRepo could not
		// resolve repositoryName and returned "". Silent here made "the PR
		// for branch X was never picked up" indistinguishable from a failed
		// reset; push detection and the poller still cover the branch.
		s.logger.Debug("no searching PR watch to re-point for branch",
			zap.String("session_id", sessionID),
			zap.String("repository_id", repositoryID),
			zap.String("branch", liveBranch))
	}
}

func anySearchingPRWatch(watches []*github.PRWatch) bool {
	for _, watch := range watches {
		if watch != nil && watch.PRNumber == 0 {
			return true
		}
	}
	return false
}

func watchForRepoBranch(watches []*github.PRWatch, repositoryID, branch string) *github.PRWatch {
	for _, watch := range watches {
		if watch != nil && watch.RepositoryID == repositoryID && watch.Branch == branch {
			return watch
		}
	}
	return nil
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
	generation := s.captureContextWindowGeneration(data.TaskSessionID)

	size, remaining, efficiency, source, ok := s.resolveContextWindowValues(ctx, data)
	if !ok {
		return
	}

	contextWindowData := map[string]interface{}{
		"size":       size,
		"used":       data.ContextWindowUsed,
		"remaining":  remaining,
		"efficiency": efficiency,
		"source":     source,
	}

	// Persist synchronously using json_set to atomically set one key without
	// clobbering other metadata keys (plan_mode, prepare_result). The watcher
	// delivers updates in arrival order; keeping persistence in this callback
	// preserves that order instead of letting independent goroutines reorder
	// samples and produce an incorrect compaction count.
	persisted, err := s.persistAndPublishContextWindowUpdate(
		context.Background(), data.TaskID, data.TaskSessionID, generation, contextWindowData,
	)
	switch {
	case err != nil:
		s.logger.Error("failed to update session with context window",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.TaskSessionID),
			zap.Error(err))
	case !persisted:
		s.logger.Debug("discarded stale context window update after reset",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.TaskSessionID))
	default:
		s.logger.Debug("persisted context window to session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.TaskSessionID))
	}
}

func (s *Service) persistAndPublishContextWindowUpdate(
	ctx context.Context,
	taskID, sessionID string,
	generation uint64,
	contextWindowData map[string]interface{},
) (bool, error) {
	persisted, count, err := s.persistContextWindowUpdate(ctx, sessionID, generation, contextWindowData)
	if !persisted || err != nil || s.eventBus == nil {
		return persisted, err
	}
	_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
		events.TaskSessionStateChanged,
		"orchestrator",
		map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"metadata": map[string]interface{}{
				contextWindowMetadataKey:                    contextWindowData,
				models.SessionMetaKeyContextCompactionCount: count,
			},
		},
	))
	return true, nil
}

func (s *Service) resolveContextWindowValues(ctx context.Context, data watcher.ContextWindowData) (int64, int64, float64, string, bool) {
	if data.ContextWindowSize > 0 {
		return data.ContextWindowSize, data.ContextWindowRemaining, data.ContextEfficiency, "acp", true
	}
	lookup := s.currentModelInfoLookup()
	if lookup == nil {
		return 0, 0, 0, "", false
	}
	modelID := s.currentRuntimeModel(ctx, data.TaskSessionID)
	if modelID == "" {
		return 0, 0, 0, "", false
	}
	info, ok := lookup.LookupModelInfo(ctx, modelID)
	if !ok || info.ContextWindow <= 0 {
		return 0, 0, 0, "", false
	}
	remaining := info.ContextWindow - data.ContextWindowUsed
	if remaining < 0 {
		remaining = 0
	}
	efficiency := float64(data.ContextWindowUsed) / float64(info.ContextWindow) * 100
	return info.ContextWindow, remaining, efficiency, "api", true
}

func (s *Service) currentRuntimeModel(ctx context.Context, sessionID string) string {
	if model, ok := s.runtimeModelBySession.Load(sessionID); ok {
		if modelID, _ := model.(string); modelID != "" {
			return modelID
		}
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	if cfg, ok := models.LoadSessionRuntimeConfig(session.Metadata); ok && cfg.Model != "" {
		return cfg.Model
	}
	if session.AgentProfileSnapshot != nil {
		if model, ok := session.AgentProfileSnapshot["model"].(string); ok {
			return model
		}
	}
	return ""
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
			data.RequestID,
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

	// Automation tasks are hidden from the kanban, so there is no UI for the
	// user to answer a permission prompt. Auto-reject and mark the run failed
	// so the failure shows up in the automation's Recent Runs.
	s.failAutomationRunOnPermission(ctx, data)
}

// failAutomationRunOnPermission checks whether the permission request belongs
// to an automation task and, if so, rejects the prompt and marks the
// corresponding automation_run row as failed.
func (s *Service) failAutomationRunOnPermission(ctx context.Context, data watcher.PermissionRequestData) {
	if s.automationService == nil || data.TaskID == "" {
		return
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil {
		return
	}
	// Keyed on origin alone — automation tasks are no longer ephemeral, and a
	// prompt nobody can answer would otherwise hang the run at task_created
	// forever, holding a max_concurrent_runs slot.
	if task.Origin != models.TaskOriginAutomationRun {
		return
	}

	// Use rejected=true so the backend persists "rejected" status. cancelled is
	// also true here because the session is going to be marked failed anyway.
	if err := s.cancelAgentPermission(ctx, ResolveAgentPermissionRequest{
		TaskID: data.TaskID, SessionID: data.TaskSessionID, RequestID: data.RequestID,
		PendingID: data.PendingID, Source: models.PermissionSourceAutomation,
	}); err != nil {
		s.logger.Warn("failed to auto-reject permission for automation run",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID),
			zap.Error(err))
	}

	errMsg := fmt.Sprintf("Permission required: %s — automation runs cannot answer prompts", data.Title)
	if err := s.automationService.MarkRunFailedByTaskID(ctx, data.TaskID, errMsg); err != nil {
		s.logger.Warn("failed to mark automation run failed after permission prompt",
			zap.String("task_id", data.TaskID), zap.Error(err))
	}
}

// pickRejectOption returns the first option_id with a reject-kind, or "" if
// none was offered.
func pickRejectOption(options []map[string]interface{}) string {
	for _, opt := range options {
		kind, _ := opt["kind"].(string)
		if strings.HasPrefix(kind, "reject") {
			if id, ok := opt["option_id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

// handleGitCommitCreated persists the commit (see persistSessionCommit) and
// forwards it to the frontend. This is the primary write path for
// task_session_commits: it fires on every commit agentctl observes, unlike
// archive capture which only ran once per task and needed the agent process
// still alive to succeed.
func (s *Service) handleGitCommitCreated(ctx context.Context, data watcher.GitEventData) {
	if data.Commit == nil {
		s.logger.Debug("missing commit data for git commit event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commit created",
		zap.String("task_id", data.TaskID),
		zap.String("commit_sha", data.Commit.CommitSHA))

	s.persistSessionCommit(ctx, data.SessionID, commitCaptureTriggerLive, &models.SessionCommit{
		CommitSHA:     data.Commit.CommitSHA,
		ParentSHA:     data.Commit.ParentSHA,
		AuthorName:    data.Commit.AuthorName,
		AuthorEmail:   data.Commit.AuthorEmail,
		CommitMessage: data.Commit.Message,
		CommittedAt:   parseCommitTime(data.Commit.CommittedAt),
		FilesChanged:  data.Commit.FilesChanged,
		Insertions:    data.Commit.Insertions,
		Deletions:     data.Commit.Deletions,
	})

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
		if err := s.updateBranchSwitchWorktreeSnapshot(ctx, data.SessionID, data.BranchSwitch.RepositoryName, data.BranchSwitch.CurrentBranch); err != nil {
			s.logger.Error("failed to update session worktree branch after branch switch",
				zap.String("session_id", data.SessionID),
				zap.String("current_branch", data.BranchSwitch.CurrentBranch),
				zap.Error(err))
		}

		// Cover the new branch with a PR watch so the poller searches for its
		// PR. This handles both rename (same PR, new branch name) and
		// stacked-PR workflows (switching to a different branch with its own
		// open PR) without stranding the branch we just left.
		s.resetPRWatchForBranchSwitch(
			ctx, data.TaskID, data.SessionID, data.BranchSwitch.RepositoryName, data.BranchSwitch.CurrentBranch,
		)
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

// updateBranchSwitchWorktreeSnapshot scopes a multi-repository branch event to
// the worktree whose path basename matches agentctl's RepositoryName tag. Older
// events and single-repository rows retain the all-worktrees fallback.
func (s *Service) updateBranchSwitchWorktreeSnapshot(ctx context.Context, sessionID, repositoryName, branch string) error {
	if repositoryName == "" {
		return s.repo.UpdateTaskSessionWorktreeBranch(ctx, sessionID, branch)
	}
	scoped, ok := s.repo.(titleBranchScopedSnapshotStore)
	if !ok {
		return s.repo.UpdateTaskSessionWorktreeBranch(ctx, sessionID, branch)
	}
	lister, ok := s.repo.(titleBranchWorktreeLister)
	if !ok {
		return s.repo.UpdateTaskSessionWorktreeBranch(ctx, sessionID, branch)
	}
	worktrees, err := lister.ListTaskSessionWorktrees(ctx, sessionID)
	if err != nil {
		return err
	}
	matched := matchingBranchSwitchWorktrees(worktrees, repositoryName)
	if len(matched) == 0 {
		if repositoryName != "" {
			return nil
		}
		return s.repo.UpdateTaskSessionWorktreeBranch(ctx, sessionID, branch)
	}
	return s.persistBranchSwitchWorktrees(ctx, scoped, sessionID, branch, matched)
}

func matchingBranchSwitchWorktrees(worktrees []*models.TaskEnvironmentRepo, repositoryName string) map[string]string {
	matched := make(map[string]string)
	for _, worktree := range worktrees {
		if worktree == nil || worktree.RepositoryID == "" {
			continue
		}
		if filepath.Base(filepath.Clean(worktree.WorktreePath)) == repositoryName {
			matched[worktree.RepositoryID] = worktree.WorktreeID
		}
	}
	if len(matched) == 0 && len(worktrees) == 1 && worktrees[0] != nil {
		matched[worktrees[0].RepositoryID] = worktrees[0].WorktreeID
	}
	return matched
}

func (s *Service) persistBranchSwitchWorktrees(
	ctx context.Context,
	scoped titleBranchScopedSnapshotStore,
	sessionID string,
	branch string,
	matched map[string]string,
) error {
	for repositoryID, worktreeID := range matched {
		if exact, exactOK := s.repo.(titleBranchWorktreeSnapshotStore); exactOK && worktreeID != "" {
			if err := exact.UpdateTaskSessionWorktreeBranchByWorktree(ctx, sessionID, worktreeID, branch); err != nil {
				return err
			}
			continue
		}
		if err := scoped.UpdateTaskSessionWorktreeBranchByRepository(ctx, sessionID, repositoryID, branch); err != nil {
			return err
		}
	}
	return nil
}

// resetPRWatchForBranchSwitch makes sure the branch the session just checked
// out is covered by a PR watch, so the poller discovers its PR on the next
// tick.
//
// It never re-points a watch that has already found a PR. That watch is the
// only handle keeping the previous branch's PR synced: both the poller and the
// on-demand sync iterate watches, and CI automation only runs off the events
// they publish. Re-pointing it froze that PR at its last-observed checks and
// review state, which stalls auto-fix (it never sees new failures) and
// auto-merge (it never sees the PR turn mergeable) for every branch but the
// one currently checked out — the multi-branch failure this replaced.
//
// A branch renamed before any PR existed still reuses the searching watch, so
// a task that hops between branches holds at most one searching watch per
// repository plus one per PR it actually opened.
func (s *Service) resetPRWatchForBranchSwitch(ctx context.Context, taskID, sessionID, repositoryName, newBranch string) {
	if s.githubService == nil {
		return
	}
	watches, err := s.githubService.ListPRWatchesBySession(ctx, sessionID)
	if err != nil {
		s.logger.Debug("failed to look up PR watch for branch switch",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	owner, repoName, repositoryID := s.resolvePushRepo(ctx, sessionID, taskID, repositoryName)
	if watchForRepoBranch(watches, repositoryID, newBranch) != nil {
		return
	}
	if s.repointSearchingPRWatch(ctx, watches, sessionID, repositoryID, newBranch, "branch switch") {
		return
	}
	if owner == "" || repoName == "" {
		return
	}
	workspaceID := s.taskWorkspaceID(ctx, taskID)
	if workspaceID == "" {
		return
	}
	effectiveTaskID := s.resolveEffectivePushTaskIDForSession(ctx, sessionID, taskID, repositoryID)
	if _, err := s.githubService.EnsurePRWatchForWorkspace(
		ctx, workspaceID, sessionID, effectiveTaskID, repositoryID, owner, repoName, newBranch,
	); err != nil {
		s.logger.Error("failed to add PR watch after branch switch",
			zap.String("session_id", sessionID), zap.String("new_branch", newBranch),
			zap.Error(err))
		return
	}
	s.logger.Info("added PR watch for switched branch",
		zap.String("session_id", sessionID),
		zap.String("repository_id", repositoryID),
		zap.String("new_branch", newBranch))
}

// repointSearchingPRWatch moves the repository's still-searching watch
// (pr_number=0) onto newBranch and reports whether it did. At most one such
// watch exists per (session, repository), so reusing it keeps branch churn
// from accumulating watches that will never find a PR.
func (s *Service) repointSearchingPRWatch(
	ctx context.Context, watches []*github.PRWatch, sessionID, repositoryID, newBranch, reason string,
) bool {
	for _, watch := range watches {
		if watch == nil || watch.RepositoryID != repositoryID || watch.PRNumber != 0 {
			continue
		}
		if err := s.githubService.ResetPRWatch(ctx, watch.ID, newBranch); err != nil {
			s.logger.Error("failed to re-point PR watch",
				zap.String("session_id", sessionID), zap.String("new_branch", newBranch),
				zap.String("reason", reason), zap.Error(err))
			return false
		}
		s.logger.Info("PR watch branch changed, updating",
			zap.String("session_id", sessionID),
			zap.String("old_branch", watch.Branch),
			zap.String("new_branch", newBranch),
			zap.String("reason", reason))
		return true
	}
	return false
}
