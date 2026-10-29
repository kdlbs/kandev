package utility

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

const attemptDiagnosticNone = "none"

var errAttemptExists = errors.New("inference attempt already exists")
var errAttemptUnavailable = errors.New("inference attempt token is unavailable")

type attemptOwner struct {
	mu                 sync.Mutex
	id                 string
	cfg                *config.InstanceConfig
	journal            *journal.Journal
	state              inferenceOwnerState
	volatileFresh      bool
	storageUnavailable bool
	diagnostics        AttemptDiagnostics
	expectedOwnerID    string
}

func newAttemptOwner(cfg *config.InstanceConfig, retained *journal.Journal) *attemptOwner {
	return newAttemptOwnerWithID(cfg, retained, uuid.NewString())
}

func newAttemptOwnerWithID(cfg *config.InstanceConfig, retained *journal.Journal, id string) *attemptOwner {
	o := &attemptOwner{id: id, cfg: cfg, journal: retained,
		state: inferenceOwnerState{Version: 1, Attempts: make(map[string]*ownedAttempt)}}
	o.volatileFresh = cfg.InferenceOwnerFresh && cfg.DurableJournalPath == "" && retained == nil
	o.state.ContinuityKnown = o.volatileFresh
	o.diagnostics = newAttemptDiagnostics(cfg, retained, o.volatileFresh)
	o.loadRetainedOwner()
	o.reconcileLocked()
	o.persistLocked(false, "")
	return o
}

func (o *attemptOwner) loadRetainedOwner() {
	if o.journal == nil {
		return
	}
	data, found, err := o.journal.ReadInferenceOwner()
	if err != nil {
		o.ownerLoadUnavailable("journal_read_failed", attemptJournalErrorClass(err))
		return
	}
	if !found {
		o.state.ContinuityKnown = o.journal.Fresh()
		phase := ownerUnproven
		if o.state.ContinuityKnown {
			phase = ownerActive
		}
		o.state.Ownership = ownershipFor(o.cfg, o.id, 1, phase)
		o.diagnostics.InitialReason = "existing_journal_without_owner"
		if o.state.ContinuityKnown {
			o.diagnostics.InitialReason = "journal_fresh_empty"
		}
		return
	}
	var saved inferenceOwnerState
	if json.Unmarshal(data, &saved) != nil {
		o.ownerLoadUnavailable("retained_decode_failed", "decode")
		return
	}
	if !validInferenceOwnerState(saved) {
		o.ownerLoadUnavailable("retained_state_invalid", "invalid_state")
		return
	}
	o.state = saved
	o.diagnostics.InitialReason = "retained_owner_loaded"
	// Only a complete, durably closed predecessor can transfer census trust.
	o.adoptRetainedClaim(saved)
}

func (o *attemptOwner) beginLocked(id, callID, preparationID string) (AttemptStatus, error) {
	if o.activateLocked() != nil {
		return AttemptStatus{AttemptID: id, State: AttemptUnknown, PromptDispatch: PromptPossiblySent}, errAttemptUnavailable
	}
	if existing := o.state.Attempts[id]; existing != nil {
		return existing.Status, errAttemptExists
	}
	o.pruneLocked()
	status := AttemptStatus{AttemptID: id, CallID: callID, PreparationID: preparationID, OwnerID: o.id,
		InstanceID: o.cfg.InstanceID, TaskID: o.cfg.TaskID, SessionID: o.cfg.SessionID,
		State: AttemptPrepared, PromptDispatch: PromptNotSent, Known: true, UpdatedAt: time.Now().UTC()}
	o.state.Attempts[id] = &ownedAttempt{Status: status}
	o.persistLocked(false, id)
	return status, nil
}

func (o *attemptOwner) change(id string, update func(*ownedAttempt)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if a := o.state.Attempts[id]; a != nil {
		update(a)
		a.Status.UpdatedAt = time.Now().UTC()
		o.persistLocked(a.Status.Settled, id)
	}
}

func (o *attemptOwner) snapshot() AttemptSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reconcileLocked()
	result := AttemptSnapshot{OwnerID: o.id, InstanceID: o.cfg.InstanceID,
		Known: o.state.ContinuityKnown, Attempts: make([]AttemptStatus, 0, len(o.state.Attempts)), Diagnostics: o.diagnosticSnapshot()}
	for _, a := range o.state.Attempts {
		if o.historicalSettled(a) {
			continue
		}
		result.Attempts = append(result.Attempts, a.Status)
		if !a.Status.Known {
			result.Known = false
		}
	}
	sort.Slice(result.Attempts, func(i, j int) bool { return result.Attempts[i].AttemptID < result.Attempts[j].AttemptID })
	return result
}

func (o *attemptOwner) get(id string) (AttemptStatus, bool) {
	for _, a := range o.snapshot().Attempts {
		if a.AttemptID == id {
			return a, true
		}
	}
	return AttemptStatus{AttemptID: id, State: AttemptUnknown, PromptDispatch: PromptPossiblySent}, false
}

func (o *attemptOwner) persistLocked(terminal bool, currentID string) {
	if o.storageUnavailable {
		o.diagnostics.LastPersistResult = "skipped_unavailable"
		return
	}
	if o.journal == nil {
		o.diagnostics.LastPersistResult = "not_configured"
		if !o.volatileFresh {
			o.state.ContinuityKnown = false
		}
		return
	}
	data, err := json.Marshal(o.state)
	if err == nil && int64(len(data)) > o.journal.MaxEventBytes() {
		o.trimTerminalLocked(int64(len(data)), o.journal.MaxEventBytes()*3/4, currentID)
		data, err = json.Marshal(o.state)
	}
	if err == nil {
		err = o.writeClaim(data, terminal)
	}
	if err != nil {
		if errors.Is(err, journal.ErrInferenceOwnerConflict) {
			o.storageUnavailable = true
		}
		o.state.ContinuityKnown = false
		o.diagnostics.LastPersistResult = "write_failed"
		if o.diagnostics.FirstPersistFailure == attemptDiagnosticNone {
			o.diagnostics.FirstPersistFailure = attemptJournalErrorClass(err)
		}
		return
	}
	o.diagnostics.LastPersistResult = "ok"
}

func (o *attemptOwner) trimTerminalLocked(encodedBytes, target int64, currentID string) {
	var terminal []*ownedAttempt
	for _, a := range o.state.Attempts {
		if o.prunable(a) && a.Status.AttemptID != currentID {
			terminal = append(terminal, a)
		}
	}
	sort.Slice(terminal, func(i, j int) bool { return terminal[i].Status.UpdatedAt.Before(terminal[j].Status.UpdatedAt) })
	for _, a := range terminal {
		if encodedBytes <= target {
			break
		}
		encoded, err := json.Marshal(a)
		if err != nil {
			break
		}
		delete(o.state.Attempts, a.Status.AttemptID)
		// UUID map key, colon, comma and quotes disappear with this entry.
		encodedBytes -= int64(len(encoded) + len(a.Status.AttemptID) + 4)
	}
}

func (o *attemptOwner) reconcileLocked() {
	changed := false
	for _, a := range o.state.Attempts {
		currentOwner := a.Status.OwnerID == o.id
		if a.Status.Settled || (currentOwner && a.Status.State != AttemptUnknown) {
			continue
		}
		before := a.Status
		a.Status.Known, a.Status.Settled, a.Status.State = false, false, AttemptUnknown
		if !currentOwner {
			a.Status.PromptDispatch = PromptPossiblySent
		}
		if a.PID > 0 && recoveredInferenceTerminated(a.PID) {
			a.Status.Known, a.Status.Settled, a.Status.State = true, true, AttemptTerminated
		}
		if a.Status != before {
			a.Status.UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	if changed {
		o.persistLocked(true, "")
	}
}

func validInferenceOwnerState(state inferenceOwnerState) bool {
	if state.Version != 1 || state.Attempts == nil || !validOwnership(state.Ownership) {
		return false
	}
	for id, a := range state.Attempts {
		if a == nil || a.Status.AttemptID != id || a.Status.OwnerID == "" || a.PID < 0 {
			return false
		}
		if a.Status.Settled && (!a.Status.Known || (a.Status.State != AttemptTerminated && a.Status.State != AttemptNotStarted)) {
			return false
		}
	}
	return true
}

func (o *attemptOwner) pruneLocked() {
	if len(o.state.Attempts) < journal.MaxStreamSubmissions {
		return
	}
	var terminal []*ownedAttempt
	for _, a := range o.state.Attempts {
		if o.prunable(a) {
			terminal = append(terminal, a)
		}
	}
	sort.Slice(terminal, func(i, j int) bool { return terminal[i].Status.UpdatedAt.Before(terminal[j].Status.UpdatedAt) })
	for _, a := range terminal {
		if len(o.state.Attempts) < journal.MaxStreamSubmissions {
			break
		}
		delete(o.state.Attempts, a.Status.AttemptID)
	}
}

func (o *attemptOwner) prunable(a *ownedAttempt) bool {
	return !a.Status.Retained && (a.Status.Settled || (!a.Consumed && a.Status.OwnerID == o.id && a.Status.State == AttemptPrepared))
}

// finalSnapshot is called only after admission closes and every owned operation drains.
func (o *attemptOwner) finalSnapshot() AttemptSnapshot {
	o.mu.Lock()
	for _, a := range o.state.Attempts {
		if a.Status.OwnerID == o.id && !a.Consumed && a.PID == 0 && a.Status.State == AttemptPrepared {
			a.Consumed = true
			settleNotStarted(a)
			a.Status.UpdatedAt = time.Now().UTC()
		}
	}
	o.closeClaimLocked()
	o.persistLocked(true, "")
	o.mu.Unlock()
	return o.snapshot()
}

func newAttemptDiagnostics(cfg *config.InstanceConfig, retained *journal.Journal, volatileFresh bool) AttemptDiagnostics {
	d := AttemptDiagnostics{Version: 1, InitialReason: "volatile_unproven", JournalMode: "not_configured",
		JournalErrorClass: attemptDiagnosticNone, FirstPersistFailure: attemptDiagnosticNone, LastPersistResult: "not_attempted",
		ManagerMintedID: cfg.InferenceOwnerFresh, DeliveryHarnessGeneration: cfg.DeliveryHarnessGeneration,
		DeliveryStreamPresent: cfg.DeliveryStreamID != ""}
	if id, err := uuid.Parse(cfg.DeliveryIncarnationID); err == nil {
		d.DeliveryIncarnationID = id.String()
	}
	if volatileFresh {
		d.InitialReason = "volatile_fresh"
	}
	if cfg.DurableJournalPath != "" {
		d.JournalMode, d.InitialReason = "configured_unavailable", "journal_unavailable"
	}
	if retained != nil {
		fresh := retained.Fresh()
		d.JournalFreshAtOwnerInit = &fresh
		d.JournalMode = "existing"
		if fresh {
			d.JournalMode = "fresh"
		}
	}
	return d
}

func (o *attemptOwner) ownerLoadUnavailable(reason, class string) {
	o.storageUnavailable = true
	o.diagnostics.InitialReason, o.diagnostics.JournalErrorClass = reason, class
}

func (o *attemptOwner) diagnosticSnapshot() *AttemptDiagnostics {
	d := o.diagnostics
	d.ContinuityKnown, d.StorageUnavailable = o.state.ContinuityKnown, o.storageUnavailable
	if d.JournalFreshAtOwnerInit != nil {
		fresh := *d.JournalFreshAtOwnerInit
		d.JournalFreshAtOwnerInit = &fresh
	}
	return &d
}

func attemptJournalErrorClass(err error) string {
	switch {
	case err == nil:
		return attemptDiagnosticNone
	case errors.Is(err, journal.ErrJournalClosed):
		return "closed"
	case errors.Is(err, journal.ErrJournalCorrupt):
		return "corrupt"
	case errors.Is(err, journal.ErrJournalFull):
		return "full"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	default:
		return "other"
	}
}
