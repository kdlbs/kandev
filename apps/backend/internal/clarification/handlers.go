// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	metaSessionIDKey  = "session_id"
	metaTaskIDKey     = "task_id"
	metaPendingIDKey  = "pending_id"
	metaRejectedKey   = "rejected"

	clarificationPersistenceTimeout = 30 * time.Second

	errClarificationRequestNotFound = "clarification request not found"
	errClarificationInternal        = "failed to authorize clarification request"
)

// handlerMessageStore is the task repository surface used by HTTP handlers
// and the Resolver.
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
	// pair within the session. Used only by cancel (A9), which never goes through
	// the claim.
	UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, questionID, status string, answer *Answer) error
	// CompleteActiveClarificationBundle atomically transitions a bundle only
	// when it still belongs to the session's current durable turn (P1's single
	// claim mechanism).
	CompleteActiveClarificationBundle(ctx context.Context, pendingID, status string, responses map[string]interface{}) ([]*taskmodels.Message, bool, error)
	// FinalizeClarificationResponseDelivery clears the durable recovery intent
	// after the response reaches a live waiter or detached-resume boundary.
	FinalizeClarificationResponseDelivery(
		ctx context.Context,
		pendingID, terminalStatus string,
		claimedMessages []*taskmodels.Message,
	) ([]*taskmodels.Message, bool, error)
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

// Handlers provides HTTP handlers for clarification requests. It stays thin:
// identity/authorization, validation, claiming, and delivery all live on
// Resolver, which is shared with the answer_question_kandev /
// list_pending_questions_kandev MCP tools (R3).
type Handlers struct {
	store          *Store
	hub            Broadcaster
	messageCreator MessageCreator
	repo           handlerMessageStore
	eventBus       EventBus
	resolver       *Resolver
	logger         *logger.Logger
}

// NewHandlers creates new clarification handlers.
func NewHandlers(
	store *Store,
	hub Broadcaster,
	messageCreator MessageCreator,
	repo handlerMessageStore,
	eventBus EventBus,
	resolver *Resolver,
	log *logger.Logger,
) *Handlers {
	return &Handlers{
		store:          store,
		hub:            hub,
		messageCreator: messageCreator,
		repo:           repo,
		eventBus:       eventBus,
		resolver:       resolver,
		logger:         log.WithFields(zap.String("component", "clarification-handlers")),
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
	resolver *Resolver,
	log *logger.Logger,
) {
	h := NewHandlers(store, hub, messageCreator, repo, eventBus, resolver, log)
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

// authorizeBundleAccessOrRespond authorizes pendingID via AuthorizeBundleAccess
// and writes the response on failure, returning whether the caller may
// proceed. ErrBundleNotFound (a foreign or nonexistent pending_id) maps to
// 404, same as before. Any other error -- e.g. a database failure inside
// resolveIdentity -- previously collapsed to that same silent 404, which hid
// real failures from operators; it is now logged and reported as 500 instead.
func (h *Handlers) authorizeBundleAccessOrRespond(c *gin.Context, pendingID string) bool {
	if _, _, err := h.resolver.AuthorizeBundleAccess(c.Request.Context(), pendingID); err != nil {
		if errors.Is(err, ErrBundleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": errClarificationRequestNotFound})
			return false
		}
		h.logger.Error("failed to authorize clarification bundle access",
			zap.String("pending_id", pendingID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": errClarificationInternal})
		return false
	}
	return true
}

func (h *Handlers) httpGetRequest(c *gin.Context) {
	pendingID := c.Param("id")

	// A2/A3/A7/A8: authorize against the bundle's durable task_id before the
	// in-memory read, so a foreign or nonexistent pending_id is the same 404.
	if !h.authorizeBundleAccessOrRespond(c, pendingID) {
		return
	}

	req, ok := h.store.GetRequest(pendingID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": errClarificationRequestNotFound})
		return
	}

	c.JSON(http.StatusOK, req)
}

func (h *Handlers) httpWaitForResponse(c *gin.Context) {
	pendingID := c.Param("id")

	// A2/A3/A5a/A7/A8: same authorization as httpGetRequest, run first. A
	// pending_id with no durable messages is now 404 rather than the 504 a
	// missing in-memory entry produces below.
	if !h.authorizeBundleAccessOrRespond(c, pendingID) {
		return
	}

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

	res, claimed, err := h.resolver.ResolveBundle(c.Request.Context(), pendingID, Outcome(body))
	h.writeResolutionResult(c, pendingID, res, claimed, err)
}

// writeResolutionResult maps a ResolveBundle outcome to R10/R11's REST
// envelope. ErrBundleNotFound (A3, A5) maps to 404, a validation error
// (N6-N8b) maps to 400, IsNotActiveError (R2's no-winner branch) maps to
// 409, and every other error is an unexpected 500. A win and a loss both
// report the same 200 envelope, distinguished only by "claimed" (R2, R10,
// R11) — there is no "resume" key in this envelope.
func (h *Handlers) writeResolutionResult(c *gin.Context, pendingID string, res *Resolution, claimed bool, err error) {
	switch {
	case err == nil:
		serialized, serErr := SerializeResponse(res.Response)
		if serErr != nil {
			h.logger.Error("failed to serialize clarification response",
				zap.String("pending_id", pendingID), zap.Error(serErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": serErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"claimed":  claimed,
			"status":   res.Status,
			"response": json.RawMessage(serialized),
		})
	case errors.Is(err, ErrBundleNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": errClarificationRequestNotFound})
	case IsValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case IsNotActiveError(err):
		c.JSON(http.StatusConflict, gin.H{"error": "clarification request is no longer active"})
	default:
		h.logger.Error("failed to resolve clarification bundle",
			zap.String("pending_id", pendingID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// httpCancelRequest implements A9: cancel does not go through
// Resolver.ResolveBundle or the durable claim at all. It authorizes via
// AuthorizeBundleAccess (M5 identity resolution + AuthorizeTaskAccess) and,
// on success, runs today's cancel unchanged — store lookup, per-question
// UpdateClarificationMessage, then publishCancelledEvent.
func (h *Handlers) httpCancelRequest(c *gin.Context) {
	pendingID := c.Param("id")

	if !h.authorizeBundleAccessOrRespond(c, pendingID) {
		return
	}

	req, ok := h.store.GetRequest(pendingID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": errClarificationRequestNotFound})
		return
	}

	cancelled := h.store.CancelRequest(pendingID)
	if !cancelled {
		c.JSON(http.StatusConflict, gin.H{"error": "clarification request is no longer active"})
		return
	}

	if h.messageCreator != nil {
		ctx := c.Request.Context()
		for _, q := range req.Questions {
			if err := h.messageCreator.UpdateClarificationMessage(
				ctx, req.SessionID, pendingID, q.ID, string(StatusCancelled), nil,
			); err != nil {
				h.logger.Error("failed to update clarification message on cancel",
					zap.String("pending_id", pendingID),
					zap.String("question_id", q.ID),
					zap.Error(err))
			}
		}
	}

	h.publishCancelledEvent(c, pendingID, req)
	c.JSON(http.StatusOK, gin.H{"success": true})
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

func stringFromMetadata(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key].(string); ok {
		return v
	}
	return ""
}

func clarificationPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// Each durability phase gets an independent bounded window so caller
	// cancellation cannot interrupt an accepted write or its compensation. A
	// failed detached response can sequence claim, resume, and restore phases,
	// so its worst-case response latency may span three of these windows.
	return context.WithTimeout(context.WithoutCancel(ctx), clarificationPersistenceTimeout)
}

func clarificationPersistenceContextPreservingDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < clarificationPersistenceTimeout {
		return context.WithDeadline(detached, deadline)
	}
	return context.WithTimeout(detached, clarificationPersistenceTimeout)
}

func generateOptionID(questionIndex, optionIndex int) string {
	return fmt.Sprintf("q%d_opt%d", questionIndex+1, optionIndex+1)
}
