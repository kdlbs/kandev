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
}

func newAttemptOwner(cfg *config.InstanceConfig, retained *journal.Journal) *attemptOwner {
	o := &attemptOwner{id: uuid.NewString(), cfg: cfg, journal: retained,
		state: inferenceOwnerState{Version: 1, Attempts: make(map[string]*ownedAttempt)}}
	o.volatileFresh = cfg.InferenceOwnerFresh && cfg.DurableJournalPath == "" && retained == nil
	o.state.ContinuityKnown = o.volatileFresh
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
		o.storageUnavailable = true
		return
	}
	if !found {
		o.state.ContinuityKnown = o.journal.Fresh()
		return
	}
	var saved inferenceOwnerState
	if json.Unmarshal(data, &saved) != nil || !validInferenceOwnerState(saved) {
		o.storageUnavailable = true
		return
	}
	o.state = saved
	// Retained entries are evidence, never a complete census of a prior owner.
	o.state.ContinuityKnown = false
}

func (o *attemptOwner) beginLocked(id, callID, preparationID string) (AttemptStatus, error) {
	if o.storageUnavailable {
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
		Known: o.state.ContinuityKnown, Attempts: make([]AttemptStatus, 0, len(o.state.Attempts))}
	for _, a := range o.state.Attempts {
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
		return
	}
	if o.journal == nil {
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
		err = o.journal.WriteInferenceOwner(context.Background(), data, terminal)
	}
	if err != nil {
		o.state.ContinuityKnown = false
	}
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
	if state.Version != 1 || state.Attempts == nil {
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
	o.persistLocked(true, "")
	o.mu.Unlock()
	return o.snapshot()
}
