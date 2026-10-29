package utility

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

const (
	ownerBorn     = "born"
	ownerActive   = "active"
	ownerClosed   = "closed"
	ownerUnproven = "unproven"
)

// inferenceOwnership is a durable census claim, not a delivery generation.
// Retained tokens still require the existing backend settlement and release.
type inferenceOwnership struct {
	OwnerID            string `json:"owner_id"`
	InstanceID         string `json:"instance_id"`
	SessionID          string `json:"session_id"`
	IncarnationID      string `json:"incarnation_id"`
	HarnessGeneration  uint64 `json:"harness_generation"`
	Revision           uint64 `json:"revision"`
	Phase              string `json:"phase"`
	PredecessorOwnerID string `json:"predecessor_owner_id,omitempty"`
}

// InferenceOwner is the single authority shared by process admission and HTTP.
type InferenceOwner struct{ attempts *attemptOwner }

// OpenInferenceOwner creates the first census in the journal initialization
// transaction. Existing files never receive this initial metadata.
func OpenInferenceOwner(cfg *config.InstanceConfig) (*journal.Journal, *InferenceOwner, error) {
	id := uuid.NewString()
	var retained *journal.Journal
	var err error
	if cfg.DurableJournalPath != "" {
		initial := inferenceOwnerState{Version: 1, ContinuityKnown: true, Attempts: make(map[string]*ownedAttempt), Ownership: ownershipFor(cfg, id, 1, ownerBorn)}
		data, marshalErr := json.Marshal(initial)
		if marshalErr != nil {
			return nil, nil, fmt.Errorf("encode inference birth: %w", marshalErr)
		}
		retained, err = journal.Open(journal.Config{Path: cfg.DurableJournalPath, InitialInferenceOwner: data})
	}
	o := newAttemptOwnerWithID(cfg, retained, id)
	return retained, &InferenceOwner{attempts: o}, err
}

// Activate durably fences a birth before any process-start admission escapes.
func (o *InferenceOwner) Activate() error {
	o.attempts.mu.Lock()
	defer o.attempts.mu.Unlock()
	return o.attempts.activateLocked()
}

// FinalSnapshot is called only after the existing process teardown has drained.
func (o *InferenceOwner) FinalSnapshot() AttemptSnapshot { return o.attempts.finalSnapshot() }

// Unavailable preserves an initialization failure before process admission.
func (o *InferenceOwner) Unavailable(err error) {
	o.attempts.mu.Lock()
	defer o.attempts.mu.Unlock()
	o.attempts.state.ContinuityKnown = false
	o.attempts.ownerLoadUnavailable("journal_unavailable", attemptJournalErrorClass(err))
}

func ownershipFor(cfg *config.InstanceConfig, id string, revision uint64, phase string) *inferenceOwnership {
	incarnation, generation := cfg.DeliveryIncarnationID, cfg.DeliveryHarnessGeneration
	if incarnation == "" {
		incarnation = cfg.SessionID
	}
	if generation == 0 {
		generation = 1
	}
	return &inferenceOwnership{OwnerID: id, InstanceID: cfg.InstanceID, SessionID: cfg.SessionID, IncarnationID: incarnation, HarnessGeneration: generation, Revision: revision, Phase: phase}
}

func validOwnership(ownership *inferenceOwnership) bool {
	if ownership == nil {
		return true
	} // Legacy states remain readable but unproven.
	if ownership.OwnerID == "" || ownership.Revision == 0 || ownership.HarnessGeneration == 0 {
		return false
	}
	switch ownership.Phase {
	case ownerBorn, ownerActive, ownerClosed, ownerUnproven:
		return true
	}
	return false
}

// settledCurrentOwnerState proves closure of concrete current claims without
// inventing historical continuity. A handoff still needs the closed phase and
// matching session identity, followed by the journal's owner CAS.
func settledCurrentOwnerState(state inferenceOwnerState) bool {
	for _, a := range state.Attempts {
		if !a.Status.Known || !a.Status.Settled || a.Status.Retained {
			return false
		}
	}
	return true
}

func (o *attemptOwner) adoptRetainedClaim(saved inferenceOwnerState) {
	prior := saved.Ownership
	o.expectedOwnerID = ""
	if prior == nil {
		o.state.ContinuityKnown = false
		o.state.Ownership = ownershipFor(o.cfg, o.id, 1, ownerUnproven)
		return
	}
	o.expectedOwnerID = prior.OwnerID
	// A birth written by this exact Open is not a predecessor handoff.
	if o.journal.Fresh() && prior.OwnerID == o.id && prior.Phase == ownerBorn && len(saved.Attempts) == 0 {
		o.diagnostics.InitialReason = "journal_fresh_empty"
		return
	}
	if prior.Revision == math.MaxUint64 {
		o.admissionFailureReason = "retained_revision_exhausted"
		o.state.ContinuityKnown = false
		o.storageUnavailable = true // Preserve the unclaimed predecessor bytes.
		return
	}
	next := ownershipFor(o.cfg, o.id, prior.Revision, ownerBorn)
	// A new owner only opens after the predecessor process is gone, so its
	// exited or never-started children are settled here instead of waiting for
	// a settler that no longer exists.
	settleAbandonedPredecessorAttempts(o.state.Attempts, o.id)
	if !settledCurrentOwnerState(o.state) || !matchingHandoffIdentity(prior, next) {
		// Remaining unknown children stay in history and in the current
		// inventory as unknown; they no longer block the session from starting.
		o.diagnostics.InitialReason = "retained_claim_adopted_unproven"
		o.state.ContinuityKnown = false
	}
	next.Revision = prior.Revision + 1
	next.PredecessorOwnerID = prior.OwnerID
	o.state.Ownership = next
	// The following constructor write compares the predecessor identity in the
	// same journal transaction. A competing claimant cannot also succeed.
}

// ActivationFailureReason is a body-free diagnostic for failed process admission.
func (o *InferenceOwner) ActivationFailureReason() string {
	o.attempts.mu.Lock()
	defer o.attempts.mu.Unlock()
	if o.attempts.admissionFailureReason != "" {
		return o.attempts.admissionFailureReason
	}
	if o.attempts.storageUnavailable {
		return "owner_storage_unavailable"
	}
	return "owner_claim_unavailable"
}

func (o *attemptOwner) activateLocked() error {
	if o.storageUnavailable {
		return errAttemptUnavailable
	}
	claim := o.state.Ownership
	if claim == nil {
		return nil
	}
	if claim.OwnerID != o.id || claim.Phase == ownerClosed {
		return errAttemptUnavailable
	}
	if claim.Phase == ownerBorn || claim.Phase == ownerUnproven {
		previousPhase := claim.Phase
		claim.Phase = ownerActive
		o.persistLocked(false, "")
		if o.diagnostics.LastPersistResult != "ok" && o.journal != nil {
			claim.Phase = previousPhase
			o.admissionFailureReason = "owner_activation_persist_failed"
			return errAttemptUnavailable
		}
	}
	return nil
}

func (o *attemptOwner) closeClaimLocked() {
	if o.state.Ownership == nil || o.state.Ownership.OwnerID != o.id {
		return
	}
	if settledCurrentOwnerState(o.state) {
		o.state.Ownership.Phase = ownerClosed
	} else {
		o.state.Ownership.Phase = ownerUnproven
	}
}

func (o *attemptOwner) historicalSettled(a *ownedAttempt) bool {
	claim := o.state.Ownership
	return claim != nil && claim.PredecessorOwnerID != "" && a.Status.OwnerID != o.id && a.Status.Known && a.Status.Settled && !a.Status.Retained
}

func (o *attemptOwner) writeClaim(data []byte, terminal bool) error {
	err := o.journal.CompareAndWriteInferenceOwner(context.Background(), o.expectedOwnerID, data, terminal)
	if err == nil {
		o.expectedOwnerID = o.id
	}
	return err
}

// settleAbandonedPredecessorAttempts runs only while this owner holds the
// exclusive journal, so the predecessor agentctl has exited. A child that never
// recorded a PID never started; a recorded child was reaped with its owner.
func settleAbandonedPredecessorAttempts(attempts map[string]*ownedAttempt, ownerID string) {
	for _, a := range attempts {
		if a.Status.OwnerID == ownerID || a.Status.Settled {
			continue
		}
		switch {
		case a.PID == 0 && (a.Status.State == AttemptPrepared || a.Status.State == AttemptNotStarted):
			a.Status.Known, a.Status.Settled, a.Status.State = true, true, AttemptNotStarted
		case a.PID > 0 && predecessorInferenceTerminated(a.PID):
			a.Status.Known, a.Status.Settled, a.Status.State = true, true, AttemptTerminated
		default:
			continue
		}
		a.Status.UpdatedAt = time.Now().UTC()
	}
}

// predecessorInferenceTerminated reports whether a child of an exited owner is
// gone. On Windows every inference child runs in a kill-on-close Job held only
// by its owner, so the kernel terminated the tree when that owner exited.
func predecessorInferenceTerminated(pid int) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return recoveredInferenceTerminated(pid)
}

func matchingHandoffIdentity(prior, next *inferenceOwnership) bool {
	return prior.InstanceID != "" && next.InstanceID != "" && prior.SessionID != "" && prior.SessionID == next.SessionID &&
		prior.IncarnationID == next.IncarnationID && next.HarnessGeneration >= prior.HarnessGeneration && next.HarnessGeneration-prior.HarnessGeneration <= 1
}
