package orchestrator

import (
	"context"
	"sync"
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

	if cache, ok := ctx.Value(workflowMetaCacheKey{}).(*workflowMetaCache); ok {
		cache.mu.Lock()
		if entry, hit := cache.byID[workflowID]; hit {
			cache.mu.Unlock()
			return entry.meta, entry.err
		}
		cache.mu.Unlock()

		meta, err := s.workflowStepGetter.GetWorkflowMeta(ctx, workflowID)
		cache.mu.Lock()
		// First writer wins if two goroutines raced the same id.
		if entry, hit := cache.byID[workflowID]; hit {
			cache.mu.Unlock()
			return entry.meta, entry.err
		}
		cache.byID[workflowID] = workflowMetaCacheEntry{meta: meta, err: err}
		cache.mu.Unlock()
		return meta, err
	}

	return s.workflowStepGetter.GetWorkflowMeta(ctx, workflowID)
}
