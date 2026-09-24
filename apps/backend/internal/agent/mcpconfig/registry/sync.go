package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	registryCacheMaxAge    = time.Hour
	registryRefreshTimeout = 2 * time.Minute
)

// CacheStore persists the public Registry cache and its health state.
type CacheStore interface {
	ListMCPRegistryEntries(context.Context, string) ([]Entry, error)
	GetMCPRegistryEntry(context.Context, string) (*Entry, error)
	ReplaceMCPRegistryEntries(context.Context, []Entry) error
	UpsertMCPRegistryEntries(context.Context, []Entry) error
	GetMCPRegistrySyncState(context.Context) (SyncState, error)
	SaveMCPRegistrySyncState(context.Context, SyncState) error
}

// SyncResult describes the cache used by a refresh attempt.
type SyncResult struct {
	Entries          []Entry
	Stale            bool
	Degraded         bool
	LastSuccessfulAt time.Time
}

type refreshCall struct {
	done   chan struct{}
	result SyncResult
	err    error
}

// SyncService owns refresh single-flight and last-good-cache behavior.
type SyncService struct {
	client *Client
	store  CacheStore
	now    func() time.Time

	mu   sync.Mutex
	call *refreshCall
}

func NewSyncService(client *Client, store CacheStore) *SyncService {
	return &SyncService{client: client, store: store, now: time.Now}
}

func (s *SyncService) Refresh(ctx context.Context, incremental bool) (SyncResult, error) {
	s.mu.Lock()
	if s.call != nil {
		call := s.call
		s.mu.Unlock()
		return waitForRefresh(ctx, call)
	}
	call := &refreshCall{done: make(chan struct{})}
	s.call = call
	s.mu.Unlock()

	go func() {
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), registryRefreshTimeout)
		defer cancel()
		call.result, call.err = s.refresh(refreshCtx, incremental)
		s.mu.Lock()
		s.call = nil
		close(call.done)
		s.mu.Unlock()
	}()
	return waitForRefresh(ctx, call)
}

func waitForRefresh(ctx context.Context, call *refreshCall) (SyncResult, error) {
	select {
	case <-call.done:
		return call.result, call.err
	case <-ctx.Done():
		return SyncResult{}, ctx.Err()
	}
}

func (s *SyncService) Cached(ctx context.Context, query string) ([]Entry, SyncState, error) {
	if s.store == nil {
		return nil, SyncState{}, ErrMarketplaceCatalogUnavailable
	}
	entries, err := s.store.ListMCPRegistryEntries(ctx, strings.TrimSpace(query))
	if err != nil {
		return nil, SyncState{}, err
	}
	state, err := s.store.GetMCPRegistrySyncState(ctx)
	if errors.Is(err, ErrSyncStateNotFound) {
		return entries, SyncState{}, nil
	}
	return entries, state, err
}

func (s *SyncService) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = registryCacheMaxAge
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = s.Refresh(ctx, true)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *SyncService) refresh(ctx context.Context, incremental bool) (SyncResult, error) {
	if !s.ready() {
		return SyncResult{}, ErrMarketplaceCatalogUnavailable
	}
	state, err := s.currentState(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	now := s.now().UTC()
	if shouldUseCachedRegistryState(state, now) {
		return s.cachedResult(ctx, state, now)
	}
	state.LastAttemptAt = now
	if err := s.store.SaveMCPRegistrySyncState(ctx, state); err != nil {
		return SyncResult{}, err
	}
	options := registryRefreshOptions(incremental, state)
	entries, err := s.client.FetchAll(ctx, options)
	if err != nil {
		return s.failedRefresh(ctx, state, err, now)
	}
	if err := s.storeEntries(ctx, options, entries); err != nil {
		return s.failedRefresh(ctx, state, err, now)
	}
	state.LastSuccessfulAt = now
	state.UpdatedSince = now
	state.Degraded = false
	state.LastError = ""
	if err := s.store.SaveMCPRegistrySyncState(ctx, state); err != nil {
		return SyncResult{}, err
	}
	cached, err := s.store.ListMCPRegistryEntries(ctx, "")
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Entries: cached, LastSuccessfulAt: state.LastSuccessfulAt}, nil
}

func (s *SyncService) ready() bool {
	return s.client != nil && s.store != nil
}

func shouldUseCachedRegistryState(state SyncState, now time.Time) bool {
	return !state.LastAttemptAt.IsZero() && now.Sub(state.LastAttemptAt) < registryCacheMaxAge
}

func (s *SyncService) cachedResult(ctx context.Context, state SyncState, now time.Time) (SyncResult, error) {
	entries, err := s.store.ListMCPRegistryEntries(ctx, "")
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		Entries: entries, Stale: state.LastSuccessfulAt.IsZero() || now.Sub(state.LastSuccessfulAt) > registryCacheMaxAge,
		Degraded: state.Degraded, LastSuccessfulAt: state.LastSuccessfulAt,
	}, nil
}

func registryRefreshOptions(incremental bool, state SyncState) ListOptions {
	options := ListOptions{IncludeDeleted: true}
	if incremental && !state.LastSuccessfulAt.IsZero() {
		updatedSince := state.LastSuccessfulAt
		options.UpdatedSince = &updatedSince
	}
	return options
}

func (s *SyncService) storeEntries(ctx context.Context, options ListOptions, entries []Entry) error {
	if options.UpdatedSince != nil {
		return s.store.UpsertMCPRegistryEntries(ctx, entries)
	}
	return s.store.ReplaceMCPRegistryEntries(ctx, entries)
}

func (s *SyncService) currentState(ctx context.Context) (SyncState, error) {
	state, err := s.store.GetMCPRegistrySyncState(ctx)
	if errors.Is(err, ErrSyncStateNotFound) {
		return SyncState{}, nil
	}
	return state, err
}

func (s *SyncService) failedRefresh(ctx context.Context, state SyncState, refreshErr error, now time.Time) (SyncResult, error) {
	state.LastAttemptAt = now
	state.Degraded = true
	state.LastError = sanitizeRegistryError(refreshErr)
	if err := s.store.SaveMCPRegistrySyncState(ctx, state); err != nil {
		return SyncResult{}, err
	}
	entries, err := s.store.ListMCPRegistryEntries(ctx, "")
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Entries: entries, Stale: true, Degraded: true, LastSuccessfulAt: state.LastSuccessfulAt}, refreshErr
}

func sanitizeRegistryError(err error) string {
	var statusErr *RegistryHTTPError
	switch {
	case errors.As(err, &statusErr):
		return fmt.Sprintf("registry returned HTTP %d", statusErr.StatusCode)
	case errors.Is(err, ErrRegistryResponseTooLarge):
		return "registry response was too large"
	case errors.Is(err, ErrRegistryTotalResponseTooLarge):
		return "registry response aggregate was too large"
	case errors.Is(err, ErrRegistryEntriesTooMany):
		return "registry returned too many entries"
	case errors.Is(err, context.DeadlineExceeded):
		return "registry request timed out"
	default:
		return "registry refresh failed"
	}
}

// ErrSyncStateNotFound is returned by persistent stores before the first refresh.
var ErrSyncStateNotFound = errors.New("registry sync state not found")
