package clarification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"go.uber.org/zap"
)

// ErrBundleNotFound is ResolveBundle's single "not found" outcome (A3, A5).
// An unknown pending_id, a denied AuthorizeTaskAccess, a bundle whose
// task_id cannot be resolved (M5a), and a claim whose session_id foreign key
// cannot be satisfied (M8a) or whose conflicting row has vanished (M9) all
// collapse to this one sentinel by design, so REST/MCP layers map every one
// of them to the same 404 without distinguishing the cause.
var ErrBundleNotFound = errors.New("clarification bundle not found")

// partialApplicationError is returned when a winning claim's per-question
// message updates fail partway through (R5, R5a). REST/MCP layers map it
// to 500; the applying request SHALL NOT receive a success response.
type partialApplicationError struct{ msg string }

func (e *partialApplicationError) Error() string { return e.msg }

// IsPartialApplicationError reports whether err is a partial-application
// failure (R5) for REST/MCP layers deciding a 500 response.
func IsPartialApplicationError(err error) bool {
	var pe *partialApplicationError
	return errors.As(err, &pe)
}

// Resolution is ResolveBundle's result: the resolutions row's current
// content, whichever caller originally claimed it.
type Resolution struct {
	PendingID  string
	SessionID  string
	TaskID     string
	Status     string
	Response   *Response
	Resume     string
	ResolvedBy string
	Source     string
	ResolvedAt time.Time
}

// resolutionStore is the minimal repository interface ResolveBundle needs
// for claiming and reading clarification_resolutions rows (M7, M8, M8a, M9).
type resolutionStore interface {
	InsertClarificationResolution(ctx context.Context, res *taskmodels.ClarificationResolution) (bool, *taskmodels.ClarificationResolution, error)
	UpdateClarificationResolutionResume(ctx context.Context, pendingID, resume string) error
}

// taskAccessAuthorizer is step 2's authorization check ("The resolution
// claim", step 2).
type taskAccessAuthorizer interface {
	AuthorizeTaskAccess(ctx context.Context, taskID string) error
}

// clarificationMessageUpdater is the narrow slice of MessageCreator
// ResolveBundle's step 5 needs (D2, R4, R5, R5a, R9a). It is satisfied
// structurally by the same MessageCreator implementation the REST handlers
// already use.
type clarificationMessageUpdater interface {
	UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, questionID, status string, answer *Answer) error
}

// Resolver implements ResolveBundle
// (docs/specs/external-question-answering/spec.md, "The resolution claim"),
// the single service-layer operation the REST endpoints and the
// answer_question_kandev MCP tool both call.
type Resolver struct {
	store       *Store
	resolutions resolutionStore
	repo        messageStore
	messages    clarificationMessageUpdater
	authorizer  taskAccessAuthorizer
	eventBus    EventBus
	logger      *logger.Logger
	now         func() time.Time // seam for deterministic tests
}

// NewResolver creates a Resolver.
func NewResolver(
	store *Store,
	resolutions resolutionStore,
	repo messageStore,
	messages clarificationMessageUpdater,
	authorizer taskAccessAuthorizer,
	eventBus EventBus,
	log *logger.Logger,
) *Resolver {
	return &Resolver{
		store:       store,
		resolutions: resolutions,
		repo:        repo,
		messages:    messages,
		authorizer:  authorizer,
		eventBus:    eventBus,
		logger:      log.WithFields(zap.String("component", "clarification-resolver")),
		now:         time.Now,
	}
}

// AuthorizeBundleAccess resolves a bundle's identity (M5) and authorizes the
// caller against its owning task (A2), without claiming or applying anything.
// REST read endpoints (GET /:id, GET /:id/wait) call this ahead of their
// in-memory read (A7) so an unauthorized caller gets ErrBundleNotFound
// exactly as A3 requires, and A5a's "no durable messages" case also
// collapses to the same sentinel.
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

// ResolveBundle runs the five ordered steps described in the spec: resolve
// identity, authorize, validate, claim, and (winner only) apply. Steps 4 and
// 5 are ordered so the durable claim lands before any event is published —
// a loser can never trigger a resume.
func (r *Resolver) ResolveBundle(ctx context.Context, pendingID string, outcome Outcome) (*Resolution, bool, error) {
	msgs, sessionID, taskID, err := r.resolveIdentity(ctx, pendingID)
	if err != nil {
		return nil, false, err
	}

	if err := r.authorizer.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, false, ErrBundleNotFound // A3
	}

	questions := bundleQuestions(msgs)

	if !outcome.Cancel {
		if err := validateOutcome(questions, outcome); err != nil {
			return nil, false, err // N8c: validation runs before the claim
		}
	}

	return r.claimAndApply(ctx, pendingID, sessionID, taskID, msgs, questions, outcome)
}

// resolveIdentity is step 1 (M5, M5a): read the bundle's durable messages
// and derive session_id/task_id. A bundle with no messages, or one whose
// task_id cannot be resolved from either source, is not-found.
func (r *Resolver) resolveIdentity(ctx context.Context, pendingID string) ([]*taskmodels.Message, string, string, error) {
	msgs, err := r.repo.FindMessagesByPendingID(ctx, pendingID)
	if err != nil {
		return nil, "", "", err
	}
	if len(msgs) == 0 {
		return nil, "", "", ErrBundleNotFound
	}
	sessionID, taskID, ok := r.resolveBundleTaskID(ctx, msgs)
	if !ok {
		return nil, "", "", ErrBundleNotFound // M5a
	}
	return msgs, sessionID, taskID, nil
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

// claimAndApply is steps 4 and 5. On a win, applyWinningResolution's error
// (R5) is returned as-is; the caller SHALL NOT receive a success response.
func (r *Resolver) claimAndApply(
	ctx context.Context,
	pendingID, sessionID, taskID string,
	msgs []*taskmodels.Message,
	questions []Question,
	outcome Outcome,
) (*Resolution, bool, error) {
	now := r.now()
	status, response := buildOutcomeResponse(pendingID, questions, outcome, now)
	serialized, err := SerializeResponse(response)
	if err != nil {
		return nil, false, err
	}

	claimed, row, err := r.resolutions.InsertClarificationResolution(ctx, &taskmodels.ClarificationResolution{
		PendingID:  pendingID,
		SessionID:  sessionID,
		TaskID:     taskID,
		Status:     status,
		Response:   serialized,
		Resume:     taskmodels.ClarificationResolutionResumePending,
		ResolvedBy: outcome.ResolvedBy,
		Source:     outcome.Source,
		ResolvedAt: now,
	})
	if err != nil {
		if errors.Is(err, sqliterepo.ErrClarificationSessionMissing) || errors.Is(err, sqliterepo.ErrClarificationResolutionNotFound) {
			r.logger.Warn("clarification bundle unclaimable", zap.String("pending_id", pendingID), zap.Error(err))
			return nil, false, ErrBundleNotFound // M8a, M9
		}
		return nil, false, err
	}

	// X3/X3a: a cancel closes the in-memory CancelCh whether it wins or
	// loses the claim, and does nothing else when it loses.
	if outcome.Cancel {
		r.store.CancelRequest(pendingID)
	}

	if !claimed {
		return r.loserResolution(row), false, nil // R2
	}

	resume, applyErr := r.applyWinningResolution(ctx, pendingID, sessionID, taskID, msgs, status, questions, response)
	if applyErr != nil {
		return nil, true, applyErr // R5
	}

	if err := r.resolutions.UpdateClarificationResolutionResume(ctx, pendingID, resume); err != nil {
		// M7a: report what we computed, not the stale "pending" left on the
		// row. This is bookkeeping about a resume that already happened (or
		// already failed); it SHALL NOT turn a 200 into a 500.
		r.logger.Warn("failed to record resume outcome", zap.String("pending_id", pendingID), zap.String("resume", resume), zap.Error(err))
	}

	return &Resolution{
		PendingID:  pendingID,
		SessionID:  sessionID,
		TaskID:     taskID,
		Status:     status,
		Response:   response,
		Resume:     resume,
		ResolvedBy: outcome.ResolvedBy,
		Source:     outcome.Source,
		ResolvedAt: now,
	}, true, nil
}

func (r *Resolver) loserResolution(row *taskmodels.ClarificationResolution) *Resolution {
	response, err := DeserializeResponse(row.Response)
	if err != nil {
		r.logger.Warn("failed to deserialize stored clarification response", zap.String("pending_id", row.PendingID), zap.Error(err))
		response = &Response{PendingID: row.PendingID, Answers: []Answer{}}
	}
	return &Resolution{
		PendingID:  row.PendingID,
		SessionID:  row.SessionID,
		TaskID:     row.TaskID,
		Status:     row.Status,
		Response:   response,
		Resume:     row.Resume,
		ResolvedBy: row.ResolvedBy,
		Source:     row.Source,
		ResolvedAt: row.ResolvedAt,
	}
}

// applyWinningResolution is step 5: durable message updates (R4, R5, R5a,
// D2, R9a) then delivery/publish (R7-R9b, X4). It returns the resume value
// the applying request itself computed (M7a) — never re-read from the row.
func (r *Resolver) applyWinningResolution(
	ctx context.Context,
	pendingID, sessionID, taskID string,
	msgs []*taskmodels.Message,
	status string,
	questions []Question,
	response *Response,
) (string, error) {
	if err := r.applyMessageUpdates(ctx, pendingID, sessionID, msgs, status, response); err != nil {
		if resumeErr := r.resolutions.UpdateClarificationResolutionResume(ctx, pendingID, taskmodels.ClarificationResolutionResumeFailed); resumeErr != nil {
			r.logger.Warn("failed to record failed resume after partial application", zap.String("pending_id", pendingID), zap.Error(resumeErr))
		}
		return taskmodels.ClarificationResolutionResumeFailed, &partialApplicationError{
			msg: fmt.Sprintf("clarification bundle %s partially applied: %v", pendingID, err),
		}
	}

	if status == taskmodels.ClarificationResolutionStatusCancelled {
		r.publishCancelled(ctx, pendingID, sessionID, taskID, questions)
		return taskmodels.ClarificationResolutionResumeNotApplicable, nil // X4
	}

	if r.deliverToWaiter(pendingID, response) {
		r.publishPrimaryAnswered(ctx, pendingID, sessionID, taskID, questions, response) // best-effort, R8c
		return taskmodels.ClarificationResolutionResumePublished, nil                    // R7
	}

	// R7a: no live waiter, or delivery failed for any reason — fall through
	// to the no-waiter branch for this resolution's own outcome.
	if status == taskmodels.ClarificationResolutionStatusAnswered {
		return r.publishAnsweredOrFailed(ctx, pendingID, sessionID, taskID, questions, response), nil // R8
	}
	r.publishStaleDismissed(ctx, pendingID, sessionID, taskID)
	return taskmodels.ClarificationResolutionResumeNotApplicable, nil // R9
}

// applyMessageUpdates writes the winning resolution's per-question status to
// every clarification_request message, in D2 order, stopping at the first
// failure (R5a). An answered message carries its matching normalized
// answer; a rejected or cancelled message carries none, including one whose
// question_id is empty (R9a).
func (r *Resolver) applyMessageUpdates(ctx context.Context, pendingID, sessionID string, msgs []*taskmodels.Message, status string, response *Response) error {
	ordered := orderBundleMessages(msgs)
	answersByID := make(map[string]Answer, len(response.Answers))
	for _, a := range response.Answers {
		answersByID[a.QuestionID] = a
	}

	writeCtx := context.WithoutCancel(ctx)
	for _, msg := range ordered {
		qid := stringFromMetadata(msg.Metadata, metaQuestionIDKey)
		var answer *Answer
		if status == taskmodels.ClarificationResolutionStatusAnswered {
			a, ok := answersByID[qid]
			if !ok {
				return fmt.Errorf("no normalized answer for question_id %q", qid)
			}
			answer = &a
		}
		if err := r.messages.UpdateClarificationMessage(writeCtx, sessionID, pendingID, qid, status, answer); err != nil {
			r.logger.Warn("failed to apply winning clarification resolution to message",
				zap.String("pending_id", pendingID), zap.String("question_id", qid), zap.String("message_id", msg.ID), zap.Error(err))
			return err
		}
	}
	return nil
}

// deliverToWaiter is R7: deliver the resolution through a live in-memory
// waiter, if one exists, so the agent's blocked tool call resolves in the
// same turn. A defensive copy is passed to Store.Respond, which mutates
// RespondedAt to its own call time — the caller's response (already
// serialized into the resolutions row) must not be mutated out from under
// it.
func (r *Resolver) deliverToWaiter(pendingID string, response *Response) bool {
	clone := *response
	clone.Answers = append([]Answer{}, response.Answers...)
	return r.store.Respond(pendingID, &clone) == nil
}

func (r *Resolver) publishAnsweredOrFailed(ctx context.Context, pendingID, sessionID, taskID string, questions []Question, response *Response) string {
	if r.eventBus == nil {
		return taskmodels.ClarificationResolutionResumeFailed
	}
	eventData := map[string]any{
		"session_id":    sessionID,
		"task_id":       taskID,
		"pending_id":    pendingID,
		metaQuestionKey: formatQuestionSummary(questions),
		"answer_text":   buildAnswerSummary(questions, response.Answers, false, ""),
		"rejected":      false,
		"reject_reason": "",
	}
	if err := r.eventBus.Publish(ctx, events.ClarificationAnswered, bus.NewEvent(events.ClarificationAnswered, "clarification-resolver", eventData)); err != nil {
		r.logger.Warn("failed to publish clarification answered event (no live waiter)", zap.String("pending_id", pendingID), zap.Error(err))
		return taskmodels.ClarificationResolutionResumeFailed // R8a rule 2
	}
	return taskmodels.ClarificationResolutionResumePublished
}

func (r *Resolver) publishStaleDismissed(ctx context.Context, pendingID, sessionID, taskID string) {
	if r.eventBus == nil {
		return
	}
	eventData := map[string]any{"session_id": sessionID, "task_id": taskID, "pending_id": pendingID}
	if err := r.eventBus.Publish(ctx, events.ClarificationStaleDismissed, bus.NewEvent(events.ClarificationStaleDismissed, "clarification-resolver", eventData)); err != nil {
		r.logger.Warn("failed to publish stale-dismissed clarification event", zap.String("pending_id", pendingID), zap.Error(err))
	}
}

// publishPrimaryAnswered is best-effort (R8c): a failed publish here does
// not change the resume value, since the delivery itself — not this event —
// is what resumes the agent.
func (r *Resolver) publishPrimaryAnswered(ctx context.Context, pendingID, sessionID, taskID string, questions []Question, response *Response) {
	if r.eventBus == nil {
		return
	}
	eventData := map[string]any{
		"session_id":    sessionID,
		"task_id":       taskID,
		"pending_id":    pendingID,
		metaQuestionKey: formatQuestionSummary(questions),
		"answer_text":   buildAnswerSummary(questions, response.Answers, response.Rejected, response.RejectReason),
		"rejected":      response.Rejected,
		"reject_reason": response.RejectReason,
	}
	if err := r.eventBus.Publish(ctx, events.ClarificationPrimaryAnswered, bus.NewEvent(events.ClarificationPrimaryAnswered, "clarification-resolver", eventData)); err != nil {
		r.logger.Warn("failed to publish primary clarification event", zap.String("pending_id", pendingID), zap.Error(err))
	}
}

// publishCancelled is X4: a failed publish here does not change the resume
// value either — clarification.cancelled is not a resume event.
func (r *Resolver) publishCancelled(ctx context.Context, pendingID, sessionID, taskID string, questions []Question) {
	if r.eventBus == nil {
		return
	}
	prompt := ""
	if len(questions) > 0 {
		prompt = questions[0].Prompt
	}
	eventData := map[string]any{
		"session_id":    sessionID,
		"task_id":       taskID,
		"pending_id":    pendingID,
		metaQuestionKey: prompt,
		"answer_text":   "The pending clarification question was cancelled.",
		"rejected":      true,
		"reject_reason": "cancelled",
	}
	if err := r.eventBus.Publish(ctx, events.ClarificationCancelled, bus.NewEvent(events.ClarificationCancelled, "clarification-resolver", eventData)); err != nil {
		r.logger.Warn("failed to publish clarification cancelled event", zap.String("pending_id", pendingID), zap.Error(err))
	}
}
