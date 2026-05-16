/**
 * HTTP API client for the office feature endpoints.
 * Covers onboarding, agents, issues, labels, documents, dashboard, skills,
 * config sync, routines, costs, approvals, and workspace settings.
 */
export class OfficeApiClient {
  constructor(private baseUrl: string) {}

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseUrl}/api/v1/office${path}`, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Office API ${method} /api/v1/office${path} failed (${res.status}): ${text}`);
    }
    return res.json() as Promise<T>;
  }

  // --- Onboarding ---

  async getOnboardingState(): Promise<Record<string, unknown>> {
    return this.request("GET", "/onboarding-state");
  }

  async completeOnboarding(data: {
    workspaceName: string;
    taskPrefix: string;
    agentName: string;
    agentProfileId: string;
    executorPreference: string;
    taskTitle?: string;
    taskDescription?: string;
  }): Promise<{ workspaceId: string; agentId: string; projectId: string }> {
    return this.request("POST", "/onboarding/complete", {
      workspaceName: data.workspaceName,
      taskPrefix: data.taskPrefix,
      agentName: data.agentName,
      agentProfileId: data.agentProfileId,
      executorPreference: data.executorPreference,
      taskTitle: data.taskTitle,
      taskDescription: data.taskDescription,
    });
  }

  async importFromFS(): Promise<Record<string, unknown>> {
    return this.request("POST", "/onboarding/import-fs", undefined);
  }

  // --- Agents ---

  async listAgents(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/agents`);
  }

  async createAgent(
    wsId: string,
    data: {
      name: string;
      role: string;
      agent_profile_id?: string;
    },
  ): Promise<Record<string, unknown>> {
    const res = await this.request<{ agent: Record<string, unknown> }>(
      "POST",
      `/workspaces/${wsId}/agents`,
      data,
    );
    return res.agent ?? (res as unknown as Record<string, unknown>);
  }

  async getAgent(id: string): Promise<Record<string, unknown>> {
    const res = await this.request<{ agent: Record<string, unknown> }>("GET", `/agents/${id}`);
    return res.agent ?? (res as unknown as Record<string, unknown>);
  }

  async updateAgent(id: string, data: Record<string, unknown>): Promise<Record<string, unknown>> {
    const res = await this.request<{ agent: Record<string, unknown> }>(
      "PATCH",
      `/agents/${id}`,
      data,
    );
    return res.agent ?? (res as unknown as Record<string, unknown>);
  }

  async deleteAgent(id: string): Promise<void> {
    await this.request("DELETE", `/agents/${id}`);
  }

  async updateAgentStatus(id: string, status: string): Promise<Record<string, unknown>> {
    const res = await this.request<{ agent: Record<string, unknown> }>(
      "PATCH",
      `/agents/${id}/status`,
      { status },
    );
    return res.agent ?? (res as unknown as Record<string, unknown>);
  }

  // --- Issues / Tasks ---

  /**
   * Create an office task via the core /api/v1/tasks endpoint (not /office/).
   * Supports `blocked_by` (IDs of tasks that must complete before this one)
   * and other office-specific fields that the base ApiClient does not expose.
   */
  async createTask(
    wsId: string,
    title: string,
    opts?: {
      parent_id?: string;
      blocked_by?: string[];
      workflow_id?: string;
      description?: string;
    },
  ): Promise<Record<string, unknown>> {
    const body: Record<string, unknown> = {
      workspace_id: wsId,
      title,
      description: opts?.description ?? "",
    };
    if (opts?.parent_id) body.parent_id = opts.parent_id;
    if (opts?.blocked_by?.length) body.blocked_by = opts.blocked_by;
    if (opts?.workflow_id) body.workflow_id = opts.workflow_id;
    // Use the core /api/v1/tasks route (not the /office/ prefix)
    const res = await fetch(`${this.baseUrl}/api/v1/tasks`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Office createTask POST /api/v1/tasks failed (${res.status}): ${text}`);
    }
    return res.json() as Promise<Record<string, unknown>>;
  }

  async listTasks(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/tasks`);
  }

  async getTask(id: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/tasks/${id}`);
  }

  async updateTaskStatus(
    id: string,
    status: string,
    comment: string,
  ): Promise<Record<string, unknown>> {
    return this.request("PATCH", `/tasks/${id}`, { status, comment });
  }

  /**
   * Assign the task to an agent (instance id). Office's TaskUpdated
   * subscriber fires a `task_assigned` run for the assignee, which is
   * what the routing dispatch path needs to fire.
   */
  async assignTask(id: string, assigneeAgentProfileId: string): Promise<Record<string, unknown>> {
    return this.request("PATCH", `/tasks/${id}`, {
      assignee_agent_profile_id: assigneeAgentProfileId,
    });
  }

  async searchTasks(wsId: string, query: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/tasks/search?q=${encodeURIComponent(query)}`);
  }

  // --- Labels ---

  async addLabel(wsId: string, taskId: string, name: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/workspaces/${wsId}/tasks/${taskId}/labels`, { name });
  }

  async removeLabel(wsId: string, taskId: string, name: string): Promise<void> {
    await this.request(
      "DELETE",
      `/workspaces/${wsId}/tasks/${taskId}/labels/${encodeURIComponent(name)}`,
    );
  }

  async listTaskLabels(wsId: string, taskId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/tasks/${taskId}/labels`);
  }

  async listWorkspaceLabels(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/labels`);
  }

  async updateLabel(
    wsId: string,
    labelId: string,
    data: { name?: string; color?: string },
  ): Promise<void> {
    await this.request("PATCH", `/workspaces/${wsId}/labels/${labelId}`, data);
  }

  async deleteLabel(wsId: string, labelId: string): Promise<void> {
    await this.request("DELETE", `/workspaces/${wsId}/labels/${labelId}`);
  }

  // --- Documents ---

  async listDocuments(taskId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/tasks/${taskId}/documents`);
  }

  async getDocument(taskId: string, key: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/tasks/${taskId}/documents/${encodeURIComponent(key)}`);
  }

  async createOrUpdateDocument(
    taskId: string,
    key: string,
    data: {
      type: string;
      title: string;
      content: string;
      author_kind?: string;
      author_name?: string;
    },
  ): Promise<Record<string, unknown>> {
    return this.request("PUT", `/tasks/${taskId}/documents/${encodeURIComponent(key)}`, data);
  }

  async deleteDocument(taskId: string, key: string): Promise<void> {
    await this.request("DELETE", `/tasks/${taskId}/documents/${encodeURIComponent(key)}`);
  }

  async listRevisions(taskId: string, key: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/tasks/${taskId}/documents/${encodeURIComponent(key)}/revisions`);
  }

  async revertDocument(
    taskId: string,
    key: string,
    revId: string,
  ): Promise<Record<string, unknown>> {
    return this.request(
      "POST",
      `/tasks/${taskId}/documents/${encodeURIComponent(key)}/revisions/${revId}/restore`,
    );
  }

  // --- Comments ---

  async listTaskComments(taskId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/tasks/${taskId}/comments`);
  }

  async createTaskComment(taskId: string, body: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/tasks/${taskId}/comments`, { body });
  }

  // --- Provider routing ---

  /**
   * Fetch the workspace routing config + known-provider catalogue.
   * Returns the default disabled-balanced shape when no row exists yet.
   */
  async getRouting(wsId: string): Promise<{
    config: {
      enabled: boolean;
      provider_order: string[];
      default_tier: "frontier" | "balanced" | "economy";
      provider_profiles: Record<
        string,
        { tier_map: { frontier?: string; balanced?: string; economy?: string }; mode?: string }
      >;
    };
    known_providers: string[];
  }> {
    return this.request("GET", `/workspaces/${wsId}/routing`);
  }

  /**
   * Persist a workspace routing config. The backend validator runs in
   * strict mode when `enabled=true`; the test callers pass a complete
   * config in that case.
   */
  async updateRouting(
    wsId: string,
    cfg: {
      enabled: boolean;
      provider_order: string[];
      default_tier: "frontier" | "balanced" | "economy";
      provider_profiles: Record<
        string,
        { tier_map: { frontier?: string; balanced?: string; economy?: string }; mode?: string }
      >;
    },
  ): Promise<Record<string, unknown>> {
    return this.request("PUT", `/workspaces/${wsId}/routing`, cfg);
  }

  async retryRoutingProvider(
    wsId: string,
    providerId: string,
  ): Promise<{ status: string; retry_at?: string }> {
    return this.request("POST", `/workspaces/${wsId}/routing/retry`, { provider_id: providerId });
  }

  async listRoutingHealth(
    wsId: string,
  ): Promise<{ health: Array<{ provider_id: string; state: string; error_code?: string }> }> {
    return this.request("GET", `/workspaces/${wsId}/routing/health`);
  }

  async getRoutingPreview(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/routing/preview`);
  }

  async getAgentRoute(agentId: string): Promise<{
    preview: Record<string, unknown>;
    overrides: {
      provider_order_source?: string;
      provider_order?: string[];
      tier_source?: string;
      tier?: string;
    };
    last_failure_code?: string;
    last_failure_run?: string;
  }> {
    return this.request("GET", `/agents/${agentId}/route`);
  }

  async listRouteAttempts(runId: string): Promise<{
    attempts: Array<{
      seq: number;
      provider_id: string;
      model: string;
      tier: string;
      outcome: string;
      error_code?: string;
    }>;
  }> {
    return this.request("GET", `/runs/${runId}/attempts`);
  }

  // --- Dashboard ---

  async getDashboard(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/dashboard`);
  }

  async getInbox(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/inbox`);
  }

  async listActivity(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/activity`);
  }

  async listRuns(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/runs`);
  }

  // --- Skills ---

  async listSkills(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/skills`);
  }

  async discoverUserHomeSkills(wsId: string, provider: string): Promise<Record<string, unknown>> {
    return this.request(
      "GET",
      `/workspaces/${wsId}/skills/discover?provider=${encodeURIComponent(provider)}`,
    );
  }

  async createSkill(
    wsId: string,
    data: { name: string; slug: string; content: string },
  ): Promise<Record<string, unknown>> {
    const res = await this.request<{ skill: Record<string, unknown> }>(
      "POST",
      `/workspaces/${wsId}/skills`,
      data,
    );
    return res.skill ?? (res as unknown as Record<string, unknown>);
  }

  async getSkill(id: string): Promise<Record<string, unknown>> {
    const res = await this.request<{ skill: Record<string, unknown> }>("GET", `/skills/${id}`);
    return res.skill ?? (res as unknown as Record<string, unknown>);
  }

  async updateSkill(
    id: string,
    data: { name?: string; slug?: string; content?: string },
  ): Promise<Record<string, unknown>> {
    const res = await this.request<{ skill: Record<string, unknown> }>(
      "PATCH",
      `/skills/${id}`,
      data,
    );
    return res.skill ?? (res as unknown as Record<string, unknown>);
  }

  async importUserHomeSkill(
    wsId: string,
    provider: string,
    key: string,
  ): Promise<Record<string, unknown>> {
    const res = await this.request<{ skills: Record<string, unknown>[] }>(
      "POST",
      `/workspaces/${wsId}/skills/import`,
      {
        source_type: "user_home",
        provider,
        key,
      },
    );
    return res.skills?.[0] ?? {};
  }

  async getSkillFile(id: string, filePath: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/skills/${id}/files?path=${encodeURIComponent(filePath)}`);
  }

  async deleteSkill(id: string): Promise<void> {
    await this.request("DELETE", `/skills/${id}`);
  }

  // --- Config ---

  async exportConfig(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/config/export`);
  }

  async getIncomingDiff(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/config/sync/incoming`);
  }

  async applyOutgoingSync(wsId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/workspaces/${wsId}/config/sync/export-fs`, undefined);
  }

  async applyIncomingSync(wsId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/workspaces/${wsId}/config/sync/import-fs`, undefined);
  }

  // --- Routines ---

  async listRoutines(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/routines`);
  }

  async createRoutine(
    wsId: string,
    data: { name: string; description?: string },
  ): Promise<Record<string, unknown>> {
    const res = await this.request<{ routine: Record<string, unknown> }>(
      "POST",
      `/workspaces/${wsId}/routines`,
      data,
    );
    return res.routine ?? (res as unknown as Record<string, unknown>);
  }

  // --- Costs ---

  async getCostSummary(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/costs/summary`);
  }

  async listBudgets(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/budgets`);
  }

  // --- Approvals ---

  async listApprovals(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/approvals`);
  }

  async decideApproval(id: string, status: string, note: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/approvals/${id}/decide`, { status, note });
  }

  // --- Tree Controls ---

  async pauseTaskTree(taskId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/tasks/${taskId}/tree/pause`);
  }

  async resumeTaskTree(taskId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/tasks/${taskId}/tree/resume`);
  }

  async cancelTaskTree(taskId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/tasks/${taskId}/tree/cancel`);
  }

  async restoreTaskTree(taskId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/tasks/${taskId}/tree/restore`);
  }

  async previewTaskTree(taskId: string): Promise<Record<string, unknown>> {
    return this.request("POST", `/tasks/${taskId}/tree/preview`);
  }

  async getSubtreeCostSummary(taskId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/tasks/${taskId}/tree/cost-summary`);
  }

  // --- Settings ---

  async getWorkspaceSettings(wsId: string): Promise<Record<string, unknown>> {
    return this.request("GET", `/workspaces/${wsId}/settings`);
  }

  async updateWorkspaceSettings(
    wsId: string,
    data: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return this.request("PATCH", `/workspaces/${wsId}/settings`, data);
  }
}
