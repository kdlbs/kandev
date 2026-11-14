package utility

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
)

const (
	closureFenced    = "fenced"
	closureObserved  = "observed"
	closureCommitted = "committed"
	closureConsumed  = "consumed"
)

var ErrInferenceClosureIncomplete = errors.New("inference closure scope is incomplete or changed")

// Explicit closure transfers historical continuity, so the newer
// current-inventory-only check is not sufficient on its own.
func completeOwnerState(state inferenceOwnerState) bool {
	return state.ContinuityKnown && settledCurrentOwnerState(state)
}

func unadmittedClaim(state inferenceOwnerState) bool {
	if state.Ownership == nil || state.Ownership.Phase != ownerBorn {
		return false
	}
	for _, attempt := range state.Attempts {
		if attempt.Status.OwnerID == state.Ownership.OwnerID {
			return false
		}
	}
	return true
}

// InferenceScopeClosure preserves the source census and the single permitted successor.
// It cannot replace missing historical containment evidence.
type InferenceScopeClosure struct {
	Version                   int                        `json:"version"`
	OperationID               string                     `json:"operation_id"`
	SessionID                 string                     `json:"session_id"`
	IncarnationID             string                     `json:"incarnation_id"`
	SourceInstanceID          string                     `json:"source_instance_id"`
	SourceOwnerID             string                     `json:"source_owner_id"`
	CandidateInstanceID       string                     `json:"candidate_instance_id"`
	HistoricalContinuityKnown bool                       `json:"historical_continuity_known"`
	Stage                     string                     `json:"stage"`
	ManagedStop               *ManagedProcessStopReceipt `json:"managed_stop,omitempty"`
	SuccessorOwnerID          string                     `json:"successor_owner_id,omitempty"`
}

func cloneClosure(c *InferenceScopeClosure) *InferenceScopeClosure {
	if c == nil {
		return nil
	}
	next := *c
	if c.ManagedStop != nil {
		proof := *c.ManagedStop
		next.ManagedStop = &proof
	}
	return &next
}

func (o *InferenceOwner) BeginInferenceClosure(ctx context.Context, operation, candidate, expectedOwner string) ([]byte, error) {
	a := o.attempts
	a.mu.Lock()
	defer a.mu.Unlock()
	prior := a.state.Ownership
	if !a.canBeginClosure(operation, candidate, expectedOwner) {
		return nil, ErrInferenceClosureIncomplete
	}
	if c := a.state.Closure; c != nil {
		if c.OperationID == operation && c.CandidateInstanceID == candidate && c.SourceOwnerID == expectedOwner {
			return json.Marshal(a.state)
		}
		if c.OperationID == operation || c.Stage != closureConsumed || c.SuccessorOwnerID != prior.OwnerID || c.CandidateInstanceID != prior.InstanceID {
			return nil, ErrInferenceClosureIncomplete
		}
	}
	next := a.state
	next.Closure = &InferenceScopeClosure{Version: 1, OperationID: operation, SessionID: prior.SessionID,
		IncarnationID: prior.IncarnationID, SourceInstanceID: prior.InstanceID, SourceOwnerID: prior.OwnerID,
		CandidateInstanceID: candidate, HistoricalContinuityKnown: true, Stage: closureFenced}
	expected, err := json.Marshal(a.state)
	if err != nil {
		return nil, err
	}
	return a.writeClosure(ctx, expected, next)
}

func validClosureStop(c *InferenceScopeClosure, proof *ManagedProcessStopReceipt) bool {
	return c != nil && c.Version == 1 && c.HistoricalContinuityKnown && proof != nil && proof.Version == 1 &&
		proof.InstanceID == c.SourceInstanceID && proof.OwnerID == c.SourceOwnerID && !proof.ObservedAt.IsZero() &&
		proof.Coverage == ManagedStopCoverage && proof.AdmissionClosed && proof.OperationsDrained &&
		proof.ManagedProcessesStopped && proof.InferenceCensusKnown && proof.RecordedChildrenTerminal
}

func terminalClosureAttempts(state inferenceOwnerState) bool {
	if !state.ContinuityKnown {
		return false
	}
	for _, a := range state.Attempts {
		if !a.Status.Known || !a.Status.Settled || (a.Status.State != AttemptTerminated && a.Status.State != AttemptNotStarted) {
			return false
		}
	}
	return true
}

func (o *InferenceOwner) ObserveInferenceClosure(ctx context.Context, operation string, proof *ManagedProcessStopReceipt) ([]byte, error) {
	a := o.attempts
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.state.Closure
	if c == nil || c.OperationID != operation || c.Stage != closureFenced || !validClosureStop(c, proof) || !terminalClosureAttempts(a.state) {
		return nil, ErrInferenceClosureIncomplete
	}
	next := a.state
	next.Closure = cloneClosure(c)
	next.Closure.Stage = closureObserved
	copied := *proof
	next.Closure.ManagedStop = &copied
	expected, err := json.Marshal(a.state)
	if err != nil {
		return nil, err
	}
	return a.writeClosure(ctx, expected, next)
}

func (o *InferenceOwner) CommitInferenceClosure(ctx context.Context, operation string, expected []byte) ([]byte, error) {
	a := o.attempts
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.state.Closure
	current, err := json.Marshal(a.state)
	if err != nil {
		return nil, err
	}
	if c == nil || c.OperationID != operation || c.Stage != closureObserved || !bytes.Equal(current, expected) ||
		!validClosureStop(c, c.ManagedStop) || !completeOwnerState(a.state) {
		return nil, ErrInferenceClosureIncomplete
	}
	next := a.state
	next.Closure = cloneClosure(c)
	next.Closure.Stage = closureCommitted
	return a.writeClosure(ctx, expected, next)
}

func (a *attemptOwner) writeClosure(ctx context.Context, expected []byte, next inferenceOwnerState) ([]byte, error) {
	if a.storageUnavailable || a.journal == nil {
		return nil, ErrInferenceClosureIncomplete
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if err = a.journal.CompareAndWriteInferenceOwnerState(ctx, expected, encoded, true); err != nil {
		return nil, err
	}
	a.state = next
	a.expectedOwnerState = append([]byte(nil), encoded...)
	a.expectedOwnerID = a.id
	return encoded, nil
}

// InferenceClosureState returns the exact bytes used by the subsequent state CAS.
func (o *InferenceOwner) InferenceClosureState() ([]byte, error) {
	a := o.attempts
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.storageUnavailable || a.journal == nil {
		return nil, ErrInferenceClosureIncomplete
	}
	return json.Marshal(a.state)
}

func (a *attemptOwner) adoptClosure(saved inferenceOwnerState, raw []byte) bool {
	c := saved.Closure
	if c == nil {
		if a.cfg.InferenceClosureOperationID != "" {
			a.ownerLoadUnavailable("closure_missing", "incomplete_scope")
			return true
		}
		return false
	}
	// A later ordinary handoff does not consume this receipt again.
	if a.cfg.InferenceClosureOperationID == "" && c.Stage == closureConsumed {
		return false
	}
	if !a.closureSuccessorMatches(saved) {
		a.ownerLoadUnavailable("closure_identity_or_state_changed", "incomplete_scope")
		return true
	}
	a.expectedOwnerState = append([]byte(nil), raw...)
	if c.Stage == closureConsumed {
		a.id = c.SuccessorOwnerID
		a.expectedOwnerID = a.id
		return true
	}
	prior := saved.Ownership
	next := ownershipFor(a.cfg, a.id, prior.Revision+1, ownerBorn)
	next.PredecessorOwnerID = prior.OwnerID
	a.state.Ownership = next
	a.state.Closure = cloneClosure(c)
	a.state.Closure.Stage = closureConsumed
	a.state.Closure.SuccessorOwnerID = a.id
	a.expectedOwnerID = prior.OwnerID
	return true
}

func (a *attemptOwner) closureSuccessorMatches(saved inferenceOwnerState) bool {
	c, p := saved.Closure, saved.Ownership
	if c == nil || p == nil || c.OperationID == "" || c.OperationID != a.cfg.InferenceClosureOperationID ||
		c.CandidateInstanceID != a.cfg.InstanceID || c.SessionID != a.cfg.SessionID ||
		c.IncarnationID != a.cfg.DeliveryIncarnationID || !validClosureStop(c, c.ManagedStop) || !completeOwnerState(saved) {
		return false
	}
	if c.Stage == closureConsumed {
		return a.consumedClosureMatches(saved)
	}
	return c.Stage == closureCommitted && p.OwnerID == c.SourceOwnerID && p.InstanceID == c.SourceInstanceID &&
		p.Revision < math.MaxUint64 && matchingClosureHandoff(p, ownershipFor(a.cfg, a.id, p.Revision+1, ownerBorn), c)
}

// Ordinary retained claims keep the same instance. An explicit closure has
// already fenced and recorded the one permitted different successor.
func matchingClosureHandoff(prior, next *inferenceOwnership, closure *InferenceScopeClosure) bool {
	return prior != nil && next != nil && closure != nil &&
		prior.InstanceID == closure.SourceInstanceID && next.InstanceID == closure.CandidateInstanceID &&
		prior.InstanceID != "" && next.InstanceID != "" && prior.InstanceID != next.InstanceID &&
		prior.SessionID != "" && prior.SessionID == next.SessionID &&
		prior.IncarnationID == next.IncarnationID && next.HarnessGeneration >= prior.HarnessGeneration &&
		next.HarnessGeneration-prior.HarnessGeneration <= 1
}

func (a *attemptOwner) canBeginClosure(operation, candidate, expectedOwner string) bool {
	prior := a.state.Ownership
	if a.storageUnavailable || a.journal == nil || !a.state.ContinuityKnown || prior == nil ||
		prior.OwnerID != expectedOwner || prior.OwnerID != a.id || operation == "" || candidate == "" ||
		candidate == prior.InstanceID || prior.SessionID == "" || prior.IncarnationID == "" {
		return false
	}
	return true
}

func (a *attemptOwner) consumedClosureMatches(saved inferenceOwnerState) bool {
	c, p := saved.Closure, saved.Ownership
	return c.SuccessorOwnerID != "" && p.OwnerID == c.SuccessorOwnerID && p.InstanceID == c.CandidateInstanceID &&
		p.SessionID == c.SessionID && p.IncarnationID == c.IncarnationID &&
		p.HarnessGeneration == ownershipFor(a.cfg, a.id, p.Revision, ownerBorn).HarnessGeneration && unadmittedClaim(saved)
}
