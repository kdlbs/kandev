package configsync

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// runDeadline bounds a single sync run (AC-OFFICE-CONFIG-SYNC-004.4a). It is
// applied by Service.SyncWorkspace, the single choke point both the poller
// and the forced "sync now" HTTP handler enter through, before the
// per-workspace lock is taken — so time spent queued behind another run for
// the same workspace counts against the budget rather than extending it.
const runDeadline = 10 * time.Minute

// Service owns config sync configuration and drives sync runs. It wraps a
// Runner (the stateless walk/apply engine in reconcile_run.go) with the
// per-workspace locking and run deadline behavior the system design's
// "Runner" component describes. The two names diverge only because Runner
// already shipped as the tested walk/apply engine before this layer was
// designed; Service is where the design's lock/schedule responsibilities
// actually live.
//
// Unlike workflowsync.Service, Service does not self-authorize: every route
// this package registers is mounted under the Office API group, whose
// officeWorkspaceScopeMiddleware already enforces per-user workspace
// ownership on the `:wsId` path param before any handler runs (see
// docs/specs/office/system-design/config-sync.md's "Security" section, and
// every other office/* subservice, none of which carries its own
// authorizer hook).
type Service struct {
	runner *Runner
	store  *Store
	logger *logger.Logger

	// locks serializes syncs and config mutations per workspace, mirroring
	// workflowsync.Service.locks (same rationale, a disjoint entity set and
	// its own lock map — see the "Scheduling and concurrency" section of
	// docs/specs/office/system-design/config-sync.md).
	locks sync.Map // workspaceID → *sync.Mutex
}

// NewService creates a config sync service over the given Runner and Store.
func NewService(runner *Runner, store *Store, log *logger.Logger) *Service {
	return &Service{
		runner: runner,
		store:  store,
		logger: log.WithFields(zap.String("component", "office-configsync-service")),
	}
}

func (s *Service) workspaceLock(workspaceID string) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore(workspaceID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// Store exposes the config store (workspace-deletion cascade, e2e reset).
func (s *Service) Store() *Store {
	return s.store
}

// GetConfigForWorkspace returns the workspace's config, or nil when unset.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*Config, error) {
	return s.store.GetConfigForWorkspace(ctx, workspaceID)
}

// HasActiveSource reports whether workspaceID has a config sync source
// configured. office/config and office/dashboard each declare their own
// single-method interface over this signature to refuse a second writer over
// the same rows (AC-OFFICE-CONFIG-SYNC-005.2/005.2b) without importing this
// package. A read failure is returned as an error rather than treated as "no
// source": the guard exists to prevent a second writer, and an unknown
// answer is not evidence that there is none.
func (s *Service) HasActiveSource(ctx context.Context, workspaceID string) (bool, error) {
	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return cfg != nil, nil
}

// SetConfigForWorkspace validates and stores the workspace's config.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*Config, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	lock := s.workspaceLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	return s.store.UpsertConfigForWorkspace(ctx, workspaceID, req)
}

// releaseOrder is the reverse of the AC-OFFICE-CONFIG-SYNC-003.9 apply order
// (skills, agents, projects, routines), matching a run's own deletion pass.
var releaseOrder = map[string]int{kindRoutine: 0, kindProject: 1, kindAgent: 2, kindSkill: 3}

// DeleteConfigForWorkspace releases every entity config sync manages for the
// workspace back to unmanaged ownership, then removes the config
// (AC-OFFICE-CONFIG-SYNC-004.9). A release failure leaves the config and
// remaining manifest rows in place so the caller can retry
// (AC-OFFICE-CONFIG-SYNC-004.9b/004.9c).
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	lock := s.workspaceLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	if err := s.release(ctx, workspaceID); err != nil {
		return fmt.Errorf("failed to release synced entities: %w", err)
	}
	return s.store.DeleteConfigForWorkspace(ctx, workspaceID)
}

// release deletes every manifest row for workspaceID one at a time, in
// AC-OFFICE-CONFIG-SYNC-003.9's reverse kind order and ascending entity_key
// within a kind (system-design/config-sync-reconciliation.md#Release). The
// manifest row's removal IS the ownership change — membership in the
// manifest is the only "managed" flag (AC-OFFICE-CONFIG-SYNC-003.8) — so no
// entity write happens here at all.
func (s *Service) release(ctx context.Context, workspaceID string) error {
	entries, err := s.store.ListManifest(ctx, workspaceID)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		if releaseOrder[entries[i].Kind] != releaseOrder[entries[j].Kind] {
			return releaseOrder[entries[i].Kind] < releaseOrder[entries[j].Kind]
		}
		return entries[i].EntityKey < entries[j].EntityKey
	})
	for _, e := range entries {
		if err := s.store.DeleteManifestEntry(ctx, workspaceID, e.Kind, e.EntityKey); err != nil {
			return fmt.Errorf("release %s %q: %w", e.Kind, e.EntityKey, err)
		}
	}
	return nil
}

// SyncWorkspace runs one sync for workspaceID. Both the poller
// (SyncDueConfigs) and the forced "sync now" HTTP handler call this same
// method, which is what makes the AC-OFFICE-CONFIG-SYNC-004.4a run deadline
// trigger-agnostic: it wraps ctx in a 10 minute timeout before taking the
// per-workspace lock, so time spent queued behind another run for this
// workspace counts against the budget.
func (s *Service) SyncWorkspace(ctx context.Context, workspaceID string) (*SyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, runDeadline)
	defer cancel()

	lock := s.workspaceLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()

	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}

	result, err := s.runner.Reconcile(ctx, cfg)
	recordSyncMetrics(s.logger, workspaceID, cfg.Provider, result, err)
	return result, err
}

// SyncDueConfigs runs a sync for every workspace whose poll interval has
// elapsed, dispatched sequentially in ascending workspace_id (the Store's
// ListConfigs ordering). Dispatch is intentionally not concurrent
// (AC-OFFICE-CONFIG-SYNC-004.4: no run waits on another workspace's lock, but
// a slow workspace does delay later ones in the same tick). Failures are
// recorded on the config row by Runner.Reconcile and logged here; never
// fatal to the tick.
func (s *Service) SyncDueConfigs(ctx context.Context) {
	configs, err := s.store.ListConfigs(ctx)
	if err != nil {
		s.logger.Warn("failed to list config sync configs", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, cfg := range configs {
		if ctx.Err() != nil {
			return
		}
		if !isSyncDue(cfg, now) {
			continue
		}
		if _, err := s.SyncWorkspace(ctx, cfg.WorkspaceID); err != nil {
			s.logger.Warn("periodic config sync failed",
				zap.String("workspace_id", cfg.WorkspaceID), zap.Error(err))
		}
	}
}

func isSyncDue(cfg *Config, now time.Time) bool {
	if !cfg.PollEnabled {
		return false
	}
	if cfg.LastSyncedAt == nil {
		return true
	}
	return now.Sub(*cfg.LastSyncedAt) >= time.Duration(cfg.IntervalSeconds)*time.Second
}
