package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type queueRecoveryRequest struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	Cursor           string `json:"cursor"`
	MaxBodyBytes     int    `json:"max_body_bytes"`
	ExpectedSnapshot string `json:"expected_snapshot_sha256"`
}

type queueRecoveryAuthority struct {
	WorkspaceID       string
	CallerTaskID      string
	CallerSessionID   string
	CallerIncarnation string
	TargetTaskID      string
	TargetIdentity    messagequeue.QueueSessionIdentity
	TargetState       models.TaskSessionState
}

var errQueueRecoveryDenied = errors.New("queue recovery access denied")
var errQueueRecoveryNotTerminal = errors.New("queue recovery target is not terminal")

func (h *Handlers) registerQueueRecoveryHandlers(d mcpActionRegistrar) {
	d.RegisterFunc(ws.ActionMCPGetTerminalSessionMessageQueue, h.handleQueueRecoveryPage)
	d.RegisterFunc(ws.ActionMCPGetChildSessionMessageQueue, h.handleQueueRecoveryPage)
	d.RegisterFunc(ws.ActionMCPGetChildQueueInventory, h.handleChildQueueInventory)
}

func validQueueRecoveryTask(task *models.Task, taskID, workspaceID string) bool {
	return task != nil && task.ID == taskID && workspaceID != "" && task.WorkspaceID == workspaceID && task.ArchivedAt == nil
}

func validQueueRecoverySession(session *models.TaskSession, taskID, sessionID string) bool {
	return session != nil && session.ID == sessionID && session.TaskID == taskID && session.QueueIncarnationID != ""
}

func (h *Handlers) queueRecoveryCaller(ctx context.Context) (queueRecoveryAuthority, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || h.taskSvc == nil || h.sessionRepo == nil {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	if principal.Surface != mcpprofile.SurfaceKanbanTask && principal.Surface != mcpprofile.SurfaceOfficeTask {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	caller, err := h.taskSvc.GetTask(ctx, principal.CallerTaskID)
	if err != nil || !validQueueRecoveryTask(caller, principal.CallerTaskID, principal.WorkspaceID) {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	workspace, err := h.taskSvc.GetWorkspace(ctx, caller.WorkspaceID)
	if err != nil || workspace == nil {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	if workspace.ID != caller.WorkspaceID {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, principal.CallerSessionID)
	if err != nil || !validQueueRecoverySession(session, caller.ID, principal.CallerSessionID) {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	return queueRecoveryAuthority{WorkspaceID: caller.WorkspaceID, CallerTaskID: caller.ID, CallerSessionID: session.ID, CallerIncarnation: session.QueueIncarnationID}, nil
}

func (h *Handlers) queueRecoveryAuthority(ctx context.Context, request queueRecoveryRequest, child bool) (queueRecoveryAuthority, error) {
	authority, err := h.queueRecoveryCaller(ctx)
	if err != nil {
		return queueRecoveryAuthority{}, err
	}
	target, err := h.taskSvc.GetTask(ctx, request.TaskID)
	if err != nil || !validQueueRecoveryTask(target, request.TaskID, authority.WorkspaceID) {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	if child && target.ParentID != authority.CallerTaskID || !child && target.ID != authority.CallerTaskID {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	authority.TargetTaskID = target.ID
	if request.SessionID == "" {
		if !child {
			return queueRecoveryAuthority{}, errQueueRecoveryDenied
		}
		return authority, nil
	}
	return h.queueRecoveryTargetSession(ctx, request.SessionID, authority, child)
}

func (h *Handlers) queueRecoveryTargetSession(ctx context.Context, sessionID string, authority queueRecoveryAuthority, child bool) (queueRecoveryAuthority, error) {
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil || !validQueueRecoverySession(session, authority.TargetTaskID, sessionID) {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	if !child && session.ID == authority.CallerSessionID {
		return queueRecoveryAuthority{}, errQueueRecoveryDenied
	}
	if !child && !isTerminalMessageTargetState(session.State) {
		return queueRecoveryAuthority{}, errQueueRecoveryNotTerminal
	}
	authority.TargetIdentity = messagequeue.QueueSessionIdentity{TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID}
	authority.TargetState = session.State
	return authority, nil
}

func queueRecoveryError(msg *ws.Message, err error) (*ws.Message, error) {
	code, text := ws.ErrorCodeInternalError, "queue recovery unavailable"
	switch {
	case errors.Is(err, errQueueRecoveryDenied), errors.Is(err, messagequeue.ErrSessionIdentityMismatch):
		code, text = ws.ErrorCodeForbidden, "queue recovery access denied"
	case errors.Is(err, messagequeue.ErrInvalidRecoveryPage):
		code, text = ws.ErrorCodeValidation, "invalid queue recovery page"
	case errors.Is(err, messagequeue.ErrRecoveryChanged):
		code, text = ws.ErrorCodeConflict, "queue changed; restart from first page"
	case errors.Is(err, errQueueRecoveryNotTerminal):
		code, text = ws.ErrorCodeConflict, "target session is not terminal"
	case errors.Is(err, messagequeue.ErrRecoveryLimit):
		code, text = ws.ErrorCodeConflict, "queue recovery limit exceeded"
	}
	return ws.NewError(msg.ID, msg.Action, code, text, nil)
}

func (h *Handlers) handleQueueRecoveryPage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var request queueRecoveryRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		return queueRecoveryError(msg, messagequeue.ErrInvalidRecoveryPage)
	}
	request.TaskID, request.SessionID = strings.TrimSpace(request.TaskID), strings.TrimSpace(request.SessionID)
	if request.SessionID == "" {
		return queueRecoveryError(msg, errQueueRecoveryDenied)
	}
	child := msg.Action == ws.ActionMCPGetChildSessionMessageQueue
	authority, err := h.queueRecoveryAuthority(ctx, request, child)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	if h.sessionLauncher == nil || h.sessionLauncher.GetMessageQueue() == nil {
		return queueRecoveryError(msg, errors.New("queue unavailable"))
	}
	page, err := h.sessionLauncher.GetMessageQueue().RecoverySnapshotPage(ctx, authority.TargetIdentity, authority.WorkspaceID, request.Cursor, request.MaxBodyBytes, request.ExpectedSnapshot)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	after, err := h.queueRecoveryAuthority(ctx, request, child)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	if after != authority {
		return queueRecoveryError(msg, messagequeue.ErrRecoveryChanged)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"task_id": request.TaskID, "session_id": request.SessionID, "page": page})
}

type queueRecoverySessionReader interface {
	ListQueueRecoverySessions(context.Context, string) ([]*models.TaskSession, error)
}

func (h *Handlers) handleChildQueueInventory(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var request queueRecoveryRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		return queueRecoveryError(msg, messagequeue.ErrInvalidRecoveryPage)
	}
	request.TaskID, request.SessionID = strings.TrimSpace(request.TaskID), ""
	authority, err := h.queueRecoveryAuthority(ctx, request, true)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	reader, ok := h.sessionRepo.(queueRecoverySessionReader)
	if !ok || h.sessionLauncher == nil || h.sessionLauncher.GetMessageQueue() == nil {
		return queueRecoveryError(msg, errors.New("inventory unavailable"))
	}
	sessions, err := reader.ListQueueRecoverySessions(ctx, request.TaskID)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	identities, err := queueRecoverySessionIdentities(sessions, request.TaskID)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	manifests, err := h.sessionLauncher.GetMessageQueue().RecoveryInventory(ctx, identities, authority.WorkspaceID)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	after, err := h.queueRecoveryAuthority(ctx, request, true)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	current, err := reader.ListQueueRecoverySessions(ctx, request.TaskID)
	if err != nil {
		return queueRecoveryError(msg, err)
	}
	if after != authority || !reflect.DeepEqual(sessions, current) {
		return queueRecoveryError(msg, messagequeue.ErrRecoveryChanged)
	}
	result := make([]map[string]interface{}, 0, len(sessions))
	for i, session := range sessions {
		result = append(result, map[string]interface{}{"session_id": session.ID, "state": session.State, "manifest": manifests[i]})
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"task_id": request.TaskID, "sessions": result, "total_sessions": len(sessions)})
}

func queueRecoverySessionIdentities(sessions []*models.TaskSession, taskID string) ([]messagequeue.QueueSessionIdentity, error) {
	if len(sessions) > 256 {
		return nil, messagequeue.ErrRecoveryLimit
	}
	result := make([]messagequeue.QueueSessionIdentity, 0, len(sessions))
	seen := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		if session == nil || session.ID == "" || session.TaskID != taskID || session.QueueIncarnationID == "" || seen[session.ID] {
			return nil, errQueueRecoveryDenied
		}
		seen[session.ID] = true
		result = append(result, messagequeue.QueueSessionIdentity{TaskID: taskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID})
	}
	return result, nil
}
