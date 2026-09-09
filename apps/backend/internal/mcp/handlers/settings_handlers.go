package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/kandev/kandev/internal/settingscatalog"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	maxSettingsPayloadBytes = 256 * 1024
	jsonNull                = "null"
)

// SettingsOperations is the domain-owned execution boundary for compact
// settings reads, updates, and target lookup. The generic handler validates
// metadata and limits, while implementations retain authorization, reference
// checks, persistence atomicity, and event publication.
type SettingsOperations interface {
	ReadSettings(context.Context, settingscatalog.ResourceTarget, []string) (any, error)
	UpdateSettings(context.Context, settingscatalog.ResourceTarget, map[string]json.RawMessage, map[string]json.RawMessage) (any, error)
	ListSettingsResources(context.Context, string, *string, string, int, string) (any, error)
}

type settingsUpdateRequest struct {
	Target  settingscatalog.ResourceTarget `json:"target"`
	Changes map[string]json.RawMessage     `json:"changes"`
	Options map[string]json.RawMessage     `json:"options,omitempty"`
}

type settingsReadRequest struct {
	Target settingscatalog.ResourceTarget `json:"target"`
	Keys   []string                       `json:"keys"`
}

type settingsDescribeRequest struct {
	Key                   string                          `json:"key"`
	Target                *settingscatalog.ResourceTarget `json:"target,omitempty"`
	Context               map[string]any                  `json:"context,omitempty"`
	Operation             string                          `json:"operation,omitempty"`
	IncludeResourceSchema bool                            `json:"include_resource_schema,omitempty"`
}

type settingsSearchRequest struct {
	Query        string `json:"query,omitempty"`
	Domain       string `json:"domain,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Cursor       string `json:"cursor,omitempty"`
}

type settingsResourceListRequest struct {
	ResourceType string  `json:"resource_type"`
	WorkspaceID  *string `json:"workspace_id,omitempty"`
	Query        string  `json:"query,omitempty"`
	Limit        int     `json:"limit,omitempty"`
	Cursor       string  `json:"cursor,omitempty"`
}

type settingsRequestError struct {
	Code         string
	Key          string
	FieldPath    string
	ExpectedType string
	Cause        error
}

func (e *settingsRequestError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

func decodeSettingsUpdate(payload []byte) (settingsUpdateRequest, error) {
	if len(payload) > maxSettingsPayloadBytes {
		return settingsUpdateRequest{}, fmt.Errorf("settings update exceeds %d bytes", maxSettingsPayloadBytes)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return settingsUpdateRequest{}, fmt.Errorf("invalid settings update: %w", err)
	}
	for key := range raw {
		if key != "target" && key != "changes" && key != "options" {
			return settingsUpdateRequest{}, fmt.Errorf("unknown settings update field %q", key)
		}
	}
	var request settingsUpdateRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return settingsUpdateRequest{}, fmt.Errorf("invalid settings update: %w", err)
	}
	if len(request.Changes) == 0 {
		return settingsUpdateRequest{}, fmt.Errorf("changes must contain at least one setting")
	}
	if len(request.Changes) > settingscatalog.MaxChangedFields {
		return settingsUpdateRequest{}, fmt.Errorf("changes cannot contain more than %d settings", settingscatalog.MaxChangedFields)
	}
	return request, nil
}

func decodeSettingsRead(payload []byte) (settingsReadRequest, error) {
	var request settingsReadRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return settingsReadRequest{}, fmt.Errorf("invalid settings read: %w", err)
	}
	if len(request.Keys) == 0 || len(request.Keys) > settingscatalog.MaxChangedFields {
		return settingsReadRequest{}, fmt.Errorf("keys must contain between 1 and %d settings", settingscatalog.MaxChangedFields)
	}
	return request, nil
}

func validateSettingsPatch(registry *settingscatalog.Registry, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) error {
	if err := registry.ValidateTarget(target); err != nil {
		return &settingsRequestError{Code: "invalid_target", Cause: err}
	}
	if len(changes) == 0 || len(changes) > settingscatalog.MaxChangedFields {
		return &settingsRequestError{Code: "invalid_changes", Cause: fmt.Errorf("changes must contain between 1 and %d settings", settingscatalog.MaxChangedFields)}
	}
	for key, value := range changes {
		field, _, ok := findTargetField(registry, target.ResourceType, key)
		if !ok {
			return &settingsRequestError{Code: "unknown_setting", Key: key, Cause: fmt.Errorf("unknown setting %q", key)}
		}
		if !field.Writable || field.Support != settingscatalog.SupportSupported {
			return &settingsRequestError{Code: "setting_not_writable", Key: field.Key, FieldPath: field.FieldPath, Cause: fmt.Errorf("setting %q is not writable", key)}
		}
		if err := validateJSONType(value, field.JSONType, field.Nullable); err != nil {
			return &settingsRequestError{Code: "invalid_value_type", Key: field.Key, FieldPath: field.FieldPath, ExpectedType: field.JSONType, Cause: err}
		}
	}
	return nil
}

func findTargetField(registry *settingscatalog.Registry, resourceType, key string) (settingscatalog.FieldDescriptor, settingscatalog.DomainDescriptor, bool) {
	domain, ok := registry.Domain(resourceType)
	if !ok {
		return settingscatalog.FieldDescriptor{}, settingscatalog.DomainDescriptor{}, false
	}
	for _, field := range domain.Fields {
		if field.Key == key || field.FieldPath == key {
			return field, domain, true
		}
	}
	return settingscatalog.FieldDescriptor{}, settingscatalog.DomainDescriptor{}, false
}

func validateJSONType(raw json.RawMessage, expected string, nullable bool) error {
	if string(raw) == jsonNull {
		if nullable {
			return nil
		}
		return errors.New("null is not accepted by this setting contract")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("value is not valid JSON: %w", err)
	}
	valid := false
	switch expected {
	case "string":
		_, valid = value.(string)
	case "boolean":
		_, valid = value.(bool)
	case "number":
		_, valid = value.(float64)
	case "integer":
		if number, ok := value.(float64); ok {
			valid = math.Trunc(number) == number
		}
	case "object":
		_, valid = value.(map[string]any)
	case "array":
		_, valid = value.([]any)
	default:
		valid = true
	}
	if !valid {
		return fmt.Errorf("expected %s", expected)
	}
	return nil
}

func (h *Handlers) SetSettingsCatalog(registry *settingscatalog.Registry) {
	h.settingsRegistry = registry
}

func (h *Handlers) SetSettingsOperations(operations SettingsOperations) {
	h.settingsOperations = operations
}

func (h *Handlers) registerSettingsHandlers(d *guardedMCPDispatcher) {
	d.RegisterFunc(ws.ActionMCPSearchSettings, h.handleSearchSettings)
	d.RegisterFunc(ws.ActionMCPDescribeSetting, h.handleDescribeSetting)
	d.RegisterFunc(ws.ActionMCPGetSettings, h.handleGetSettings)
	d.RegisterFunc(ws.ActionMCPUpdateSettings, h.handleUpdateSettings)
	d.RegisterFunc(ws.ActionMCPListSettingsResources, h.handleListSettingsResources)
}

func (h *Handlers) handleSearchSettings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.settingsRegistry == nil {
		return h.settingsUnavailable(msg)
	}
	var request settingsSearchRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid settings search", nil)
	}
	result, err := h.settingsRegistry.Search(settingscatalog.SearchRequest{
		Query: request.Query, Domain: request.Domain, ResourceType: request.ResourceType, Limit: request.Limit, Cursor: request.Cursor,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func (h *Handlers) handleDescribeSetting(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.settingsRegistry == nil {
		return h.settingsUnavailable(msg)
	}
	var request settingsDescribeRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil || strings.TrimSpace(request.Key) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "key is required", nil)
	}
	field, domain, ok := h.settingsRegistry.Find(request.Key)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "setting was not found", map[string]interface{}{"code": "setting_not_found", "key": request.Key})
	}
	if request.Target != nil {
		if err := h.settingsRegistry.ValidateTarget(*request.Target); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		if request.Target.ResourceType != domain.ResourceType {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "target resource_type does not match setting", nil)
		}
	}
	operation := request.Operation
	if operation == "" {
		operation = "update"
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"key": field.Key, "field_path": field.FieldPath, "domain": domain.Domain, "resource_type": domain.ResourceType,
		"support": field.Support, "classification": field.Classification, "description": field.Description,
		"schema": fieldSchema(field), "target": domain.Target, "operations": domain.Operations,
		"operation": operation, "settings_href": field.SettingsHref, "dependencies": field.Dependencies,
		"default_behavior": field.DefaultBehavior, "change_timing": field.ChangeTiming, "sensitive": field.Sensitive,
		"include_resource_schema": request.IncludeResourceSchema, "context": request.Context,
	})
}

func fieldSchema(field settingscatalog.FieldDescriptor) map[string]interface{} {
	return map[string]interface{}{"type": field.JSONType, "nullable": field.Nullable, "replacement": field.Replacement, "writable": field.Writable}
}

func (h *Handlers) handleGetSettings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.settingsRegistry == nil || h.settingsOperations == nil {
		return h.settingsUnavailable(msg)
	}
	request, err := decodeSettingsRead(msg.Payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	if err := h.settingsRegistry.ValidateTarget(request.Target); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	for _, key := range request.Keys {
		_, _, ok := findTargetField(h.settingsRegistry, request.Target.ResourceType, key)
		if !ok {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "setting was not found", map[string]interface{}{"code": "setting_not_found", "key": key})
		}
	}
	result, err := h.settingsOperations.ReadSettings(ctx, request.Target, request.Keys)
	if err != nil {
		return h.settingsOperationError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func (h *Handlers) handleUpdateSettings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.settingsRegistry == nil || h.settingsOperations == nil {
		return h.settingsUnavailable(msg)
	}
	request, err := decodeSettingsUpdate(msg.Payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	if err := validateSettingsPatch(h.settingsRegistry, request.Target, request.Changes); err != nil {
		return h.settingsOperationError(msg, err)
	}
	result, err := h.settingsOperations.UpdateSettings(ctx, request.Target, request.Changes, request.Options)
	if err != nil {
		return h.settingsOperationError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func (h *Handlers) handleListSettingsResources(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.settingsRegistry == nil || h.settingsOperations == nil {
		return h.settingsUnavailable(msg)
	}
	var request settingsResourceListRequest
	if err := json.Unmarshal(msg.Payload, &request); err != nil || strings.TrimSpace(request.ResourceType) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "resource_type is required", nil)
	}
	domain, ok := h.settingsRegistry.Domain(request.ResourceType)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "resource type was not found", nil)
	}
	if domain.Target.RequiresWorkspaceID && (request.WorkspaceID == nil || strings.TrimSpace(*request.WorkspaceID) == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required for resource lookup", nil)
	}
	if !domain.Target.AllowsWorkspaceID && request.WorkspaceID != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "resource type does not accept workspace_id", nil)
	}
	limit := request.Limit
	if limit == 0 {
		limit = settingscatalog.DefaultSearchLimit
	}
	if limit < 1 || limit > settingscatalog.MaxSearchResults {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "limit must be between 1 and 50", nil)
	}
	result, err := h.settingsOperations.ListSettingsResources(ctx, request.ResourceType, request.WorkspaceID, request.Query, limit, request.Cursor)
	if err != nil {
		return h.settingsOperationError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

func (h *Handlers) settingsUnavailable(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "settings service is unavailable", map[string]interface{}{"code": "settings_unavailable"})
}

func (h *Handlers) settingsOperationError(msg *ws.Message, err error) (*ws.Message, error) {
	var requestErr *settingsRequestError
	if errors.As(err, &requestErr) {
		details := map[string]interface{}{"code": requestErr.Code}
		if requestErr.Key != "" {
			details["key"] = requestErr.Key
		}
		if requestErr.FieldPath != "" {
			details["field_path"] = requestErr.FieldPath
		}
		if requestErr.ExpectedType != "" {
			details["expected_type"] = requestErr.ExpectedType
		}
		details["suggested_tool"] = "describe_setting_kandev"
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, requestErr.Error(), details)
	}
	h.logger.Error("settings operation failed", zap.Error(err), zap.String("action", msg.Action))
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "settings operation failed", nil)
}
