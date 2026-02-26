import type {
  Workspace,
  Workflow,
  CreateTaskResponse,
  ListWorkflowsResponse,
  ListWorkflowStepsResponse,
} from "../../lib/types/http";

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

/**
 * HTTP API client for seeding test data via the backend REST API.
 */
export class ApiClient {
  constructor(private baseUrl: string) {}

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

  async createTask(
    workspaceId: string,
    title: string,
    opts?: { description?: string; workflow_id?: string; workflow_step_id?: string },
  ): Promise<CreateTaskResponse> {
    return this.request("POST", "/api/v1/tasks", {
      workspace_id: workspaceId,
      title,
      description: opts?.description ?? "",
      ...(opts?.workflow_id ? { workflow_id: opts.workflow_id } : {}),
      ...(opts?.workflow_step_id ? { workflow_step_id: opts.workflow_step_id } : {}),
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
  ): Promise<{ step: { id: string } }> {
    return this.request("POST", `/api/v1/workflow/steps`, {
      workflow_id: workflowId,
      name,
      position,
    });
  }

  async saveUserSettings(settings: {
    enable_preview_on_click?: boolean;
    workspace_id?: string;
    workflow_filter_id?: string;
  }): Promise<void> {
    await this.request("PATCH", "/api/v1/user/settings", settings);
  }

  async moveTask(taskId: string, workflowId: string, workflowStepId: string): Promise<void> {
    await this.request("POST", `/api/v1/tasks/${taskId}/move`, {
      workflow_id: workflowId,
      workflow_step_id: workflowStepId,
    });
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

  async mockGitHubGetStatus(): Promise<{
    authenticated: boolean;
    username: string;
    auth_method: string;
  }> {
    return this.request("GET", "/api/v1/github/status");
  }
}
