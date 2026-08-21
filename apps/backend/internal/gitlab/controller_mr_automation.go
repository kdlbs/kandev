package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// errLifecyclePromptOverridesUnsupported mirrors github's sentinel — GitLab's
// three prompts are immutable server-owned templates; only the booleans are
// a supported API surface (AC4).
var errLifecyclePromptOverridesUnsupported = errors.New("lifecycle prompt overrides are not supported")

// errAtLeastOneMRAutomationOptionRequired backs AC3: unlike GitHub's PATCH
// (which treats an empty body as a no-op GET), an empty MR automation PATCH
// is rejected outright.
var errAtLeastOneMRAutomationOptionRequired = errors.New("at least one MR automation option is required")

// errUnknownMRAutomationField rejects a misspelled or unsupported key rather
// than silently ignoring it — without this, a caller-side typo like
// "promt_on_merged" would drop that field with no error while any other
// valid field in the same body still applied.
var errUnknownMRAutomationField = errors.New("unknown MR automation field")

// errNullMRAutomationSwitch rejects an explicit `null` for a boolean switch.
// Unmarshaling `null` into a *bool sets it back to nil, which is
// indistinguishable from the field never being sent at all — silently
// treating a caller's explicit null as "no change" rather than surfacing it
// as a bad request.
var errNullMRAutomationSwitch = errors.New("MR automation switch must be a boolean, not null")

var errNullMRAutomationIdentity = errors.New("MR automation identity fields must not be null")

// RegisterMRAutomationHTTPRoutes registers the GET/PATCH MR automation
// endpoints on an existing /api/v1/gitlab router group.
func (c *Controller) RegisterMRAutomationHTTPRoutes(api *gin.RouterGroup) {
	api.GET("/tasks/:taskID/mr-automation", c.httpGetTaskMRAutomation)
	api.PATCH("/tasks/:taskID/mr-automation", c.httpPatchTaskMRAutomation)
}

func (c *Controller) httpGetTaskMRAutomation(ctx *gin.Context) {
	resp, err := c.service.GetTaskMRAutomationResponse(ctx.Request.Context(), ctx.Param("taskID"))
	if err != nil {
		if writeMRAutomationTaskNotFound(ctx, err) {
			return
		}
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
		if writeMRAutomationTaskNotFound(ctx, err) {
			return
		}
		// A patch naming an MR that isn't linked (or naming one only
		// partially) is a caller mistake, not a server fault.
		if errors.Is(err, ErrTaskMRNotLinked) {
			ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
			return
		}
		c.logger.Error("update task MR automation failed", zap.String("task_id", ctx.Param("taskID")), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "failed to update MR automation options"})
		return
	}
	c.publishTaskMRAutomationUpdated(ctx.Request.Context(), resp)
	ctx.JSON(http.StatusOK, resp)
}

// writeMRAutomationTaskNotFound maps the scoped-visibility denial from
// Service.authorizeTaskMRAccess, and an unknown task ID from
// store.WorkspaceIDForTask (ErrNotConfigured), to a 404 — matching the
// "foreign workspace is indistinguishable from nonexistent" no-existence-leak
// convention (task/service/service_access.go) instead of a generic 500.
func writeMRAutomationTaskNotFound(ctx *gin.Context, err error) bool {
	if errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) ||
		errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: "task not found"})
		return true
	}
	return false
}

func parseTaskMRAutomationPatch(ctx *gin.Context) (TaskMRAutomationPatch, error) {
	if ctx.Request.Body == nil || ctx.Request.ContentLength == 0 {
		return TaskMRAutomationPatch{}, nil
	}
	raw, err := decodeSingleJSONObject(ctx.Request.Body)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return TaskMRAutomationPatch{}, nil
		}
		return TaskMRAutomationPatch{}, err
	}
	var patch TaskMRAutomationPatch
	for key, value := range raw {
		if err := applyMRAutomationPatchField(&patch, key, value); err != nil {
			return TaskMRAutomationPatch{}, err
		}
	}
	return patch, nil
}

// decodeSingleJSONObject decodes exactly one JSON object from body and
// requires EOF immediately after it. Decode alone stops after the first
// JSON value, so a body like `{...}{...}` would otherwise silently apply
// only the first object and ignore the rest — requiring EOF here rejects
// any trailing content as malformed instead.
func decodeSingleJSONObject(body io.Reader) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(body)
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected trailing content after JSON body")
		}
		return nil, err
	}
	return raw, nil
}

func applyMRAutomationPatchField(patch *TaskMRAutomationPatch, key string, value json.RawMessage) error {
	switch key {
	case "auto_fix_enabled":
		return decodeMRAutomationSwitch(value, &patch.AutoFixEnabled)
	case "auto_merge_enabled":
		return decodeMRAutomationSwitch(value, &patch.AutoMergeEnabled)
	case "auto_fix_prompt_override":
		return decodeMRAutoFixPromptOverride(value, &patch.AutoFixPromptOverride)
	case "prompt_on_review_requested":
		return decodeMRAutomationSwitch(value, &patch.PromptOnReviewRequested)
	case "prompt_on_merged":
		return decodeMRAutomationSwitch(value, &patch.PromptOnMerged)
	case "prompt_on_closed":
		return decodeMRAutomationSwitch(value, &patch.PromptOnClosed)
	case "repository_id":
		return decodeMRAutomationIdentityString(value, &patch.RepositoryID)
	case "project_path":
		return decodeMRAutomationIdentityString(value, &patch.ProjectPath)
	case "mr_iid":
		return decodeMRAutomationIdentityInteger(value, &patch.MRIID)
	case "review_prompt_override", "merged_prompt_override", "closed_prompt_override":
		return errLifecyclePromptOverridesUnsupported
	default:
		return fmt.Errorf("%w: %q", errUnknownMRAutomationField, key)
	}
}

// decodeMRAutoFixPromptOverride unmarshals the auto-fix prompt override
// field. A JSON null is treated the same as an explicit empty string —
// "restore the default prompt" (AC5) — rather than rejected, matching
// GitHub's equivalent field. This differs deliberately from the boolean
// switches: there is no "field absent vs explicitly null" ambiguity to
// guard against here, since both mean the same thing for this field.
func decodeMRAutoFixPromptOverride(value json.RawMessage, dst **string) error {
	if string(value) == "null" {
		empty := ""
		*dst = &empty
		return nil
	}
	var prompt string
	if err := json.Unmarshal(value, &prompt); err != nil {
		return err
	}
	*dst = &prompt
	return nil
}

// decodeMRAutomationSwitch unmarshals a boolean patch field, rejecting an
// explicit JSON null rather than silently unmarshaling it into a nil *bool
// (indistinguishable from the field being absent).
func decodeMRAutomationSwitch(value json.RawMessage, dst **bool) error {
	if string(value) == "null" {
		return errNullMRAutomationSwitch
	}
	return json.Unmarshal(value, dst)
}

func decodeMRAutomationIdentityString(value json.RawMessage, dst **string) error {
	if string(value) == "null" {
		return errNullMRAutomationIdentity
	}
	return json.Unmarshal(value, dst)
}

func decodeMRAutomationIdentityInteger(value json.RawMessage, dst **int) error {
	if string(value) == "null" {
		return errNullMRAutomationIdentity
	}
	return json.Unmarshal(value, dst)
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
