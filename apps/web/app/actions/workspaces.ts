/* eslint-disable max-lines -- Workspace and workflow actions share one server-action boundary. */

"use server";

import { getBackendConfig } from "@/lib/config";
import { workflowId as toWorkflowId } from "@/lib/types/ids";
import {
  normalizeWorkflowProfileSessionStartPolicy,
  normalizeWorkflowProfileSessionEndPolicy,
} from "@/lib/types/http";
import type {
  ApproveSessionResponse,
  Workflow,
  ListWorkflowsResponse,
  ListTasksResponse,
  ListWorkflowStepsResponse,
  RepositoryDiscoveryResponse,
  ListRepositoriesResponse,
  LocalRepositoryStatusResponse,
  ListRepositoryScriptsResponse,
  ListWorkspacesResponse,
  RepositoryPathValidationResponse,
  Repository,
  RepositorySecretBinding,
  RepositoryScript,
  StepEvents,
  Workspace,
  WorkflowStep,
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
  const text = response.status === 204 ? "" : await response.text();
  if (!response.ok) {
    let message = `Request failed: ${response.status} ${response.statusText}`;
    if (text) {
      try {
        const body = JSON.parse(text) as { error?: string; message?: string };
        const detail = body.error ?? body.message;
        if (detail) {
          message = detail;
        }
      } catch {
        // body was not JSON, fall back to status text
      }
    }
    throw new Error(message);
  }
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
    default_config_agent_profile_id?: string;
  },
) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

// Workspace deletion has two backend routes and the caller must pick by flag.
//
// The office route (`/api/v1/office/workspaces/:id`) additionally purges
// office-owned rows, which have no FK cascade to `workspaces` and are removed by
// an explicit statement list in
// `internal/office/repository/sqlite/workspace_deletion.go`. But the whole
// `/api/v1/office` router group is only mounted when the office services exist,
// and those are skipped when `features.office` is off — the production default.
// Calling it there 404s, which is how Settings deletion broke for every
// non-office install.
//
// So: office on -> office route (cascade plus office cleanup); office off ->
// generic route, which owns the same task/workflow cascade and confirm-name
// guard and is the only one mounted. `officeEnabled` is required rather than
// defaulted so a new call site has to make the choice explicitly.
export async function deleteWorkspaceAction(
  id: string,
  confirmName: string,
  officeEnabled: boolean,
) {
  const path = officeEnabled ? `office/workspaces/${id}` : `workspaces/${id}`;
  await fetchJson<void>(`${apiBaseUrl}/api/v1/${path}`, {
    method: "DELETE",
    body: JSON.stringify({ confirm_name: confirmName }),
  });
}

export async function listWorkflowsAction(workspaceId: string): Promise<ListWorkflowsResponse> {
  const url = new URL(`${apiBaseUrl}/api/v1/workflows`);
  url.searchParams.set("workspace_id", workspaceId);
  return fetchJson<ListWorkflowsResponse>(url.toString());
}

type ListTasksOptions = {
  page?: number;
  pageSize?: number;
  query?: string;
  workflowId?: string | null;
  repositoryId?: string | null;
  sort?: string;
};

export async function listTasksByWorkspaceAction(
  workspaceId: string,
  options: ListTasksOptions = {},
): Promise<ListTasksResponse> {
  const { page = 1, pageSize = 50, query, workflowId, repositoryId, sort } = options;
  const url = new URL(`${apiBaseUrl}/api/v1/workspaces/${workspaceId}/tasks`);
  url.searchParams.set("page", String(page));
  url.searchParams.set("page_size", String(pageSize));
  if (query) url.searchParams.set("query", query);
  if (workflowId) url.searchParams.set("workflow_id", workflowId);
  if (repositoryId) url.searchParams.set("repository_id", repositoryId);
  if (sort) url.searchParams.set("sort", sort);
  return fetchJson<ListTasksResponse>(url.toString());
}

export async function createWorkflowAction(payload: {
  workspace_id: string;
  name: string;
  description?: string;
  prompt?: string;
  workflow_template_id?: string;
}) {
  return fetchJson<Workflow>(`${apiBaseUrl}/api/v1/workflows`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateWorkflowAction(
  id: string,
  payload: {
    name?: string;
    description?: string;
    prompt?: string;
    agent_profile_id?: string;
  },
) {
  return fetchJson<Workflow>(`${apiBaseUrl}/api/v1/workflows/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function deleteWorkflowAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workflows/${id}`, { method: "DELETE" });
}

export async function reorderWorkflowsAction(workspaceId: string, workflowIds: string[]) {
  return fetchJson<{ success: boolean }>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/workflows/reorder`,
    { method: "PUT", body: JSON.stringify({ workflow_ids: workflowIds }) },
  );
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

// Path-based branch listing moved to lib/api/domains/workspace-api.ts as a
// unified `listBranches({ repositoryId | path })` after the endpoint unification.
// Local repo status (branch + dirty files) backs the fresh-branch consent flow.
export async function getLocalRepositoryStatusAction(
  workspaceId: string,
  path: string,
): Promise<LocalRepositoryStatusResponse> {
  const params = `?path=${encodeURIComponent(path)}`;
  return fetchJson<LocalRepositoryStatusResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/local-status${params}`,
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

export {
  getRepositoryDiscoveryAction,
  refreshRepositoryDiscoveryAction,
  listDesktopDiscoveryRootsAction,
  addDesktopDiscoveryRootAction,
  reconnectDesktopDiscoveryRootAction,
  removeDesktopDiscoveryRootAction,
} from "./repository-discovery";

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
  provider_host?: string;
  provider_owner: string;
  provider_name: string;
  default_branch: string;
  worktree_branch_prefix: string;
  worktree_branch_template: string;
  pull_before_worktree: boolean;
  setup_script: string;
  cleanup_script: string;
  dev_script: string;
  copy_files: string;
  secret_bindings?: RepositorySecretBinding[];
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
        provider_host: payload.provider_host,
        provider_owner: payload.provider_owner,
        provider_name: payload.provider_name,
        default_branch: payload.default_branch,
        worktree_branch_prefix: payload.worktree_branch_prefix,
        worktree_branch_template: payload.worktree_branch_template,
        pull_before_worktree: payload.pull_before_worktree,
        setup_script: payload.setup_script,
        cleanup_script: payload.cleanup_script,
        dev_script: payload.dev_script,
        copy_files: payload.copy_files,
        secret_bindings: payload.secret_bindings,
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
  is_start_step?: boolean;
  show_in_command_panel?: boolean;
  agent_profile_id?: StepDefinition["agent_profile_id"];
  profile_session_start_policy?: WorkflowStep["profile_session_start_policy"];
  profile_session_end_policy?: WorkflowStep["profile_session_end_policy"];
  auto_advance_requires_signal?: boolean;
  cancel_triggers_turn_complete?: boolean;
  wip_limit?: number;
  pull_from_step_id?: string | null;
};

type BackendWorkflowTemplate = Omit<WorkflowTemplate, "default_steps"> & {
  steps?: BackendTemplateStep[];
  default_steps?: BackendTemplateStep[];
};

const normalizeWorkflowTemplate = (template: BackendWorkflowTemplate): WorkflowTemplate => {
  const steps = template.default_steps ?? template.steps ?? [];
  const default_steps: StepDefinition[] = steps.map((step) => ({
    ...step,
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      step.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      step.profile_session_end_policy,
    ),
    pull_from_step_id: step.pull_from_step_id ?? null,
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
  show_in_command_panel?: boolean;
  auto_archive_after_hours?: number;
  agent_profile_id?: string;
  profile_session_start_policy?: WorkflowStep["profile_session_start_policy"];
  profile_session_end_policy?: WorkflowStep["profile_session_end_policy"];
  auto_advance_requires_signal?: boolean;
  cancel_triggers_turn_complete?: boolean;
  wip_limit?: number;
  pull_from_step_id?: string | null;
  stage_type?: WorkflowStep["stage_type"];
  created_at: string;
  updated_at: string;
};

const transformWorkflowStep = (step: BackendWorkflowStep): WorkflowStep => ({
  id: step.id,
  workflow_id: toWorkflowId(step.workflow_id),
  name: step.name,
  position: step.position,
  color: step.color,
  prompt: step.prompt,
  events: step.events,
  allow_manual_move: step.allow_manual_move,
  is_start_step: step.is_start_step,
  show_in_command_panel: step.show_in_command_panel,
  auto_archive_after_hours: step.auto_archive_after_hours,
  agent_profile_id: step.agent_profile_id,
  profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
    step.profile_session_start_policy,
  ),
  profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
    step.profile_session_end_policy,
  ),
  auto_advance_requires_signal: step.auto_advance_requires_signal,
  cancel_triggers_turn_complete: step.cancel_triggers_turn_complete,
  wip_limit: step.wip_limit ?? 0,
  pull_from_step_id: step.pull_from_step_id ?? null,
  stage_type: step.stage_type,
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
  show_in_command_panel?: boolean;
  agent_profile_id?: string;
  allow_manual_move?: boolean;
  auto_advance_requires_signal?: boolean;
  wip_limit?: number;
  pull_from_step_id?: string | null;
  stage_type?: WorkflowStep["stage_type"];
  cancel_triggers_turn_complete?: boolean;
  profile_session_start_policy?: WorkflowStep["profile_session_start_policy"];
  profile_session_end_policy?: WorkflowStep["profile_session_end_policy"];
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
    show_in_command_panel: payload.show_in_command_panel ?? true,
    agent_profile_id: payload.agent_profile_id,
    wip_limit: payload.wip_limit ?? 0,
    pull_from_step_id: payload.pull_from_step_id ?? "",
    stage_type: payload.stage_type,
    cancel_triggers_turn_complete: payload.cancel_triggers_turn_complete ?? false,
    profile_session_start_policy: payload.profile_session_start_policy,
    profile_session_end_policy: payload.profile_session_end_policy,
    auto_advance_requires_signal: payload.auto_advance_requires_signal ?? false,
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
      | "show_in_command_panel"
      | "auto_archive_after_hours"
      | "agent_profile_id"
      | "auto_advance_requires_signal"
      | "cancel_triggers_turn_complete"
      | "wip_limit"
      | "pull_from_step_id"
      | "stage_type"
      | "profile_session_start_policy"
      | "profile_session_end_policy"
    >
  >,
): Promise<WorkflowStep> {
  const body = Object.fromEntries(
    Object.entries(payload).filter(([, value]) => value !== undefined),
  ) as Record<string, unknown>;
  if (body.profile_session_start_policy !== undefined)
    body.profile_session_start_policy = normalizeWorkflowProfileSessionStartPolicy(
      body.profile_session_start_policy,
    );
  if (body.profile_session_end_policy !== undefined)
    body.profile_session_end_policy = normalizeWorkflowProfileSessionEndPolicy(
      body.profile_session_end_policy,
    );
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

export async function getRepositoryActiveSessionCountAction(
  repositoryId: string,
): Promise<{ active_session_count: number }> {
  return fetchJson<{ active_session_count: number }>(
    `${apiBaseUrl}/api/v1/repositories/${repositoryId}/active-session-count`,
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

export async function exportWorkflowAction(workflowId: string): Promise<string> {
  const response = await fetch(`${apiBaseUrl}/api/v1/workflows/${workflowId}/export`);
  if (!response.ok) throw new Error(`Export failed: ${response.statusText}`);
  return response.text();
}

export async function exportAllWorkflowsAction(
  workspaceId: string,
  workflowIds?: string[],
): Promise<string> {
  // When workflowIds is provided, restrict the export to exactly that set
  // (the settings UI passes its kanban workflows, excluding office ones). An
  // empty list is sent intentionally as `ids=` so nothing is exported, rather
  // than omitting the param and falling back to exporting every workflow.
  // Encode the user-provided values that flow into the request URL (workspace
  // ID in the path, workflow IDs in the query) so they can't alter the request
  // target — clears CodeQL's js/request-forgery (SSRF) check on this path.
  const query =
    workflowIds !== undefined ? `?ids=${encodeURIComponent(workflowIds.join(","))}` : "";
  const response = await fetch(
    `${apiBaseUrl}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/workflows/export${query}`,
  );
  if (!response.ok) throw new Error(`Export failed: ${response.statusText}`);
  return response.text();
}

export async function importWorkflowsAction(
  workspaceId: string,
  yamlContent: string,
): Promise<ImportWorkflowsResult> {
  return fetchJson<ImportWorkflowsResult>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/workflows/import`,
    {
      method: "POST",
      headers: { "Content-Type": "application/x-yaml" },
      body: yamlContent,
    },
  );
}
