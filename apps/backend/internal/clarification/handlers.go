// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
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
)

// messageStore is the minimal task repository interface required by clarification handlers.
type messageStore interface {
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
	FindMessageByPendingID(ctx context.Context, pendingID string) (*taskmodels.Message, error)
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*taskmodels.Message, error)
	FindPendingClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*taskmodels.Message, error)
	UpdateMessage(ctx context.Context, message *taskmodels.Message) error
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
	resolver       *Resolver
	logger         *logger.Logger
}

// NewHandlers creates new clarification handlers.
func NewHandlers(store *Store, hub Broadcaster, messageCreator MessageCreator, repo messageStore, eventBus EventBus, resolver *Resolver, log *logger.Logger) *Handlers {
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
func RegisterRoutes(router *gin.Engine, store *Store, hub Broadcaster, messageCreator MessageCreator, repo messageStore, eventBus EventBus, resolver *Resolver, log *logger.Logger) {
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

func (h *Handlers) httpGetRequest(c *gin.Context) {
	pendingID := c.Param("id")

	// A2/A3/A7/A8: authorize against the bundle's durable task_id before the
	// in-memory read, so a foreign or nonexistent pending_id is the same 404.
	if _, _, err := h.resolver.AuthorizeBundleAccess(c.Request.Context(), pendingID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
		return
	}

	req, ok := h.store.GetRequest(pendingID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
		return
	}

	c.JSON(http.StatusOK, req)
}

func (h *Handlers) httpWaitForResponse(c *gin.Context) {
	pendingID := c.Param("id")

	// A2/A3/A5a/A7/A8: same authorization as httpGetRequest, run first. A
	// pending_id with no durable messages is now 404 rather than the 504 a
	// missing in-memory entry produces below.
	if _, _, err := h.resolver.AuthorizeBundleAccess(c.Request.Context(), pendingID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
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

	res, claimed, err := h.resolver.ResolveBundle(c.Request.Context(), pendingID, Outcome{
		Answers:      body.Answers,
		Rejected:     body.Rejected,
		RejectReason: body.RejectReason,
		Source:       taskmodels.ClarificationResolutionSourceWeb,
		ResolvedBy:   resolvedByFromContext(c.Request.Context()),
	})
	h.writeResolutionResult(c, pendingID, res, claimed, err)
}

func (h *Handlers) httpCancelRequest(c *gin.Context) {
	pendingID := c.Param("id")

	res, claimed, err := h.resolver.ResolveBundle(c.Request.Context(), pendingID, Outcome{
		Cancel:     true,
		Source:     taskmodels.ClarificationResolutionSourceWeb,
		ResolvedBy: resolvedByFromContext(c.Request.Context()),
	})
	h.writeResolutionResult(c, pendingID, res, claimed, err)
}

// writeResolutionResult maps a ResolveBundle outcome to R10's REST envelope,
// shared by httpRespond and httpCancelRequest. ErrBundleNotFound (A3, A5),
// a validation error (N6-N8b — cancel never produces one, X5), and a
// partialApplicationError (R5) each map to their own status code; every
// other error is an unexpected 500. A win or a loss both report the same
// 200 envelope, distinguished only by "claimed" (R2, R10, R11).
func (h *Handlers) writeResolutionResult(c *gin.Context, pendingID string, res *Resolution, claimed bool, err error) {
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"claimed":  claimed,
			"status":   res.Status,
			"response": res.Response,
			"resume":   res.Resume,
		})
	case errors.Is(err, ErrBundleNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "clarification request not found"})
	case IsValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case IsPartialApplicationError(err):
		h.logger.Error("clarification bundle partially applied",
			zap.String("pending_id", pendingID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		h.logger.Error("failed to resolve clarification bundle",
			zap.String("pending_id", pendingID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// resolvedByFromContext returns the caller's user ID for the resolutions
// row's diagnostic-only resolved_by column, or "" for an unscoped caller —
// no identity in context, or the synthetic single-user identity auth-disabled
// mode injects (internal/auth/authn; see apps/backend/AGENTS.md's per-user
// scoping notes).
func resolvedByFromContext(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return ""
	}
	return identity.UserID
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

func generateOptionID(questionIndex, optionIndex int) string {
	return fmt.Sprintf("q%d_opt%d", questionIndex+1, optionIndex+1)
}
