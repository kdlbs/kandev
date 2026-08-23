package orchestrator

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/workflow/engine"
)

// stepSpecCacheTTL is the correctness floor for cached compiled steps. Step
// writes reach the cache either through workflow_step.created/updated/deleted
// events (the fast path, invalidating within the same process tick) or through
// this TTL (the floor): writer paths that don't publish those events today —
// git-backed workflow sync, step reorder, ClearStepReferences on step delete —
// leave a stale entry for at most one TTL window instead of indefinitely. Same
// defensive-interval reasoning as gitSnapshotCache's persist interval.
const stepSpecCacheTTL = 5 * time.Second

// stepSpecCacheMaxEntries bounds the combined size of both cache maps so a
// long-lived backend with many workflows can't grow it without limit.
const stepSpecCacheMaxEntries = 2048

// posDirection distinguishes LoadNextStep from LoadPreviousStep entries that
// otherwise share a (workflowID, position) key.
type posDirection string

const (
	posDirectionNext posDirection = "next"
	posDirectionPrev posDirection = "prev"
)

type stepSpecCacheEntry struct {
	spec      engine.StepSpec
	expiresAt time.Time
	storedAt  time.Time
}

// stepSpecCache caches engine.CompileStep output for workflowStore's
// LoadStep/LoadNextStep/LoadPreviousStep. StepSpec.Events is a reference type
// (map[Trigger][]Action), so every cached entry is shared across callers; all
// consumers only read it (range/len), never mutate it — see
// workflow_step_cache_test.go and the callers cited in the task plan. A
// cached StepSpec returned by get must never be mutated by callers.
//
// byStep is keyed by stepID (LoadStep). byPos is keyed by
// workflowID+"\x00"+direction+"\x00"+position (LoadNextStep/LoadPreviousStep),
// since those are looked up by position, not step ID.
type stepSpecCache struct {
	mu      sync.RWMutex
	byStep  map[string]stepSpecCacheEntry
	byPos   map[string]stepSpecCacheEntry
	ttl     time.Duration
	maxSize int
	now     func() time.Time

	// sfStep/sfPos coalesce concurrent misses on the same key into one
	// fetch, so N sessions hitting an expired/cold entry in the same instant
	// don't all fall through to the DB — the exact contention this cache
	// exists to remove. Separate groups mirror byStep/byPos: a stepID and a
	// posKey live in disjoint key spaces already (posKey embeds NUL-joined
	// segments a stepID can't contain), but two groups keep that invariant
	// structural rather than relied-upon.
	sfStep singleflight.Group
	sfPos  singleflight.Group
}

func newStepSpecCache() *stepSpecCache {
	return &stepSpecCache{
		byStep:  make(map[string]stepSpecCacheEntry),
		byPos:   make(map[string]stepSpecCacheEntry),
		ttl:     stepSpecCacheTTL,
		maxSize: stepSpecCacheMaxEntries,
		now:     time.Now,
	}
}

func posKey(workflowID string, direction posDirection, position int) string {
	return workflowID + "\x00" + string(direction) + "\x00" + strconv.Itoa(position)
}

func (c *stepSpecCache) getStep(stepID string) (engine.StepSpec, bool) {
	return c.get(c.byStep, stepID)
}

func (c *stepSpecCache) putStep(stepID string, spec engine.StepSpec) {
	c.put(c.byStep, stepID, spec)
}

func (c *stepSpecCache) getPos(workflowID string, direction posDirection, position int) (engine.StepSpec, bool) {
	return c.get(c.byPos, posKey(workflowID, direction, position))
}

func (c *stepSpecCache) putPos(workflowID string, direction posDirection, position int, spec engine.StepSpec) {
	c.put(c.byPos, posKey(workflowID, direction, position), spec)
}

// getOrLoadStep returns stepID's cached spec when fresh; otherwise it runs
// fetch, coalescing concurrent misses on the same stepID via sfStep so only
// one caller reaches the DB. A fetch error is propagated to every waiter and
// never cached, matching getStep/putStep's existing miss contract.
func (c *stepSpecCache) getOrLoadStep(stepID string, fetch func() (engine.StepSpec, error)) (engine.StepSpec, error) {
	if spec, ok := c.getStep(stepID); ok {
		return spec, nil
	}
	v, err, _ := c.sfStep.Do(stepID, func() (interface{}, error) {
		if spec, ok := c.getStep(stepID); ok { // another caller may have filled it
			return spec, nil
		}
		spec, ferr := fetch()
		if ferr != nil {
			return engine.StepSpec{}, ferr
		}
		c.putStep(stepID, spec)
		return spec, nil
	})
	if err != nil {
		return engine.StepSpec{}, err
	}
	return v.(engine.StepSpec), nil
}

// getOrLoadPos is getOrLoadStep for the byPos cache, coalescing concurrent
// misses on the same (workflowID, direction, position) key via sfPos.
func (c *stepSpecCache) getOrLoadPos(workflowID string, direction posDirection, position int, fetch func() (engine.StepSpec, error)) (engine.StepSpec, error) {
	if spec, ok := c.getPos(workflowID, direction, position); ok {
		return spec, nil
	}
	key := posKey(workflowID, direction, position)
	v, err, _ := c.sfPos.Do(key, func() (interface{}, error) {
		if spec, ok := c.getPos(workflowID, direction, position); ok { // another caller may have filled it
			return spec, nil
		}
		spec, ferr := fetch()
		if ferr != nil {
			return engine.StepSpec{}, ferr
		}
		c.putPos(workflowID, direction, position, spec)
		return spec, nil
	})
	if err != nil {
		return engine.StepSpec{}, err
	}
	return v.(engine.StepSpec), nil
}

// get reports a hit only when the key is present and unexpired. m must be
// c.byStep or c.byPos; the map header is passed by value but the underlying
// table is shared, so reads observe concurrent writes safely under the lock.
func (c *stepSpecCache) get(m map[string]stepSpecCacheEntry, key string) (engine.StepSpec, bool) {
	c.mu.RLock()
	entry, ok := m[key]
	c.mu.RUnlock()
	if !ok || c.now().After(entry.expiresAt) {
		return engine.StepSpec{}, false
	}
	return entry.spec, true
}

func (c *stepSpecCache) put(m map[string]stepSpecCacheEntry, key string, spec engine.StepSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if _, exists := m[key]; !exists && c.maxSize > 0 && len(c.byStep)+len(c.byPos) >= c.maxSize {
		c.evictOldestLocked()
	}
	m[key] = stepSpecCacheEntry{spec: spec, expiresAt: now.Add(c.ttl), storedAt: now}
}

// evictOldestLocked drops the single oldest entry (by storedAt) across both
// maps. Caller must hold c.mu. O(n) over the cache; only invoked when the
// cache is full, which is rare in practice — workflows hold a handful of
// steps and the combined cap is generous.
func (c *stepSpecCache) evictOldestLocked() {
	var oldestMap map[string]stepSpecCacheEntry
	var oldestKey string
	var oldestAt time.Time
	consider := func(m map[string]stepSpecCacheEntry) {
		for k, entry := range m {
			if oldestMap == nil || entry.storedAt.Before(oldestAt) {
				oldestMap, oldestKey, oldestAt = m, k, entry.storedAt
			}
		}
	}
	consider(c.byStep)
	consider(c.byPos)
	if oldestMap != nil {
		delete(oldestMap, oldestKey)
	}
}

// invalidate drops stepID's byStep entry and every cached entry belonging to
// workflowID. A step edit changes that step's own spec (byStep) and can shift
// Next/Prev results for its whole workflow (a position or move_to_step change),
// so the coarser workflow-scoped invalidation is required there — see the
// task plan's finding on ReorderSteps rewriting Position across every step.
func (c *stepSpecCache) invalidate(workflowID, stepID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stepID != "" {
		delete(c.byStep, stepID)
	}
	if workflowID != "" {
		for k, entry := range c.byStep {
			if entry.spec.WorkflowID == workflowID {
				delete(c.byStep, k)
			}
		}
		prefix := workflowID + "\x00"
		for k := range c.byPos {
			if strings.HasPrefix(k, prefix) {
				delete(c.byPos, k)
			}
		}
	}
}
