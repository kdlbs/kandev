export type SettingsCoverageStatus = "supported" | "exception" | "pending";

export type SettingsExceptionCategory =
  | "client-local"
  | "read-only"
  | "interactive-only"
  | "action"
  | "deployment-owned"
  | "plugin-owned"
  | "separate-product-contract";

export type SettingsCoverageEvidence = {
  id: string;
  domain: string;
  fieldPath: string;
  sourcePaths: string[];
  status: SettingsCoverageStatus;
  owner: string;
  exceptionCategory?: SettingsExceptionCategory;
  reason?: string;
  recovery?: string;
};

// i18n-exempt: settings coverage resource identifiers are protocol metadata, not UI copy.
export const SETTINGS_COVERAGE_RESOURCE_TYPES: Readonly<Record<string, readonly string[]>> = {
  agents: ["agent"],
  profiles: ["agent_profile", "agent_profile_mcp"],
  user_preferences: ["user_settings"],
  workflows: ["workflow", "workflow_step"],
  workspaces: ["workspace", "repository", "repository_set", "repository_script"],
  execution: ["executor", "executor_profile", "environment"],
  tasks: ["task"],
  prompts: ["prompt"],
  utilities: ["utility_agent"],
  editors: ["editor"],
  notifications: ["notification_provider"],
  issue_integrations: [
    "issue_integration",
    "jira_settings",
    "jira_issue_watch",
    "linear_settings",
    "linear_issue_watch",
    "sentry_instance",
    "sentry_issue_watch",
  ],
  code_host_integrations: [
    "code_host_integration",
    "github_settings",
    "github_review_watch",
    "github_issue_watch",
    "gitlab_settings",
    "gitlab_review_watch",
    "gitlab_issue_watch",
    "azure_devops_settings",
    "azure_devops_work_item_watch",
    "azure_devops_pull_request_watch",
  ],
  automation: ["automation", "automation_trigger"],
  runtime: ["runtime_flag"],
  storage: ["storage_maintenance"],
};

const source = (...paths: string[]) => paths;

const PROFILE_TASK = "task-01-profile-contract";
const PROFILE_CRUD_SOURCE = "apps/backend/internal/agent/settings/controller/profile_crud.go";
const PROFILE_DTO_SOURCE = "apps/backend/internal/agent/settings/dto/dto.go";
const USER_TASK = "task-05-user-preferences";
const USER_DTO_SOURCE = "apps/backend/internal/user/dto/dto.go";
const WORKFLOW_TASK = "task-06-workflow-settings";
const WORKSPACE_TASK = "task-07-workspace-settings";
const WORKSPACE_SETTINGS_SOURCE =
  "apps/web/components/settings/workspaces/workspace-settings-shell.tsx";
const TASK_REQUESTS_SOURCE = "apps/backend/internal/task/service/service_requests.go";

const supported = (
  id: string,
  domain: string,
  fieldPath: string,
  sourcePaths: string[],
  owner: string,
): SettingsCoverageEvidence => ({
  id,
  domain,
  fieldPath,
  sourcePaths,
  status: "supported",
  owner,
});

type SettingsExceptionInput = {
  id: string;
  domain: string;
  fieldPath: string;
  category: SettingsExceptionCategory;
  reason: string;
  recovery: string;
  sourcePaths: string[];
};

const exception = ({
  id,
  domain,
  fieldPath,
  category,
  reason,
  recovery,
  sourcePaths,
}: SettingsExceptionInput): SettingsCoverageEvidence => ({
  id,
  domain,
  fieldPath,
  sourcePaths,
  status: "exception",
  owner: "explicit-exception",
  exceptionCategory: category,
  reason,
  recovery,
});

// This list is intentionally maintained from UI and domain inputs rather than
// generated from the runtime catalog. The adapter slices in this change bind
// each listed eligible control to the compact settings contract.
export const SETTINGS_COVERAGE_INVENTORY: SettingsCoverageEvidence[] = [
  supported(
    "agent-definition-fields",
    "agents",
    "settings",
    source(
      "apps/web/components/settings/agents",
      "apps/backend/internal/agent/settings/controller",
    ),
    "task-01-profile-contract",
  ),
  supported(
    "agent-profile-name",
    "agent_profile",
    "name",
    source(
      "apps/web/components/settings/profile-edit/profile-details-card.tsx",
      PROFILE_CRUD_SOURCE,
    ),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-model",
    "agent_profile",
    "model",
    source("apps/web/components/settings/profile-model-fields.tsx", PROFILE_CRUD_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-fallback-model",
    "agent_profile",
    "fallback_model",
    source("apps/web/components/settings/profile-model-config.ts", PROFILE_CRUD_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-auto-fallback",
    "agent_profile",
    "auto_fallback",
    source("apps/web/components/settings/profile-model-config.ts", PROFILE_CRUD_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-mode",
    "agent_profile",
    "mode",
    source("apps/web/components/settings/profile-form-fields.tsx", PROFILE_DTO_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-config-options",
    "agent_profile",
    "config_options",
    source(
      "apps/web/components/settings/profile-advanced-options.tsx",
      "apps/backend/internal/agent/settings/profileconfig/profileconfig.go",
    ),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-cli-flags",
    "agent_profile",
    "cli_flags",
    source("apps/web/components/settings/cli-flags-field.tsx", PROFILE_DTO_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-environment-variables",
    "agent_profile",
    "env_vars",
    source("apps/web/components/settings/profile-edit/env-vars-card.tsx", PROFILE_DTO_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-command-prefix",
    "agent_profile",
    "command_prefix",
    source("apps/web/components/settings/command-prefix-field.tsx", PROFILE_CRUD_SOURCE),
    PROFILE_TASK,
  ),
  supported(
    "agent-profile-mcp-document",
    "agent_profile_mcp",
    "servers",
    source(
      "apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx",
      "apps/backend/internal/agent/mcpconfig/service.go",
    ),
    "task-02-profile-mutations",
  ),
  supported(
    "user-settings-task-behavior",
    "user_settings",
    "confirm_task_archive",
    source("apps/web/components/settings/task-behavior-settings.tsx", USER_DTO_SOURCE),
    USER_TASK,
  ),
  supported(
    "user-settings-keyboard-shortcuts",
    "user_settings",
    "keyboard_shortcuts",
    source("apps/web/components/settings/keyboard-shortcuts-card.tsx", USER_DTO_SOURCE),
    USER_TASK,
  ),
  supported(
    "user-settings-terminal",
    "user_settings",
    "terminal_font_size",
    source("apps/web/components/settings/terminal-editors-settings.tsx", USER_DTO_SOURCE),
    USER_TASK,
  ),
  supported(
    "user-settings-saved-layouts",
    "user_settings",
    "saved_layouts",
    source("apps/web/components/settings/layouts/layout-settings.tsx", USER_DTO_SOURCE),
    USER_TASK,
  ),
  supported(
    "user-settings-utility-defaults",
    "user_settings",
    "default_utility_model",
    source("apps/web/components/settings/utility-agents-settings.tsx", USER_DTO_SOURCE),
    USER_TASK,
  ),
  supported(
    "workflow-name",
    "workflow",
    "name",
    source(WORKSPACE_SETTINGS_SOURCE, "apps/backend/internal/workflow/service"),
    WORKFLOW_TASK,
  ),
  supported(
    "workflow-description",
    "workflow",
    "description",
    source(WORKSPACE_SETTINGS_SOURCE, "apps/backend/internal/workflow/service"),
    WORKFLOW_TASK,
  ),
  supported(
    "workflow-step-fields",
    "workflow_step",
    "settings",
    source(
      "apps/web/components/settings/workflow-step-settings.tsx",
      "apps/backend/internal/mcp/handlers/config_workflow_handlers.go",
    ),
    WORKFLOW_TASK,
  ),
  supported(
    "workspace-name",
    "workspace",
    "name",
    source(WORKSPACE_SETTINGS_SOURCE, "apps/backend/internal/task/service/service_resources.go"),
    WORKSPACE_TASK,
  ),
  supported(
    "workspace-default-executor",
    "workspace",
    "default_executor_id",
    source(WORKSPACE_SETTINGS_SOURCE, TASK_REQUESTS_SOURCE),
    WORKSPACE_TASK,
  ),
  supported(
    "workspace-default-agent-profile",
    "workspace",
    "default_agent_profile_id",
    source(WORKSPACE_SETTINGS_SOURCE, TASK_REQUESTS_SOURCE),
    WORKSPACE_TASK,
  ),
  supported(
    "repository-fields",
    "repository",
    "settings",
    source("apps/web/components/settings/repository-card.tsx", TASK_REQUESTS_SOURCE),
    WORKSPACE_TASK,
  ),
  supported(
    "repository-set-fields",
    "repository_set",
    "settings",
    source(
      "apps/web/components/settings/repository-set-card.tsx",
      "apps/backend/internal/task/service",
    ),
    WORKSPACE_TASK,
  ),
  supported(
    "repository-script-fields",
    "repository_script",
    "settings",
    source(
      "apps/web/components/settings/repository-custom-scripts.tsx",
      "apps/backend/internal/scriptengine",
    ),
    WORKSPACE_TASK,
  ),
  supported(
    "executor-fields",
    "executor",
    "settings",
    source("apps/web/components/settings/executor-profile-dialog.tsx", TASK_REQUESTS_SOURCE),
    "task-08-executor-settings",
  ),
  supported(
    "executor-profile-fields",
    "executor_profile",
    "settings",
    source(
      "apps/web/components/settings/profile-edit/profile-details-card.tsx",
      TASK_REQUESTS_SOURCE,
    ),
    "task-08-executor-settings",
  ),
  supported(
    "executor-environment-fields",
    "environment",
    "settings",
    source(
      "apps/web/components/settings/profile-edit/env-vars-card.tsx",
      "apps/backend/internal/task/models",
    ),
    "task-08-executor-settings",
  ),
  supported(
    "task-settings-fields",
    "task",
    "settings",
    source(
      "apps/web/components/settings/task-behavior-settings.tsx",
      "apps/backend/internal/mcp/handlers/config_task_handlers.go",
    ),
    "task-09-task-settings",
  ),
  supported(
    "saved-prompt-name",
    "prompt",
    "name",
    source(
      "apps/web/components/settings/prompts-settings.tsx",
      "apps/backend/internal/prompts/models",
    ),
    "task-10-prompt-settings",
  ),
  supported(
    "saved-prompt-content",
    "prompt",
    "content",
    source(
      "apps/web/components/settings/settings-prompt-editor.tsx",
      "apps/backend/internal/prompts/service",
    ),
    "task-10-prompt-settings",
  ),
  supported(
    "utility-agent-fields",
    "utility_agent",
    "settings",
    source(
      "apps/web/components/settings/utility-agents-settings.tsx",
      "apps/backend/internal/utility/service",
    ),
    "task-11-utility-settings",
  ),
  supported(
    "editor-definition-fields",
    "editor",
    "settings",
    source(
      "apps/web/components/settings/editors-settings.tsx",
      "apps/backend/internal/editors/service",
    ),
    "task-12-editor-settings",
  ),
  supported(
    "notification-provider-fields",
    "notification_provider",
    "settings",
    source(
      "apps/web/components/settings/notifications-settings.tsx",
      "apps/backend/internal/notifications/service",
    ),
    "task-13-notification-settings",
  ),
  supported(
    "issue-integration-fields",
    "issue_integrations",
    "settings",
    source(
      "apps/web/components/settings/integrations",
      "apps/backend/internal/jira",
      "apps/backend/internal/linear",
    ),
    "task-14-issue-integration-settings",
  ),
  supported(
    "code-host-integration-fields",
    "code_host_integrations",
    "settings",
    source(
      "apps/web/components/settings/integrations",
      "apps/backend/internal/github",
      "apps/backend/internal/gitlab",
      "apps/backend/internal/azuredevops",
    ),
    "task-15-codehost-settings",
  ),
  supported(
    "automation-fields",
    "automation",
    "settings",
    source("apps/web/app/settings/workspace/[id]/automations", "apps/backend/internal/automation"),
    "task-16-automation-settings",
  ),
  supported(
    "runtime-flag-override",
    "runtime_flag",
    "override",
    source(
      "apps/web/components/settings/system/feature-toggles-route.tsx",
      "apps/backend/internal/runtimeflags/registry.go",
    ),
    "task-17-runtime-settings",
  ),
  supported(
    "storage-maintenance-settings",
    "storage_maintenance",
    "settings",
    source(
      "apps/web/components/settings/system/storage/storage-maintenance-settings.tsx",
      "apps/backend/internal/system/storage/settings.go",
    ),
    "task-18-storage-settings",
  ),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "user-settings-pane-size",
    domain: "user_settings",
    fieldPath: "pane_sizes",
    category: "client-local",
    reason: "Stored in the browser layout owner.",
    recovery: "Use the local Settings control on this device.",
    sourcePaths: source("apps/web/components/settings/layouts/layout-settings.tsx"),
  }),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "startup-yaml",
    domain: "startup_configuration",
    fieldPath: "config.yaml",
    category: "deployment-owned",
    reason: "Startup configuration belongs to the deployment source catalog.",
    recovery: "Edit the deployment configuration and restart through the operator workflow.",
    sourcePaths: source("apps/backend/internal/common/config/catalog.go"),
  }),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "credential-enrollment",
    domain: "integration_credentials",
    fieldPath: "credentials",
    category: "interactive-only",
    reason: "Credential enrollment requires the provider authorization flow.",
    recovery: "Use the integration connection flow in Settings.",
    sourcePaths: source("apps/web/components/settings/integrations"),
  }),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "plugin-settings",
    domain: "plugin",
    fieldPath: "settings",
    category: "plugin-owned",
    reason: "Plugin settings are owned by the installed plugin.",
    recovery: "Open the plugin-owned Settings page.",
    sourcePaths: source("apps/web/components/settings/plugins"),
  }),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "runtime-health",
    domain: "runtime",
    fieldPath: "health",
    category: "read-only",
    reason: "Runtime health is computed state, not saved configuration.",
    recovery: "Use the System Status page.",
    sourcePaths: source("apps/web/components/settings/system"),
  }),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "workflow-reorder",
    domain: "workflow",
    fieldPath: "position",
    category: "action",
    reason: "Reordering is an explicit lifecycle operation.",
    recovery: "Use the workflow reorder action.",
    sourcePaths: source("apps/backend/internal/mcp/server/config_handlers.go"),
  }),
  // i18n-exempt: settings discovery metadata is consumed by agents, not rendered UI.
  exception({
    id: "office-organization",
    domain: "office",
    fieldPath: "organization",
    category: "separate-product-contract",
    reason: "Office organization management has its own contract.",
    recovery: "Use the Office organization surface.",
    sourcePaths: source("apps/backend/internal/office"),
  }),
];

export function validateSettingsCoverageInventory(entries: SettingsCoverageEvidence[]): string[] {
  const errors: string[] = [];
  for (const entry of entries) {
    if (!entry.owner.trim()) errors.push(`${entry.id}: owner is required`);
    if (entry.status === "pending") {
      errors.push(`${entry.id}: eligible fields cannot remain pending`);
    }
    if (entry.status === "exception") {
      if (!entry.exceptionCategory) errors.push(`${entry.id}: exception category is required`);
      if (!entry.reason?.trim()) errors.push(`${entry.id}: exception reason is required`);
      if (!entry.recovery?.trim()) errors.push(`${entry.id}: exception recovery is required`);
    }
  }
  return errors;
}

export const SETTINGS_COVERAGE_DOMAIN_COUNT = new Set(
  SETTINGS_COVERAGE_INVENTORY.map((entry) => entry.domain),
).size;
