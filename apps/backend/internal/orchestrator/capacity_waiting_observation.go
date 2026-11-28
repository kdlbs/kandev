package orchestrator

import (
	"context"
	"errors"
)

// Only the existing eligible task snapshot, live native worker registry and
// original Office claim gates establish waiters. Missing sources are unknown.
func (c *sessionCeilingController) observedCapacityRequestsLocked(ctx context.Context, population gradedPopulation) (map[string]admissionRequest, error) {
	if c.queuedTasks == nil || c.nativeWaiters == nil || c.officeWaiters == nil {
		return nil, errors.New("capacity waiting source unavailable")
	}
	requests := map[string]admissionRequest{}
	if err := c.observeTaskCapacityRequestsLocked(ctx, population, requests); err != nil {
		return nil, err
	}
	if err := c.observeNativeCapacityRequestsLocked(ctx, population, requests); err != nil {
		return nil, err
	}
	if err := c.observeOfficeCapacityRequestsLocked(ctx, population, requests); err != nil {
		return nil, err
	}
	return requests, nil
}

func (c *sessionCeilingController) observeTaskCapacityRequestsLocked(ctx context.Context, population gradedPopulation, requests map[string]admissionRequest) error {
	candidates, err := c.queuedTasks(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		key := queuedTaskCapacityKey(candidate.owner.taskID)
		held := c.reservations[key]
		if held != nil && held.queuedTask != nil {
			same, readErr := sameCeilingDeferralIdentity(held.queuedTask.deferral, candidate.owner.deferral)
			if readErr != nil {
				return readErr
			}
			if same {
				continue
			}
		}
		req := c.resolveAdmissionGradeLocked(ctx, candidate.request)
		if _, counted := population.counted[req.sessionID]; req.sessionID != "" && counted {
			continue
		}
		if officeTaskSelectionCounted(req, population) {
			continue
		}
		requests[key] = req
	}
	return nil
}

func (c *sessionCeilingController) observeNativeCapacityRequestsLocked(ctx context.Context, population gradedPopulation, requests map[string]admissionRequest) error {
	candidates, err := c.nativeWaiters(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		route := candidate.Route
		if !route.valid() {
			return errors.New("native capacity waiter owner unavailable")
		}
		held := c.reservations[route.ID]
		if held != nil && held.nativeOwner != nil && *held.nativeOwner == route {
			continue
		}
		if nativeRouteRetained(route, population.inferenceBindings) {
			continue
		}
		req := admissionRequest{sessionID: route.ID, taskID: route.TaskID, profileID: route.LogicalProfileID, origin: launchOriginAutomatic, seam: "nativeRoute"}
		requests[route.ID] = c.resolveAdmissionGradeLocked(ctx, req)
	}
	return nil
}

func (c *sessionCeilingController) observeOfficeCapacityRequestsLocked(ctx context.Context, population gradedPopulation, requests map[string]admissionRequest) error {
	candidates, err := c.officeWaiters(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if c.observedOfficeGrantLocked(candidate) {
			continue
		}
		req, eligible, readErr := c.officeCapacityRequestLocked(ctx, candidate)
		if readErr != nil {
			return readErr
		}
		if !eligible || officeTaskSelectionCounted(req, population) {
			continue
		}
		requests[queuedOfficeCapacityKey(candidate.RunID)] = req
	}
	return nil
}

func (c *sessionCeilingController) observedOfficeGrantLocked(candidate OfficeCapacityWaiter) bool {
	for _, held := range c.reservations {
		if held.queuedOffice != nil && sameOfficeCapacityOwner(*held.queuedOffice, candidate) {
			return true
		}
		if held.officeRunID != "" && held.officeRunID == candidate.RunID {
			return true
		}
	}
	return false
}
