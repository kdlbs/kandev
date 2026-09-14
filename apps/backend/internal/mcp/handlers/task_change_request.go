package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	TaskChangeRequestProviderGitHub = "github"
	TaskChangeRequestProviderGitLab = "gitlab"

	TaskChangeRequestIdentityResolved   = "resolved"
	TaskChangeRequestIdentityUnresolved = "unresolved"
	TaskChangeRequestIdentityAmbiguous  = "ambiguous"

	TaskChangeRequestPromptScopeTaskProvider = "task_provider"

	TaskChangeRequestProviderStatusAvailable   = "available"
	TaskChangeRequestProviderStatusUnavailable = "unavailable"
	TaskChangeRequestProviderStatusFailed      = "failed"

	TaskChangeRequestAutomationScopeAssociation = "association"
	TaskChangeRequestAutomationScopeTask        = "task"
	TaskChangeRequestAutomationStatusApplied    = "applied"
	TaskChangeRequestAutomationStatusPartial    = "partial"
	TaskChangeRequestAutomationStatusFailed     = "failed"
	TaskChangeRequestAutomationProviderApplied  = "applied"
	TaskChangeRequestAutomationProviderFailed   = "failed"
	TaskChangeRequestAutomationProviderSkipped  = "not_attempted"
)

// TaskChangeRequestReadService reads the provider-neutral, task-bound change
// request snapshot. The MCP handler supplies the task ID from its trusted
// execution context rather than from a public tool argument.
type TaskChangeRequestReadService interface {
	GetTaskChangeRequests(context.Context, string) (TaskChangeRequestReadResponse, error)
}

// TaskChangeRequestReadResponse is the stable result envelope for the bound
// change request read tool.
type TaskChangeRequestReadResponse struct {
	TaskID               string                                  `json:"task_id"`
	Complete             bool                                    `json:"complete"`
	ChangeRequests       []TaskChangeRequest                     `json:"change_requests"`
	ProviderSettings     []TaskChangeRequestProviderSettings     `json:"provider_settings"`
	ProviderCapabilities []TaskChangeRequestProviderCapabilities `json:"provider_capabilities"`
	Errors               []TaskChangeRequestProviderError        `json:"errors"`
}

// TaskChangeRequest is one persisted provider association. ProviderState
// preserves provider-specific state when State is normalized to the shared
// open/closed/merged/unknown vocabulary.
type TaskChangeRequest struct {
	Provider      string  `json:"provider"`
	RepositoryID  *string `json:"repository_id"`
	Number        int     `json:"number"`
	URL           string  `json:"url"`
	Title         string  `json:"title,omitempty"`
	State         string  `json:"state"`
	ProviderState string  `json:"provider_state,omitempty"`
	// ProviderProjectPath is retained only inside the backend adapter while it
	// matches GitLab automation rows. It is never exposed to MCP callers.
	ProviderProjectPath string `json:"-"`
	// ProviderRepositoryID preserves the provider row's original identity when
	// a legacy GitLab association has no canonical repository ID. It is used
	// only for matching persisted automation rows.
	ProviderRepositoryID string                             `json:"-"`
	Draft                *bool                              `json:"draft"`
	BaseRef              string                             `json:"base_ref,omitempty"`
	HeadRef              string                             `json:"head_ref,omitempty"`
	HeadSHA              string                             `json:"head_sha"`
	MergedAt             *time.Time                         `json:"merged_at"`
	ClosedAt             *time.Time                         `json:"closed_at"`
	UpdatedAt            *time.Time                         `json:"updated_at"`
	IdentityStatus       string                             `json:"identity_status"`
	Automation           *TaskChangeRequestAutomation       `json:"automation"`
	AutomationStatus     *TaskChangeRequestAutomationStatus `json:"automation_status"`
	Capabilities         map[string]bool                    `json:"capabilities"`
	CapabilityReasons    map[string]string                  `json:"capability_reasons,omitempty"`
}

// TaskChangeRequestAutomation contains the five switches for one association.
// Pointers preserve the distinction between a confirmed false value and a
// provider read that could not load settings.
type TaskChangeRequestAutomation struct {
	AutoFixEnabled          *bool `json:"auto_fix_enabled"`
	AutoMergeEnabled        *bool `json:"auto_merge_enabled"`
	PromptOnReviewRequested *bool `json:"prompt_on_review_requested"`
	PromptOnMerged          *bool `json:"prompt_on_merged"`
	PromptOnClosed          *bool `json:"prompt_on_closed"`
}

// TaskChangeRequestAutomationStatus exposes provider-labelled operational
// state without renaming provider-specific checkpoints into shared concepts.
type TaskChangeRequestAutomationStatus struct {
	AutoFixRoundCount *int    `json:"auto_fix_round_count"`
	LastError         *string `json:"last_error"`
	StateKnown        bool    `json:"state_known"`
}

// TaskChangeRequestProviderSettings contains task/provider prompt settings.
// Lifecycle notification prompts remain server-owned and are intentionally
// absent from this DTO.
type TaskChangeRequestProviderSettings struct {
	Provider               string                             `json:"provider"`
	Status                 string                             `json:"status"`
	Configured             bool                               `json:"configured"`
	Available              bool                               `json:"available"`
	ReasonCode             string                             `json:"reason_code,omitempty"`
	AutoFixPromptOverride  *string                            `json:"auto_fix_prompt_override"`
	EffectiveAutoFixPrompt string                             `json:"effective_auto_fix_prompt"`
	UsingDefaultPrompt     bool                               `json:"using_default_prompt"`
	AutoFixMaxRounds       int                                `json:"auto_fix_max_rounds"`
	PromptScope            string                             `json:"prompt_scope"`
	AutomationStatus       *TaskChangeRequestAutomationStatus `json:"automation_status"`
}

// TaskChangeRequestProviderCapabilities separates static feature support from
// current workspace availability.
type TaskChangeRequestProviderCapabilities struct {
	Provider     string          `json:"provider"`
	Status       string          `json:"status"`
	Configured   bool            `json:"configured"`
	Available    bool            `json:"available"`
	ReasonCode   string          `json:"reason_code,omitempty"`
	Capabilities map[string]bool `json:"capabilities"`
}

// TaskChangeRequestProviderError identifies a provider read that made the
// snapshot incomplete. Message is sanitized before it reaches this DTO.
type TaskChangeRequestProviderError struct {
	Provider string `json:"provider"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// TaskChangeRequestAutomationService applies a validated, task-bound
// provider-neutral automation patch.
type TaskChangeRequestAutomationService interface {
	UpdateTaskChangeRequestAutomation(
		context.Context, string, TaskChangeRequestAutomationRequest,
	) (TaskChangeRequestAutomationResult, error)
}

// TaskChangeRequestAutomationRequest is the internal request passed from the
// neutral MCP handler to the backendapp coordinator.
type TaskChangeRequestAutomationRequest struct {
	Target TaskChangeRequestAutomationTarget
	Patch  TaskChangeRequestAutomationPatch
}

// TaskChangeRequestAutomationTarget selects one association or explicit task
// provider targets. A task target never means all providers implicitly.
type TaskChangeRequestAutomationTarget struct {
	Scope        string
	Provider     string
	RepositoryID string
	Number       int
	Providers    []string
}

// TaskChangeRequestAutomationPatch is the partial set of supported switches
// and the task/provider prompt override.
type TaskChangeRequestAutomationPatch struct {
	AutoFixEnabled          *bool
	AutoMergeEnabled        *bool
	PromptOnReviewRequested *bool
	PromptOnMerged          *bool
	PromptOnClosed          *bool
	AutoFixPromptOverride   *string
}

// HasAny reports whether at least one field was explicitly supplied.
func (p TaskChangeRequestAutomationPatch) HasAny() bool {
	return p.AutoFixEnabled != nil || p.AutoMergeEnabled != nil ||
		p.PromptOnReviewRequested != nil || p.PromptOnMerged != nil ||
		p.PromptOnClosed != nil || p.AutoFixPromptOverride != nil
}

// TaskChangeRequestAutomationIdentity identifies an affected provider row.
type TaskChangeRequestAutomationIdentity struct {
	Provider     string `json:"provider"`
	RepositoryID string `json:"repository_id"`
	Number       int    `json:"number"`
}

// TaskChangeRequestAutomationProviderResult reports one provider transaction
// and whether its resulting state was read back successfully.
type TaskChangeRequestAutomationProviderResult struct {
	Provider                string                                `json:"provider"`
	Status                  string                                `json:"status"`
	Affected                []TaskChangeRequestAutomationIdentity `json:"affected"`
	ResultingSettings       *TaskChangeRequestProviderSettings    `json:"resulting_settings,omitempty"`
	ResultingChangeRequests []TaskChangeRequest                   `json:"resulting_change_requests,omitempty"`
	StateKnown              bool                                  `json:"state_known"`
	ErrorCode               string                                `json:"error_code,omitempty"`
	ErrorMessage            string                                `json:"error_message,omitempty"`
}

// TaskChangeRequestAutomationResult describes independent provider outcomes.
// Partial results deliberately remain usable for retrying only failed targets.
type TaskChangeRequestAutomationResult struct {
	TaskID     string                                      `json:"task_id"`
	Status     string                                      `json:"status"`
	Providers  []TaskChangeRequestAutomationProviderResult `json:"providers"`
	StateKnown bool                                        `json:"state_known"`
}

func (h *Handlers) SetTaskChangeRequestReadService(reader TaskChangeRequestReadService) {
	h.taskChangeRequestReader = reader
}

// SetTaskChangeRequestAutomationService wires provider-neutral automation
// updates at the backendapp composition boundary.
func (h *Handlers) SetTaskChangeRequestAutomationService(automation TaskChangeRequestAutomationService) {
	h.taskChangeRequestAutomation = automation
}

func (h *Handlers) handleGetTaskChangeRequests(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, errResp, err := parseBoundTaskChangeRequestReadPayload(msg)
	if errResp != nil || err != nil {
		return errResp, err
	}
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not available", nil)
	}
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || strings.TrimSpace(principal.WorkspaceID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "trusted MCP caller identity is required", nil)
	}
	if taskID != principal.CallerTaskID || task.WorkspaceID != principal.WorkspaceID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "task is outside the current MCP caller scope", nil)
	}
	if h.taskChangeRequestReader == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task change request read is not available", nil)
	}
	result, err := h.taskChangeRequestReader.GetTaskChangeRequests(ctx, taskID)
	if err != nil {
		h.logger.Error("failed to read task change requests", zap.String("task_id", taskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to read task change requests", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func parseBoundTaskChangeRequestReadPayload(msg *ws.Message) (string, *ws.Message, error) {
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(msg.Payload, &fields); err != nil || fields == nil {
		if err == nil {
			err = errors.New("object payload is required")
		}
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return "", response, responseErr
	}
	for field := range fields {
		if field != "task_id" {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fmt.Sprintf("unknown task change request read field %q", field), nil)
			return "", response, responseErr
		}
	}
	var payload struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return "", response, responseErr
	}
	payload.TaskID = strings.TrimSpace(payload.TaskID)
	if payload.TaskID == "" {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "bound task_id is required", nil)
		return "", response, responseErr
	}
	return payload.TaskID, nil, nil
}

func (h *Handlers) handleUpdateTaskChangeRequestAutomation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	request, errResp, err := parseTaskChangeRequestAutomationPayload(msg)
	if errResp != nil || err != nil {
		return errResp, err
	}
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || strings.TrimSpace(principal.WorkspaceID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "trusted MCP caller identity is required", nil)
	}
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not available", nil)
	}
	task, taskErr := h.taskSvc.GetTask(ctx, principal.CallerTaskID)
	if taskErr != nil || task == nil || task.WorkspaceID != principal.WorkspaceID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "task is outside the current MCP caller scope", nil)
	}
	if h.taskChangeRequestAutomation == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task change request automation is not available", nil)
	}
	result, updateErr := h.taskChangeRequestAutomation.UpdateTaskChangeRequestAutomation(
		ctx, principal.CallerTaskID, request,
	)
	if updateErr != nil {
		h.logger.Error("failed to update task change request automation", zap.Error(updateErr), zap.String("task_id", principal.CallerTaskID))
		return taskChangeRequestAutomationErrorResponse(msg, result, updateErr)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func parseTaskChangeRequestAutomationPayload(msg *ws.Message) (TaskChangeRequestAutomationRequest, *ws.Message, error) {
	root, err := taskChangeRequestFields(msg.Payload)
	if err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return TaskChangeRequestAutomationRequest{}, response, responseErr
	}
	if err := rejectFields(root, "target", "patch"); err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		return TaskChangeRequestAutomationRequest{}, response, responseErr
	}
	targetRaw, ok := root["target"]
	if !ok || isJSONNull(targetRaw) {
		return automationPayloadValidationError(msg, "target is required")
	}
	patchRaw, ok := root["patch"]
	if !ok || isJSONNull(patchRaw) {
		return automationPayloadValidationError(msg, "patch is required")
	}
	targetFields, err := objectFields(targetRaw)
	if err != nil {
		return automationPayloadValidationError(msg, "target must be an object")
	}
	patchFields, err := objectFields(patchRaw)
	if err != nil {
		return automationPayloadValidationError(msg, "patch must be an object")
	}
	target, err := parseTaskChangeRequestAutomationTarget(targetFields)
	if err != nil {
		return automationPayloadValidationError(msg, err.Error())
	}
	patch, err := parseTaskChangeRequestAutomationPatch(patchFields)
	if err != nil {
		return automationPayloadValidationError(msg, err.Error())
	}
	if target.Scope == TaskChangeRequestAutomationScopeAssociation && patch.AutoFixPromptOverride != nil {
		return automationPayloadValidationError(msg, "auto_fix_prompt_override is only valid for a task target")
	}
	return TaskChangeRequestAutomationRequest{Target: target, Patch: patch}, nil, nil
}

func parseTaskChangeRequestAutomationTarget(fields map[string]json.RawMessage) (TaskChangeRequestAutomationTarget, error) {
	if err := rejectFields(fields, "scope", "provider", "repository_id", "number", "providers"); err != nil {
		return TaskChangeRequestAutomationTarget{}, err
	}
	scope, err := requiredAutomationString(fields, "scope")
	if err != nil {
		return TaskChangeRequestAutomationTarget{}, err
	}
	target := TaskChangeRequestAutomationTarget{Scope: scope}
	switch scope {
	case TaskChangeRequestAutomationScopeAssociation:
		return parseAssociationAutomationTarget(fields, target)
	case TaskChangeRequestAutomationScopeTask:
		return parseTaskAutomationTarget(fields, target)
	default:
		return TaskChangeRequestAutomationTarget{}, errors.New("scope must be association or task")
	}
}

func parseAssociationAutomationTarget(
	fields map[string]json.RawMessage, target TaskChangeRequestAutomationTarget,
) (TaskChangeRequestAutomationTarget, error) {
	if hasAnyField(fields, "providers") {
		return TaskChangeRequestAutomationTarget{}, errors.New("providers is only valid for a task target")
	}
	provider, err := requiredAutomationString(fields, "provider")
	if err != nil {
		return TaskChangeRequestAutomationTarget{}, err
	}
	if provider != TaskChangeRequestProviderGitHub && provider != TaskChangeRequestProviderGitLab {
		return TaskChangeRequestAutomationTarget{}, errors.New("provider must be github or gitlab")
	}
	target.Provider = provider
	target.RepositoryID, err = requiredAutomationString(fields, "repository_id")
	if err != nil {
		return TaskChangeRequestAutomationTarget{}, err
	}
	target.Number, err = requiredAutomationNumber(fields, "number")
	if err != nil {
		return TaskChangeRequestAutomationTarget{}, err
	}
	return target, nil
}

func parseTaskAutomationTarget(
	fields map[string]json.RawMessage, target TaskChangeRequestAutomationTarget,
) (TaskChangeRequestAutomationTarget, error) {
	for _, field := range []string{"provider", "repository_id", "number"} {
		if hasAnyField(fields, field) {
			return TaskChangeRequestAutomationTarget{}, fmt.Errorf("%s is only valid for an association target", field)
		}
	}
	providers, err := requiredAutomationStringArray(fields, "providers")
	if err != nil {
		return TaskChangeRequestAutomationTarget{}, err
	}
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if provider != TaskChangeRequestProviderGitHub && provider != TaskChangeRequestProviderGitLab {
			return TaskChangeRequestAutomationTarget{}, errors.New("providers must contain only github or gitlab")
		}
		if _, exists := seen[provider]; exists {
			return TaskChangeRequestAutomationTarget{}, errors.New("providers must not contain duplicates")
		}
		seen[provider] = struct{}{}
	}
	target.Providers = providers
	return target, nil
}

func parseTaskChangeRequestAutomationPatch(fields map[string]json.RawMessage) (TaskChangeRequestAutomationPatch, error) {
	if err := rejectFields(fields,
		"auto_fix_enabled", "auto_merge_enabled", "prompt_on_review_requested",
		"prompt_on_merged", "prompt_on_closed", "auto_fix_prompt_override",
	); err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	patch := TaskChangeRequestAutomationPatch{}
	var err error
	patch.AutoFixEnabled, err = optionalAutomationBool(fields, "auto_fix_enabled")
	if err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	patch.AutoMergeEnabled, err = optionalAutomationBool(fields, "auto_merge_enabled")
	if err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	patch.PromptOnReviewRequested, err = optionalAutomationBool(fields, "prompt_on_review_requested")
	if err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	patch.PromptOnMerged, err = optionalAutomationBool(fields, "prompt_on_merged")
	if err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	patch.PromptOnClosed, err = optionalAutomationBool(fields, "prompt_on_closed")
	if err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	patch.AutoFixPromptOverride, err = optionalAutomationString(fields, "auto_fix_prompt_override")
	if err != nil {
		return TaskChangeRequestAutomationPatch{}, err
	}
	if !patch.HasAny() {
		return TaskChangeRequestAutomationPatch{}, errors.New("patch must contain at least one supported field")
	}
	return patch, nil
}

func taskChangeRequestAutomationErrorResponse(msg *ws.Message, result TaskChangeRequestAutomationResult, err error) (*ws.Message, error) {
	details := map[string]interface{}{
		"task_id":     result.TaskID,
		"status":      result.Status,
		"providers":   result.Providers,
		"state_known": result.StateKnown,
	}
	if err != nil && result.Status == "" {
		details["error_code"] = "automation_update_failed"
		details["error_message"] = "task change request automation update failed"
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "failed to update task change request automation", details)
}

func automationPayloadValidationError(msg *ws.Message, message string) (TaskChangeRequestAutomationRequest, *ws.Message, error) {
	response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, message, nil)
	return TaskChangeRequestAutomationRequest{}, response, responseErr
}

func rejectFields(fields map[string]json.RawMessage, allowed ...string) error {
	known := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		known[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := known[field]; !ok {
			return fmt.Errorf("unknown task change request automation field %q", field)
		}
	}
	return nil
}

func objectFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("object is required")
	}
	return fields, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == jsonNull
}

func hasAnyField(fields map[string]json.RawMessage, key string) bool {
	_, ok := fields[key]
	return ok
}

func requiredAutomationString(fields map[string]json.RawMessage, key string) (string, error) {
	raw, ok := fields[key]
	if !ok || isJSONNull(raw) {
		return "", fmt.Errorf("%s is required", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a nonempty string", key)
	}
	return strings.TrimSpace(value), nil
}

func requiredAutomationNumber(fields map[string]json.RawMessage, key string) (int, error) {
	raw, ok := fields[key]
	if !ok || isJSONNull(raw) {
		return 0, fmt.Errorf("%s is required", key)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func requiredAutomationStringArray(fields map[string]json.RawMessage, key string) ([]string, error) {
	raw, ok := fields[key]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("%s is required", key)
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil || len(values) == 0 {
		return nil, fmt.Errorf("%s must be a nonempty array of strings", key)
	}
	for i := range values {
		if strings.TrimSpace(values[i]) == "" {
			return nil, fmt.Errorf("%s must contain nonempty strings", key)
		}
		values[i] = strings.TrimSpace(values[i])
	}
	return values, nil
}

func optionalAutomationBool(fields map[string]json.RawMessage, key string) (*bool, error) {
	raw, ok := fields[key]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s cannot be null", key)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a boolean", key)
	}
	return &value, nil
}

func optionalAutomationString(fields map[string]json.RawMessage, key string) (*string, error) {
	raw, ok := fields[key]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s cannot be null", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	return &value, nil
}
