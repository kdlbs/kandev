package orchestrator

import (
	"context"
	"errors"
	"fmt"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

const ceilingReasonOfficeTaskOwnerMismatch = "office_task_capacity_owner_mismatch"

var ErrOfficeTaskCapacityDeferred = errors.New("office task waiting for common execution capacity")

func (s *Service) SetOfficeTaskCapacityRequeuer(requeue func(context.Context, string, string) error) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	c.officeTaskRequeue = requeue
}

func taskOfficeAdmissionRequest(ctx context.Context, req admissionRequest) admissionRequest {
	if req.seam == "startTask" {
		if owner, ok := runmodels.TaskCapacityRunFromContext(ctx); ok {
			req.officeRunID, req.officeTaskID = owner.RunID, owner.TaskID
		}
	}
	return req
}

// A policy change can discard this exact unused queue pregrant; no process or
// durable execution belongs to a reservation that still has queuedOffice set.
func (c *sessionCeilingController) releaseUnusedOfficeTaskGrantLocked(req admissionRequest) {
	key := queuedOfficeCapacityKey(req.officeRunID)
	held := c.reservations[key]
	if req.taskID != "" && req.officeRunID != "" && held != nil && held.queuedOffice != nil && held.queuedOffice.TaskID == req.taskID {
		delete(c.reservations, key)
	}
}

func (s *Service) deferOfficeTaskCapacity(ctx context.Context, taskID, runID, reason string) error {
	if reason == ceilingReasonOfficeTaskOwnerMismatch {
		return fmt.Errorf("%w: task context does not own this launch", ErrOfficeTaskCapacityDeferred)
	}
	c := s.sessionCeiling
	c.mu.Lock()
	requeue := c.officeTaskRequeue
	for _, held := range c.reservations {
		if held.officeRunID == runID && held.queuedOffice == nil {
			c.mu.Unlock()
			return fmt.Errorf("%w: existing task activation owns the run", ErrOfficeTaskCapacityDeferred)
		}
	}
	c.mu.Unlock()
	if requeue == nil {
		return fmt.Errorf("%w: queue requeue owner unavailable", ErrOfficeTaskCapacityDeferred)
	}
	if err := requeue(context.WithoutCancel(ctx), runID, taskID); err != nil {
		return errors.Join(ErrOfficeTaskCapacityDeferred, err)
	}
	return fmt.Errorf("%w: %s", ErrOfficeTaskCapacityDeferred, reason)
}
