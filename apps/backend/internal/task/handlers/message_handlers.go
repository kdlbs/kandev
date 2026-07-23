package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// OrchestratorService defines the interface for orchestrator operations
type OrchestratorService interface {
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error)
	ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error
	StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, planMode, autoStart bool, attachments []v1.MessageAttachment) error
	ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) error
	StepRequiresCompletionSignal(ctx context.Context, taskID string) bool
}

// MessageHandlers handles WebSocket requests for messages
type MessageHandlers struct {
	service            *service.Service
	orchestrator       OrchestratorService
	logger             *logger.Logger
	referenceValidator entityrefs.SubmissionValidator
}

// NewMessageHandlers creates a new MessageHandlers instance
func NewMessageHandlers(
	svc *service.Service,
	orchestrator OrchestratorService,
	log *logger.Logger,
	validators ...entityrefs.SubmissionValidator,
) *MessageHandlers {
	handlers := &MessageHandlers{
		service:      svc,
		orchestrator: orchestrator,
		logger:       log.WithFields(zap.String("component", "task-message-handlers")),
	}
	if len(validators) > 0 {
		handlers.referenceValidator = validators[0]
	}
	return handlers
}

// RegisterMessageRoutes registers message HTTP + WebSocket handlers
func RegisterMessageRoutes(
	router *gin.Engine,
	dispatcher *ws.Dispatcher,
	svc *service.Service,
	orchestrator OrchestratorService,
	log *logger.Logger,
	validators ...entityrefs.SubmissionValidator,
) {
	handlers := NewMessageHandlers(svc, orchestrator, log, validators...)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *MessageHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/agent-sessions/:id/messages", h.httpListMessages)
	api.GET("/task-sessions/:id/messages", h.httpListMessages) // Alias for SSR compatibility
	api.GET("/task-sessions/:id/messages/:message_id/shell-output", h.httpGetShellOutput)
}

func (h *MessageHandlers) httpGetShellOutput(c *gin.Context) {
	message, err := h.service.GetMessage(c.Request.Context(), c.Param("message_id"))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			h.logger.Error("failed to get shell output message", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get shell output"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	if !ok || message.TaskSessionID != c.Param("id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	status, _ := message.Metadata["status"].(string)
	c.JSON(http.StatusOK, dto.ShellOutputSnapshotResponse{
		MessageID: message.ID,
		Status:    status,
		UpdatedAt: message.UpdatedAt,
		Output:    output,
	})
}

func (h *MessageHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionMessageAdd, h.wsAddMessage)
	dispatcher.RegisterFunc(ws.ActionMessageList, h.wsListMessages)
	dispatcher.RegisterFunc(ws.ActionMessageSearch, h.wsSearchMessages)
}

type listMessagesParams struct {
	before    string
	after     string
	sort      string
	limit     int
	paginated bool
}

func (h *MessageHandlers) parseListMessageParams(c *gin.Context) (listMessagesParams, bool) {
	before := c.Query("before")
	after := c.Query("after")
	sort := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	limitProvided := strings.TrimSpace(c.Query("limit")) != ""
	if before != "" && after != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only one of before or after can be set"})
		return listMessagesParams{}, false
	}
	if sort != "" && sort != "asc" && sort != "desc" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sort must be asc or desc"})
		return listMessagesParams{}, false
	}
	limit := 0
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	return listMessagesParams{
		before:    before,
		after:     after,
		sort:      sort,
		limit:     limit,
		paginated: limitProvided || before != "" || after != "" || sort != "",
	}, true
}

func (h *MessageHandlers) fetchMessages(
	ctx context.Context,
	sessionID string,
	params listMessagesParams,
) (dto.ListMessagesResponse, error) {
	if params.paginated {
		return h.fetchMessagesPaginated(ctx, sessionID, params)
	}
	messages, err := h.service.ListMessages(ctx, sessionID)
	if err != nil {
		return dto.ListMessagesResponse{}, err
	}
	result := messagesToAPI(messages)
	return dto.ListMessagesResponse{Messages: result, Total: len(result)}, nil
}

func (h *MessageHandlers) fetchMessagesPaginated(
	ctx context.Context,
	sessionID string,
	params listMessagesParams,
) (dto.ListMessagesResponse, error) {
	messages, hasMore, err := h.service.ListMessagesPaginated(ctx, service.ListMessagesRequest{
		TaskSessionID: sessionID,
		Limit:         params.limit,
		Before:        params.before,
		After:         params.after,
		Sort:          params.sort,
	})
	if err != nil {
		return dto.ListMessagesResponse{}, err
	}
	result := messagesToAPI(messages)
	cursor := ""
	if len(result) > 0 {
		cursor = result[len(result)-1].ID
	}
	return dto.ListMessagesResponse{
		Messages: result,
		Total:    len(result),
		HasMore:  hasMore,
		Cursor:   cursor,
	}, nil
}

func messagesToAPI(messages []*models.Message) []*v1.Message {
	result := make([]*v1.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.ToAPI())
	}
	return result
}

func (h *MessageHandlers) httpListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task session id is required"})
		return
	}
	params, ok := h.parseListMessageParams(c)
	if !ok {
		return
	}
	resp, err := h.fetchMessages(c.Request.Context(), sessionID, params)
	if err != nil {
		h.logger.Error("failed to list messages", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

type wsAddMessageRequest struct {
	TaskID            string                 `json:"task_id"`
	TaskSessionID     string                 `json:"session_id"`
	Content           string                 `json:"content"`
	AuthorID          string                 `json:"author_id,omitempty"`
	Model             string                 `json:"model,omitempty"`
	PlanMode          bool                   `json:"plan_mode,omitempty"`
	HasReviewComments bool                   `json:"has_review_comments,omitempty"`
	Attachments       []v1.MessageAttachment `json:"attachments,omitempty"`
	ContextFiles      []v1.ContextFileMeta   `json:"context_files,omitempty"`
	EntityReferences  []v1.EntityReference   `json:"entity_references,omitempty"`
}

func (h *MessageHandlers) wsAddMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAddMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.Content = strings.TrimSpace(req.Content)

	if errMsg := validateAddMessageRequest(req); errMsg != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, errMsg, nil)
	}

	// Check session state — may block the message or flag it as a create-start
	sessionResp, wsErr := h.checkSessionStateForMessage(ctx, msg, req.TaskSessionID)
	if wsErr != nil {
		return wsErr, nil
	}
	if len(req.EntityReferences) > 0 {
		if sessionResp.Session.IsPassthrough || h.referenceValidator == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid entity references", nil)
		}
		references, err := h.referenceValidator.ValidateForSubmission(
			ctx,
			req.TaskSessionID,
			req.TaskID,
			req.EntityReferences,
		)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid entity references", nil)
		}
		req.EntityReferences = references
	}

	// Transition task from REVIEW → IN_PROGRESS if needed
	if err := h.ensureTaskInProgress(ctx, req.TaskID); err != nil {
		h.logger.Error("failed to get task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task", nil)
	}

	// Run on_turn_start synchronously BEFORE wrapping the prompt with the
	// Kandev MCP system block. A workflow step transition fired by
	// on_turn_start changes which step's `auto_advance_requires_signal`
	// applies — running the wrap first would bake in the previous step's
	// flag and either hide or expose `step_complete_kandev` on the wrong
	// first turn. dispatchPromptAsync no longer calls ProcessOnTurnStart;
	// it forwards the (now correctly-wrapped) prompt to the agent.
	if h.orchestrator != nil {
		if err := h.orchestrator.ProcessOnTurnStart(ctx, req.TaskID, req.TaskSessionID); err != nil {
			h.logger.Warn("failed to process on_turn_start",
				zap.String("task_id", req.TaskID),
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
		}
		var err error
		sessionResp, err = h.resolveSessionAfterTurnStart(ctx, req.TaskID, req.TaskSessionID, sessionResp)
		if err != nil {
			h.logger.Warn("failed to resolve prompt session after on_turn_start",
				zap.String("task_id", req.TaskID),
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to resolve prompt session", nil)
		}
		req.TaskSessionID = sessionResp.Session.ID
	}
	if wsErr := h.errorForBlockedMessageSession(msg, sessionResp.Session.State); wsErr != nil {
		return wsErr, nil
	}
	isCreatedSession := sessionResp.Session.State == models.TaskSessionStateCreated

	// Build metadata with attachments, plan mode, review comments, and context files
	meta := orchestrator.NewUserMessageMeta().
		WithPlanMode(req.PlanMode).
		WithReviewComments(req.HasReviewComments).
		WithAttachments(req.Attachments).
		WithContextFiles(req.ContextFiles).
		WithEntityReferences(req.EntityReferences)

	// First message on a CREATED session is the kanban "type in chat to start
	// the agent" path. Wrap with the Kandev MCP system block before persisting
	// so the DB row matches what the agent receives (and "Show formatted"
	// reveals it). The orchestrator's wrap in StartCreatedSession is
	// idempotent (HasKandevContext guard), so passing the wrapped content
	// through dispatchPromptAsync does not double-wrap downstream.
	// NOTE: req.Content is user-controlled — do NOT guard this wrap on
	// HasKandevContext. A malicious or naive client could craft a body
	// containing a fake "<kandev-system>KANDEV MCP TOOLS</kandev-system>"
	// block and bypass server-side injection of the canonical task/session/
	// tool context. Wrap unconditionally; the orchestrator's own guard sees
	// our wrap downstream and skips its second pass.
	// Passthrough sessions skip the wrap: the prompt is typed straight into
	// the agent CLI's TTY and the user sees it verbatim — they don't want a
	// wall of MCP-tool boilerplate prepended to "hello".
	storedContent := orchestrator.AppendEntityReferenceContext(req.Content, req.EntityReferences)
	if isCreatedSession && !sessionResp.Session.IsPassthrough && (req.Content != "" || len(req.Attachments) > 0) {
		task, err := h.service.GetTask(ctx, req.TaskID)
		if err != nil {
			h.logger.Error("failed to resolve first-turn MCP capabilities", zap.String("task_id", req.TaskID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task", nil)
		}
		configMode, _ := sessionResp.Session.Metadata["config_mode"].(bool)
		requiresSignal := h.orchestrator != nil && h.orchestrator.StepRequiresCompletionSignal(ctx, req.TaskID)
		storedContent = sysprompt.InjectKandevContextWithOptions(req.TaskID, req.TaskSessionID, storedContent, sysprompt.KandevContextOptions{
			RequiresCompletionSignal:       requiresSignal,
			IncludeCoordinatorTaskControls: !task.IsOfficeOwnedAndAssigned() && !configMode,
		})
	}
	req.Content = storedContent

	message, err := h.service.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		Content:       storedContent,
		AuthorType:    "user",
		AuthorID:      req.AuthorID,
		Metadata:      meta.ToMap(),
	})
	if err != nil {
		h.logger.Error("failed to create message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create message", nil)
	}

	apiMsg := message.ToAPI()
	response, err := ws.NewResponse(msg.ID, msg.Action, apiMsg)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to encode response", nil)
	}

	// Auto-forward message as prompt to running agent if orchestrator is available.
	// This runs async so the WS request can respond immediately.
	// Use context.WithoutCancel so the prompt continues even if the WebSocket client disconnects.
	if h.orchestrator != nil {
		h.dispatchPromptAsync(ctx, req, sessionResp.Session.AgentProfileID, isCreatedSession)
	}

	return response, nil
}

func (h *MessageHandlers) resolveSessionAfterTurnStart(
	ctx context.Context,
	taskID, submittedSessionID string,
	current *dto.GetTaskSessionResponse,
) (*dto.GetTaskSessionResponse, error) {
	if current.Session.ID == "" {
		return nil, errors.New("submitted session response missing session id")
	}
	reloaded, err := h.service.GetTaskSession(ctx, submittedSessionID)
	if err != nil {
		h.logger.Warn("failed to reload session after on_turn_start",
			zap.String("task_id", taskID),
			zap.String("session_id", submittedSessionID),
			zap.Error(err))
		return nil, errors.New("failed to reload submitted session after on_turn_start")
	}
	if reloaded.State != models.TaskSessionStateCompleted {
		return &dto.GetTaskSessionResponse{Session: dto.FromTaskSession(reloaded)}, nil
	}
	primary, err := h.service.GetPrimarySession(ctx, taskID)
	if err != nil || primary == nil {
		if err != nil {
			h.logger.Warn("failed to load primary session after on_turn_start switch",
				zap.String("task_id", taskID),
				zap.String("session_id", submittedSessionID),
				zap.Error(err))
		}
		return nil, errors.New("submitted session completed during on_turn_start without replacement primary session")
	}
	if primary.ID == submittedSessionID {
		return nil, errors.New("submitted session completed during on_turn_start but remains primary")
	}
	return &dto.GetTaskSessionResponse{Session: dto.FromTaskSession(primary)}, nil
}

func (h *MessageHandlers) errorForBlockedMessageSession(msg *ws.Message, state models.TaskSessionState) *ws.Message {
	switch state {
	case models.TaskSessionStateRunning:
		wsErr, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Agent is currently processing. Please wait for the current operation to complete.", nil)
		return wsErr
	case models.TaskSessionStateFailed, models.TaskSessionStateCancelled, models.TaskSessionStateCompleted:
		wsErr, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Session has ended. Please create a new session to continue.", nil)
		return wsErr
	default:
		return nil
	}
}

// validateAddMessageRequest returns a non-empty error string if the request is invalid.
func validateAddMessageRequest(req wsAddMessageRequest) string {
	if req.TaskSessionID == "" {
		return "session_id is required"
	}
	if req.TaskID == "" {
		return "task_id is required"
	}
	// Content can be empty if there are attachments (image-only messages)
	if req.Content == "" && len(req.Attachments) == 0 {
		return "content or attachments are required"
	}
	if err := validateAttachments(req.Attachments); err != nil {
		return err.Error()
	}
	return ""
}

// checkSessionStateForMessage loads the session and returns an error WS message if the
// session is in a state that blocks new messages (running, failed, cancelled).
func (h *MessageHandlers) checkSessionStateForMessage(ctx context.Context, msg *ws.Message, sessionID string) (*dto.GetTaskSessionResponse, *ws.Message) {
	session, err := h.service.GetTaskSession(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to get task session", zap.String("session_id", sessionID), zap.Error(err))
		wsErr, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task session", nil)
		return nil, wsErr
	}
	sessionDTO := dto.FromTaskSession(session)
	resp := &dto.GetTaskSessionResponse{Session: sessionDTO}
	if wsErr := h.errorForBlockedMessageSession(msg, sessionDTO.State); wsErr != nil {
		if sessionDTO.State == models.TaskSessionStateRunning {
			h.logBlockedRunningSession(sessionID, sessionDTO.State)
		}
		return nil, wsErr
	}
	return resp, nil
}

func (h *MessageHandlers) logBlockedRunningSession(sessionID string, state models.TaskSessionState) {
	h.logger.Warn("rejected message submission while agent is busy",
		zap.String("session_id", sessionID),
		zap.String("session_state", string(state)))
}

// ensureTaskInProgress fetches the task and transitions it from REVIEW → IN_PROGRESS if needed.
// Returns an error only when the task cannot be fetched.
func (h *MessageHandlers) ensureTaskInProgress(ctx context.Context, taskID string) error {
	task, err := h.service.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.State != v1.TaskStateReview {
		return nil
	}
	if _, err := h.service.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		h.logger.Error("failed to transition task from REVIEW to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		h.logger.Info("task transitioned from REVIEW to IN_PROGRESS",
			zap.String("task_id", taskID))
	}
	return nil
}

// dispatchPromptAsync forwards the message to the agent as a prompt in a
// background goroutine. The caller (wsAddMessage) is responsible for running
// on_turn_start synchronously BEFORE wrapping the prompt, so this function
// only handles the agent-facing dispatch.
func (h *MessageHandlers) dispatchPromptAsync(ctx context.Context, req wsAddMessageRequest, agentProfileID string, isCreatedSession bool) {
	taskID := req.TaskID
	sessionID := req.TaskSessionID
	content := req.Content
	model := req.Model
	planMode := req.PlanMode
	attachments := req.Attachments
	go func() {
		promptCtx := context.WithoutCancel(ctx)
		h.forwardMessageAsPrompt(promptCtx, taskID, sessionID, agentProfileID, content, model, planMode, attachments, isCreatedSession)
	}()
}

// forwardMessageAsPrompt sends a user message to the agent as a prompt.
// For CREATED sessions, it starts the agent; otherwise it prompts the running agent,
// with automatic resume handling if the agent is not found.
func (h *MessageHandlers) forwardMessageAsPrompt(
	ctx context.Context,
	taskID, sessionID, agentProfileID, content, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	startCreated bool,
) {
	// For CREATED sessions, start the agent with this message as the initial prompt
	if startCreated {
		if err := h.orchestrator.StartCreatedSession(ctx, taskID, sessionID, agentProfileID, content, true, planMode, false, attachments); err != nil {
			h.logger.Warn("failed to start created session from message",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))

			errorMsg := "Failed to start agent"
			if _, createErr := h.service.CreateMessage(ctx, &service.CreateMessageRequest{
				TaskSessionID: sessionID,
				TaskID:        taskID,
				Content:       errorMsg,
				AuthorType:    "agent",
				Type:          string(v1.MessageTypeError),
				Metadata: map[string]interface{}{
					"error": err.Error(),
				},
			}); createErr != nil {
				h.logger.Error("failed to create error message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(createErr))
			}
		}
		return
	}

	_, err := h.orchestrator.PromptTask(ctx, taskID, sessionID, content, model, planMode, attachments, false)
	if err != nil {
		err = h.handlePromptWithResume(ctx, taskID, sessionID, content, model, planMode, attachments, err)
	}
	if err != nil {
		// Don't create a prompt error message if the agent itself reported the error.
		// The agent failure path (handleAgentFailed) already sets the session to FAILED
		// with the error_message, which the UI displays via agent-status.
		if !isAgentReportedError(err) {
			h.createPromptErrorMessage(ctx, taskID, sessionID, err)
		}
	}
}

// isAgentReportedError returns true when the error originated from the agent's
// own error event (surfaced via waitForPromptDone with the ErrAgentReported
// sentinel wrapped in).
func isAgentReportedError(err error) bool {
	return errors.Is(err, lifecycle.ErrAgentReported)
}

// isTimeoutError reports whether err looks like a timeout. Used by
// createPromptErrorMessage to render the "Request timed out…" UX hint.
//
// Several upstream producers along the prompt path (waitForSessionReady,
// agent-stream connect waits, agentctl health waits) return
// fmt.Errorf("timeout …") rather than wrapping a typed timeout, so a strict
// errors.As(net.Error) check would silently downgrade their user message to
// the generic "Failed to send message to agent". The substring fallback
// preserves the pre-refactor UX for those cases; classifying upstream errors
// properly is tracked separately.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

// handlePromptWithResume attempts to resume a session and retry a prompt when the
// initial prompt fails with ErrExecutionNotFound.
func (h *MessageHandlers) handlePromptWithResume(
	ctx context.Context,
	taskID, sessionID, content, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	origErr error,
) error {
	if !errors.Is(origErr, executor.ErrExecutionNotFound) {
		return origErr
	}
	if resumeErr := h.orchestrator.ResumeTaskSession(ctx, taskID, sessionID); resumeErr != nil {
		h.logger.Warn("failed to resume task session for prompt",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(resumeErr))
		return origErr
	}
	// Wait for the agent to become ready after resume.
	// ResumeTaskSession starts the agent asynchronously, so we poll
	// the session state until it transitions to a promptable state.
	if waitErr := h.waitForSessionReady(ctx, sessionID); waitErr != nil {
		h.logger.Warn("session did not become ready after resume",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(waitErr))
		return waitErr
	}
	_, err := h.orchestrator.PromptTask(ctx, taskID, sessionID, content, model, planMode, attachments, false)
	return err
}

// createPromptErrorMessage creates an agent error message visible to the user when
// forwarding a prompt to the agent fails.
func (h *MessageHandlers) createPromptErrorMessage(ctx context.Context, taskID, sessionID string, promptErr error) {
	h.logger.Warn("failed to forward message as prompt to agent",
		zap.String("task_id", taskID),
		zap.Error(promptErr))

	errorMsg := "Failed to send message to agent"
	if isTimeoutError(promptErr) {
		// isTimeoutError already covers context.DeadlineExceeded (which
		// implements Timeout()==true) and the substring fallback for plain
		// "timeout …" producers — no separate errors.Is needed here.
		errorMsg = "Request timed out. The agent may be processing a complex task. Please try again."
	} else if errors.Is(promptErr, executor.ErrExecutionNotFound) {
		errorMsg = "Agent is not running. Please restart the session."
	}

	if _, createErr := h.service.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       errorMsg,
		AuthorType:    "agent",
		Type:          string(v1.MessageTypeError),
		Metadata: map[string]interface{}{
			"error": promptErr.Error(),
		},
	}); createErr != nil {
		h.logger.Error("failed to create error message for prompt failure",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(createErr))
	}
}

// waitForSessionReady delegates to the shared service.WaitForSessionReady helper.
// Kept as a thin wrapper so existing tests on this method continue to pass.
func (h *MessageHandlers) waitForSessionReady(ctx context.Context, sessionID string) error {
	return h.service.WaitForSessionReady(ctx, sessionID)
}

type wsListMessagesRequest struct {
	TaskSessionID string `json:"session_id"`
	Limit         int    `json:"limit"`
	Before        string `json:"before"`
	After         string `json:"after"`
	Sort          string `json:"sort"`
}

type wsSearchMessagesRequest struct {
	TaskSessionID string `json:"session_id"`
	Query         string `json:"query"`
	Limit         int    `json:"limit"`
}

const messageSnippetRadius = 60

// buildSnippet returns a short excerpt around the first case-insensitive match
// of query within content. Falls back to a leading slice when no match found
// (e.g. query matched only raw_content). Works in rune space so multi-byte
// characters (emoji, CJK, accented letters) are never sliced mid-rune.
func buildSnippet(content, query string) string {
	if content == "" {
		return ""
	}
	contentRunes := []rune(content)
	queryRunes := []rune(strings.TrimSpace(query))
	maxLen := messageSnippetRadius*2 + len(queryRunes)

	idx := -1
	if len(queryRunes) > 0 {
		idx = indexRunesFold(contentRunes, queryRunes)
	}
	if idx < 0 {
		if len(contentRunes) <= maxLen {
			return content
		}
		return strings.TrimSpace(string(contentRunes[:maxLen])) + "…"
	}
	start := idx - messageSnippetRadius
	if start < 0 {
		start = 0
	}
	end := idx + len(queryRunes) + messageSnippetRadius
	if end > len(contentRunes) {
		end = len(contentRunes)
	}
	snippet := string(contentRunes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(contentRunes) {
		snippet += "…"
	}
	return snippet
}

// indexRunesFold returns the rune-index of the first case-insensitive
// occurrence of needle in haystack, or -1. Comparison is per-rune via
// unicode.ToLower — 1:1 rune count, so the returned index is always a valid
// slice boundary in the original haystack.
func indexRunesFold(haystack, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if unicode.ToLower(haystack[i+j]) != unicode.ToLower(needle[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func (h *MessageHandlers) wsSearchMessages(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSearchMessagesRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return ws.NewResponse(msg.ID, msg.Action, dto.SearchMessagesResponse{Hits: []dto.MessageSearchHit{}, Total: 0})
	}

	messages, err := h.service.SearchMessages(ctx, req.TaskSessionID, query, req.Limit)
	if err != nil {
		h.logger.Error("failed to search messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to search messages", nil)
	}
	hits := make([]dto.MessageSearchHit, 0, len(messages))
	for _, m := range messages {
		hits = append(hits, dto.MessageSearchHit{
			ID:         m.ID,
			TurnID:     m.TurnID,
			AuthorType: string(m.AuthorType),
			Type:       string(m.Type),
			Snippet:    buildSnippet(m.Content, query),
			CreatedAt:  m.CreatedAt,
		})
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.SearchMessagesResponse{Hits: hits, Total: len(hits)})
}

func (h *MessageHandlers) wsListMessages(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListMessagesRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Before != "" && req.After != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "only one of before or after can be set", nil)
	}
	if req.Sort != "" && req.Sort != "asc" && req.Sort != "desc" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "sort must be asc or desc", nil)
	}

	messages, hasMore, err := h.service.ListMessagesPaginated(ctx, service.ListMessagesRequest{
		TaskSessionID: req.TaskSessionID,
		Limit:         req.Limit,
		Before:        req.Before,
		After:         req.After,
		Sort:          req.Sort,
	})
	if err != nil {
		h.logger.Error("failed to list messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list messages", nil)
	}
	result := make([]*v1.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.ToAPI())
	}
	cursor := ""
	if len(result) > 0 {
		cursor = result[len(result)-1].ID
	}
	resp := dto.ListMessagesResponse{
		Messages: result,
		Total:    len(result),
		HasMore:  hasMore,
		Cursor:   cursor,
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
