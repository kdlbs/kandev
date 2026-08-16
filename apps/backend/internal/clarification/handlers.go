// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wsmsg "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Metadata key constants used when constructing event payloads and reading
// per-message clarification metadata. Pulled out so goconst stays happy and
// renames stay safe.
const (
	metaQuestionKey   = "question"
	metaQuestionIDKey = "question_id"
	metaStatusKey     = "status"

	clarificationPersistenceTimeout = 30 * time.Second
)

// handlerMessageStore is the task repository surface used by HTTP handlers.
type handlerMessageStore interface {
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*taskmodels.Message, error)
}

// cancellationMessageStore is the task repository surface used by session cancellation.
type cancellationMessageStore interface {
	FindActiveClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*taskmodels.Message, error)
	DetachActiveClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*taskmodels.Message, error)
	ExpireActiveClarificationBundle(ctx context.Context, sessionID, pendingID string) ([]*taskmodels.Message, error)
}

type clarificationStore interface {
	CreateRequest(req *Request) (string, bool)
	GetRequest(pendingID string) (*Request, bool)
	WaitForResponse(ctx context.Context, pendingID string) (*Response, error)
	Respond(pendingID string, response *Response) error
	CancelRequest(pendingID string) bool
}

// Broadcaster interface for sending WebSocket notifications
type Broadcaster interface {
	BroadcastToSession(sessionID string, msg *wsmsg.Message)
}

// MessageCreator interface for creating messages in the database
type MessageCreator interface {
	// CreateClarificationRequestMessages creates one chat message per question in
	// a multi-question clarification request, all sharing the given pending_id.
	// Only the last message returned should set RequestsInput=true so the chat
	// scrolls to the bottom of the group. Returns the created message IDs in the
	// same order as the input questions.
	CreateClarificationRequestMessages(ctx context.Context, taskID, sessionID, pendingID string, questions []Question, clarificationContext string) ([]string, error)
	// UpdateClarificationMessage updates the per-question clarification message's
	// status (and stores the matching answer if any) for a (pending_id, question_id)
	// pair within the session.
	UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, questionID, status string, answer *Answer) error
	// CompleteActiveClarificationBundle atomically transitions a bundle only
	// when it still belongs to the session's current durable turn.
	CompleteActiveClarificationBundle(ctx context.Context, pendingID, status string, responses map[string]interface{}) ([]*taskmodels.Message, bool, error)
	// RestoreActiveClarificationBundle reopens a claimed bundle when detached
	// resume acceptance fails and returns the committed pending rows for publication.
	RestoreActiveClarificationBundle(
		ctx context.Context,
		pendingID, terminalStatus string,
		claimedMessages []*taskmodels.Message,
	) ([]*taskmodels.Message, bool, error)
	// PublishClarificationBundleUpdates exposes committed terminal or restored-pending rows.
	// Restored rows synchronously converge the durable task summary before publication.
	PublishClarificationBundleUpdates(ctx context.Context, messages []*taskmodels.Message) error
}

// EventBus interface for publishing events.
type EventBus interface {
	Publish(ctx context.Context, topic string, event *bus.Event) error
}

// DetachedClarificationResume contains the durable context required to resume
// a session after its original clarification waiter has gone away.
type DetachedClarificationResume struct {
	TaskID              string
	SessionID           string
	PendingID           string
	ClarificationTurnID string
	ClaimedMessageIDs   []string
	Question            string
	AnswerText          string
	Rejected            bool
	RejectReason        string
}

// DetachedClarificationResumer acknowledges whether the orchestrator accepted
// a detached answer before the handler exposes the bundle as terminal.
type DetachedClarificationResumer interface {
	ResumeDetachedClarification(ctx context.Context, request DetachedClarificationResume) error
}

// Handlers provides HTTP handlers for clarification requests.
type Handlers struct {
	store           clarificationStore
	hub             Broadcaster
	messageCreator  MessageCreator
	repo            handlerMessageStore
	eventBus        EventBus
	detachedResumer DetachedClarificationResumer
	logger          *logger.Logger
}

// NewHandlers creates new clarification handlers.
func NewHandlers(
	store *Store,
	hub Broadcaster,
	messageCreator MessageCreator,
	repo handlerMessageStore,
	eventBus EventBus,
	detachedResumer DetachedClarificationResumer,
	log *logger.Logger,
) *Handlers {
	return &Handlers{
		store:           store,
		hub:             hub,
		messageCreator:  messageCreator,
		repo:            repo,
		eventBus:        eventBus,
		detachedResumer: detachedResumer,
		logger:          log.WithFields(zap.String("component", "clarification-handlers")),
	}
}

// RegisterRoutes registers clarification HTTP routes.
func RegisterRoutes(
	router *gin.Engine,
	store *Store,
	hub Broadcaster,
	messageCreator MessageCreator,
	repo handlerMessageStore,
	eventBus EventBus,
	detachedResumer DetachedClarificationResumer,
	log *logger.Logger,
) {
	h := NewHandlers(store, hub, messageCreator, repo, eventBus, detachedResumer, log)
	api := router.Group("/api/v1/clarification")
	api.POST("/request", h.httpCreateRequest)
	api.GET("/:id", h.httpGetRequest)
	api.GET("/:id/wait", h.httpWaitForResponse)
	api.POST("/:id/respond", h.httpRespond)
	api.POST("/:id/cancel", h.httpCancelRequest)
}

// CreateRequestBody is the request body for creating a clarification request.
// A single request may bundle 1..N questions; the bundle is gated on the user
// answering every question (or rejecting the bundle as a whole).
type CreateRequestBody struct {
	SessionID string     `json:"session_id" binding:"required"`
	TaskID    string     `json:"task_id"`
	Questions []Question `json:"questions" binding:"required,min=1,dive"`
	Context   string     `json:"context"`
}

// CreateRequestResponse is the response for creating a clarification request.
type CreateRequestResponse struct {
	PendingID string `json:"pending_id"`
}

func (h *Handlers) httpCreateRequest(c *gin.Context) {
	var body CreateRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	if errMsg := NormalizeAndValidateQuestions(body.Questions); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	// Look up the task ID for this session
	sessionID := body.SessionID
	taskID := body.TaskID
	if taskID == "" {
		session, err := h.repo.GetTaskSession(c.Request.Context(), sessionID)
		if err != nil {
			h.logger.Warn("failed to look up session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			taskID = session.TaskID
		}
	}

	req := &Request{
		SessionID: sessionID,
		TaskID:    taskID,
		Questions: body.Questions,
		Context:   body.Context,
	}

	pendingID, isNew := h.store.CreateRequest(req)

	// Create one message per question in the database; all share the same
	// pending_id and are rendered as a stacked group on the frontend. The
	// session.message.added WebSocket event fires per message. On failure we
	// also cancel the in-store pending entry so any blocking WaitForResponse
	// caller unblocks immediately rather than waiting for the MCP timeout.
	// When dedup fires (isNew=false) the messages already exist, so skip creation.
	if isNew && h.messageCreator != nil {
		_, err := h.messageCreator.CreateClarificationRequestMessages(
			c.Request.Context(),
			taskID,
			sessionID,
			pendingID,
			body.Questions,
			body.Context,
		)
		if err != nil {
			h.logger.Error("failed to create clarification request messages",
				zap.String("pending_id", pendingID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			h.store.CancelRequest(pendingID)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to create clarification messages: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, CreateRequestResponse{PendingID: pendingID})
}

// NormalizeAndValidateQuestions is the single source of truth for clarification
// bundle validation. It mutates `questions` to assign missing IDs (q1, q2, ...)
// and option IDs, and enforces:
//   - 1..4 questions per bundle
//   - unique question IDs (rejects duplicates)
//   - non-empty prompt
//   - 2..6 options per question
//
// Both the HTTP handler (httpCreateRequest) and the WebSocket-side MCP handler
// (handleAskUserQuestion) call this so validation never drifts between paths.
// Returns "" on success or an error message describing the first failure.
func NormalizeAndValidateQuestions(questions []Question) string {
	if len(questions) == 0 {
		return "questions must contain at least 1 question"
	}
	if len(questions) > 4 {
		return "questions must contain at most 4 questions"
	}
	seen := map[string]bool{}
	for i := range questions {
		if questions[i].ID == "" {
			questions[i].ID = fmt.Sprintf("q%d", i+1)
		}
		if seen[questions[i].ID] {
			return fmt.Sprintf("duplicate question id %q", questions[i].ID)
		}
		seen[questions[i].ID] = true
		if questions[i].Prompt == "" {
			return fmt.Sprintf("question %d is missing required 'prompt'", i+1)
		}
		if len(questions[i].Options) < 2 {
			return fmt.Sprintf("question %d must have at least 2 options", i+1)
		}
		if len(questions[i].Options) > 6 {
			return fmt.Sprintf("question %d must have at most 6 options", i+1)
		}
		for j := range questions[i].Options {
			if questions[i].Options[j].ID == "" {
				questions[i].Options[j].ID = generateOptionID(i, j)
			}
		}
	}
	return ""
}

func (h *Handlers) httpGetRequest(c *gin.Context) {
	pendingID := c.Param("id")

	req, ok := h.store.GetRequest(pendingID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
		return
	}

	c.JSON(http.StatusOK, req)
}

func (h *Handlers) httpWaitForResponse(c *gin.Context) {
	pendingID := c.Param("id")
	resp, err := h.store.WaitForResponse(c.Request.Context(), pendingID)
	if err != nil {
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RespondBody is the request body for responding to a clarification request.
// The frontend posts every answer at once when the user finishes the bundle
// (decision A: per-question commit collected in the hook, batched on the wire).
// Answers must contain exactly one entry per question in the original request,
// or be empty when Rejected=true.
type RespondBody struct {
	Answers      []Answer `json:"answers"`
	Rejected     bool     `json:"rejected"`
	RejectReason string   `json:"reject_reason"`
}

func (h *Handlers) httpRespond(c *gin.Context) {
	pendingID := c.Param("id")
	var body RespondBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	writeCtx := context.WithoutCancel(c.Request.Context())
	claim, statusCode, errorMessage := h.claimClarificationResponse(writeCtx, pendingID, body)
	if statusCode != 0 {
		c.JSON(statusCode, gin.H{"error": errorMessage})
		return
	}
	statusCode, errorMessage = h.deliverClaimedClarificationResponse(writeCtx, pendingID, body, claim)
	if statusCode != 0 {
		c.JSON(statusCode, gin.H{"error": errorMessage})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type clarificationResponseClaim struct {
	response       *Response
	terminalStatus string
	messages       []*taskmodels.Message
}

func (h *Handlers) claimClarificationResponse(
	ctx context.Context,
	pendingID string,
	body RespondBody,
) (*clarificationResponseClaim, int, string) {
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	if !body.Rejected {
		if errMsg := h.validateRespondAnswers(persistenceCtx, pendingID, body.Answers); errMsg != "" {
			return nil, http.StatusBadRequest, errMsg
		}
	}
	terminalStatus := string(StatusAnswered)
	var responses map[string]interface{}
	if body.Rejected {
		terminalStatus = string(StatusRejected)
	} else {
		responses = make(map[string]interface{}, len(body.Answers))
		for i := range body.Answers {
			answer := body.Answers[i]
			responses[answer.QuestionID] = answer
		}
	}
	if h.messageCreator == nil {
		return nil, http.StatusInternalServerError, "clarification message service unavailable"
	}
	completedMessages, claimed, claimErr := h.messageCreator.CompleteActiveClarificationBundle(
		persistenceCtx,
		pendingID,
		terminalStatus,
		responses,
	)
	if claimErr != nil {
		h.logger.Error("failed to claim clarification response",
			zap.String("pending_id", pendingID),
			zap.Error(claimErr))
		return nil, http.StatusInternalServerError, "failed to update clarification state"
	}
	if !claimed {
		return nil, http.StatusConflict, "clarification request is no longer active"
	}
	return &clarificationResponseClaim{
		response: &Response{
			PendingID:    pendingID,
			Answers:      body.Answers,
			Rejected:     body.Rejected,
			RejectReason: body.RejectReason,
		},
		terminalStatus: terminalStatus,
		messages:       completedMessages,
	}, 0, ""
}

func (h *Handlers) deliverClaimedClarificationResponse(
	ctx context.Context,
	pendingID string,
	body RespondBody,
	claim *clarificationResponseClaim,
) (int, string) {
	// Durable current-turn ownership is claimed before touching the live waiter.
	// This closes the window where a newer turn exists but its canceller has not
	// yet drained the superseded in-memory request.
	deliveryErr := h.store.Respond(pendingID, claim.response)
	if deliveryErr == nil {
		h.publishClarificationBundleUpdates(ctx, pendingID, claim.messages)
		h.publishPrimaryAnsweredEvent(ctx, pendingID, body.Answers, body.Rejected, body.RejectReason)
		h.logger.Info("clarification answered via primary path (same turn)",
			zap.String("pending_id", pendingID),
			zap.Int("answers", len(body.Answers)),
			zap.Bool("rejected", body.Rejected))
		return 0, ""
	}
	if errors.Is(deliveryErr, ErrAlreadyResponded) {
		// Defensive only: the durable claim above is exclusive. Retain this as a
		// safety net if the in-memory waiter ever diverges from durable state.
		h.publishClarificationBundleUpdates(ctx, pendingID, claim.messages)
		h.logger.Warn("duplicate response attempt", zap.String("pending_id", pendingID))
		return http.StatusConflict, "response already submitted"
	}
	if !errors.Is(deliveryErr, ErrNotFound) {
		restored := h.restoreFailedClarificationClaim(ctx, pendingID, claim.terminalStatus, claim.messages)
		h.logger.Error("failed to deliver clarification response",
			zap.String("pending_id", pendingID),
			zap.Bool("restored", restored),
			zap.Error(deliveryErr))
		if restored {
			return http.StatusInternalServerError, "failed to deliver clarification response; response can be retried"
		}
		return http.StatusInternalServerError, "failed to deliver clarification response and recover pending clarification state"
	}
	return h.deliverDetachedClarificationResponse(ctx, pendingID, body, claim, deliveryErr)
}

func (h *Handlers) deliverDetachedClarificationResponse(
	ctx context.Context,
	pendingID string,
	body RespondBody,
	claim *clarificationResponseClaim,
	deliveryErr error,
) (int, string) {
	// Fallback path: entry not found (agent timed out, entry was cleaned up).
	// If the user rejected (clicked X to dismiss), they're discarding a stale
	// overlay — not continuing the conversation. Treat as a no-op so we don't
	// surprise them by resuming the agent with "User declined to answer".
	// The overlay is already detached (agent_disconnected, still pending), so
	// dismissing it must not resume the agent with "User declined to answer".
	// We still need to mark the bundle rejected in the DB; otherwise the durable
	// pending-clarification guard would keep blocking future workflow transitions.
	if body.Rejected {
		h.publishStaleDismissedEvent(ctx, pendingID)
		h.publishClarificationBundleUpdates(ctx, pendingID, claim.messages)
		h.logger.Info("clarification rejected after agent moved on; no-op",
			zap.String("pending_id", pendingID))
		return 0, ""
	}

	// User is providing an affirmative answer after the agent moved on. Ask the
	// orchestrator to accept a new turn containing the answer before publishing
	// the terminal clarification update.
	h.logger.Info("clarification entry not found, using acknowledged resume fallback",
		zap.String("pending_id", pendingID),
		zap.String("error", deliveryErr.Error()))

	if err := h.resumeDetachedClarification(
		ctx, pendingID, body.Answers, body.Rejected, body.RejectReason, claim.messages,
	); err != nil {
		if detachedResumeWasAccepted(err) {
			// The prompt reached agentctl. Keep the durable answer terminal so an
			// HTTP retry cannot dispatch it again, even though turn publication
			// failed and the caller must receive a server error.
			h.publishClarificationBundleUpdates(ctx, pendingID, claim.messages)
			h.logger.Error("accepted clarification resume was not durably published",
				zap.String("pending_id", pendingID),
				zap.Error(err))
			return http.StatusInternalServerError,
				"clarification response was accepted, but dispatch state could not be finalized"
		}
		restored := h.restoreFailedClarificationClaim(ctx, pendingID, claim.terminalStatus, claim.messages)
		h.logger.Error("failed to resume detached clarification",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		if restored {
			return http.StatusInternalServerError, "failed to resume clarification; response can be retried"
		}
		return http.StatusInternalServerError, "failed to resume clarification and recover pending clarification state"
	}
	h.publishClarificationBundleUpdates(ctx, pendingID, claim.messages)
	return 0, ""
}

func (h *Handlers) publishClarificationBundleUpdates(
	ctx context.Context,
	pendingID string,
	messages []*taskmodels.Message,
) {
	publishCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	if err := h.messageCreator.PublishClarificationBundleUpdates(publishCtx, messages); err != nil {
		h.logger.Error("failed to publish clarification bundle updates",
			zap.String("pending_id", pendingID),
			zap.Error(err))
	}
}

func detachedResumeWasAccepted(err error) bool {
	var accepted interface{ DetachedResumeAccepted() bool }
	return errors.As(err, &accepted) && accepted.DetachedResumeAccepted()
}

func (h *Handlers) restoreFailedClarificationClaim(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*taskmodels.Message,
) bool {
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	restoredMessages, restored, err := h.messageCreator.RestoreActiveClarificationBundle(
		persistenceCtx,
		pendingID,
		terminalStatus,
		claimedMessages,
	)
	if err != nil {
		h.logger.Error("failed to restore clarification after delivery failure",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return false
	}
	if !restored {
		h.logger.Warn("clarification claim was not restorable after delivery failure",
			zap.String("pending_id", pendingID))
		return false
	}
	if err := h.messageCreator.PublishClarificationBundleUpdates(persistenceCtx, restoredMessages); err != nil {
		h.logger.Error("failed to converge restored clarification state",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		// The durable bundle is pending again, so retry remains safe even when
		// its best-effort live projection could not be acknowledged.
		return true
	}
	return true
}

func clarificationPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), clarificationPersistenceTimeout)
}

// validateRespondAnswers enforces the all-required gate **and** the question-id
// invariant: every answer must target a real question in the bundle, every
// question must have an answer, and no question id may be answered twice.
// Returns "" on success or an error message describing the first failure.
//
// The expected question ids come from the in-store request; if the in-memory
// entry has already been cleaned up (agent timed out before user responded),
// the persisted messages serve as fallback so the late-respond path is
// validated the same way as the primary path.
func (h *Handlers) validateRespondAnswers(ctx context.Context, pendingID string, answers []Answer) string {
	expected := h.expectedQuestionIDs(ctx, pendingID)
	if len(expected) == 0 {
		// Couldn't determine the expected set — fall back to permissive (the
		// primary-path Respond will still error sensibly if the bundle is gone).
		return ""
	}
	expectedSet := make(map[string]bool, len(expected))
	for _, id := range expected {
		expectedSet[id] = true
	}

	if len(answers) != len(expected) {
		return fmt.Sprintf("expected %d answers, got %d", len(expected), len(answers))
	}

	seen := make(map[string]bool, len(answers))
	for i, a := range answers {
		if a.QuestionID == "" {
			return fmt.Sprintf("answer %d is missing question_id", i+1)
		}
		if !expectedSet[a.QuestionID] {
			return fmt.Sprintf("answer %d references unknown question id %q", i+1, a.QuestionID)
		}
		if seen[a.QuestionID] {
			return fmt.Sprintf("answer %d duplicates question id %q", i+1, a.QuestionID)
		}
		seen[a.QuestionID] = true
	}
	return ""
}

// expectedQuestionIDs returns the ordered question ids the user is expected to
// answer for the given pending bundle. Persisted recovery includes only pending
// siblings because an earlier partial write may have made other rows terminal.
func (h *Handlers) expectedQuestionIDs(ctx context.Context, pendingID string) []string {
	if req, ok := h.store.GetRequest(pendingID); ok && req != nil {
		ids := make([]string, 0, len(req.Questions))
		for _, q := range req.Questions {
			ids = append(ids, q.ID)
		}
		return ids
	}
	if h.repo == nil {
		return nil
	}
	msgs, err := h.repo.FindMessagesByPendingID(ctx, pendingID)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		status := stringFromMetadata(m.Metadata, metaStatusKey)
		if status != "" && status != string(StatusPending) {
			continue
		}
		if id := stringFromMetadata(m.Metadata, metaQuestionIDKey); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (h *Handlers) httpCancelRequest(c *gin.Context) {
	pendingID := c.Param("id")
	req, ok := h.store.GetRequest(pendingID)
	if !ok || req == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
		return
	}
	if !h.store.CancelRequest(pendingID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
		return
	}

	if h.messageCreator != nil {
		for _, q := range req.Questions {
			if err := h.messageCreator.UpdateClarificationMessage(
				c.Request.Context(),
				req.SessionID,
				pendingID,
				q.ID,
				string(StatusCancelled),
				nil,
			); err != nil {
				h.logger.Warn("failed to mark clarification cancelled",
					zap.String("pending_id", pendingID),
					zap.String("question_id", q.ID),
					zap.Error(err))
			}
		}
	}
	h.publishCancelledEvent(c, pendingID, req)
	h.logger.Info("clarification cancelled by operator",
		zap.String("pending_id", pendingID),
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func stringFromMetadata(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key].(string); ok {
		return v
	}
	return ""
}

// resumeDetachedClarification synchronously asks the orchestrator to accept a new
// turn. Used when the original clarification waiter has gone away.
func (h *Handlers) resumeDetachedClarification(
	ctx context.Context,
	pendingID string,
	answers []Answer,
	rejected bool,
	rejectReason string,
	claimedMessages []*taskmodels.Message,
) error {
	if h.detachedResumer == nil {
		return errors.New("detached clarification resumer unavailable")
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()

	clarificationCtx, err := h.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil {
		return fmt.Errorf("resolve clarification fallback context: %w", err)
	}
	if clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		return fmt.Errorf(
			"missing session/task for clarification fallback event: session=%q task=%q",
			clarificationCtx.SessionID,
			clarificationCtx.TaskID,
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
	if err := h.detachedResumer.ResumeDetachedClarification(persistenceCtx, request); err != nil {
		return fmt.Errorf("resume detached clarification: %w", err)
	}

	h.logger.Info("clarification answered via acknowledged fallback (new turn)",
		zap.String("pending_id", pendingID),
		zap.String("session_id", clarificationCtx.SessionID),
		zap.String("task_id", clarificationCtx.TaskID))
	return nil
}

func clarificationClaimRecovery(messages []*taskmodels.Message) (string, []string, error) {
	messageIDs := make([]string, 0, len(messages))
	turnID := ""
	turnMessageID := ""
	for _, message := range messages {
		if message == nil || message.ID == "" {
			continue
		}
		if message.TurnID != "" && turnID == "" {
			turnID = message.TurnID
			turnMessageID = message.ID
		} else if message.TurnID != "" && message.TurnID != turnID {
			return "", nil, fmt.Errorf(
				"clarification bundle mixes turn %q from message %s with turn %q from message %s",
				turnID,
				turnMessageID,
				message.TurnID,
				message.ID,
			)
		}
		messageIDs = append(messageIDs, message.ID)
	}
	return turnID, messageIDs, nil
}

func (h *Handlers) publishCancelledEvent(c *gin.Context, pendingID string, req *Request) {
	if h.eventBus == nil || req == nil {
		return
	}
	prompt := ""
	if len(req.Questions) > 0 {
		prompt = req.Questions[0].Prompt
	}
	eventData := map[string]any{
		"session_id":    req.SessionID,
		"task_id":       req.TaskID,
		"pending_id":    pendingID,
		"question":      prompt,
		"answer_text":   "The pending clarification question was cancelled by the operator.",
		"rejected":      true,
		"reject_reason": "cancelled",
	}
	if err := h.eventBus.Publish(c.Request.Context(), events.ClarificationCancelled, bus.NewEvent(
		events.ClarificationCancelled,
		"clarification-handlers",
		eventData,
	)); err != nil {
		h.logger.Error("failed to publish clarification cancelled event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", req.SessionID),
			zap.Error(err))
	}
}

func (h *Handlers) publishStaleDismissedEvent(ctx context.Context, pendingID string) {
	if h.eventBus == nil {
		return
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	clarificationCtx, err := h.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil || clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		h.logger.Warn("failed to resolve context for stale-dismissed clarification event",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return
	}
	eventData := map[string]any{
		"session_id": clarificationCtx.SessionID,
		"task_id":    clarificationCtx.TaskID,
		"pending_id": pendingID,
	}
	if err := h.eventBus.Publish(persistenceCtx, events.ClarificationStaleDismissed, bus.NewEvent(
		events.ClarificationStaleDismissed,
		"clarification-handlers",
		eventData,
	)); err != nil {
		h.logger.Warn("failed to publish stale-dismissed clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.Error(err))
	}
}

func (h *Handlers) publishPrimaryAnsweredEvent(ctx context.Context, pendingID string, answers []Answer, rejected bool, rejectReason string) {
	if h.eventBus == nil {
		return
	}
	persistenceCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	clarificationCtx, err := h.resolveClarificationEventContext(persistenceCtx, pendingID)
	if err != nil {
		h.logger.Warn("failed to resolve context for primary clarification event",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return
	}
	if clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		h.logger.Warn("missing session/task for primary clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.String("task_id", clarificationCtx.TaskID))
		return
	}

	answerText := buildAnswerSummary(clarificationCtx.Questions, answers, rejected, rejectReason)
	eventData := map[string]any{
		"session_id":    clarificationCtx.SessionID,
		"task_id":       clarificationCtx.TaskID,
		"pending_id":    pendingID,
		metaQuestionKey: clarificationCtx.QuestionSummary,
		"answer_text":   answerText,
		"rejected":      rejected,
		"reject_reason": rejectReason,
	}
	if err := h.eventBus.Publish(persistenceCtx, events.ClarificationPrimaryAnswered, bus.NewEvent(
		events.ClarificationPrimaryAnswered,
		"clarification-handlers",
		eventData,
	)); err != nil {
		h.logger.Warn("failed to publish primary clarification event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.Error(err))
	}
}

type clarificationEventContext struct {
	SessionID       string
	TaskID          string
	Questions       []Question // Source-of-truth questions used to label answers; falls back to a single synthetic Question when only metadata is available.
	QuestionSummary string     // Pre-formatted multi-line "Q1: ...\nQ2: ..." used by the orchestrator resume prompt.
}

func (h *Handlers) resolveClarificationEventContext(ctx context.Context, pendingID string) (clarificationEventContext, error) {
	var out clarificationEventContext

	if req, ok := h.store.GetRequest(pendingID); ok && req != nil {
		out.SessionID = req.SessionID
		out.TaskID = req.TaskID
		out.Questions = req.Questions
		out.QuestionSummary = formatQuestionSummary(req.Questions)
		if out.SessionID != "" && out.TaskID != "" && len(out.Questions) > 0 {
			return out, nil
		}
	}

	if h.repo == nil {
		return out, fmt.Errorf("message repository unavailable")
	}

	msgs, err := h.repo.FindMessagesByPendingID(ctx, pendingID)
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

// questionsFromMessages reconstructs a Question slice from persisted clarification
// messages, ordered by metadata.question_index so the rebuilt summary matches
// the bundle the agent originally sent. Used as a fallback when the in-store
// request has been cleaned up but the persisted metadata still carries the
// original question text.
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

func generateOptionID(questionIndex, optionIndex int) string {
	return fmt.Sprintf("q%d_opt%d", questionIndex+1, optionIndex+1)
}
