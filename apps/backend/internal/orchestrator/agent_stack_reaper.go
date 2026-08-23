package orchestrator

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Agent-stack reaping.
//
// A normal turn completion settles the session (WAITING_FOR_INPUT or
// COMPLETED) and the task (REVIEW) but leaves the ACP process tree alive
// waiting for the next prompt; agent.completed — the only event that tears a
// stack down — fires on process exit, which may never come. Idle stacks
// accumulate for days (the 2026-08-23 incident counted 11 stacks / ~3-3.5 GB
// RSS on one host) until a memory-hungry turn pushes the machine into swap
// thrash.
//
// Reaping deliberately does NOT fire on the REVIEW transition. REVIEW is
// written by setSessionWaitingForInput on *every* completed turn, so stopping
// there would delete warm-stack reuse outright: each follow-up prompt would
// pay a process relaunch plus session/load replay, lose the provider prompt
// cache, and — for an agent whose SessionConfig.SupportsRecovery() is false
// (Auggie) or that does not advertise the ACP LoadSession capability — fall
// back to re-injecting the whole transcript. The interactive follow-up window
// is exactly the window a REVIEW trigger would destroy, so idleness is
// measured, not assumed.
//
// Three triggers remain, in increasing order of bluntness:
//
//   - Task COMPLETED. The task is done; nobody is coming back to this stack.
//     Executor.StopByTaskID only reaches CREATED/STARTING/RUNNING/
//     WAITING_FOR_INPUT sessions, so IDLE (office fire-and-forget) and
//     COMPLETED sessions keep a live stack that nothing else tears down, and
//     markTaskCompletedForTerminalStep never stopped agents at all.
//   - Idle TTL (agentStackIdleTTL). Measured from the *session* row's
//     UpdatedAt, not the executors_running row's: the latter is refreshed by
//     execution persistence and status writes, so a long-lived stack that
//     just finished a turn would look ancient.
//   - Live-stack cap (agentStackLiveCap). Bounds concurrent stacks regardless
//     of timing, which is the shape the incident actually took: eleven
//     simultaneously-live stacks, not one very old one. Evicts the least
//     recently active reapable sessions first.
//
// All three share stopIdleSessionAgentStack, the single fail-closed stop
// primitive. Turn re-entry is guaranteed by the prompt path: promptTask calls
// ensureSessionRunning, which lazy-resumes a settled session with no live
// execution. Resume tokens, worktrees, and message history are untouched by
// the stop.
//
// Everything is gated by ServiceConfig.AgentStackReaping (runtime flag
// features.agentStackReaping, default ON; kill switch) and hard guards:
// never a working session state, never a session with an active turn, never a
// session with a prompt in admission, never when the turn service is
// unavailable (an uncertain signal is a skip, not a force). Failures log at
// warn and leave the row for the next tick.
const (
	// stopReasonAgentStackTaskCompleted is the StopAgentWithReason reason for
	// the task-COMPLETED lifecycle trigger.
	stopReasonAgentStackTaskCompleted = "agent stack reaping: task completed"

	// stopReasonAgentStackIdleTTL is the StopAgentWithReason reason for the
	// idle-TTL safety net.
	stopReasonAgentStackIdleTTL = "agent stack reaping: idle ttl"

	// stopReasonAgentStackOverCap is the StopAgentWithReason reason for a stop
	// forced by the concurrent live-stack cap.
	stopReasonAgentStackOverCap = "agent stack reaping: live stack cap"
)

// isReapableIdleSessionState reports whether a settled session state permits
// stack reaping. Deliberately mirrors the idle-reclaim state set: sessions
// still working (STARTING/RUNNING) are never stopped by reaping, and FAILED /
// CANCELLED sessions are owned by their own cancellation cleanup paths.
func isReapableIdleSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateIdle,
		models.TaskSessionStateCompleted:
		return true
	default:
		return false
	}
}

func agentStackReapingReasonForTaskState(state v1.TaskState) string {
	if state == v1.TaskStateCompleted {
		return stopReasonAgentStackTaskCompleted
	}
	return ""
}

func (s *Service) scheduleAgentStackStopForTaskState(taskID string, state v1.TaskState) {
	reason := agentStackReapingReasonForTaskState(state)
	if reason == "" {
		return
	}
	s.scheduleIdleAgentStackStopForTask(taskID, reason)
}

// agentStackSweeper owns the detached task-triggered sweep goroutines so
// Service.Stop can cancel them and join before returning, matching the
// goroutine-ownership invariant the idle reaper already follows. Task sweeps
// cannot be run inline (the callers hold taskRuntimeStateMu and
// StopAgentWithReason blocks on agentctl), so they need an owner of their own.
type agentStackSweeper struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	ctx     context.Context
	workers sync.WaitGroup
	started bool
}

func newAgentStackSweeper() *agentStackSweeper {
	return &agentStackSweeper{}
}

func (w *agentStackSweeper) start(parent context.Context) {
	if w == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	// context.WithoutCancel severs the triggering request's deadline — a sweep
	// must outlive the WebSocket call that caused the task transition — while
	// WithCancel re-attaches it to shutdown only.
	w.ctx, w.cancel = context.WithCancel(context.WithoutCancel(parent))
	w.started = true
}

// spawn runs fn in a tracked goroutine. Returns false when the sweeper is
// stopped or was never started, so callers can skip work during shutdown.
func (w *agentStackSweeper) spawn(fn func(ctx context.Context)) bool {
	if w == nil || fn == nil {
		return false
	}
	w.mu.Lock()
	if !w.started || w.ctx == nil || w.ctx.Err() != nil {
		w.mu.Unlock()
		return false
	}
	sweepCtx := w.ctx
	w.workers.Add(1)
	w.mu.Unlock()
	go func() {
		defer w.workers.Done()
		fn(sweepCtx)
	}()
	return true
}

func (w *agentStackSweeper) stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	w.mu.Unlock()
	w.workers.Wait()
	w.mu.Lock()
	w.cancel = nil
	w.ctx = nil
	w.started = false
	w.mu.Unlock()
}

// beginPromptAdmission marks sessionID as having a prompt inside its admission
// window and returns the release. promptTask holds this from before
// ensureSessionRunning until claimPromptDispatch has taken the session RUNNING
// and reserved a turn.
//
// Without it the reaper has a real window to kill a live prompt:
// ensureSessionRunning releases the per-session lifecycle lock on return, and
// claimPromptDispatch only re-takes it later, so in between the session still
// reads WAITING_FOR_INPUT with no active turn. A sweep landing there stops the
// execution the prompt is about to use, and because the execution existed
// before ensureSessionRunning, promptTask's resumedForPrompt fresh-launch
// fallback does not apply — the user's prompt just fails.
func (s *Service) beginPromptAdmission(sessionID string) func() {
	if sessionID == "" {
		return func() {}
	}
	s.promptAdmissionMu.Lock()
	if s.promptAdmission == nil {
		s.promptAdmission = make(map[string]int)
	}
	s.promptAdmission[sessionID]++
	s.promptAdmissionMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.promptAdmissionMu.Lock()
			defer s.promptAdmissionMu.Unlock()
			if s.promptAdmission[sessionID] <= 1 {
				delete(s.promptAdmission, sessionID)
				return
			}
			s.promptAdmission[sessionID]--
		})
	}
}

func (s *Service) hasPromptInAdmission(sessionID string) bool {
	s.promptAdmissionMu.Lock()
	defer s.promptAdmissionMu.Unlock()
	return s.promptAdmission[sessionID] > 0
}

// stopIdleSessionAgentStack stops the live agent execution backing one idle
// session, fail-closed. Returns true when a stop was issued. The caller may
// pass a session loaded moments earlier; the state is re-read under the
// per-session lifecycle lock so a turn that started between the trigger and
// this call is honored, not killed.
func (s *Service) stopIdleSessionAgentStack(ctx context.Context, session *models.TaskSession, reason string) bool {
	if !s.config.AgentStackReaping || s.repo == nil || s.agentManager == nil || reason == "" {
		return false
	}
	if session == nil || session.ID == "" {
		return false
	}
	releaseLifecycleLock := s.acquireSessionLifecycleLock(session.ID)
	defer releaseLifecycleLock()

	// Re-read under the lock: the trigger's snapshot may predate a new turn.
	current, ok := s.loadReapableIdleSession(ctx, session.ID)
	if !ok {
		return false
	}
	// Fail-closed on turn signals: no turn service, or an unreadable turn
	// state, means the session may be mid-conversation. sessionHasActiveTurn
	// already treats both as active.
	if s.sessionHasActiveTurn(ctx, session.ID) {
		return false
	}
	// A prompt between ensureSessionRunning and claimPromptDispatch owns this
	// execution even though the row still looks settled.
	if s.hasPromptInAdmission(session.ID) {
		return false
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, session.ID)
	if err != nil || executionID == "" {
		// No live execution tracked: nothing to stop here. The row-repair
		// side of cleanup belongs to reclaimIdleSession.
		return false
	}
	if s.hasExecutionTeardownOwner(session.ID, executionID) {
		return false
	}
	if err := s.agentManager.StopAgentWithReason(ctx, executionID, reason, false); err != nil {
		s.logger.Warn("agent stack reaping: stop failed; will retry on a later trigger",
			zap.String("session_id", session.ID),
			zap.String("execution_id", executionID),
			zap.String("reason", reason),
			zap.Error(err))
		return false
	}
	s.logger.Info("agent stack reaping: idle stack stopped; session preserved for resume",
		zap.String("task_id", current.TaskID),
		zap.String("session_id", session.ID),
		zap.String("execution_id", executionID),
		zap.String("reason", reason),
		zap.String("session_state", string(current.State)))
	return true
}

func (s *Service) loadReapableIdleSession(ctx context.Context, sessionID string) (*models.TaskSession, bool) {
	current, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if !isTaskSessionNotFound(err) {
			s.logger.Warn("agent stack reaping: session lookup failed; skipping",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return nil, false
	}
	if current == nil || !isReapableIdleSessionState(current.State) {
		return nil, false
	}
	return current, true
}

// stopIdleAgentStacksForTask sweeps a task's sessions and stops every stack
// that passes the fail-closed guards. Working sessions are skipped by the
// per-session state guard, so a multi-session task keeps its active stacks.
func (s *Service) stopIdleAgentStacksForTask(ctx context.Context, taskID, reason string) {
	if !s.config.AgentStackReaping || s.repo == nil || taskID == "" {
		return
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("agent stack reaping: task session list failed",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.stopIdleSessionAgentStack(ctx, session, reason)
	}
}

// scheduleIdleAgentStackStopForTask launches the stop sweep on the
// service-owned sweeper. The COMPLETED writers hold taskRuntimeStateMu and
// StopAgentWithReason can block on agentctl, so the sweep must not run inline;
// every guard is re-validated inside the goroutine, which also closes the
// CAS-to-stop race with a prompt arriving in between.
func (s *Service) scheduleIdleAgentStackStopForTask(taskID, reason string) {
	if !s.config.AgentStackReaping || s.repo == nil || taskID == "" {
		return
	}
	if s.stackSweeper.spawn(func(sweepCtx context.Context) {
		s.stopIdleAgentStacksForTask(sweepCtx, taskID, reason)
	}) {
		return
	}
	// The sweeper is not running (tests that never call Service.Start, or a
	// shutdown in flight). Falling back to an untracked goroutine would
	// reintroduce the leak, so log and let the idle-TTL tick catch the stack.
	s.logger.Debug("agent stack reaping: sweeper unavailable; deferring to the idle tick",
		zap.String("task_id", taskID),
		zap.String("reason", reason))
}

// agentStackCandidate is one reapable live stack considered by the cap pass,
// ordered by how long its session has been settled.
type agentStackCandidate struct {
	session  *models.TaskSession
	idleFrom time.Time
}

// enforceAgentStackCap bounds the number of concurrently live ACP stacks.
// Counting uses every non-stopped executors_running row (ADR 0003 makes that
// table the execution-id source of truth), so working sessions count toward
// the cap even though they can never be evicted; eviction only ever touches
// sessions that already pass the fail-closed reapable guards, oldest-idle
// first.
func (s *Service) enforceAgentStackCap(ctx context.Context, rows []*models.ExecutorRunning, now time.Time) {
	if !s.config.AgentStackReaping || s.repo == nil || s.idleReaper == nil {
		return
	}
	limit := s.idleReaper.stackLiveCap
	if limit <= 0 {
		return
	}
	live := liveAgentStackRows(rows)
	overflow := len(live) - limit
	if overflow <= 0 {
		return
	}
	// Only now is the per-session read worth its cost: under the cap the tick
	// does one list and nothing else.
	candidates := s.reapableStackCandidates(ctx, live)
	if len(candidates) == 0 {
		return
	}
	stopped := s.evictOldestIdleStacks(ctx, candidates, overflow)
	if stopped > 0 {
		s.logger.Info("agent stack reaping: live-stack cap enforced",
			zap.Int("live_stacks", len(live)),
			zap.Int("cap", limit),
			zap.Int("stopped", stopped),
			zap.Time("observed_at", now))
	}
}

func liveAgentStackRows(rows []*models.ExecutorRunning) []*models.ExecutorRunning {
	live := make([]*models.ExecutorRunning, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.SessionID == "" || row.Status == models.ExecutorRunningStatusStopped {
			continue
		}
		live = append(live, row)
	}
	return live
}

// reapableStackCandidates loads the sessions behind live rows and keeps the
// ones a stop may touch, sorted oldest-idle first.
func (s *Service) reapableStackCandidates(
	ctx context.Context,
	live []*models.ExecutorRunning,
) []agentStackCandidate {
	candidates := make([]agentStackCandidate, 0, len(live))
	for _, row := range live {
		session, err := s.repo.GetTaskSession(ctx, row.SessionID)
		if err != nil || session == nil || !isReapableIdleSessionState(session.State) {
			continue
		}
		candidates = append(candidates, agentStackCandidate{
			session:  session,
			idleFrom: sessionIdleSince(session, row),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].idleFrom.Before(candidates[j].idleFrom)
	})
	return candidates
}

// evictOldestIdleStacks stops up to overflow candidates and reports how many
// stops actually landed. Each one still runs the full fail-closed guard set.
func (s *Service) evictOldestIdleStacks(
	ctx context.Context,
	candidates []agentStackCandidate,
	overflow int,
) int {
	stopped := 0
	for _, candidate := range candidates {
		if stopped >= overflow || ctx.Err() != nil {
			break
		}
		if s.stopIdleSessionAgentStack(ctx, candidate.session, stopReasonAgentStackOverCap) {
			stopped++
		}
	}
	return stopped
}

// sessionIdleSince reports when a settled session last changed. The session
// row is the activity signal: executors_running.UpdatedAt is refreshed by
// execution persistence and status writes, so a stack launched hours ago that
// finished a turn seconds ago would otherwise read as ancient. The executor
// row is only a fallback for a session row with no usable timestamp.
func sessionIdleSince(session *models.TaskSession, row *models.ExecutorRunning) time.Time {
	if session != nil && !session.UpdatedAt.IsZero() {
		return session.UpdatedAt.UTC()
	}
	if session != nil && !session.StartedAt.IsZero() {
		return session.StartedAt.UTC()
	}
	if row != nil {
		return row.UpdatedAt.UTC()
	}
	return time.Time{}
}
