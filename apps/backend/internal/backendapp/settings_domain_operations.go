//nolint:goconst // Resource type identifiers are protocol vocabulary kept readable at dispatch sites.
package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/automation"
	editormodels "github.com/kandev/kandev/internal/editors/models"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/notifications/dto"
	"github.com/kandev/kandev/internal/settingscatalog"
	storage "github.com/kandev/kandev/internal/system/storage"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

//nolint:cyclop,funlen // The central read dispatch preserves one domain-owned adapter boundary.
func (s *settingsOperations) readDomainSettings(ctx context.Context, target settingscatalog.ResourceTarget, keys []string) (any, error) {
	if err := s.requireDomainDependency(target.ResourceType); err != nil {
		return nil, err
	}
	if target.ResourceID == nil && target.ResourceType != "storage_maintenance" {
		return nil, fmt.Errorf("resource_id is required for %q", target.ResourceType)
	}
	if err := s.validateDomainWorkspace(ctx, target); err != nil {
		return nil, err
	}
	var value any
	var err error
	switch target.ResourceType {
	case "workflow":
		value, err = s.deps.task.GetWorkflow(ctx, *target.ResourceID)
	case "workflow_step":
		var response *workflowcontroller.GetStepResponse
		response, err = s.deps.workflowCtrl.GetStep(ctx, *target.ResourceID)
		if response != nil && response.Step != nil {
			value = response.Step
		} else if err == nil {
			err = fmt.Errorf("workflow step lookup returned no step")
		}
	case "workspace":
		value, err = s.deps.task.GetWorkspace(ctx, *target.ResourceID)
	case "repository":
		value, err = s.deps.task.GetRepository(ctx, *target.ResourceID)
	case "repository_script":
		value, err = s.deps.task.GetRepositoryScript(ctx, *target.ResourceID)
	case "repository_set":
		value, err = s.deps.task.GetRepositorySet(ctx, *target.ResourceID)
	case "executor":
		value, err = s.deps.task.GetExecutor(ctx, *target.ResourceID)
	case "executor_profile":
		value, err = s.deps.task.GetExecutorProfile(ctx, *target.ResourceID)
	case "environment":
		value, err = s.deps.task.GetEnvironment(ctx, *target.ResourceID)
	case "task":
		value, err = s.deps.task.GetTask(ctx, *target.ResourceID)
	case "prompt":
		value, err = s.readPrompt(ctx, *target.ResourceID)
	case "utility_agent":
		value, err = s.deps.utility.GetAgentByID(ctx, *target.ResourceID)
	case "editor":
		value, err = s.readEditor(ctx, *target.ResourceID)
	case "notification_provider":
		value, err = s.readNotificationProvider(ctx, *target.ResourceID)
	case "automation":
		value, err = s.deps.automation.GetAutomation(ctx, *target.ResourceID)
	case "automation_trigger":
		value, err = s.deps.automation.GetTrigger(ctx, *target.ResourceID)
	case "runtime_flag":
		value, err = s.readRuntimeFlag(ctx, *target.ResourceID)
	case "storage_maintenance":
		value, err = s.deps.storage.GetSettings(ctx)
	default:
		return nil, fmt.Errorf("settings read is not implemented for resource type %q", target.ResourceType)
	}
	if err != nil {
		return nil, err
	}
	values := projectJSON(value, s.registry, target.ResourceType, keys)
	redactSettingsValues(values, s.registry, target.ResourceType)
	return map[string]any{"target": target, "values": values, "source": target.ResourceType}, nil
}

//nolint:cyclop,funlen // The central update dispatch preserves one domain-owned adapter boundary.
func (s *settingsOperations) updateDomainSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage, options map[string]json.RawMessage) (any, error) {
	if err := s.requireDomainDependency(target.ResourceType); err != nil {
		return nil, err
	}
	if target.ResourceID == nil && target.ResourceType != "storage_maintenance" {
		return nil, fmt.Errorf("resource_id is required for %q", target.ResourceType)
	}
	if err := s.validateDomainWorkspace(ctx, target); err != nil {
		return nil, err
	}
	normalized, err := normalizeChangeMap(changes, target.ResourceType)
	if err != nil {
		return nil, err
	}
	var updated any
	switch target.ResourceType {
	case "workflow":
		var request taskservice.UpdateWorkflowRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateWorkflow(ctx, *target.ResourceID, &request)
	case "workflow_step":
		updated, err = s.updateWorkflowStep(ctx, *target.ResourceID, normalized)
	case "workspace":
		var request taskservice.UpdateWorkspaceRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateWorkspace(ctx, *target.ResourceID, &request)
	case "repository":
		var request taskservice.UpdateRepositoryRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateRepository(ctx, *target.ResourceID, &request)
	case "repository_script":
		var request taskservice.UpdateRepositoryScriptRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateRepositoryScript(ctx, *target.ResourceID, &request)
	case "repository_set":
		var request taskservice.UpdateRepositorySetRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateRepositorySet(ctx, *target.ResourceID, &request)
	case "executor":
		var request taskservice.UpdateExecutorRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateExecutor(ctx, *target.ResourceID, &request)
	case "executor_profile":
		var request taskservice.UpdateExecutorProfileRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateExecutorProfile(ctx, *target.ResourceID, &request)
	case "environment":
		var request taskservice.UpdateEnvironmentRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateEnvironment(ctx, *target.ResourceID, &request)
	case "task":
		var request taskservice.UpdateTaskRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.task.UpdateTask(ctx, *target.ResourceID, &request)
	case "prompt":
		updated, err = s.updatePrompt(ctx, *target.ResourceID, normalized)
	case "utility_agent":
		updated, err = s.updateUtilityAgent(ctx, *target.ResourceID, normalized)
	case "editor":
		updated, err = s.updateEditor(ctx, *target.ResourceID, normalized)
	case "notification_provider":
		updated, err = s.updateNotificationProvider(ctx, *target.ResourceID, normalized)
	case "automation":
		var request automation.UpdateAutomationRequest
		if err := decodePatch(normalized, &request); err != nil {
			return nil, err
		}
		updated, err = s.deps.automation.UpdateAutomation(ctx, *target.ResourceID, &request)
	case "automation_trigger":
		updated, err = s.updateAutomationTrigger(ctx, *target.ResourceID, normalized)
	case "runtime_flag":
		updated, err = s.updateRuntimeFlag(ctx, *target.ResourceID, normalized)
	case "storage_maintenance":
		updated, err = s.updateStorageSettings(ctx, normalized, options)
	default:
		return nil, fmt.Errorf("settings update is not implemented for resource type %q", target.ResourceType)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"target": target, "accepted_fields": changePaths(changes), "settings": s.sanitizeSettingsValue(target.ResourceType, updated), "source": target.ResourceType}, nil
}

func decodePatch(changes map[string]json.RawMessage, target any) error {
	payload, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("invalid settings patch: %w", err)
	}
	return nil
}

func (s *settingsOperations) updateWorkflowStep(ctx context.Context, id string, changes map[string]json.RawMessage) (any, error) {
	request := workflowcontroller.UpdateStepRequest{ID: id}
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	response, err := s.deps.workflowCtrl.UpdateStep(ctx, request)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("workflow step update returned no step")
	}
	return response.Step, nil
}

func (s *settingsOperations) readPrompt(ctx context.Context, id string) (map[string]any, error) {
	prompts, err := s.deps.prompts.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	for _, prompt := range prompts {
		if prompt != nil && prompt.ID == id {
			return map[string]any{
				"id": prompt.ID, "name": prompt.Name, "content": prompt.Content, "builtin": prompt.Builtin,
				"created_at": prompt.CreatedAt, "updated_at": prompt.UpdatedAt,
			}, nil
		}
	}
	return nil, fmt.Errorf("prompt not found")
}

func (s *settingsOperations) updatePrompt(ctx context.Context, id string, changes map[string]json.RawMessage) (any, error) {
	var name, content *string
	if err := decodeOptionalChange(changes, "name", &name); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "content", &content); err != nil {
		return nil, err
	}
	return s.deps.prompts.UpdatePrompt(ctx, id, name, content)
}

func (s *settingsOperations) readEditor(ctx context.Context, id string) (any, error) {
	editors, err := s.deps.editors.ListEditors(ctx)
	if err != nil {
		return nil, err
	}
	for _, editor := range editors {
		if editor != nil && editor.ID == id {
			return editor, nil
		}
	}
	return nil, fmt.Errorf("editor not found")
}

func (s *settingsOperations) updateUtilityAgent(ctx context.Context, id string, changes map[string]json.RawMessage) (any, error) {
	var name, description, prompt, agentID, model, profileID, bindingState *string
	var enabled *bool
	for key, target := range map[string]any{
		"name": &name, "description": &description, "prompt": &prompt, "agent_id": &agentID,
		"model": &model, "agent_profile_id": &profileID, "profile_binding_state": &bindingState,
		"enabled": &enabled,
	} {
		if err := decodeOptionalChange(changes, key, target); err != nil {
			return nil, err
		}
	}
	return s.deps.utility.UpdateAgent(ctx, id, name, description, prompt, agentID, model, profileID, bindingState, enabled)
}

func (s *settingsOperations) updateEditor(ctx context.Context, id string, changes map[string]json.RawMessage) (any, error) {
	var name, kind *string
	var enabled *bool
	var config json.RawMessage
	if err := decodeOptionalChange(changes, "name", &name); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "kind", &kind); err != nil {
		return nil, err
	}
	if err := decodeOptionalChange(changes, "enabled", &enabled); err != nil {
		return nil, err
	}
	if raw, ok := changes["config"]; ok {
		config = append(json.RawMessage(nil), raw...)
	}
	return s.deps.editors.UpdateEditor(ctx, editorservice.UpdateEditorInput{
		EditorID: id, Name: name, Kind: kind, Config: config, Enabled: enabled,
	})
}

func (s *settingsOperations) readNotificationProvider(ctx context.Context, id string) (any, error) {
	response, err := s.deps.notifications.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	for _, provider := range response.Providers {
		if provider.ID == id {
			return provider, nil
		}
	}
	return nil, fmt.Errorf("notification provider not found")
}

func (s *settingsOperations) updateNotificationProvider(ctx context.Context, id string, changes map[string]json.RawMessage) (any, error) {
	var request dto.UpdateProviderRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	return s.deps.notifications.UpdateProvider(ctx, id, request)
}

func (s *settingsOperations) readRuntimeFlag(ctx context.Context, key string) (any, error) {
	states, err := s.deps.runtimeFlags.ListStates(ctx)
	if err != nil {
		return nil, err
	}
	for _, state := range states {
		if state.Key == key {
			return runtimeFlagSettingsValue(state), nil
		}
	}
	return nil, fmt.Errorf("runtime flag not found")
}

func (s *settingsOperations) updateRuntimeFlag(ctx context.Context, key string, changes map[string]json.RawMessage) (any, error) {
	raw, ok := changes["override"]
	if !ok {
		return nil, fmt.Errorf("override is required")
	}
	var value *bool
	if string(raw) != "null" {
		var parsed bool
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("override must be boolean or null")
		}
		value = &parsed
	}
	states, err := s.deps.runtimeFlags.SetOverride(ctx, key, value)
	if err != nil {
		return nil, err
	}
	for _, state := range states {
		if state.Key == key {
			return map[string]any{
				"accepted_fields": []string{"override"},
				"settings":        runtimeFlagSettingsValue(state),
				"source":          "runtime",
			}, nil
		}
	}
	return nil, fmt.Errorf("runtime flag not found after update")
}

func runtimeFlagSettingsValue(value any) map[string]any {
	payload := map[string]any{}
	encoded, err := json.Marshal(value)
	if err != nil || json.Unmarshal(encoded, &payload) != nil {
		return map[string]any{}
	}
	override, exists := payload["override_value"]
	if !exists {
		override = nil
	}
	payload["override"] = override
	delete(payload, "override_value")
	return payload
}

func (s *settingsOperations) updateAutomationTrigger(ctx context.Context, id string, changes map[string]json.RawMessage) (any, error) {
	var request automation.UpdateTriggerRequest
	if err := decodePatch(changes, &request); err != nil {
		return nil, err
	}
	if err := s.deps.automation.UpdateTrigger(ctx, id, &request); err != nil {
		return nil, err
	}
	return s.deps.automation.GetTrigger(ctx, id)
}

func (s *settingsOperations) requireDomainDependency(resourceType string) error {
	var available bool
	switch resourceType {
	case "workflow":
		available = s.deps.task != nil
	case "workflow_step":
		available = s.deps.workflow != nil && s.deps.workflowCtrl != nil
	case "workspace", "repository", "repository_script", "repository_set", "executor", "executor_profile", "environment", "task":
		available = s.deps.task != nil
	case "prompt":
		available = s.deps.prompts != nil
	case "utility_agent":
		available = s.deps.utility != nil
	case "editor":
		available = s.deps.editors != nil
	case "notification_provider":
		available = s.deps.notifications != nil
	case "automation", "automation_trigger":
		available = s.deps.automation != nil
	case "runtime_flag":
		available = s.deps.runtimeFlags != nil
	case "storage_maintenance":
		available = s.deps.storage != nil
	default:
		return fmt.Errorf("settings dependency is not defined for resource type %q", resourceType)
	}
	if !available {
		return fmt.Errorf("settings dependency for %q is unavailable", resourceType)
	}
	return nil
}

//nolint:cyclop,gocognit,funlen // Each resource type resolves its owning workspace through its domain service.
func (s *settingsOperations) validateDomainWorkspace(ctx context.Context, target settingscatalog.ResourceTarget) error {
	if target.WorkspaceID == nil || *target.WorkspaceID == "" {
		return nil
	}
	if target.ResourceID == nil {
		return fmt.Errorf("resource_id is required for %q", target.ResourceType)
	}
	workspaceID := ""
	switch target.ResourceType {
	case "workspace":
		workspaceID = *target.ResourceID
	case "workflow":
		item, err := s.deps.task.GetWorkflow(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		workspaceID = item.WorkspaceID
	case "workflow_step":
		response, err := s.deps.workflowCtrl.GetStep(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		if response == nil || response.Step == nil {
			return fmt.Errorf("workflow step lookup returned no step")
		}
		workflow, err := s.deps.task.GetWorkflow(ctx, response.Step.WorkflowID)
		if err != nil {
			return err
		}
		workspaceID = workflow.WorkspaceID
	case "repository":
		item, err := s.deps.task.GetRepository(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		workspaceID = item.WorkspaceID
	case "repository_script":
		item, err := s.deps.task.GetRepositoryScript(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		repository, err := s.deps.task.GetRepository(ctx, item.RepositoryID)
		if err != nil {
			return err
		}
		workspaceID = repository.WorkspaceID
	case "repository_set":
		item, err := s.deps.task.GetRepositorySet(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		workspaceID = item.WorkspaceID
	case "task":
		item, err := s.deps.task.GetTask(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		workspaceID = item.WorkspaceID
	case "automation":
		item, err := s.deps.automation.GetAutomation(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		workspaceID = item.WorkspaceID
	case "automation_trigger":
		trigger, err := s.deps.automation.GetTrigger(ctx, *target.ResourceID)
		if err != nil {
			return err
		}
		automation, err := s.deps.automation.GetAutomation(ctx, trigger.AutomationID)
		if err != nil {
			return err
		}
		workspaceID = automation.WorkspaceID
	default:
		return fmt.Errorf("resource type %q does not support workspace targeting", target.ResourceType)
	}
	if workspaceID != *target.WorkspaceID {
		return fmt.Errorf("resource %q is not in workspace %q", *target.ResourceID, *target.WorkspaceID)
	}
	return nil
}

//nolint:cyclop // Complete-document storage updates keep validation and confirmation atomic.
func (s *settingsOperations) updateStorageSettings(ctx context.Context, changes map[string]json.RawMessage, options map[string]json.RawMessage) (any, error) {
	current, err := s.deps.storage.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, err
	}
	for key, raw := range changes {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("invalid storage setting %q: %w", key, err)
		}
		if err := setNestedJSONValue(document, key, value); err != nil {
			return nil, err
		}
	}
	nextPayload, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	var next storage.StorageMaintenanceSettings
	if err := json.Unmarshal(nextPayload, &next); err != nil {
		return nil, fmt.Errorf("invalid storage settings: %w", err)
	}
	dedicatedDocker, adoptGoCache := false, false
	if raw, ok := options["confirm_dedicated_docker"]; ok {
		if err := json.Unmarshal(raw, &dedicatedDocker); err != nil {
			return nil, fmt.Errorf("confirm_dedicated_docker must be boolean")
		}
	}
	if raw, ok := options["adopt_go_cache"]; ok {
		if err := json.Unmarshal(raw, &adoptGoCache); err != nil {
			return nil, fmt.Errorf("adopt_go_cache must be boolean")
		}
	}
	for key := range options {
		if key != "confirm_dedicated_docker" && key != "adopt_go_cache" {
			return nil, fmt.Errorf("unsupported storage option %q", key)
		}
	}
	return s.deps.storage.SaveSettingsWithConfirmations(ctx, next, storage.NewSaveConfirmations(dedicatedDocker, adoptGoCache))
}

func setNestedJSONValue(document map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("storage setting path %q is invalid", path)
	}
	current := document
	for _, part := range parts[:len(parts)-1] {
		nested, ok := current[part].(map[string]any)
		if !ok {
			return fmt.Errorf("storage setting path %q is invalid", path)
		}
		current = nested
	}
	current[parts[len(parts)-1]] = value
	return nil
}

func decodeOptionalChange(changes map[string]json.RawMessage, key string, target any) error {
	raw, ok := changes[key]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid %s setting: %w", key, err)
	}
	return nil
}

//nolint:cyclop,gocognit,funlen,maintidx // Resource lookup intentionally keeps each domain's list and label semantics together.
func (s *settingsOperations) listDomainSettingsResources(ctx context.Context, resourceType string, workspaceID *string, query string, limit int, cursor string) (any, error) {
	if err := s.requireDomainDependency(resourceType); err != nil {
		return nil, err
	}
	resources := make([]map[string]any, 0)
	needle := strings.ToLower(strings.TrimSpace(query))
	add := func(id, label, wsID string, extra map[string]any) {
		if id == "" || (workspaceID != nil && strings.TrimSpace(*workspaceID) != "" && wsID != strings.TrimSpace(*workspaceID)) {
			return
		}
		if needle != "" && !strings.Contains(strings.ToLower(id), needle) && !strings.Contains(strings.ToLower(label), needle) {
			return
		}
		item := map[string]any{
			"target": settingscatalog.ResourceTarget{
				ResourceType: resourceType,
				ResourceID:   settingsStringPointer(id),
				WorkspaceID:  nonEmptyPointer(wsID),
			},
			"label": label,
		}
		for key, value := range extra {
			item[key] = value
		}
		resources = append(resources, item)
	}
	var err error
	switch resourceType {
	case "workflow":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for workflow lookup")
		}
		items, listErr := s.deps.task.ListWorkflows(ctx, *workspaceID, false)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, item.WorkspaceID, nil)
			}
		}
	case "workflow_step":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for workflow step lookup")
		}
		items, listErr := s.deps.workflow.ListStepsByWorkspaceID(ctx, *workspaceID)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, *workspaceID, map[string]any{"workflow_id": item.WorkflowID})
			}
		}
	case "workspace":
		items, listErr := s.deps.task.ListWorkspaces(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, item.ID, nil)
			}
		}
	case "repository":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for repository lookup")
		}
		items, listErr := s.deps.task.ListRepositories(ctx, *workspaceID)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, item.WorkspaceID, nil)
			}
		}
	case "repository_script":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for repository script lookup")
		}
		repositories, listErr := s.deps.task.ListRepositories(ctx, *workspaceID)
		err = listErr
		for _, repository := range repositories {
			if repository == nil {
				continue
			}
			items, scriptErr := s.deps.task.ListRepositoryScripts(ctx, repository.ID)
			if scriptErr != nil {
				err = scriptErr
				break
			}
			for _, item := range items {
				if item != nil {
					add(item.ID, item.Name, *workspaceID, map[string]any{"repository_id": item.RepositoryID})
				}
			}
		}
	case "repository_set":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for repository set lookup")
		}
		items, listErr := s.deps.task.ListRepositorySets(ctx, *workspaceID)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, item.WorkspaceID, nil)
			}
		}
	case "executor":
		items, listErr := s.deps.task.ListExecutors(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, "", map[string]any{"type": item.Type})
			}
		}
	case "executor_profile":
		items, listErr := s.deps.task.ListAllExecutorProfiles(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, "", map[string]any{"executor_id": item.ExecutorID})
			}
		}
	case "environment":
		items, listErr := s.deps.task.ListEnvironments(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, "", map[string]any{"kind": item.Kind})
			}
		}
	case "prompt":
		items, listErr := s.deps.prompts.ListPrompts(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, "", nil)
			}
		}
	case "utility_agent":
		items, listErr := s.deps.utility.ListAgents(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, "", nil)
			}
		}
	case "editor":
		items, listErr := s.deps.editors.ListEditors(ctx)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, "", nil)
			}
		}
	case "notification_provider":
		response, listErr := s.deps.notifications.ListProviders(ctx)
		err = listErr
		for _, item := range response.Providers {
			add(item.ID, item.Name, "", map[string]any{"type": item.Type})
		}
	case "automation":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for automation lookup")
		}
		items, listErr := s.deps.automation.ListAutomations(ctx, *workspaceID)
		err = listErr
		for _, item := range items {
			if item != nil {
				add(item.ID, item.Name, item.WorkspaceID, nil)
			}
		}
	case "automation_trigger":
		if workspaceID == nil || *workspaceID == "" {
			return nil, fmt.Errorf("workspace_id is required for automation trigger lookup")
		}
		automations, listErr := s.deps.automation.ListAutomations(ctx, *workspaceID)
		err = listErr
		for _, item := range automations {
			if item == nil {
				continue
			}
			for _, trigger := range item.Triggers {
				add(trigger.ID, fmt.Sprintf("%s (%s)", item.Name, trigger.Type), item.WorkspaceID, map[string]any{
					"automation_id": item.ID,
					"type":          trigger.Type,
				})
			}
		}
	case "runtime_flag":
		items, listErr := s.deps.runtimeFlags.ListStates(ctx)
		err = listErr
		for _, item := range items {
			add(item.Key, item.Label, "", map[string]any{"env_locked": item.EnvLocked})
		}
	default:
		return nil, fmt.Errorf("resource lookup is not implemented for resource type %q", resourceType)
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(resources, func(i, j int) bool {
		return fmt.Sprint(resources[i]["label"]) < fmt.Sprint(resources[j]["label"])
	})
	return pageSettingsResources(resources, limit, cursor), nil
}

func pageSettingsResources(resources []map[string]any, limit int, cursor string) map[string]any {
	start := parseSettingsCursor(cursor)
	if start > len(resources) {
		start = len(resources)
	}
	end := start + limit
	if end > len(resources) {
		end = len(resources)
	}
	result := map[string]any{"resources": resources[start:end], "total": len(resources)}
	if end < len(resources) {
		result["next_cursor"] = strconv.Itoa(end)
	}
	return result
}

var _ = editormodels.Editor{}
var _ = taskmodels.Task{}
var _ = utilitymodels.UtilityAgent{}
var _ = workflowmodels.WorkflowStep{}
