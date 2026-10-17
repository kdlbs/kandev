package orchestrator

import (
	"context"
	"fmt"
	"maps"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

// SetInferenceCapacityOwner uses the existing admission and settings-row owners.
func (s *Service) SetInferenceCapacityOwner(store *sessioncapacity.Store, refresh func(context.Context)) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inferenceStore, c.inferenceRefresh, c.inferenceKnown = store, refresh, false
}

func (s *Service) InferenceCapacityPolicy() (bool, bool) {
	if s == nil || s.sessionCeiling == nil {
		return false, false
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policy == sessioncapacity.PolicyAll, c.ceiling != unlimitedSessionCeiling || len(c.gradeLimits) > 0
}

func (s *Service) InferenceInventoryKnown() bool {
	if s == nil || s.sessionCeiling == nil {
		return false
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inferenceKnown
}

func (c *sessionCeilingController) appendInferenceGradesLocked(ctx context.Context, p *gradedPopulation, grade string) error {
	if c.inferenceStore == nil {
		return nil
	}
	if !c.inferenceKnown {
		c.logPopulationUnknown("inference_inventory", "unknown", "", "")
		return fmt.Errorf("inference population is unknown")
	}
	bindings, err := c.inferenceStore.LoadInferenceBindings(ctx)
	if err != nil {
		c.logPopulationUnknown("inference_bindings", populationReadFailure(err), "", "")
		return err
	}
	p.inferenceBindings = bindings
	for key, binding := range bindings {
		if !binding.GradeKnown {
			c.logPopulationUnknown("inference_bindings", "grade_unknown", "attempt_id", binding.AttemptID)
		}
		p.counted[key] = struct{}{}
		c.observeGradeLocked(p, binding.Grade, binding.GradeKnown, grade)
	}
	return nil
}

func (s *Service) ReserveInferenceCapacity(ctx context.Context, binding sessioncapacity.InferenceBinding) (bool, string, error) {
	if s == nil || s.sessionCeiling == nil {
		return false, ceilingReasonUnknownPopulation, fmt.Errorf("inference capacity owner unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	if binding.Key != "inference:"+binding.OwnerID+":"+binding.AttemptID || binding.InstanceID == "" || binding.OwnerID == "" || binding.AttemptID == "" {
		return false, ceilingReasonUnknownPopulation, fmt.Errorf("inference ownership identity unavailable")
	}
	active, err := c.inferenceStore.LoadInferenceBindings(ctx)
	if err != nil {
		return false, ceilingReasonUnknownPopulation, err
	}
	if held, ok := active[binding.Key]; ok {
		if !sameInferenceOwner(held, binding) {
			return false, ceilingReasonUnknownPopulation, fmt.Errorf("inference ownership changed")
		}
		return true, "", nil
	}
	if retainedNativeChild(binding, active) {
		return false, ceilingReasonUnknownPopulation, fmt.Errorf("retained native inference owner is still active")
	}
	if handled, admitted, reason, err := c.transferNativeRouteLocked(ctx, binding); handled {
		return admitted, reason, err
	}
	if reason, err := c.inspectFreshInferenceCapacityLocked(ctx, binding); err != nil || reason != "" {
		return false, reason, err
	}
	return c.persistInferenceBindingLocked(ctx, binding)
}

func (c *sessionCeilingController) inspectFreshInferenceCapacityLocked(ctx context.Context, binding sessioncapacity.InferenceBinding) (string, error) {
	if c.policy != sessioncapacity.PolicyAll {
		return "", nil
	}
	req := admissionRequest{grade: binding.Grade, gradeKnown: binding.GradeKnown, seam: "inferenceCapacity"}
	var priorityErr error
	req, priorityErr = c.prepareQueuedTaskCapacityLocked(ctx, req)
	if priorityErr != nil {
		return ceilingReasonUnknownPopulation, priorityErr
	}
	decision, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
	if decision.reasonCode != "" {
		return decision.reasonCode, nil
	}
	if reason := c.allCapacityLimitReason(p, maximum); reason != "" {
		return reason, nil
	}
	return "", nil
}

func (c *sessionCeilingController) persistInferenceBindingLocked(ctx context.Context, binding sessioncapacity.InferenceBinding) (bool, string, error) {
	err := c.inferenceStore.UpdateInferenceBindings(ctx, func(active map[string]sessioncapacity.InferenceBinding) error {
		if prior, exists := active[binding.Key]; exists {
			if !sameInferenceOwner(prior, binding) {
				return fmt.Errorf("inference ownership changed")
			}
			return nil
		}
		active[binding.Key] = binding
		return nil
	})
	if err != nil {
		c.inferenceKnown = false
		return false, ceilingReasonUnknownPopulation, err
	}
	return true, "", nil
}

func sameInferenceOwner(a, b sessioncapacity.InferenceBinding) bool {
	return a.Key == b.Key && a.InstanceID == b.InstanceID && a.OwnerID == b.OwnerID && a.AttemptID == b.AttemptID && a.PreparationID == b.PreparationID
}

func (s *Service) InferenceCapacityBindings(ctx context.Context) (map[string]sessioncapacity.InferenceBinding, error) {
	if s == nil || s.sessionCeiling == nil {
		return nil, fmt.Errorf("inference capacity owner unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	store := c.inferenceStore
	c.mu.Unlock()
	return store.LoadInferenceBindings(ctx)
}

func (s *Service) SyncInferenceCapacity(ctx context.Context, known bool, update func(map[string]sessioncapacity.InferenceBinding) error) error {
	return s.syncInferenceCapacity(ctx, &known, update)
}

func (s *Service) UpdateInferenceCapacity(ctx context.Context, update func(map[string]sessioncapacity.InferenceBinding) error) error {
	return s.syncInferenceCapacity(ctx, nil, update)
}

func (s *Service) syncInferenceCapacity(ctx context.Context, known *bool, update func(map[string]sessioncapacity.InferenceBinding) error) error {
	if s == nil || s.sessionCeiling == nil || update == nil {
		return fmt.Errorf("inference capacity update unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	changed := known != nil && c.inferenceKnown != *known
	err := c.inferenceStore.UpdateInferenceBindings(ctx, func(active map[string]sessioncapacity.InferenceBinding) error {
		before := maps.Clone(active)
		if err := update(active); err != nil {
			return err
		}
		changed = changed || !maps.Equal(before, active)
		return nil
	})
	if err != nil {
		c.logPopulationUnknown("inference_bindings", "update_failed", "", "")
	}
	if known != nil {
		c.inferenceKnown = *known && err == nil
	} else if err != nil {
		c.inferenceKnown = false
	}
	c.mu.Unlock()
	if changed {
		s.signalCeilingSweep()
	}
	return err
}

func (s *Service) InferenceExecutionGrade(ctx context.Context, taskID, logicalProfileID string) (string, error) {
	if s == nil || s.sessionCeiling == nil {
		return "", fmt.Errorf("inference grade owner unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	resolve := c.resolveGrade
	c.mu.Unlock()
	if resolve == nil {
		return "", fmt.Errorf("inference grade resolver unavailable")
	}
	return resolve(ctx, admissionRequest{taskID: taskID, profileID: logicalProfileID})
}

func (s *Service) InferenceCapacityChanged() <-chan struct{} {
	c := s.sessionCeiling
	if c == nil {
		return nil
	}
	c.inferenceWakeMu.Lock()
	defer c.inferenceWakeMu.Unlock()
	if c.inferenceWake == nil {
		c.inferenceWake = make(chan struct{})
	}
	return c.inferenceWake
}

func (c *sessionCeilingController) notifyInferenceWaiters() {
	if c == nil {
		return
	}
	c.inferenceWakeMu.Lock()
	defer c.inferenceWakeMu.Unlock()
	if c.inferenceWake != nil {
		close(c.inferenceWake)
	}
	c.inferenceWake = make(chan struct{})
}

func (s *Service) refreshInferenceCapacity(ctx context.Context) {
	c := s.sessionCeiling
	if c == nil {
		return
	}
	c.mu.Lock()
	refresh := c.inferenceRefresh
	all := c.policy == sessioncapacity.PolicyAll
	c.mu.Unlock()
	if refresh == nil {
		return
	}
	if !all {
		bindings, err := s.InferenceCapacityBindings(ctx)
		if err == nil && len(bindings) == 0 {
			return
		}
	}
	refresh(ctx)
	c.notifyInferenceWaiters()
}
