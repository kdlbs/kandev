package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// Office decision-waiting detector.
//
// The stuck-signal watchdog next door only sees a task whose session accepted
// a completion signal. It cannot see the other Office stall: a task parked at
// a review or approval step where the seat that owes a decision never records
// one. Nothing is running, nothing failed, no signal is pending — the card
// simply stops moving, and the only present symptom is the absence of an
// event. This pass detects that state and reports it.
//
// It never acts on what it finds (REQ-OFFICE-STALL-VISIBILITY-003). Advancing
// the task would mean manufacturing the decision the quorum gate is there to
// require, so this file contains no write to workflow_step_decisions, no
// queued run, and no step transition — only a log line and a counter. That
// restraint is the feature, not a limitation of it.
const (
	// officeDecisionWaitingThreshold is how long an Office task may sit at a
	// step with an undecided decision-required seat before the detector
	// reports it.
	//
	// PROVISIONAL. 60 minutes is a first guess, not a measurement: a human
	// reviewer who steps away for ten minutes is behaving normally, and an
	// agent reviewer that has genuinely died is equally stuck at ten minutes
	// or at sixty, so the cost of waiting is only reporting latency while the
	// cost of being early is a log full of healthy tasks. Re-tune once the
	// counter has produced real distribution data.
	//
	// Deliberately NOT stuckSignalWatchdogThreshold. The two constants
	// measure unrelated phenomena — one an agent process that stopped
	// emitting events mid-turn, the other a decision nobody has made yet —
	// and sharing the value would make a future tuning of session silence
	// silently retune this, in the wrong direction and with no test to catch
	// it.
	officeDecisionWaitingThreshold = 60 * time.Minute
)

// officeDecisionCandidateLister is implemented by the durable task repository.
// A narrow optional interface, mirroring stuckSignalSessionLister, so test
// doubles that do not exercise this scan stay compatible.
type officeDecisionCandidateLister interface {
	ListOfficeDecisionWaitCandidates(
		ctx context.Context,
		quietSince time.Time,
		after *models.OfficeDecisionWaitCursor,
	) ([]models.OfficeDecisionWaitCandidate, *models.OfficeDecisionWaitCursor, error)
}

// officeRunInFlightReader answers whether a task still has a queued or claimed
// run. It is the detector's false-positive guard: a task with work in flight
// is being worked on, not stalled, however long its seat has gone undecided.
// Implemented by the Office SQLite repository (HasInFlightRunForTask) and
// wired from internal/backendapp, where that repository is constructed; the
// orchestrator does not otherwise reach the runs table, so this stays an
// injected dependency rather than a repo cast.
type officeRunInFlightReader interface {
	HasInFlightRunForTask(ctx context.Context, taskID string) (bool, error)
}

// SetOfficeRunInFlightReader wires the run-queue read the Office
// decision-waiting detector needs. Until it is wired the detector reports
// nothing and counts every evaluation as skipped, rather than reporting
// tasks it cannot prove are idle.
func (s *Service) SetOfficeRunInFlightReader(reader officeRunInFlightReader) {
	s.officeStallDependenciesMu.Lock()
	s.officeRunInFlight = reader
	s.officeStallDependenciesMu.Unlock()
}

// officeDecisionWaitingKey identifies one observation for dedupe purposes.
// Keyed by task and step so a task that moves to another decision step — or
// returns to this one after a rejection round — is reported again, while a
// task sitting still is reported once.
func officeDecisionWaitingKey(taskID, stepID string) string {
	return taskID + "\x00" + stepID
}

// detectOfficeDecisionWaitingOnce is the per-tick scan, called from the
// idle-session reaper's existing ticker alongside reclaimStuckSignalSessionsOnce
// rather than owning a second background goroutine. It walks bounded keyset
// pages until the scan budget expires or the candidate set is complete.
func (s *Service) detectOfficeDecisionWaitingOnce(ctx context.Context) {
	lister, ok := s.repo.(officeDecisionCandidateLister)
	if !ok {
		officeStallSkipped(officeStallSkipCandidateListUnwired)
		return
	}
	now := time.Now().UTC()
	scanCtx, cancel := context.WithTimeout(ctx, stuckSignalScanBudget)
	defer cancel()
	live := make(map[string]struct{})
	var cursor *models.OfficeDecisionWaitCursor
	scanned := 0
	for {
		if scanCtx.Err() != nil {
			s.logOfficeDecisionScanBudgetExceeded(scanned)
			return
		}
		candidates, next, err := lister.ListOfficeDecisionWaitCandidates(
			scanCtx, now.Add(-officeDecisionWaitingThreshold), cursor,
		)
		if err != nil {
			if scanCtx.Err() != nil {
				s.logOfficeDecisionScanBudgetExceeded(scanned)
				return
			}
			officeStallSkipped(officeStallSkipCandidateListFailed)
			s.logger.Warn("office stall watchdog: listing decision-wait candidates failed; tick skipped",
				zap.Error(err))
			return
		}
		for _, candidate := range candidates {
			if scanCtx.Err() != nil {
				s.logOfficeDecisionScanBudgetExceeded(scanned)
				return
			}
			scanned++
			if s.evaluateOfficeDecisionWaiting(scanCtx, candidate, now) {
				live[officeDecisionWaitingKey(candidate.TaskID, candidate.StepID)] = struct{}{}
			}
		}
		if next == nil || len(candidates) == 0 {
			break
		}
		cursor = next
	}
	s.pruneOfficeDecisionWaiting(live)
}

func (s *Service) logOfficeDecisionScanBudgetExceeded(scanned int) {
	s.logger.Warn("office stall watchdog: decision-wait scan budget exceeded; deferring remaining candidates to the next tick",
		zap.Int("candidates_scanned", scanned))
}

// evaluateOfficeDecisionWaiting applies the remaining predicate to one
// candidate, reports it if every clause holds, and reports whether the
// candidate should stay live for pruning purposes. The order is deliberate:
// the cheapest, most-selective reads run first and the runs-table read runs
// last, because most candidates are rejected before reaching it.
//
// Live must track the full predicate, not just the cheap SQL prefilter that
// produced the candidate: the query has no way to see a recorded decision or
// an in-flight run, so a resolved task keeps matching it for as long as
// tasks.updated_at stays quiet. Marking it live from the SQL match alone
// would keep its dedupe key past resolution, and a rework round that
// re-stalls the same task at the same step — without ever touching
// updated_at — would then find the old key already present and be silently
// swallowed by surfaceOfficeDecisionWaiting's LoadOrStore, contradicting
// officeDecisionWaitingKey's own "returns to this one after a rejection
// round — is reported again" contract.
//
// Every unreadable input fails closed to "do not report" and is counted. A
// detector that guessed on missing input would report healthy tasks, and the
// only thing this feature produces is reports. Failing closed here also
// means an unreadable input is not live, so a transient read failure costs at
// most one duplicate report on a later successful tick rather than risking a
// permanently swallowed one.
func (s *Service) evaluateOfficeDecisionWaiting(
	ctx context.Context, candidate models.OfficeDecisionWaitCandidate, now time.Time,
) bool {
	if !s.officeStepAwaitsDecision(ctx, candidate) {
		return false
	}
	_, _, reader := s.officeStallDependencies()
	if reader == nil {
		officeStallSkipped(officeStallSkipRunReaderUnwired)
		return false
	}
	inFlight, err := reader.HasInFlightRunForTask(ctx, candidate.TaskID)
	if err != nil {
		officeStallSkipped(officeStallSkipRunReaderError)
		return false
	}
	if inFlight {
		return false
	}
	s.surfaceOfficeDecisionWaiting(candidate, now)
	return true
}

// officeStepAwaitsDecision reports whether the candidate's current step holds
// a decision-required reviewer or approver seat with no decision recorded
// against it. Both stores are required: without them the detector cannot tell
// a genuinely undecided step from one it simply could not read.
func (s *Service) officeStepAwaitsDecision(
	ctx context.Context, candidate models.OfficeDecisionWaitCandidate,
) bool {
	participantsStore, decisionsStore, _ := s.officeStallDependencies()
	if participantsStore == nil {
		officeStallSkipped(officeStallSkipParticipantStore)
		return false
	}
	if decisionsStore == nil {
		officeStallSkipped(officeStallSkipDecisionStore)
		return false
	}
	participants, err := participantsStore.ListStepParticipants(ctx, candidate.StepID, candidate.TaskID)
	if err != nil {
		officeStallSkipped(officeStallSkipParticipantReadFailed)
		return false
	}
	if !hasDecidingSeat(participants) {
		return false
	}
	decisions, err := decisionsStore.ListStepDecisions(ctx, candidate.TaskID, candidate.StepID)
	if err != nil {
		officeStallSkipped(officeStallSkipDecisionReadFailed)
		return false
	}
	return !hasActiveDecision(decisions)
}

// hasActiveDecision reports whether any row is not superseded.
// ListStepDecisions returns superseded rows alongside active ones (see
// engine.DecisionInfo), and a rework round leaves the superseded row in
// place rather than deleting it, so a re-entered step must be judged on
// active rows only or it reads as permanently decided.
func hasActiveDecision(decisions []engine.DecisionInfo) bool {
	for _, d := range decisions {
		if d.SupersededAt == nil {
			return true
		}
	}
	return false
}

// hasDecidingSeat reports whether any seat at the step owes a decision.
// Watcher, collaborator and runner seats never do, and a reviewer seat with
// DecisionRequired false is an observer — none of them can strand a task, so
// none of them makes it a candidate.
func hasDecidingSeat(participants []engine.ParticipantInfo) bool {
	for _, p := range participants {
		if !p.DecisionRequired {
			continue
		}
		switch wfmodels.ParticipantRole(p.Role) {
		case wfmodels.ParticipantRoleReviewer, wfmodels.ParticipantRoleApprover:
			return true
		}
	}
	return false
}

// surfaceOfficeDecisionWaiting reports one decision-waiting task exactly once
// per (task, step). Reporting only — see this file's header for why there is
// no recovery here.
func (s *Service) surfaceOfficeDecisionWaiting(
	candidate models.OfficeDecisionWaitCandidate, now time.Time,
) {
	key := officeDecisionWaitingKey(candidate.TaskID, candidate.StepID)
	if _, seen := s.officeDecisionWaiting.LoadOrStore(key, struct{}{}); seen {
		return
	}
	officeStallDecisionWaitingTotal.Add(officeStallLabel("detector", "decision_waiting"), 1)
	s.logger.Warn("office stall watchdog: Office task is waiting on a decision that has not been recorded",
		zap.String("task_id", candidate.TaskID),
		zap.String("step_id", candidate.StepID),
		zap.Time("task_updated_at", candidate.UpdatedAt),
		zap.Duration("quiet_for", now.Sub(candidate.UpdatedAt)),
		zap.Duration("threshold", officeDecisionWaitingThreshold))
}

// pruneOfficeDecisionWaiting drops dedupe entries for tasks that are no longer
// candidates, so the map stays bounded by the number of currently-waiting
// tasks rather than growing for the process's lifetime. A task that becomes a
// candidate again after being pruned is correctly reported again: it is a new
// stall, not a repeat of the old one.
func (s *Service) pruneOfficeDecisionWaiting(live map[string]struct{}) {
	s.officeDecisionWaiting.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok {
			if _, still := live[k]; !still {
				s.officeDecisionWaiting.Delete(k)
			}
		}
		return true
	})
}
