package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// defaultAgentErrorVerdictTimeout bounds how long a deferred failure waits for
// a session-state event before the session row is read again.
const defaultAgentErrorVerdictTimeout = 2 * time.Minute

// Session states that decide a deferred failure. Literals mirror the task
// session states published on task_session.state_changed.
const (
	taskSessionStateCreated   = "CREATED"
	taskSessionStateStarting  = "STARTING"
	taskSessionStateFailed    = "FAILED"
	taskSessionStateCancelled = "CANCELLED"
)

// agentErrorVerdictState holds the opt-in session-verdict guard. When it is
// enabled, a task run's agent failure is not made terminal (and no
// on_agent_error replacement is dispatched) until the failed session records
// a failure verdict. A successor launched in the same session, for example
// by dynamic routing failover, keeps the run claimed so the successor's
// completion settles it instead of a second agent repeating the work.
type agentErrorVerdictState struct {
	enabled bool
	timeout time.Duration
	pending sync.Map // session ID -> *pendingAgentFailure
}

type pendingAgentFailure struct {
	runID string
	data  AgentLifecycleData
	timer *time.Timer
}

// SetAgentErrorSessionVerdict enables or disables the session-verdict guard.
func (s *Service) SetAgentErrorSessionVerdict(enabled bool) {
	s.agentErrorVerdict.enabled = enabled
}

// SetAgentErrorVerdictTimeout overrides the re-check delay for a deferred
// failure. Non-positive values restore the default.
func (s *Service) SetAgentErrorVerdictTimeout(timeout time.Duration) {
	s.agentErrorVerdict.timeout = timeout
}

// sessionVerdict classifies a session state observed after an agent failure.
type sessionVerdict int

const (
	sessionVerdictPending    sessionVerdict = iota // the failed execution's state is still current
	sessionVerdictFailed                           // the orchestrator recorded the failure
	sessionVerdictSuperseded                       // a successor owns the session
)

func classifySessionVerdict(state, errorMessage string) sessionVerdict {
	switch state {
	case taskSessionStateFailed, taskSessionStateCancelled:
		return sessionVerdictFailed
	case taskSessionStateWaitingForInput:
		if errorMessage != "" {
			return sessionVerdictFailed
		}
		return sessionVerdictSuperseded
	case "", taskSessionStateCreated, taskSessionStateStarting, taskSessionStateRunning:
		return sessionVerdictPending
	default:
		return sessionVerdictSuperseded
	}
}

// deferAgentFailureUntilSessionVerdict parks the failure when the guard is
// enabled and the session has not recorded its verdict yet. It returns false
// when the caller must apply the failure now. Exactly one of this call, the
// session-state handler, and the timeout re-check takes a parked failure.
func (s *Service) deferAgentFailureUntilSessionVerdict(
	ctx context.Context, run *runmodels.Run, data AgentLifecycleData,
) bool {
	if !s.agentErrorVerdict.enabled || data.SessionID == "" || run == nil {
		return false
	}
	timeout := s.agentErrorVerdict.timeout
	if timeout <= 0 {
		timeout = defaultAgentErrorVerdictTimeout
	}
	pending := &pendingAgentFailure{runID: run.ID, data: data}
	// The timer is armed before the entry is published so no taker ever
	// observes a partially initialized entry.
	pending.timer = time.AfterFunc(timeout, func() {
		s.recheckPendingAgentFailure(context.Background(), data.SessionID, pending)
	})
	if previous, loaded := s.agentErrorVerdict.pending.Swap(data.SessionID, pending); loaded {
		stopPendingTimer(previous.(*pendingAgentFailure))
	}
	state, errorMessage, err := s.repo.GetTaskSessionState(ctx, data.SessionID)
	verdict := sessionVerdictPending
	if err == nil {
		verdict = classifySessionVerdict(state, errorMessage)
	}
	if err != nil || verdict == sessionVerdictFailed {
		// An unreadable session fails closed, and a recorded failure keeps the
		// immediate failure path. Another taker already owns the failure
		// otherwise.
		if s.agentErrorVerdict.pending.CompareAndDelete(data.SessionID, pending) {
			stopPendingTimer(pending)
			return false
		}
		return true
	}
	if verdict == sessionVerdictSuperseded {
		// A clean park or terminal successor verdict is already recorded, so
		// this failure belongs to a predecessor turn and must be consumed
		// instead of failing the successor's run.
		s.takePendingAgentFailure(ctx, data.SessionID, pending, sessionVerdictSuperseded)
		return true
	}
	s.logger.Info("deferring Office run failure until the session records its verdict",
		zap.String("run_id", run.ID), zap.String("session_id", data.SessionID))
	s.AppendRunEvent(ctx, run.ID, "agent_error.deferred", "info", map[string]interface{}{
		"session_id": data.SessionID,
	})
	return true
}

// resolvePendingAgentFailure applies the verdict a session-state event carries
// to a failure parked for that session.
func (s *Service) resolvePendingAgentFailure(ctx context.Context, data TaskSessionStateData) {
	value, ok := s.agentErrorVerdict.pending.Load(data.SessionID)
	if !ok {
		return
	}
	pending := value.(*pendingAgentFailure)
	verdict := classifySessionVerdict(data.NewState, data.ErrorMessage)
	if verdict == sessionVerdictPending && data.NewState != taskSessionStateStarting {
		return
	}
	if verdict == sessionVerdictPending {
		// STARTING after a failure is a successor launch in the same session.
		verdict = sessionVerdictSuperseded
	}
	s.takePendingAgentFailure(ctx, data.SessionID, pending, verdict)
}

// recheckPendingAgentFailure reads the session again when no state event
// arrived in time. A still-live or already-superseded session keeps the run
// claimed for its current execution; a recorded failure applies the deferred
// failure. An unreadable session fails closed: a read error is not evidence of
// a successor, so the failure is applied rather than silently discarded.
func (s *Service) recheckPendingAgentFailure(ctx context.Context, sessionID string, pending *pendingAgentFailure) {
	state, errorMessage, err := s.repo.GetTaskSessionState(ctx, sessionID)
	verdict := sessionVerdictFailed
	if err == nil && classifySessionVerdict(state, errorMessage) != sessionVerdictFailed {
		verdict = sessionVerdictSuperseded
	}
	s.takePendingAgentFailure(ctx, sessionID, pending, verdict)
}

// hasPendingAgentFailure reports whether a failure is parked for the session.
func (s *Service) hasPendingAgentFailure(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	_, ok := s.agentErrorVerdict.pending.Load(sessionID)
	return ok
}

// dropPendingAgentFailure discards a parked failure once the session's run
// completed through a successor. The pending entry is keyed by session, but a
// completed event names one run: only that run may consume the entry. A
// completion without a trustworthy run identity (legacy task+agent fallback)
// leaves the entry parked, so a delayed completion for a finished run cannot
// erase the next run's failure.
func (s *Service) dropPendingAgentFailure(sessionID, runID string) {
	if sessionID == "" || runID == "" {
		return
	}
	value, loaded := s.agentErrorVerdict.pending.Load(sessionID)
	if !loaded {
		return
	}
	pending := value.(*pendingAgentFailure)
	if pending.runID != runID {
		return
	}
	if s.agentErrorVerdict.pending.CompareAndDelete(sessionID, pending) {
		stopPendingTimer(pending)
	}
}

func (s *Service) takePendingAgentFailure(
	ctx context.Context, sessionID string, pending *pendingAgentFailure, verdict sessionVerdict,
) {
	if !s.agentErrorVerdict.pending.CompareAndDelete(sessionID, pending) {
		return
	}
	stopPendingTimer(pending)
	if verdict != sessionVerdictFailed {
		s.logger.Info("Office run failure superseded by a successor in the same session",
			zap.String("run_id", pending.runID), zap.String("session_id", sessionID))
		s.AppendRunEvent(ctx, pending.runID, "agent_error.superseded", "info", map[string]interface{}{
			"session_id": sessionID,
		})
		return
	}
	run, err := s.repo.GetClaimedRunByID(ctx, pending.runID)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		s.logger.Warn("deferred Office run failure: claimed run lookup failed",
			zap.String("run_id", pending.runID), zap.Error(err))
		return
	}
	if err := s.failClaimedTaskRun(ctx, run, pending.data); err != nil {
		s.logger.Warn("deferred Office run failure could not be applied",
			zap.String("run_id", pending.runID), zap.Error(err))
	}
}

func stopPendingTimer(pending *pendingAgentFailure) {
	if pending != nil && pending.timer != nil {
		pending.timer.Stop()
	}
}
