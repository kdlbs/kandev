package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type httpBackgroundWorkloadListResponse struct {
	Workloads []streams.WorkloadRunObservation `json:"workloads"`
}

type httpBackgroundWorkloadDetailResponse struct {
	Workload streams.WorkloadRunObservation `json:"workload"`
}

func toWorkloadObservation(w *models.BackgroundWorkload) streams.WorkloadRunObservation {
	if w == nil {
		return streams.WorkloadRunObservation{}
	}
	parentID := ""
	if w.ParentWorkID != nil {
		parentID = *w.ParentWorkID
	}
	state := streams.RunState(w.State)
	if state == "" {
		state = streams.RunStateUnknown
	}
	caps := streams.WorkloadCapabilities{
		Discovery:         "events_only",
		Output:            "snapshot",
		Transcript:        w.Kind == string(streams.WorkloadKindSubagent),
		Parentage:         parentID != "",
		ReasoningSummary:  false,
		AttributableUsage: w.Kind == string(streams.WorkloadKindSubagent),
		Actions: map[streams.WorkloadActionKind]streams.ActionCapability{
			streams.WorkloadActionStop: {
				Supported: true,
				Available: state == streams.RunStateRunning || state == streams.RunStateWaiting,
			},
			streams.WorkloadActionInterrupt: {
				Supported: w.Kind == string(streams.WorkloadKindSubagent),
				Available: (state == streams.RunStateRunning || state == streams.RunStateWaiting) && w.Kind == string(streams.WorkloadKindSubagent),
			},
			streams.WorkloadActionWriteInput: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
			streams.WorkloadActionCloseInput: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
		},
	}
	if w.Kind == string(streams.WorkloadKindShell) {
		caps.Actions[streams.WorkloadActionInterrupt] = streams.ActionCapability{
			Supported: false,
			Available: false,
			Reason:    string(streams.ActionReasonUnsupported),
		}
	}
	return streams.WorkloadRunObservation{
		SessionID:       w.SessionID,
		WorkID:          w.ID,
		Kind:            streams.NormalizeWorkloadKind(w.Kind),
		Title:           w.Title,
		State:           state,
		ParentWorkID:    parentID,
		OriginTurnID:    w.OriginTurnID,
		SourceMessageID: w.SourceMessageID,
		SourceCallID:    w.SourceCallID,
		ExitCode:        w.ExitCode,
		Capabilities:    caps,
		Output:          w.Output,
		OutputTruncated: w.OutputTruncated,
		OutputOffset:    w.OutputOffset,
		StartedAt:       w.StartedAt,
		FinishedAt:      w.FinishedAt,
		Revision:        w.Revision,
	}
}

func (h *TaskHandlers) httpListBackgroundWorkloads(c *gin.Context) {
	if !h.isBackgroundWorkEnabled() {
		c.JSON(http.StatusOK, httpBackgroundWorkloadListResponse{Workloads: []streams.WorkloadRunObservation{}})
		return
	}
	sessionID := c.Param("id")
	if strings.TrimSpace(sessionID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}
	if h.service == nil {
		c.JSON(http.StatusOK, httpBackgroundWorkloadListResponse{Workloads: []streams.WorkloadRunObservation{}})
		return
	}
	workloads, err := h.service.ListBackgroundWorkloads(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to list background workloads", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dtos := make([]streams.WorkloadRunObservation, 0, len(workloads))
	for _, w := range workloads {
		dtos = append(dtos, toWorkloadObservation(w))
	}
	c.JSON(http.StatusOK, httpBackgroundWorkloadListResponse{Workloads: dtos})
}

func (h *TaskHandlers) httpGetBackgroundWorkload(c *gin.Context) {
	if !h.isBackgroundWorkEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "background work feature is disabled"})
		return
	}
	sessionID := c.Param("id")
	workID := c.Param("workId")
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(workID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id and work id are required"})
		return
	}
	if h.service == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workload not found"})
		return
	}
	workload, err := h.service.GetBackgroundWorkload(c.Request.Context(), sessionID, workID)
	if err != nil {
		h.logger.Error("failed to get background workload", zap.String("session_id", sessionID), zap.String("work_id", workID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if workload == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workload not found"})
		return
	}
	c.JSON(http.StatusOK, httpBackgroundWorkloadDetailResponse{Workload: toWorkloadObservation(workload)})
}

func (h *TaskHandlers) wsListBackgroundWorkloads(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if !h.isBackgroundWorkEnabled() {
		return ws.NewResponse(msg.ID, msg.Action, httpBackgroundWorkloadListResponse{Workloads: []streams.WorkloadRunObservation{}})
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := msg.ParsePayload(&req); err != nil || strings.TrimSpace(req.SessionID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session_id is required", nil)
	}
	if h.service == nil {
		return ws.NewResponse(msg.ID, msg.Action, httpBackgroundWorkloadListResponse{Workloads: []streams.WorkloadRunObservation{}})
	}
	workloads, err := h.service.ListBackgroundWorkloads(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	dtos := make([]streams.WorkloadRunObservation, 0, len(workloads))
	for _, w := range workloads {
		dtos = append(dtos, toWorkloadObservation(w))
	}
	return ws.NewResponse(msg.ID, msg.Action, httpBackgroundWorkloadListResponse{Workloads: dtos})
}

func (h *TaskHandlers) wsGetBackgroundWorkload(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if !h.isBackgroundWorkEnabled() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "background work feature is disabled", nil)
	}
	var req struct {
		SessionID string `json:"session_id"`
		WorkID    string `json:"work_id"`
	}
	if err := msg.ParsePayload(&req); err != nil || strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.WorkID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session_id and work_id are required", nil)
	}
	if h.service == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "workload not found", nil)
	}
	workload, err := h.service.GetBackgroundWorkload(ctx, req.SessionID, req.WorkID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	if workload == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "workload not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, httpBackgroundWorkloadDetailResponse{Workload: toWorkloadObservation(workload)})
}

func (h *TaskHandlers) httpExecuteBackgroundAction(c *gin.Context) {
	if !h.isBackgroundWorkEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "background work feature is disabled"})
		return
	}
	sessionID := c.Param("id")
	workID := c.Param("workId")
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(workID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id and work id are required"})
		return
	}
	var req streams.BackgroundWorkActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	req.WorkID = workID

	if h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "service not available"})
		return
	}
	resp, err := h.service.ExecuteBackgroundWorkAction(c.Request.Context(), sessionID, req)
	if err != nil {
		h.logger.Error("execute background action failed", zap.String("session_id", sessionID), zap.String("work_id", workID), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "response": resp})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpGetBackgroundWorkloadUsage(c *gin.Context) {
	if !h.isBackgroundWorkEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "background work feature is disabled"})
		return
	}
	sessionID := c.Param("id")
	workID := c.Param("workId")
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(workID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id and work id are required"})
		return
	}
	if h.service == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not available"})
		return
	}
	usage, err := h.service.GetBackgroundWorkloadUsage(c.Request.Context(), sessionID, workID)
	if err != nil {
		h.logger.Error("get background workload usage failed", zap.String("session_id", sessionID), zap.String("work_id", workID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (h *TaskHandlers) wsExecuteBackgroundAction(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if !h.isBackgroundWorkEnabled() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "background work feature is disabled", nil)
	}
	var payload struct {
		SessionID   string                     `json:"session_id"`
		WorkID      string                     `json:"work_id"`
		RunID       string                     `json:"run_id,omitempty"`
		Action      streams.WorkloadActionKind `json:"action"`
		Data        string                     `json:"data,omitempty"`
		OperationID string                     `json:"operation_id,omitempty"`
	}
	if err := msg.ParsePayload(&payload); err != nil || strings.TrimSpace(payload.SessionID) == "" || strings.TrimSpace(payload.WorkID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session_id, work_id, and action are required", nil)
	}
	if h.service == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "service not available", nil)
	}
	req := streams.BackgroundWorkActionRequest{
		WorkID:      payload.WorkID,
		RunID:       payload.RunID,
		Action:      payload.Action,
		Data:        payload.Data,
		OperationID: payload.OperationID,
	}
	resp, err := h.service.ExecuteBackgroundWorkAction(ctx, payload.SessionID, req)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), map[string]interface{}{
			"work_id": resp.WorkID,
			"action":  string(resp.Action),
			"error":   resp.Error,
		})
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
