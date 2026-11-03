package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// messageCapacityWait identifies one queue head that is blocked only by the
// enabled session ceiling. The registry is process-local and intentionally
// passive after a restart: boots never start an agent to replay pending input.
type messageCapacityWait struct {
	taskID        string
	sessionID     string
	incarnationID string
	entryID       string
	queuedAt      time.Time
	// startup owns the bounded ticket acquired before a CREATED start or
	// completed-session resume. It is released once the session leaves that
	// state, or when the startup fails.
	startup *messageTurnReservation
}

func (w messageCapacityWait) key() string {
	if w.incarnationID != "" {
		return w.incarnationID
	}
	return w.sessionID
}

type messageCapacityWaitRegistry struct {
	mu    sync.Mutex
	waits map[string]messageCapacityWait
}

func newMessageCapacityWaitRegistry() *messageCapacityWaitRegistry {
	return &messageCapacityWaitRegistry{waits: make(map[string]messageCapacityWait)}
}

func (r *messageCapacityWaitRegistry) register(wait messageCapacityWait) {
	if r == nil || wait.sessionID == "" {
		return
	}
	r.mu.Lock()
	if existing, ok := r.waits[wait.key()]; ok && wait.startup == nil {
		wait.startup = existing.startup
	}
	r.waits[wait.key()] = wait
	r.mu.Unlock()
}

func sameMessageCapacityWait(left, right messageCapacityWait) bool {
	return left.taskID == right.taskID && left.sessionID == right.sessionID &&
		left.incarnationID == right.incarnationID && left.entryID == right.entryID
}

func (r *messageCapacityWaitRegistry) removeIfCurrent(key string, expected messageCapacityWait) {
	if r == nil || key == "" {
		return
	}
	r.mu.Lock()
	if current, ok := r.waits[key]; ok && sameMessageCapacityWait(current, expected) {
		delete(r.waits, key)
	}
	r.mu.Unlock()
}

func (r *messageCapacityWaitRegistry) setStartup(key string, ticket *messageTurnReservation) {
	if r == nil || key == "" {
		return
	}
	r.mu.Lock()
	wait, ok := r.waits[key]
	if ok {
		wait.startup = ticket
		r.waits[key] = wait
	}
	r.mu.Unlock()
}

func (r *messageCapacityWaitRegistry) clearStartupIfCurrent(key string, expected messageCapacityWait) {
	if r == nil || key == "" {
		return
	}
	r.mu.Lock()
	if current, ok := r.waits[key]; ok && sameMessageCapacityWait(current, expected) {
		current.startup = nil
		r.waits[key] = current
	}
	r.mu.Unlock()
}

func (r *messageCapacityWaitRegistry) snapshot() []messageCapacityWait {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	waits := make([]messageCapacityWait, 0, len(r.waits))
	for _, wait := range r.waits {
		waits = append(waits, wait)
	}
	return waits
}

func (r *messageCapacityWaitRegistry) len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.waits)
}

// RegisterMessageCapacityWait records an accepted queue head that a direct
// request persisted because the session ceiling had no capacity. It does not
// dispatch; the shared sweep retries when capacity is available.
func (s *Service) RegisterMessageCapacityWait(ctx context.Context, taskID, sessionID, entryID string) {
	if s == nil || s.messageQueue == nil {
		return
	}
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		// Keep the accepted input in the only retry index. The sweep can resolve
		// the incarnation again after a transient read failure.
		s.messageCapacityWaitRegistry().register(messageCapacityWait{
			taskID: taskID, sessionID: sessionID, entryID: entryID,
		})
		return
	}
	s.registerMessageCapacityWait(ctx, identity, entryID, time.Time{})
}

// registerMessageCapacityWait records the accepted queue head after a capacity
// refusal. It publishes the queue status so the pending input is visible even
// though no dispatch occurred.
func (s *Service) registerMessageCapacityWait(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	entryID string,
	queuedAt time.Time,
) {
	if s == nil || identity.SessionID == "" {
		return
	}
	registry := s.messageCapacityWaitRegistry()
	entryID, queuedAt = s.resolveMessageCapacityWaitHead(ctx, identity, entryID, queuedAt)
	registry.register(messageCapacityWait{
		taskID: identity.TaskID, sessionID: identity.SessionID,
		incarnationID: identity.SessionIncarnationID, entryID: entryID, queuedAt: queuedAt,
	})
	if s.messageQueue != nil {
		s.publishQueueStatusEventForIdentity(context.WithoutCancel(ctx), identity)
	}
}

// resolveMessageCapacityWaitHead fills the accepted head identity from the
// queue snapshot when the caller did not already supply the entry ID or time.
func (s *Service) resolveMessageCapacityWaitHead(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	entryID string,
	queuedAt time.Time,
) (string, time.Time) {
	if s.messageQueue == nil || (entryID != "" && !queuedAt.IsZero()) {
		return entryID, queuedAt
	}
	status, err := s.messageQueue.Snapshot(ctx, identity)
	if err != nil || status == nil || len(status.Entries) == 0 {
		return entryID, queuedAt
	}
	if entryID == "" {
		entryID = status.Entries[0].ID
	}
	if queuedAt.IsZero() {
		queuedAt = status.Entries[0].QueuedAt
	}
	return entryID, queuedAt
}

// messageCapacityWaitRegistry returns the process-local wait registry,
// creating it lazily for focused test services built without NewService.
func (s *Service) messageCapacityWaitRegistry() *messageCapacityWaitRegistry {
	s.messageCapacityWaitsOnce.Do(func() {
		if s.messageCapacityWaits == nil {
			s.messageCapacityWaits = newMessageCapacityWaitRegistry()
		}
	})
	return s.messageCapacityWaits
}

// deferQueuedMessageForCapacity restores the exact queue head after a capacity
// refusal and registers it for the existing ceiling sweep. Restoration keeps
// the ID, position, payload, and delivery ledger; no second prompt copy is
// created.
func (s *Service) deferQueuedMessageForCapacity(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
	reservation *queuedDispatchReservation,
	refusal *messageTurnCapacityRefusal,
) {
	if queuedMsg == nil {
		return
	}
	identity := messagequeue.QueueSessionIdentity{TaskID: queuedMsg.TaskID, SessionID: queuedMsg.SessionID}
	if reservation != nil && reservation.identity.SessionIncarnationID != "" {
		identity = reservation.identity
		s.restoreQueuedMessageForSession(ctx, identity, queuedMsg)
	} else {
		if s.messageQueue != nil {
			if resolved, err := s.messageQueue.ResolveSessionIdentity(ctx, queuedMsg.TaskID, queuedMsg.SessionID); err == nil {
				identity = resolved
			}
		}
		s.restoreQueuedMessage(ctx, queuedMsg)
	}
	s.registerMessageCapacityWait(ctx, identity, queuedMsg.ID, queuedMsg.QueuedAt)
	reasonCode := ceilingReasonRefused
	if refusal != nil && refusal.reasonCode != "" {
		reasonCode = refusal.reasonCode
	}
	s.logger.Info("queued message retained until session ceiling capacity is available",
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("session_id", queuedMsg.SessionID),
		zap.String("queue_id", queuedMsg.ID),
		zap.String(ceilingFieldReasonCode, reasonCode))
}

// MetaKeyCompletedFollowUpIntent marks a queued message that a caller
// explicitly addressed to a supported completed session. Only the agent-message
// handler sets it; raw client queue metadata cannot grant the resume permission.
const MetaKeyCompletedFollowUpIntent = "completed_follow_up_intent"

// retryMessageCapacityWaits is the message half of the shared ceiling sweep.
// Each pass attempts at most one drain per wait. A refusal restores the entry
// and re-registers it, so the next pass (or a release signal) retries.
func (s *Service) retryMessageCapacityWaits(ctx context.Context) {
	if s == nil || s.messageQueue == nil || s.sessionCeiling == nil {
		return
	}
	registry := s.messageCapacityWaitRegistry()
	waits := registry.snapshot()
	if len(waits) == 0 {
		return
	}
	sort.Slice(waits, func(i, j int) bool {
		return s.messageCapacityWaitLess(ctx, waits[i], waits[j])
	})
	for _, wait := range waits {
		if ctx.Err() != nil {
			return
		}
		if s.retryMessageCapacityWait(ctx, registry, wait) {
			return
		}
	}
}

func (s *Service) messageCapacityWaitLess(ctx context.Context, left, right messageCapacityWait) bool {
	if leftRank, rightRank := s.messageCapacityTaskRank(ctx, left.taskID), s.messageCapacityTaskRank(ctx, right.taskID); leftRank != rightRank {
		return leftRank > rightRank
	}
	if !left.queuedAt.Equal(right.queuedAt) {
		return left.queuedAt.Before(right.queuedAt)
	}
	if left.sessionID != right.sessionID {
		return left.sessionID < right.sessionID
	}
	return left.entryID < right.entryID
}

func (s *Service) dropTerminalMessageCapacityWait(
	registry *messageCapacityWaitRegistry, wait messageCapacityWait, err error,
) bool {
	if !isTerminalCapacityWaitError(err) {
		return false
	}
	s.releaseWaitStartup(registry, wait)
	registry.removeIfCurrent(wait.key(), wait)
	return true
}

// messageCapacityWaitHead validates the current queue head against the accepted
// identity. It returns false when the wait should not continue this pass.
func (s *Service) messageCapacityWaitHead(
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
	identity messagequeue.QueueSessionIdentity,
	status *messagequeue.QueueStatus,
) (messagequeue.QueuedMessage, bool) {
	if status == nil {
		return messagequeue.QueuedMessage{}, false
	}
	if len(status.Entries) == 0 {
		s.releaseWaitStartup(registry, wait)
		registry.removeIfCurrent(wait.key(), wait)
		return messagequeue.QueuedMessage{}, false
	}
	head := status.Entries[0]
	if wait.entryID != "" && head.ID != wait.entryID {
		// The accepted head was removed or superseded. Track the new head
		// instead of dispatching a different entry in the same pass.
		registry.register(messageCapacityWait{
			taskID: identity.TaskID, sessionID: identity.SessionID,
			incarnationID: identity.SessionIncarnationID, entryID: head.ID, queuedAt: head.QueuedAt,
		})
		return messagequeue.QueuedMessage{}, false
	}
	return head, true
}

// retryMessageCapacityWait attempts one registered wait. It returns true when
// the pass must stop because capacity became unavailable.
func (s *Service) retryMessageCapacityWait(
	ctx context.Context,
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
) bool {
	identity, err := s.messageCapacityWaitIdentity(ctx, wait)
	if err != nil {
		s.dropTerminalMessageCapacityWait(registry, wait, err)
		return false
	}
	status, err := s.messageQueue.Snapshot(ctx, identity)
	if err != nil {
		s.dropTerminalMessageCapacityWait(registry, wait, err)
		return false
	}
	head, ok := s.messageCapacityWaitHead(registry, wait, identity, status)
	if !ok {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, identity.SessionID)
	if err != nil {
		s.dropTerminalMessageCapacityWait(registry, wait, err)
		return false
	}
	if session == nil {
		s.releaseWaitStartup(registry, wait)
		registry.removeIfCurrent(wait.key(), wait)
		return false
	}
	if wait.startup != nil && !s.sessionCeiling.ownsTurnReservation(wait.startup) {
		// A released or expired startup ticket no longer authorizes the wait;
		// clear the handle and let this pass reacquire under fresh ownership.
		registry.clearStartupIfCurrent(wait.key(), wait)
		wait.startup = nil
	}
	if wait.startup != nil {
		return s.settleMessageCapacityWaitStartup(ctx, registry, wait, status.AutoRun, session)
	}
	if !status.AutoRun {
		return false
	}
	switch session.State {
	case models.TaskSessionStateCreated:
		return s.startCreatedSessionForCapacityWait(ctx, registry, wait, identity, session)
	case models.TaskSessionStateCompleted:
		if !boolField(head.Metadata, MetaKeyCompletedFollowUpIntent) {
			registry.removeIfCurrent(wait.key(), wait)
			return false
		}
		return s.resumeCompletedSessionForCapacityWait(ctx, registry, wait, identity)
	}
	if !s.sessionCeiling.turnCapacityAvailableForSession(ctx, identity.SessionID) {
		return true
	}
	outcome := s.drainQueuedMessageForPromptableSessionWithTaskAdmissionAndIdentity(
		ctx, identity.TaskID, identity.SessionID, &identity,
	)
	s.finishMessageCapacityWaitDrain(registry, wait, session, outcome)
	return false
}

func (s *Service) finishMessageCapacityWaitDrain(
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
	session *models.TaskSession,
	outcome queueDrainOutcome,
) {
	if outcome != queueDrainSkipped || !isTerminalSessionState(session.State) {
		return
	}
	s.releaseWaitStartup(registry, wait)
	registry.removeIfCurrent(wait.key(), wait)
}

// settleMessageCapacityWaitStartup keeps a startup ticket until its session
// leaves CREATED/COMPLETED, then releases it so the normal drain can continue.
func (s *Service) settleMessageCapacityWaitStartup(
	ctx context.Context,
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
	autoRun bool,
	session *models.TaskSession,
) bool {
	if session.State == models.TaskSessionStateCreated || session.State == models.TaskSessionStateCompleted {
		if !autoRun {
			s.releaseWaitStartup(registry, wait)
		}
		return false
	}
	s.releaseWaitStartup(registry, wait)
	s.signalCeilingSweep()
	return false
}

func (s *Service) releaseWaitStartup(registry *messageCapacityWaitRegistry, wait messageCapacityWait) {
	if wait.startup != nil {
		wait.startup.release()
	}
	registry.clearStartupIfCurrent(wait.key(), wait)
}

// capacityWaitStartup runs the start or resume work that owns the acquired
// ticket. It returns an error only when the attempt did not leave a counted
// session behind.
type capacityWaitStartup func(ctx context.Context) error

// startMessageCapacityWaitWithTicket acquires a bounded ticket before scheduling
// startup work and holds it until the session reaches a counted state. Only the
// attempt that owns the ticket may perform the work.
func (s *Service) startMessageCapacityWaitWithTicket(
	ctx context.Context,
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
	identity messagequeue.QueueSessionIdentity,
	session *models.TaskSession,
	startup capacityWaitStartup,
) bool {
	if s.messageCapacityStartupBlocked(ctx, session) {
		return false
	}
	ticket, refusal := s.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: identity.TaskID, sessionID: identity.SessionID,
		incarnationID: identity.SessionIncarnationID,
		intent:        turnCapacityBounded, seam: "message_wait_startup",
	})
	if refusal != nil || ticket == nil {
		return false
	}
	registry.setStartup(wait.key(), ticket)
	err := startup(withMessageTurnReservation(ctx, ticket))
	if err == nil || errors.Is(err, ErrAgentPromptInProgress) {
		return false
	}
	ticket.release()
	registry.clearStartupIfCurrent(wait.key(), wait)
	return false
}

// startCreatedSessionForCapacityWait runs a created-session startup under the
// bounded ticket so no sibling can observe the same final slot.
func (s *Service) startCreatedSessionForCapacityWait(
	ctx context.Context,
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
	identity messagequeue.QueueSessionIdentity,
	session *models.TaskSession,
) bool {
	return s.startMessageCapacityWaitWithTicket(ctx, registry, wait, identity, session,
		func(startupCtx context.Context) error {
			execution, err := s.startCreatedSessionWithComposedPrompt(
				startupCtx, identity.TaskID, identity.SessionID,
				session.AgentProfileID, "", "", "", true, false, false, false, nil, nil, false,
			)
			if err == nil && execution == nil {
				return errCapacityWaitStartupNoop
			}
			if err != nil && !errors.Is(err, ErrAgentPromptInProgress) {
				s.logger.Warn("failed to start session for capacity-waiting queued prompt",
					zap.String("task_id", identity.TaskID), zap.String("session_id", identity.SessionID), zap.Error(err))
			}
			return err
		},
	)
}

// resumeCompletedSessionForCapacityWait runs a completed-session resume under
// the bounded ticket so its explicit follow-up cannot exceed the ceiling.
func (s *Service) resumeCompletedSessionForCapacityWait(
	ctx context.Context,
	registry *messageCapacityWaitRegistry,
	wait messageCapacityWait,
	identity messagequeue.QueueSessionIdentity,
) bool {
	session, err := s.repo.GetTaskSession(ctx, identity.SessionID)
	if err != nil || session == nil {
		return false
	}
	return s.startMessageCapacityWaitWithTicket(ctx, registry, wait, identity, session,
		func(startupCtx context.Context) error {
			response, err := s.LaunchSession(startupCtx, &LaunchSessionRequest{
				TaskID: identity.TaskID, SessionID: identity.SessionID,
				Intent: IntentResume, AllowCompletedSessionResume: true,
			})
			if err == nil && response == nil {
				return errCapacityWaitStartupNoop
			}
			if err != nil {
				s.logger.Warn("failed to resume completed session for capacity wait",
					zap.String("task_id", identity.TaskID), zap.String("session_id", identity.SessionID), zap.Error(err))
			}
			return err
		},
	)
}

var errCapacityWaitStartupNoop = errors.New("capacity wait startup did not launch")

// messageCapacityStartupBlocked applies the same task, clarification, recovery,
// ownership, and routing barriers that an ordinary queue drain checks before it
// may start or resume a runtime.
func (s *Service) messageCapacityStartupBlocked(ctx context.Context, session *models.TaskSession) bool {
	if session == nil || s.isSessionResetInProgress(session.ID) ||
		s.isCancelInFlight(session.ID) || s.isQueuedDispatchInFlight(session.ID) || s.isSteerInFlight(session.ID) {
		return true
	}
	if s.checkSessionRecoveryBlock(ctx, session.ID) != nil {
		return true
	}
	if s.sessionHasLiveClarification(ctx, session.ID) || s.sessionHasPendingClarification(ctx, session.ID) {
		return true
	}
	if s.queueDrainBlockedByDynamicRoute(ctx, session.ID) || s.queueDrainBlockedByFixedProfile(ctx, session.ID) {
		return true
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil {
		return true
	}
	if !task.WIPAdmitted && task.QueuedForStepID != "" {
		return true
	}
	return managedConversationBlocksQueueDispatch(task)
}

func (s *Service) messageCapacityWaitIdentity(
	ctx context.Context, wait messageCapacityWait,
) (messagequeue.QueueSessionIdentity, error) {
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, wait.taskID, wait.sessionID)
	if err != nil {
		return messagequeue.QueueSessionIdentity{}, err
	}
	if wait.incarnationID != "" && identity.SessionIncarnationID != wait.incarnationID {
		return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
	}
	return identity, nil
}

// isTerminalCapacityWaitError distinguishes confirmed deletion or incarnation
// replacement from a transient read failure. Only confirmed terminal outcomes
// may drop the process-local retry index.
func isTerminalCapacityWaitError(err error) bool {
	return errors.Is(err, messagequeue.ErrSessionIdentityMismatch) ||
		errors.Is(err, models.ErrTaskSessionNotFound) ||
		errors.Is(err, repoerrors.ErrTaskNotFound) ||
		errors.Is(err, sql.ErrNoRows)
}

func (s *Service) messageCapacityTaskRank(ctx context.Context, taskID string) int {
	if s.repo == nil || taskID == "" {
		return queue.PriorityRank("")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return queue.PriorityRank("")
	}
	return queue.PriorityRank(task.Priority)
}
