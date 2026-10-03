package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

type ConfigChatSessionRetirer interface {
	RetireConfigChatSession(context.Context, string, string, func(context.Context) error) error
}

const (
	configChatRestartStageValidate = "validate"
	configChatRestartStageStop     = "stop"
	configChatRestartStageDelete   = "delete"
	configChatRestartStageCreate   = "create"
	configChatRestartStageStart    = "start"
)

type configChatRestartFailure struct {
	Code        string                      `json:"code"`
	Stage       string                      `json:"stage"`
	OldDeleted  bool                        `json:"old_deleted"`
	Replacement *httpStartQuickChatResponse `json:"replacement,omitempty"`
}

func (h *TaskHandlers) httpRestartConfigChat(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var body struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.TaskID == "" || body.SessionID == "" {
		c.JSON(http.StatusBadRequest, configChatRestartFailure{Code: "config_chat_restart_invalid_target", Stage: configChatRestartStageValidate})
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), constants.TaskDeleteTimeout+constants.AgentLaunchTimeout)
	defer cancel()
	workspaceID := c.Param("id")
	if err := h.service.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		handleNotFound(c, h.logger, err, "configuration chat not found")
		return
	}
	release, admitted := h.configChatAdmission.begin(workspaceID, body.SessionID)
	if !admitted {
		c.JSON(http.StatusConflict, configChatRestartFailure{Code: "config_chat_restart_busy", Stage: configChatRestartStageValidate})
		return
	}
	defer release()
	selection, err := h.service.ValidateConfigChatRestart(ctx, workspaceID, body.TaskID, body.SessionID)
	if err != nil {
		var invalid *service.ConfigChatRestartValidationError
		if errors.As(err, &invalid) {
			c.JSON(http.StatusConflict, configChatRestartFailure{Code: invalid.Code, Stage: configChatRestartStageValidate})
		} else {
			handleNotFound(c, h.logger, err, "configuration chat not found")
		}
		return
	}
	if h.configChatRetirer == nil {
		c.JSON(http.StatusServiceUnavailable, configChatRestartFailure{Code: "config_chat_restart_stop_failed", Stage: configChatRestartStageStop})
		return
	}
	failure := configChatRestartFailure{Stage: configChatRestartStageValidate}
	err = h.service.WithTaskDeleteConfirmation(ctx, c.GetHeader(taskDeleteConfirmationHeader), body.TaskID, false, false, func() error {
		failure.Stage = configChatRestartStageStop
		return h.configChatRetirer.RetireConfigChatSession(ctx, body.TaskID, body.SessionID, func(deleteCtx context.Context) error {
			failure.Stage = configChatRestartStageDelete
			if deleteErr := h.service.DeleteTaskWithLifecycle(deleteCtx, body.TaskID); deleteErr != nil {
				return deleteErr
			}
			failure.OldDeleted = true
			return nil
		})
	})
	if err != nil {
		h.respondConfigChatRestartFailure(c, failure, err)
		return
	}
	failure.Stage = configChatRestartStageCreate
	replacement, err := h.createConfigChatReplacement(ctx, workspaceID, selection)
	failure.Replacement = replacement
	if err != nil {
		h.respondConfigChatRestartFailure(c, failure, err)
		return
	}
	failure.Stage = configChatRestartStageStart
	_, err = h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID: replacement.TaskID, SessionID: replacement.SessionID,
		AgentProfileID: selection.AgentProfileID, Intent: orchestrator.IntentStartCreated,
		ActivationSource: orchestrator.LaunchActivationSourceUserAction,
		NoInitialPrompt:  true, SkipMessageRecord: true,
	})
	if err != nil {
		h.respondConfigChatRestartFailure(c, failure, err)
		return
	}
	c.JSON(http.StatusOK, replacement)
}

func (h *TaskHandlers) createConfigChatReplacement(ctx context.Context, workspaceID string, selection *service.ConfigChatRestartSelection) (*httpStartQuickChatResponse, error) {
	result, err := h.service.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaceID, Title: "Config Chat", IsEphemeral: true,
		Metadata: map[string]interface{}{"config_mode": true, models.MetaKeyAgentProfileID: selection.AgentProfileID,
			models.MetaKeyExecutorID: selection.ExecutorID, models.MetaKeyExecutorProfileID: selection.ExecutorProfileID},
	})
	if err != nil {
		return nil, err
	}
	response, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID: result.Task.ID, Intent: orchestrator.IntentPrepare, AgentProfileID: selection.AgentProfileID,
		ExecutorID: selection.ExecutorID, ExecutorProfileID: selection.ExecutorProfileID, DeferredStart: true,
	})
	if err != nil {
		retained, lookupErr := h.preparedConfigChatReplacement(ctx, result.Task.ID, selection.AgentProfileID)
		if retained != nil {
			return retained, err
		}
		return nil, errors.Join(err, lookupErr, h.service.DeleteTaskWithLifecycle(ctx, result.Task.ID))
	}
	return &httpStartQuickChatResponse{TaskID: result.Task.ID, SessionID: response.SessionID, AgentProfileID: selection.AgentProfileID}, nil
}

func (h *TaskHandlers) preparedConfigChatReplacement(ctx context.Context, taskID, agentProfileID string) (*httpStartQuickChatResponse, error) {
	sessions, err := h.service.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session != nil && session.IsPrimary && session.TaskID == taskID {
			return &httpStartQuickChatResponse{TaskID: taskID, SessionID: session.ID, AgentProfileID: agentProfileID}, nil
		}
	}
	return nil, nil
}

func (h *TaskHandlers) respondConfigChatRestartFailure(c *gin.Context, failure configChatRestartFailure, err error) {
	status := http.StatusInternalServerError
	failure.Code = "config_chat_restart_" + failure.Stage + "_failed"
	if failure.Stage == configChatRestartStageValidate {
		failure.Code = "config_chat_restart_confirmation_invalid"
		status = http.StatusConflict
		if errors.Is(err, service.ErrTaskDeleteConfirmationRequired) {
			status = http.StatusPreconditionRequired
			failure.Code = "config_chat_restart_confirmation_required"
		}
		if errors.Is(err, service.ErrTaskDeleteConfirmationIdentity) {
			status = http.StatusUnauthorized
		}
	}
	h.logger.Warn("configuration chat restart failed", zap.String("stage", failure.Stage), zap.Bool("old_deleted", failure.OldDeleted), zap.Error(err))
	c.JSON(status, failure)
}
