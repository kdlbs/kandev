package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// errLifecyclePromptOverridesUnsupported mirrors github's sentinel — GitLab's
// three prompts are immutable server-owned templates; only the booleans are
// a supported API surface (AC4).
var errLifecyclePromptOverridesUnsupported = errors.New("lifecycle prompt overrides are not supported")

// errAtLeastOneMRAutomationOptionRequired backs AC3: unlike GitHub's PATCH
// (which treats an empty body as a no-op GET), an empty MR automation PATCH
// is rejected outright.
var errAtLeastOneMRAutomationOptionRequired = errors.New("at least one MR automation option is required")

// RegisterMRAutomationHTTPRoutes registers the GET/PATCH MR automation
// endpoints on an existing /api/v1/gitlab router group.
func (c *Controller) RegisterMRAutomationHTTPRoutes(api *gin.RouterGroup) {
	api.GET("/tasks/:taskID/mr-automation", c.httpGetTaskMRAutomation)
	api.PATCH("/tasks/:taskID/mr-automation", c.httpPatchTaskMRAutomation)
}

func (c *Controller) httpGetTaskMRAutomation(ctx *gin.Context) {
	resp, err := c.service.GetTaskMRAutomationResponse(ctx.Request.Context(), ctx.Param("taskID"))
	if err != nil {
		c.logger.Error("get task MR automation failed", zap.String("task_id", ctx.Param("taskID")), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "failed to load MR automation options"})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (c *Controller) httpPatchTaskMRAutomation(ctx *gin.Context) {
	patch, err := parseTaskMRAutomationPatch(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}
	if !patch.HasAny() {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: errAtLeastOneMRAutomationOptionRequired.Error()})
		return
	}
	resp, err := c.service.UpdateTaskMRAutomationOptions(ctx.Request.Context(), ctx.Param("taskID"), patch)
	if err != nil {
		c.logger.Error("update task MR automation failed", zap.String("task_id", ctx.Param("taskID")), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "failed to update MR automation options"})
		return
	}
	c.publishTaskMRAutomationUpdated(ctx.Request.Context(), resp)
	ctx.JSON(http.StatusOK, resp)
}

func parseTaskMRAutomationPatch(ctx *gin.Context) (TaskMRAutomationPatch, error) {
	if ctx.Request.Body == nil || ctx.Request.ContentLength == 0 {
		return TaskMRAutomationPatch{}, nil
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(ctx.Request.Body).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return TaskMRAutomationPatch{}, nil
		}
		return TaskMRAutomationPatch{}, err
	}
	var patch TaskMRAutomationPatch
	for key, value := range raw {
		switch key {
		case "prompt_on_review_requested":
			if err := json.Unmarshal(value, &patch.PromptOnReviewRequested); err != nil {
				return TaskMRAutomationPatch{}, err
			}
		case "prompt_on_merged":
			if err := json.Unmarshal(value, &patch.PromptOnMerged); err != nil {
				return TaskMRAutomationPatch{}, err
			}
		case "prompt_on_closed":
			if err := json.Unmarshal(value, &patch.PromptOnClosed); err != nil {
				return TaskMRAutomationPatch{}, err
			}
		case "review_prompt_override", "merged_prompt_override", "closed_prompt_override":
			return TaskMRAutomationPatch{}, errLifecyclePromptOverridesUnsupported
		}
	}
	return patch, nil
}

func (c *Controller) publishTaskMRAutomationUpdated(ctx context.Context, resp *TaskMRAutomationResponse) {
	if c.service == nil || resp == nil {
		return
	}
	c.service.mu.RLock()
	eb := c.service.eventBus
	c.service.mu.RUnlock()
	if eb == nil {
		return
	}
	event := bus.NewEvent(events.GitLabTaskMROptionsUpdated, eventSource, resp)
	if err := eb.Publish(ctx, events.GitLabTaskMROptionsUpdated, event); err != nil {
		c.logger.Debug("publish task MR automation update failed", zap.String("task_id", resp.TaskID), zap.Error(err))
	}
}
