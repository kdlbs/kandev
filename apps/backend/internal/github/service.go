package github

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// Auth method constants.
const (
	AuthMethodNone = "none"
	AuthMethodPAT  = "pat"
)

// defaultBranchMain and defaultBranchMaster are the conventional default branch
// names sorted to the top of branch pickers.
const (
	defaultBranchMain   = "main"
	defaultBranchMaster = "master"
)

// reviewEventApprove is the GitHub Reviews API event value for a positive
// review. Extracted because it appears in the controller validator, the
// service's self-approval guard, and tests.
const reviewEventApprove = "APPROVE"

// prSyncFreshnessWindow is how long PR data is considered fresh (skip GitHub API).
const prSyncFreshnessWindow = 30 * time.Second

// cleanupFetchFailureThreshold is the number of consecutive GetPRFeedback /
// GetIssueState errors a single dedup row may accumulate before the cleanup
// loop logs a Warn. The previous behavior swallowed the error at Debug level
// so transient outages — auth-token expiry, rate-limit exhaustion — silently
// blocked cleanup forever.
const cleanupFetchFailureThreshold = 5

// TaskDeleter deletes tasks by ID. Used for cleaning up merged PR tasks.
// Implementations should return errors wrapping ErrTaskNotFound when the
// task is already gone.
type TaskDeleter interface {
	DeleteTask(ctx context.Context, taskID string) error
}

// isTaskNotFound reports whether an error from TaskDeleter signals the task
// was already gone. Adapters wrap their underlying not-found error with
// ErrTaskNotFound — see cmd/kandev/turn_adapters.go's taskDeleterAdapter.
func isTaskNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrTaskNotFound)
}

// TaskSessionChecker checks whether the user genuinely engaged with a task
// (authored at least one non-auto-start message). Used by cleanup logic to
// preserve tasks the user touched while sweeping auto-started-only ones.
type TaskSessionChecker interface {
	HasUserAuthoredMessage(ctx context.Context, taskID string) (bool, error)
}

// SecretManager handles secret creation, update, and deletion.
type SecretManager interface {
	Create(ctx context.Context, name, value string) (id string, err error)
	Update(ctx context.Context, id, value string) error
	Delete(ctx context.Context, id string) error
}

// Service coordinates GitHub integration operations.
type Service struct {
	mu                   sync.Mutex
	client               Client
	authMethod           string
	secrets              SecretProvider
	secretManager        SecretManager
	store                *Store
	eventBus             bus.EventBus
	logger               *logger.Logger
	taskDeleter          TaskDeleter
	taskSessionChecker   TaskSessionChecker
	syncGroup            singleflight.Group
	taskEventSubs        []bus.Subscription
	searchCache          *ttlCache
	prStatusCache        *ttlCache
	prFeedbackCache      *ttlCache
	mergeMethodsCache    *ttlCache
	accessibleReposCache *ttlCache
	repoErrorCache       *ttlCache
	protectionCache      *branchProtectionCache
	rateTracker          *RateTracker

	// cleanupFailureMu guards cleanupFailureCounts; the cleanup loop is the
	// only writer but the global sweep + per-watch sweep can run concurrently
	// in different goroutines, and the map is shared between them.
	cleanupFailureMu     sync.Mutex
	cleanupFailureCounts map[string]int

	// inflightWorkspaceRefreshes tracks workspaces whose stale-PR background
	// refresh is currently running, so overlapping ListWorkspaceTaskPRs polls
	// from the frontend coalesce instead of stacking goroutines (each of
	// which fires its own batched GraphQL request). Keys are workspaceID;
	// presence means "a refresh is in flight". sync.Map is the right shape
	// here because keys are unbounded and short-lived and the hot path is a
	// LoadOrStore guard, not iteration.
	inflightWorkspaceRefreshes sync.Map

	// stopCtx / stopCancel / bgWG own the lifecycle of background goroutines
	// the service spawns lazily (currently refreshStaleWorkspaceWatches).
	// Stop() cancels stopCtx and waits for bgWG to drain, satisfying the
	// "long-running goroutine has a single owner with explicit start/stop"
	// convention in apps/backend/AGENTS.md. stopOnce makes Stop idempotent
	// so the Provide cleanup func can be invoked from any number of
	// shutdown paths.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	bgWG       sync.WaitGroup
	stopOnce   sync.Once
}

// NewService creates a new GitHub service.
func NewService(client Client, authMethod string, secrets SecretProvider, store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	stopCtx, stopCancel := context.WithCancel(context.Background())
	return &Service{
		client:               client,
		authMethod:           authMethod,
		secrets:              secrets,
		store:                store,
		eventBus:             eventBus,
		logger:               log,
		searchCache:          newTTLCache(),
		prStatusCache:        newTTLCache(),
		prFeedbackCache:      newPRFeedbackCache(),
		mergeMethodsCache:    newMergeMethodsCache(),
		accessibleReposCache: newAccessibleReposCache(),
		repoErrorCache:       newRepoErrorCache(),
		protectionCache:      newBranchProtectionCache(),
		rateTracker:          NewRateTracker(eventBus, log),
		cleanupFailureCounts: make(map[string]int),
		stopCtx:              stopCtx,
		stopCancel:           stopCancel,
	}
}

// Stop cancels in-flight background goroutines (currently the
// per-workspace stale-PR refreshes spawned by refreshStaleWorkspaceWatches)
// and waits for them to drain. Idempotent: safe to call from the Provide
// cleanup func and any other shutdown path.
func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.stopCancel()
		s.bgWG.Wait()
	})
}

// RateTracker exposes the service's rate-limit tracker so factory callers
// can wire it into individual clients.
func (s *Service) RateTracker() *RateTracker {
	return s.rateTracker
}

// newPATClient builds a PATClient pre-wired with the service's shared rate
// tracker. Centralizing this guards against forgetting the wiring on auth
// flips (e.g. ConfigureToken), which would otherwise leave PAT calls
// invisible to the rate-limit UI, health checks, and poller throttling.
func (s *Service) newPATClient(token string) *PATClient {
	c := NewPATClient(token)
	attachRateTracker(c, s.rateTracker, s.logger)
	return c
}

// SetTaskDeleter sets the task deletion dependency for cleanup operations.
func (s *Service) SetTaskDeleter(d TaskDeleter) { s.taskDeleter = d }

// SetTaskSessionChecker sets the session checker for cleanup operations.
func (s *Service) SetTaskSessionChecker(c TaskSessionChecker) { s.taskSessionChecker = c }

// SetSecretManager sets the secret manager for token configuration operations.
func (s *Service) SetSecretManager(m SecretManager) { s.secretManager = m }

// Client returns the underlying GitHub client (may be nil if not authenticated).
func (s *Service) Client() Client {
	return s.client
}

// TestStore returns the store for test/mock use only.
func (s *Service) TestStore() *Store {
	return s.store
}

// ListTaskPRsByTaskIDs forwards to the underlying store. Exposed so other
// packages (e.g. internal/office) can read PR associations without
// importing internal/github/store.
func (s *Service) ListTaskPRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*TaskPR, error) {
	if s.store == nil {
		return map[string][]*TaskPR{}, nil
	}
	return s.store.ListTaskPRsByTaskIDs(ctx, taskIDs)
}

// TestEventBus returns the event bus for test/mock use only.
func (s *Service) TestEventBus() bus.EventBus {
	return s.eventBus
}

// IsAuthenticated returns whether the service has a working GitHub client.
// Returns false when using the NoopClient fallback (authMethod == "none").
func (s *Service) IsAuthenticated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && s.authMethod != AuthMethodNone
}

// AuthMethod returns the authentication method ("gh_cli", "pat", or "none").
func (s *Service) AuthMethod() string {
	return s.authMethod
}

// isRepoCachedAsMissing reports whether the (owner, repo) tuple is in the
// 10-min negative cache. Called from SyncWatchesBatched and per-watch
// probes before acquiring the gh throttle, so the storm against
// missing/unauthorized repos short-circuits without burning a slot.
func (s *Service) isRepoCachedAsMissing(owner, repo string) bool {
	if s == nil || s.repoErrorCache == nil {
		return false
	}
	v, ok := s.repoErrorCache.get(repoErrorCacheKey(owner, repo))
	if !ok {
		return false
	}
	_, isErr := v.(cachedErr)
	return isErr
}

// repoErrorGenSnapshot returns the current cache generation. Callers
// snapshot this BEFORE issuing the fetch that may classify a repo as
// missing, and pass the snapshot to markRepoAsMissing — that way an
// evictRepoNegative / ClearRepoErrorCache that fires DURING the fetch
// (bumping gen) causes the post-fetch write to be dropped, and the
// negative cache reflects the user's latest intent rather than the
// stale classifier result.
func (s *Service) repoErrorGenSnapshot() uint64 {
	if s == nil || s.repoErrorCache == nil {
		return 0
	}
	return s.repoErrorCache.generation()
}

// markRepoAsMissing stores ErrRepoNotResolvable for (owner, repo) in the
// negative cache. Called after a per-watch or batched probe deterministically
// classifies the repo as missing/unauthorized (see isRepoNotResolvableErr).
// `gen` must be a snapshot taken via repoErrorGenSnapshot BEFORE the fetch
// — passing the current generation at write time would defeat the guard:
// a concurrent del() / clear() that bumped gen between the fetch start and
// the write would still be present at write time, so a write-time snapshot
// would land in the new generation and the just-evicted entry would be
// reinserted. The fetch-start snapshot turns the write into a no-op when
// any eviction has fired since the fetch began.
func (s *Service) markRepoAsMissing(owner, repo string, gen uint64) {
	if s == nil || s.repoErrorCache == nil {
		return
	}
	s.repoErrorCache.setIfCurrentGeneration(
		repoErrorCacheKey(owner, repo), cachedErr{err: ErrRepoNotResolvable}, gen)
}

// evictRepoNegative drops any negative-cache entry for (owner, repo). Called
// from the PR watch (re)create paths and AssociatePRByURL so a freshly
// linked or re-linked repository is probed immediately on the next sync,
// without waiting out the 10-min TTL.
func (s *Service) evictRepoNegative(owner, repo string) {
	if s == nil || s.repoErrorCache == nil {
		return
	}
	s.repoErrorCache.del(repoErrorCacheKey(owner, repo))
}

// ClearRepoErrorCache drops every entry from the negative repo-resolution
// cache. Called from ConfigureToken / ClearToken so a token swap or clear
// invalidates classifications that were made under the prior identity —
// without this, a repo that the old token couldn't see would stay
// "permanent: true" for up to 10 minutes after the user fixes auth, and
// the frontend retry loop would stop early on stale data. Nil-guards the
// cache because tests construct Service literals without going through
// NewService.
func (s *Service) ClearRepoErrorCache() {
	if s == nil || s.repoErrorCache == nil {
		return
	}
	s.repoErrorCache.clear()
}

// ClearPRCaches drops the PR status and PR feedback caches. Called from
// the token-swap path because review visibility differs across identities
// (a PR readable to the old token may not be readable to the new one, and
// vice versa); without this, the new identity could see stale review /
// check / comment data for up to one TTL after the swap.
func (s *Service) ClearPRCaches() {
	if s == nil {
		return
	}
	if s.prStatusCache != nil {
		s.prStatusCache.clear()
	}
	if s.prFeedbackCache != nil {
		s.prFeedbackCache.clear()
	}
}
