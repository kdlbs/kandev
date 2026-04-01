import type {
  Workspace,
  Workflow,
  CreateTaskResponse,
  ListWorkflowsResponse,
  ListWorkflowStepsResponse,
} from "../../lib/types/http";
import type { Agent } from "../../lib/types/http-agents";

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
  additions?: number;
  deletions?: number;
  merged_at?: string;
  requested_reviewers?: Array<{ login: string; type: string }>;
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
};

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
  const body: Record<string, unknown> = {
    workspace_id: workspaceId,
    title,
    description: opts?.description ?? "",
  };
  setIf(body, "workflow_id", opts?.workflow_id);
  setIf(body, "workflow_step_id", opts?.workflow_step_id);
  setIf(body, "metadata", opts ? buildTaskMetadata(opts) : undefined);
  setIf(
    body,
    "repositories",
    opts?.repositories ?? opts?.repository_ids?.map((id) => ({ repository_id: id })),
  );
  if (opts?.plan_mode) body.plan_mode = true;
  setIf(body, "parent_id", opts?.parent_id);
  return body;
}

/** Build the optional fields object for createTaskWithAgent requests. */
function buildOptionalAgentTaskFields(opts?: {
  workflow_id?: string;
  workflow_step_id?: string;
  repository_ids?: string[];
  executor_id?: string;
  executor_profile_id?: string;
  metadata?: Record<string, unknown>;
  parent_id?: string;
}): Record<string, unknown> {
  const fields: Record<string, unknown> = {};
  if (opts?.workflow_id) fields.workflow_id = opts.workflow_id;
  if (opts?.workflow_step_id) fields.workflow_step_id = opts.workflow_step_id;
  if (opts?.repository_ids)
    fields.repositories = opts.repository_ids.map((id) => ({ repository_id: id }));
  if (opts?.executor_id) fields.executor_id = opts.executor_id;
  if (opts?.executor_profile_id) fields.executor_profile_id = opts.executor_profile_id;
  if (opts?.metadata) fields.metadata = opts.metadata;
  if (opts?.parent_id) fields.parent_id = opts.parent_id;
  return fields;
}

/**
 * HTTP API client for seeding test data via the backend REST API.
 */
export class ApiClient {
  constructor(private baseUrl: string) {}

  /** Perform an HTTP request and return the raw Response (does not throw on non-2xx). */
  async rawRequest(method: string, path: string, body?: unknown): Promise<Response> {
    return fetch(`${this.baseUrl}${path}`, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
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

  async healthCheck(): Promise<void> {
    await this.request("GET", "/health");
  }

  async createWorkspace(name: string): Promise<Workspace> {
    return this.request("POST", "/api/v1/workspaces", { name });
  }

  async createWorkflow(workspaceId: string, name: string, templateId?: string): Promise<Workflow> {
    return this.request("POST", "/api/v1/workflows", {
      workspace_id: workspaceId,
      name,
      ...(templateId ? { workflow_template_id: templateId } : {}),
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
    },
  ): Promise<CreateTaskResponse> {
    return this.request("POST", "/api/v1/tasks", buildCreateTaskBody(workspaceId, title, opts));
  }

  async listAgents(): Promise<{ agents: Agent[]; total: number }> {
    return this.request("GET", "/api/v1/agents");
  }

  async deleteAgentProfile(profileId: string, force?: boolean): Promise<void> {
    const qs = force ? "?force=true" : "";
    await this.request("DELETE", `/api/v1/agent-profiles/${profileId}${qs}`);
  }

  /** Delete all agent profiles except the ones in keepIds. */
  async cleanupTestProfiles(keepIds: string[]): Promise<void> {
    const { agents } = await this.listAgents();
    for (const agent of agents) {
      for (const profile of agent.profiles ?? []) {
        if (!keepIds.includes(profile.id)) {
          await this.deleteAgentProfile(profile.id);
        }
      }
    }
  }

  async createAgentProfile(
    agentId: string,
    name: string,
    opts: {
      model: string;
      auto_approve?: boolean;
      cli_passthrough?: boolean;
    },
  ): Promise<{ id: string }> {
    return this.request("POST", `/api/v1/agents/${agentId}/profiles`, {
      name,
      model: opts.model,
      auto_approve: opts.auto_approve ?? true,
      cli_passthrough: opts.cli_passthrough ?? false,
    });
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
      executor_id?: string;
      executor_profile_id?: string;
      metadata?: Record<string, unknown>;
      /** Parent task ID for subtasks. */
      parent_id?: string;
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
      ...(opts?.provider_owner ? { provider_owner: opts.provider_owner } : {}),
      ...(opts?.provider_name ? { provider_name: opts.provider_name } : {}),
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
    name: string,
    opts?: { mcp_policy?: string; prepare_script?: string; cleanup_script?: string },
  ): Promise<{ id: string; name: string }> {
    return this.request("POST", `/api/v1/executors/${executorId}/profiles`, {
      name,
      ...(opts?.mcp_policy ? { mcp_policy: opts.mcp_policy } : {}),
      ...(opts?.prepare_script ? { prepare_script: opts.prepare_script } : {}),
      ...(opts?.cleanup_script ? { cleanup_script: opts.cleanup_script } : {}),
    });
  }

  async deleteExecutorProfile(profileId: string): Promise<void> {
    await this.request("DELETE", `/api/v1/executor-profiles/${profileId}`);
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
      [key: string]: unknown;
    };
  }> {
    return this.request("GET", "/api/v1/user/settings");
  }

  async saveUserSettings(settings: {
    enable_preview_on_click?: boolean;
    workspace_id?: string;
    workflow_filter_id?: string;
    terminal_link_behavior?: "new_tab" | "browser_panel";
    terminal_font_family?: string;
    terminal_font_size?: number;
    keyboard_shortcuts?: Record<string, unknown>;
    default_utility_agent_id?: string;
    default_utility_model?: string;
  }): Promise<void> {
    await this.request("PATCH", "/api/v1/user/settings", settings);
  }

  async moveTask(taskId: string, workflowId: string, workflowStepId: string): Promise<void> {
    await this.request("POST", `/api/v1/tasks/${taskId}/move`, {
      workflow_id: workflowId,
      workflow_step_id: workflowStepId,
    });
  }

  async updateWorkflowStep(
    stepId: string,
    updates: {
      prompt?: string;
      events?: {
        on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_exit?: Array<{ type: string; config?: Record<string, unknown> }>;
      };
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

  // --- GitHub Mock Control ---

  async mockGitHubReset(): Promise<void> {
    await this.request("DELETE", "/api/v1/github/mock/reset");
  }

  async mockGitHubSetUser(username: string): Promise<void> {
    await this.request("PUT", "/api/v1/github/mock/user", { username });
  }

  async mockGitHubAddPRs(prs: MockPR[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/prs", { prs });
  }

  async mockGitHubAddOrgs(orgs: MockOrg[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/orgs", { orgs });
  }

  async mockGitHubAddRepos(org: string, repos: MockRepo[]): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/repos", { org, repos });
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
    additions?: number;
    deletions?: number;
  }): Promise<void> {
    await this.request("POST", "/api/v1/github/mock/task-prs", data);
  }

  async mockGitHubGetStatus(): Promise<{
    authenticated: boolean;
    username: string;
    auth_method: string;
  }> {
    return this.request("GET", "/api/v1/github/status");
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
      state: string;
      started_at: string;
      task_environment_id?: string;
      worktree_path?: string;
      worktree_branch?: string;
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
  }> {
    return this.request("GET", `/api/v1/tasks/${taskId}`);
  }

  async getTaskEnvironment(taskId: string): Promise<{
    id: string;
    task_id: string;
    worktree_id?: string;
    worktree_path?: string;
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
    });
  }

  async triggerReviewWatch(watchId: string): Promise<{ new_prs: number; cleaned?: number }> {
    return this.request("POST", `/api/v1/github/watches/review/${watchId}/trigger`, undefined);
  }
}
