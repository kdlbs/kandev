package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
)

// A priority grant refers to the original durable task deferral. It is an
// ordinary in-flight reservation, not a second queue or persisted marker.
type queuedTaskCapacityOwner struct {
	taskID   string
	deferral models.CeilingDeferral
}
type queuedTaskCapacityCandidate struct {
	owner   queuedTaskCapacityOwner
	request admissionRequest
}

func queuedTaskCapacityKey(taskID string) string { return "queued-task:" + taskID }

func (s *Service) wireQueuedTaskCapacityPriority() {
	c := s.sessionCeiling
	if c == nil {
		return
	}
	c.queuedTasks = s.readQueuedTaskCapacityCandidates
	c.queuedWake = s.signalCeilingSweep
}

// These reads deliberately do not acquire a per-task or message-queue admission
// lock while the common capacity lock is held. Actual dispatch still owns the
// existing exact entry claim and validates its destination before launch.
func (s *Service) readQueuedTaskCapacityCandidates(ctx context.Context) ([]queuedTaskCapacityCandidate, error) {
	lister, ok := s.repo.(ceilingDeferredTaskLister)
	if !ok {
		return nil, nil
	}
	tasks, err := lister.ListTasksWithCeilingDeferred(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]queuedTaskCapacityCandidate, 0, len(tasks))
	for _, indexed := range tasks {
		if indexed == nil {
			continue
		}
		candidate, valid, readErr := s.readQueuedTaskCapacityCandidate(ctx, indexed.ID)
		if readErr != nil {
			return nil, readErr
		}
		if valid {
			candidates = append(candidates, candidate)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i].owner, candidates[j].owner
		if a.deferral.QueuedAt.IsZero() != b.deferral.QueuedAt.IsZero() {
			return !a.deferral.QueuedAt.IsZero()
		}
		if a.deferral.QueuedAt.Equal(b.deferral.QueuedAt) {
			return a.taskID < b.taskID
		}
		return a.deferral.QueuedAt.Before(b.deferral.QueuedAt)
	})
	return candidates, nil
}

func (s *Service) readQueuedTaskCapacityCandidate(ctx context.Context, id string) (queuedTaskCapacityCandidate, bool, error) {
	empty := queuedTaskCapacityCandidate{}
	task, err := s.repo.GetTask(ctx, id)
	if err != nil || task == nil {
		return empty, false, err
	}
	deferral, valid, err := s.readCeilingDeferredLaunchSnapshot(ctx, task, s.validateQueuedCapacityEntrySnapshot)
	if err != nil || !valid {
		return empty, false, err
	}
	eligible, err := s.queuedTaskPolicySnapshot(ctx, task, deferral)
	if err != nil || !eligible {
		return empty, false, err
	}
	req := admissionRequest{taskID: task.ID, sessionID: sessionIDFromCeilingPayload(deferral), profileID: stringField(deferral.Payload, metaKeyAgentProfileID),
		profileExplicit: boolField(deferral.Payload, "profile_explicit"), workflowStepID: stringField(deferral.Payload, metaKeyWorkflowStepID), origin: ceilingAdmissionOrigin(launchOrigin(deferral.Origin)), seam: "queuedTaskPriority"}
	if deferral.Kind == models.CeilingLaunchStart {
		req.startSelection = &explicitWorkflowStartSelection{entryID: int64Field(deferral.Payload, "workflow_entry_id")}
	}
	return queuedTaskCapacityCandidate{owner: queuedTaskCapacityOwner{taskID: task.ID, deferral: deferral}, request: req}, true, nil
}

func (s *Service) validateQueuedCapacityEntrySnapshot(ctx context.Context, task *models.Task, deferral models.CeilingDeferral) (ceilingEntryDisposition, string, error) {
	return s.validateCeilingEntryWithDestinationState(ctx, task, deferral, true)
}

func (s *Service) queuedTaskPolicySnapshot(ctx context.Context, task *models.Task, deferral models.CeilingDeferral) (bool, error) {
	origin, err := ceilingStoredOrigin(deferral)
	if err != nil {
		return false, err
	}
	if origin == launchOriginManual {
		return true, nil
	}
	if id := sessionIDFromCeilingPayload(deferral); id != "" {
		if s.messageQueue == nil {
			return false, fmt.Errorf("queued task auto-run policy is unavailable")
		}
		enabled, err := s.messageQueue.AutoRunSnapshot(ctx, id)
		if err != nil || !enabled {
			return false, err
		}
	}
	if deferral.Kind == models.CeilingLaunchStart {
		if _, _, drop := s.evaluateCeilingStartDropReasons(ctx, task, deferral.Payload); drop {
			return false, nil
		}
	}
	return true, nil
}

type queuedTaskArrival struct {
	at      time.Time
	taskID  string
	claimed bool
}

func (c *sessionCeilingController) prepareQueuedTaskCapacityLocked(ctx context.Context, req admissionRequest) (admissionRequest, error) {
	if c.policy != sessioncapacity.PolicyAll {
		return req, nil
	}
	var candidates []queuedTaskCapacityCandidate
	var err error
	if c.queuedTasks != nil {
		candidates, err = c.queuedTasks(ctx)
	}
	if err != nil {
		return req, err
	}
	native, err := c.readNativeCapacityWaitersLocked(ctx)
	if err != nil {
		return req, err
	}
	office, err := c.readOfficeCapacityWaitersLocked(ctx)
	if err != nil {
		return req, err
	}
	if req.seam == officeRunCapacitySeam && req.officeRunID != "" && !claimedOfficeCapacityOwner(req.officeRunID, office) {
		return req, fmt.Errorf("office queue owner is not eligible")
	}
	c.pruneQueuedOfficeCapacityLocked(office)
	c.pruneQueuedNativeCapacityLocked(native)
	if err = c.pruneQueuedTaskCapacityLocked(candidates); err != nil {
		return req, err
	}
	req, arrival, err := c.readQueuedTaskArrivalLocked(ctx, req)
	if err != nil || req.queuedGrantKey != "" {
		return req, err
	}
	arrival = nativeCapacityArrival(req, arrival, native)
	req, arrival = c.readOfficeCapacityArrivalLocked(req, arrival, office)
	if req.queuedGrantKey != "" {
		return req, nil
	}
	return req, c.grantOlderCapacityCandidatesLocked(ctx, arrival, candidates, native, office)
}

func (c *sessionCeilingController) readQueuedTaskArrivalLocked(ctx context.Context, req admissionRequest) (admissionRequest, queuedTaskArrival, error) {
	arrival := queuedTaskArrival{at: c.now(), taskID: req.taskID}
	claim, _ := ctx.Value(ceilingDispatchClaimContextKey{}).(*ceilingDeferredLaunchClaim)
	if claim == nil || claim.taskID != req.taskID || req.seam == "nativeRoute" || req.seam == "inferenceCapacity" {
		return req, arrival, nil
	}
	arrival.at, arrival.claimed = claim.deferral.QueuedAt, true
	key := queuedTaskCapacityKey(req.taskID)
	held := c.reservations[key]
	if held == nil || held.queuedTask == nil {
		return req, arrival, nil
	}
	same, err := sameCeilingDeferralIdentity(held.queuedTask.deferral, claim.deferral)
	if err != nil {
		return req, arrival, err
	}
	if same {
		req.queuedGrantKey = key
	}
	return req, arrival, nil
}

func (c *sessionCeilingController) pruneQueuedTaskCapacityLocked(candidates []queuedTaskCapacityCandidate) error {
	valid := make(map[string]queuedTaskCapacityOwner, len(candidates))
	for _, candidate := range candidates {
		valid[candidate.owner.taskID] = candidate.owner
	}
	for key, held := range c.reservations {
		if held.queuedTask == nil {
			continue
		}
		current, exists := valid[held.queuedTask.taskID]
		if !exists {
			delete(c.reservations, key)
			continue
		}
		same, err := sameCeilingDeferralIdentity(held.queuedTask.deferral, current.deferral)
		if err != nil {
			return err
		}
		if !same {
			delete(c.reservations, key)
		}
	}
	return nil
}

func (c *sessionCeilingController) reserveQueuedTaskCandidateLocked(ctx context.Context, key string, candidate queuedTaskCapacityCandidate) (bool, error) {
	req := c.resolveAdmissionGradeLocked(ctx, candidate.request)
	if !req.gradeKnown {
		return false, nil
	}
	decision, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
	if decision.reasonCode == ceilingReasonUnknownPopulation {
		return false, fmt.Errorf("queued capacity population is unknown")
	}
	if decision.reasonCode != "" || c.allCapacityLimitReason(p, maximum) != "" {
		return false, nil
	}
	target := req.sessionID
	req.sessionID = key
	c.reserveGradeLocked(req)
	held := c.reservations[key]
	held.sessionID = target
	owner := candidate.owner
	held.queuedTask = &owner
	return true, nil
}

// Only the existing dispatch claim for this exact durable record can consume
// its priority grant. Grade persistence happens before ownership moves.
func (c *sessionCeilingController) consumeQueuedTaskCapacityLocked(ctx context.Context, req admissionRequest, p gradedPopulation) (admissionDecision, bool) {
	held := c.reservations[req.queuedGrantKey]
	if req.queuedGrantKey == "" || held == nil || held.queuedTask == nil {
		return admissionDecision{}, false
	}
	decision := admissionDecision{admitted: true, reservationKey: req.queuedGrantKey, ceiling: c.ceiling, population: p.total, populationKnown: true}
	decision = c.persistAdmissionGradeLocked(ctx, req, decision)
	if !decision.admitted {
		return decision, true
	}
	if req.sessionID != "" {
		if existing := c.reservations[req.sessionID]; existing != nil && existing != held {
			delete(c.reservations, req.queuedGrantKey)
			decision.reservationKey = req.sessionID
			return decision, true
		}
		delete(c.reservations, req.queuedGrantKey)
		c.reservations[req.sessionID] = held
		decision.reservationKey = req.sessionID
		held.sessionID = req.sessionID
	}
	held.queuedTask = nil
	return decision, true
}
