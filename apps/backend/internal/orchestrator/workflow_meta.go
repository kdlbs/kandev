package orchestrator

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"
)

// workflowMetaCacheKey is the context key for a request-scoped WorkflowMeta cache.
// Step-entry paths seed the cache once so profile resolution and prompt build
// share a single GetWorkflowMeta provider read.
type workflowMetaCacheKey struct{}

type workflowMetaCacheEntry struct {
	meta WorkflowMeta
	err  error
}

type workflowMetaCache struct {
	mu   sync.Mutex
	byID map[string]workflowMetaCacheEntry
	// group coalesces concurrent same-ID loads within one request so dual
	// consumers (and any accidental parallel fan-out) share one provider call.
	group singleflight.Group
}

// withWorkflowMetaCache returns a context carrying a mutable per-request cache
// for GetWorkflowMeta results. Nested calls reuse the existing cache.
func withWorkflowMetaCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(workflowMetaCacheKey{}).(*workflowMetaCache); ok {
		return ctx
	}
	return context.WithValue(ctx, workflowMetaCacheKey{}, &workflowMetaCache{
		byID: make(map[string]workflowMetaCacheEntry),
	})
}

// getWorkflowMeta loads workflow agent-profile id + prompt via WorkflowStepGetter,
// caching the result on ctx when seeded with withWorkflowMetaCache.
func (s *Service) getWorkflowMeta(ctx context.Context, workflowID string) (WorkflowMeta, error) {
	if s.workflowStepGetter == nil || workflowID == "" {
		return WorkflowMeta{}, nil
	}

	cache, ok := ctx.Value(workflowMetaCacheKey{}).(*workflowMetaCache)
	if !ok {
		return s.workflowStepGetter.GetWorkflowMeta(ctx, workflowID)
	}

	cache.mu.Lock()
	if entry, hit := cache.byID[workflowID]; hit {
		cache.mu.Unlock()
		return entry.meta, entry.err
	}
	cache.mu.Unlock()

	// Coalesce concurrent same-ID loads; first writer still wins on the map.
	v, _, _ := cache.group.Do(workflowID, func() (any, error) {
		cache.mu.Lock()
		if entry, hit := cache.byID[workflowID]; hit {
			cache.mu.Unlock()
			return entry, nil
		}
		cache.mu.Unlock()

		meta, loadErr := s.workflowStepGetter.GetWorkflowMeta(ctx, workflowID)
		entry := workflowMetaCacheEntry{meta: meta, err: loadErr}
		cache.mu.Lock()
		if existing, hit := cache.byID[workflowID]; hit {
			cache.mu.Unlock()
			return existing, nil
		}
		cache.byID[workflowID] = entry
		cache.mu.Unlock()
		return entry, nil
	})
	entry := v.(workflowMetaCacheEntry)
	return entry.meta, entry.err
}
