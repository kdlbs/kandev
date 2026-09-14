//revive:disable:file-length-limit // This adapter owns the complete shared settings boundary.
//nolint:goconst // Settings protocol keys stay readable at dispatch sites.
package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/settingscatalog"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userdto "github.com/kandev/kandev/internal/user/dto"
	userservice "github.com/kandev/kandev/internal/user/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type settingsOperations struct {
	registry    *settingscatalog.Registry
	profiles    *agentsettingscontroller.Controller
	user        *usercontroller.Controller
	userService *userservice.Service
	deps        settingsDomainDependencies
}

type settingsBroadcaster interface {
	Broadcast(*ws.Message)
}

type settingsDomainDependencies struct {
	broadcaster   settingsBroadcaster
	authEnabled   func() bool
	task          settingsTaskService
	workflow      settingsWorkflowService
	workflowCtrl  settingsWorkflowController
	prompts       settingsPromptService
	utility       settingsUtilityService
	editors       settingsEditorService
	notifications settingsNotificationController
	runtimeFlags  settingsRuntimeFlagsService
	storage       settingsStorageService
	automation    settingsAutomationService
	agent         settingsAgentService
	jira          settingsJiraService
	linear        settingsLinearService
	sentry        settingsSentryService
	github        settingsGitHubService
	gitlab        settingsGitLabService
	azureDevOps   settingsAzureDevOpsService
}

func newSettingsOperations(
	registry *settingscatalog.Registry,
	profiles *agentsettingscontroller.Controller,
	userService *userservice.Service,
	deps settingsDomainDependencies,
) mcphandlers.SettingsOperations {
	var user *usercontroller.Controller
	if userService != nil {
		user = usercontroller.NewController(userService)
	}
	return &settingsOperations{registry: registry, profiles: profiles, user: user, userService: userService, deps: deps}
}

//nolint:cyclop,gocognit // Authorization combines caller mode, field authority, and workspace ownership rules.
func (s *settingsOperations) authorizeSettingsMutation(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		if s.deps.authEnabled != nil && !s.deps.authEnabled() {
			return nil
		}
		return fmt.Errorf("settings mutation requires a caller identity")
	}
	if identity.Synthetic {
		return nil
	}
	if isIntegrationResource(target.ResourceType) {
		if err := s.requireIntegrationDependency(target.ResourceType); err != nil {
			return err
		}
	} else {
		switch target.ResourceType {
		case "agent", "agent_profile", "agent_profile_mcp", "user_settings":
		default:
			if err := s.requireDomainDependency(target.ResourceType); err != nil {
				return err
			}
		}
	}

	for key := range changes {
		field, ok := settingsField(s.registry, target.ResourceType, key)
		if !ok {
			return fmt.Errorf("unknown setting %q", key)
		}
		if field.Authority == "user.self" {
			continue
		}
		scope, ok := settingsAuthorityScope(field.Authority)
		if !ok {
			return fmt.Errorf("setting %q has no authorization binding", field.Key)
		}
		if scope == authz.ScopeOrgConfigManage || scope == authz.ScopeOrgSettingsManage {
			subject := authz.Subject{
				UserID: identity.UserID, OrgID: identity.OrgID,
				OrgRole: authz.NormalizeOrgRole(string(identity.Role)),
			}
			if !authz.SubjectOrgScopes(subject).Has(scope) {
				return fmt.Errorf("caller lacks %s", scope)
			}
			continue
		}
		if s.deps.task == nil {
			return fmt.Errorf("workspace authorization is unavailable")
		}
		workspaceID, err := s.settingsWorkspaceID(ctx, target)
		if err != nil {
			return err
		}
		if err := s.deps.task.AuthorizeWorkspaceScope(ctx, workspaceID, scope); err != nil {
			return err
		}
	}
	return nil
}

func settingsAuthorityScope(authority string) (authz.Scope, bool) {
	switch authority {
	case "org.config.manage", "execution.manage", "prompt.manage", "utility.manage", "editor.manage", "notification.manage":
		return authz.ScopeOrgConfigManage, true
	case "org.settings.manage":
		return authz.ScopeOrgSettingsManage, true
	case "workspace.manage":
		return authz.ScopeWorkspaceManage, true
	case "repository.manage":
		return authz.ScopeRepositoryManage, true
	case "task.manage", "task.write":
		return authz.ScopeTaskWrite, true
	default:
		return "", false
	}
}

//nolint:cyclop,gocognit,funlen // Resource-to-workspace resolution must remain adjacent to the authorization boundary.
func (s *settingsOperations) settingsWorkspaceID(ctx context.Context, target settingscatalog.ResourceTarget) (string, error) {
	if target.WorkspaceID != nil && strings.TrimSpace(*target.WorkspaceID) != "" {
		return strings.TrimSpace(*target.WorkspaceID), nil
	}
	if target.ResourceID == nil || strings.TrimSpace(*target.ResourceID) == "" {
		return "", fmt.Errorf("resource_id is required for workspace authorization")
	}
	id := strings.TrimSpace(*target.ResourceID)
	switch target.ResourceType {
	case "workspace":
		return id, nil
	case "workflow":
		item, err := s.deps.task.GetWorkflow(ctx, id)
		if err != nil {
			return "", err
		}
		if item == nil {
			return "", fmt.Errorf("workflow not found")
		}
		return item.WorkspaceID, nil
	case "workflow_step":
		response, err := s.deps.workflowCtrl.GetStep(ctx, id)
		if err != nil {
			return "", err
		}
		if response == nil || response.Step == nil {
			return "", fmt.Errorf("workflow step not found")
		}
		workflow, err := s.deps.task.GetWorkflow(ctx, response.Step.WorkflowID)
		if err != nil {
			return "", err
		}
		if workflow == nil {
			return "", fmt.Errorf("workflow not found")
		}
		return workflow.WorkspaceID, nil
	case "repository":
		item, err := s.deps.task.GetRepository(ctx, id)
		if err != nil {
			return "", err
		}
		if item == nil {
			return "", fmt.Errorf("repository not found")
		}
		return item.WorkspaceID, nil
	case "repository_script":
		item, err := s.deps.task.GetRepositoryScript(ctx, id)
		if err != nil {
			return "", err
		}
		if item == nil {
			return "", fmt.Errorf("repository script not found")
		}
		repository, err := s.deps.task.GetRepository(ctx, item.RepositoryID)
		if err != nil {
			return "", err
		}
		if repository == nil {
			return "", fmt.Errorf("repository not found")
		}
		return repository.WorkspaceID, nil
	case "repository_set":
		item, err := s.deps.task.GetRepositorySet(ctx, id)
		if err != nil {
			return "", err
		}
		if item == nil {
			return "", fmt.Errorf("repository set not found")
		}
		return item.WorkspaceID, nil
	case "task":
		item, err := s.deps.task.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if item == nil {
			return "", fmt.Errorf("task not found")
		}
		return item.WorkspaceID, nil
	case "automation":
		item, err := s.deps.automation.GetAutomation(ctx, id)
		if err != nil {
			return "", err
		}
		if item == nil {
			return "", fmt.Errorf("automation not found")
		}
		return item.WorkspaceID, nil
	case "automation_trigger":
		trigger, err := s.deps.automation.GetTrigger(ctx, id)
		if err != nil {
			return "", err
		}
		if trigger == nil {
			return "", fmt.Errorf("automation trigger not found")
		}
		automation, err := s.deps.automation.GetAutomation(ctx, trigger.AutomationID)
		if err != nil {
			return "", err
		}
		if automation == nil {
			return "", fmt.Errorf("automation not found")
		}
		return automation.WorkspaceID, nil
	case "agent":
		agent, err := s.profiles.GetAgent(ctx, id)
		if err != nil {
			return "", err
		}
		return agentWorkspaceID(*agent), nil
	case "agent_profile":
		profile, err := s.findProfile(ctx, target)
		if err != nil {
			return "", err
		}
		return profile.WorkspaceID, nil
	default:
		return "", fmt.Errorf("resource type %q does not have a workspace scope", target.ResourceType)
	}
}

func (s *settingsOperations) ReadSettings(ctx context.Context, target settingscatalog.ResourceTarget, keys []string) (any, error) {
	if isIntegrationResource(target.ResourceType) {
		return s.readIntegrationSettings(ctx, target, keys)
	}
	switch target.ResourceType {
	case "agent":
		return s.readAgentSettings(ctx, target, keys)
	case "agent_profile":
		profile, err := s.findProfile(ctx, target)
		if err != nil {
			return nil, err
		}
		values := projectJSON(profile, s.registry, target.ResourceType, keys)
		redactSettingsValues(values, s.registry, target.ResourceType)
		return map[string]any{
			"target": target, "values": values, "source": "profile", "updated_at": profile.UpdatedAt,
		}, nil
	case "agent_profile_mcp":
		if s.profiles == nil || target.ResourceID == nil {
			return nil, fmt.Errorf("profile MCP settings are unavailable")
		}
		config, err := s.profiles.GetAgentProfileMcpConfig(ctx, *target.ResourceID)
		if err != nil {
			return nil, err
		}
		values := projectJSON(config, s.registry, target.ResourceType, keys)
		redactSettingsValues(values, s.registry, target.ResourceType)
		return map[string]any{"target": target, "values": values, "source": "profile_mcp"}, nil
	case "user_settings":
		if s.user == nil {
			return nil, fmt.Errorf("user settings are unavailable")
		}
		response, err := s.user.GetUserSettings(ctx)
		if err != nil {
			return nil, err
		}
		values := projectJSON(response.Settings, s.registry, target.ResourceType, keys)
		return map[string]any{"target": target, "values": values, "source": "user_settings", "updated_at": response.Settings.UpdatedAt}, nil
	default:
		return s.readDomainSettings(ctx, target, keys)
	}
}

func (s *settingsOperations) DescribeSetting(ctx context.Context, request mcphandlers.SettingDescriptionRequest) (mcphandlers.SettingDescription, error) {
	field, domain, ok := s.registry.Find(request.Key)
	if !ok {
		return mcphandlers.SettingDescription{}, fmt.Errorf("setting %q was not found", request.Key)
	}
	field, err := s.targetDescriptionField(field, domain, request.Target)
	if err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	if len(request.Context) == 0 {
		return mcphandlers.SettingDescription{}, nil
	}
	if err := validateDescriptionContext(request.Key, field, request.Target, request.Context); err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	return s.describeProfileOptions(ctx, request)
}

func (s *settingsOperations) targetDescriptionField(
	field settingscatalog.FieldDescriptor,
	domain settingscatalog.DomainDescriptor,
	target *settingscatalog.ResourceTarget,
) (settingscatalog.FieldDescriptor, error) {
	if target == nil {
		return field, nil
	}
	if err := s.registry.ValidateTarget(*target); err != nil {
		return settingscatalog.FieldDescriptor{}, err
	}
	if target.ResourceType != domain.ResourceType {
		return settingscatalog.FieldDescriptor{}, fmt.Errorf("target resource_type does not match setting")
	}
	return settingscatalog.FieldForTarget(field, target), nil
}

func validateDescriptionContext(
	key string,
	field settingscatalog.FieldDescriptor,
	target *settingscatalog.ResourceTarget,
	contextValues map[string]any,
) error {
	if field.Key != "agent_profile.config_options" || target == nil || target.ResourceID == nil {
		return fmt.Errorf("setting context is not supported for %q", key)
	}
	for contextKey := range contextValues {
		switch contextKey {
		case "model", "mode", "config_options", "refresh":
		default:
			return fmt.Errorf("unsupported setting context %q", contextKey)
		}
	}
	return nil
}

func (s *settingsOperations) describeProfileOptions(
	ctx context.Context,
	request mcphandlers.SettingDescriptionRequest,
) (mcphandlers.SettingDescription, error) {
	profile, err := s.findProfile(ctx, *request.Target)
	if err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	agentName, err := s.profileAgentName(ctx, profile.ID)
	if err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	refresh, err := descriptionBoolContext(request.Context, "refresh")
	if err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	caps, err := s.profiles.FetchDynamicModels(ctx, agentName, refresh)
	if err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	choices := make([]map[string]any, 0, len(caps.Models)+len(caps.Modes))
	for _, model := range caps.Models {
		choices = append(choices, map[string]any{"kind": "model", "id": model.ID, "label": model.Name, "description": model.Description})
	}
	for _, mode := range caps.Modes {
		choices = append(choices, map[string]any{"kind": "mode", "id": mode.ID, "label": mode.Name, "description": mode.Description})
	}
	model, err := descriptionModelContext(request.Context, profile.Model)
	if err != nil {
		return mcphandlers.SettingDescription{}, err
	}
	configOptions := []agentsettingsdto.ConfigOptionDTO{}
	if model != "" {
		mode, err := descriptionStringContext(request.Context, "mode")
		if err != nil {
			return mcphandlers.SettingDescription{}, err
		}
		selectedOptions, err := descriptionConfigOptionsContext(request.Context, "config_options")
		if err != nil {
			return mcphandlers.SettingDescription{}, err
		}
		resolution, err := s.profiles.ResolveAgentModelConfig(ctx, agentName, agentsettingsdto.ResolveAgentModelConfigRequest{
			Model: model, Mode: mode, ConfigOptions: selectedOptions, Refresh: refresh,
		})
		if err != nil {
			return mcphandlers.SettingDescription{}, err
		}
		configOptions = resolution.ConfigOptions
	}
	for _, option := range configOptions {
		item := map[string]any{
			"kind": "config_option", "id": option.ID, "label": option.Name,
			"type": option.Type, "current_value": option.CurrentValue, "description": option.Description,
		}
		if len(option.Options) > 0 {
			values := make([]map[string]any, 0, len(option.Options))
			for _, value := range option.Options {
				values = append(values, map[string]any{"value": value.Value, "label": value.Name, "description": value.Description})
			}
			item["options"] = values
		}
		choices = append(choices, item)
	}
	return mcphandlers.SettingDescription{Choices: choices}, nil
}

func descriptionModelContext(contextValues map[string]any, fallback string) (string, error) {
	value, ok := contextValues["model"]
	if !ok {
		return fallback, nil
	}
	model, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("setting context %q must be a string", "model")
	}
	return model, nil
}

func descriptionStringContext(contextValues map[string]any, key string) (string, error) {
	value, ok := contextValues[key]
	if !ok {
		return "", nil
	}
	parsed, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("setting context %q must be a string", key)
	}
	return parsed, nil
}

func descriptionConfigOptionsContext(contextValues map[string]any, key string) (map[string]string, error) {
	value, ok := contextValues[key]
	if !ok {
		return nil, nil
	}
	entries, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("setting context %q must be an object", key)
	}
	result := make(map[string]string, len(entries))
	for entryKey, entryValue := range entries {
		parsed, ok := entryValue.(string)
		if !ok {
			return nil, fmt.Errorf("setting context %q values must be strings", key)
		}
		result[entryKey] = parsed
	}
	return result, nil
}

func descriptionBoolContext(contextValues map[string]any, key string) (bool, error) {
	value, ok := contextValues[key]
	if !ok {
		return false, nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("setting context %q must be boolean", key)
	}
	return parsed, nil
}

func (s *settingsOperations) profileAgentName(ctx context.Context, profileID string) (string, error) {
	response, err := s.profiles.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	for _, agent := range response.Agents {
		for _, profile := range agent.Profiles {
			if profile.ID == profileID {
				return agent.Name, nil
			}
		}
	}
	return "", fmt.Errorf("agent profile %q is not visible", profileID)
}

func (s *settingsOperations) UpdateSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage, options map[string]json.RawMessage) (any, error) {
	if err := s.authorizeSettingsMutation(ctx, target, changes); err != nil {
		return nil, err
	}
	if isIntegrationResource(target.ResourceType) {
		return s.updateIntegrationSettings(ctx, target, changes, options)
	}
	switch target.ResourceType {
	case "agent":
		return s.updateAgentSettings(ctx, target, changes)
	case "agent_profile":
		if s.profiles == nil || target.ResourceID == nil {
			return nil, fmt.Errorf("profile settings are unavailable")
		}
		request, err := decodeProfileUpdate(*target.ResourceID, changes, options)
		if err != nil {
			return nil, err
		}
		profile, err := s.profiles.UpdateProfile(ctx, agentsettingscontroller.UpdateProfileRequestFromDTO(request))
		if err != nil {
			return nil, err
		}
		return map[string]any{"target": target, "accepted_fields": changePaths(changes), "profile": s.sanitizeSettingsValue(target.ResourceType, profile), "source": "profile", "updated_at": profile.UpdatedAt}, nil
	case "agent_profile_mcp":
		return s.updateProfileMCP(ctx, target, changes)
	case "user_settings":
		if s.user == nil {
			return nil, fmt.Errorf("user settings are unavailable")
		}
		request, err := decodeUserSettingsUpdate(changes)
		if err != nil {
			return nil, err
		}
		response, err := s.user.UpdateUserSettings(ctx, request)
		if err != nil {
			return nil, err
		}
		return map[string]any{"target": target, "accepted_fields": changePaths(changes), "settings": s.sanitizeSettingsValue(target.ResourceType, response.Settings), "source": "user_settings", "updated_at": response.Settings.UpdatedAt}, nil
	default:
		return s.updateDomainSettings(ctx, target, changes, options)
	}
}

//nolint:cyclop,gocognit // This public boundary selects personal, profile, integration, and domain resource lookup.
func (s *settingsOperations) ListSettingsResources(ctx context.Context, resourceType string, workspaceID *string, query string, limit int, cursor string) (any, error) {
	if isIntegrationResource(resourceType) {
		return s.listIntegrationSettingsResources(ctx, resourceType, workspaceID, query, limit, cursor)
	}
	if resourceType == "user_settings" {
		return map[string]any{"resources": []any{map[string]any{"target": settingscatalog.ResourceTarget{ResourceType: resourceType}, "label": "Current user"}}, "total": 1}, nil
	}
	if resourceType == "agent" && s.profiles != nil {
		response, err := s.profiles.ListAgents(ctx)
		if err != nil {
			return nil, err
		}
		resources := make([]map[string]any, 0, len(response.Agents))
		needle := strings.ToLower(strings.TrimSpace(query))
		for _, agent := range response.Agents {
			if workspaceID != nil && *workspaceID != "" && (agent.WorkspaceID == nil || *agent.WorkspaceID != *workspaceID) {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(agent.ID), needle) && !strings.Contains(strings.ToLower(agent.Name), needle) {
				continue
			}
			resources = append(resources, map[string]any{
				"target": settingscatalog.ResourceTarget{ResourceType: resourceType, ResourceID: settingsStringPointer(agent.ID), WorkspaceID: nonEmptyPointer(agentWorkspaceID(agent))},
				"label":  agent.Name,
			})
		}
		sort.Slice(resources, func(i, j int) bool { return resources[i]["label"].(string) < resources[j]["label"].(string) })
		return pageSettingsResources(resources, limit, cursor), nil
	}
	if resourceType != "agent_profile" || s.profiles == nil {
		return s.listDomainSettingsResources(ctx, resourceType, workspaceID, query, limit, cursor)
	}
	response, err := s.profiles.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	resources := make([]map[string]any, 0)
	needle := strings.ToLower(strings.TrimSpace(query))
	for _, agent := range response.Agents {
		for _, profile := range agent.Profiles {
			if workspaceID != nil && *workspaceID != "" && profile.WorkspaceID != *workspaceID {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(profile.ID), needle) && !strings.Contains(strings.ToLower(profile.Name), needle) {
				continue
			}
			resources = append(resources, map[string]any{
				"target": settingscatalog.ResourceTarget{ResourceType: resourceType, ResourceID: settingsStringPointer(profile.ID), WorkspaceID: nonEmptyPointer(profile.WorkspaceID)},
				"label":  profile.Name, "agent_id": profile.AgentID, "agent_name": agent.Name,
			})
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i]["label"].(string) < resources[j]["label"].(string)
	})
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
		result["next_cursor"] = fmt.Sprintf("%d", end)
	}
	return result, nil
}

func agentWorkspaceID(agent agentsettingsdto.AgentDTO) string {
	if agent.WorkspaceID == nil {
		return ""
	}
	return *agent.WorkspaceID
}

func (s *settingsOperations) readAgentSettings(ctx context.Context, target settingscatalog.ResourceTarget, keys []string) (any, error) {
	if s.profiles == nil || target.ResourceID == nil {
		return nil, fmt.Errorf("agent settings are unavailable")
	}
	agent, err := s.profiles.GetAgent(ctx, *target.ResourceID)
	if err != nil {
		return nil, err
	}
	if target.WorkspaceID != nil && (agent.WorkspaceID == nil || *agent.WorkspaceID != *target.WorkspaceID) {
		return nil, fmt.Errorf("agent is not visible in the requested workspace")
	}
	values := projectJSON(agent, s.registry, target.ResourceType, keys)
	redactSettingsValues(values, s.registry, target.ResourceType)
	redactAgentProfileCollections(values)
	return map[string]any{"target": target, "values": values, "source": "agent", "updated_at": agent.UpdatedAt}, nil
}

func (s *settingsOperations) updateAgentSettings(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	if s.profiles == nil || target.ResourceID == nil {
		return nil, fmt.Errorf("agent settings are unavailable")
	}
	normalized, err := normalizeChangeMap(changes, "agent")
	if err != nil {
		return nil, err
	}
	request := agentsettingscontroller.UpdateAgentRequest{ID: *target.ResourceID}
	for key, raw := range normalized {
		switch key {
		case "supports_mcp":
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("supports_mcp must be boolean")
			}
			request.SupportsMCP = &value
		case "mcp_config_path":
			request.MCPConfigPathSet = true
			var value *string
			if string(raw) != "null" {
				var decoded string
				if err := json.Unmarshal(raw, &decoded); err != nil {
					return nil, fmt.Errorf("mcp_config_path must be string or null")
				}
				value = &decoded
			}
			request.MCPConfigPath = value
		default:
			return nil, fmt.Errorf("setting %q is not writable", key)
		}
	}
	updated, err := s.profiles.UpdateAgent(ctx, request)
	if err != nil {
		return nil, err
	}
	return map[string]any{"target": target, "accepted_fields": changePaths(changes), "agent": s.sanitizeSettingsValue(target.ResourceType, updated), "source": "agent", "updated_at": updated.UpdatedAt}, nil
}

func (s *settingsOperations) findProfile(ctx context.Context, target settingscatalog.ResourceTarget) (*agentsettingsdto.AgentProfileDTO, error) {
	if s.profiles == nil || target.ResourceID == nil {
		return nil, fmt.Errorf("profile settings are unavailable")
	}
	response, err := s.profiles.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, agent := range response.Agents {
		for index := range agent.Profiles {
			profile := &agent.Profiles[index]
			if profile.ID == *target.ResourceID {
				if target.WorkspaceID != nil && profile.WorkspaceID != *target.WorkspaceID {
					return nil, fmt.Errorf("profile is not visible in the requested workspace")
				}
				return profile, nil
			}
		}
	}
	return nil, fmt.Errorf("agent profile not found")
}

func decodeProfileUpdate(profileID string, changes map[string]json.RawMessage, options map[string]json.RawMessage) (agentsettingsdto.ProfileUpdateRequest, error) {
	normalized, err := normalizeChangeMap(changes, "agent_profile")
	if err != nil {
		return agentsettingsdto.ProfileUpdateRequest{}, err
	}
	normalized["id"] = json.RawMessage(strconvQuote(profileID))
	requestJSON, err := json.Marshal(normalized)
	if err != nil {
		return agentsettingsdto.ProfileUpdateRequest{}, err
	}
	var request agentsettingsdto.ProfileUpdateRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return agentsettingsdto.ProfileUpdateRequest{}, fmt.Errorf("invalid profile settings: %w", err)
	}
	if force, exists := options["force"]; exists {
		if err := json.Unmarshal(force, &request.Force); err != nil {
			return agentsettingsdto.ProfileUpdateRequest{}, fmt.Errorf("force must be boolean")
		}
	}
	for key := range options {
		if key != "force" {
			return agentsettingsdto.ProfileUpdateRequest{}, fmt.Errorf("unsupported profile option %q", key)
		}
	}
	return request, nil
}

func decodeUserSettingsUpdate(changes map[string]json.RawMessage) (userdto.UpdateUserSettingsRequest, error) {
	normalized, err := normalizeChangeMap(changes, "user_settings")
	if err != nil {
		return userdto.UpdateUserSettingsRequest{}, err
	}
	if raw, ok := normalized["sidebar_task_colors"]; ok {
		if string(raw) == "null" {
			return userdto.UpdateUserSettingsRequest{}, fmt.Errorf("sidebar_task_colors must be an object")
		}
		var colors map[string]*string
		if err := json.Unmarshal(raw, &colors); err != nil || colors == nil {
			return userdto.UpdateUserSettingsRequest{}, fmt.Errorf("sidebar_task_colors must be an object")
		}
		patch, marshalErr := json.Marshal(map[string]any{"colors": colors})
		if marshalErr != nil {
			return userdto.UpdateUserSettingsRequest{}, marshalErr
		}
		delete(normalized, "sidebar_task_colors")
		normalized["sidebar_task_color_patch"] = patch
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return userdto.UpdateUserSettingsRequest{}, err
	}
	var request userdto.UpdateUserSettingsRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return userdto.UpdateUserSettingsRequest{}, fmt.Errorf("invalid user settings: %w", err)
	}
	return request, nil
}

func normalizeChangeMap(changes map[string]json.RawMessage, resourceType string) (map[string]json.RawMessage, error) {
	normalized := make(map[string]json.RawMessage, len(changes))
	for key, value := range changes {
		prefix := resourceType + "."
		key = strings.TrimPrefix(key, prefix)
		if _, exists := normalized[key]; exists {
			return nil, fmt.Errorf("setting %q was supplied more than once", key)
		}
		normalized[key] = value
	}
	return normalized, nil
}

func (s *settingsOperations) updateProfileMCP(ctx context.Context, target settingscatalog.ResourceTarget, changes map[string]json.RawMessage) (any, error) {
	if s.profiles == nil || target.ResourceID == nil {
		return nil, fmt.Errorf("profile MCP settings are unavailable")
	}
	profile, err := s.findProfile(ctx, target)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeChangeMap(changes, "agent_profile_mcp")
	if err != nil {
		return nil, err
	}
	request := agentsettingscontroller.UpdateAgentProfileMcpConfigPatchRequest{}
	for key, value := range normalized {
		switch key {
		case "enabled":
			var enabled bool
			if string(value) == "null" || json.Unmarshal(value, &enabled) != nil {
				return nil, fmt.Errorf("enabled must be boolean")
			}
			request.Enabled = &enabled
		case "servers":
			if string(value) == "null" {
				return nil, fmt.Errorf("servers must be an object")
			}
			var servers map[string]mcpconfig.ServerDef
			if err := json.Unmarshal(value, &servers); err != nil || servers == nil {
				return nil, fmt.Errorf("servers must be an object")
			}
			request.Servers = &servers
		default:
			return nil, fmt.Errorf("setting %q is not writable", key)
		}
	}
	updated, err := s.profiles.UpdateAgentProfileMcpConfigPatch(ctx, *target.ResourceID, request)
	if err != nil {
		return nil, err
	}
	s.broadcastProfileMCPConfigUpdated(*target.ResourceID, profile.WorkspaceID)
	return map[string]any{"target": target, "accepted_fields": changePaths(changes), "settings": s.sanitizeSettingsValue(target.ResourceType, updated), "source": "profile_mcp"}, nil
}

func (s *settingsOperations) broadcastProfileMCPConfigUpdated(profileID, workspaceID string) {
	if s.deps.broadcaster == nil || profileID == "" {
		return
	}
	var scopedWorkspaceID any
	if workspaceID != "" {
		scopedWorkspaceID = workspaceID
	}
	notification, err := ws.NewNotification(ws.ActionAgentProfileMCPConfigUpdated, map[string]any{
		"profile_id":   profileID,
		"workspace_id": scopedWorkspaceID,
	})
	if err != nil {
		return
	}
	if workspaceID != "" {
		workspaceHub, ok := s.deps.broadcaster.(interface {
			BroadcastToWorkspaceOrDrop(string, *ws.Message)
		})
		if ok {
			workspaceHub.BroadcastToWorkspaceOrDrop(workspaceID, notification)
		}
		return
	}
	//ws:global profile MCP updates without workspace ownership are global.
	s.deps.broadcaster.Broadcast(notification)
}

//nolint:nestif // The repository-set projection needs its nested stored-resource fallback.
func projectJSON(value any, registry *settingscatalog.Registry, resourceType string, keys []string) map[string]any {
	payload := map[string]any{}
	encoded, err := json.Marshal(value)
	if err != nil {
		return payload
	}
	_ = json.Unmarshal(encoded, &payload)
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		field, ok := settingsField(registry, resourceType, key)
		if !ok {
			continue
		}
		if current, exists := lookupJSONPath(payload, field.FieldPath); exists {
			result[field.FieldPath] = current
			continue
		}
		if resourceType == "repository_set" && field.FieldPath == "repository_ids" {
			if items, ok := payload["repositories"].([]any); ok {
				ids := make([]any, 0, len(items))
				for _, item := range items {
					if object, ok := item.(map[string]any); ok {
						if id, exists := object["repository_id"]; exists {
							ids = append(ids, id)
						}
					}
				}
				result[field.FieldPath] = ids
			}
		}
	}
	return result
}

func lookupJSONPath(payload map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = payload
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func settingsField(registry *settingscatalog.Registry, resourceType, key string) (settingscatalog.FieldDescriptor, bool) {
	domain, ok := registry.Domain(resourceType)
	if !ok {
		return settingscatalog.FieldDescriptor{}, false
	}
	for _, field := range domain.Fields {
		if field.Key == key || field.FieldPath == key {
			return field, true
		}
	}
	return settingscatalog.FieldDescriptor{}, false
}

func (s *settingsOperations) sanitizeSettingsValue(resourceType string, value any) any {
	if isIntegrationResource(resourceType) {
		paths := make([]string, 0)
		if domain, ok := s.registry.Domain(resourceType); ok {
			for _, field := range domain.Fields {
				paths = append(paths, field.FieldPath)
			}
		}
		values := projectJSON(value, s.registry, resourceType, paths)
		redactSettingsValues(values, s.registry, resourceType)
		return values
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"redacted": true}
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return map[string]any{"redacted": true}
	}
	if object, ok := decoded.(map[string]any); ok {
		redactSettingsValues(object, s.registry, resourceType)
		if resourceType == "agent" {
			redactAgentProfileCollections(object)
		}
		return object
	}
	return decoded
}

func redactAgentProfileCollections(values map[string]any) {
	profiles, ok := values["profiles"].([]any)
	if !ok {
		return
	}
	for _, rawProfile := range profiles {
		profile, ok := rawProfile.(map[string]any)
		if !ok {
			continue
		}
		if envVars, exists := profile["env_vars"]; exists {
			profile["env_vars"] = redactSensitiveValue(envVars)
		}
	}
}

func redactSettingsValues(values map[string]any, registry *settingscatalog.Registry, resourceType string) {
	domain, ok := registry.Domain(resourceType)
	if !ok {
		return
	}
	for _, field := range domain.Fields {
		if !field.Sensitive {
			continue
		}
		if current, exists := lookupJSONPath(values, field.FieldPath); exists {
			_ = setJSONPath(values, field.FieldPath, redactSensitiveValue(current))
		}
	}
}

func setJSONPath(payload map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("invalid JSON path %q", path)
	}
	current := payload
	for _, part := range parts[:len(parts)-1] {
		nested, ok := current[part].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid JSON path %q", path)
		}
		current = nested
	}
	current[parts[len(parts)-1]] = value
	return nil
}

func redactSensitiveValue(value any) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactSensitiveValue(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed)+1)
		redacted := false
		for key, item := range typed {
			if sensitiveJSONKey(key) {
				result[key] = "[redacted]"
				redacted = true
				continue
			}
			switch item.(type) {
			case map[string]any, []any:
				result[key] = redactSensitiveValue(item)
			default:
				if safeJSONKey(key) {
					result[key] = item
				} else {
					result[key] = "[redacted]"
					redacted = true
				}
			}
		}
		if redacted {
			result["redacted"] = true
		}
		return result
	default:
		return "[redacted]"
	}
}

func sensitiveJSONKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	if normalized == "secret_id" || normalized == "secret_ref" {
		return false
	}
	for _, token := range []string{"value", "secret", "token", "password", "credential", "authorization", "api_key", "private_key", "headers", "env", "env_vars"} {
		if normalized == token || strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func safeJSONKey(key string) bool {
	switch strings.ToLower(key) {
	case "id", "key", "name", "type", "kind", "scope", "ref", "secret_id", "description", "command", "args", "enabled", "required":
		return true
	default:
		return false
	}
}

func changePaths(changes map[string]json.RawMessage) []string {
	paths := make([]string, 0, len(changes))
	for key := range changes {
		paths = append(paths, key)
	}
	sort.Strings(paths)
	return paths
}

func parseSettingsCursor(cursor string) int {
	var value int
	if _, err := fmt.Sscanf(cursor, "%d", &value); err != nil || value < 0 {
		return 0
	}
	return value
}

func settingsStringPointer(value string) *string { return &value }

func nonEmptyPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func strconvQuote(value string) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
