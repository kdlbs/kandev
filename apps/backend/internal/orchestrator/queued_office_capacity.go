package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

// OfficeCapacityWaiter describes an original Office queue owner.
// The producer retains Office local priority/age order and real claim gates.
type OfficeCapacityWaiter struct {
	RunID, TaskID, WorkspaceID, ProfileID, Grade string
	QueuedAt                                     time.Time
	GradeKnown, Claimed                          bool
}

func queuedOfficeCapacityKey(id string) string { return "queued-office:" + id }

func (s *Service) SetOfficeCapacityWaiters(read func(context.Context) ([]OfficeCapacityWaiter, error)) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	c.officeWaiters = read
	c.officeTaskRequest = s.officeTaskCapacityRequest
	c.mu.Unlock()
}

func (c *sessionCeilingController) readOfficeCapacityWaitersLocked(ctx context.Context) ([]OfficeCapacityWaiter, error) {
	if c.officeWaiters == nil {
		return nil, nil
	}
	return c.officeWaiters(ctx)
}

func sameOfficeCapacityOwner(a, b OfficeCapacityWaiter) bool {
	return a.RunID == b.RunID && a.TaskID == b.TaskID && a.WorkspaceID == b.WorkspaceID && a.ProfileID == b.ProfileID && a.QueuedAt.Equal(b.QueuedAt)
}

func (c *sessionCeilingController) pruneQueuedOfficeCapacityLocked(candidates []OfficeCapacityWaiter) {
	valid := make(map[string]OfficeCapacityWaiter, len(candidates))
	for _, candidate := range candidates {
		valid[candidate.RunID] = candidate
	}
	for key, held := range c.reservations {
		if held.queuedOffice == nil {
			continue
		}
		current, exists := valid[held.queuedOffice.RunID]
		if !exists || !sameOfficeCapacityOwner(*held.queuedOffice, current) {
			delete(c.reservations, key)
			continue
		}
		held.queuedOffice.Claimed = current.Claimed
	}
}

func (c *sessionCeilingController) readOfficeCapacityArrivalLocked(req admissionRequest, arrival queuedTaskArrival, candidates []OfficeCapacityWaiter) (admissionRequest, queuedTaskArrival) {
	if req.officeRunID == "" {
		return req, arrival
	}
	for _, candidate := range candidates {
		if candidate.RunID != req.officeRunID || !candidate.Claimed || candidate.TaskID != req.taskID {
			continue
		}
		req.grade, req.gradeKnown = candidate.Grade, candidate.GradeKnown
		arrival = queuedTaskArrival{at: candidate.QueuedAt, taskID: queuedOfficeCapacityKey(candidate.RunID), claimed: true}
		key := queuedOfficeCapacityKey(candidate.RunID)
		held := c.reservations[key]
		if held != nil && held.queuedOffice != nil && sameOfficeCapacityOwner(*held.queuedOffice, candidate) {
			req.queuedGrantKey = key
			req.grade, req.gradeKnown = held.grade, held.gradeKnown
		}
		return req, arrival
	}
	return req, arrival
}

func (c *sessionCeilingController) nextQueuedOfficeCapacityLocked(ctx context.Context, candidates []OfficeCapacityWaiter) (*OfficeCapacityWaiter, admissionRequest, error) {
	for _, candidate := range candidates {
		if candidate.RunID == "" || candidate.QueuedAt.IsZero() || c.reservations[queuedOfficeCapacityKey(candidate.RunID)] != nil {
			continue
		}
		// A task-bound claimed owner can only consume its existing pregrant.
		// It is never allocated a second grant after the task handoff.
		if candidate.TaskID != "" && candidate.Claimed {
			continue
		}
		req, eligible, err := c.officeCapacityRequestLocked(ctx, candidate)
		if err != nil {
			return nil, req, err
		}
		if !eligible || !req.gradeKnown {
			continue
		}
		decision, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
		if decision.reasonCode == ceilingReasonUnknownPopulation {
			return nil, req, fmt.Errorf("office capacity population is unknown")
		}
		if decision.reasonCode != "" || (c.allCapacityLimitReason(p, maximum) != "" && !officeTaskSelectionCounted(req, p)) {
			continue
		}
		return &candidate, req, nil
	}
	return nil, admissionRequest{}, nil
}

// Office contributes its first capacity-eligible local head. Comparing that
// head with other sources' original arrival does not reorder Office classes.
func (c *sessionCeilingController) grantOfficeCapacityBeforeLocked(ctx context.Context, candidates []OfficeCapacityWaiter, at time.Time, id string) error {
	for range candidates {
		candidate, req, err := c.nextQueuedOfficeCapacityLocked(ctx, candidates)
		if err != nil {
			return err
		}
		if candidate == nil {
			return nil
		}
		key := queuedOfficeCapacityKey(candidate.RunID)
		if candidate.QueuedAt.After(at) || (candidate.QueuedAt.Equal(at) && key >= id) {
			return nil
		}
		key = c.reserveGradeLocked(req)
		c.reservations[key].queuedOffice = candidate
		if req.startSelection != nil && req.startSelection.session != nil {
			c.reservations[key].sessionID = req.startSelection.session.ID
		}
		if c.queuedWake != nil {
			c.queuedWake()
		}
	}
	return nil
}

func (c *sessionCeilingController) consumeQueuedOfficeCapacityLocked(req admissionRequest, p gradedPopulation) (admissionDecision, bool) {
	held := c.reservations[req.queuedGrantKey]
	if req.officeRunID == "" || held == nil || held.queuedOffice == nil || !held.queuedOffice.Claimed || held.queuedOffice.RunID != req.officeRunID || held.queuedOffice.TaskID != req.taskID {
		return admissionDecision{}, false
	}
	if existing := c.reservations[req.sessionID]; existing != nil && existing != held {
		return admissionDecision{ceiling: c.ceiling, reasonCode: ceilingReasonUnknownPopulation}, true
	}
	key := req.queuedGrantKey
	if req.sessionID != "" {
		delete(c.reservations, key)
		key = req.sessionID
		c.reservations[key] = held
		held.sessionID = req.sessionID
	}
	held.officeRunID = req.officeRunID
	held.queuedOffice = nil
	return admissionDecision{admitted: true, reservationKey: key, ceiling: c.ceiling, population: p.total, populationKnown: true}, true
}

// Office calls this before its original claim writer transaction. The IDs
// merely identify existing common reservations; the writer still owns gate
// revalidation, queue-status CAS and exactly one launch-ledger append.
func (s *Service) RunCapacityClaimCandidates(ctx context.Context) ([]string, error) {
	if s == nil || s.sessionCeiling == nil {
		return nil, fmt.Errorf("office capacity owner unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.policy != sessioncapacity.PolicyAll {
		return nil, nil
	}
	if _, err := c.prepareQueuedTaskCapacityLocked(ctx, admissionRequest{seam: "officeQueueClaim"}); err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, held := range c.reservations {
		if held.queuedOffice != nil && !held.queuedOffice.Claimed {
			ids = append(ids, held.queuedOffice.RunID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func claimedOfficeCapacityOwner(id string, candidates []OfficeCapacityWaiter, taskIDs ...string) bool {
	taskID := ""
	if len(taskIDs) > 0 {
		taskID = taskIDs[0]
	}
	for _, candidate := range candidates {
		if candidate.RunID == id && candidate.Claimed && candidate.TaskID == taskID {
			return true
		}
	}
	return false
}
