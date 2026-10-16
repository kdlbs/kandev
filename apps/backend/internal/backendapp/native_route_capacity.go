package backendapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
)

type nativeRouteIdentityReader interface {
	reviewInferenceIdentityReader
}
type nativeCallIdentityReader interface {
	GetCallByID(context.Context, string) (*utilitymodels.UtilityAgentCall, error)
}

type nativeRouteCapacity struct {
	capacity *orchestrator.Service
	consumer *sharedInferenceConsumer
	tasks    nativeRouteIdentityReader
	calls    nativeCallIdentityReader
}

func wireNativeRouteCapacity(resolver *agentruntime.ProfileExecutionResolver, capacity *orchestrator.Service,
	consumer *sharedInferenceConsumer, tasks nativeRouteIdentityReader, calls nativeCallIdentityReader,
) {
	if resolver == nil {
		return
	}
	owner := &nativeRouteCapacity{capacity: capacity, consumer: consumer, tasks: tasks, calls: calls}
	resolver.SetNativeRouteAdmission(owner.admit, capacity.ReleaseNativeRouteCapacity)
}

func (o *nativeRouteCapacity) route(ctx context.Context, id, profileID string) (orchestrator.NativeInferenceRoute, error) {
	route := orchestrator.NativeInferenceRoute{ID: id, LogicalProfileID: profileID}
	var err error
	switch {
	case strings.HasPrefix(id, "native:utility-call:"):
		route, err = o.callRoute(ctx, route)
	case strings.HasPrefix(id, "native:review-run:"):
		route, err = o.reviewRoute(ctx, route)
	default:
		return route, errors.New("unsupported native inference owner")
	}
	if err != nil {
		return route, err
	}
	return o.verifySession(ctx, route)
}

func (o *nativeRouteCapacity) callRoute(ctx context.Context, route orchestrator.NativeInferenceRoute) (orchestrator.NativeInferenceRoute, error) {
	if o.calls == nil {
		return route, errors.New("utility owner reader unavailable")
	}
	call, err := o.calls.GetCallByID(ctx, strings.TrimPrefix(route.ID, "native:utility-call:"))
	if err != nil {
		return route, err
	}
	if call == nil || call.ID == "" || call.Status != "pending" || call.AgentProfileID != route.LogicalProfileID {
		return route, errors.New("utility owner is not current")
	}
	route.CallID, route.SessionID = call.ID, call.SessionID
	return route, nil
}

func (o *nativeRouteCapacity) reviewRoute(ctx context.Context, route orchestrator.NativeInferenceRoute) (orchestrator.NativeInferenceRoute, error) {
	if o.tasks == nil {
		return route, errors.New("review owner reader unavailable")
	}
	run, err := o.tasks.GetTaskReviewRun(ctx, strings.TrimPrefix(route.ID, "native:review-run:"))
	if err != nil {
		return route, err
	}
	if run == nil || run.ID == "" || run.TaskID == "" || run.SessionID == "" || (run.Status != taskmodels.ReviewRunPending && run.Status != taskmodels.ReviewRunRunning) {
		return route, errors.New("review owner is not current")
	}
	route.ReviewRunID, route.TaskID, route.SessionID = run.ID, run.TaskID, run.SessionID
	return route, nil
}

func (o *nativeRouteCapacity) verifySession(ctx context.Context, route orchestrator.NativeInferenceRoute) (orchestrator.NativeInferenceRoute, error) {
	if route.SessionID == "" {
		return route, nil
	}
	if o.tasks == nil {
		return route, errors.New("native session reader unavailable")
	}
	session, err := o.tasks.GetTaskSession(ctx, route.SessionID)
	if err != nil {
		return route, err
	}
	if session == nil || session.ID != route.SessionID || session.TaskID == "" || (route.TaskID != "" && route.TaskID != session.TaskID) {
		return route, errors.New("native owner and session do not match")
	}
	route.TaskID = session.TaskID
	return route, nil
}

func (o *nativeRouteCapacity) admit(ctx context.Context, id, profileID string) error {
	route, err := o.route(ctx, id, profileID)
	if err != nil {
		return &agentruntime.InferenceCapacityError{Reason: "native_owner_unavailable", Cause: err}
	}
	if o.consumer == nil || o.capacity == nil {
		return &agentruntime.InferenceCapacityError{Reason: "owner_unavailable"}
	}
	o.consumer.Refresh(ctx)
	wake := o.capacity.InferenceCapacityChanged()
	admitted, reason, err := o.capacity.ReserveNativeRouteCapacity(ctx, route)
	if err != nil {
		return &agentruntime.InferenceCapacityError{Reason: reason, Cause: err}
	}
	if admitted {
		return nil
	}
	return &nativeRouteWait{owner: o, id: id, profileID: profileID, wake: wake, reason: reason}
}

// The existing capacity wake-up and native pending row own this wait. It adds
// no queue, timer, counter or inference attempt before eligibility is secured.
type nativeRouteWait struct {
	owner                 *nativeRouteCapacity
	id, profileID, reason string
	wake                  <-chan struct{}
}

func (w *nativeRouteWait) Error() string {
	return fmt.Sprintf("native inference admission %s", w.reason)
}
func (w *nativeRouteWait) Unwrap() error {
	return &agentruntime.InferenceCapacityError{Reason: w.reason}
}
func (w *nativeRouteWait) AwaitNativeRoute(ctx context.Context) error {
	wake := w.wake
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		}
		err := w.owner.admit(ctx, w.id, w.profileID)
		var again *nativeRouteWait
		if !errors.As(err, &again) {
			return err
		}
		wake = again.wake
	}
}
