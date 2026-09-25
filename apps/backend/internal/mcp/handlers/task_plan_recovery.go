package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/planws"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) handleListTaskPlanRevisions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID               string `json:"task_id"`
		BeforeRevisionNumber int    `json:"before_revision_number"`
		Limit                int    `json:"limit"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	page, err := h.planService.ListAgentPlanRevisionMetadata(
		ctx, req.TaskID, req.BeforeRevisionNumber, req.Limit,
	)
	if err != nil {
		return planws.Error(msg, err, "Failed to list task plan revisions")
	}
	revisions := make([]map[string]interface{}, 0, len(page.Revisions))
	for _, revision := range page.Revisions {
		revisions = append(revisions, planRevisionMetadataPayload(revision))
	}
	payload := map[string]interface{}{
		"task_id":   req.TaskID,
		"revisions": revisions,
	}
	if page.NextBeforeRevisionNum > 0 {
		payload["next_before_revision_number"] = page.NextBeforeRevisionNum
	}
	return ws.NewResponse(msg.ID, msg.Action, payload)
}

func (h *Handlers) handleGetTaskPlanRevision(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID     string `json:"task_id"`
		RevisionID string `json:"revision_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	revision, version, err := h.planService.GetAgentPlanRevision(ctx, req.TaskID, req.RevisionID)
	if err != nil {
		return planws.Error(msg, err, "Failed to get task plan revision")
	}
	return ws.NewResponse(msg.ID, msg.Action, struct {
		*dto.TaskPlanRevisionDTO
		RevisionVersion string `json:"revision_version"`
	}{
		TaskPlanRevisionDTO: dto.TaskPlanRevisionFromModel(revision),
		RevisionVersion:     version,
	})
}

func (h *Handlers) handleRestoreTaskPlanRevision(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID                  string `json:"task_id"`
		RevisionID              string `json:"revision_id"`
		ExpectedVersion         string `json:"expected_version"`
		ExpectedRevisionVersion string `json:"expected_revision_version"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	result, err := h.planService.RestoreAgentPlanRevision(ctx, service.RestorePlanRequest{
		TaskID:                  req.TaskID,
		RevisionID:              req.RevisionID,
		ExpectedVersion:         req.ExpectedVersion,
		ExpectedRevisionVersion: req.ExpectedRevisionVersion,
	})
	if err != nil {
		return planws.Error(msg, err, "Failed to restore task plan revision")
	}
	status := "restored"
	if result.AlreadyCurrent {
		status = "already_current"
	}
	payload := map[string]interface{}{
		"task_id":          req.TaskID,
		"status":           status,
		"title":            result.Plan.Title,
		"version":          result.Plan.WriteVersion,
		"revision_id":      result.Revision.ID,
		"revision_number":  result.Revision.RevisionNumber,
		"content_bytes":    len(result.Plan.Content),
		"revision_version": service.PlanRevisionVersion(result.Revision),
	}
	if result.Revision.RevertOfRevisionID != nil {
		payload["revert_of_revision_id"] = *result.Revision.RevertOfRevisionID
	}
	return ws.NewResponse(msg.ID, msg.Action, payload)
}

func planRevisionMetadataPayload(revision *models.TaskPlanRevision) map[string]interface{} {
	payload := map[string]interface{}{
		"revision_id":     revision.ID,
		"task_id":         revision.TaskID,
		"revision_number": revision.RevisionNumber,
		"title":           revision.Title,
		"content_bytes":   revision.ContentBytes,
		"author_kind":     revision.AuthorKind,
		"author_name":     revision.AuthorName,
		"created_at":      revision.CreatedAt,
		"updated_at":      revision.UpdatedAt,
	}
	if revision.RevertOfRevisionID != nil {
		payload["revert_of_revision_id"] = *revision.RevertOfRevisionID
	}
	if revision.WorkflowStepID != "" {
		payload["workflow_step_id"] = revision.WorkflowStepID
	}
	if revision.WorkflowStepName != "" {
		payload["workflow_step_name"] = revision.WorkflowStepName
	}
	if revision.WorkflowStepColor != "" {
		payload["workflow_step_color"] = revision.WorkflowStepColor
	}
	return payload
}
