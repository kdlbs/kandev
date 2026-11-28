package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
)

var errNativeRouteNotCurrent = errors.New("native owner is not current")

// Physical preparations and native root callers share the existing liveness
// registry. Only native roots carry metadata; neither kind is a waiting queue.
type inferenceWorker struct {
	native *orchestrator.NativeCapacityWaiter
}

func (b *sharedInferenceConsumer) registerNativeWorker(route orchestrator.NativeInferenceRoute, at time.Time) error {
	b.workerMu.Lock()
	defer b.workerMu.Unlock()
	if b.workers == nil {
		b.workers = make(map[string]inferenceWorker)
	}
	if previous, exists := b.workers[route.ID]; exists {
		if previous.native == nil || previous.native.Route != route {
			return errors.New("live native owner changed")
		}
		return nil
	}
	b.workers[route.ID] = inferenceWorker{native: &orchestrator.NativeCapacityWaiter{Route: route, QueuedAt: at}}
	return nil
}

func (b *sharedInferenceConsumer) nativeWorkers() []orchestrator.NativeCapacityWaiter {
	b.workerMu.Lock()
	defer b.workerMu.Unlock()
	result := make([]orchestrator.NativeCapacityWaiter, 0)
	for _, worker := range b.workers {
		if worker.native != nil {
			result = append(result, *worker.native)
		}
	}
	return result
}

func (o *nativeRouteCapacity) queuedAt(ctx context.Context, route orchestrator.NativeInferenceRoute) (time.Time, error) {
	if route.CallID != "" {
		call, err := o.calls.GetCallByID(ctx, route.CallID)
		if err != nil {
			return time.Time{}, err
		}
		if call == nil {
			return time.Time{}, errNativeRouteNotCurrent
		}
		return call.CreatedAt, nil
	}
	run, err := o.tasks.GetTaskReviewRun(ctx, route.ReviewRunID)
	if err != nil {
		return time.Time{}, err
	}
	if run == nil {
		return time.Time{}, errNativeRouteNotCurrent
	}
	return run.CreatedAt, nil
}

func (o *nativeRouteCapacity) waiters(ctx context.Context) ([]orchestrator.NativeCapacityWaiter, error) {
	if o.consumer == nil {
		return nil, errors.New("native liveness registry unavailable")
	}
	var result []orchestrator.NativeCapacityWaiter
	for _, live := range o.consumer.nativeWorkers() {
		route, err := o.route(ctx, live.Route.ID, live.Route.LogicalProfileID)
		if errors.Is(err, errNativeRouteNotCurrent) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if route != live.Route {
			continue
		}
		result = append(result, live)
	}
	return result, nil
}

func (o *nativeRouteCapacity) release(id string) {
	if o.consumer != nil {
		o.consumer.markWorker(id, false)
	}
	if o.capacity != nil {
		o.capacity.ReleaseNativeRouteCapacity(id)
	}
}
