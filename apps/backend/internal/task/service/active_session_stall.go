package service

import (
	"context"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	// defaultStallDetectionThreshold is the event-silence window after which
	// an execution-less active session is classified as stalled
	// (tasks.stallDetectionThreshold defaults to 2h).
	defaultStallDetectionThreshold = 2 * time.Hour
	// stallHealGraceMultiplier derives the interrupted-session recovery grace
	// window from the stall threshold. Recovery waits twice as long as
	// detection so an operator has a full threshold window to act on the
	// task.stalled event before the sweep recovers the orphaned session, and
	// so a live in-flight launch that has not yet registered its execution
	// never races the grace window.
	stallHealGraceMultiplier = 2
)

// stallThreshold returns the configured stall-detection threshold, falling
// back to the default when the setter was never called (zero value).
func (s *Service) stallThreshold() time.Duration {
	if s.stallDetectionThreshold > 0 {
		return s.stallDetectionThreshold
	}
	return defaultStallDetectionThreshold
}

// runActiveSessionSweep is the session reconciliation sweep's active-task
// pass: for every unarchived task that still holds an active session
// (CREATED/STARTING/RUNNING/WAITING_FOR_INPUT), it classifies each session
// against the in-memory execution store. A session with a registered live
// execution is healthy regardless of silence. A session with no live
// execution has lost its actor (backend restart, lost executor, or a launch
// that never registered) and nothing in any request path will ever advance
// it again, so the pass:
//
//   - emits one task.stalled event per stall episode once the session has
//     been event-silent beyond the stall threshold (detection only), and
//   - returns the session to WAITING_FOR_INPUT once the silence exceeds the
//     grace window (twice the threshold), preserving the same conversation
//     for lazy recovery and delivering the session.state_changed event clients
//     key off.
//
// The execution check is fail-closed: without the registry the pass cannot
// prove "no live execution", so it skips rather than healing or alerting on
// a guess. The event-silence clock is the newest of the session row's
// updated_at and its newest task_session_messages row, so any state
// transition or message resets it.
func (s *Service) runActiveSessionSweep(ctx context.Context, now time.Time) {
	if s.sessionExecutionRegistry == nil {
		return
	}
	tasks, err := s.tasks.ListUnarchivedTasksWithActiveSessions(ctx)
	if err != nil {
		s.logger.Error("active-session sweep: failed to list candidates", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		s.clearAllStallNotifications()
		return
	}
	s.pruneStallNotificationsToCandidates(tasks)

	threshold := s.stallThreshold()
	grace := threshold * stallHealGraceMultiplier
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		s.sweepTaskSessions(ctx, task, now, threshold, grace)
	}
}

// sweepTaskSessions runs the sweep classification for one candidate task.
func (s *Service) sweepTaskSessions(
	ctx context.Context,
	task *models.Task,
	now time.Time,
	threshold, grace time.Duration,
) {
	activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, task.ID)
	if err != nil {
		s.logger.Warn("active-session sweep: failed to list active sessions",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return
	}
	if len(activeSessions) == 0 {
		// Reconciled by a concurrent pass between the candidate list query
		// and this read.
		return
	}

	liveSessions := s.liveSessionSet(task.ID)
	orphaned := make([]*models.TaskSession, 0, len(activeSessions))
	for _, session := range activeSessions {
		if session == nil {
			continue
		}
		if _, live := liveSessions[session.ID]; !live {
			orphaned = append(orphaned, session)
		}
	}
	if len(orphaned) == 0 {
		s.clearStallNotifications(task.ID)
		return
	}

	s.retryPendingRecoverySettlements(ctx, task, activeSessions, liveSessions)
	orphaned, filterErr := s.excludeIdleWaitingSessions(ctx, orphaned)
	if filterErr != nil {
		// A turn read failure leaves the session's intent unknown. Do not
		// report or recover any part of this task from an incomplete snapshot.
		return
	}
	if len(orphaned) == 0 {
		s.clearStallNotifications(task.ID)
		return
	}

	stalled, healable, lastEventBySession, classErr := s.classifyOrphanedSessions(
		ctx, orphaned, now, threshold, grace,
	)
	if classErr != nil {
		// The per-session activity clock could not be read. updated_at alone
		// understates activity (message writes never touch it), so classifying
		// from the incomplete clock could stall-report or heal recently
		// active work. Fail closed: skip this task this tick and retry next
		// sweep.
		return
	}
	s.notifyStalledSessions(ctx, task, stalled, lastEventBySession, now, threshold)
	s.healOrphanedSessions(ctx, task, activeSessions, orphaned, healable, lastEventBySession)
}

// excludeIdleWaitingSessions removes ordinary waiting conversations from the
// execution-loss candidate set. WAITING_FOR_INPUT with no active turn is a
// healthy idle state and must neither emit task.stalled nor participate in
// task-wide healing. A turn read error fails closed for the whole task.
func (s *Service) excludeIdleWaitingSessions(
	ctx context.Context,
	candidates []*models.TaskSession,
) ([]*models.TaskSession, error) {
	filtered := make([]*models.TaskSession, 0, len(candidates))
	for _, session := range candidates {
		if session == nil || session.State != models.TaskSessionStateWaitingForInput {
			filtered = append(filtered, session)
			continue
		}
		if _, pending := models.LoadInterruptedRecoverySettlement(session.Metadata); pending {
			// A prior recovery already owns this waiting row. The retry pass
			// above handles its effects; it must not be reclassified as a new
			// stall episode while the settlement marker remains.
			continue
		}
		turn, err := s.GetActiveTurn(ctx, session.ID)
		if err != nil {
			s.logger.Warn("active-session sweep: failed to inspect waiting session turn; skipping task",
				zap.String("session_id", session.ID),
				zap.Error(err))
			return nil, err
		}
		if turn != nil {
			filtered = append(filtered, session)
		}
	}
	return filtered, nil
}

// classifyOrphanedSessions splits execution-less sessions into stalled
// (event-silent beyond threshold) and healable (silent beyond the grace
// window), and reports each stalled session's resolved event time for the
// task.stalled payload. It returns an error when a session's activity clock
// cannot be read: silence classification is only sound on a complete clock,
// so callers must skip the task rather than fall back to a stale timestamp.
func (s *Service) classifyOrphanedSessions(
	ctx context.Context,
	orphaned []*models.TaskSession,
	now time.Time,
	threshold, grace time.Duration,
) (stalled, healable []*models.TaskSession, lastEventBySession map[string]time.Time, err error) {
	times, readErr := s.messages.GetLastMessageTimeBySessionIDs(ctx, sessionIDs(orphaned))
	if readErr != nil {
		s.logger.Warn("active-session sweep: failed to load last message times; skipping task this tick",
			zap.Error(readErr))
		return nil, nil, nil, readErr
	}
	lastEventBySession = make(map[string]time.Time, len(orphaned))
	for _, session := range orphaned {
		lastEvent := lastSessionEventAt(session, times)
		lastEventBySession[session.ID] = lastEvent
		silence := now.Sub(lastEvent)
		if silence < threshold {
			continue
		}
		stalled = append(stalled, session)
		if silence >= grace {
			healable = append(healable, session)
		}
	}
	return stalled, healable, lastEventBySession, nil
}

// sessionIDs extracts the non-empty session IDs from sessions.
func sessionIDs(sessions []*models.TaskSession) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session != nil && session.ID != "" {
			ids = append(ids, session.ID)
		}
	}
	return ids
}

// lastSessionEventAt resolves one session's event clock: the newer of its
// newest task_session_messages row (when one exists) and the session row's
// own updated_at, so any persisted state transition resets the silence.
func lastSessionEventAt(session *models.TaskSession, lastEventBySession map[string]time.Time) time.Time {
	lastEvent := session.UpdatedAt
	if lastMessage, ok := lastEventBySession[session.ID]; ok && lastMessage.After(lastEvent) {
		lastEvent = lastMessage
	}
	return lastEvent
}

// liveSessionSet reads the registry's live-execution snapshot for one task.
func (s *Service) liveSessionSet(taskID string) map[string]struct{} {
	ids := s.sessionExecutionRegistry.LiveSessionIDsForTask(taskID)
	live := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			live[id] = struct{}{}
		}
	}
	return live
}

// stallPayload field keys (task_id / workspace_id exist as constants for
// other features; these name this payload's own keys).
const (
	stallPayloadTaskIDKey      = "task_id"
	stallPayloadWorkspaceIDKey = "workspace_id"
)

// notifyStalledSessions emits the task.stalled warning and event for
// sessions not yet reported in this stall episode. See stallNotifiedSessions
// for the episode semantics. The payload's timing fields describe exactly
// the sessions it names: a session already reported earlier in the episode
// must not skew stalled_for/last_event_at for a newly reported sibling.
//
// Delivery is recorded only after the event publish succeeds, so a transient
// publish failure retries on the next sweep tick.
func (s *Service) notifyStalledSessions(
	ctx context.Context,
	task *models.Task,
	stalled []*models.TaskSession,
	lastEventBySession map[string]time.Time,
	now time.Time,
	threshold time.Duration,
) {
	if s.stallNotifiedSessions == nil {
		s.stallNotifiedSessions = make(map[string]map[string]struct{})
	}
	notified := s.stallNotifiedSessions[task.ID]
	if len(stalled) == 0 {
		if notified != nil {
			delete(s.stallNotifiedSessions, task.ID)
		}
		return
	}
	notified = s.pruneStallEpisode(task.ID, stalled)
	newlyStalled, oldestNewEvent := selectNewlyStalled(stalled, notified, lastEventBySession)
	if len(newlyStalled) == 0 {
		return
	}

	sessionIDs := make([]string, 0, len(newlyStalled))
	for _, session := range newlyStalled {
		sessionIDs = append(sessionIDs, session.ID)
	}
	sort.Strings(sessionIDs)
	silence := now.Sub(oldestNewEvent)
	s.logger.Warn("stalled_task detected: active session with no live execution and no recent events",
		zap.String("task_id", task.ID),
		zap.Strings("session_ids", sessionIDs),
		zap.Duration("silent_for", silence),
		zap.Duration("threshold", threshold))
	if s.eventBus == nil {
		return
	}
	payload := map[string]interface{}{
		stallPayloadTaskIDKey:      task.ID,
		stallPayloadWorkspaceIDKey: task.WorkspaceID,
		"session_ids":              sessionIDs,
		"stalled_for":              silence.String(),
		"last_event_at":            oldestNewEvent.UTC().Format(time.RFC3339Nano),
		"detection_only":           true,
	}
	event := bus.NewEvent(events.TaskStalled, "task-reconciliation", payload)
	if err := s.eventBus.Publish(ctx, events.TaskStalled, event); err != nil {
		s.logger.Error("failed to publish stalled task event",
			zap.String("task_id", task.ID),
			zap.Error(err))
		return
	}
	for _, sessionID := range sessionIDs {
		notified[sessionID] = struct{}{}
	}
}

// healOrphanedSessions returns execution-less sessions that have been silent
// beyond the grace window to WAITING_FOR_INPUT. It preserves the conversation
// and settles only the turn observed by the compare-and-set snapshot. No
// cancellation effects run: questions, queued work, and workflow ownership
// remain available for lazy recovery.
//
// Healing is task-scoped at the eligibility boundary, so it only runs when
// every active session of the task is execution-less and past the grace
// window. The write itself is candidate-scoped, so a same-session resume or a
// newer turn cannot be changed by a stale sweep snapshot.
//
// The guard is re-evaluated at the recovery boundary itself: the
// liveness snapshot was taken before the silence reads, and an execution
// that registered (or a session that started) in between must abort the
// heal. Re-checking LiveSessionIDsForTask immediately before the write is
// not a lock — a registration can still land after it — but it closes the
// window from "seconds of reads ago" to "instantaneous", and any residual
// race must also co-occur with every session of the task having been
// silent past twice the stall threshold. The next sweep tick heals once
// the condition genuinely holds.
func (s *Service) healOrphanedSessions(
	ctx context.Context,
	task *models.Task,
	activeSessions, orphaned, healable []*models.TaskSession,
	lastEventBySession map[string]time.Time,
) {
	if len(healable) == 0 || len(orphaned) != len(activeSessions) || len(healable) != len(orphaned) {
		return
	}
	if live := s.liveSessionSet(task.ID); len(live) > 0 {
		s.logger.Info("active-session sweep: heal aborted, a live execution appeared for the task",
			zap.String("task_id", task.ID),
			zap.Int("live_sessions", len(live)))
		return
	}
	repo, ok := s.sessions.(interface {
		RecoverTaskSessionByCandidate(context.Context, models.ActiveSessionRecoveryCandidate, time.Time) (*models.TaskSession, error)
	})
	if !ok {
		return
	}
	candidates := make([]models.ActiveSessionRecoveryCandidate, 0, len(healable))
	for _, session := range healable {
		turn, err := s.GetActiveTurn(ctx, session.ID)
		if err != nil {
			s.logger.Warn("active-session sweep: failed to inspect current turn; skipping heal",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			return
		}
		lastEvent := lastEventBySession[session.ID]
		if lastEvent.IsZero() {
			lastEvent = session.UpdatedAt
		}
		candidate := models.ActiveSessionRecoveryCandidate{
			TaskID:              task.ID,
			SessionID:           session.ID,
			ExpectedState:       session.State,
			ExpectedUpdatedAt:   session.UpdatedAt,
			ExpectedLastEventAt: lastEvent,
		}
		if turn != nil {
			candidate.ExpectedTurnID = turn.ID
		}
		if err := s.captureRecoveryExecutorSnapshot(ctx, &candidate); err != nil {
			s.logger.Warn("active-session sweep: failed to capture executor reservation; skipping heal",
				zap.String("task_id", task.ID),
				zap.String("session_id", session.ID),
				zap.Error(err))
			return
		}
		candidates = append(candidates, candidate)
	}
	s.logger.Info("active-session sweep: preserving interrupted sessions for lazy recovery",
		zap.String("task_id", task.ID),
		zap.Strings("session_ids", recoveryCandidateIDs(candidates)))
	deadline := archivecascade.ArchiveDeadline(ctx)
	healCtx, cancel := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancel()
	for _, candidate := range candidates {
		if !time.Now().Before(deadline) {
			return
		}
		if _, live := s.liveSessionSet(task.ID)[candidate.SessionID]; live {
			continue
		}
		recovered, err := repo.RecoverTaskSessionByCandidate(healCtx, candidate, time.Time{})
		if err != nil {
			s.logger.Warn("active-session sweep: failed to recover interrupted session",
				zap.String("task_id", task.ID),
				zap.String("session_id", candidate.SessionID),
				zap.Error(err))
			continue
		}
		if recovered == nil {
			continue
		}
		completionCtx, cancelCompletion := context.WithTimeout(
			context.WithoutCancel(ctx), taskPublicationTimeout,
		)
		s.settleRecoveredSession(completionCtx, candidate, recovered)
		cancelCompletion()
	}
}

// retryPendingRecoverySettlements retries cross-repository effects that did
// not finish during an earlier recovery write. The session metadata carries
// the original turn and executor snapshot, so this pass never reads a
// successor identity merely to repair it.
func (s *Service) retryPendingRecoverySettlements(
	ctx context.Context,
	task *models.Task,
	activeSessions []*models.TaskSession,
	liveSessions map[string]struct{},
) {
	if task == nil {
		return
	}
	for _, session := range activeSessions {
		if session == nil || session.ID == "" {
			continue
		}
		if _, live := liveSessions[session.ID]; live {
			continue
		}
		settlement, ok := models.LoadInterruptedRecoverySettlement(session.Metadata)
		if !ok {
			continue
		}
		candidate := models.ActiveSessionRecoveryCandidate{
			TaskID:                           task.ID,
			SessionID:                        session.ID,
			ExpectedState:                    settlement.ExpectedState,
			RecoveredUpdatedAt:               settlement.RecoveredUpdatedAt,
			ExpectedTurnID:                   settlement.ExpectedTurnID,
			ExpectedExecutorID:               settlement.ExpectedExecutorID,
			ExpectedExecutorAgentExecutionID: settlement.ExpectedExecutorAgentExecutionID,
			ExpectedExecutorUpdatedAt:        settlement.ExpectedExecutorUpdatedAt,
		}
		completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), taskPublicationTimeout)
		s.settleRecoveredSession(completionCtx, candidate, session)
		cancel()
	}
}

func recoveryCandidateIDs(candidates []models.ActiveSessionRecoveryCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.SessionID)
	}
	return ids
}

// pruneStallNotificationsToCandidates removes stale task entries while the
// candidate query is healthy. A task that disappears from the query can no
// longer have an active stall episode, so retaining its entry leaks memory and
// suppresses a future episode if the task later becomes active again. Query
// failures return before this helper and therefore preserve the prior map.
func (s *Service) pruneStallNotificationsToCandidates(tasks []*models.Task) {
	candidateIDs := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if task != nil && task.ID != "" {
			candidateIDs[task.ID] = struct{}{}
		}
	}
	for taskID := range s.stallNotifiedSessions {
		if _, present := candidateIDs[taskID]; !present {
			delete(s.stallNotifiedSessions, taskID)
		}
	}
}

// pruneStallEpisode maintains the per-session episode set for one task:
// entries for sessions no longer in the current stalled set are removed (so
// a later stall on that session reports a fresh episode even while a
// sibling of the same task remains stalled), and the task's entry is
// created when it does not exist yet. It returns the live set to consult.
func (s *Service) pruneStallEpisode(taskID string, stalled []*models.TaskSession) map[string]struct{} {
	notified := s.stallNotifiedSessions[taskID]
	if notified == nil {
		notified = make(map[string]struct{}, len(stalled))
		s.stallNotifiedSessions[taskID] = notified
	}
	stalledIDs := make(map[string]struct{}, len(stalled))
	for _, session := range stalled {
		stalledIDs[session.ID] = struct{}{}
	}
	for sessionID := range notified {
		if _, still := stalledIDs[sessionID]; !still {
			delete(notified, sessionID)
		}
	}
	return notified
}

// selectNewlyStalled picks the stalled sessions not yet reported in the
// open episode and reports the oldest event time across that selection, so
// the payload's timing fields describe exactly the sessions it names.
func selectNewlyStalled(
	stalled []*models.TaskSession,
	notified map[string]struct{},
	lastEventBySession map[string]time.Time,
) (newlyStalled []*models.TaskSession, oldestNewEvent time.Time) {
	newlyStalled = make([]*models.TaskSession, 0, len(stalled))
	for _, session := range stalled {
		if _, seen := notified[session.ID]; seen {
			continue
		}
		newlyStalled = append(newlyStalled, session)
		lastEvent := lastEventBySession[session.ID]
		if oldestNewEvent.IsZero() || lastEvent.Before(oldestNewEvent) {
			oldestNewEvent = lastEvent
		}
	}
	return newlyStalled, oldestNewEvent
}

// clearStallNotifications ends a task's stall episode once it no longer has
// any stalled session, so a later stall on the same task reports again.
func (s *Service) clearStallNotifications(taskID string) {
	delete(s.stallNotifiedSessions, taskID)
}

// clearAllStallNotifications resets episode tracking when no candidates
// remain at all.
func (s *Service) clearAllStallNotifications() {
	s.stallNotifiedSessions = make(map[string]map[string]struct{})
}
