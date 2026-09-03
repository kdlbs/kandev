package clarification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ErrBundleNotFound is ResolveBundle's single "not found" outcome (A3, A5).
// An unknown pending_id, a denied AuthorizeTaskAccess, a bundle whose
// task_id cannot be resolved (M5a), and a bundle belonging to the separate
// autopilot parent-question protocol all collapse to this one sentinel by
// design, so REST/MCP layers map every one of them to the same 404 without
// distinguishing the cause.
var ErrBundleNotFound = errors.New("clarification bundle not found")

// errClarificationNotActive is R2's no-winner branch: the claim was lost and
// the re-read found no answered/rejected message either, so the bundle is
// simply not active (superseded turn, terminal session, expired, cancelled,
// or malformed). REST/MCP layers map it to 409 with an unchanged message.
var errClarificationNotActive = errors.New("clarification request is no longer active")

// IsNotActiveError reports whether err is R2's no-winner outcome, for
// REST/MCP layers deciding a 409 response.
func IsNotActiveError(err error) bool {
	return errors.Is(err, errClarificationNotActive)
}

// Resolution is ResolveBundle's result: the winning (or, on a lost claim,
// reconstructed - R2b) terminal status and response for a bundle.
type Resolution struct {
	PendingID string
	SessionID string
	TaskID    string
	Status    string
	Response  *Response
}

// taskAccessAuthorizer is step 2's authorization check ("The resolution
// claim", step 2).
type taskAccessAuthorizer interface {
	AuthorizeTaskAccess(ctx context.Context, taskID string) error
}

// clarificationResponseClaim carries a winning claim's committed rows through
// delivery. messages is immutable after claim construction: a delivery
// callback can outlive the responder's bounded wait, so callback results
// stay local.
type clarificationResponseClaim struct {
	response       *Response
	terminalStatus string
	messages       []*taskmodels.Message
}

// Resolver implements ResolveBundle
// (docs/specs/integrations/requirements/external-question-answering.md, "Resolution semantics"),
// the single service-layer operation the REST endpoints and the
// answer_question_kandev MCP tool both call. It resolves a bundle's identity
// and authorization (M5, A1-A9), validates the outcome (N6-N8b), claims it
// through upstream's CompleteActiveClarificationBundle (P1, R1, R3, R6), and
// on a win delivers through upstream's existing live-waiter/detached-resume
// path (R7) unchanged; on a loss it reconstructs the winner's outcome (R2).
type Resolver struct {
	store                   *Store
	repo                    handlerMessageStore
	messages                MessageCreator
	authorizer              taskAccessAuthorizer
	detachedResumer         DetachedClarificationResumer
	eventBus                EventBus
	primaryAnsweredNotifier PrimaryAnsweredNotifier
	logger                  *logger.Logger
	now                     func() time.Time // seam for deterministic tests
}

// NewResolver creates a Resolver.
func NewResolver(
	store *Store,
	repo handlerMessageStore,
	messages MessageCreator,
	authorizer taskAccessAuthorizer,
	detachedResumer DetachedClarificationResumer,
	eventBus EventBus,
	primaryAnsweredNotifier PrimaryAnsweredNotifier,
	log *logger.Logger,
) *Resolver {
	return &Resolver{
		store:                   store,
		repo:                    repo,
		messages:                messages,
		authorizer:              authorizer,
		detachedResumer:         detachedResumer,
		eventBus:                eventBus,
		primaryAnsweredNotifier: primaryAnsweredNotifier,
		logger:                  log.WithFields(zap.String("component", "clarification-resolver")),
		now:                     time.Now,
	}
}

// AuthorizeBundleAccess resolves a bundle's identity (M5) and authorizes the
// caller against its owning task (A2), without claiming or applying anything.
// REST read endpoints (GET /:id, GET /:id/wait) call this ahead of their
// in-memory read (A7) so an unauthorized caller gets ErrBundleNotFound
// exactly as A3 requires, and A5a's "no durable messages" case also
// collapses to the same sentinel. POST /:id/cancel calls this alone (A9):
// cancel does not go through ResolveBundle.
func (r *Resolver) AuthorizeBundleAccess(ctx context.Context, pendingID string) (sessionID, taskID string, err error) {
	_, sessionID, taskID, err = r.resolveIdentity(ctx, pendingID)
	if err != nil {
		return "", "", err
	}
	if err := r.authorizer.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return "", "", ErrBundleNotFound // A3
	}
	return sessionID, taskID, nil
}

// resolveIdentity is step 1 (M5, M5a): read the bundle's durable messages
// and derive session_id/task_id. A bundle with no messages, one whose
// task_id cannot be resolved from either source, or one belonging to the
// separate autopilot parent-question protocol (isParentQuestionBundle) is
// not-found.
func (r *Resolver) resolveIdentity(ctx context.Context, pendingID string) ([]*taskmodels.Message, string, string, error) {
	msgs, err := r.repo.FindMessagesByPendingID(ctx, pendingID)
	if err != nil {
		return nil, "", "", err
	}
	if len(msgs) == 0 {
		return nil, "", "", ErrBundleNotFound
	}
	if isParentQuestionBundle(msgs) {
		return nil, "", "", ErrBundleNotFound
	}
	sessionID, taskID, ok := r.resolveBundleTaskID(ctx, msgs)
	if !ok {
		return nil, "", "", ErrBundleNotFound // M5a
	}
	return msgs, sessionID, taskID, nil
}

// isParentQuestionBundle reports whether any message in the bundle carries
// the autopilot parent-question marker (models.MetaKeyParentQuestion,
// parent_question.go's parentQuestionMetadata). Those records share this
// bundle's message type and status/pending_id metadata keys, but they
// belong to a separate, ownership-gated resolution channel
// (message_task_kandev's reply_to_question_id, validateParentQuestionOwnership)
// that ResolveBundle and AuthorizeBundleAccess must never claim or read —
// doing so would let any workspace-authorized caller (REST or the external
// answer_question_kandev/list_pending_questions_kandev MCP tools) resolve a
// running autopilot agent's question to its parent, bypassing that
// ownership check and the parent-only interaction boundary the spec's S2
// relies on.
func isParentQuestionBundle(msgs []*taskmodels.Message) bool {
	for _, m := range msgs {
		if marker, _ := m.Metadata[taskmodels.MetaKeyParentQuestion].(bool); marker {
			return true
		}
	}
	return false
}

// resolveBundleTaskID implements M5: session_id is always present on the
// message row. task_id may be empty there; when so, it is resolved from the
// bundle's session row. When neither source yields one, ok is false (M5a).
func (r *Resolver) resolveBundleTaskID(ctx context.Context, msgs []*taskmodels.Message) (sessionID, taskID string, ok bool) {
	sessionID = msgs[0].TaskSessionID
	for _, m := range msgs {
		if m.TaskID != "" {
			return sessionID, m.TaskID, true
		}
	}
	session, err := r.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID == "" {
		return sessionID, "", false
	}
	return sessionID, session.TaskID, true
}

// ResolveBundle runs the ordered steps described in the spec: resolve
// identity, authorize, validate, claim, and (winner only) deliver. Cancel
// SHALL NOT be submitted here (A9): cancel is authorized via
// AuthorizeBundleAccess alone and never claims.
func (r *Resolver) ResolveBundle(ctx context.Context, pendingID string, outcome Outcome) (*Resolution, bool, error) {
	msgs, sessionID, taskID, err := r.resolveIdentity(ctx, pendingID)
	if err != nil {
		return nil, false, err
	}
	if err := r.authorizer.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, false, ErrBundleNotFound // A3
	}

	questions := bundleQuestions(msgs)
	if err := validateOutcome(questions, outcome); err != nil {
		return nil, false, err // N8c, R2c: validation runs before the claim
	}

	return r.claimAndDeliver(ctx, pendingID, sessionID, taskID, questions, outcome)
}

// claimAndDeliver is steps 4 and 5. R4: the durable claim commits before any
// resume event is published, so a loser can never trigger a resume.
func (r *Resolver) claimAndDeliver(
	ctx context.Context,
	pendingID, sessionID, taskID string,
	questions []Question,
	outcome Outcome,
) (*Resolution, bool, error) {
	status, response := buildOutcomeResponse(pendingID, questions, outcome, r.now())

	claimCtx, cancel := clarificationClaimContext(ctx)
	defer cancel()
	completedMessages, claimed, claimErr := r.messages.CompleteActiveClarificationBundle(
		claimCtx,
		pendingID,
		status,
		responsesFromAnswers(response.Answers),
	)
	if claimErr != nil {
		r.logger.Error("failed to claim clarification response",
			zap.String("pending_id", pendingID), zap.Error(claimErr))
		return nil, false, fmt.Errorf("failed to update clarification state: %w", claimErr) // R4a
	}
	if !claimed {
		return r.reconstructLoser(ctx, pendingID, sessionID, taskID) // R2
	}

	if err := r.deliverClaimedResolution(ctx, pendingID, status, response, completedMessages); err != nil {
		return nil, true, err
	}

	return &Resolution{
		PendingID: pendingID,
		SessionID: sessionID,
		TaskID:    taskID,
		Status:    status,
		Response:  response,
	}, true, nil
}

// responsesFromAnswers builds the map CompleteActiveClarificationBundle
// requires for an answered outcome, keyed by question_id. Returns nil for a
// rejection (empty Answers), matching upstream's own claim call.
func responsesFromAnswers(answers []Answer) map[string]interface{} {
	if len(answers) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(answers))
	for _, a := range answers {
		out[a.QuestionID] = a
	}
	return out
}

// reconstructLoser is R2/R2a/R2b: a lost claim re-reads the bundle's durable
// messages exactly once and branches on what it finds, without retrying and
// without claiming or modifying anything.
func (r *Resolver) reconstructLoser(ctx context.Context, pendingID, sessionID, taskID string) (*Resolution, bool, error) {
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	msgs, err := r.repo.FindMessagesByPendingID(persistenceCtx, pendingID)
	if err != nil {
		r.logger.Error("failed to re-read clarification bundle after lost claim",
			zap.String("pending_id", pendingID), zap.Error(err))
		return nil, false, fmt.Errorf("failed to determine clarification outcome: %w", err)
	}
	status, response, hasWinner := reconstructWinnerResolution(pendingID, msgs, r.logger)
	if !hasWinner {
		return nil, false, errClarificationNotActive
	}
	return &Resolution{
		PendingID: pendingID,
		SessionID: sessionID,
		TaskID:    taskID,
		Status:    status,
		Response:  response,
	}, false, nil
}

// reconstructWinnerResolution implements R2b. status is the status of the
// first message in D2 order whose status is answered or rejected; answers is
// built by walking the bundle's messages in D2 order and taking each
// message's metadata.response where present, in that order. reject_reason
// is always "" (R2b: upstream stores no reject reason on the message rows).
func reconstructWinnerResolution(pendingID string, msgs []*taskmodels.Message, log *logger.Logger) (string, *Response, bool) {
	ordered := orderBundleMessages(msgs)

	status := ""
	var respondedAt time.Time
	for _, m := range ordered {
		s := stringFromMetadata(m.Metadata, metaStatusKey)
		if s == string(StatusAnswered) || s == string(StatusRejected) {
			status = s
			respondedAt = m.UpdatedAt
			break
		}
	}
	if status == "" {
		return "", nil, false
	}

	answers := make([]Answer, 0, len(ordered))
	for _, m := range ordered {
		if a, ok := answerFromMessageMetadata(m.ID, m.Metadata, log); ok {
			answers = append(answers, a)
		}
	}

	return status, &Response{
		PendingID:    pendingID,
		Answers:      answers,
		Rejected:     status == string(StatusRejected),
		RejectReason: "",
		RespondedAt:  respondedAt,
	}, true
}

// answerFromMessageMetadata reads one message's stored metadata.response, if
// present, as an Answer. A message that carries no response contributes no
// entry. A malformed stored value is skipped and logged rather than aborting
// the whole reconstruction.
func answerFromMessageMetadata(messageID string, meta map[string]any, log *logger.Logger) (Answer, bool) {
	raw, present := meta["response"]
	if !present {
		return Answer{}, false
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		if log != nil {
			log.Warn("clarification message has malformed response metadata; skipping in replay",
				zap.String("message_id", messageID))
		}
		return Answer{}, false
	}
	answer := Answer{QuestionID: questionIDFromMetadata(meta)}
	if opts, ok := rawMap["selected_options"].([]interface{}); ok {
		for _, o := range opts {
			if s, ok := o.(string); ok {
				answer.SelectedOptions = append(answer.SelectedOptions, s)
			}
		}
	}
	if s, ok := rawMap["custom_text"].(string); ok {
		answer.CustomText = s
	}
	return answer, true
}

// deliverClaimedResolution is R7: the winner's delivery and resume path is
// upstream's, reached unchanged. It calls the existing
// Store.RespondWithDeliveryConfirmation path and SHALL NOT re-implement live
// delivery, detached resume dispatch, or the RestoreActiveClarificationBundle
// rollback.
func (r *Resolver) deliverClaimedResolution(
	ctx context.Context,
	pendingID, status string,
	response *Response,
	completedMessages []*taskmodels.Message,
) error {
	claim := &clarificationResponseClaim{
		response:       response,
		terminalStatus: status,
		messages:       completedMessages,
	}

	// Durable current-turn ownership is claimed before touching the live waiter.
	// This closes the window where a newer turn exists but its canceller has not
	// yet drained the superseded in-memory request.
	deliveryCtx, cancelDelivery := context.WithTimeout(
		context.WithoutCancel(ctx),
		clarificationPersistenceTimeout+5*time.Second,
	)
	defer cancelDelivery()
	// The callback may outlive this handler's bounded wait. Keep its result in
	// callback-owned storage so the immutable claim remains safe for recovery.
	finalizedMessages := make(chan []*taskmodels.Message, 1)
	deliveryErr := r.store.RespondWithDeliveryConfirmation(
		deliveryCtx,
		pendingID,
		claim.response,
		func() error {
			finalized, confirmErr := r.confirmLiveClarificationResponseDelivery(ctx, pendingID, claim)
			if confirmErr == nil {
				r.notifyPrimaryAnsweredBeforeWaiter(
					ctx,
					pendingID,
					response.Answers,
					response.Rejected,
					response.RejectReason,
					r.clarificationClaimTurnID(pendingID, finalized),
				)
				finalizedMessages <- finalized
			}
			return confirmErr
		},
	)
	if deliveryErr == nil {
		finalized := <-finalizedMessages
		r.publishClarificationBundleUpdates(ctx, pendingID, finalized)
		r.logger.Info("clarification answered via primary path (same turn)",
			zap.String("pending_id", pendingID),
			zap.Int("answers", len(response.Answers)),
			zap.Bool("rejected", response.Rejected))
		return nil
	}
	if errors.Is(deliveryErr, ErrAlreadyResponded) {
		// Defensive only: the durable claim above is exclusive. Retain this as a
		// safety net if the in-memory waiter ever diverges from durable state.
		if finalized, ok := r.finalizeClarificationResponseDelivery(ctx, pendingID, claim); ok {
			r.publishClarificationBundleUpdates(ctx, pendingID, finalized)
		}
		r.logger.Warn("duplicate response attempt", zap.String("pending_id", pendingID))
		return errors.New("response already submitted")
	}
	if !errors.Is(deliveryErr, ErrNotFound) {
		restored := r.restoreFailedClarificationClaim(ctx, pendingID, claim.terminalStatus, claim.messages)
		r.logger.Error("failed to deliver clarification response",
			zap.String("pending_id", pendingID),
			zap.Bool("restored", restored),
			zap.Error(deliveryErr))
		if restored {
			return errors.New("failed to deliver clarification response; response can be retried")
		}
		return errors.New("failed to deliver clarification response and recover pending clarification state")
	}
	return r.deliverDetachedClarificationResponse(ctx, pendingID, response, claim, deliveryErr)
}

func (r *Resolver) deliverDetachedClarificationResponse(
	ctx context.Context,
	pendingID string,
	response *Response,
	claim *clarificationResponseClaim,
	deliveryErr error,
) error {
	// Fallback path: entry not found (agent timed out, entry was cleaned up).
	// If the user rejected (clicked X to dismiss), they're discarding a stale
	// overlay — not continuing the conversation. Treat as a no-op so we don't
	// surprise them by resuming the agent with "User declined to answer".
	if response.Rejected {
		finalized, ok := r.finalizeClarificationResponseDelivery(ctx, pendingID, claim)
		if !ok {
			return errors.New("clarification rejection was recorded, but delivery state could not be finalized")
		}
		r.publishStaleDismissedEvent(ctx, pendingID)
		r.publishClarificationBundleUpdates(ctx, pendingID, finalized)
		r.logger.Info("clarification rejected after agent moved on; no-op",
			zap.String("pending_id", pendingID))
		return nil
	}

	// User is providing an affirmative answer after the agent moved on. Ask the
	// orchestrator to accept a new turn containing the answer before publishing
	// the terminal clarification update.
	r.logger.Info("clarification entry not found, using acknowledged resume fallback",
		zap.String("pending_id", pendingID),
		zap.String("error", deliveryErr.Error()))

	if err := r.resumeDetachedClarification(
		ctx, pendingID, response.Answers, response.Rejected, response.RejectReason, claim.messages,
	); err != nil {
		if detachedResumeWasAccepted(err) {
			// The prompt reached agentctl. Keep the durable answer terminal so a
			// retry cannot dispatch it again, even though turn publication failed
			// and the caller must receive a server error.
			if finalized, ok := r.finalizeClarificationResponseDelivery(ctx, pendingID, claim); ok {
				r.publishClarificationBundleUpdates(ctx, pendingID, finalized)
			}
			r.logger.Error("accepted clarification resume was not durably published",
				zap.String("pending_id", pendingID), zap.Error(err))
			return errors.New("clarification response was accepted, but dispatch state could not be finalized")
		}
		restored := r.restoreFailedClarificationClaim(ctx, pendingID, claim.terminalStatus, claim.messages)
		r.logger.Error("failed to resume detached clarification",
			zap.String("pending_id", pendingID), zap.Error(err))
		if restored {
			return errors.New("failed to resume clarification; response can be retried")
		}
		return errors.New("failed to resume clarification and recover pending clarification state")
	}
	finalized, ok := r.finalizeClarificationResponseDelivery(ctx, pendingID, claim)
	if !ok {
		return errors.New("clarification response was accepted, but delivery state could not be finalized")
	}
	r.publishClarificationBundleUpdates(ctx, pendingID, finalized)
	return nil
}

func (r *Resolver) confirmLiveClarificationResponseDelivery(
	ctx context.Context,
	pendingID string,
	claim *clarificationResponseClaim,
) ([]*taskmodels.Message, error) {
	finalizedMessages, finalized := r.finalizeClarificationResponseDelivery(ctx, pendingID, claim)
	if !finalized {
		return nil, errors.New("clarification delivery confirmation failed")
	}
	return finalizedMessages, nil
}

func (r *Resolver) finalizeClarificationResponseDelivery(
	ctx context.Context,
	pendingID string,
	claim *clarificationResponseClaim,
) ([]*taskmodels.Message, bool) {
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	finalizedMessages, finalized, err := r.messages.FinalizeClarificationResponseDelivery(
		persistenceCtx,
		pendingID,
		claim.terminalStatus,
		claim.messages,
	)
	if err != nil {
		r.logger.Error("failed to finalize clarification response delivery",
			zap.String("pending_id", pendingID), zap.Error(err))
		return nil, false
	}
	if !finalized {
		r.logger.Warn("clarification response delivery was not finalizable",
			zap.String("pending_id", pendingID))
		return nil, false
	}
	// The initial claim (completeClaimedClarificationMessages) is the only
	// write that touches updated_at; finalization only clears an internal
	// delivery-recovery marker, so the column is stable from the moment the
	// claim commits. Stamp the winner's own response with that same
	// column value (claim.response is the same pointer returned to the
	// caller) so both it and a later caller's R2b reconstruction, which
	// re-reads this column, report the identical instant.
	if claim.response != nil && len(finalizedMessages) > 0 {
		claim.response.RespondedAt = finalizedMessages[0].UpdatedAt
	}
	return finalizedMessages, true
}

func (r *Resolver) clarificationClaimTurnID(pendingID string, messages []*taskmodels.Message) string {
	turnID, err := clarificationClaimTurnIdentity(messages)
	if err != nil {
		r.logger.Warn("clarification bundle has inconsistent turn IDs",
			zap.String("pending_id", pendingID), zap.Error(err))
		return ""
	}
	return turnID
}

func clarificationClaimTurnIdentity(messages []*taskmodels.Message) (string, error) {
	turnID := ""
	turnMessageID := ""
	for _, message := range messages {
		if message == nil || message.ID == "" || message.TurnID == "" {
			continue
		}
		if turnID == "" {
			turnID = message.TurnID
			turnMessageID = message.ID
			continue
		}
		if message.TurnID != turnID {
			return "", fmt.Errorf(
				"clarification bundle mixes turn %q from message %s with turn %q from message %s",
				turnID, turnMessageID, message.TurnID, message.ID,
			)
		}
	}
	return turnID, nil
}

// notifyPrimaryAnsweredBeforeWaiter is the synchronous local ordering
// boundary. It must complete before the delivery-confirmation callback returns
// to Store, because Store then releases the live waiter. Event-bus fan-out is
// deliberately kept after this notifier and is never used as its acknowledgement.
func (r *Resolver) notifyPrimaryAnsweredBeforeWaiter(
	ctx context.Context,
	pendingID string,
	answers []Answer,
	rejected bool,
	rejectReason, clarificationTurnID string,
) {
	if r.eventBus == nil && r.primaryAnsweredNotifier == nil {
		return
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	clarificationCtx, err := r.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil {
		r.logger.Warn("failed to resolve context for primary clarification event",
			zap.String("pending_id", pendingID), zap.Error(err))
		return
	}
	if clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		r.logger.Warn("missing session/task for primary clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.String("task_id", clarificationCtx.TaskID))
		return
	}

	answerText := buildAnswerSummary(clarificationCtx.Questions, answers, rejected, rejectReason)
	answered := PrimaryAnswered{
		SessionID:           clarificationCtx.SessionID,
		TaskID:              clarificationCtx.TaskID,
		PendingID:           pendingID,
		ClarificationTurnID: clarificationTurnID,
		Question:            clarificationCtx.QuestionSummary,
		AnswerText:          answerText,
		Rejected:            rejected,
		RejectReason:        rejectReason,
	}
	if r.primaryAnsweredNotifier != nil {
		r.primaryAnsweredNotifier(persistenceCtx, answered)
	}
	r.publishPrimaryAnsweredEvent(persistenceCtx, answered)
}

// publishPrimaryAnsweredEvent is asynchronous fan-out only. The local
// watchdog notifier is intentionally not called from this method: NATS
// publication returns before its subscribers process the event.
func (r *Resolver) publishPrimaryAnsweredEvent(ctx context.Context, answered PrimaryAnswered) {
	if r.eventBus == nil {
		return
	}
	eventData := map[string]any{
		metaSessionIDKey:        answered.SessionID,
		metaTaskIDKey:           answered.TaskID,
		metaPendingIDKey:        answered.PendingID,
		"clarification_turn_id": answered.ClarificationTurnID,
		metaQuestionKey:         answered.Question,
		"answer_text":           answered.AnswerText,
		metaRejectedKey:         answered.Rejected,
		"reject_reason":         answered.RejectReason,
		// The orchestrator handled this event synchronously in the resolver's
		// process. Its event-bus subscription must not arm a second watchdog.
		"handled_inline": r.primaryAnsweredNotifier != nil,
	}
	if err := r.eventBus.Publish(ctx, events.ClarificationPrimaryAnswered, bus.NewEvent(
		events.ClarificationPrimaryAnswered,
		"clarification-resolver",
		eventData,
	)); err != nil {
		r.logger.Warn("failed to publish primary clarification event",
			zap.String("pending_id", answered.PendingID),
			zap.String("session_id", answered.SessionID),
			zap.Error(err))
	}
}

func (r *Resolver) publishClarificationBundleUpdates(
	ctx context.Context,
	pendingID string,
	messages []*taskmodels.Message,
) {
	publishCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	if err := r.messages.PublishClarificationBundleUpdates(publishCtx, messages); err != nil {
		r.logger.Error("failed to publish clarification bundle updates",
			zap.String("pending_id", pendingID), zap.Error(err))
	}
}

func (r *Resolver) restoreFailedClarificationClaim(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*taskmodels.Message,
) bool {
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	restoredMessages, restored, err := r.messages.RestoreActiveClarificationBundle(
		persistenceCtx, pendingID, terminalStatus, claimedMessages,
	)
	if err != nil {
		r.logger.Error("failed to restore clarification after delivery failure",
			zap.String("pending_id", pendingID), zap.Error(err))
		return false
	}
	if !restored {
		r.logger.Warn("clarification claim was not restorable after delivery failure",
			zap.String("pending_id", pendingID))
		return false
	}
	if err := r.messages.PublishClarificationBundleUpdates(persistenceCtx, restoredMessages); err != nil {
		r.logger.Error("failed to converge restored clarification state",
			zap.String("pending_id", pendingID), zap.Error(err))
		// The durable bundle is pending again, so retry remains safe even when
		// its best-effort live projection could not be acknowledged.
		return true
	}
	return true
}

func (r *Resolver) publishStaleDismissedEvent(ctx context.Context, pendingID string) {
	if r.eventBus == nil {
		return
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	clarificationCtx, err := r.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil || clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		r.logger.Warn("failed to resolve context for stale-dismissed clarification event",
			zap.String("pending_id", pendingID), zap.Error(err))
		return
	}
	eventData := map[string]any{
		"session_id": clarificationCtx.SessionID,
		"task_id":    clarificationCtx.TaskID,
		"pending_id": pendingID,
	}
	if err := r.eventBus.Publish(persistenceCtx, events.ClarificationStaleDismissed, bus.NewEvent(
		events.ClarificationStaleDismissed,
		"clarification-resolver",
		eventData,
	)); err != nil {
		r.logger.Warn("failed to publish stale-dismissed clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.Error(err))
	}
}

// resumeDetachedClarification synchronously asks the orchestrator to accept a
// new turn. Used when the original clarification waiter has gone away.
func (r *Resolver) resumeDetachedClarification(
	ctx context.Context,
	pendingID string,
	answers []Answer,
	rejected bool,
	rejectReason string,
	claimedMessages []*taskmodels.Message,
) error {
	if r.detachedResumer == nil {
		return errors.New("detached clarification resumer unavailable")
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()

	clarificationCtx, err := r.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil {
		return fmt.Errorf("resolve clarification fallback context: %w", err)
	}
	if clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		return fmt.Errorf(
			"missing session/task for clarification fallback event: session=%q task=%q",
			clarificationCtx.SessionID, clarificationCtx.TaskID,
		)
	}

	answerText := buildAnswerSummary(clarificationCtx.Questions, answers, rejected, rejectReason)
	clarificationTurnID, claimedMessageIDs, err := clarificationClaimRecovery(claimedMessages)
	if err != nil {
		return fmt.Errorf("build clarification claim recovery: %w", err)
	}

	request := DetachedClarificationResume{
		SessionID:           clarificationCtx.SessionID,
		TaskID:              clarificationCtx.TaskID,
		PendingID:           pendingID,
		ClarificationTurnID: clarificationTurnID,
		ClaimedMessageIDs:   claimedMessageIDs,
		Question:            clarificationCtx.QuestionSummary,
		AnswerText:          answerText,
		Rejected:            rejected,
		RejectReason:        rejectReason,
	}
	if err := r.detachedResumer.ResumeDetachedClarification(persistenceCtx, request); err != nil {
		return fmt.Errorf("resume detached clarification: %w", err)
	}

	r.logger.Info("clarification answered via acknowledged fallback (new turn)",
		zap.String("pending_id", pendingID),
		zap.String("session_id", clarificationCtx.SessionID),
		zap.String("task_id", clarificationCtx.TaskID))
	return nil
}

func clarificationClaimRecovery(messages []*taskmodels.Message) (string, []string, error) {
	turnID, err := clarificationClaimTurnIdentity(messages)
	if err != nil {
		return "", nil, err
	}
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		if message == nil || message.ID == "" {
			continue
		}
		messageIDs = append(messageIDs, message.ID)
	}
	return turnID, messageIDs, nil
}

func detachedResumeWasAccepted(err error) bool {
	var accepted interface{ DetachedResumeAccepted() bool }
	return errors.As(err, &accepted) && accepted.DetachedResumeAccepted()
}

type clarificationEventContext struct {
	SessionID       string
	TaskID          string
	Questions       []Question // Source-of-truth questions used to label answers; falls back to a single synthetic Question when only metadata is available.
	QuestionSummary string     // Pre-formatted multi-line "Q1: ...\nQ2: ..." used by the orchestrator resume prompt.
}

func (r *Resolver) resolveClarificationEventContext(ctx context.Context, pendingID string) (clarificationEventContext, error) {
	var out clarificationEventContext

	if req, ok := r.store.GetRequest(pendingID); ok && req != nil {
		out.SessionID = req.SessionID
		out.TaskID = req.TaskID
		out.Questions = req.Questions
		out.QuestionSummary = formatQuestionSummary(req.Questions)
		if out.SessionID != "" && out.TaskID != "" && len(out.Questions) > 0 {
			return out, nil
		}
	}

	if r.repo == nil {
		return out, fmt.Errorf("message repository unavailable")
	}

	msgs, err := r.repo.FindMessagesByPendingID(ctx, pendingID)
	if err != nil {
		return out, err
	}
	if len(msgs) == 0 {
		return out, fmt.Errorf("no messages for pending_id %s", pendingID)
	}

	if out.SessionID == "" {
		out.SessionID = msgs[0].TaskSessionID
	}
	if out.TaskID == "" {
		out.TaskID = msgs[0].TaskID
	}
	if len(out.Questions) == 0 {
		out.Questions = questionsFromMessages(msgs)
		out.QuestionSummary = formatQuestionSummary(out.Questions)
	}

	return out, nil
}

// questionsFromMessages reconstructs a Question slice from persisted
// clarification messages, ordered by metadata.question_index so the rebuilt
// summary matches the bundle the agent originally sent. Used as a fallback
// when the in-store request has been cleaned up but the persisted metadata
// still carries the original question text. Unlike bundleQuestions
// (bundle_question.go), used for validation and answer application, this
// does not parse options: it feeds only the resume-summary text.
func questionsFromMessages(msgs []*taskmodels.Message) []Question {
	sorted := make([]*taskmodels.Message, len(msgs))
	copy(sorted, msgs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return questionIndexFromMetadata(sorted[i].Metadata) < questionIndexFromMetadata(sorted[j].Metadata)
	})
	out := make([]Question, 0, len(sorted))
	for _, m := range sorted {
		out = append(out, questionFromMessageMetadata(m.Metadata))
	}
	return out
}

func questionFromMessageMetadata(meta map[string]any) Question {
	q := Question{ID: stringFromMetadata(meta, metaQuestionIDKey)}
	qData, ok := meta[metaQuestionKey].(map[string]any)
	if !ok {
		return q
	}
	if v, ok := qData["prompt"].(string); ok {
		q.Prompt = v
	}
	if v, ok := qData["title"].(string); ok {
		q.Title = v
	}
	if q.ID == "" {
		if v, ok := qData["id"].(string); ok {
			q.ID = v
		}
	}
	return q
}

func questionIndexFromMetadata(meta map[string]any) int {
	if meta == nil {
		return 0
	}
	switch v := meta["question_index"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func formatQuestionSummary(questions []Question) string {
	if len(questions) == 0 {
		return ""
	}
	if len(questions) == 1 {
		return questions[0].Prompt
	}
	parts := make([]string, 0, len(questions))
	for i, q := range questions {
		parts = append(parts, fmt.Sprintf("Q%d: %s", i+1, q.Prompt))
	}
	return strings.Join(parts, "\n")
}

// buildAnswerSummary constructs a human-readable summary of the user's response
// across every question in the bundle. Used in the orchestrator resume prompt
// and for chat history rendering.
func buildAnswerSummary(questions []Question, answers []Answer, rejected bool, rejectReason string) string {
	if rejected {
		if rejectReason != "" {
			return fmt.Sprintf("User declined to answer. Reason: %s", rejectReason)
		}
		return "User declined to answer."
	}
	if len(answers) == 0 {
		return "User provided no specific answer."
	}
	if len(questions) <= 1 && len(answers) == 1 {
		return formatSingleAnswer(answers[0])
	}

	answersByID := make(map[string]Answer, len(answers))
	for _, a := range answers {
		answersByID[a.QuestionID] = a
	}

	parts := make([]string, 0, len(answers))
	for i, q := range questions {
		ans, ok := answersByID[q.ID]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("A%d: %s", i+1, formatAnswerBody(ans)))
	}
	if len(parts) == 0 {
		// No matches by id — fall back to positional formatting so we still
		// surface the answers rather than silently dropping them.
		for i, a := range answers {
			parts = append(parts, fmt.Sprintf("A%d: %s", i+1, formatAnswerBody(a)))
		}
	}
	return strings.Join(parts, "\n")
}

func formatSingleAnswer(a Answer) string {
	if a.CustomText != "" {
		return fmt.Sprintf("User answered: %s", a.CustomText)
	}
	if len(a.SelectedOptions) > 0 {
		return fmt.Sprintf("User selected: %v", a.SelectedOptions)
	}
	return "User provided no specific answer."
}

func formatAnswerBody(a Answer) string {
	if a.CustomText != "" {
		return a.CustomText
	}
	if len(a.SelectedOptions) > 0 {
		return fmt.Sprintf("%v", a.SelectedOptions)
	}
	return "(no answer)"
}
