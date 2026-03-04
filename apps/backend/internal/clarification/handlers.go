// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wsmsg "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// messageStore is the minimal task repository interface required by clarification handlers.
type messageStore interface {
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
	FindMessageByPendingID(ctx context.Context, pendingID string) (*taskmodels.Message, error)
	UpdateMessage(ctx context.Context, message *taskmodels.Message) error
}

// Broadcaster interface for sending WebSocket notifications
type Broadcaster interface {
	BroadcastToSession(sessionID string, msg *wsmsg.Message)
}

// MessageCreator interface for creating messages in the database
type MessageCreator interface {
	// CreateClarificationRequestMessage creates a message for a clarification request
	CreateClarificationRequestMessage(ctx context.Context, taskID, sessionID, pendingID string, question Question, clarificationContext string) (string, error)
	// UpdateClarificationMessage updates a clarification message's status and response
	UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, status string, answer *Answer) error
}

// EventBus interface for publishing events.
type EventBus interface {
	Publish(ctx context.Context, topic string, event *bus.Event) error
}

// Handlers provides HTTP handlers for clarification requests.
type Handlers struct {
	store          *Store
	hub            Broadcaster
	messageCreator MessageCreator
	repo           messageStore
	eventBus       EventBus
	logger         *logger.Logger
}

// NewHandlers creates new clarification handlers.
func NewHandlers(store *Store, hub Broadcaster, messageCreator MessageCreator, repo messageStore, eventBus EventBus, log *logger.Logger) *Handlers {
	return &Handlers{
		store:          store,
		hub:            hub,
		messageCreator: messageCreator,
		repo:           repo,
		eventBus:       eventBus,
		logger:         log.WithFields(zap.String("component", "clarification-handlers")),
	}
}

// RegisterRoutes registers clarification HTTP routes.
func RegisterRoutes(router *gin.Engine, store *Store, hub Broadcaster, messageCreator MessageCreator, repo messageStore, eventBus EventBus, log *logger.Logger) {
	h := NewHandlers(store, hub, messageCreator, repo, eventBus, log)
	api := router.Group("/api/v1/clarification")
	api.POST("/request", h.httpCreateRequest)
	api.GET("/:id", h.httpGetRequest)
	api.GET("/:id/wait", h.httpWaitForResponse)
	api.POST("/:id/respond", h.httpRespond)
}

// CreateRequestBody is the request body for creating a clarification request.
type CreateRequestBody struct {
	SessionID string   `json:"session_id" binding:"required"`
	TaskID    string   `json:"task_id"`
	Question  Question `json:"question" binding:"required"`
	Context   string   `json:"context"`
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

	// Validate question has ID, generate if missing
	if body.Question.ID == "" {
		body.Question.ID = "q1"
	}
	// Validate options have IDs
	for j := range body.Question.Options {
		if body.Question.Options[j].ID == "" {
			body.Question.Options[j].ID = generateOptionID(0, j)
		}
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
		Question:  body.Question,
		Context:   body.Context,
	}

	pendingID := h.store.CreateRequest(req)

	// Create a message in the database for the clarification request.
	// This triggers the session.message.added WebSocket event which the frontend listens to.
	if h.messageCreator != nil {
		_, err := h.messageCreator.CreateClarificationRequestMessage(
			c.Request.Context(),
			taskID,
			sessionID,
			pendingID,
			body.Question,
			body.Context,
		)
		if err != nil {
			h.logger.Error("failed to create clarification request message",
				zap.String("pending_id", pendingID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			// Don't fail the request - the clarification is still created in the store
		}
	}

	c.JSON(http.StatusOK, CreateRequestResponse{PendingID: pendingID})
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
type RespondBody struct {
	Answers      []Answer `json:"answers"` // Keep as array for frontend compatibility, but only first is used
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

	// Extract the single answer (first one if provided)
	var answer *Answer
	if len(body.Answers) > 0 {
		answer = &body.Answers[0]
	}

	// Build the response object
	resp := &Response{
		PendingID:    pendingID,
		Answer:       answer,
		Rejected:     body.Rejected,
		RejectReason: body.RejectReason,
	}

	// Try the primary path: deliver via channel to blocking WaitForResponse.
	// If the agent is still waiting, this unblocks the MCP handler and the
	// answer is returned within the same agent turn (no extra cost).
	err := h.store.Respond(pendingID, resp)

	if err == nil {
		// Primary path succeeded — agent will receive the answer directly.
		h.updateClarificationMessage(c, pendingID, body.Rejected, answer)
		h.logger.Info("clarification answered via primary path (same turn)",
			zap.String("pending_id", pendingID))
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// Duplicate response — someone clicked twice quickly.
	if errors.Is(err, ErrAlreadyResponded) {
		h.logger.Warn("duplicate response attempt",
			zap.String("pending_id", pendingID))
		c.JSON(http.StatusConflict, gin.H{"error": "response already submitted"})
		return
	}

	// Fallback path: entry not found (agent timed out, entry was cleaned up).
	// Look up the original request context from the database message and
	// publish an event so the orchestrator resumes the agent with a new turn.
	h.logger.Info("clarification entry not found, using event fallback",
		zap.String("pending_id", pendingID),
		zap.String("error", err.Error()))

	h.updateClarificationMessage(c, pendingID, body.Rejected, answer)
	h.respondViaEventFallback(c, pendingID, answer, body.Rejected, body.RejectReason)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// updateClarificationMessage updates the message in the database with status and answer.
func (h *Handlers) updateClarificationMessage(c *gin.Context, pendingID string, rejected bool, answer *Answer) {
	if h.messageCreator == nil {
		return
	}
	sessionID := h.lookupSessionForPending(c, pendingID)
	status := "answered"
	if rejected {
		status = "rejected"
	}
	if err := h.messageCreator.UpdateClarificationMessage(c.Request.Context(), sessionID, pendingID, status, answer); err != nil {
		h.logger.Warn("failed to update clarification message",
			zap.String("pending_id", pendingID),
			zap.Error(err))
	}
}

// respondViaEventFallback publishes a ClarificationAnswered event for the orchestrator
// to resume the agent with a new turn. Used when the agent timed out.
func (h *Handlers) respondViaEventFallback(c *gin.Context, pendingID string, answer *Answer, rejected bool, rejectReason string) {
	if h.eventBus == nil {
		return
	}

	// Look up session/task/question from the database message
	msg, err := h.repo.FindMessageByPendingID(c.Request.Context(), pendingID)
	if err != nil {
		h.logger.Error("failed to find message for event fallback",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return
	}

	sessionID := msg.TaskSessionID
	taskID := msg.TaskID
	question := ""
	if msg.Metadata != nil {
		if qData, ok := msg.Metadata["question"].(map[string]interface{}); ok {
			if q, ok := qData["prompt"].(string); ok {
				question = q
			}
		}
	}

	answerText := buildAnswerText(answer, rejected, rejectReason)

	eventData := map[string]any{
		"session_id":    sessionID,
		"task_id":       taskID,
		"question":      question,
		"answer_text":   answerText,
		"rejected":      rejected,
		"reject_reason": rejectReason,
	}
	if err := h.eventBus.Publish(c.Request.Context(), events.ClarificationAnswered, bus.NewEvent(
		events.ClarificationAnswered,
		"clarification-handlers",
		eventData,
	)); err != nil {
		h.logger.Error("failed to publish clarification answered event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	h.logger.Info("clarification answered via event fallback (new turn)",
		zap.String("pending_id", pendingID),
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID))
}

// lookupSessionForPending returns the session ID for a pending clarification.
// Falls back to finding it from the database message.
func (h *Handlers) lookupSessionForPending(c *gin.Context, pendingID string) string {
	// Try the in-memory store first
	if req, ok := h.store.GetRequest(pendingID); ok {
		return req.SessionID
	}
	// Fall back to database
	msg, err := h.repo.FindMessageByPendingID(c.Request.Context(), pendingID)
	if err != nil {
		return ""
	}
	return msg.TaskSessionID
}

// buildAnswerText constructs a human-readable answer text from the response.
func buildAnswerText(answer *Answer, rejected bool, rejectReason string) string {
	if rejected {
		if rejectReason != "" {
			return fmt.Sprintf("User declined to answer. Reason: %s", rejectReason)
		}
		return "User declined to answer."
	}
	if answer == nil {
		return "User provided no specific answer."
	}
	if answer.CustomText != "" {
		return fmt.Sprintf("User answered: %s", answer.CustomText)
	}
	if len(answer.SelectedOptions) > 0 {
		return fmt.Sprintf("User selected: %v", answer.SelectedOptions)
	}
	return "User provided no specific answer."
}

func generateOptionID(questionIndex, optionIndex int) string {
	return fmt.Sprintf("q%d_opt%d", questionIndex+1, optionIndex+1)
}
