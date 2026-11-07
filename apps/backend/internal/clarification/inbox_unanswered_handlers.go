package clarification

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// unansweredInboxStore is the repository surface for the Needs-you Inbox's
// unanswered section: questions a newer turn superseded while they were
// still pending. It is optional; a store without it serves the section as
// disabled.
type unansweredInboxStore interface {
	GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error)
	ListUnansweredClarificationBundles(ctx context.Context, opts taskmodels.ListUnansweredClarificationsOptions) (*taskmodels.ClarificationBundlePage, error)
	CloseUnansweredClarificationBundle(ctx context.Context, pendingID string, closeReq taskmodels.CloseUnansweredClarification) ([]*taskmodels.Message, error)
}

// Shared question metadata is not a per-user inbox sidecar: changing it
// requires the existing task-write or session-prompt authority for the action.
type unansweredActionAuthorizer interface {
	AuthorizeTaskScope(context.Context, string, authz.Scope) error
	AuthorizeTaskSessionPromptAccess(context.Context, string, string) error
}

const (
	maxLateAnswerMessageIDLength = 128

	unansweredConflictCode    = "not_unanswered"
	errUnansweredNotAvailable = "clarification is not an unanswered question"
	errUnansweredWrite        = "failed to update unanswered question"
)

// inboxUnansweredBundleView is one row of the unanswered section. The agent is
// not waiting on these bundles; answering sends an ordinary message.
type inboxUnansweredBundleView struct {
	inboxBundleView
	// OriginalSessionTerminal reports that the asking session has ended.
	OriginalSessionTerminal bool `json:"original_session_terminal"`
	// SendToTaskSessionID is the task's current primary session, offered as
	// an explicit alternative target only while the asking session has ended.
	SendToTaskSessionID *string `json:"send_to_task_session_id"`
}

// inboxUnansweredListResponse is GET /api/v1/clarification-inbox/unanswered's
// response envelope. Enabled=false means the workspace setting is off and the
// section is not shown.
type inboxUnansweredListResponse struct {
	Enabled       bool                        `json:"enabled"`
	RetentionDays int                         `json:"retention_days"`
	Bundles       []inboxUnansweredBundleView `json:"bundles"`
	Count         int                         `json:"count"`
	NextCursor    string                      `json:"next_cursor,omitempty"`
}

// inboxConflictBody is a 409 body with a machine-readable code.
type inboxConflictBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// inboxLateAnswerBody is the late-answer close request: the identity of the
// ordinary message the client has already had admitted.
type inboxLateAnswerBody struct {
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
}

// httpListInboxUnanswered backs GET /api/v1/clarification-inbox/unanswered,
// authorized exactly like the main Needs-you read.
func (h *Handlers) httpListInboxUnanswered(c *gin.Context) {
	requestCtx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(requestCtx, inboxReadTimeout)
	defer cancel()
	if err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		if h.respondInboxReadError(c, requestCtx, err) {
			return
		}
		h.respondInboxWorkspaceAuthzError(c, err)
		return
	}
	resp := inboxUnansweredListResponse{Bundles: []inboxUnansweredBundleView{}}
	if h.unanswered == nil {
		c.JSON(http.StatusOK, resp)
		return
	}
	workspace, err := h.unanswered.GetWorkspace(ctx, workspaceID)
	if err != nil {
		h.failInboxUnanswered(c, requestCtx, "failed to read workspace for unanswered questions", err)
		return
	}
	resp.RetentionDays = taskmodels.ClampClarificationUnansweredRetentionDays(workspace.ClarificationUnansweredRetentionDays)
	if !workspace.ClarificationUnansweredInboxEnabled {
		c.JSON(http.StatusOK, resp)
		return
	}
	resp.Enabled = true

	page, err := h.unanswered.ListUnansweredClarificationBundles(ctx, taskmodels.ListUnansweredClarificationsOptions{
		WorkspaceIDs:    []string{workspaceID},
		CursorCreatedAt: cursorCreatedAt,
		CursorPendingID: cursorPendingID,
		Limit:           limit,
		Sidecar:         &taskmodels.ClarificationSidecarFilter{UserID: InboxUserID(ctx), Now: h.now()},
		Now:             h.now(),
	})
	if err != nil {
		h.failInboxUnanswered(c, requestCtx, "failed to list unanswered questions", err)
		return
	}
	views, err := h.buildInboxUnansweredViews(ctx, page.Bundles)
	if err != nil {
		h.failInboxUnanswered(c, requestCtx, "failed to hydrate unanswered questions", err)
		return
	}
	resp.Bundles = views
	resp.Count = len(views)
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeInboxCursor(last.CreatedAt, last.PendingID)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) failInboxUnanswered(c *gin.Context, requestCtx context.Context, message string, err error) {
	if h.respondInboxReadError(c, requestCtx, err) {
		return
	}
	h.logger.Error(message, zap.Error(err))
	respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
}

func (h *Handlers) buildInboxUnansweredViews(
	ctx context.Context, bundles []taskmodels.ClarificationBundleSummary,
) ([]inboxUnansweredBundleView, error) {
	base, err := h.buildInboxBundleViews(ctx, bundles)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(base))
	for _, view := range base {
		taskIDs = append(taskIDs, view.TaskID)
	}
	primaries := h.batchInboxPrimarySessions(ctx, taskIDs)
	views := make([]inboxUnansweredBundleView, 0, len(base))
	for _, view := range base {
		entry := inboxUnansweredBundleView{
			inboxBundleView:         view,
			OriginalSessionTerminal: isTerminalSessionState(taskmodels.TaskSessionState(view.SessionState)),
		}
		if primary := primaries[view.TaskID]; entry.OriginalSessionTerminal && primary != "" && primary != view.SessionID {
			entry.SendToTaskSessionID = &primary
		}
		views = append(views, entry)
	}
	return views, nil
}

// batchInboxPrimarySessions resolves each task's primary session. A lookup
// failure omits the send-to-task alternative rather than guessing a target.
func (h *Handlers) batchInboxPrimarySessions(ctx context.Context, taskIDs []string) map[string]string {
	primaries := make(map[string]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return primaries
	}
	sessionsByTaskID, err := h.inboxTasks.BatchGetSessionsForTasks(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to batch-load primary sessions for unanswered questions", zap.Error(err))
		return primaries
	}
	for taskID, sessions := range sessionsByTaskID {
		for _, session := range sessions {
			if session != nil && session.IsPrimary {
				primaries[taskID] = session.ID
			}
		}
	}
	return primaries
}

func isTerminalSessionState(state taskmodels.TaskSessionState) bool {
	switch state {
	case taskmodels.TaskSessionStateCompleted, taskmodels.TaskSessionStateFailed, taskmodels.TaskSessionStateCancelled:
		return true
	default:
		return false
	}
}

// httpCloseInboxUnanswered backs
// POST /api/v1/clarification-inbox/unanswered/:pendingID/close: the user
// closes the question without sending an answer.
func (h *Handlers) httpCloseInboxUnanswered(c *gin.Context) {
	pendingID := c.Param("pendingID")
	sessionID, taskID, ok := h.authorizeUnansweredBundle(c, pendingID)
	if !ok || !h.authorizeUnansweredAction(c, taskID, "") {
		return
	}
	h.closeInboxUnanswered(c, pendingID, taskmodels.CloseUnansweredClarification{
		Reason: taskmodels.ClarificationClosedByUser, At: h.now(), SourceSessionID: sessionID,
	})
}

// httpLateAnswerInboxUnanswered backs
// POST /api/v1/clarification-inbox/unanswered/:pendingID/late-answer: after
// the client's late-answer message was admitted, the question it answered is
// closed as answered_late. No other bundle is touched.
func (h *Handlers) httpLateAnswerInboxUnanswered(c *gin.Context) {
	pendingID := c.Param("pendingID")
	var body inboxLateAnswerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respondInboxError(c, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}
	body.MessageID = strings.TrimSpace(body.MessageID)
	body.SessionID = strings.TrimSpace(body.SessionID)
	if body.MessageID == "" || len(body.MessageID) > maxLateAnswerMessageIDLength {
		respondInboxError(c, http.StatusBadRequest, "message_id is required and must be at most 128 characters")
		return
	}
	sessionID, taskID, ok := h.authorizeUnansweredBundle(c, pendingID)
	if !ok {
		return
	}
	if body.SessionID == "" {
		body.SessionID = sessionID
	}
	if body.SessionID != sessionID && !h.validateSendToTaskTarget(c, sessionID, taskID, body.SessionID) {
		return
	}
	if !h.authorizeUnansweredAction(c, taskID, body.SessionID) {
		return
	}
	h.closeInboxUnanswered(c, pendingID, taskmodels.CloseUnansweredClarification{
		Reason: taskmodels.ClarificationClosedAnsweredLate, At: h.now(),
		MessageID: body.MessageID, SessionID: body.SessionID, SourceSessionID: sessionID,
	})
}

func (h *Handlers) authorizeUnansweredAction(c *gin.Context, taskID, sessionID string) bool {
	authorizer, ok := h.inboxTasks.(unansweredActionAuthorizer)
	if !ok {
		respondInboxError(c, http.StatusServiceUnavailable, errUnansweredNotAvailable)
		return false
	}
	var err error
	if sessionID == "" {
		err = authorizer.AuthorizeTaskScope(c.Request.Context(), taskID, authz.ScopeTaskWrite)
	} else {
		err = authorizer.AuthorizeTaskSessionPromptAccess(c.Request.Context(), taskID, sessionID)
	}
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		respondInboxError(c, http.StatusNotFound, errClarificationRequestNotFound)
	case errors.Is(err, service.ErrForbidden):
		respondInboxError(c, http.StatusForbidden, "insufficient permissions")
	default:
		h.logger.Error("failed to authorize unanswered question action", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errClarificationInternal)
	}
	return false
}

// validateSendToTaskTarget admits a late answer delivered to a session other
// than the asking one only when the asking session has ended and the target
// belongs to the same task.
func (h *Handlers) validateSendToTaskTarget(c *gin.Context, originalSessionID, taskID, targetSessionID string) bool {
	ctx := c.Request.Context()
	original, err := h.repo.GetTaskSession(ctx, originalSessionID)
	if err != nil || original == nil {
		respondInboxError(c, http.StatusConflict, errUnansweredNotAvailable)
		return false
	}
	if !isTerminalSessionState(original.State) {
		respondInboxError(c, http.StatusBadRequest, "a late answer targets the asking session while it has not ended")
		return false
	}
	target, err := h.repo.GetTaskSession(ctx, targetSessionID)
	if err != nil || target == nil || target.TaskID != taskID {
		respondInboxError(c, http.StatusBadRequest, "session_id must name a session of the question's task")
		return false
	}
	return true
}

// authorizeUnansweredBundle runs the same bundle authorization as the
// sidecar and respond endpoints and returns the bundle's session and task.
func (h *Handlers) authorizeUnansweredBundle(c *gin.Context, pendingID string) (sessionID, taskID string, ok bool) {
	sessionID, taskID, err := h.resolver.AuthorizeBundleAccess(c.Request.Context(), pendingID)
	if err != nil {
		if errors.Is(err, ErrBundleNotFound) {
			respondInboxError(c, http.StatusNotFound, errClarificationRequestNotFound)
			return "", "", false
		}
		h.logger.Error("failed to authorize unanswered question access",
			zap.String("pending_id", pendingID), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errClarificationInternal)
		return "", "", false
	}
	if h.unanswered == nil {
		respondInboxError(c, http.StatusConflict, errUnansweredNotAvailable)
		return "", "", false
	}
	return sessionID, taskID, true
}

func (h *Handlers) closeInboxUnanswered(c *gin.Context, pendingID string, closeReq taskmodels.CloseUnansweredClarification) {
	closed, err := h.unanswered.CloseUnansweredClarificationBundle(c.Request.Context(), pendingID, closeReq)
	if err != nil {
		if errors.Is(err, taskmodels.ErrUnansweredAdmissionUnknown) {
			c.JSON(http.StatusConflict, inboxConflictBody{Error: "late answer admission could not be confirmed", Code: "late_answer_not_admitted"})
			return
		}
		h.logger.Error(errUnansweredWrite, zap.String("pending_id", pendingID), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errUnansweredWrite)
		return
	}
	if len(closed) == 0 {
		c.JSON(http.StatusConflict, inboxConflictBody{Error: errUnansweredNotAvailable, Code: unansweredConflictCode})
		return
	}
	c.Status(http.StatusNoContent)
}
