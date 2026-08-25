package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// blockingStepGetter blocks the first GetStep call on release, closing
// entered exactly once so a concurrent-miss test can deterministically wait
// for the fetch to actually start before releasing it. If singleflight
// coalescing ever regresses and GetStep is invoked a second time while the
// first is still blocked, the second close(entered) panics, failing loudly.
type blockingStepGetter struct {
	*countingStepGetter
	entered chan struct{}
	release chan struct{}
}

func newBlockingStepGetter() *blockingStepGetter {
	return &blockingStepGetter{
		countingStepGetter: newCountingStepGetter(),
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
}

func (b *blockingStepGetter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	close(b.entered)
	<-b.release
	return b.countingStepGetter.GetStep(ctx, stepID)
}

// erroringStepGetter always fails GetStep, mirroring the real repository's
// error return for a missing step (unlike mockStepGetter, which returns
// nil, nil and would make LoadStep call engine.CompileStep(nil)).
type erroringStepGetter struct {
	*mockStepGetter
	err error
}

func (e *erroringStepGetter) GetStep(_ context.Context, _ string) (*wfmodels.WorkflowStep, error) {
	return nil, e.err
}

// countingStepGetter wraps mockStepGetter and counts calls per method so
// tests can assert the cache actually avoided a repeated getter call.
type countingStepGetter struct {
	*mockStepGetter
	mu               sync.Mutex
	getStepCalls     int
	getNextCalls     int
	getPreviousCalls int
}

func newCountingStepGetter() *countingStepGetter {
	return &countingStepGetter{mockStepGetter: newMockStepGetter()}
}

func (c *countingStepGetter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	c.mu.Lock()
	c.getStepCalls++
	c.mu.Unlock()
	return c.mockStepGetter.GetStep(ctx, stepID)
}

func (c *countingStepGetter) GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	c.mu.Lock()
	c.getNextCalls++
	c.mu.Unlock()
	return c.mockStepGetter.GetNextStepByPosition(ctx, workflowID, currentPosition)
}

func (c *countingStepGetter) GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	c.mu.Lock()
	c.getPreviousCalls++
	c.mu.Unlock()
	return c.mockStepGetter.GetPreviousStepByPosition(ctx, workflowID, currentPosition)
}

func stepEvent(eventType, stepID, workflowID string) *bus.Event {
	return bus.NewEvent(eventType, "test", map[string]interface{}{
		"step": map[string]interface{}{
			"id":          stepID,
			"workflow_id": workflowID,
		},
	})
}

// 1. A cache hit must not re-invoke the getter.
func TestWorkflowStore_LoadStep_CachesAcrossCalls(t *testing.T) {
	ctx := context.Background()
	stepGetter := newCountingStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Planning", Position: 0}

	store := newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	var last engine.StepSpec
	for i := 0; i < 4; i++ {
		spec, err := store.LoadStep(ctx, "wf1", "step1")
		if err != nil {
			t.Fatalf("LoadStep call %d failed: %v", i, err)
		}
		last = spec
	}

	if last.ID != "step1" {
		t.Fatalf("expected spec for step1, got %q", last.ID)
	}
	stepGetter.mu.Lock()
	calls := stepGetter.getStepCalls
	stepGetter.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected exactly 1 getter call across 4 LoadStep calls, got %d", calls)
	}
}

// 2. Expiry after TTL forces a refetch, via an injected clock (no sleeps).
func TestWorkflowStore_LoadStep_ExpiresAfterTTL(t *testing.T) {
	ctx := context.Background()
	stepGetter := newCountingStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Planning"}

	store := newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())
	now := time.Now()
	store.stepCache.now = func() time.Time { return now }

	if _, err := store.LoadStep(ctx, "wf1", "step1"); err != nil {
		t.Fatalf("first LoadStep failed: %v", err)
	}
	// Still within TTL: no refetch.
	now = now.Add(stepSpecCacheTTL - time.Millisecond)
	if _, err := store.LoadStep(ctx, "wf1", "step1"); err != nil {
		t.Fatalf("second LoadStep failed: %v", err)
	}
	// Past TTL: must refetch.
	now = now.Add(2 * time.Millisecond)
	if _, err := store.LoadStep(ctx, "wf1", "step1"); err != nil {
		t.Fatalf("third LoadStep failed: %v", err)
	}

	stepGetter.mu.Lock()
	calls := stepGetter.getStepCalls
	stepGetter.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected 2 getter calls (initial + post-expiry refetch), got %d", calls)
	}
}

// 3. workflow_step.updated for a step drops that step's byStep entry.
func TestStepSpecCache_UpdatedEventDropsStepEntry(t *testing.T) {
	ctx := context.Background()
	stepGetter := newCountingStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Planning"}

	svc := &Service{logger: testLogger()}
	svc.workflowStore = newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	if _, err := svc.workflowStore.LoadStep(ctx, "wf1", "step1"); err != nil {
		t.Fatalf("LoadStep failed: %v", err)
	}

	if err := svc.handleWorkflowStepCacheInvalidation(ctx, stepEvent(events.WorkflowStepUpdated, "step1", "wf1")); err != nil {
		t.Fatalf("handleWorkflowStepCacheInvalidation failed: %v", err)
	}

	if _, err := svc.workflowStore.LoadStep(ctx, "wf1", "step1"); err != nil {
		t.Fatalf("LoadStep after invalidation failed: %v", err)
	}

	stepGetter.mu.Lock()
	calls := stepGetter.getStepCalls
	stepGetter.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected a refetch after invalidation (2 calls), got %d", calls)
	}
}

// 4. workflow_step.updated drops the Next/Prev entries of the *same*
// workflow and leaves other workflows' entries untouched.
func TestStepSpecCache_UpdatedEventDropsSameWorkflowPositionEntriesOnly(t *testing.T) {
	ctx := context.Background()
	stepGetter := newCountingStepGetter()
	stepGetter.steps["wf1-step1"] = &wfmodels.WorkflowStep{ID: "wf1-step1", WorkflowID: "wf1", Position: 0}
	stepGetter.steps["wf1-step2"] = &wfmodels.WorkflowStep{ID: "wf1-step2", WorkflowID: "wf1", Position: 1}
	stepGetter.steps["wf2-step1"] = &wfmodels.WorkflowStep{ID: "wf2-step1", WorkflowID: "wf2", Position: 0}
	stepGetter.steps["wf2-step2"] = &wfmodels.WorkflowStep{ID: "wf2-step2", WorkflowID: "wf2", Position: 1}

	svc := &Service{logger: testLogger()}
	svc.workflowStore = newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	if _, err := svc.workflowStore.LoadNextStep(ctx, "wf1", 0); err != nil {
		t.Fatalf("LoadNextStep(wf1) failed: %v", err)
	}
	if _, err := svc.workflowStore.LoadNextStep(ctx, "wf2", 0); err != nil {
		t.Fatalf("LoadNextStep(wf2) failed: %v", err)
	}

	if err := svc.handleWorkflowStepCacheInvalidation(ctx, stepEvent(events.WorkflowStepUpdated, "wf1-step2", "wf1")); err != nil {
		t.Fatalf("handleWorkflowStepCacheInvalidation failed: %v", err)
	}

	if _, err := svc.workflowStore.LoadNextStep(ctx, "wf1", 0); err != nil {
		t.Fatalf("LoadNextStep(wf1) after invalidation failed: %v", err)
	}
	if _, err := svc.workflowStore.LoadNextStep(ctx, "wf2", 0); err != nil {
		t.Fatalf("LoadNextStep(wf2) after invalidation failed: %v", err)
	}

	stepGetter.mu.Lock()
	nextCalls := stepGetter.getNextCalls
	stepGetter.mu.Unlock()
	// wf1: 2 (initial + refetch after invalidation). wf2: 1 (still cached).
	if nextCalls != 3 {
		t.Fatalf("expected 3 GetNextStepByPosition calls (wf1 refetched, wf2 still cached), got %d", nextCalls)
	}
}

// 5. A deleted step can have references cleared from other steps before its
// event is published. The event must therefore drop every direct entry for
// the workflow, not only the deleted step's entry.
func TestStepSpecCache_DeletedEventDropsAllWorkflowStepEntries(t *testing.T) {
	cache := newStepSpecCache()
	cache.putStep("source", engine.StepSpec{ID: "source", WorkflowID: "wf1"})
	cache.putStep("deleted", engine.StepSpec{ID: "deleted", WorkflowID: "wf1"})
	cache.putStep("other-workflow", engine.StepSpec{ID: "other-workflow", WorkflowID: "wf2"})

	cache.invalidate("wf1", "deleted")

	if _, ok := cache.getStep("source"); ok {
		t.Fatal("expected workflow-wide invalidation to drop the cached source step")
	}
	if _, ok := cache.getStep("deleted"); ok {
		t.Fatal("expected deleted step entry to be removed")
	}
	if _, ok := cache.getStep("other-workflow"); !ok {
		t.Fatal("expected entries for other workflows to remain cached")
	}
}

// 6. Both .deleted and .created invalidate (created covers the demoted-
// start-step fan-out, which publishes .updated for demoted steps but the
// cache must also tolerate a .created arriving for an unrelated step ID).
func TestStepSpecCache_CreatedAndDeletedEventsInvalidate(t *testing.T) {
	for _, eventType := range []string{events.WorkflowStepCreated, events.WorkflowStepDeleted} {
		t.Run(eventType, func(t *testing.T) {
			ctx := context.Background()
			stepGetter := newCountingStepGetter()
			stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1"}

			svc := &Service{logger: testLogger()}
			svc.workflowStore = newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

			if _, err := svc.workflowStore.LoadStep(ctx, "wf1", "step1"); err != nil {
				t.Fatalf("LoadStep failed: %v", err)
			}
			if err := svc.handleWorkflowStepCacheInvalidation(ctx, stepEvent(eventType, "step1", "wf1")); err != nil {
				t.Fatalf("handleWorkflowStepCacheInvalidation failed: %v", err)
			}
			if _, err := svc.workflowStore.LoadStep(ctx, "wf1", "step1"); err != nil {
				t.Fatalf("LoadStep after invalidation failed: %v", err)
			}

			stepGetter.mu.Lock()
			calls := stepGetter.getStepCalls
			stepGetter.mu.Unlock()
			if calls != 2 {
				t.Fatalf("expected a refetch after %s (2 calls), got %d", eventType, calls)
			}
		})
	}
}

// 7. A getter error is returned and not cached.
func TestWorkflowStore_LoadStep_ErrorNotCached(t *testing.T) {
	ctx := context.Background()
	stepGetter := &erroringStepGetter{mockStepGetter: newMockStepGetter(), err: errors.New("boom")}

	store := newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	if _, err := store.LoadStep(ctx, "wf1", "step1"); err == nil {
		t.Fatal("expected an error for a missing step")
	}
	if _, ok := store.stepCache.getStep("step1"); ok {
		t.Fatal("an error result must not be cached")
	}
	if _, err := store.LoadStep(ctx, "wf1", "step1"); err == nil {
		t.Fatal("expected the error again on the second call")
	}
}

// 8. LoadNextStep's step == nil case still returns today's "no next step"
// error, uncached.
func TestWorkflowStore_LoadNextStep_NoNextStepNotCached(t *testing.T) {
	ctx := context.Background()
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Position: 0}

	store := newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	_, err := store.LoadNextStep(ctx, "wf1", 0)
	if err == nil {
		t.Fatal("expected an error when there is no next step")
	}
	if _, ok := store.stepCache.getPos("wf1", posDirectionNext, 0); ok {
		t.Fatal("a nil-step result must not be cached")
	}
}

// 9. Eviction at maxEntries: filling the cache past its bound evicts the
// oldest entry rather than growing without limit.
func TestStepSpecCache_EvictsOldestAtMaxEntries(t *testing.T) {
	cache := newStepSpecCache()
	cache.maxSize = 2
	now := time.Now()
	cache.now = func() time.Time { return now }

	cache.putStep("step1", engine.StepSpec{ID: "step1"})
	now = now.Add(time.Millisecond)
	cache.putStep("step2", engine.StepSpec{ID: "step2"})
	now = now.Add(time.Millisecond)
	cache.putStep("step3", engine.StepSpec{ID: "step3"})

	if _, ok := cache.getStep("step1"); ok {
		t.Fatal("expected the oldest entry (step1) to have been evicted")
	}
	if _, ok := cache.getStep("step2"); !ok {
		t.Fatal("expected step2 to still be cached")
	}
	if _, ok := cache.getStep("step3"); !ok {
		t.Fatal("expected step3 to still be cached")
	}
}

// 10. Concurrent get/put/invalidate must not race (run with -race).
func TestStepSpecCache_ConcurrentAccess(t *testing.T) {
	cache := newStepSpecCache()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			cache.putStep("step1", engine.StepSpec{ID: "step1"})
		}(i)
		go func(i int) {
			defer wg.Done()
			cache.getStep("step1")
		}(i)
		go func(i int) {
			defer wg.Done()
			cache.invalidate("wf1", "step1")
		}(i)
	}
	wg.Wait()
}

// 11. LoadPreviousStep caches across calls and is invalidated by a workflow
// step event, exercising the byPos "prev" direction — the rest of this suite
// only exercises "next" via LoadNextStep.
func TestWorkflowStore_LoadPreviousStep_CachesAndInvalidates(t *testing.T) {
	ctx := context.Background()
	stepGetter := newCountingStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Position: 0}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}

	svc := &Service{logger: testLogger()}
	svc.workflowStore = newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	spec, err := svc.workflowStore.LoadPreviousStep(ctx, "wf1", 1)
	if err != nil {
		t.Fatalf("first LoadPreviousStep failed: %v", err)
	}
	if spec.ID != "step1" {
		t.Fatalf("expected previous step %q, got %q", "step1", spec.ID)
	}
	if _, err := svc.workflowStore.LoadPreviousStep(ctx, "wf1", 1); err != nil {
		t.Fatalf("second LoadPreviousStep failed: %v", err)
	}

	stepGetter.mu.Lock()
	calls := stepGetter.getPreviousCalls
	stepGetter.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected exactly 1 getter call across 2 LoadPreviousStep calls, got %d", calls)
	}

	if err := svc.handleWorkflowStepCacheInvalidation(ctx, stepEvent(events.WorkflowStepUpdated, "step1", "wf1")); err != nil {
		t.Fatalf("handleWorkflowStepCacheInvalidation failed: %v", err)
	}

	if _, err := svc.workflowStore.LoadPreviousStep(ctx, "wf1", 1); err != nil {
		t.Fatalf("LoadPreviousStep after invalidation failed: %v", err)
	}

	stepGetter.mu.Lock()
	calls = stepGetter.getPreviousCalls
	stepGetter.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected a refetch after invalidation (2 calls), got %d", calls)
	}
}

// 12. subscribeWorkflowStepCacheEvents actually registers the handler on the
// bus and reacts to a delivered event, rather than trusting that
// handleWorkflowStepCacheInvalidation (called directly by tests 3-5) is ever
// wired up. A wrong event-type constant or a missing Subscribe call would
// leave every LoadStep call here served from a stale cache entry.
func TestSubscribeWorkflowStepCacheEvents_DeliversThroughBus(t *testing.T) {
	ctx := context.Background()
	stepGetter := newCountingStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1"}

	eventBus := bus.NewMemoryEventBus(testLogger())
	svc := &Service{logger: testLogger(), eventBus: eventBus}
	svc.workflowStore = newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())
	svc.subscribeWorkflowStepCacheEvents()

	for _, eventType := range []string{events.WorkflowStepCreated, events.WorkflowStepUpdated, events.WorkflowStepDeleted} {
		if _, err := svc.workflowStore.LoadStep(ctx, "wf1", "step1"); err != nil {
			t.Fatalf("LoadStep before %s failed: %v", eventType, err)
		}
		if err := eventBus.Publish(ctx, eventType, stepEvent(eventType, "step1", "wf1")); err != nil {
			t.Fatalf("publish %s failed: %v", eventType, err)
		}
	}
	if _, err := svc.workflowStore.LoadStep(ctx, "wf1", "step1"); err != nil {
		t.Fatalf("final LoadStep failed: %v", err)
	}

	stepGetter.mu.Lock()
	calls := stepGetter.getStepCalls
	stepGetter.mu.Unlock()
	// One fetch per LoadStep call (4 total): each of the 3 bus-delivered
	// events must invalidate the entry the prior LoadStep just populated, or
	// this would stay at 1.
	if calls != 4 {
		t.Fatalf("expected 4 getter calls (each bus-delivered event invalidating the prior load), got %d", calls)
	}
}

// 13. Concurrent misses on the same key coalesce into one getter call via
// stepCache's singleflight group, rather than every waiter independently
// re-reading and recompiling the step.
func TestWorkflowStore_LoadStep_CoalescesConcurrentMisses(t *testing.T) {
	ctx := context.Background()
	stepGetter := newBlockingStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Planning"}

	store := newWorkflowStore(nil, stepGetter, nil, noopPublisher, testLogger())

	const n = 10
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := store.LoadStep(ctx, "wf1", "step1")
			errCh <- err
		}()
	}

	<-stepGetter.entered      // the leader has reached the DB read
	close(stepGetter.release) // let it (and only it) complete the fetch
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("LoadStep returned error: %v", err)
		}
	}

	stepGetter.mu.Lock()
	calls := stepGetter.getStepCalls
	stepGetter.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected exactly 1 getter call across %d concurrent LoadStep calls, got %d", n, calls)
	}
}
