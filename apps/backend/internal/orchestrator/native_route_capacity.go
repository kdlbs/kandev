package orchestrator

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

const nativeRouteCapacitySeam = "nativeRoute"

// NativeInferenceRoute refers to an existing utility call or review run. The
// bootstrap adapter verifies its relationships from the native repositories.
type NativeInferenceRoute struct {
	ID, TaskID, SessionID, LogicalProfileID, CallID, ReviewRunID string
}

func (r NativeInferenceRoute) valid() bool {
	if r.CallID != "" {
		return r.ReviewRunID == "" && r.ID == "native:utility-call:"+r.CallID
	}
	return r.ReviewRunID != "" && r.TaskID != "" && r.SessionID != "" && r.ID == "native:review-run:"+r.ReviewRunID
}

func nativeBindingRouteID(binding sessioncapacity.InferenceBinding) string {
	if binding.ReviewRunID != "" {
		return "native:review-run:" + binding.ReviewRunID
	}
	if binding.CallID != "" {
		return "native:utility-call:" + binding.CallID
	}
	return ""
}

func (r NativeInferenceRoute) matches(binding sessioncapacity.InferenceBinding) bool {
	return r.ID == nativeBindingRouteID(binding) && r.TaskID == binding.TaskID && r.SessionID == binding.SessionID && r.LogicalProfileID == binding.LogicalProfileID
}

func nativeRouteRetained(route NativeInferenceRoute, active map[string]sessioncapacity.InferenceBinding) bool {
	for _, held := range active {
		if route.CallID != "" && route.CallID == held.CallID && held.ReviewRunID == "" {
			return true
		}
		if route.ReviewRunID != "" && held.ReviewRunID == route.ReviewRunID {
			return true
		}
	}
	return false
}

// ReserveNativeRouteCapacity uses the same mutex/population/AND rule as task
// and Office admission, before a native route can consume a selection ordinal.
func (s *Service) ReserveNativeRouteCapacity(ctx context.Context, route NativeInferenceRoute) (bool, string, error) {
	if s == nil || s.sessionCeiling == nil || !route.valid() {
		return false, ceilingReasonUnknownPopulation, errors.New("native inference owner unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, ceilingReasonUnknownPopulation, err
	}
	if c.inferenceStore == nil {
		return false, ceilingReasonUnknownPopulation, errors.New("native inference inventory unavailable")
	}
	active, err := c.inferenceStore.LoadInferenceBindings(ctx)
	if err != nil {
		return false, ceilingReasonUnknownPopulation, err
	}
	if nativeRouteRetained(route, active) {
		return false, "retained_owner_active", nil
	}
	if c.policy != sessioncapacity.PolicyAll {
		return true, "", nil
	}
	return c.reserveNativeRouteLocked(ctx, route)
}

func (c *sessionCeilingController) reserveNativeRouteLocked(ctx context.Context, route NativeInferenceRoute) (bool, string, error) {
	req := admissionRequest{sessionID: route.ID, taskID: route.TaskID, profileID: route.LogicalProfileID, origin: launchOriginAutomatic, seam: nativeRouteCapacitySeam}
	held := c.reservations[route.ID]
	if held != nil {
		if held.nativeOwner == nil || *held.nativeOwner != route {
			return false, ceilingReasonUnknownPopulation, errors.New("native reservation owner changed")
		}
		req.grade, req.gradeKnown = held.grade, held.gradeKnown
	} else if c.resolveGrade != nil {
		var err error
		req.grade, err = c.resolveGrade(ctx, admissionRequest{taskID: route.TaskID, profileID: route.LogicalProfileID})
		req.gradeKnown = err == nil
		if err != nil {
			return false, ceilingReasonUnknownPopulation, err
		}
	}
	var priorityErr error
	req, priorityErr = c.prepareQueuedTaskCapacityLocked(ctx, req)
	if priorityErr != nil {
		return false, ceilingReasonUnknownPopulation, priorityErr
	}
	decision, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
	if decision.reasonCode != "" {
		return false, decision.reasonCode, nil
	}
	if held != nil {
		held.nativeQueued = false
		return true, "", nil
	}
	if reason := c.allCapacityLimitReason(p, maximum); reason != "" {
		return false, reason, nil
	}
	key := c.reserveGradeLocked(req)
	c.reservations[key].nativeOwner = &route
	return true, "", nil
}

func (s *Service) ReleaseNativeRouteCapacity(id string) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	held := c.reservations[id]
	changed := held != nil && held.nativeOwner != nil && held.nativeOwner.ID == id
	if changed {
		delete(c.reservations, id)
	}
	c.mu.Unlock()
	if changed {
		s.signalCeilingSweep()
	}
}

// Retire only an alias proven to belong to both the persisted child and the
// caller. A replay must not leave that same child counted twice.
func (c *sessionCeilingController) retirePersistedNativeAliasLocked(held, binding sessioncapacity.InferenceBinding) {
	id := nativeBindingRouteID(binding)
	alias := c.reservations[id]
	if alias != nil && alias.nativeOwner != nil && alias.nativeOwner.matches(held) && alias.nativeOwner.matches(binding) {
		delete(c.reservations, id)
	}
}

// transferNativeRouteLocked persists the exact child and removes the early
// alias under the same controller lock. Failed persistence retains the alias.
func (c *sessionCeilingController) transferNativeRouteLocked(ctx context.Context, binding sessioncapacity.InferenceBinding) (bool, bool, string, error) {
	id := nativeBindingRouteID(binding)
	held := c.reservations[id]
	if held == nil || held.nativeOwner == nil {
		return false, false, "", nil
	}
	if !held.nativeOwner.matches(binding) {
		return true, false, ceilingReasonUnknownPopulation, errors.New("native child relationships changed")
	}
	// The alias is the slot being transferred, including when the policy
	// changed after reservation. A refusal restores the original owner.
	delete(c.reservations, id)
	if c.policy == sessioncapacity.PolicyAll {
		req := admissionRequest{grade: held.grade, gradeKnown: held.gradeKnown, seam: nativeRouteCapacitySeam}
		decision, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
		if decision.reasonCode != "" {
			c.reservations[id] = held
			return true, false, decision.reasonCode, nil
		}
		if reason := c.allCapacityLimitReason(p, maximum); reason != "" {
			c.reservations[id] = held
			return true, false, reason, nil
		}
	}
	binding.Grade, binding.GradeKnown = held.grade, held.gradeKnown
	admitted, reason, err := c.persistInferenceBindingLocked(ctx, binding)
	if !admitted {
		c.reservations[id] = held
	}
	return true, admitted, reason, err
}

func retainedNativeChild(binding sessioncapacity.InferenceBinding, active map[string]sessioncapacity.InferenceBinding) bool {
	if strings.HasPrefix(nativeBindingRouteID(binding), "native:") {
		route := NativeInferenceRoute{CallID: binding.CallID, ReviewRunID: binding.ReviewRunID, TaskID: binding.TaskID}
		if binding.ReviewRunID != "" {
			route.CallID = ""
		}
		return nativeRouteRetained(route, active)
	}
	return false
}
