package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const registryCacheMaxAge = time.Hour

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
		select {
		case <-call.done:
			return call.result, call.err
		case <-ctx.Done():
			return SyncResult{}, ctx.Err()
		}
	}
	call := &refreshCall{done: make(chan struct{})}
	s.call = call
	s.mu.Unlock()

	call.result, call.err = s.refresh(ctx, incremental)
	s.mu.Lock()
	s.call = nil
	close(call.done)
	s.mu.Unlock()
	return call.result, call.err
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
	if s.client == nil || s.store == nil {
		return SyncResult{}, ErrMarketplaceCatalogUnavailable
	}
	state, err := s.currentState(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	now := s.now().UTC()
	state.LastAttemptAt = now
	_ = s.store.SaveMCPRegistrySyncState(ctx, state)
	options := ListOptions{IncludeDeleted: true}
	if incremental && !state.LastSuccessfulAt.IsZero() {
		updatedSince := state.LastSuccessfulAt
		options.UpdatedSince = &updatedSince
	}
	entries, err := s.client.FetchAll(ctx, options)
	if err != nil {
		return s.failedRefresh(ctx, state, err, now)
	}
	if options.UpdatedSince != nil {
		err = s.store.UpsertMCPRegistryEntries(ctx, entries)
	} else {
		err = s.store.ReplaceMCPRegistryEntries(ctx, entries)
	}
	if err != nil {
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
	case errors.Is(err, context.DeadlineExceeded):
		return "registry request timed out"
	default:
		return "registry refresh failed"
	}
}

// ErrSyncStateNotFound is returned by persistent stores before the first refresh.
var ErrSyncStateNotFound = errors.New("registry sync state not found")
