package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"strconv"
	"time"
)

const controlPause = "pause"

type assistantControlRequest struct {
	models.OperationRequest
	Action                 string `json:"action"`
	ExpectedBindingVersion int64  `json:"expected_binding_version"`
	After                  string `json:"after"`
}
type assistantStopReceipt struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

func (h *Handler) assistantControl(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req assistantControlRequest
	if c.ShouldBindJSON(&req) != nil {
		c.AbortWithStatus(400)
		return
	}
	if req.Action != controlPause && req.Action != "resume" && req.Action != "stop_managed_work" {
		c.AbortWithStatus(422)
		return
	}
	claims := &runtimeauth.AgentClaims{TaskID: b.ConversationID, WorkspaceID: b.WorkspaceID, RunID: "human:" + b.OwnerUserID}
	h.performOperation(c, claims, req.OperationRequest, req, 200, func() (any, error) {
		current, err := h.Service.Repo.AssistantBindingByID(c.Request.Context(), b.ID)
		if err != nil || current.Version != req.ExpectedBindingVersion {
			return nil, rejectOperation(409, "assistant_binding_superseded")
		}
		if req.Action == "stop_managed_work" {
			return h.Service.stopManagedWork(c.Request.Context(), current, req)
		}
		if err = h.Service.Repo.SetAssistantPaused(c.Request.Context(), current, req.Action == controlPause); err != nil {
			return nil, rejectOperation(409, "assistant_binding_superseded")
		}
		return gin.H{"paused": req.Action == controlPause, "binding_version": current.Version}, nil
	})
}
func (s *Service) stopManagedWork(ctx context.Context, b *models.AssistantBinding, req assistantControlRequest) (any, error) {
	if s.Inputs == nil {
		return nil, rejectOperation(503, "native_control_unavailable")
	}
	scope := cursorScope("assistant-stop-v1", b.OwnerUserID, b.ID, strconv.FormatInt(b.Version, 10))
	after, err := decodeScopedCursor(req.After, scope)
	if err != nil {
		return nil, rejectOperation(400, "invalid_stop_cursor")
	}
	tasks, err := s.Repo.AssistantManagedTasks(ctx, b.ID, after, 26)
	if err != nil {
		return nil, err
	}
	if err = s.Repo.AdvanceAssistantControlIntent(ctx, b, *req.ExpectedIntentRevision); err != nil {
		return nil, rejectOperation(409, "intent_superseded")
	}
	next := ""
	if len(tasks) > 25 {
		tasks = tasks[:25]
		next = encodeScopedCursor(scope, tasks[24])
	}
	revision := *req.ExpectedIntentRevision + 1
	receipts := []assistantStopReceipt{}
	partial := false
	for _, task := range tasks {
		sessions, err := s.managedControlSessions(ctx, b, task)
		if err != nil {
			partial = true
			receipts = append(receipts, assistantStopReceipt{TaskID: task, Status: statusUnknown})
			continue
		}
		for _, session := range sessions {
			if session == nil || session.TaskID != task {
				continue
			}
			result := s.stopManagedSession(ctx, b, req.OperationID, revision, session)
			receipts = append(receipts, result)
			partial = partial || result.Status == statusUnknown || result.Status == statusFailed
		}
	}
	return gin.H{"sessions": receipts, "partial": partial, nextCursorKey: next, intentRevisionKey: revision}, nil
}
func (s *Service) managedControlSessions(ctx context.Context, b *models.AssistantBinding, task string) ([]*taskmodels.TaskSession, error) {
	if !s.attentionVisible(ctx, b, models.Attention{TaskID: task}) {
		return nil, fmt.Errorf("managed task unavailable")
	}
	return s.Tasks.ListTaskSessions(ctx, task)
}
func (s *Service) stopManagedSession(ctx context.Context, b *models.AssistantBinding, parent string, revision int64, session *taskmodels.TaskSession) assistantStopReceipt {
	result := assistantStopReceipt{TaskID: session.TaskID, SessionID: session.ID, Status: "already_finished"}
	switch session.State {
	case taskmodels.TaskSessionStateCreated, taskmodels.TaskSessionStateStarting, taskmodels.TaskSessionStateRunning, taskmodels.TaskSessionStateWaitingForInput:
	default:
		return result
	}
	if !s.canStopManagedSession(ctx, b, session.TaskID) {
		result.Status = statusFailed
		return result
	}
	identity := fmt.Sprintf("%x", sha256.Sum256([]byte(parent+":"+session.TaskID+":"+session.ID)))
	operation, created, err := s.Repo.BeginOperation(ctx, models.Operation{BindingID: b.ID, BindingVersion: b.Version, ConversationID: b.ConversationID, RunID: "human:" + b.OwnerUserID, OperationID: "stop:" + identity, Target: "stop-session:" + session.ID, RequestHash: identity, IntentRevision: revision})
	if err != nil {
		result.Status = statusUnknown
		return result
	}
	if !created {
		if operation.State == statusAcknowledged && json.Unmarshal([]byte(operation.ResponseJSON), &result) == nil {
			return result
		}
		result.Status = statusUnknown
		return result
	}
	if err = s.Repo.DispatchOperation(ctx, operation); err != nil {
		result.Status = statusFailed
		return s.saveStopReceipt(ctx, operation.ID, result)
	}
	if !s.canStopManagedSession(ctx, b, session.TaskID) {
		result.Status = statusFailed
		return s.saveStopReceipt(ctx, operation.ID, result)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	scoped, stopCtx, err := s.managedStopWorkspace(stopCtx, b, session.TaskID)
	if err != nil {
		cancel()
		result.Status = statusFailed
		return s.saveStopReceipt(ctx, operation.ID, result)
	}
	status, err := s.Inputs.StopSession(stopCtx, scoped, session.TaskID, session.ID)
	cancel()
	result.Status = status
	if err != nil || (status != "stopped" && status != "already_finished" && status != statusFailed) {
		result.Status = statusUnknown
	}
	return s.saveStopReceipt(ctx, operation.ID, result)
}
func (s *Service) saveStopReceipt(ctx context.Context, id string, result assistantStopReceipt) assistantStopReceipt {
	state := statusAcknowledged
	if result.Status == statusUnknown {
		state = statusUnknown
	}
	raw, _ := json.Marshal(result)
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.Repo.FinishOperation(persistCtx, id, state, string(raw), 200); err != nil {
		result.Status = statusUnknown
	}
	return result
}

func (s *Service) canStopManagedSession(ctx context.Context, b *models.AssistantBinding, taskID string) bool {
	current, err := s.Repo.AssistantBindingByID(ctx, b.ID)
	return err == nil && current.Version == b.Version && s.attentionVisible(ctx, current, models.Attention{TaskID: taskID})
}
