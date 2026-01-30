// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository"
	wsmsg "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

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

// Handlers provides HTTP handlers for clarification requests.
type Handlers struct {
	store          *Store
	hub            Broadcaster
	messageCreator MessageCreator
	repo           repository.Repository
	logger         *logger.Logger
}

// NewHandlers creates new clarification handlers.
func NewHandlers(store *Store, hub Broadcaster, messageCreator MessageCreator, repo repository.Repository, log *logger.Logger) *Handlers {
	return &Handlers{
		store:          store,
		hub:            hub,
		messageCreator: messageCreator,
		repo:           repo,
		logger:         log.WithFields(zap.String("component", "clarification-handlers")),
	}
}

// RegisterRoutes registers clarification HTTP routes.
func RegisterRoutes(router *gin.Engine, store *Store, hub Broadcaster, messageCreator MessageCreator, repo repository.Repository, log *logger.Logger) {
	h := NewHandlers(store, hub, messageCreator, repo, log)
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

	// Get the pending request to find the session ID
	pending, ok := h.store.GetRequest(pendingID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
		return
	}

	// Extract the single answer (first one if provided)
	var answer *Answer
	if len(body.Answers) > 0 {
		answer = &body.Answers[0]
	}

	resp := &Response{
		PendingID:    pendingID,
		Answer:       answer,
		Rejected:     body.Rejected,
		RejectReason: body.RejectReason,
		RespondedAt:  time.Now(),
	}

	if err := h.store.Respond(pendingID, resp); err != nil {
		h.logger.Warn("failed to respond to clarification",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Update the message in the database with status and answer
	status := "answered"
	if body.Rejected {
		status = "rejected"
	}
	if err := h.messageCreator.UpdateClarificationMessage(c.Request.Context(), pending.SessionID, pendingID, status, answer); err != nil {
		h.logger.Warn("failed to update clarification message",
			zap.String("pending_id", pendingID),
			zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func generateOptionID(questionIndex, optionIndex int) string {
	return fmt.Sprintf("q%d_opt%d", questionIndex+1, optionIndex+1)
}

