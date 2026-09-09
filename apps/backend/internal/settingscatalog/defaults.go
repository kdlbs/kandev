//nolint:goconst // Catalog owner and scope labels are stable metadata vocabulary.
package settingscatalog

// DefaultDomainDescriptors is the stable cross-domain inventory used by the
// backend when composition has not supplied a narrower registry.
func DefaultDomainDescriptors() []DomainDescriptor {
	return []DomainDescriptor{
		profileDomain(),
		profileMCPDomain(),
		agentDomain(),
		userSettingsDomain(),
		workflowDomain(),
		workflowStepDomain(),
		workspaceDomain(),
		repositoryDomain(),
		repositoryScriptDomain(),
		repositorySetDomain(),
		executorDomain(),
		executorProfileDomain(),
		environmentDomain(),
		taskDomain(),
		promptDomain(),
		utilityAgentDomain(),
		editorDomain(),
		notificationProviderDomain(),
		compatibilityIntegrationDomain("issue_integrations", "issue_integration", "Issue integrations", "Use provider-specific Jira, Linear, and Sentry resource types for settings updates."),
		compatibilityIntegrationDomain("code_host_integrations", "code_host_integration", "Code host integrations", "Use provider-specific GitHub, GitLab, and Azure DevOps resource types for settings updates."),
		jiraSettingsDomain(),
		linearSettingsDomain(),
		sentryInstanceDomain(),
		jiraIssueWatchDomain(),
		linearIssueWatchDomain(),
		sentryIssueWatchDomain(),
		githubSettingsDomain(),
		gitlabSettingsDomain(),
		azureDevOpsSettingsDomain(),
		githubReviewWatchDomain(),
		githubIssueWatchDomain(),
		gitlabReviewWatchDomain(),
		gitlabIssueWatchDomain(),
		azureDevOpsWorkItemWatchDomain(),
		azureDevOpsPullRequestWatchDomain(),
		automationDomain(),
		automationTriggerDomain(),
		runtimeFlagDomain(),
		storageMaintenanceDomain(),
	}
}

func compatibilityIntegrationDomain(domain, resourceType, label, description string) DomainDescriptor {
	return DomainDescriptor{
		Domain: domain, ResourceType: resourceType, Label: label, Description: description, Owner: domain,
		Scope: ScopeWorkspace, Target: TargetRules{Scope: ScopeWorkspace, RequiresWorkspaceID: true}, SettingsHref: "/settings/integrations",
		Fields: []FieldDescriptor{{
			Key: resourceType + ".settings", FieldPath: "settings", Label: "Provider settings", Description: description,
			JSONType: "object", Support: SupportReadOnly, Classification: ClassificationException, Owner: domain,
			ExceptionReason: "Provider-specific resource types preserve the provider's validation and workspace boundary.",
			Recovery:        "Use the provider-specific settings resource returned by search or target lookup.",
		}},
	}
}

func integrationReadOnly(resourceType, path, label, description, jsonType, owner string) FieldDescriptor {
	return FieldDescriptor{
		Key: resourceType + "." + path, FieldPath: path, Label: label, Description: description, JSONType: jsonType,
		Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner,
	}
}

func integrationWritable(resourceType, path, label, description, jsonType, owner, authority string) FieldDescriptor {
	return domainWritable(resourceType, path, label, description, jsonType, owner, authority)
}

func integrationNullableWritable(resourceType, path, label, description, jsonType, owner, authority string) FieldDescriptor {
	field := integrationWritable(resourceType, path, label, description, jsonType, owner, authority)
	field.Nullable = true
	return field
}

func integrationDomain(domain, resourceType, label, description, owner string, target TargetRules, fields ...FieldDescriptor) DomainDescriptor {
	return supportedDomain(domain, resourceType, label, description, owner, target.Scope, target, fields...)
}

func workspaceIntegrationTarget() TargetRules {
	return TargetRules{Scope: ScopeWorkspace, RequiresWorkspaceID: true}
}

func watchTarget() TargetRules {
	return TargetRules{Scope: ScopeResource, RequiresResourceID: true, RequiresWorkspaceID: true, AllowsWorkspaceID: true}
}

func jiraSettingsDomain() DomainDescriptor {
	owner := "jira"
	target := workspaceIntegrationTarget()
	return integrationDomain("issue_integrations", "jira_settings", "Jira settings", "Noncredential Jira defaults for one workspace.", owner, target,
		integrationWritable("jira_settings", "defaultProjectKey", "Default project", "Default Jira project key for issue workflows.", "string", owner, "workspace.manage"),
		integrationReadOnly("jira_settings", "siteUrl", "Site URL", "Configured Jira site identity. Credential and identity changes use the connection flow.", "string", owner),
		integrationReadOnly("jira_settings", "email", "Account email", "Configured Jira account metadata.", "string", owner),
		integrationReadOnly("jira_settings", "authMethod", "Authentication method", "Configured Jira authentication method.", "string", owner),
		integrationReadOnly("jira_settings", "instanceType", "Instance type", "Configured Jira deployment type.", "string", owner),
		integrationReadOnly("jira_settings", "hasSecret", "Credential present", "Whether a credential is enrolled, without exposing it.", "boolean", owner),
	)
}

func linearSettingsDomain() DomainDescriptor {
	owner := "linear"
	target := workspaceIntegrationTarget()
	return integrationDomain("issue_integrations", "linear_settings", "Linear settings", "Noncredential Linear defaults for one workspace.", owner, target,
		integrationWritable("linear_settings", "defaultTeamKey", "Default team", "Default Linear team key for issue workflows.", "string", owner, "workspace.manage"),
		integrationReadOnly("linear_settings", "authMethod", "Authentication method", "Configured Linear authentication method.", "string", owner),
		integrationReadOnly("linear_settings", "orgSlug", "Organization slug", "Computed Linear organization metadata.", "string", owner),
		integrationReadOnly("linear_settings", "hasSecret", "Credential present", "Whether a credential is enrolled, without exposing it.", "boolean", owner),
	)
}

func sentryInstanceDomain() DomainDescriptor {
	owner := "sentry"
	return integrationDomain("issue_integrations", "sentry_instance", "Sentry instances", "Named noncredential Sentry instance settings.", owner, TargetRules{Scope: ScopeResource, RequiresResourceID: true, RequiresWorkspaceID: true, AllowsWorkspaceID: true},
		integrationWritable("sentry_instance", "name", "Name", "Display name for the Sentry instance.", "string", owner, "workspace.manage"),
		integrationReadOnly("sentry_instance", "url", "URL", "Configured Sentry instance identity. Connection changes use the interactive flow.", "string", owner),
		integrationReadOnly("sentry_instance", "authMethod", "Authentication method", "Configured Sentry authentication method.", "string", owner),
		integrationReadOnly("sentry_instance", "hasSecret", "Credential present", "Whether a credential is enrolled, without exposing it.", "boolean", owner),
	)
}

func issueWatchFields(resourceType, owner, filterPath, filterType string) []FieldDescriptor {
	fields := []FieldDescriptor{
		integrationWritable(resourceType, "workflowId", "Workflow", "Workflow that receives matching issues.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "workflowStepId", "Workflow step", "Workflow step that receives matching issues.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "repositoryId", "Repository", "Optional repository binding for created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "baseBranch", "Base branch", "Base branch for the optional repository binding.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "agentProfileId", "Agent profile", "Profile used for created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "executorProfileId", "Executor profile", "Executor profile used for created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "prompt", "Prompt", "Prompt added to created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "enabled", "Enabled", "Whether background polling is enabled.", "boolean", owner, "workspace.manage"),
		integrationWritable(resourceType, "pollIntervalSeconds", "Poll interval", "Background polling interval in seconds.", "integer", owner, "workspace.manage"),
		integrationNullableWritable(resourceType, "maxInflightTasks", "Concurrent task limit", "Maximum open tasks created by this watch, or null for unlimited.", "integer", owner, "workspace.manage"),
	}
	if filterPath != "" {
		fields = append(fields, integrationWritable(resourceType, filterPath, "Filter", "Provider-specific issue filter.", filterType, owner, "workspace.manage"))
	}
	return fields
}

func jiraIssueWatchDomain() DomainDescriptor {
	return integrationDomain("issue_integrations", "jira_issue_watch", "Jira issue watches", "Jira issue watch schedules and task bindings.", "jira", watchTarget(), issueWatchFields("jira_issue_watch", "jira", "jql", "string")...)
}

func linearIssueWatchDomain() DomainDescriptor {
	fields := issueWatchFields("linear_issue_watch", "linear", "filter", "object")
	fields = append(fields, integrationWritable("linear_issue_watch", "sortBy", "Sort order", "Order in which matching Linear issues are dispatched.", "string", "linear", "workspace.manage"))
	return integrationDomain("issue_integrations", "linear_issue_watch", "Linear issue watches", "Linear issue watch schedules and task bindings.", "linear", watchTarget(), fields...)
}

func sentryIssueWatchDomain() DomainDescriptor {
	return integrationDomain("issue_integrations", "sentry_issue_watch", "Sentry issue watches", "Sentry issue watch schedules and task bindings.", "sentry", watchTarget(), issueWatchFields("sentry_issue_watch", "sentry", "filter", "object")...)
}

func githubSettingsDomain() DomainDescriptor {
	owner := "github"
	return integrationDomain("code_host_integrations", "github_settings", "GitHub settings", "Noncredential GitHub scope and query preferences for one workspace.", owner, workspaceIntegrationTarget(),
		integrationWritable("github_settings", "task_git_credentials_mode", "Task Git credentials", "Credential routing mode for task worktrees.", "string", owner, "workspace.manage"),
		integrationWritable("github_settings", "repo_scope_mode", "Repository scope mode", "Repository scope selection mode.", "string", owner, "workspace.manage"),
		integrationWritable("github_settings", "repo_scope_orgs", "Organizations", "Organization allowlist for repository scope.", "array", owner, "workspace.manage"),
		integrationWritable("github_settings", "repo_scope_repos", "Repositories", "Repository allowlist for repository scope.", "array", owner, "workspace.manage"),
		integrationWritable("github_settings", "saved_presets", "Saved presets", "Workspace-scoped GitHub action presets.", "object", owner, "workspace.manage"),
		integrationWritable("github_settings", "default_query_presets", "Default query presets", "Workspace-scoped GitHub query presets.", "object", owner, "workspace.manage"),
	)
}

func gitlabSettingsDomain() DomainDescriptor {
	owner := "gitlab"
	return integrationDomain("code_host_integrations", "gitlab_settings", "GitLab settings", "Configured GitLab workspace connection metadata.", owner, workspaceIntegrationTarget(),
		integrationReadOnly("gitlab_settings", "host", "Host", "Configured GitLab host. Connection identity changes use the interactive flow.", "string", owner),
		integrationReadOnly("gitlab_settings", "auth_method", "Authentication method", "Configured GitLab authentication method.", "string", owner),
		integrationReadOnly("gitlab_settings", "username", "Username", "Last authenticated GitLab username.", "string", owner),
	)
}

func azureDevOpsSettingsDomain() DomainDescriptor {
	owner := "azuredevops"
	return integrationDomain("code_host_integrations", "azure_devops_settings", "Azure DevOps settings", "Noncredential Azure DevOps query and action presets for one workspace.", owner, workspaceIntegrationTarget(),
		integrationWritable("azure_devops_settings", "workItemQueries", "Work item queries", "Saved work item query presets.", "array", owner, "workspace.manage"),
		integrationWritable("azure_devops_settings", "pullRequestQueries", "Pull request queries", "Saved pull request query presets.", "array", owner, "workspace.manage"),
		integrationWritable("azure_devops_settings", "workItemActions", "Work item actions", "Saved work item action presets.", "array", owner, "workspace.manage"),
		integrationWritable("azure_devops_settings", "pullRequestActions", "Pull request actions", "Saved pull request action presets.", "array", owner, "workspace.manage"),
	)
}

func codeHostWatchFields(resourceType, owner string, projects bool, issue bool) []FieldDescriptor {
	fields := []FieldDescriptor{
		integrationWritable(resourceType, "workflow_id", "Workflow", "Workflow that receives matching items.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "workflow_step_id", "Workflow step", "Workflow step that receives matching items.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "agent_profile_id", "Agent profile", "Profile used for created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "executor_profile_id", "Executor profile", "Executor profile used for created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "prompt", "Prompt", "Prompt added to created tasks.", "string", owner, "workspace.manage"),
		integrationWritable(resourceType, "enabled", "Enabled", "Whether background polling is enabled.", "boolean", owner, "workspace.manage"),
		integrationWritable(resourceType, "poll_interval_seconds", "Poll interval", "Background polling interval in seconds.", "integer", owner, "workspace.manage"),
		integrationWritable(resourceType, "cleanup_policy", "Cleanup policy", "Policy for tasks created by this watch.", "string", owner, "workspace.manage"),
		integrationNullableWritable(resourceType, "max_inflight_tasks", "Concurrent task limit", "Maximum open tasks created by this watch, or null for unlimited.", "integer", owner, "workspace.manage"),
	}
	if projects {
		fields = append(fields, integrationWritable(resourceType, "projects", "Projects", "Provider project filters.", "array", owner, "workspace.manage"))
	} else {
		fields = append(fields, integrationWritable(resourceType, "repos", "Repositories", "Provider repository filters.", "array", owner, "workspace.manage"))
	}
	if issue {
		fields = append(fields, integrationWritable(resourceType, "labels", "Labels", "Issue label filters.", "array", owner, "workspace.manage"))
		fields = append(fields, integrationWritable(resourceType, "custom_query", "Custom query", "Provider-specific issue query.", "string", owner, "workspace.manage"))
	} else {
		fields = append(fields, integrationWritable(resourceType, "review_scope", "Review scope", "Review assignment scope.", "string", owner, "workspace.manage"))
		fields = append(fields, integrationWritable(resourceType, "custom_query", "Custom query", "Provider-specific review query.", "string", owner, "workspace.manage"))
	}
	return fields
}

func githubReviewWatchDomain() DomainDescriptor {
	return integrationDomain("code_host_integrations", "github_review_watch", "GitHub review watches", "GitHub review watch schedules and task bindings.", "github", watchTarget(), codeHostWatchFields("github_review_watch", "github", false, false)...)
}

func githubIssueWatchDomain() DomainDescriptor {
	return integrationDomain("code_host_integrations", "github_issue_watch", "GitHub issue watches", "GitHub issue watch schedules and task bindings.", "github", watchTarget(), codeHostWatchFields("github_issue_watch", "github", false, true)...)
}

func gitlabReviewWatchDomain() DomainDescriptor {
	return integrationDomain("code_host_integrations", "gitlab_review_watch", "GitLab review watches", "GitLab review watch schedules and task bindings.", "gitlab", watchTarget(), codeHostWatchFields("gitlab_review_watch", "gitlab", true, false)...)
}

func gitlabIssueWatchDomain() DomainDescriptor {
	return integrationDomain("code_host_integrations", "gitlab_issue_watch", "GitLab issue watches", "GitLab issue watch schedules and task bindings.", "gitlab", watchTarget(), codeHostWatchFields("gitlab_issue_watch", "gitlab", true, true)...)
}

func azureDevOpsWorkItemWatchDomain() DomainDescriptor {
	fields := withoutIntegrationFields(codeHostWatchFields("azure_devops_work_item_watch", "azuredevops", false, true), "repos", "labels", "custom_query")
	fields = append(fields,
		integrationWritable("azure_devops_work_item_watch", "projectId", "Project", "Azure DevOps project filter.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_work_item_watch", "wiql", "WIQL", "Azure Boards query.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_work_item_watch", "repositoryId", "Repository", "Optional repository binding.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_work_item_watch", "baseBranch", "Base branch", "Base branch for the optional repository binding.", "string", "azuredevops", "workspace.manage"),
	)
	return integrationDomain("code_host_integrations", "azure_devops_work_item_watch", "Azure work item watches", "Azure work item watch schedules and task bindings.", "azuredevops", watchTarget(), fields...)
}

func azureDevOpsPullRequestWatchDomain() DomainDescriptor {
	fields := withoutIntegrationFields(codeHostWatchFields("azure_devops_pull_request_watch", "azuredevops", false, false), "repos", "review_scope", "custom_query")
	fields = append(fields,
		integrationWritable("azure_devops_pull_request_watch", "projectId", "Project", "Azure DevOps project filter.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_pull_request_watch", "azureRepositoryId", "Azure repository", "Azure Repos repository filter.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_pull_request_watch", "status", "Status", "Pull request status filter.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_pull_request_watch", "creatorId", "Creator", "Pull request creator filter.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_pull_request_watch", "reviewerId", "Reviewer", "Pull request reviewer filter.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_pull_request_watch", "repositoryId", "Repository", "Optional repository binding.", "string", "azuredevops", "workspace.manage"),
		integrationWritable("azure_devops_pull_request_watch", "baseBranch", "Base branch", "Base branch for the optional repository binding.", "string", "azuredevops", "workspace.manage"),
	)
	return integrationDomain("code_host_integrations", "azure_devops_pull_request_watch", "Azure pull request watches", "Azure pull request watch schedules and task bindings.", "azuredevops", watchTarget(), fields...)
}

func withoutIntegrationFields(fields []FieldDescriptor, paths ...string) []FieldDescriptor {
	remove := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		remove[path] = struct{}{}
	}
	filtered := make([]FieldDescriptor, 0, len(fields))
	for _, field := range fields {
		if _, skip := remove[field.FieldPath]; !skip {
			filtered = append(filtered, field)
		}
	}
	return filtered
}

func agentDomain() DomainDescriptor {
	owner := "agent-settings"
	return DomainDescriptor{
		Domain: "agents", ResourceType: "agent", Label: "Agent definitions",
		Description: "Saved agent definition metadata and MCP connection settings.", Owner: owner,
		Scope: ScopeResource, Target: TargetRules{Scope: ScopeResource, RequiresResourceID: true, AllowsWorkspaceID: true},
		SettingsHref: "/settings/agents",
		Operations:   []OperationDescriptor{{Name: "update", Binding: "agent-settings.agent.update", Authority: "org.config.manage"}},
		Fields: []FieldDescriptor{
			{Key: "agent.name", FieldPath: "name", Label: "Name", Description: "Stable agent definition name.", JSONType: "string", Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner},
			{Key: "agent.workspace_id", FieldPath: "workspace_id", Label: "Workspace", Description: "Owning workspace, when the definition is workspace-scoped.", JSONType: "string", Nullable: true, Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner},
			domainWritable("agent", "supports_mcp", "Supports MCP", "Whether the agent definition supports MCP configuration.", "boolean", owner, "org.config.manage"),
			func() FieldDescriptor {
				field := domainWritable("agent", "mcp_config_path", "MCP configuration path", "Path used by the agent definition for MCP configuration.", "string", owner, "org.config.manage")
				field.Nullable = true
				return field
			}(),
			{Key: "agent.tui_config", FieldPath: "tui_config", Label: "TUI definition", Description: "Custom TUI definition owned by the explicit agent lifecycle surface.", JSONType: "object", Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner},
			{Key: "agent.profiles", FieldPath: "profiles", Label: "Profiles", Description: "Profiles attached to this agent definition.", JSONType: "array", Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner},
			{Key: "agent.capability_status", FieldPath: "capability_status", Label: "Capability status", Description: "Computed host capability status.", JSONType: "string", Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner},
			{Key: "agent.capability_error", FieldPath: "capability_error", Label: "Capability error", Description: "Computed host capability diagnostic.", JSONType: "string", Support: SupportReadOnly, Classification: ClassificationComputed, Owner: owner},
		},
	}
}

func supportedDomain(domain, resourceType, label, description, owner string, scope Scope, target TargetRules, fields ...FieldDescriptor) DomainDescriptor {
	return DomainDescriptor{
		Domain: domain, ResourceType: resourceType, Label: label, Description: description, Owner: owner,
		Scope: scope, Target: target, Fields: fields,
		Operations:   []OperationDescriptor{{Name: "update", Binding: owner + "." + resourceType + ".update", Authority: "domain.owner"}},
		SettingsHref: "/settings",
	}
}

func domainWritable(resourceType, path, label, description, jsonType, owner, authority string) FieldDescriptor {
	return FieldDescriptor{
		Key: resourceType + "." + path, FieldPath: path, Label: label, Description: description, JSONType: jsonType,
		Support: SupportSupported, Classification: ClassificationWritable, Owner: owner, Writable: true,
		Validator: owner + "." + resourceType, Authority: authority, SettingsHref: "/settings",
	}
}

func domainSensitiveWritable(resourceType, path, label, description, jsonType, owner, authority string) FieldDescriptor {
	field := domainWritable(resourceType, path, label, description, jsonType, owner, authority)
	field.Sensitive = true
	return field
}

func exceptionField(resourceType, path, label, description, jsonType, owner, recovery string) FieldDescriptor {
	return FieldDescriptor{
		Key: resourceType + "." + path, FieldPath: path, Label: label, Description: description,
		JSONType: jsonType, Support: SupportException, Classification: ClassificationException,
		Owner: owner, SettingsHref: "/settings", ExceptionReason: description, Recovery: recovery,
	}
}

func lifecycleException(resourceType, path, label, description, jsonType, owner, recovery string) FieldDescriptor {
	field := exceptionField(resourceType, path, label, description, jsonType, owner, recovery)
	field.Classification = ClassificationAction
	return field
}

func workflowDomain() DomainDescriptor {
	owner := "workflow"
	target := TargetRules{Scope: ScopeResource, RequiresResourceID: true, AllowsWorkspaceID: true}
	return supportedDomain("workflows", "workflow", "Workflows", "Saved workflow definitions.", owner, ScopeResource, target,
		domainWritable("workflow", "name", "Name", "Workflow name.", "string", owner, "workspace.manage"),
		domainWritable("workflow", "description", "Description", "Workflow description.", "string", owner, "workspace.manage"),
		domainWritable("workflow", "prompt", "Prompt", "Workflow-level instructions.", "string", owner, "workspace.manage"),
		domainWritable("workflow", "agent_profile_id", "Agent profile", "Profile used by the workflow when configured.", "string", owner, "workspace.manage"),
	)
}

func workflowStepDomain() DomainDescriptor {
	owner := "workflow"
	fields := []FieldDescriptor{
		domainWritable("workflow_step", "name", "Name", "Step name.", "string", owner, "workspace.manage"),
		lifecycleException("workflow_step", "position", "Position", "Step ordering is controlled by the workflow reorder action.", "integer", owner, "Use the workflow step reorder action."),
		domainWritable("workflow_step", "color", "Color", "Step display color.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "prompt", "Prompt", "Step instructions.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "events", "Events", "Validated step event actions and bindings.", "object", owner, "workspace.manage"),
		domainWritable("workflow_step", "allow_manual_move", "Manual move", "Whether users may move tasks into this step.", "boolean", owner, "workspace.manage"),
		domainWritable("workflow_step", "is_start_step", "Start step", "Whether this is the workflow start step.", "boolean", owner, "workspace.manage"),
		domainWritable("workflow_step", "show_in_command_panel", "Command panel", "Whether the step appears in the command panel.", "boolean", owner, "workspace.manage"),
		domainWritable("workflow_step", "auto_archive_after_hours", "Auto archive", "Hours before tasks in this step are archived.", "integer", owner, "workspace.manage"),
		domainWritable("workflow_step", "agent_profile_id", "Agent profile", "Profile bound to this step.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "profile_session_start_policy", "Session start policy", "Session policy when entering a profile-bound step.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "profile_session_end_policy", "Session end policy", "Session policy when leaving a profile-bound step.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "wip_limit", "WIP limit", "Maximum admitted work in this step.", "integer", owner, "workspace.manage"),
		domainWritable("workflow_step", "pull_from_step_id", "Pull source", "Step that supplies work to this step.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "stage_type", "Stage type", "Workflow stage type.", "string", owner, "workspace.manage"),
		domainWritable("workflow_step", "auto_advance_requires_signal", "Completion signal", "Require an explicit completion signal before advancing.", "boolean", owner, "workspace.manage"),
		domainWritable("workflow_step", "cancel_triggers_turn_complete", "Cancel transition", "Whether cancellation runs turn-complete actions.", "boolean", owner, "workspace.manage"),
	}
	return supportedDomain("workflows", "workflow_step", "Workflow steps", "Editable steps in a saved workflow.", owner, ScopeResource, targetRulesResourceWorkspace(), fields...)
}

func targetRulesResourceWorkspace() TargetRules {
	return TargetRules{Scope: ScopeResource, RequiresResourceID: true, AllowsWorkspaceID: true}
}

func workspaceDomain() DomainDescriptor {
	owner := "workspace"
	return supportedDomain("workspaces", "workspace", "Workspaces", "Workspace defaults and display settings.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("workspace", "name", "Name", "Workspace name.", "string", owner, "workspace.manage"),
		domainWritable("workspace", "description", "Description", "Workspace description.", "string", owner, "workspace.manage"),
		domainWritable("workspace", "default_executor_id", "Default executor", "Default execution target.", "string", owner, "workspace.manage"),
		domainWritable("workspace", "default_environment_id", "Default environment", "Default runtime environment.", "string", owner, "workspace.manage"),
		domainWritable("workspace", "default_agent_profile_id", "Default agent profile", "Default profile for new work.", "string", owner, "workspace.manage"),
		domainWritable("workspace", "default_config_agent_profile_id", "Default configuration profile", "Profile used for configuration work.", "string", owner, "workspace.manage"),
	)
}

func repositoryDomain() DomainDescriptor {
	owner := "workspace"
	paths := []struct{ path, label, typ string }{
		{"name", "Name", "string"}, {"source_type", "Source type", "string"}, {"local_path", "Local path", "string"},
		{"provider", "Provider", "string"}, {"provider_repo_id", "Provider repository ID", "string"}, {"provider_host", "Provider host", "string"},
		{"provider_scope", "Provider scope", "string"}, {"provider_owner", "Provider owner", "string"}, {"provider_name", "Provider name", "string"},
		{"remote_url", "Remote URL", "string"}, {"default_branch", "Default branch", "string"}, {"worktree_branch_prefix", "Branch prefix", "string"},
		{"worktree_branch_template", "Branch template", "string"}, {"pull_before_worktree", "Pull before worktree", "boolean"},
		{"setup_script", "Setup script", "string"}, {"cleanup_script", "Cleanup script", "string"}, {"dev_script", "Development script", "string"},
		{"copy_files", "Copy files", "string"}, {"secret_bindings", "Secret bindings", "array"},
	}
	fields := make([]FieldDescriptor, 0, len(paths))
	for _, item := range paths {
		field := domainWritable("repository", item.path, item.label, "Repository setting.", item.typ, owner, "workspace.manage")
		if item.path == "secret_bindings" {
			field.Sensitive = true
		}
		fields = append(fields, field)
	}
	return supportedDomain("workspaces", "repository", "Repositories", "Workspace repository settings.", owner, ScopeResource, targetRulesResourceWorkspace(), fields...)
}

func repositoryScriptDomain() DomainDescriptor {
	owner := "workspace"
	return supportedDomain("workspaces", "repository_script", "Repository scripts", "Custom repository commands.", owner, ScopeResource, targetRulesResourceWorkspace(),
		domainWritable("repository_script", "name", "Name", "Script name.", "string", owner, "workspace.manage"),
		domainWritable("repository_script", "command", "Command", "Script command.", "string", owner, "workspace.manage"),
		lifecycleException("repository_script", "position", "Position", "Script ordering is controlled by the repository script reorder action.", "integer", owner, "Use the repository script reorder action."),
	)
}

func repositorySetDomain() DomainDescriptor {
	owner := "workspace"
	return supportedDomain("workspaces", "repository_set", "Repository sets", "Reusable repository selections.", owner, ScopeResource, targetRulesResourceWorkspace(),
		domainWritable("repository_set", "name", "Name", "Repository set name.", "string", owner, "workspace.manage"),
		domainWritable("repository_set", "description", "Description", "Repository set description.", "string", owner, "workspace.manage"),
		domainWritable("repository_set", "repository_ids", "Repositories", "Ordered repository membership.", "array", owner, "workspace.manage"),
	)
}

func executorDomain() DomainDescriptor {
	owner := "execution"
	return supportedDomain("execution", "executor", "Executors", "Execution target settings.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("executor", "name", "Name", "Executor name.", "string", owner, "execution.manage"),
		domainWritable("executor", "type", "Type", "Executor runtime type.", "string", owner, "execution.manage"),
		domainWritable("executor", "status", "Status", "Whether the executor accepts work.", "string", owner, "execution.manage"),
		domainWritable("executor", "resumable", "Resumable", "Whether sessions can resume on this executor.", "boolean", owner, "execution.manage"),
		domainSensitiveWritable("executor", "config", "Configuration", "Validated executor configuration.", "object", owner, "execution.manage"),
	)
}

func executorProfileDomain() DomainDescriptor {
	owner := "execution"
	return supportedDomain("execution", "executor_profile", "Executor profiles", "Named executor presets.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("executor_profile", "name", "Name", "Profile name.", "string", owner, "execution.manage"),
		domainWritable("executor_profile", "mcp_policy", "MCP policy", "MCP policy for this executor profile.", "string", owner, "execution.manage"),
		domainWritable("executor_profile", "config", "Configuration", "Validated executor profile configuration.", "object", owner, "execution.manage"),
		domainWritable("executor_profile", "prepare_script", "Prepare script", "Preparation command.", "string", owner, "execution.manage"),
		domainWritable("executor_profile", "cleanup_script", "Cleanup script", "Cleanup command.", "string", owner, "execution.manage"),
		domainSensitiveWritable("executor_profile", "env_vars", "Environment variables", "Environment values or secret references.", "array", owner, "execution.manage"),
	)
}

func environmentDomain() DomainDescriptor {
	owner := "execution"
	return supportedDomain("execution", "environment", "Environments", "Execution environment definitions.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("environment", "name", "Name", "Environment name.", "string", owner, "execution.manage"),
		domainWritable("environment", "kind", "Kind", "Environment kind.", "string", owner, "execution.manage"),
		domainWritable("environment", "worktree_root", "Worktree root", "Worktree root path.", "string", owner, "execution.manage"),
		domainWritable("environment", "image_tag", "Image tag", "Container image tag.", "string", owner, "execution.manage"),
		domainWritable("environment", "dockerfile", "Dockerfile", "Dockerfile content.", "string", owner, "execution.manage"),
		domainWritable("environment", "build_config", "Build configuration", "Validated build options.", "object", owner, "execution.manage"),
	)
}

func taskDomain() DomainDescriptor {
	owner := "task"
	return supportedDomain("tasks", "task", "Tasks", "Permitted saved task configuration.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true, AllowsWorkspaceID: true},
		domainWritable("task", "title", "Title", "Task title.", "string", owner, "task.manage"),
		domainWritable("task", "description", "Description", "Task description.", "string", owner, "task.manage"),
		domainWritable("task", "priority", "Priority", "Task priority.", "string", owner, "task.manage"),
		lifecycleException("task", "workflow_step_id", "Workflow step", "Task movement is controlled by the task lifecycle action.", "string", owner, "Use the task move action."),
		lifecycleException("task", "position", "Position", "Task ordering is controlled by the task reorder action.", "integer", owner, "Use the task reorder action."),
		exceptionField("task", "metadata", "Metadata", "Open-ended task metadata is controlled by domain-specific task operations.", "object", owner, "Use the task metadata operation for a known metadata key."),
	)
}

func promptDomain() DomainDescriptor {
	owner := "prompts"
	return supportedDomain("prompts", "prompt", "Saved prompts", "Saved prompt definitions.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("prompt", "name", "Name", "Prompt name.", "string", owner, "prompt.manage"),
		domainWritable("prompt", "content", "Content", "Prompt content.", "string", owner, "prompt.manage"),
	)
}

func utilityAgentDomain() DomainDescriptor {
	owner := "utilities"
	return supportedDomain("utilities", "utility_agent", "Utility agents", "Utility agent definitions.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("utility_agent", "name", "Name", "Utility name.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "description", "Description", "Utility description.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "prompt", "Prompt", "Utility prompt template.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "agent_id", "Agent", "Inference agent identifier.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "model", "Model", "Default model.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "agent_profile_id", "Agent profile", "Profile binding.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "profile_binding_state", "Profile binding state", "Profile inheritance state.", "string", owner, "utility.manage"),
		domainWritable("utility_agent", "enabled", "Enabled", "Whether the utility is available.", "boolean", owner, "utility.manage"),
	)
}

func editorDomain() DomainDescriptor {
	owner := "editors"
	return supportedDomain("editors", "editor", "Editor definitions", "Custom editor definitions.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("editor", "name", "Name", "Editor display name.", "string", owner, "editor.manage"),
		domainWritable("editor", "kind", "Kind", "Custom editor kind.", "string", owner, "editor.manage"),
		domainWritable("editor", "config", "Configuration", "Validated editor configuration.", "object", owner, "editor.manage"),
		domainWritable("editor", "enabled", "Enabled", "Whether the editor can be selected.", "boolean", owner, "editor.manage"),
	)
}

func notificationProviderDomain() DomainDescriptor {
	owner := "notifications"
	return supportedDomain("notifications", "notification_provider", "Notification providers", "Notification provider settings and subscriptions.", owner, ScopeResource, TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		domainWritable("notification_provider", "name", "Name", "Provider name.", "string", owner, "notification.manage"),
		domainWritable("notification_provider", "type", "Type", "Provider type.", "string", owner, "notification.manage"),
		domainSensitiveWritable("notification_provider", "config", "Configuration", "Validated provider configuration.", "object", owner, "notification.manage"),
		domainWritable("notification_provider", "enabled", "Enabled", "Whether notifications are enabled.", "boolean", owner, "notification.manage"),
		domainWritable("notification_provider", "events", "Events", "Subscribed event types.", "array", owner, "notification.manage"),
	)
}

func automationDomain() DomainDescriptor {
	owner := "automations"
	return supportedDomain("automations", "automation", "Workspace automations", "Workspace automation definitions.", owner, ScopeWorkspace, TargetRules{Scope: ScopeWorkspace, RequiresResourceID: true, AllowsWorkspaceID: true},
		domainWritable("automation", "name", "Name", "Automation name.", "string", owner, "workspace.manage"),
		domainWritable("automation", "description", "Description", "Automation description.", "string", owner, "workspace.manage"),
		domainWritable("automation", "workflow_id", "Workflow", "Workflow binding.", "string", owner, "workspace.manage"),
		domainWritable("automation", "workflow_step_id", "Workflow step", "Workflow step binding.", "string", owner, "workspace.manage"),
		domainWritable("automation", "agent_profile_id", "Agent profile", "Agent profile binding.", "string", owner, "workspace.manage"),
		domainWritable("automation", "executor_profile_id", "Executor profile", "Executor profile binding.", "string", owner, "workspace.manage"),
		domainWritable("automation", "repositories", "Repositories", "Repository and base-branch selection.", "array", owner, "workspace.manage"),
		domainWritable("automation", "prompt", "Prompt", "Automation prompt template.", "string", owner, "workspace.manage"),
		domainWritable("automation", "task_title_template", "Task title template", "Generated task title template.", "string", owner, "workspace.manage"),
		domainWritable("automation", "enabled", "Enabled", "Whether the automation is active.", "boolean", owner, "workspace.manage"),
		domainWritable("automation", "max_concurrent_runs", "Concurrent runs", "Maximum simultaneous runs.", "integer", owner, "workspace.manage"),
		domainWritable("automation", "continuation_policy", "Continuation policy", "How automation runs reuse tasks.", "string", owner, "workspace.manage"),
		domainWritable("automation", "task_mode", "Task mode", "Whether runs create visible tasks.", "string", owner, "workspace.manage"),
		domainWritable("automation", "repository_mode", "Repository mode", "How runs select repositories.", "string", owner, "workspace.manage"),
	)
}

func automationTriggerDomain() DomainDescriptor {
	owner := "automations"
	return supportedDomain("automations", "automation_trigger", "Automation triggers", "Triggers attached to a workspace automation.", owner, ScopeResource, targetRulesResourceWorkspace(),
		domainSensitiveWritable("automation_trigger", "config", "Configuration", "Trigger configuration.", "object", owner, "workspace.manage"),
		domainWritable("automation_trigger", "enabled", "Enabled", "Whether the trigger is active.", "boolean", owner, "workspace.manage"),
	)
}

func runtimeFlagDomain() DomainDescriptor {
	owner := "runtime"
	field := domainWritable("runtime_flag", "override", "Override", "Explicit persisted runtime override. Null clears the override.", "boolean", owner, "org.settings.manage")
	field.Nullable = true
	return supportedDomain("runtime", "runtime_flag", "Runtime overrides", "Permitted persisted runtime flag overrides.", owner, ScopeInstallation, TargetRules{Scope: ScopeInstallation, RequiresResourceID: true}, field)
}

func storageMaintenanceDomain() DomainDescriptor {
	owner := "storage"
	target := TargetRules{Scope: ScopeInstallation, Singleton: true}
	paths := []struct{ path, label, typ string }{
		{"enabled", "Enabled", "boolean"}, {"check_interval_hours", "Check interval", "integer"}, {"idle_for_minutes", "Idle period", "integer"},
		{"orphan_grace_hours", "Orphan grace", "integer"}, {"quarantine_retention_hours", "Quarantine retention", "integer"},
		{"workspaces.enabled", "Workspace cleanup", "boolean"}, {"workspaces.dependency_cleanup_enabled", "Dependency cleanup", "boolean"},
		{"kandev_containers.enabled", "Kandev containers", "boolean"}, {"go_cache.enabled", "Go cache", "boolean"}, {"go_cache.max_bytes", "Go cache limit", "integer"},
		{"docker.dedicated_daemon_acknowledged", "Dedicated Docker acknowledgement", "boolean"}, {"docker.build_cache_enabled", "Docker build cache", "boolean"},
		{"docker.build_cache_keep_bytes", "Docker build cache limit", "integer"}, {"docker.build_cache_unused_hours", "Docker build cache age", "integer"},
		{"docker.unused_images_enabled", "Unused image cleanup", "boolean"}, {"docker.unused_images_hours", "Unused image age", "integer"},
	}
	fields := make([]FieldDescriptor, 0, len(paths))
	for _, item := range paths {
		fields = append(fields, domainWritable("storage_maintenance", item.path, item.label, "Storage maintenance policy.", item.typ, owner, "org.settings.manage"))
	}
	return supportedDomain("storage", "storage_maintenance", "Storage maintenance", "Install-wide storage maintenance policy.", owner, ScopeInstallation, target, fields...)
}

func userSettingsDomain() DomainDescriptor {
	values := []string{
		"workspace_id", "workspace", "string", "kanban_view_mode", "kanban view mode", "string", "startup_page", "startup page", "string",
		"workflow_filter_id", "workflow filter", "string", "repository_ids", "repository selection", "array", "tasks_list_sort", "task sort", "string",
		"tasks_list_group", "task grouping", "string", "tasks_list_show_details", "task details", "boolean", "initial_setup_complete", "setup complete", "boolean",
		"preferred_shell", "preferred shell", "string", "default_editor_id", "default editor", "string", "enable_preview_on_click", "preview on click", "boolean",
		"chat_submit_key", "chat submit key", "string", "review_auto_mark_on_scroll", "review auto mark", "boolean", "confirm_task_archive", "confirm task archive", "boolean",
		"prevent_auto_start_agent_on_open", "prevent auto start", "boolean", "unread_divider", "unread divider", "boolean", "agent_generated_task_titles", "generated titles", "boolean",
		"mcp_task_agent_profile_default", "MCP task profile", "string", "show_anchored_prompt_bar", "anchored prompt bar", "boolean", "show_scroll_to_last_prompt", "scroll to last prompt", "boolean",
		"show_scroll_to_start", "scroll to start", "boolean", "show_transcript_auto_scroll_control", "transcript scroll control", "boolean", "show_todo_list_panel", "todo list panel", "boolean",
		"show_todo_list_panel_only_when_not_empty", "todo list empty state", "boolean", "show_release_notification", "release notification", "boolean", "release_notes_last_seen_version", "release notes version", "string",
		"lsp_auto_start_languages", "LSP auto start", "array", "lsp_auto_install_languages", "LSP auto install", "array", "lsp_server_configs", "LSP server configs", "object",
		"lsp_status_location", "LSP status location", "string", "saved_layouts", "saved layouts", "array", "sidebar_views", "sidebar views", "array",
		"sidebar_active_view_id", "active sidebar view", "string", "sidebar_draft", "sidebar draft", "object", "thread_views", "thread views", "array",
		"thread_active_view_id", "active thread view", "string", "thread_view_draft", "thread draft", "object", "sidebar_task_prefs", "sidebar task preferences", "object",
		"sidebar_task_color_automation", "sidebar color automation", "object", "sidebar_task_colors", "sidebar task colors", "object", "task_create_last_used", "last task create values", "object",
		"jira_saved_views", "Jira saved views", "object", "jira_task_presets", "Jira task presets", "object", "github_saved_presets", "GitHub saved presets", "object",
		"github_default_query_presets", "GitHub query presets", "object", "gitlab_saved_presets", "GitLab saved presets", "object", "azure_devops_browse_preferences", "Azure DevOps browse preferences", "object",
		"default_utility_agent_id", "default utility agent", "string", "default_utility_model", "default utility model", "string", "default_utility_agent_profile_id", "default utility profile", "string",
		"keyboard_shortcuts", "keyboard shortcuts", "object", "terminal_link_behavior", "terminal link behavior", "string", "terminal_font_family", "terminal font family", "string",
		"terminal_font_size", "terminal font size", "integer", "changes_panel_layout", "changes panel layout", "string", "last_seen_display", "last seen display", "string",
		"system_metrics_display", "system metrics display", "object", "app_status_bar_enabled", "status bar", "boolean", "resolve_session_hostnames", "resolve hostnames", "boolean",
		"app_status_bar_order", "status bar order", "object", "quick_chat_tab_order_by_workspace", "quick chat tab order", "object", "kanban_hidden_step_ids", "hidden kanban steps", "object",
		"workflow_ids_with_auto_hide_empty_steps", "auto-hide workflows", "array",
	}
	fields := make([]FieldDescriptor, 0, len(values)/3)
	for index := 0; index+2 < len(values); index += 3 {
		field := preferenceField(values[index], values[index+1], values[index+2])
		switch values[index] {
		case "sidebar_draft", "thread_view_draft", "jira_saved_views", "jira_task_presets", "github_saved_presets", "github_default_query_presets", "gitlab_saved_presets", "azure_devops_browse_preferences":
			field.Nullable = true
		}
		fields = append(fields, field)
	}
	return DomainDescriptor{
		Domain: "user_preferences", ResourceType: "user_settings", Label: "Personal preferences",
		Description: "Portable settings belonging to the authenticated user.", Owner: "user-settings", Scope: ScopeCaller,
		Target: TargetRules{Scope: ScopeCaller, Singleton: true}, SettingsHref: "/settings/preferences", Fields: fields,
		Operations: []OperationDescriptor{{Name: "update", Binding: "user-settings.update"}},
	}
}

func preferenceField(path, label, jsonType string) FieldDescriptor {
	return FieldDescriptor{
		Key: "user_settings." + path, FieldPath: path, Label: label,
		Description: "Portable preference owned by the current user.", JSONType: jsonType,
		Support: SupportSupported, Classification: ClassificationWritable, Owner: "user-settings", Writable: true,
		Validator: "user-settings", Authority: "user.self", ChangeTiming: "future_views", SettingsHref: "/settings/preferences",
	}
}

func DefaultRegistry() (*Registry, error) {
	return NewRegistry(DefaultDomainDescriptors())
}

func profileDomain() DomainDescriptor {
	return DomainDescriptor{
		Domain:       "profiles",
		ResourceType: "agent_profile",
		Label:        "Agent profiles",
		Description:  "Saved agent profile settings used for future sessions.",
		Owner:        "agent-settings",
		Scope:        ScopeResource,
		Target:       TargetRules{Scope: ScopeResource, RequiresResourceID: true, AllowsWorkspaceID: true},
		SettingsHref: "/settings/agents",
		Operations: []OperationDescriptor{{
			Name:        "update",
			Binding:     "agent-settings.profile.update",
			Authority:   "org.config.manage",
			Description: "Validate and update one saved profile through the agent settings controller.",
		}},
		Fields: []FieldDescriptor{
			profileWritable("agent_profile.name", "name", "Profile name", "string", "The saved display name.", false),
			profileWritable("agent_profile.model", "model", "Model", "string", "Preferred model. Empty inherits the provider default.", false),
			profileWritable("agent_profile.fallback_model", "fallback_model", "Fallback model", "string", "Optional fallback model.", false),
			profileWritable("agent_profile.auto_fallback", "auto_fallback", "Automatic fallback", "boolean", "Enable automatic fallback behavior.", false),
			profileWritable("agent_profile.mode", "mode", "Mode", "string", "Agent operating mode.", false),
			profileWritable("agent_profile.config_options", "config_options", "Configuration options", "object", "Typed provider options. Supplied maps replace the saved map.", true),
			profileWritable("agent_profile.allow_indexing", "allow_indexing", "Allow indexing", "boolean", "Legacy compatibility permission.", false),
			profileWritable("agent_profile.auto_approve", "auto_approve", "Auto approve", "boolean", "Automatically approve supported agent permissions.", false),
			profileWritable("agent_profile.cli_passthrough", "cli_passthrough", "CLI passthrough", "boolean", "Enable CLI passthrough mode.", false),
			profileWritable("agent_profile.enabled", "enabled", "Enabled", "boolean", "Allow this profile for new work.", false),
			profileWritable("agent_profile.cli_flags", "cli_flags", "CLI flags", "array", "Complete replacement list of validated CLI flags.", true),
			profileWritableSensitive("agent_profile.env_vars", "env_vars", "Environment variables", "array", "Complete replacement list. Secret values remain references.", true),
			profileWritable("agent_profile.command_prefix", "command_prefix", "Command prefix", "string", "Optional validated launcher prefix.", false),
			profileWritable("agent_profile.dynamic", "dynamic", "Dynamic routing", "object", "Versioned dynamic routing document.", true),
		},
	}
}

func profileMCPDomain() DomainDescriptor {
	return DomainDescriptor{
		Domain:       "profiles",
		ResourceType: "agent_profile_mcp",
		Label:        "Profile MCP documents",
		Description:  "Separate MCP server settings for a saved agent profile.",
		Owner:        "agent-settings",
		Scope:        ScopeResource,
		Target:       TargetRules{Scope: ScopeResource, RequiresResourceID: true},
		SettingsHref: "/settings/agents",
		Operations:   []OperationDescriptor{{Name: "update", Binding: "agent-settings.profile-mcp.update", Authority: "org.config.manage"}},
		Fields: []FieldDescriptor{
			profileWritable("agent_profile_mcp.enabled", "enabled", "MCP enabled", "boolean", "Enable the profile MCP document.", false),
			profileWritableSensitive("agent_profile_mcp.servers", "servers", "MCP servers", "object", "Complete replacement map of validated MCP servers.", true),
			{Key: "agent_profile_mcp.meta", FieldPath: "meta", Label: "MCP metadata", Description: "Preserved metadata from the MCP document.", JSONType: "object", Support: SupportReadOnly, Classification: ClassificationComputed, Owner: "agent-settings", Sensitive: true},
		},
	}
}

func profileWritable(key, path, label, jsonType, description string, replacement bool) FieldDescriptor {
	return FieldDescriptor{
		Key:            key,
		FieldPath:      path,
		Label:          label,
		Description:    description,
		JSONType:       jsonType,
		Support:        SupportSupported,
		Classification: ClassificationWritable,
		Owner:          "agent-settings",
		Writable:       true,
		Validator:      "agent-settings.profile",
		Authority:      "org.config.manage",
		Replacement:    replacement,
		ChangeTiming:   "future_sessions",
		SettingsHref:   "/settings/agents",
	}
}

func profileWritableSensitive(key, path, label, jsonType, description string, replacement bool) FieldDescriptor {
	field := profileWritable(key, path, label, jsonType, description, replacement)
	field.Sensitive = true
	return field
}
