package orchestrator

import (
	"context"
	"errors"
	"sync"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
)

const officeRunCapacitySeam = "officeRunSession"

// RunSessionCapacitySource reads existing durable attempts, not a second queue.
type RunSessionCapacitySource interface {
	ListAdmittedRunExecutions(context.Context) ([]models.AdmittedExecution, error)
}

func (s *Service) SetRunSessionCapacitySource(source RunSessionCapacitySource) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	c.runSessions = source
	c.mu.Unlock()
}

func runSessionCapacityKey(id string) string { return "office-run:" + id }

func (s *Service) RunSessionCapacityEnabled() bool {
	if s == nil || s.sessionCeiling == nil {
		return false
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policy == sessioncapacity.PolicyAll
}

// RunSessionCapacityReservation holds only the existing controller reservation.
// The caller persists Grade on the immutable attempt before allocation.
type RunSessionCapacityReservation struct {
	service *Service
	key, id string
	Grade   string
	mu      sync.Mutex
	done    bool
}

// ReserveRunSessionCapacity uses the same atomic AND admission as task entries.
// Automatic-only/environment behavior remains unchanged by this opt-in bridge.
func (s *Service) ReserveRunSessionCapacity(ctx context.Context, id, grade string) (*RunSessionCapacityReservation, string, error) {
	if s == nil || s.sessionCeiling == nil {
		return nil, ceilingReasonUnknownPopulation, errors.New("run capacity owner unavailable")
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.policy != sessioncapacity.PolicyAll {
		return nil, "", nil
	}
	if err := ctx.Err(); err != nil {
		return nil, ceilingReasonUnknownPopulation, err
	}
	if id == "" || c.runSessions == nil {
		return nil, ceilingReasonUnknownPopulation, errors.New("run capacity inventory unavailable")
	}
	key := runSessionCapacityKey(id)
	if held := c.reservations[key]; held != nil {
		grade = held.grade
	}
	req := admissionRequest{sessionID: key, grade: grade, gradeKnown: true, origin: launchOriginAutomatic, seam: officeRunCapacitySeam}
	decision := c.admitAllLocked(ctx, req, false)
	if !decision.admitted {
		return nil, decision.reasonCode, nil
	}
	return &RunSessionCapacityReservation{service: s, key: decision.reservationKey, id: id, Grade: grade}, "", nil
}

// Confirm transfers ownership only after the exact durable graded attempt exists.
// A failed/unknown read leaves the reservation intact and can be retried.
func (r *RunSessionCapacityReservation) Confirm(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return nil
	}
	c := r.service.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runSessions == nil {
		return errors.New("run capacity inventory unavailable")
	}
	rows, err := c.runSessions.ListAdmittedRunExecutions(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == r.id && row.GradeKnown && row.Grade == r.Grade {
			delete(c.reservations, r.key)
			r.done = true
			return nil
		}
	}
	return errors.New("run capacity durable owner not confirmed")
}

// Release compensates an unstarted reservation. After Confirm, the durable
// attempt remains counted until its existing terminal CAS succeeds.
func (r *RunSessionCapacityReservation) Release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.done = true
	if r.key != "" {
		r.service.releaseCeilingReservation(r.key)
	}
}

func (s *Service) NotifyRunSessionCapacityReleased() {
	if s != nil {
		s.signalCeilingSweep()
	}
}

func (c *sessionCeilingController) appendRunSessionGradesLocked(ctx context.Context, population *gradedPopulation, grade string) error {
	if c.runSessions == nil {
		return nil
	}
	rows, err := c.runSessions.ListAdmittedRunExecutions(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		population.counted[runSessionCapacityKey(row.ID)] = struct{}{}
		c.observeGradeLocked(population, row.Grade, row.GradeKnown, grade)
	}
	return nil
}
