package handlers

import (
	"context"
	"database/sql"
	"errors"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	fieldCallerTaskID    = "caller_task_id"
	fieldCallerSessionID = "caller_session_id"
	fieldSourceSessionID = "source_session_id"
	fieldSourceTaskID    = "source_task_id"
	fieldSenderTaskID    = "sender_task_id"
	fieldWorkspaceID     = "workspace_id"
	fieldWorkflowID      = "workflow_id"
)

const (
	reasonGuardedScope = "guarded_session_scope"
	reasonLookupFailed = "guard_lookup_failed"
)

// guardedRefFields are the payload keys the guard matches the way the
// handlers decode them. Every key that folds to one of these names is that
// field.
var guardedRefFields = []string{
	keyTaskID, keySessionID, fieldWorkflowID, "parent_id", "depends_on_task_id", "blocked_by", fieldWorkspaceID, "parent_is_self",
	fieldSourceTaskID, fieldSourceSessionID, fieldSenderTaskID, senderSessionIDKey, fieldCallerTaskID, fieldCallerSessionID,
}

// guardedRefError is a refusal produced while checking one request.
type guardedRefError struct {
	code        string
	reason      string
	message     string
	requirement string
}

func (e *guardedRefError) Error() string { return e.reason }

func scopeRefusal() *guardedRefError {
	return &guardedRefError{code: ws.ErrorCodeForbidden, reason: reasonGuardedScope, message: "target is outside the scope of this guarded session"}
}

func lookupRefusal() *guardedRefError {
	return &guardedRefError{code: ws.ErrorCodeUnavailable, reason: reasonLookupFailed, message: "scope lookup is temporarily unavailable"}
}

func (e *guardedRefError) response(msg *ws.Message) (*ws.Message, error) {
	details := map[string]interface{}{mcpReasonKey: e.reason}
	if e.requirement != "" {
		details["requirement"] = e.requirement
	}
	return ws.NewError(msg.ID, msg.Action, e.code, e.message, details)
}

// authorizeGuardedRequest applies the guarded-session surface to a task
// session principal that holds a task guard.
// Other principals (unguarded, coordinator, automation) pass unchanged.
func (h *Handlers) authorizeGuardedRequest(ctx context.Context, msg *ws.Message) (*ws.Message, *ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsGuarded() {
		return nil, msg, nil
	}
	tool, listed := mcpprofile.GuardedToolByAction(msg.Action)
	if !listed || tool.Class == mcpprofile.ClassDenied {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction,
			"tool is not available to a guarded session", nil)
		return response, nil, err
	}
	if tool.Class == mcpprofile.ClassInternal {
		if !mcporigin.IsTrustedInternalCall(ctx) {
			response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction,
				"tool is not available to a guarded session", nil)
			return response, nil, err
		}
		return nil, msg, nil
	}
	refusal, replacement := h.checkGuardedCall(ctx, principal, tool, msg)
	if refusal != nil {
		response, err := refusal.response(msg)
		return response, nil, err
	}
	return nil, replacement, nil
}

func (h *Handlers) checkGuardedCall(
	ctx context.Context,
	principal mcpscope.Principal,
	tool mcpprofile.GuardedTool,
	msg *ws.Message,
) (*guardedRefError, *ws.Message) {
	payload, err := parseGuardedPayload(msg.Payload)
	if err != nil || !payload.canonicalize(guardedRefFields) {
		return scopeRefusal(), nil
	}
	rewriteGuardedAttribution(payload, principal, tool)
	if tool.Class == mcpprofile.ClassRead {
		if refusal := h.checkGuardedReadScope(ctx, principal, payload); refusal != nil {
			return refusal, nil
		}
		if tool.PinWorkspace {
			payload.set(fieldWorkspaceID, principal.WorkspaceID)
		}
	} else if refusal := h.checkGuardedScope(ctx, principal, tool, msg.Action, payload); refusal != nil {
		return refusal, nil
	}
	replacement := *msg
	replacement.Payload = payload.marshal()
	return nil, &replacement
}

func rewriteGuardedAttribution(payload *guardedPayload, principal mcpscope.Principal, tool mcpprofile.GuardedTool) {
	for _, field := range tool.Attribution {
		if field == senderSessionIDKey || field == fieldCallerSessionID || field == fieldSourceSessionID {
			payload.set(field, principal.CallerSessionID)
			continue
		}
		payload.set(field, principal.CallerTaskID)
	}
}

func (h *Handlers) checkGuardedScope(
	ctx context.Context,
	principal mcpscope.Principal,
	tool mcpprofile.GuardedTool,
	action string,
	payload *guardedPayload,
) *guardedRefError {
	if h.taskSvc == nil {
		return lookupRefusal()
	}
	if action == "mcp.create_task" {
		if refusal := h.normalizeGuardedCreateTask(principal, payload); refusal != nil {
			return refusal
		}
	}
	refs, refusal := guardedReferences(ctx, h, payload)
	if refusal != nil {
		return refusal
	}
	if refusal := h.checkGuardedRefs(ctx, principal, tool, refs); refusal != nil {
		return refusal
	}
	return nil
}

// checkGuardedReadScope confines a read to the caller's workspace: every
// referenced task, session, workflow and workspace must belong to it.
func (h *Handlers) checkGuardedReadScope(
	ctx context.Context,
	principal mcpscope.Principal,
	payload *guardedPayload,
) *guardedRefError {
	workspace, _, typed := payload.stringField(fieldWorkspaceID)
	if !typed || (workspace != "" && workspace != principal.WorkspaceID) {
		return scopeRefusal()
	}
	taskID, _, taskTyped := payload.stringField(keyTaskID)
	sessionID, _, sessionTyped := payload.stringField(keySessionID)
	workflowID, _, workflowTyped := payload.stringField(fieldWorkflowID)
	if !taskTyped || !sessionTyped || !workflowTyped {
		return scopeRefusal()
	}
	if taskID == "" && sessionID == "" && workflowID == "" {
		return nil
	}
	if h.taskSvc == nil {
		return lookupRefusal()
	}
	if taskID != "" {
		if refusal := h.checkTaskWorkspace(ctx, principal, taskID); refusal != nil {
			return refusal
		}
	}
	if refusal := h.checkSessionWorkspace(ctx, principal, sessionID); refusal != nil {
		return refusal
	}
	return h.checkWorkflowWorkspace(ctx, principal, workflowID)
}

func (h *Handlers) checkSessionWorkspace(ctx context.Context, principal mcpscope.Principal, sessionID string) *guardedRefError {
	if sessionID == "" {
		return nil
	}
	owner, refusal := h.taskOfSession(ctx, sessionID)
	if refusal != nil {
		return refusal
	}
	return h.checkTaskWorkspace(ctx, principal, owner)
}

func (h *Handlers) checkWorkflowWorkspace(ctx context.Context, principal mcpscope.Principal, workflowID string) *guardedRefError {
	if workflowID == "" {
		return nil
	}
	workflow, err := h.taskSvc.GetWorkflow(ctx, workflowID)
	if err != nil || workflow == nil {
		return refusalForLookup(err)
	}
	if workflow.WorkspaceID != principal.WorkspaceID {
		return scopeRefusal()
	}
	return nil
}

func (h *Handlers) checkTaskWorkspace(ctx context.Context, principal mcpscope.Principal, taskID string) *guardedRefError {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return refusalForLookup(err)
	}
	if task.WorkspaceID != principal.WorkspaceID {
		return scopeRefusal()
	}
	return nil
}

// normalizeGuardedCreateTask pins the workspace and the parent of a
// create_task call to the caller's. Child admission is owned by task creation.
func (h *Handlers) normalizeGuardedCreateTask(
	principal mcpscope.Principal,
	payload *guardedPayload,
) *guardedRefError {
	workspace, _, typed := payload.stringField(fieldWorkspaceID)
	if !typed || (workspace != "" && workspace != principal.WorkspaceID) {
		return scopeRefusal()
	}
	payload.set(fieldWorkspaceID, principal.WorkspaceID)
	parent, _, typed := payload.stringField("parent_id")
	if !typed {
		return scopeRefusal()
	}
	if parent != "" && parent != "self" && parent != principal.CallerTaskID {
		return scopeRefusal()
	}
	payload.set("parent_id", principal.CallerTaskID)
	payload.remove("parent_is_self")
	return nil
}

// guardedReferences lists the referenced tasks in the order task_id, the
// task of session_id, parent_id, depends_on_task_id, blocked_by.
func guardedReferences(ctx context.Context, h *Handlers, payload *guardedPayload) ([]string, *guardedRefError) {
	var ids []string
	taskID, _, typed := payload.stringField(keyTaskID)
	if !typed {
		return nil, scopeRefusal()
	}
	if taskID != "" {
		ids = append(ids, taskID)
	}
	session, _, typed := payload.stringField(keySessionID)
	if !typed {
		return nil, scopeRefusal()
	}
	if session != "" {
		id, refusal := h.taskOfSession(ctx, session)
		if refusal != nil {
			return nil, refusal
		}
		ids = append(ids, id)
	}
	for _, field := range []string{"parent_id", "depends_on_task_id"} {
		value, _, typed := payload.stringField(field)
		if !typed {
			return nil, scopeRefusal()
		}
		if value != "" {
			ids = append(ids, value)
		}
	}
	blocked, typed := payload.stringListField("blocked_by")
	if !typed {
		return nil, scopeRefusal()
	}
	for _, id := range blocked {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (h *Handlers) taskOfSession(ctx context.Context, sessionID string) (string, *guardedRefError) {
	session, err := h.taskSvc.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return "", refusalForLookup(err)
	}
	if session.TaskID == "" {
		return "", scopeRefusal()
	}
	return session.TaskID, nil
}

// isLookupMiss reports an error that means the target does not exist or is
// not visible, as opposed to a repository failure.
func isLookupMiss(err error) bool {
	return err == nil ||
		errors.Is(err, sql.ErrNoRows) ||
		errors.Is(err, repoerrors.ErrTaskNotFound) ||
		errors.Is(err, repoerrors.ErrWorkspaceNotFound) ||
		errors.Is(err, repoerrors.ErrWorkflowNotFound) ||
		errors.Is(err, models.ErrTaskSessionNotFound) ||
		service.IsForbidden(err)
}

func refusalForLookup(err error) *guardedRefError {
	if isLookupMiss(err) {
		return scopeRefusal()
	}
	return lookupRefusal()
}

func (h *Handlers) checkGuardedRefs(
	ctx context.Context,
	principal mcpscope.Principal,
	tool mcpprofile.GuardedTool,
	ids []string,
) *guardedRefError {
	checked := map[string]bool{}
	for _, id := range ids {
		if id == principal.CallerTaskID || checked[id] {
			continue
		}
		if tool.Class == mcpprofile.ClassSelf {
			return scopeRefusal()
		}
		if refusal := h.checkDirectChild(ctx, principal, id); refusal != nil {
			return refusal
		}
		checked[id] = true
	}
	return nil
}

// checkDirectChild accepts a direct child of the caller in the caller's
// workspace. Every other task, missing or foreign, is refused alike.
func (h *Handlers) checkDirectChild(
	ctx context.Context,
	principal mcpscope.Principal,
	taskID string,
) *guardedRefError {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return refusalForLookup(err)
	}
	if task.WorkspaceID != principal.WorkspaceID || task.ParentID != principal.CallerTaskID {
		return scopeRefusal()
	}
	return nil
}
