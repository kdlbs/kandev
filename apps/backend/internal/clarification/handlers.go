// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

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
)

// messageStore is the minimal task repository interface required by clarification handlers.
type messageStore interface {
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
	FindMessageByPendingID(ctx context.Context, pendingID string) (*taskmodels.Message, error)
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*taskmodels.Message, error)
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

	pendingID := h.store.CreateRequest(req)

	// Create one message per question in the database; all share the same
	// pending_id and are rendered as a stacked group on the frontend. The
	// session.message.added WebSocket event fires per message. On failure we
	// also cancel the in-store pending entry so any blocking WaitForResponse
	// caller unblocks immediately rather than waiting for the MCP timeout.
	if h.messageCreator != nil {
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

	// Gate: when not rejecting, the user must have answered every question and
	// each answer must reference a question id from the original bundle (no
	// duplicates, no fabricated ids). We compare against the in-store request
	// first; if the entry is gone (agent already moved on), fall back to the
	// persisted messages so we still validate even after the in-memory cleanup.
	if !body.Rejected {
		if errMsg := h.validateRespondAnswers(c.Request.Context(), pendingID, body.Answers); errMsg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
			return
		}
	}

	resp := &Response{
		PendingID:    pendingID,
		Answers:      body.Answers,
		Rejected:     body.Rejected,
		RejectReason: body.RejectReason,
	}

	// Try the primary path: deliver via channel to blocking WaitForResponse.
	// If the agent is still waiting, this unblocks the MCP handler and the
	// answer is returned within the same agent turn (no extra cost).
	err := h.store.Respond(pendingID, resp)

	if err == nil {
		// Primary path succeeded — agent will receive the answers directly.
		h.applyAnswersToMessages(c, pendingID, body.Rejected, body.Answers)
		h.publishPrimaryAnsweredEvent(c, pendingID, body.Answers, body.Rejected, body.RejectReason)
		h.logger.Info("clarification answered via primary path (same turn)",
			zap.String("pending_id", pendingID),
			zap.Int("answers", len(body.Answers)),
			zap.Bool("rejected", body.Rejected))
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
	// If the user rejected (clicked X to dismiss), they're discarding a stale
	// overlay — not continuing the conversation. Treat as a no-op so we don't
	// surprise them by resuming the agent with "User declined to answer".
	// The message status is already "expired" (set by the canceller), so the
	// chat history will keep rendering the "Timed out" entry.
	if body.Rejected {
		h.logger.Info("clarification rejected after agent moved on; no-op",
			zap.String("pending_id", pendingID))
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// User is providing an affirmative answer after the agent moved on. Update
	// the clarification record and publish an event so the orchestrator resumes
	// the agent with a new turn containing the answer.
	h.logger.Info("clarification entry not found, using event fallback",
		zap.String("pending_id", pendingID),
		zap.String("error", err.Error()))

	h.applyAnswersToMessages(c, pendingID, body.Rejected, body.Answers)
	h.respondViaEventFallback(c, pendingID, body.Answers, body.Rejected, body.RejectReason)

	c.JSON(http.StatusOK, gin.H{"success": true})
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
// answer for the given pending bundle. Falls back to the persisted messages if
// the in-store request has been cleaned up.
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
		if id := stringFromMetadata(m.Metadata, metaQuestionIDKey); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// applyAnswersToMessages flips per-question status (answered/rejected) on every
// message that belongs to the bundle. When rejected, every question is marked
// rejected; when answered, each answer updates the matching question's row.
func (h *Handlers) applyAnswersToMessages(c *gin.Context, pendingID string, rejected bool, answers []Answer) {
	if h.messageCreator == nil {
		return
	}
	sessionID := h.lookupSessionForPending(c, pendingID)

	if rejected {
		// Mark every question in the bundle as rejected. Guard h.repo for
		// parity with the sibling expectedQuestionIDs path; production wires
		// both, but unit tests sometimes pass a nil repo.
		if h.repo == nil {
			return
		}
		msgs, err := h.repo.FindMessagesByPendingID(c.Request.Context(), pendingID)
		if err != nil || len(msgs) == 0 {
			h.logger.Debug("rejected clarification: no messages to update",
				zap.String("pending_id", pendingID),
				zap.Error(err))
			return
		}
		for _, msg := range msgs {
			questionID := stringFromMetadata(msg.Metadata, "question_id")
			if questionID == "" {
				continue
			}
			if err := h.messageCreator.UpdateClarificationMessage(c.Request.Context(), sessionID, pendingID, questionID, "rejected", nil); err != nil {
				h.logger.Warn("failed to mark clarification question rejected",
					zap.String("pending_id", pendingID),
					zap.String("question_id", questionID),
					zap.Error(err))
			}
		}
		return
	}

	for i := range answers {
		ans := answers[i]
		if ans.QuestionID == "" {
			continue
		}
		if err := h.messageCreator.UpdateClarificationMessage(c.Request.Context(), sessionID, pendingID, ans.QuestionID, "answered", &ans); err != nil {
			h.logger.Warn("failed to update clarification question",
				zap.String("pending_id", pendingID),
				zap.String("question_id", ans.QuestionID),
				zap.Error(err))
		}
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

// respondViaEventFallback publishes a ClarificationAnswered event for the orchestrator
// to resume the agent with a new turn. Used when the agent timed out.
func (h *Handlers) respondViaEventFallback(c *gin.Context, pendingID string, answers []Answer, rejected bool, rejectReason string) {
	if h.eventBus == nil {
		return
	}

	clarificationCtx, err := h.resolveClarificationEventContext(c.Request.Context(), pendingID)
	if err != nil {
		h.logger.Error("failed to resolve context for clarification fallback event",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return
	}
	if clarificationCtx.SessionID == "" || clarificationCtx.TaskID == "" {
		h.logger.Error("missing session/task for clarification fallback event",
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
	if err := h.eventBus.Publish(c.Request.Context(), events.ClarificationAnswered, bus.NewEvent(
		events.ClarificationAnswered,
		"clarification-handlers",
		eventData,
	)); err != nil {
		h.logger.Error("failed to publish clarification answered event",
			zap.String("pending_id", pendingID),
			zap.String("session_id", clarificationCtx.SessionID),
			zap.Error(err))
	}

	h.logger.Info("clarification answered via event fallback (new turn)",
		zap.String("pending_id", pendingID),
		zap.String("session_id", clarificationCtx.SessionID),
		zap.String("task_id", clarificationCtx.TaskID))
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

func (h *Handlers) publishPrimaryAnsweredEvent(c *gin.Context, pendingID string, answers []Answer, rejected bool, rejectReason string) {
	if h.eventBus == nil {
		return
	}
	clarificationCtx, err := h.resolveClarificationEventContext(c.Request.Context(), pendingID)
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
	if err := h.eventBus.Publish(c.Request.Context(), events.ClarificationPrimaryAnswered, bus.NewEvent(
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
