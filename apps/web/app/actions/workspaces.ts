"use server";

import { getBackendConfig } from "@/lib/config";
import type {
  ApproveSessionResponse,
  Workflow,
  ListWorkflowsResponse,
  ListTasksResponse,
  ListWorkflowStepsResponse,
  RepositoryDiscoveryResponse,
  ListRepositoriesResponse,
  RepositoryBranchesResponse,
  ListRepositoryScriptsResponse,
  ListWorkspacesResponse,
  RepositoryPathValidationResponse,
  Repository,
  RepositoryScript,
  StepEvents,
  Workspace,
  WorkflowStep,
  WorkflowExportData,
  ImportWorkflowsResult,
  ListWorkflowTemplatesResponse,
  WorkflowTemplate,
  StepDefinition,
} from "@/lib/types/http";

const { apiBaseUrl } = getBackendConfig();

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      ...(options?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText}`);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

export async function listWorkspacesAction(): Promise<ListWorkspacesResponse> {
  return fetchJson<ListWorkspacesResponse>(`${apiBaseUrl}/api/v1/workspaces`);
}

export async function getWorkspaceAction(id: string): Promise<Workspace> {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces/${id}`);
}

export async function createWorkspaceAction(payload: {
  name: string;
  description?: string;
  default_executor_id?: string;
  default_environment_id?: string;
  default_agent_profile_id?: string;
}) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateWorkspaceAction(
  id: string,
  payload: {
    name?: string;
    description?: string;
    default_executor_id?: string;
    default_environment_id?: string;
    default_agent_profile_id?: string;
  },
) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function deleteWorkspaceAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workspaces/${id}`, { method: "DELETE" });
}

export async function listWorkflowsAction(workspaceId: string): Promise<ListWorkflowsResponse> {
  const url = new URL(`${apiBaseUrl}/api/v1/workflows`);
  url.searchParams.set("workspace_id", workspaceId);
  return fetchJson<ListWorkflowsResponse>(url.toString());
}

export async function listTasksByWorkspaceAction(
  workspaceId: string,
  page = 1,
  pageSize = 50,
  query = "",
): Promise<ListTasksResponse> {
  const url = new URL(`${apiBaseUrl}/api/v1/workspaces/${workspaceId}/tasks`);
  url.searchParams.set("page", String(page));
  url.searchParams.set("page_size", String(pageSize));
  if (query) url.searchParams.set("query", query);
  return fetchJson<ListTasksResponse>(url.toString());
}

export async function createWorkflowAction(payload: {
  workspace_id: string;
  name: string;
  description?: string;
  workflow_template_id?: string;
}) {
  return fetchJson<Workflow>(`${apiBaseUrl}/api/v1/workflows`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateWorkflowAction(
  id: string,
  payload: { name?: string; description?: string },
) {
  return fetchJson<Workflow>(`${apiBaseUrl}/api/v1/workflows/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function deleteWorkflowAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workflows/${id}`, { method: "DELETE" });
}

export async function listRepositoriesAction(
  workspaceId: string,
  params?: { includeScripts?: boolean },
): Promise<ListRepositoriesResponse> {
  const searchParams = new URLSearchParams();
  if (params?.includeScripts) {
    searchParams.set("include_scripts", "true");
  }
  const queryString = searchParams.toString();
  const url = `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories${queryString ? `?${queryString}` : ""}`;
  return fetchJson<ListRepositoriesResponse>(url);
}

export async function listLocalRepositoryBranchesAction(
  workspaceId: string,
  path: string,
): Promise<RepositoryBranchesResponse> {
  const params = `?path=${encodeURIComponent(path)}`;
  return fetchJson<RepositoryBranchesResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/branches${params}`,
  );
}

export async function discoverRepositoriesAction(
  workspaceId: string,
  root?: string,
): Promise<RepositoryDiscoveryResponse> {
  const params = root ? `?root=${encodeURIComponent(root)}` : "";
  return fetchJson<RepositoryDiscoveryResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/discover${params}`,
  );
}

export async function validateRepositoryPathAction(
  workspaceId: string,
  path: string,
): Promise<RepositoryPathValidationResponse> {
  const params = `?path=${encodeURIComponent(path)}`;
  return fetchJson<RepositoryPathValidationResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/validate${params}`,
  );
}

export async function createRepositoryAction(payload: {
  workspace_id: string;
  name: string;
  source_type: string;
  local_path: string;
  provider: string;
  provider_repo_id: string;
  provider_owner: string;
  provider_name: string;
  default_branch: string;
  worktree_branch_prefix: string;
  pull_before_worktree: boolean;
  setup_script: string;
  cleanup_script: string;
  dev_script: string;
}) {
  return fetchJson<Repository>(
    `${apiBaseUrl}/api/v1/workspaces/${payload.workspace_id}/repositories`,
    {
      method: "POST",
      body: JSON.stringify({
        name: payload.name,
        source_type: payload.source_type,
        local_path: payload.local_path,
        provider: payload.provider,
        provider_repo_id: payload.provider_repo_id,
        provider_owner: payload.provider_owner,
        provider_name: payload.provider_name,
        default_branch: payload.default_branch,
        worktree_branch_prefix: payload.worktree_branch_prefix,
        pull_before_worktree: payload.pull_before_worktree,
        setup_script: payload.setup_script,
        cleanup_script: payload.cleanup_script,
        dev_script: payload.dev_script,
      }),
    },
  );
}

export async function updateRepositoryAction(id: string, payload: Partial<Repository>) {
  return fetchJson<Repository>(`${apiBaseUrl}/api/v1/repositories/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function deleteRepositoryAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/repositories/${id}`, { method: "DELETE" });
}

export async function listRepositoryScriptsAction(
  repositoryId: string,
): Promise<ListRepositoryScriptsResponse> {
  return fetchJson<ListRepositoryScriptsResponse>(
    `${apiBaseUrl}/api/v1/repositories/${repositoryId}/scripts`,
  );
}

export async function createRepositoryScriptAction(payload: {
  repository_id: string;
  name: string;
  command: string;
  position: number;
}) {
  return fetchJson<RepositoryScript>(
    `${apiBaseUrl}/api/v1/repositories/${payload.repository_id}/scripts`,
    {
      method: "POST",
      body: JSON.stringify({
        name: payload.name,
        command: payload.command,
        position: payload.position,
      }),
    },
  );
}

export async function updateRepositoryScriptAction(id: string, payload: Partial<RepositoryScript>) {
  return fetchJson<RepositoryScript>(`${apiBaseUrl}/api/v1/scripts/${id}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export async function deleteRepositoryScriptAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/scripts/${id}`, { method: "DELETE" });
}

type BackendTemplateStep = {
  name: string;
  position: number;
  color?: string;
  prompt?: string;
  events?: StepEvents;
};

type BackendWorkflowTemplate = Omit<WorkflowTemplate, "default_steps"> & {
  steps?: BackendTemplateStep[];
  default_steps?: BackendTemplateStep[];
};

const normalizeWorkflowTemplate = (template: BackendWorkflowTemplate): WorkflowTemplate => {
  const steps = template.default_steps ?? template.steps ?? [];
  const default_steps: StepDefinition[] = steps.map((step) => ({
    name: step.name,
    position: step.position,
    color: step.color,
    prompt: step.prompt,
    events: step.events,
  }));
  return {
    ...template,
    default_steps,
  };
};

// Workflow Templates
export async function listWorkflowTemplatesAction(): Promise<ListWorkflowTemplatesResponse> {
  const response = await fetchJson<ListWorkflowTemplatesResponse>(
    `${apiBaseUrl}/api/v1/workflow/templates`,
  );
  return {
    ...response,
    templates: (response.templates ?? []).map((template) =>
      normalizeWorkflowTemplate(template as BackendWorkflowTemplate),
    ),
  };
}

type BackendWorkflowStep = {
  id: string;
  workflow_id: string;
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: StepEvents;
  allow_manual_move?: boolean;
  is_start_step?: boolean;
  auto_archive_after_hours?: number;
  created_at: string;
  updated_at: string;
};

const transformWorkflowStep = (step: BackendWorkflowStep): WorkflowStep => ({
  id: step.id,
  workflow_id: step.workflow_id,
  name: step.name,
  position: step.position,
  color: step.color,
  prompt: step.prompt,
  events: step.events,
  allow_manual_move: step.allow_manual_move,
  is_start_step: step.is_start_step,
  auto_archive_after_hours: step.auto_archive_after_hours,
  created_at: step.created_at,
  updated_at: step.updated_at,
});

// Workspace Workflow Steps (batch: all steps for all workflows in a workspace)
export async function listWorkspaceWorkflowStepsAction(
  workspaceId: string,
): Promise<ListWorkflowStepsResponse> {
  const response = await fetchJson<{ steps: BackendWorkflowStep[] | null }>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/workflow-steps`,
  );
  return {
    steps: (response.steps ?? []).map(transformWorkflowStep),
    total: response.steps?.length ?? 0,
  };
}

// Workflow Steps
export async function listWorkflowStepsAction(
  workflowId: string,
): Promise<ListWorkflowStepsResponse> {
  const response = await fetchJson<{ steps: BackendWorkflowStep[] | null }>(
    `${apiBaseUrl}/api/v1/workflows/${workflowId}/workflow/steps`,
  );
  return {
    steps: (response.steps ?? []).map(transformWorkflowStep),
    total: response.steps?.length ?? 0,
  };
}

export async function createWorkflowStepAction(payload: {
  workflow_id: string;
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: StepEvents;
  is_start_step?: boolean;
  allow_manual_move?: boolean;
}): Promise<WorkflowStep> {
  const body = {
    workflow_id: payload.workflow_id,
    name: payload.name,
    position: payload.position,
    color: payload.color,
    prompt: payload.prompt ?? "",
    events: payload.events,
    allow_manual_move: payload.allow_manual_move ?? true,
    is_start_step: payload.is_start_step ?? false,
  };
  const response = await fetchJson<BackendWorkflowStep>(`${apiBaseUrl}/api/v1/workflow/steps`, {
    method: "POST",
    body: JSON.stringify(body),
  });
  return transformWorkflowStep(response);
}

export async function updateWorkflowStepAction(
  stepId: string,
  payload: Partial<
    Pick<
      WorkflowStep,
      | "name"
      | "position"
      | "color"
      | "prompt"
      | "events"
      | "allow_manual_move"
      | "is_start_step"
      | "auto_archive_after_hours"
    >
  >,
): Promise<WorkflowStep> {
  const body: Record<string, unknown> = {};
  if (payload.name !== undefined) body.name = payload.name;
  if (payload.position !== undefined) body.position = payload.position;
  if (payload.color !== undefined) body.color = payload.color;
  if (payload.prompt !== undefined) body.prompt = payload.prompt;
  if (payload.events !== undefined) body.events = payload.events;
  if (payload.allow_manual_move !== undefined) body.allow_manual_move = payload.allow_manual_move;
  if (payload.is_start_step !== undefined) body.is_start_step = payload.is_start_step;
  if (payload.auto_archive_after_hours !== undefined)
    body.auto_archive_after_hours = payload.auto_archive_after_hours;
  const response = await fetchJson<BackendWorkflowStep>(
    `${apiBaseUrl}/api/v1/workflow/steps/${stepId}`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
  );
  return transformWorkflowStep(response);
}

export async function deleteWorkflowStepAction(stepId: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workflow/steps/${stepId}`, { method: "DELETE" });
}

export async function reorderWorkflowStepsAction(
  workflowId: string,
  stepIds: string[],
): Promise<ListWorkflowStepsResponse> {
  const response = await fetchJson<{ steps: BackendWorkflowStep[] | null }>(
    `${apiBaseUrl}/api/v1/workflows/${workflowId}/workflow/steps/reorder`,
    {
      method: "PUT",
      body: JSON.stringify({ step_ids: stepIds }),
    },
  );
  return {
    steps: (response.steps ?? []).map(transformWorkflowStep),
    total: response.steps?.length ?? 0,
  };
}

// Session Workflow Actions
export async function setPrimarySessionAction(taskId: string, sessionId: string) {
  return fetchJson(`${apiBaseUrl}/api/v1/tasks/${taskId}/primary-session`, {
    method: "PUT",
    body: JSON.stringify({ session_id: sessionId }),
  });
}

export async function approveSessionAction(sessionId: string): Promise<ApproveSessionResponse> {
  return fetchJson<ApproveSessionResponse>(`${apiBaseUrl}/api/v1/sessions/${sessionId}/approve`, {
    method: "POST",
  });
}

export async function getWorkflowTaskCount(workflowId: string): Promise<{ task_count: number }> {
  return fetchJson<{ task_count: number }>(
    `${apiBaseUrl}/api/v1/workflows/${workflowId}/task-count`,
  );
}

export async function getStepTaskCount(stepId: string): Promise<{ task_count: number }> {
  return fetchJson<{ task_count: number }>(
    `${apiBaseUrl}/api/v1/workflow/steps/${stepId}/task-count`,
  );
}

export async function bulkMoveTasks(payload: {
  source_workflow_id: string;
  source_step_id?: string;
  target_workflow_id: string;
  target_step_id: string;
}): Promise<{ moved_count: number }> {
  return fetchJson<{ moved_count: number }>(`${apiBaseUrl}/api/v1/tasks/bulk-move`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function moveSessionToStepAction(sessionId: string, stepId: string) {
  return fetchJson(`${apiBaseUrl}/api/v1/sessions/${sessionId}/workflow-step`, {
    method: "PUT",
    body: JSON.stringify({ step_id: stepId }),
  });
}

// Workflow Export/Import

export async function exportWorkflowAction(workflowId: string): Promise<WorkflowExportData> {
  return fetchJson<WorkflowExportData>(`${apiBaseUrl}/api/v1/workflows/${workflowId}/export`);
}

export async function exportAllWorkflowsAction(workspaceId: string): Promise<WorkflowExportData> {
  return fetchJson<WorkflowExportData>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/workflows/export`,
  );
}

export async function importWorkflowsAction(
  workspaceId: string,
  data: WorkflowExportData,
): Promise<ImportWorkflowsResult> {
  return fetchJson<ImportWorkflowsResult>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/workflows/import`,
    {
      method: "POST",
      body: JSON.stringify(data),
    },
  );
}
