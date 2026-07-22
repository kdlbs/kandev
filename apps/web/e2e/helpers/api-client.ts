import type {
  Workspace,
  Workflow,
  CreateTaskResponse,
  ListWorkflowsResponse,
  ListWorkflowStepsResponse,
  TaskSessionState,
  TaskCreateLastUsedApi,
  MCPTaskAgentProfileDefault,
} from "../../lib/types/http";
import type { Agent, AgentProfile } from "../../lib/types/http-agents";
import type { TaskCIAutomationOptions, TaskCIAutomationPatch } from "../../lib/types/github";
import type { VoiceModeSettings } from "../../lib/types/http-voice";
import type {
  GitLabMRApproval,
  GitLabMRCommit,
  GitLabMRDiscussion,
  GitLabMRFile,
  GitLabPipeline,
  GitLabProjectMember,
  Issue as MockGitLabIssue,
  MR as MockGitLabMR,
  TaskMR,
} from "../../lib/types/gitlab";
import type {
  SSHAgentReadinessResponse,
  SSHProbeShellsResponse,
  SSHSession,
  SSHTestRequest,
  SSHTestResult,
} from "../../lib/types/http-ssh";

// --- GitHub Mock Types ---

export type MockPR = {
  number: number;
  title: string;
  state: string;
  head_branch: string;
  head_sha?: string;
  base_branch: string;
  author_login: string;
  repo_owner: string;
  repo_name: string;
  html_url?: string;
  url?: string;
  body?: string;
  draft?: boolean;
  mergeable?: boolean;
  mergeable_state?: string;
  additions?: number;
  deletions?: number;
  merged_at?: string;
  requested_reviewers?: Array<{ login: string; type: string }>;
};

export type MockIssue = {
  number: number;
  title: string;
  body?: string;
  url?: string;
  html_url?: string;
  state?: string;
  author_login?: string;
  repo_owner: string;
  repo_name: string;
  labels?: string[];
  assignees?: string[];
  created_at?: string;
  updated_at?: string;
  closed_at?: string | null;
};

export type MockOrg = {
  login: string;
  avatar_url?: string;
};

export type MockRepo = {
  full_name: string;
  owner: string;
  name: string;
  private?: boolean;
};

export type MockReview = {
  id: number;
  author: string;
  author_avatar?: string;
  state: string;
  body?: string;
  created_at?: string;
};

export type MockCheckRun = {
  name: string;
  source?: string;
  status: string;
  conclusion?: string;
  html_url?: string;
  started_at?: string;
  completed_at?: string;
};

export type MockGitHubConnectionStatus = "active" | "invalid" | "suspended" | "revoked";

export type MockGitHubWorkspaceConnection = {
  source: "pat" | "gh_cli" | "github_app_installation" | "legacy_shared";
  status: MockGitHubConnectionStatus;
  login?: string;
  installation_id?: number;
  installation_account_login?: string;
  installation_account_type?: string;
  app_registration_id?: string;
  capabilities?: Record<string, boolean>;
};

export type MockGitHubPersonalConnection = {
  login: string;
  status: MockGitHubConnectionStatus;
  github_user_id?: number;
  access_expires_at?: string;
};

export type MockGitHubCLIAccount = {
  host: string;
  login: string;
  active: boolean;
  state: string;
};

export type MockGitHubAppRegistration = {
  id: string;
  display_name: string;
  app_id: number;
};

export type MockGitLabMRSeed = Pick<MockGitLabMR, "iid" | "title"> & Partial<MockGitLabMR>;
export type MockGitLabIssueSeed = Pick<MockGitLabIssue, "iid" | "title"> & Partial<MockGitLabIssue>;

function setIf(body: Record<string, unknown>, key: string, value: unknown) {
  if (value !== undefined && value !== null) body[key] = value;
}

type CreateTaskOpts = {
  description?: string;
  workflow_id?: string;
  workflow_step_id?: string;
  agent_profile_id?: string;
  repository_ids?: string[];
  repositories?: Array<{ repository_id: string; base_branch?: string; checkout_branch?: string }>;
  plan_mode?: boolean;
  metadata?: Record<string, unknown>;
  parent_id?: string;
  workspace_mode?: "inherit_parent" | "new_workspace" | "shared_group";
  workspace_group_id?: string;
  attachments?: MessageAttachmentInput[];
};

function buildTaskMetadata(opts: CreateTaskOpts): Record<string, unknown> | undefined {
  const meta: Record<string, unknown> = { ...(opts.metadata ?? {}) };
  if (opts.agent_profile_id && meta.agent_profile_id == null) {
    meta.agent_profile_id = opts.agent_profile_id;
  }
  return Object.keys(meta).length > 0 ? meta : undefined;
}

function buildCreateTaskBody(
  workspaceId: string,
  title: string,
  opts?: CreateTaskOpts,
): Record<string, unknown> {
  const options = opts ?? {};
  const body: Record<string, unknown> = {
    workspace_id: workspaceId,
    title,
    description: options.description ?? "",
  };
  setIf(body, "workflow_id", options.workflow_id);
  setIf(body, "workflow_step_id", options.workflow_step_id);
  setIf(body, "metadata", buildTaskMetadata(options));
  setIf(
    body,
    "repositories",
    options.repositories ?? options.repository_ids?.map((id) => ({ repository_id: id })),
  );
  setIf(body, "attachments", options.attachments);
  if (options.plan_mode) body.plan_mode = true;
  setIf(body, "parent_id", options.parent_id);
  setIf(body, "workspace_mode", options.workspace_mode);
  setIf(body, "workspace_group_id", options.workspace_group_id);
  return body;
}

type MessageAttachmentInput = {
  type: string;
  data: string;
  mime_type: string;
  name?: string;
  delivery_mode?: "prompt" | "path";
};

type OptionalAgentTaskOpts = {
  workflow_id?: string;
  workflow_step_id?: string;
  repository_ids?: string[];
  repositories?: Array<{ repository_id: string; base_branch?: string; checkout_branch?: string }>;
  executor_id?: string;
  executor_profile_id?: string;
  metadata?: Record<string, unknown>;
  parent_id?: string;
  attachments?: MessageAttachmentInput[];
};

/** `repositories` (with per-entry branches) takes precedence over the shorthand
 * `repository_ids` so callers can pin a non-default base_branch. */
function pickRepositories(opts: OptionalAgentTaskOpts): unknown {
  if (opts.repositories) return opts.repositories;
  if (opts.repository_ids) return opts.repository_ids.map((id) => ({ repository_id: id }));
  return undefined;
}

/** Build the optional fields object for createTaskWithAgent requests. */
function buildOptionalAgentTaskFields(opts?: OptionalAgentTaskOpts): Record<string, unknown> {
  const fields: Record<string, unknown> = {};
  if (!opts) return fields;
  setIf(fields, "workflow_id", opts.workflow_id);
  setIf(fields, "workflow_step_id", opts.workflow_step_id);
  setIf(fields, "repositories", pickRepositories(opts));
  setIf(fields, "executor_id", opts.executor_id);
  setIf(fields, "executor_profile_id", opts.executor_profile_id);
  setIf(fields, "metadata", opts.metadata);
  setIf(fields, "parent_id", opts.parent_id);
  setIf(fields, "attachments", opts.attachments);
  return fields;
}

/**
 * HTTP API client for seeding test data via the backend REST API.
 */
export class ApiClient {
  constructor(private baseUrl: string) {}

  /** Perform an HTTP request and return the raw Response (does not throw on non-2xx). */
  async rawRequest(
    method: string,
    path: string,
    body?: unknown,
    options?: Pick<RequestInit, "redirect">,
  ): Promise<Response> {
    return fetch(`${this.baseUrl}${path}`, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
      ...options,
    });
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API ${method} ${path} failed (${res.status}): ${text}`);
    }
    return res.json() as Promise<T>;
  }

  private async activeWorkspaceId(): Promise<string | undefined> {
    const { settings } = await this.getUserSettings();
    const workspaceId = settings.workspace_id;
    return typeof workspaceId === "string" && workspaceId.trim() !== ""
      ? workspaceId.trim()
      : undefined;
  }

  private async withActiveWorkspace(path: string, workspaceId?: string): Promise<string> {
    const resolved = workspaceId?.trim() || (await this.activeWorkspaceId());
    if (!resolved) return path;
    const separator = path.includes("?") ? "&" : "?";
    return `${path}${separator}workspace_id=${encodeURIComponent(resolved)}`;
  }

  async healthCheck(): Promise<void> {
    await this.request("GET", "/health");
  }

  async createWorkspace(name: string): Promise<Workspace> {
    return this.request("POST", "/api/v1/workspaces", { name });
  }

  async deleteWorkspace(workspaceId: string, confirmName: string): Promise<void> {
    await this.request("DELETE", `/api/v1/workspaces/${workspaceId}`, {
      confirm_name: confirmName,
    });
  }

  async listWorkspaces(): Promise<{ workspaces: Workspace[]; total: number }> {
    return this.request("GET", "/api/v1/workspaces");
  }

  async createWorkflow(workspaceId: string, name: string, templateId?: string): Promise<Workflow> {
    return this.request("POST", "/api/v1/workflows", {
      workspace_id: workspaceId,
      name,
      ...(templateId ? { workflow_template_id: templateId } : {}),
    });
  }

  /**
   * Seed a workflow with an explicit style (kanban / office / custom) via the
   * KANDEV_E2E_MOCK test harness. Production has no HTTP path that creates an
   * office-style workflow (the normal create endpoint always normalises to
   * kanban), so this is the only way to stand up an office workflow for the
   * "exclude office from settings export" coverage (issue #1109).
   */
  async seedWorkflow(
    workspaceId: string,
    name: string,
    style: "kanban" | "office" | "custom",
  ): Promise<{ workflow_id: string }> {
    return this.request("POST", "/api/v1/_test/workflows", {
      workspace_id: workspaceId,
      name,
      style,
    });
  }

  /**
   * Seed a task row directly via the test harness, bypassing the service-layer
   * subtask-depth guard. Use this (not `createTask`) to build nested chains
   * deeper than one level — `createTask` rejects depth > 1 for kanban tasks.
   */
  async seedTask(
    workspaceId: string,
    title: string,
    opts?: {
      workflow_id?: string;
      workflow_step_id?: string;
      parent_id?: string;
      state?: string;
    },
  ): Promise<{ task_id: string }> {
    return this.request("POST", "/api/v1/_test/tasks", {
      workspace_id: workspaceId,
      title,
      workflow_id: opts?.workflow_id ?? "",
      workflow_step_id: opts?.workflow_step_id ?? "",
      parent_id: opts?.parent_id ?? "",
      state: opts?.state ?? "",
    });
  }

  async reorderWorkflows(
    workspaceId: string,
    workflowIds: string[],
  ): Promise<{ success: boolean }> {
    return this.request("PUT", `/api/v1/workspaces/${workspaceId}/workflows/reorder`, {
      workflow_ids: workflowIds,
    });
  }

  async createTask(
    workspaceId: string,
    title: string,
    opts?: {
      description?: string;
      workflow_id?: string;
      workflow_step_id?: string;
      /** Stored in task.Metadata so auto_start_agent can pick it up on on_enter. */
      agent_profile_id?: string;
      /** Repository IDs to associate with the task (required for agent execution). */
      repository_ids?: string[];
      /** Full repository entries with optional checkout_branch / base_branch. */
      repositories?: Array<{
        repository_id: string;
        base_branch?: string;
        checkout_branch?: string;
      }>;
      /** When true, task is placed at position 0 regardless of is_start_step. */
      plan_mode?: boolean;
      /** Extra metadata to store on the task. */
      metadata?: Record<string, unknown>;
      /** Parent task ID for subtasks. */
      parent_id?: string;
      /** Workspace sharing policy used by parent/child task trees. */
      workspace_mode?: "inherit_parent" | "new_workspace" | "shared_group";
      /** Existing group required when workspace_mode is shared_group. */
      workspace_group_id?: string;
      attachments?: MessageAttachmentInput[];
    },
  ): Promise<CreateTaskResponse> {
    return this.request("POST", "/api/v1/tasks", buildCreateTaskBody(workspaceId, title, opts));
  }

  async updateTaskState(
    taskId: string,
    state: "BACKLOG" | "IN_PROGRESS" | "REVIEW" | "COMPLETED" | "FAILED" | "CANCELLED",
  ): Promise<void> {
    await this.request("PATCH", `/api/v1/tasks/${taskId}`, { state });
  }

  /** Rename a task. Used in tests that exercise live-title resolution on the
   *  cross-task message badge. */
  async updateTaskTitle(taskId: string, title: string): Promise<void> {
    await this.request("PATCH", `/api/v1/tasks/${taskId}`, { title });
  }

  async listAgents(): Promise<{ agents: Agent[]; total: number }> {
    return this.request("GET", "/api/v1/agents");
  }

  async deleteAgentProfile(profileId: string, force?: boolean): Promise<void> {
    const qs = force ? "?force=true" : "";
    await this.request("DELETE", `/api/v1/agent-profiles/${profileId}${qs}`);
  }

  /**
   * Delete kanban-only agent profiles except the ones in keepIds.
   *
   * Office-scoped profiles (those with a non-empty `workspace_id`) are
   * always preserved — they belong to onboarded office workspaces and
   * are managed via the office agent endpoints, not by this helper.
   * Without this guard the per-test cleanup deletes the seeded CEO and
   * cascades into "No agents yet" failures across office tests.
   */
  async cleanupTestProfiles(keepIds: string[]): Promise<void> {
    const { agents } = await this.listAgents();
    for (const agent of agents) {
      for (const profile of agent.profiles ?? []) {
        const wsId = (profile as unknown as { workspace_id?: string }).workspace_id;
        if (wsId) continue;
        if (!keepIds.includes(profile.id)) {
          await this.deleteAgentProfile(profile.id, true);
        }
      }
    }
  }

  async createAgentProfile(
    agentId: string,
    name: string,
    opts: {
      model: string;
      mode?: string;
      config_options?: Record<string, string>;
      cli_passthrough?: boolean;
      cli_flags?: Array<{ description: string; flag: string; enabled: boolean }>;
      env_vars?: Array<{ key: string; value?: string; secret_id?: string }>;
    },
  ): Promise<{
    id: string;
    cli_flags: Array<{ description: string; flag: string; enabled: boolean }>;
  }> {
    return this.request("POST", `/api/v1/agents/${agentId}/profiles`, {
      name,
      model: opts.model,
      mode: opts.mode,
      config_options: opts.config_options,
      cli_passthrough: opts.cli_passthrough ?? false,
      cli_flags: opts.cli_flags,
      env_vars: opts.env_vars,
    });
  }

  async getAgentProfile(profileId: string): Promise<AgentProfile> {
    // The profile does not have its own GET endpoint; fetch via listAgents
    // and find the matching row. Keeps the helper surface small.
    const { agents } = await this.listAgents();
    for (const agent of agents) {
      for (const profile of agent.profiles ?? []) {
        if (profile.id === profileId) {
          return profile;
        }
      }
    }
    throw new Error(`profile ${profileId} not found`);
  }

  async updateAgentProfile(
    profileId: string,
    patch: {
      name?: string;
      model?: string;
      mode?: string;
      config_options?: Record<string, string>;
      cli_passthrough?: boolean;
      cli_flags?: Array<{ description: string; flag: string; enabled: boolean }>;
      env_vars?: Array<{ key: string; value?: string; secret_id?: string }>;
    },
  ): Promise<void> {
    await this.request("PATCH", `/api/v1/agent-profiles/${profileId}`, patch);
  }

  async listPrompts(): Promise<{
    prompts: Array<{ id: string; name: string; content: string; builtin: boolean }>;
  }> {
    return this.request("GET", "/api/v1/prompts");
  }

  async createPrompt(
    name: string,
    content: string,
  ): Promise<{ id: string; name: string; content: string; builtin: boolean }> {
    return this.request("POST", "/api/v1/prompts", { name, content });
  }

  async updatePrompt(
    promptId: string,
    patch: { name?: string; content?: string },
  ): Promise<{ id: string; name: string; content: string; builtin: boolean }> {
    return this.request("PATCH", `/api/v1/prompts/${promptId}`, patch);
  }

  async deletePrompt(promptId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/prompts/${promptId}`);
  }

  async createTaskWithAgent(
    workspaceId: string,
    title: string,
    agentProfileId: string,
    opts?: {
      description?: string;
      workflow_id?: string;
      workflow_step_id?: string;
      repository_ids?: string[];
      /** Full repository entries with optional checkout_branch / base_branch. */
      repositories?: Array<{
        repository_id: string;
        base_branch?: string;
        checkout_branch?: string;
      }>;
      executor_id?: string;
      executor_profile_id?: string;
      metadata?: Record<string, unknown>;
      /** Parent task ID for subtasks. */
      parent_id?: string;
      attachments?: MessageAttachmentInput[];
    },
  ): Promise<CreateTaskResponse> {
    return this.request("POST", "/api/v1/tasks", {
      workspace_id: workspaceId,
      title,
      description: opts?.description ?? "",
      start_agent: true,
      agent_profile_id: agentProfileId,
      ...buildOptionalAgentTaskFields(opts),
    });
  }

  async startQuickChat(
    workspaceId: string,
    agentProfileId: string,
    title: string,
  ): Promise<{ task_id: string; session_id: string }> {
    return this.request("POST", `/api/v1/workspaces/${workspaceId}/quick-chat`, {
      title,
      agent_profile_id: agentProfileId,
    });
  }

  /** Start a config chat session via the dedicated config-chat endpoint. */
  async startConfigChat(
    workspaceId: string,
    agentProfileId: string,
    prompt: string,
  ): Promise<{ task_id: string; session_id: string }> {
    return this.request("POST", `/api/v1/workspaces/${workspaceId}/config-chat`, {
      agent_profile_id: agentProfileId,
      prompt,
    });
  }

  async listWorkflows(workspaceId: string): Promise<ListWorkflowsResponse> {
    return this.request("GET", `/api/v1/workspaces/${workspaceId}/workflows`);
  }

  async listWorkflowSteps(workflowId: string): Promise<ListWorkflowStepsResponse> {
    return this.request("GET", `/api/v1/workflows/${workflowId}/workflow/steps`);
  }

  async createWorkflowStep(
    workflowId: string,
    name: string,
    position: number,
    opts?: { is_start_step?: boolean },
  ): Promise<{ id: string }> {
    return this.request("POST", `/api/v1/workflow/steps`, {
      workflow_id: workflowId,
      name,
      position,
      ...(opts?.is_start_step != null ? { is_start_step: opts.is_start_step } : {}),
    });
  }

  async createRepository(
    workspaceId: string,
    localPath: string,
    defaultBranch = "main",
    opts?: {
      name?: string;
      provider?: string;
      provider_host?: string;
      provider_owner?: string;
      provider_name?: string;
    },
  ): Promise<{ id: string }> {
    return this.request("POST", `/api/v1/workspaces/${workspaceId}/repositories`, {
      name: opts?.name ?? "E2E Repo",
      source_type: "local",
      local_path: localPath,
      default_branch: defaultBranch,
      ...(opts?.provider ? { provider: opts.provider } : {}),
      ...(opts?.provider_host ? { provider_host: opts.provider_host } : {}),
      ...(opts?.provider_owner ? { provider_owner: opts.provider_owner } : {}),
      ...(opts?.provider_name ? { provider_name: opts.provider_name } : {}),
    });
  }

  async updateRepository(
    repositoryId: string,
    updates: {
      provider?: string;
      provider_host?: string;
      provider_owner?: string;
      provider_name?: string;
      dev_script?: string;
      setup_script?: string;
      cleanup_script?: string;
      copy_files?: string;
    },
  ): Promise<void> {
    await this.request("PATCH", `/api/v1/repositories/${repositoryId}`, updates);
  }

  async createRepositoryScript(
    repositoryId: string,
    name: string,
    command: string,
    position = 0,
  ): Promise<{
    id: string;
    repository_id: string;
    name: string;
    command: string;
    position: number;
  }> {
    return this.request("POST", `/api/v1/repositories/${repositoryId}/scripts`, {
      name,
      command,
      position,
    });
  }

  async createExecutor(
    name: string,
    type: string,
  ): Promise<{ id: string; name: string; type: string }> {
    return this.request("POST", "/api/v1/executors", { name, type });
  }

  async updateWorkspace(
    workspaceId: string,
    updates: {
      default_executor_id?: string;
      default_agent_profile_id?: string;
      default_config_agent_profile_id?: string;
    },
  ): Promise<void> {
    await this.request("PATCH", `/api/v1/workspaces/${workspaceId}`, updates);
  }

  async deleteExecutor(executorId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/executors/${executorId}`);
  }

  async createExecutorProfile(
    executorId: string,
    nameOrPayload:
      | string
      | {
          name: string;
          mcp_policy?: string;
          prepare_script?: string;
          cleanup_script?: string;
          config?: Record<string, string>;
          env_vars?: Array<{ key: string; value?: string; secret_id?: string }>;
        },
    opts?: { mcp_policy?: string; prepare_script?: string; cleanup_script?: string },
  ): Promise<{ id: string; name: string }> {
    if (typeof nameOrPayload === "object") {
      return this.request("POST", `/api/v1/executors/${executorId}/profiles`, nameOrPayload);
    }
    return this.request("POST", `/api/v1/executors/${executorId}/profiles`, {
      name: nameOrPayload,
      ...(opts?.mcp_policy ? { mcp_policy: opts.mcp_policy } : {}),
      ...(opts?.prepare_script ? { prepare_script: opts.prepare_script } : {}),
      ...(opts?.cleanup_script ? { cleanup_script: opts.cleanup_script } : {}),
    });
  }

  async deleteExecutorProfile(profileId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/executor-profiles/${profileId}`);
  }

  async getExecutorProfile(
    executorId: string,
    profileId: string,
  ): Promise<{
    id: string;
    name: string;
    config?: Record<string, string>;
    prepare_script?: string;
    cleanup_script?: string;
  }> {
    return this.request("GET", `/api/v1/executors/${executorId}/profiles/${profileId}`);
  }

  async listExecutors(): Promise<{
    executors: Array<{
      id: string;
      name: string;
      type: string;
      profiles?: Array<{ id: string; name: string }>;
    }>;
  }> {
    return this.request("GET", "/api/v1/executors");
  }

  async getUserSettings(): Promise<{
    settings: {
      terminal_link_behavior?: string;
      terminal_font_family?: string;
      terminal_font_size?: number;
      mcp_task_agent_profile_default?: MCPTaskAgentProfileDefault;
      [key: string]: unknown;
    };
  }> {
    return this.request("GET", "/api/v1/user/settings");
  }

  async saveUserSettings(settings: {
    enable_preview_on_click?: boolean;
    confirm_task_archive?: boolean;
    mcp_task_agent_profile_default?: MCPTaskAgentProfileDefault;
    workspace_id?: string;
    workflow_filter_id?: string;
    repository_ids?: string[];
    terminal_link_behavior?: "new_tab" | "browser_panel";
    terminal_font_family?: string;
    terminal_font_size?: number;
    keyboard_shortcuts?: Record<string, unknown>;
    default_utility_agent_id?: string;
    default_utility_model?: string;
    sidebar_views?: unknown[];
    saved_layouts?: unknown[];
    kanban_view_mode?: string;
    tasks_list_sort?: string;
    tasks_list_group?: string;
    task_create_last_used?: TaskCreateLastUsedApi;
    voice_mode?: VoiceModeSettings;
  }): Promise<void> {
    await this.request("PATCH", "/api/v1/user/settings", settings);
  }

  async moveTask(taskId: string, workflowId: string, workflowStepId: string): Promise<void> {
    await this.request("POST", `/api/v1/tasks/${taskId}/move`, {
      workflow_id: workflowId,
      workflow_step_id: workflowStepId,
    });
  }

  async updateWorkflow(
    workflowId: string,
    updates: { name?: string; description?: string; agent_profile_id?: string },
  ): Promise<Workflow> {
    return this.request("PATCH", `/api/v1/workflows/${workflowId}`, updates);
  }

  async updateWorkflowStep(
    stepId: string,
    updates: {
      prompt?: string;
      agent_profile_id?: string;
      events?: {
        on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_exit?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_comment?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_blocker_resolved?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_children_completed?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_approval_resolved?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_heartbeat?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_budget_alert?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_agent_error?: Array<{ type: string; config?: Record<string, unknown> }>;
      };
      wip_limit?: number;
      pull_from_step_id?: string | null;
    },
  ): Promise<void> {
    await this.request("PUT", `/api/v1/workflow/steps/${stepId}`, { id: stepId, ...updates });
  }

  async deleteWorkflow(workflowId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/workflows/${workflowId}`);
  }

  async deleteWorkflowStep(stepId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/workflow/steps/${stepId}`);
  }

  async listWorkflowTemplates(): Promise<{
    templates: Array<{ id: string; name: string; default_steps?: Array<{ name: string }> }>;
  }> {
    return this.request("GET", "/api/v1/workflow/templates");
  }

  // --- Workflow Export/Import ---

  async exportWorkflow(workflowId: string): Promise<string> {
    const res = await this.rawRequest("GET", `/api/v1/workflows/${workflowId}/export`);
    if (!res.ok) throw new Error(`Export failed (${res.status}): ${await res.text()}`);
    return res.text();
  }

  async exportAllWorkflows(workspaceId: string): Promise<string> {
    const res = await this.rawRequest("GET", `/api/v1/workspaces/${workspaceId}/workflows/export`);
    if (!res.ok) throw new Error(`Export failed (${res.status}): ${await res.text()}`);
    return res.text();
  }

  async importWorkflows(
    workspaceId: string,
    yamlContent: string,
  ): Promise<{ created: string[]; skipped: string[] }> {
    const res = await fetch(`${this.baseUrl}/api/v1/workspaces/${workspaceId}/workflows/import`, {
      method: "POST",
      headers: { "Content-Type": "application/x-yaml" },
      body: yamlContent,
    });
    if (!res.ok) throw new Error(`Import failed (${res.status}): ${await res.text()}`);
    return res.json() as Promise<{ created: string[]; skipped: string[] }>;
  }

  async deleteTask(taskId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/tasks/${taskId}`);
  }

  async archiveTask(taskId: string): Promise<void> {
    await this.request("POST", `/api/v1/tasks/${taskId}/archive`);
  }

  async getAgentProfileMcpConfig(
    profileId: string,
  ): Promise<{ profile_id: string; enabled: boolean; servers: Record<string, unknown> }> {
    return this.request("GET", `/api/v1/agent-profiles/${profileId}/mcp-config`);
  }

  // --- E2E Test Reset ---

  async e2eReset(workspaceId: string, keepWorkflowIds?: string[]): Promise<void> {
    const params = keepWorkflowIds?.length ? `?keep_workflows=${keepWorkflowIds.join(",")}` : "";
    await this.request("DELETE", `/api/v1/e2e/reset/${workspaceId}${params}`);
  }

  /**
   * Creates a hidden (system-only) workflow. Mirrors the path that
   * improve-kandev's bootstrap takes when ensuring its workflow exists,
   * without depending on the gh CLI or repo cloning.
   */
  async e2eCreateHiddenWorkflow(
    workspaceId: string,
    name: string,
  ): Promise<{ id: string; workspace_id: string; name: string; hidden: boolean }> {
    return this.request("POST", "/api/v1/e2e/hidden-workflow", {
      workspace_id: workspaceId,
      name,
    });
  }

  // --- E2E Mock Harness (KANDEV_E2E_MOCK=true) ---
  // These routes are mounted only when the backend was started with the
  // env var set. They write directly to task_sessions / messages so the
  // live-presence UI can be exercised without launching a real executor.

  async seedTaskSession(
    taskId: string,
    opts: {
      state: TaskSessionState;
      sessionId?: string;
      agentProfileId?: string;
      startedAt?: string;
      completedAt?: string;
      commandCount?: number;
      metadata?: Record<string, unknown>;
    },
  ): Promise<{ session_id: string }> {
    const body: Record<string, unknown> = {
      task_id: taskId,
      state: opts.state,
    };
    if (opts.sessionId !== undefined) body.session_id = opts.sessionId;
    if (opts.agentProfileId !== undefined) body.agent_profile_id = opts.agentProfileId;
    if (opts.startedAt !== undefined) body.started_at = opts.startedAt;
    if (opts.completedAt !== undefined) body.completed_at = opts.completedAt;
    if (opts.commandCount !== undefined) body.command_count = opts.commandCount;
    if (opts.metadata !== undefined) body.metadata = opts.metadata;
    return this.request("POST", "/api/v1/_test/task-sessions", body);
  }

  async seedSessionMessage(
    sessionId: string,
    opts: {
      type: string;
      content?: string;
      metadata?: Record<string, unknown>;
    },
  ): Promise<void> {
    const body: Record<string, unknown> = { session_id: sessionId, type: opts.type };
    if (opts.content !== undefined) body.content = opts.content;
    if (opts.metadata !== undefined) body.metadata = opts.metadata;
    await this.request("POST", "/api/v1/_test/messages", body);
  }

  async seedToolCallMessages(sessionId: string, count: number): Promise<void> {
    for (let i = 0; i < count; i++) {
      await this.seedSessionMessage(sessionId, {
        type: "tool_call",
        content: `synthetic tool call ${i + 1}`,
      });
    }
  }

  /**
   * Seed `count` agent text messages (type "message"), oldest-to-newest.
   * Unlike `seedToolCallMessages`, these are `message`-type rows so the chat's
   * newest-window fetch contains a user/agent message and does not trigger the
   * auto-backfill that would otherwise pull older pages on its own.
   */
  async seedAgentMessages(
    sessionId: string,
    count: number,
    prefix = "filler message",
  ): Promise<void> {
    for (let i = 0; i < count; i++) {
      await this.seedSessionMessage(sessionId, {
        type: "message",
        content: `${prefix} ${i + 1}`,
      });
    }
  }

  async testHarnessHealth(): Promise<{ ok: boolean }> {
    return this.request("GET", "/api/v1/_test/health");
  }

  /** Set desired_skills on an agent_profiles row via the e2e harness. */
  async setProfileDesiredSkills(profileId: string, slugs: string[]): Promise<void> {
    await this.request("POST", `/api/v1/_test/agent-profiles/${profileId}/desired-skills`, {
      slugs,
    });
  }

  async seedAgentFailure(opts: {
    taskId: string;
    agentProfileId: string;
    errorMessage?: string;
  }): Promise<{ run_id: string; consecutive_failures: number; threshold: number }> {
    const payload: Record<string, unknown> = {
      task_id: opts.taskId,
      agent_profile_id: opts.agentProfileId,
    };
    if (opts.errorMessage !== undefined) payload.error_message = opts.errorMessage;
    return this.request("POST", "/api/v1/_test/agent-failures", payload);
  }

  async seedRunEvent(opts: {
    runId: string;
    eventType: string;
    level?: "info" | "warn" | "error" | "debug";
    payload?: string;
  }): Promise<{ ok: boolean }> {
    const body: Record<string, unknown> = {
      run_id: opts.runId,
      event_type: opts.eventType,
    };
    if (opts.level !== undefined) body.level = opts.level;
    if (opts.payload !== undefined) body.payload = opts.payload;
    return this.request("POST", "/api/v1/_test/run-events", body);
  }

  async seedRunSkillSnapshot(opts: {
    runId: string;
    skillId: string;
    version?: string;
    contentHash?: string;
    materializedPath?: string;
  }): Promise<{ ok: boolean }> {
    const body: Record<string, unknown> = {
      run_id: opts.runId,
      skill_id: opts.skillId,
    };
    if (opts.version !== undefined) body.version = opts.version;
    if (opts.contentHash !== undefined) body.content_hash = opts.contentHash;
    if (opts.materializedPath !== undefined) body.materialized_path = opts.materializedPath;
    return this.request("POST", "/api/v1/_test/run-skills", body);
  }

  async seedComment(opts: {
    taskId: string;
    authorType: "user" | "agent";
    authorId: string;
    body: string;
    source?: string;
    createdAt?: string;
  }): Promise<{ comment_id: string }> {
    const payload: Record<string, unknown> = {
      task_id: opts.taskId,
      author_type: opts.authorType,
      author_id: opts.authorId,
      body: opts.body,
    };
    if (opts.source !== undefined) payload.source = opts.source;
    if (opts.createdAt !== undefined) payload.created_at = opts.createdAt;
    return this.request("POST", "/api/v1/_test/comments", payload);
  }

  // --- GitHub Mock Control ---

  async mockGitHubReset(): Promise<void> {
    await this.request("DELETE", "/api/v1/github/mock/reset");
  }

  async mockGitHubSetUser(username: string): Promise<void> {
    await this.request("PUT", "/api/v1/github/mock/user", { username });

    const workspaceId = await this.activeWorkspaceId();
    if (!workspaceId) return;

    await this.mockGitHubSetWorkspaceConnection(workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
  }

  async mockGitHubSetWorkspaceConnection(
    workspaceId: string,
    connection: MockGitHubWorkspaceConnection,
  ): Promise<void> {
    await this.request(
      "PUT",
      `/api/v1/github/mock/workspace-connections/${encodeURIComponent(workspaceId)}`,
      connection,
    );
  }

  async mockGitHubDeleteWorkspaceConnection(workspaceId: string): Promise<void> {
    await this.request(
      "DELETE",
      `/api/v1/github/mock/workspace-connections/${encodeURIComponent(workspaceId)}`,
    );
  }

  async mockGitHubSetWorkspaceConnectionStatus(
    workspaceId: string,
    status: MockGitHubConnectionStatus,
  ): Promise<void> {
    await this.request(
      "PUT",
      `/api/v1/github/mock/workspace-connections/${encodeURIComponent(workspaceId)}/status`,
      { status },
    );
  }

  async mockGitHubSetPersonalConnection(
    workspaceId: string,
    connection: MockGitHubPersonalConnection,
  ): Promise<void> {
    await this.request(
      "PUT",
      `/api/v1/github/mock/personal-connections/${encodeURIComponent(workspaceId)}`,
      connection,
    );
  }

  async mockGitHubDeletePersonalConnection(workspaceId: string): Promise<void> {
    await this.request(
      "DELETE",
      `/api/v1/github/mock/personal-connections/${encodeURIComponent(workspaceId)}`,
    );
  }

  async mockGitHubSetCLIAccounts(accounts: MockGitHubCLIAccount[]): Promise<void> {
    await this.request("PUT", "/api/v1/github/mock/cli-accounts", { accounts });
  }

  async mockGitHubSetAppRegistration(registration: MockGitHubAppRegistration) {
    return this.request<Record<string, unknown>>(
      "PUT",
      `/api/v1/github/mock/app-registrations/${encodeURIComponent(registration.id)}`,
      { display_name: registration.display_name, app_id: registration.app_id },
    );
  }

  async mockGitHubAddPRs(prs: MockPR[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/prs", { prs });
    await this.seedMockGitHubRepositoryAccess(prs);
  }

  async mockGitHubAddIssues(issues: MockIssue[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/issues", { issues });
    await this.seedMockGitHubRepositoryAccess(issues);
  }

  async mockGitHubAddOrgs(orgs: MockOrg[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/orgs", { orgs });
  }

  async mockGitHubAddRepos(org: string, repos: MockRepo[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/repos", { org, repos });
  }

  private async seedMockGitHubRepositoryAccess(
    items: Array<{ repo_owner: string; repo_name: string }>,
  ): Promise<void> {
    const reposByOwner = new Map<string, Map<string, MockRepo>>();
    for (const item of items) {
      const owner = item.repo_owner.trim();
      const name = item.repo_name.trim();
      if (!owner || !name) continue;
      const repos = reposByOwner.get(owner) ?? new Map<string, MockRepo>();
      repos.set(name, { full_name: `${owner}/${name}`, owner, name });
      reposByOwner.set(owner, repos);
    }
    await Promise.all(
      Array.from(reposByOwner, ([owner, repos]) =>
        this.mockGitHubAddRepos(owner, Array.from(repos.values())),
      ),
    );
  }

  async mockGitHubAddReviews(
    owner: string,
    repo: string,
    number: number,
    reviews: MockReview[],
  ): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/reviews", {
      owner,
      repo,
      number,
      reviews,
    });
  }

  async mockGitHubAddCheckRuns(
    owner: string,
    repo: string,
    ref: string,
    checks: MockCheckRun[],
  ): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/checks", {
      owner,
      repo,
      ref,
      checks,
    });
  }

  async mockGitHubAddPRFiles(
    owner: string,
    repo: string,
    number: number,
    files: Array<{
      filename: string;
      status: string;
      additions: number;
      deletions: number;
      patch?: string;
      old_path?: string;
    }>,
  ): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/files", {
      owner,
      repo,
      number,
      files,
    });
  }

  async mockGitHubAddPRCommits(
    owner: string,
    repo: string,
    number: number,
    commits: Array<{
      sha: string;
      message: string;
      author_login: string;
      author_date: string;
    }>,
  ): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/commits", {
      owner,
      repo,
      number,
      commits,
    });
    await this.seedMockGitHubRepositoryAccess([{ repo_owner: owner, repo_name: repo }]);

    // PR commit fixtures predate workspace-scoped GitHub authentication and
    // expect the shared mock client to be available for provider lookups.
    const workspaceId = await this.activeWorkspaceId();
    if (!workspaceId) return;
    await Promise.all([
      this.mockGitHubSetWorkspaceConnection(workspaceId, {
        source: "legacy_shared",
        status: "active",
      }),
      this.mockGitHubSetPersonalConnection(workspaceId, {
        login: "e2e-test",
        status: "active",
        github_user_id: 1,
        access_expires_at: "2100-01-01T00:00:00Z",
      }),
    ]);
  }

  async mockGitHubAddBranches(
    owner: string,
    repo: string,
    branches: Array<{ name: string }>,
  ): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/branches", {
      owner,
      repo,
      branches,
    });

    // Branch fixtures predate workspace-scoped GitHub authentication and
    // expect the shared mock client to be available for provider lookups.
    const workspaceId = await this.activeWorkspaceId();
    if (!workspaceId) return;
    await this.mockGitHubSetWorkspaceConnection(workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
  }

  async mockGitHubAssociateTaskPR(data: {
    task_id: string;
    owner: string;
    repo: string;
    pr_number: number;
    pr_url: string;
    pr_title: string;
    head_branch: string;
    base_branch: string;
    author_login: string;
    state?: string;
    review_state?: string;
    checks_state?: string;
    mergeable_state?: string;
    additions?: number;
    deletions?: number;
    review_count?: number;
    pending_review_count?: number;
    required_reviews?: number;
    checks_total?: number;
    checks_passing?: number;
    unresolved_review_threads?: number;
  }): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/task-prs", data);
  }

  async getTaskCIAutomationOptions(taskId: string): Promise<TaskCIAutomationOptions> {
    return this.request("GET", `/api/v1/github/tasks/${encodeURIComponent(taskId)}/ci-options`);
  }

  async updateTaskCIAutomationOptions(
    taskId: string,
    patch: TaskCIAutomationPatch,
  ): Promise<TaskCIAutomationOptions> {
    return this.request(
      "PATCH",
      `/api/v1/github/tasks/${encodeURIComponent(taskId)}/ci-options`,
      patch,
    );
  }

  async getTaskPR(taskId: string): Promise<{
    task_id: string;
    pr_number: number;
    state: string;
    review_state: string;
    checks_state: string;
    mergeable_state: string;
    review_count: number;
    pending_review_count: number;
    required_reviews?: number | null;
  } | null> {
    const res = await this.rawRequest("GET", `/api/v1/github/task-prs/${taskId}`);
    if (res.status === 404) return null;
    if (!res.ok) {
      throw new Error(`getTaskPR failed (${res.status}): ${await res.text()}`);
    }
    return res.json();
  }

  async mockGitHubSeedPRFeedback(data: {
    owner: string;
    repo: string;
    pr_number: number;
    checks?: Array<{
      name: string;
      source?: string;
      status?: string;
      conclusion?: string;
      html_url?: string;
      output?: string;
      started_at?: string | null;
      completed_at?: string | null;
    }>;
    reviews?: Array<{
      id: number;
      author: string;
      author_avatar?: string;
      state: string;
      body?: string;
      created_at?: string;
    }>;
    comments?: Array<{
      id: number;
      author: string;
      author_avatar?: string;
      body: string;
      path?: string;
      line?: number;
      side?: string;
      comment_type?: string;
      created_at?: string;
      updated_at?: string;
    }>;
  }): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/pr-feedback", data);
    await this.seedMockGitHubRepositoryAccess([{ repo_owner: data.owner, repo_name: data.repo }]);
  }

  async mockGitHubSetAuthHealth(data: { authenticated: boolean; error?: string }): Promise<void> {
    await this.request("PUT", "/api/v1/github/mock/auth-health", data);
  }

  /**
   * Toggles the mock client's "list accessible repos unavailable" branch.
   * When set to true, GET /api/v1/github/repos responds with 503
   * `github_not_configured` — used by Remote-tab e2e specs that need to
   * verify the "Connect GitHub" banner in the chip popover.
   */
  async mockGitHubSetReposUnavailable(unavailable: boolean): Promise<void> {
    await this.request("PUT", "/api/v1/github/mock/repos-unavailable", { unavailable });
  }

  async mockGitHubGetStatus(): Promise<{
    authenticated: boolean;
    username: string;
    auth_method: string;
  }> {
    return this.request("GET", "/api/v1/github/status");
  }

  // --- GitLab Mock Control ---

  private gitLabWorkspacePath(path: string, workspaceId: string): string {
    const separator = path.includes("?") ? "&" : "?";
    return `${path}${separator}workspace_id=${encodeURIComponent(workspaceId)}`;
  }

  async configureGitLab(
    workspaceId: string,
    host = "https://gitlab.com",
    token = `e2e-gitlab:${workspaceId}`,
  ): Promise<void> {
    await this.request("PUT", this.gitLabWorkspacePath("/api/v1/gitlab/config", workspaceId), {
      host,
      auth_method: "pat",
      token,
    });
  }

  async configureGitLabRepositoryRemote(repositoryId: string, remoteUrl: string): Promise<void> {
    await this.request("PUT", `/api/v1/_test/repositories/${repositoryId}/git-remote`, {
      remote_url: remoteUrl,
    });
  }

  async clearGitLabRepositoryRemote(repositoryId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/_test/repositories/${repositoryId}/git-remote`);
  }

  async getGitLabPushRecord(repositoryId: string): Promise<{ args: string }> {
    return this.request("GET", `/api/v1/_test/repositories/${repositoryId}/git-push-record`);
  }

  async mockGitLabReset(workspaceId: string): Promise<void> {
    await this.request(
      "DELETE",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/reset", workspaceId),
    );
  }

  async mockGitLabSetUser(workspaceId: string, username: string): Promise<void> {
    await this.request("PUT", this.gitLabWorkspacePath("/api/v1/gitlab/mock/user", workspaceId), {
      username,
    });
  }

  async mockGitLabAddMRs(
    workspaceId: string,
    project: string,
    mrs: MockGitLabMRSeed[],
  ): Promise<void> {
    await this.request("POST", this.gitLabWorkspacePath("/api/v1/gitlab/mock/mrs", workspaceId), {
      project,
      mrs,
    });
  }

  async mockGitLabAddIssues(
    workspaceId: string,
    project: string,
    issues: MockGitLabIssueSeed[],
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/issues", workspaceId),
      { project, issues },
    );
  }

  async mockGitLabAddPipelines(
    workspaceId: string,
    project: string,
    pipelines: GitLabPipeline[],
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/pipelines", workspaceId),
      { project, pipelines },
    );
  }

  async mockGitLabAddDiscussions(
    workspaceId: string,
    project: string,
    iid: number,
    discussions: GitLabMRDiscussion[],
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/discussions", workspaceId),
      { project, iid, discussions },
    );
  }

  async mockGitLabAddApprovals(
    workspaceId: string,
    project: string,
    iid: number,
    approvals: GitLabMRApproval[],
    required: number,
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/approvals", workspaceId),
      { project, iid, approvals, required },
    );
  }

  async mockGitLabAddBranches(
    workspaceId: string,
    project: string,
    branches: Array<{ name: string }>,
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/branches", workspaceId),
      { project, branches },
    );
  }

  async mockGitLabAddMembers(
    workspaceId: string,
    project: string,
    members: GitLabProjectMember[],
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/members", workspaceId),
      { project, members },
    );
  }

  async mockGitLabAddFiles(
    workspaceId: string,
    project: string,
    iid: number,
    files: GitLabMRFile[],
  ): Promise<void> {
    await this.request("POST", this.gitLabWorkspacePath("/api/v1/gitlab/mock/files", workspaceId), {
      project,
      iid,
      files,
    });
  }

  async mockGitLabAddCommits(
    workspaceId: string,
    project: string,
    iid: number,
    commits: GitLabMRCommit[],
  ): Promise<void> {
    await this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/mock/commits", workspaceId),
      { project, iid, commits },
    );
  }

  async linkTaskGitLabMR(
    workspaceId: string,
    data: { task_id: string; repository_id?: string; mr_url: string },
  ): Promise<TaskMR> {
    return this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/task-mrs", workspaceId),
      data,
    );
  }

  async createGitLabReviewWatch(data: {
    workspace_id: string;
    workflow_id: string;
    workflow_step_id: string;
    agent_profile_id: string;
    executor_profile_id: string;
    projects?: Array<{ path: string }>;
    prompt?: string;
    enabled?: boolean;
  }): Promise<{ id: string; workspace_id: string }> {
    return this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/watches/review", data.workspace_id),
      {
        ...data,
        projects: data.projects ?? [],
        prompt: data.prompt ?? "Review {{mr.url}}",
        review_scope: "user_and_teams",
        custom_query: "",
        enabled: data.enabled ?? true,
        poll_interval_seconds: 300,
        cleanup_policy: "auto",
      },
    );
  }

  async createGitLabIssueWatch(data: {
    workspace_id: string;
    workflow_id: string;
    workflow_step_id: string;
    agent_profile_id: string;
    executor_profile_id: string;
    projects?: Array<{ path: string }>;
    prompt?: string;
    enabled?: boolean;
  }): Promise<{ id: string; workspace_id: string }> {
    return this.request(
      "POST",
      this.gitLabWorkspacePath("/api/v1/gitlab/watches/issue", data.workspace_id),
      {
        ...data,
        projects: data.projects ?? [],
        prompt: data.prompt ?? "Investigate {{issue.url}}",
        labels: [],
        custom_query: "",
        enabled: data.enabled ?? true,
        poll_interval_seconds: 300,
        cleanup_policy: "auto",
      },
    );
  }

  // --- Session ---

  async listSessionMessages(sessionId: string): Promise<{
    messages: Array<{
      id: string;
      content: string;
      author_type: string;
      raw_content?: string;
      metadata?: Record<string, unknown>;
    }>;
  }> {
    return this.request("GET", `/api/v1/task-sessions/${sessionId}/messages`);
  }

  async listTasks(
    workspaceId: string,
  ): Promise<{ tasks: Array<{ id: string; title: string; workflow_step_id?: string }> }> {
    return this.request("GET", `/api/v1/workspaces/${workspaceId}/tasks`);
  }

  async listTaskSessions(taskId: string): Promise<{
    sessions: Array<{
      id: string;
      task_id: string;
      agent_profile_id?: string;
      state: string;
      started_at: string;
      task_environment_id?: string;
      worktree_path?: string;
      worktree_branch?: string;
      error_message?: string;
      metadata?: Record<string, unknown>;
    }>;
    total: number;
  }> {
    return this.request("GET", `/api/v1/tasks/${taskId}/sessions`);
  }

  async setPrimarySession(sessionId: string): Promise<void> {
    await this.request("POST", `/api/v1/task-sessions/${sessionId}/set-primary`);
  }

  async deleteSession(sessionId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/task-sessions/${sessionId}`);
  }

  async getTask(taskId: string): Promise<{
    id: string;
    title: string;
    primary_session_id?: string | null;
    state?: string;
    workflow_step_id?: string;
    parent_id?: string;
    metadata?: Record<string, unknown> | null;
    repositories?: Array<{
      id: string;
      task_id: string;
      repository_id: string;
      base_branch: string;
      checkout_branch?: string;
      position: number;
    }>;
  }> {
    return this.request("GET", `/api/v1/tasks/${taskId}`);
  }

  async getTaskPlan(taskId: string): Promise<{
    task_id: string;
    content: string;
    title?: string;
    created_by: string;
    updated_at: string;
    implementation_started_at?: string | null;
    implementation_started_session_id?: string | null;
    implementation_started_by?: string | null;
  } | null> {
    try {
      return await this.wsRequest("task.plan.get", { task_id: taskId });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      // Backend returns "plan not found" when no plan exists for the task.
      if (message.includes("plan not found")) return null;
      throw error;
    }
  }

  /**
   * Send a one-shot WebSocket request and await the matching response.
   * Used for actions exposed only over WS (e.g. session.launch). Opens a
   * fresh connection, awaits one response by id, then closes.
   */
  async wsRequest<T>(action: string, payload: unknown, timeoutMs = 30_000): Promise<T> {
    const wsUrl = this.baseUrl.replace(/^http/, "ws") + "/ws";
    const id = `e2e-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const ws = new WebSocket(wsUrl);
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        ws.close();
        reject(new Error(`wsRequest("${action}") timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      ws.addEventListener("open", () => {
        ws.send(
          JSON.stringify({
            id,
            type: "request",
            action,
            payload,
            timestamp: new Date().toISOString(),
          }),
        );
      });
      ws.addEventListener("message", (event) => {
        const msg = JSON.parse(String(event.data));
        if (msg.id !== id) return;
        clearTimeout(timer);
        ws.close();
        if (msg.type === "error") {
          reject(new Error(`wsRequest("${action}") failed: ${msg.payload?.message ?? msg.action}`));
          return;
        }
        resolve(msg.payload as T);
      });
      ws.addEventListener("error", (event) => {
        clearTimeout(timer);
        reject(new Error(`wsRequest("${action}") socket error: ${String(event)}`));
      });
    });
  }

  /**
   * Launch a new session on a task — wraps the WS `session.launch` action.
   * Used by tests that need a second session on a task with an existing
   * environment (multi-session reuse, recovery scenarios).
   */
  async launchSession(
    payload: {
      task_id: string;
      agent_profile_id: string;
      executor_id?: string;
      executor_profile_id?: string;
      prompt: string;
      intent?: string;
      workflow_step_id?: string;
      auto_start?: boolean;
    },
    timeoutMs = 30_000,
  ): Promise<{ session_id: string; agent_execution_id: string; state: string }> {
    return this.wsRequest("session.launch", payload, timeoutMs);
  }

  /** Stop a running session via WS `session.stop` — same path the UI uses. */
  async stopSession(payload: {
    session_id: string;
    reason?: string;
    force?: boolean;
  }): Promise<{ success: boolean }> {
    return this.wsRequest("session.stop", payload);
  }

  async getTaskEnvironment(taskId: string): Promise<{
    id: string;
    task_id: string;
    executor_type?: string;
    executor_profile_id?: string;
    container_id?: string;
    sandbox_id?: string;
    worktree_id?: string;
    worktree_path?: string;
    workspace_path?: string;
    status: string;
  } | null> {
    const res = await this.rawRequest("GET", `/api/v1/tasks/${taskId}/environment`);
    if (res.status === 404) return null;
    if (!res.ok) {
      throw new Error(`getTaskEnvironment failed (${res.status}): ${await res.text()}`);
    }
    return res.json();
  }

  // --- GitHub Review Watch ---

  async createReviewWatch(
    workspaceId: string,
    workflowId: string,
    workflowStepId: string,
    agentProfileId: string,
    opts?: {
      repos?: Array<{ owner: string; name: string }>;
      prompt?: string;
      review_scope?: string;
      poll_interval_seconds?: number;
      cleanup_policy?: "auto" | "always" | "never";
    },
  ): Promise<{ id: string }> {
    return this.request("POST", "/api/v1/github/watches/review", {
      workspace_id: workspaceId,
      workflow_id: workflowId,
      workflow_step_id: workflowStepId,
      agent_profile_id: agentProfileId,
      repos: opts?.repos ?? [],
      prompt: opts?.prompt ?? "",
      review_scope: opts?.review_scope ?? "user_and_teams",
      poll_interval_seconds: opts?.poll_interval_seconds ?? 300,
      cleanup_policy: opts?.cleanup_policy ?? "auto",
    });
  }

  async updateReviewWatch(
    watchId: string,
    workspaceId: string,
    patch: {
      enabled?: boolean;
      cleanup_policy?: "auto" | "always" | "never";
      prompt?: string;
      repos?: Array<{ owner: string; name: string }>;
    },
  ): Promise<void> {
    await this.request(
      "PUT",
      `/api/v1/github/watches/review/${watchId}?workspace_id=${encodeURIComponent(workspaceId)}`,
      patch,
    );
  }

  async deleteReviewWatch(watchId: string, workspaceId: string): Promise<void> {
    await this.request(
      "DELETE",
      `/api/v1/github/watches/review/${watchId}?workspace_id=${encodeURIComponent(workspaceId)}`,
    );
  }

  async triggerReviewWatch(
    watchId: string,
    workspaceId: string,
  ): Promise<{ new_prs: number; new_prs_found: number; cleaned?: number }> {
    const params = new URLSearchParams({ workspace_id: workspaceId });
    return this.request(
      "POST",
      `/api/v1/github/watches/review/${watchId}/trigger?${params}`,
      undefined,
    );
  }

  /**
   * Invokes the manual cleanup sweep — same code path the settings-page
   * "Clean up merged" button uses. Returns the number of tasks deleted.
   */
  async cleanupMergedReviewTasks(): Promise<{ deleted: number }> {
    return this.request("POST", "/api/v1/github/cleanup/review-tasks", undefined);
  }

  async cleanupClosedIssueTasks(): Promise<{ deleted: number }> {
    return this.request("POST", "/api/v1/github/cleanup/issue-tasks", undefined);
  }

  /**
   * Posts a user-authored message on an existing task session via the same WS
   * action the chat UI uses. The resulting message lacks the auto_start
   * metadata flag, so the cleanup loop counts it as real user engagement.
   * Use after the auto-started agent finishes so the session is in a state
   * that accepts new prompts.
   */
  async addUserMessage(
    taskId: string,
    sessionId: string,
    content: string,
    attachments?: MessageAttachmentInput[],
  ): Promise<void> {
    await this.wsRequest("message.add", {
      task_id: taskId,
      session_id: sessionId,
      content,
      attachments,
    });
  }

  async queueMessage(
    taskId: string,
    sessionId: string,
    content: string,
    attachments?: MessageAttachmentInput[],
  ): Promise<void> {
    await this.wsRequest("message.queue.add", {
      task_id: taskId,
      session_id: sessionId,
      content,
      attachments,
    });
  }

  // --- Integration config seeding (real API, not mock) ---

  async setJiraConfig(payload: {
    siteUrl: string;
    email: string;
    authMethod?: "api_token" | "pat" | "session_cookie";
    instanceType?: "cloud" | "server";
    defaultProjectKey?: string;
    secret: string;
    workspaceId?: string;
  }): Promise<unknown> {
    const { workspaceId, ...config } = payload;
    const path = await this.withActiveWorkspace("/api/v1/jira/config", workspaceId);
    return this.request("POST", path, {
      ...config,
      authMethod: payload.authMethod ?? "api_token",
      instanceType: payload.instanceType ?? "cloud",
    });
  }

  async setLinearConfig(payload: {
    secret: string;
    defaultTeamKey?: string;
    workspaceId?: string;
  }): Promise<unknown> {
    const { workspaceId, ...config } = payload;
    const path = await this.withActiveWorkspace("/api/v1/linear/config", workspaceId);
    return this.request("POST", path, {
      defaultTeamKey: "",
      ...config,
      authMethod: "api_key",
    });
  }

  async createSentryInstance(payload: {
    name: string;
    secret: string;
    url?: string;
    workspaceId?: string;
  }): Promise<{ id: string }> {
    const { workspaceId, ...config } = payload;
    const path = await this.withActiveWorkspace("/api/v1/sentry/instances", workspaceId);
    return this.request("POST", path, {
      authMethod: "auth_token",
      url: "https://sentry.io",
      ...config,
    });
  }

  /**
   * Poll until the integration config reports `lastOk: true`. SetConfig kicks
   * off an async auth-health probe in a goroutine, so the row's `lastOk` flips
   * shortly after — but with no synchronous signal. Tests that gate UI on
   * `useJiraAvailable` / `useLinearAvailable` need to await this before
   * navigating, otherwise the import bar can race the first render.
   */
  async waitForIntegrationAuthHealthy(
    integration: "jira" | "linear" | "sentry",
    options: number | { timeoutMs?: number; workspaceId?: string } = 5_000,
  ): Promise<void> {
    const timeoutMs = typeof options === "number" ? options : (options.timeoutMs ?? 5_000);
    const workspaceId = typeof options === "number" ? undefined : options.workspaceId;
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      if (await this.integrationReportsHealthy(integration, workspaceId)) return;
      await new Promise<void>((resolve) => setTimeout(resolve, 100));
    }
    throw new Error(`${integration} config never reported lastOk: true within ${timeoutMs}ms`);
  }

  // integrationReportsHealthy probes an integration's current auth-health.
  // Sentry is multi-instance (checks whether ANY instance is healthy); the
  // others expose a single workspace config.
  private async integrationReportsHealthy(
    integration: "jira" | "linear" | "sentry",
    workspaceId?: string,
  ): Promise<boolean> {
    if (integration === "sentry") {
      const path = await this.withActiveWorkspace("/api/v1/sentry/instances", workspaceId);
      const res = await this.rawRequest("GET", path);
      if (!res.ok || res.status !== 200) return false;
      const body = (await res.json()) as {
        instances?: Array<{ hasSecret?: boolean; lastOk?: boolean }>;
      };
      return (body.instances ?? []).some((i) => Boolean(i.hasSecret) && Boolean(i.lastOk));
    }
    const path = await this.withActiveWorkspace(`/api/v1/${integration}/config`, workspaceId);
    const res = await this.rawRequest("GET", path);
    if (!res.ok || res.status !== 200) return false;
    const cfg = (await res.json()) as { hasSecret?: boolean; lastOk?: boolean };
    return Boolean(cfg.hasSecret) && Boolean(cfg.lastOk);
  }

  // --- Azure DevOps Mock Control ---

  async mockAzureDevOpsSeed(state: unknown): Promise<void> {
    await this.request("POST", "/api/v1/azure-devops/mock/state", state);
  }

  async mockAzureDevOpsReset(): Promise<void> {
    await this.request("DELETE", "/api/v1/azure-devops/mock/state");
  }

  async setAzureDevOpsConfig(
    workspaceId: string,
    payload: { organizationUrl: string; pat: string },
  ): Promise<void> {
    await this.request(
      "POST",
      `/api/v1/azure-devops/config?workspace_id=${encodeURIComponent(workspaceId)}`,
      { ...payload, authMethod: "pat" },
    );
  }

  // --- Jira Mock Control ---

  async mockJiraReset(): Promise<void> {
    await this.request("DELETE", "/api/v1/jira/mock/reset");
  }

  async mockJiraSetAuthResult(result: {
    ok: boolean;
    accountId?: string;
    displayName?: string;
    email?: string;
    error?: string;
  }): Promise<void> {
    await this.request("PUT", "/api/v1/jira/mock/auth-result", result);
  }

  async mockJiraSetAuthHealth(args: {
    ok: boolean;
    error?: string;
    workspaceId?: string;
  }): Promise<void> {
    const { workspaceId, ...payload } = args;
    const path = await this.withActiveWorkspace("/api/v1/jira/mock/auth-health", workspaceId);
    await this.request("PUT", path, payload);
  }

  async mockJiraSetProjects(projects: MockJiraProject[]): Promise<void> {
    await this.request("POST", "/api/v1/jira/mock/projects", { projects });
  }

  async mockJiraSetProjectStatuses(projectKey: string, statuses: MockJiraStatus[]): Promise<void> {
    await this.request("POST", "/api/v1/jira/mock/project-statuses", {
      projectKey,
      statuses,
    });
  }

  async mockJiraAddTickets(tickets: MockJiraTicket[]): Promise<void> {
    await this.request("POST", "/api/v1/jira/mock/tickets", { tickets });
  }

  async mockJiraAddTransitions(
    ticketKey: string,
    transitions: MockJiraTransition[],
  ): Promise<void> {
    await this.request("POST", "/api/v1/jira/mock/transitions", {
      ticketKey,
      transitions,
    });
  }

  async mockJiraSetSearchHits(tickets: MockJiraTicket[]): Promise<void> {
    await this.request("POST", "/api/v1/jira/mock/search-hits", { tickets });
  }

  async mockJiraSetGetTicketError(args: { statusCode: number; message: string }): Promise<void> {
    await this.request("PUT", "/api/v1/jira/mock/get-ticket-error", args);
  }

  // --- Linear Mock Control ---

  async mockLinearReset(): Promise<void> {
    await this.request("DELETE", "/api/v1/linear/mock/reset");
  }

  async mockLinearSetAuthResult(result: {
    ok: boolean;
    userId?: string;
    displayName?: string;
    email?: string;
    orgSlug?: string;
    orgName?: string;
    error?: string;
  }): Promise<void> {
    await this.request("PUT", "/api/v1/linear/mock/auth-result", result);
  }

  async mockLinearSetAuthHealth(args: {
    ok: boolean;
    error?: string;
    orgSlug?: string;
    workspaceId?: string;
  }): Promise<void> {
    const { workspaceId, ...payload } = args;
    const path = await this.withActiveWorkspace("/api/v1/linear/mock/auth-health", workspaceId);
    await this.request("PUT", path, payload);
  }

  async mockLinearSetTeams(teams: MockLinearTeam[]): Promise<void> {
    await this.request("POST", "/api/v1/linear/mock/teams", { teams });
  }

  async mockLinearSetStates(teamKey: string, states: MockLinearState[]): Promise<void> {
    await this.request("POST", "/api/v1/linear/mock/states", { teamKey, states });
  }

  async mockLinearAddIssues(issues: MockLinearIssue[]): Promise<void> {
    await this.request("POST", "/api/v1/linear/mock/issues", { issues });
  }

  async mockLinearSetGetIssueError(args: { statusCode: number; message: string }): Promise<void> {
    await this.request("PUT", "/api/v1/linear/mock/get-issue-error", args);
  }

  // --- Sentry Mock Control ---

  async mockSentryReset(): Promise<void> {
    await this.request("DELETE", "/api/v1/sentry/mock/reset");
  }

  // mockSentrySetAuthResult seeds the auth-check result for one instance's mock
  // dataset. Omit instanceId to target the pre-save ("") dataset used by
  // test-connection before an instance exists.
  async mockSentrySetAuthResult(
    result: { ok: boolean; userId?: string; displayName?: string; email?: string; error?: string },
    instanceId?: string,
  ): Promise<void> {
    const query = instanceId ? `?instanceId=${encodeURIComponent(instanceId)}` : "";
    await this.request("PUT", `/api/v1/sentry/mock/auth-result${query}`, result);
  }

  // mockSentrySetAuthHealth synchronously stamps last_ok/last_error on one
  // instance row, bypassing the async probe.
  async mockSentrySetAuthHealth(args: {
    instanceId: string;
    ok: boolean;
    error?: string;
  }): Promise<void> {
    const { instanceId, ...payload } = args;
    await this.request(
      "PUT",
      `/api/v1/sentry/mock/auth-health?instanceId=${encodeURIComponent(instanceId)}`,
      payload,
    );
  }

  async mockSentrySetOrganizations(
    instanceId: string,
    organizations: MockSentryOrganization[],
  ): Promise<void> {
    await this.request(
      "POST",
      `/api/v1/sentry/mock/organizations?instanceId=${encodeURIComponent(instanceId)}`,
      { organizations },
    );
  }

  async mockSentrySetProjects(instanceId: string, projects: MockSentryProject[]): Promise<void> {
    await this.request(
      "POST",
      `/api/v1/sentry/mock/projects?instanceId=${encodeURIComponent(instanceId)}`,
      { projects },
    );
  }

  // --- Sentry instance + watch CRUD ---

  async createSentryInstance(opts: {
    workspaceId: string;
    name: string;
    url?: string;
    secret: string;
    authMethod?: string;
  }): Promise<MockSentryInstance> {
    return this.request<MockSentryInstance>(
      "POST",
      `/api/v1/sentry/instances?workspace_id=${encodeURIComponent(opts.workspaceId)}`,
      {
        workspaceId: opts.workspaceId,
        name: opts.name,
        authMethod: opts.authMethod ?? "auth_token",
        url: opts.url ?? "https://sentry.io",
        secret: opts.secret,
      },
    );
  }

  async listSentryInstances(workspaceId: string): Promise<MockSentryInstance[]> {
    const { instances } = await this.request<{ instances: MockSentryInstance[] }>(
      "GET",
      `/api/v1/sentry/instances?workspace_id=${encodeURIComponent(workspaceId)}`,
    );
    return instances ?? [];
  }

  // deleteSentryInstanceRaw returns the raw Response so callers can assert the
  // 409 SENTRY_INSTANCE_IN_USE (with watchCount) when a watch still binds.
  async deleteSentryInstanceRaw(workspaceId: string, instanceId: string): Promise<Response> {
    return this.rawRequest(
      "DELETE",
      `/api/v1/sentry/instances/${encodeURIComponent(instanceId)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    );
  }

  // deleteAllSentryInstances removes every watch (FK-protected) then every
  // instance in a workspace — the per-test cleanup replacement for the removed
  // /config DELETE.
  async deleteAllSentryInstances(workspaceId?: string): Promise<void> {
    const watchesPath = await this.withActiveWorkspace("/api/v1/sentry/watches/issue", workspaceId);
    const watchesRes = await this.rawRequest("GET", watchesPath);
    if (watchesRes.ok) {
      const body = (await watchesRes.json()) as { watches?: Array<{ id: string }> };
      for (const w of body.watches ?? []) {
        const p = await this.withActiveWorkspace(
          `/api/v1/sentry/watches/issue/${encodeURIComponent(w.id)}`,
          workspaceId,
        );
        await this.rawRequest("DELETE", p).catch(() => undefined);
      }
    }
    const listPath = await this.withActiveWorkspace("/api/v1/sentry/instances", workspaceId);
    const res = await this.rawRequest("GET", listPath);
    if (!res.ok) return;
    const body = (await res.json()) as { instances?: Array<{ id: string }> };
    for (const instance of body.instances ?? []) {
      const p = await this.withActiveWorkspace(
        `/api/v1/sentry/instances/${encodeURIComponent(instance.id)}`,
        workspaceId,
      );
      await this.rawRequest("DELETE", p).catch(() => undefined);
    }
  }

  async createSentryIssueWatch(opts: {
    workspaceId: string;
    sentryInstanceId: string;
    workflowId: string;
    workflowStepId: string;
    agentProfileId: string;
    executorProfileId?: string;
    orgSlug: string;
    projectSlug?: string;
    prompt?: string;
    pollIntervalSeconds?: number;
  }): Promise<{ id: string; sentryInstanceId: string; enabled: boolean }> {
    return this.request(
      "POST",
      `/api/v1/sentry/watches/issue?workspace_id=${encodeURIComponent(opts.workspaceId)}`,
      {
        workspaceId: opts.workspaceId,
        sentryInstanceId: opts.sentryInstanceId,
        workflowId: opts.workflowId,
        workflowStepId: opts.workflowStepId,
        repositoryId: "",
        baseBranch: "",
        filter: { orgSlug: opts.orgSlug, projectSlug: opts.projectSlug },
        agentProfileId: opts.agentProfileId,
        executorProfileId: opts.executorProfileId ?? "",
        prompt: opts.prompt ?? "Investigate {{issue.title}}",
        pollIntervalSeconds: opts.pollIntervalSeconds ?? 300,
      },
    );
  }

  // --- Linear issue watch CRUD ---
  // Used by the agent-profile-delete spec to exercise the watcher dependency
  // surface added in the watcher self-heal PR. Filter shape matches
  // linear.CreateIssueWatchRequest (Go side).

  async createLinearIssueWatch(opts: {
    workspaceId: string;
    workflowId: string;
    workflowStepId: string;
    agentProfileId: string;
    executorProfileId?: string;
    filter?: { teamKey?: string };
    prompt?: string;
    enabled?: boolean;
    pollIntervalSeconds?: number;
  }): Promise<{ id: string; enabled: boolean; lastError?: string }> {
    return this.request("POST", "/api/v1/linear/watches/issue", {
      workspaceId: opts.workspaceId,
      workflowId: opts.workflowId,
      workflowStepId: opts.workflowStepId,
      agentProfileId: opts.agentProfileId,
      executorProfileId: opts.executorProfileId ?? "",
      filter: { teamKey: "ENG", ...(opts.filter ?? {}) },
      prompt: opts.prompt ?? "",
      enabled: opts.enabled ?? true,
      pollIntervalSeconds: opts.pollIntervalSeconds ?? 300,
    });
  }

  async getLinearIssueWatch(
    workspaceId: string,
    watchId: string,
  ): Promise<{
    id: string;
    enabled: boolean;
    lastError?: string;
    lastErrorAt?: string;
  } | null> {
    // No single-watch GET on the route table (only POST/PATCH/DELETE/trigger);
    // walk the workspace list and find by id. Scoped by workspace_id so the
    // result set stays small even if the install accumulates watchers. The
    // list endpoint wraps the rows in a { watches: [...] } envelope.
    const { watches } = await this.request<{
      watches: Array<{ id: string; enabled: boolean; lastError?: string; lastErrorAt?: string }>;
    }>("GET", `/api/v1/linear/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`);
    return watches.find((w) => w.id === watchId) ?? null;
  }

  // --- Agent dashboard E2E seed helpers (KANDEV_E2E_MOCK=true) ---
  // These wrappers append rows to office_runs / office_cost_events /
  // office_activity_log directly so the agent dashboard E2E spec can
  // populate the charts without launching an executor or running
  // through the production mutation pipelines.

  async seedRun(opts: {
    agentProfileId: string;
    reason?: string;
    status?: "queued" | "claimed" | "finished" | "failed" | "cancelled";
    taskId?: string;
    sessionId?: string;
    capabilities?: string;
    inputSnapshot?: string;
    commentId?: string;
    /**
     * Routine that triggered this run. Surfaces as `payload.routine_id`
     * so the office run summary's Linked column deeplinks to
     * `/office/routines/<id>`.
     */
    routineId?: string;
    idempotencyKey?: string;
    errorMessage?: string;
    requestedAt?: string;
    claimedAt?: string;
    finishedAt?: string;
  }): Promise<{ run_id: string }> {
    const payload: Record<string, unknown> = { agent_profile_id: opts.agentProfileId };
    if (opts.reason !== undefined) payload.reason = opts.reason;
    if (opts.status !== undefined) payload.status = opts.status;
    if (opts.taskId !== undefined) payload.task_id = opts.taskId;
    if (opts.sessionId !== undefined) payload.session_id = opts.sessionId;
    if (opts.capabilities !== undefined) payload.capabilities = opts.capabilities;
    if (opts.inputSnapshot !== undefined) payload.input_snapshot = opts.inputSnapshot;
    if (opts.commentId !== undefined) payload.comment_id = opts.commentId;
    if (opts.routineId !== undefined) payload.routine_id = opts.routineId;
    if (opts.idempotencyKey !== undefined) payload.idempotency_key = opts.idempotencyKey;
    if (opts.errorMessage !== undefined) payload.error_message = opts.errorMessage;
    if (opts.requestedAt !== undefined) payload.requested_at = opts.requestedAt;
    if (opts.claimedAt !== undefined) payload.claimed_at = opts.claimedAt;
    if (opts.finishedAt !== undefined) payload.finished_at = opts.finishedAt;
    return this.request("POST", "/api/v1/_test/runs", payload);
  }

  /**
   * Patches a previously-seeded run's status (and optional error_message).
   * The harness publishes office.run.processed afterwards so WS-driven UI
   * sees the transition without a page reload.
   */
  async updateRunStatus(
    runId: string,
    opts: { status: string; errorMessage?: string },
  ): Promise<void> {
    const body: Record<string, unknown> = { status: opts.status };
    if (opts.errorMessage !== undefined) body.error_message = opts.errorMessage;
    await this.request("PATCH", `/api/v1/_test/runs/${runId}`, body);
  }

  async seedCostEvent(opts: {
    agentProfileId: string;
    taskId?: string;
    sessionId?: string;
    tokensIn?: number;
    tokensOut?: number;
    tokensCachedIn?: number;
    costSubcents?: number;
    occurredAt?: string;
  }): Promise<{ id: string }> {
    const payload: Record<string, unknown> = {
      agent_profile_id: opts.agentProfileId,
      tokens_in: opts.tokensIn ?? 0,
      tokens_out: opts.tokensOut ?? 0,
      tokens_cached_in: opts.tokensCachedIn ?? 0,
      cost_subcents: opts.costSubcents ?? 0,
    };
    if (opts.taskId !== undefined) payload.task_id = opts.taskId;
    if (opts.sessionId !== undefined) payload.session_id = opts.sessionId;
    if (opts.occurredAt !== undefined) payload.occurred_at = opts.occurredAt;
    return this.request("POST", "/api/v1/_test/cost-events", payload);
  }

  async seedActivity(opts: {
    workspaceId: string;
    actorType: string;
    actorId: string;
    action: string;
    targetType?: string;
    targetId?: string;
    details?: string;
    runId?: string;
    sessionId?: string;
    createdAt?: string;
  }): Promise<{ id: string }> {
    const payload: Record<string, unknown> = {
      workspace_id: opts.workspaceId,
      actor_type: opts.actorType,
      actor_id: opts.actorId,
      action: opts.action,
    };
    if (opts.targetType !== undefined) payload.target_type = opts.targetType;
    if (opts.targetId !== undefined) payload.target_id = opts.targetId;
    if (opts.details !== undefined) payload.details = opts.details;
    if (opts.runId !== undefined) payload.run_id = opts.runId;
    if (opts.sessionId !== undefined) payload.session_id = opts.sessionId;
    if (opts.createdAt !== undefined) payload.created_at = opts.createdAt;
    return this.request("POST", "/api/v1/_test/activity", payload);
  }

  async mintRuntimeToken(opts: {
    agentProfileId: string;
    workspaceId: string;
    runId: string;
    taskId?: string;
    sessionId?: string;
    capabilities?: string;
  }): Promise<{ token: string }> {
    const payload: Record<string, unknown> = {
      agent_profile_id: opts.agentProfileId,
      workspace_id: opts.workspaceId,
      run_id: opts.runId,
    };
    if (opts.taskId !== undefined) payload.task_id = opts.taskId;
    if (opts.sessionId !== undefined) payload.session_id = opts.sessionId;
    if (opts.capabilities !== undefined) payload.capabilities = opts.capabilities;
    return this.request("POST", "/api/v1/_test/runtime-token", payload);
  }

  async runtimeUpdateTaskStatus(token: string, taskId: string, status: string): Promise<Response> {
    return fetch(`${this.baseUrl}/api/v1/office/runtime/tasks/${taskId}/status`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ status }),
    });
  }

  async runtimePostComment(token: string, taskId: string, body: string): Promise<Response> {
    return fetch(`${this.baseUrl}/api/v1/office/runtime/comments`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ task_id: taskId, body }),
    });
  }

  async runtimeCreateSubtask(
    token: string,
    parentTaskId: string,
    data: { title: string; description?: string; assigneeAgentId?: string },
  ): Promise<Response> {
    return fetch(`${this.baseUrl}/api/v1/office/runtime/tasks/${parentTaskId}/subtasks`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        parent_task_id: parentTaskId,
        title: data.title,
        description: data.description ?? "",
        assignee_agent_id: data.assigneeAgentId,
      }),
    });
  }

  async runtimeCreateAgent(
    token: string,
    data: { name: string; role: string; reason?: string },
  ): Promise<Response> {
    return fetch(`${this.baseUrl}/api/v1/office/runtime/agents`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(data),
    });
  }

  async runtimePutMemory(token: string, path: string, content: string): Promise<Response> {
    return fetch(`${this.baseUrl}/api/v1/office/runtime/memory${path}`, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ content }),
    });
  }

  async runtimeGetMemory(token: string, path: string): Promise<Response> {
    return fetch(`${this.baseUrl}/api/v1/office/runtime/memory${path}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  }

  // --- SSH executor helpers ---

  /**
   * Create an SSH executor. The `config` map carries the fields the SSH
   * runtime reads at launch time: ssh_host / ssh_port / ssh_user /
   * ssh_host_fingerprint / ssh_identity_source / ssh_identity_file /
   * ssh_proxy_jump. Pre-trusting a fingerprint here lets tests skip the UI
   * test-then-trust flow.
   */
  async createSSHExecutor(
    name: string,
    config: Record<string, string>,
  ): Promise<{ id: string; name: string; type: string; config: Record<string, string> }> {
    return this.request("POST", "/api/v1/executors", { name, type: "ssh", config });
  }

  async updateExecutor(
    executorId: string,
    patch: { name?: string; config?: Record<string, string> },
  ): Promise<void> {
    await this.request("PATCH", `/api/v1/executors/${executorId}`, patch);
  }

  async getExecutor(executorId: string): Promise<{
    id: string;
    name: string;
    type: string;
    config?: Record<string, string>;
  }> {
    return this.request("GET", `/api/v1/executors/${executorId}`);
  }

  async testSSHConnection(req: SSHTestRequest): Promise<SSHTestResult> {
    return this.request("POST", "/api/v1/ssh/test", req);
  }

  async listSSHSessions(executorId: string): Promise<SSHSession[]> {
    return this.request("GET", `/api/v1/ssh/executors/${executorId}/sessions`);
  }

  async probeSSHAgents(
    executorId: string,
    body?: { shell?: string },
  ): Promise<SSHAgentReadinessResponse> {
    return this.request("POST", `/api/v1/ssh/executors/${executorId}/probe-agents`, body ?? {});
  }

  async probeSSHShells(executorId: string): Promise<SSHProbeShellsResponse> {
    return this.request("POST", `/api/v1/ssh/executors/${executorId}/probe-shells`);
  }

  /**
   * Seed an automation via the E2E HTTP endpoint (avoids WS / Node 24 requirement).
   * Only works when KANDEV_MOCK_AGENT is active.
   */
  async seedAutomation(opts: {
    workspaceId: string;
    name: string;
    workflowId?: string;
    workflowStepId?: string;
  }): Promise<{ id: string; workspace_id: string; name: string }> {
    return this.request("POST", "/api/v1/e2e/automations", {
      workspace_id: opts.workspaceId,
      name: opts.name,
      workflow_id: opts.workflowId ?? "",
      workflow_step_id: opts.workflowStepId ?? "",
    });
  }

  /**
   * Seed an automation run row via the E2E HTTP endpoint.
   * Only works when KANDEV_MOCK_AGENT is active.
   */
  async seedAutomationRun(
    automationId: string,
    status = "skipped",
    taskId?: string,
  ): Promise<{ id: string; automation_id: string; status: string; task_id: string }> {
    return this.request("POST", "/api/v1/e2e/automation-runs", {
      automation_id: automationId,
      status,
      task_id: taskId ?? "",
    });
  }
}

// --- Jira / Linear mock payload types ---

export type MockJiraProject = { id: string; key: string; name: string };

export type MockJiraStatus = { id: string; name: string; statusCategory?: string };

export type MockJiraTransition = {
  id: string;
  name: string;
  toStatusId: string;
  toStatusName: string;
};

export type MockJiraTicket = {
  key: string;
  summary?: string;
  description?: string;
  statusId?: string;
  statusName?: string;
  statusCategory?: string;
  projectKey?: string;
  issueType?: string;
  url?: string;
  transitions?: MockJiraTransition[];
};

export type MockLinearTeam = { id: string; key: string; name: string };

export type MockLinearState = {
  id: string;
  name: string;
  type: string;
  color?: string;
  position?: number;
};

export type MockLinearIssue = {
  id: string;
  identifier: string;
  title?: string;
  description?: string;
  stateId?: string;
  stateName?: string;
  stateType?: string;
  stateCategory?: string;
  teamId?: string;
  teamKey?: string;
  priority?: number;
  url?: string;
};

// --- Sentry mock payload types ---

export type MockSentryOrganization = { id: string; slug: string; name: string };

export type MockSentryProject = { id: string; slug: string; name: string; orgSlug: string };

// MockSentryInstance is the subset of a created instance the e2e helpers read.
export type MockSentryInstance = {
  id: string;
  workspaceId: string;
  name: string;
  url: string;
  hasSecret: boolean;
  lastOk: boolean;
};
