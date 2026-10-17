package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// NativeCapacityWaiter is a live caller backed by its original pending row.
// Its producer reads the existing worker registry, not all pending DB rows.
type NativeCapacityWaiter struct {
	Route    NativeInferenceRoute
	QueuedAt time.Time
}

func (s *Service) SetNativeCapacityWaiters(read func(context.Context) ([]NativeCapacityWaiter, error)) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	c.nativeWaiters = read
	c.mu.Unlock()
}

func (c *sessionCeilingController) readNativeCapacityWaitersLocked(ctx context.Context) ([]NativeCapacityWaiter, error) {
	if c.nativeWaiters == nil {
		return nil, nil
	}
	return c.nativeWaiters(ctx)
}

func nativeCapacityArrival(req admissionRequest, arrival queuedTaskArrival, native []NativeCapacityWaiter) queuedTaskArrival {
	if req.seam != "nativeRoute" {
		return arrival
	}
	for _, candidate := range native {
		if candidate.Route.ID == req.sessionID && !candidate.QueuedAt.IsZero() {
			return queuedTaskArrival{at: candidate.QueuedAt, taskID: candidate.Route.ID, claimed: true}
		}
	}
	return arrival
}

type queuedCapacityGrant struct {
	id     string
	at     time.Time
	task   *queuedTaskCapacityCandidate
	native *NativeCapacityWaiter
}

func capacityGrantCandidates(tasks []queuedTaskCapacityCandidate, native []NativeCapacityWaiter) []queuedCapacityGrant {
	result := make([]queuedCapacityGrant, 0, len(tasks)+len(native))
	for i := range tasks {
		candidate := &tasks[i]
		result = append(result, queuedCapacityGrant{id: candidate.owner.taskID, at: candidate.owner.deferral.QueuedAt, task: candidate})
	}
	for i := range native {
		candidate := &native[i]
		result = append(result, queuedCapacityGrant{id: candidate.Route.ID, at: candidate.QueuedAt, native: candidate})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].at.Equal(result[j].at) {
			return result[i].id < result[j].id
		}
		return result[i].at.Before(result[j].at)
	})
	return result
}

func (c *sessionCeilingController) grantOlderCapacityCandidatesLocked(ctx context.Context, arrival queuedTaskArrival, tasks []queuedTaskCapacityCandidate, native []NativeCapacityWaiter, office []OfficeCapacityWaiter) error {
	changed := false
	for _, candidate := range capacityGrantCandidates(tasks, native) {
		if candidate.at.IsZero() || (arrival.claimed && candidate.id == arrival.taskID) {
			continue
		}
		if candidate.at.After(arrival.at) || (candidate.at.Equal(arrival.at) && candidate.id >= arrival.taskID) {
			continue
		}
		if err := c.grantOfficeCapacityBeforeLocked(ctx, office, candidate.at, candidate.id); err != nil {
			return err
		}
		reserved, err := c.reserveCapacityGrantLocked(ctx, candidate)
		if err != nil {
			return err
		}
		changed = changed || reserved
	}
	if changed && c.queuedWake != nil {
		c.queuedWake()
	}
	return c.grantOfficeCapacityBeforeLocked(ctx, office, arrival.at, arrival.taskID)
}

func (c *sessionCeilingController) reserveCapacityGrantLocked(ctx context.Context, candidate queuedCapacityGrant) (bool, error) {
	if candidate.native != nil {
		return c.reserveQueuedNativeCapacityLocked(ctx, *candidate.native)
	}
	task := *candidate.task
	key := queuedTaskCapacityKey(task.owner.taskID)
	if c.reservations[key] != nil || (task.request.sessionID != "" && c.reservations[task.request.sessionID] != nil) {
		return false, nil
	}
	return c.reserveQueuedTaskCandidateLocked(ctx, key, task)
}

func (c *sessionCeilingController) reserveQueuedNativeCapacityLocked(ctx context.Context, candidate NativeCapacityWaiter) (bool, error) {
	route := candidate.Route
	if !route.valid() || c.reservations[route.ID] != nil {
		return false, nil
	}
	if c.inferenceStore == nil {
		return false, fmt.Errorf("native inference inventory unavailable")
	}
	active, err := c.inferenceStore.LoadInferenceBindings(ctx)
	if err != nil {
		return false, err
	}
	if nativeRouteRetained(route, active) {
		return false, nil
	}
	req := admissionRequest{sessionID: route.ID, taskID: route.TaskID, profileID: route.LogicalProfileID, origin: launchOriginAutomatic, seam: "nativeRoute"}
	if c.resolveGrade == nil {
		return false, nil
	}
	req.grade, err = c.resolveGrade(ctx, admissionRequest{taskID: route.TaskID, profileID: route.LogicalProfileID})
	req.gradeKnown = err == nil
	if !req.gradeKnown {
		return false, nil
	}
	decision, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
	if decision.reasonCode == ceilingReasonUnknownPopulation {
		return false, fmt.Errorf("queued native population is unknown")
	}
	if decision.reasonCode != "" || c.allCapacityLimitReason(p, maximum) != "" {
		return false, nil
	}
	key := c.reserveGradeLocked(req)
	c.reservations[key].nativeOwner = &route
	c.reservations[key].nativeQueued = true
	return true, nil
}

func (c *sessionCeilingController) pruneQueuedNativeCapacityLocked(candidates []NativeCapacityWaiter) {
	valid := make(map[string]NativeInferenceRoute, len(candidates))
	for _, candidate := range candidates {
		valid[candidate.Route.ID] = candidate.Route
	}
	for key, held := range c.reservations {
		if !held.nativeQueued {
			continue
		}
		current, exists := valid[key]
		if !exists || held.nativeOwner == nil || *held.nativeOwner != current {
			delete(c.reservations, key)
		}
	}
}
