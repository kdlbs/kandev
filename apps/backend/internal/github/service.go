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
	mergeMethodsCache    *ttlCache
	accessibleReposCache *ttlCache
	protectionCache      *branchProtectionCache
	rateTracker          *RateTracker

	// cleanupFailureMu guards cleanupFailureCounts; the cleanup loop is the
	// only writer but the global sweep + per-watch sweep can run concurrently
	// in different goroutines, and the map is shared between them.
	cleanupFailureMu     sync.Mutex
	cleanupFailureCounts map[string]int
}

// NewService creates a new GitHub service.
func NewService(client Client, authMethod string, secrets SecretProvider, store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		client:               client,
		authMethod:           authMethod,
		secrets:              secrets,
		store:                store,
		eventBus:             eventBus,
		logger:               log,
		searchCache:          newTTLCache(),
		prStatusCache:        newTTLCache(),
		mergeMethodsCache:    newMergeMethodsCache(),
		accessibleReposCache: newAccessibleReposCache(),
		protectionCache:      newBranchProtectionCache(),
		rateTracker:          NewRateTracker(eventBus, log),
		cleanupFailureCounts: make(map[string]int),
	}
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
