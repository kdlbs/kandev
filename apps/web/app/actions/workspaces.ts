'use server';

import { getBackendConfig } from '@/lib/config';
import type {
  ApproveSessionResponse,
  Board,
  ListBoardsResponse,
  ListWorkflowStepsResponse,
  RepositoryDiscoveryResponse,
  ListRepositoriesResponse,
  RepositoryBranchesResponse,
  ListRepositoryScriptsResponse,
  ListWorkspacesResponse,
  RepositoryPathValidationResponse,
  Repository,
  RepositoryScript,
  StepBehaviors,
  Workspace,
  WorkflowStep,
  WorkflowStepType,
  ListWorkflowTemplatesResponse,
  WorkflowTemplate,
  StepDefinition,
} from '@/lib/types/http';

const { apiBaseUrl } = getBackendConfig();

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
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
    method: 'POST',
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
  }
) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteWorkspaceAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workspaces/${id}`, { method: 'DELETE' });
}

export async function listBoardsAction(workspaceId: string): Promise<ListBoardsResponse> {
  const url = new URL(`${apiBaseUrl}/api/v1/boards`);
  url.searchParams.set('workspace_id', workspaceId);
  return fetchJson<ListBoardsResponse>(url.toString());
}

export async function createBoardAction(payload: {
  workspace_id: string;
  name: string;
  description?: string;
  workflow_template_id?: string;
}) {
  return fetchJson<Board>(`${apiBaseUrl}/api/v1/boards`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateBoardAction(id: string, payload: { name?: string; description?: string }) {
  return fetchJson<Board>(`${apiBaseUrl}/api/v1/boards/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteBoardAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/boards/${id}`, { method: 'DELETE' });
}

export async function listRepositoriesAction(workspaceId: string): Promise<ListRepositoriesResponse> {
  return fetchJson<ListRepositoriesResponse>(`${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories`);
}

export async function listLocalRepositoryBranchesAction(
  workspaceId: string,
  path: string
): Promise<RepositoryBranchesResponse> {
  const params = `?path=${encodeURIComponent(path)}`;
  return fetchJson<RepositoryBranchesResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/branches${params}`
  );
}

export async function discoverRepositoriesAction(
  workspaceId: string,
  root?: string
): Promise<RepositoryDiscoveryResponse> {
  const params = root ? `?root=${encodeURIComponent(root)}` : '';
  return fetchJson<RepositoryDiscoveryResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/discover${params}`
  );
}

export async function validateRepositoryPathAction(
  workspaceId: string,
  path: string
): Promise<RepositoryPathValidationResponse> {
  const params = `?path=${encodeURIComponent(path)}`;
  return fetchJson<RepositoryPathValidationResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/validate${params}`
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
  return fetchJson<Repository>(`${apiBaseUrl}/api/v1/workspaces/${payload.workspace_id}/repositories`, {
    method: 'POST',
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
  });
}

export async function updateRepositoryAction(id: string, payload: Partial<Repository>) {
  return fetchJson<Repository>(`${apiBaseUrl}/api/v1/repositories/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteRepositoryAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/repositories/${id}`, { method: 'DELETE' });
}

export async function listRepositoryScriptsAction(repositoryId: string): Promise<ListRepositoryScriptsResponse> {
  return fetchJson<ListRepositoryScriptsResponse>(`${apiBaseUrl}/api/v1/repositories/${repositoryId}/scripts`);
}

export async function createRepositoryScriptAction(payload: {
  repository_id: string;
  name: string;
  command: string;
  position: number;
}) {
  return fetchJson<RepositoryScript>(`${apiBaseUrl}/api/v1/repositories/${payload.repository_id}/scripts`, {
    method: 'POST',
    body: JSON.stringify({
      name: payload.name,
      command: payload.command,
      position: payload.position,
    }),
  });
}

export async function updateRepositoryScriptAction(id: string, payload: Partial<RepositoryScript>) {
  return fetchJson<RepositoryScript>(`${apiBaseUrl}/api/v1/scripts/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export async function deleteRepositoryScriptAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/scripts/${id}`, { method: 'DELETE' });
}

type BackendWorkflowTemplateStep = {
  name: string;
  step_type: WorkflowStepType;
  position: number;
  color?: string;
  auto_start_agent?: boolean;
  plan_mode?: boolean;
  require_approval?: boolean;
  prompt_prefix?: string;
  prompt_suffix?: string;
};

type BackendWorkflowTemplate = Omit<WorkflowTemplate, 'default_steps'> & {
  steps?: BackendWorkflowTemplateStep[];
  default_steps?: BackendWorkflowTemplateStep[];
};

const toStepBehaviors = (step: BackendWorkflowTemplateStep): StepBehaviors | undefined => {
  const behaviors: StepBehaviors = {};
  if (step.auto_start_agent !== undefined) {
    behaviors.autoStartAgent = step.auto_start_agent;
  }
  if (step.plan_mode !== undefined) {
    behaviors.planMode = step.plan_mode;
  }
  if (step.require_approval !== undefined) {
    behaviors.requireApproval = step.require_approval;
  }
  if (step.prompt_prefix) {
    behaviors.promptPrefix = step.prompt_prefix;
  }
  if (step.prompt_suffix) {
    behaviors.promptSuffix = step.prompt_suffix;
  }
  return Object.keys(behaviors).length ? behaviors : undefined;
};

const normalizeWorkflowTemplate = (template: BackendWorkflowTemplate): WorkflowTemplate => {
  const steps = template.default_steps ?? template.steps ?? [];
  const default_steps: StepDefinition[] = steps.map((step) => ({
    name: step.name,
    step_type: step.step_type,
    position: step.position,
    color: step.color,
    behaviors: toStepBehaviors(step),
  }));
  return {
    ...template,
    default_steps,
  };
};

// Workflow Templates
export async function listWorkflowTemplatesAction(): Promise<ListWorkflowTemplatesResponse> {
  const response = await fetchJson<ListWorkflowTemplatesResponse>(
    `${apiBaseUrl}/api/v1/workflow/templates`
  );
  return {
    ...response,
    templates: (response.templates ?? []).map((template) =>
      normalizeWorkflowTemplate(template as BackendWorkflowTemplate)
    ),
  };
}

// Helper to transform backend workflow step (flat fields) to frontend format (nested behaviors)
type BackendWorkflowStep = {
  id: string;
  board_id: string;
  name: string;
  step_type: WorkflowStepType;
  position: number;
  color: string;
  task_state?: string;
  auto_start_agent?: boolean;
  plan_mode?: boolean;
  require_approval?: boolean;
  prompt_prefix?: string;
  prompt_suffix?: string;
  on_complete_step_id?: string;
  on_approval_step_id?: string;
  allow_manual_move?: boolean;
  created_at: string;
  updated_at: string;
};

function transformWorkflowStep(step: BackendWorkflowStep): WorkflowStep {
  return {
    id: step.id,
    board_id: step.board_id,
    name: step.name,
    step_type: step.step_type,
    position: step.position,
    color: step.color,
    behaviors: {
      autoStartAgent: step.auto_start_agent ?? false,
      planMode: step.plan_mode ?? false,
      requireApproval: step.require_approval ?? false,
      promptPrefix: step.prompt_prefix ?? '',
      promptSuffix: step.prompt_suffix ?? '',
    },
    created_at: step.created_at,
    updated_at: step.updated_at,
  };
}

// Workflow Steps
export async function listWorkflowStepsAction(boardId: string): Promise<ListWorkflowStepsResponse> {
  const response = await fetchJson<{ steps: BackendWorkflowStep[] | null }>(
    `${apiBaseUrl}/api/v1/boards/${boardId}/workflow/steps`
  );
  return {
    steps: (response.steps ?? []).map(transformWorkflowStep),
    total: response.steps?.length ?? 0,
  };
}

export async function createWorkflowStepAction(payload: {
  board_id: string;
  name: string;
  step_type: WorkflowStepType;
  position: number;
  color: string;
  behaviors?: StepBehaviors;
}): Promise<WorkflowStep> {
  // Flatten behaviors into top-level fields for the backend
  const body = {
    board_id: payload.board_id,
    name: payload.name,
    step_type: payload.step_type,
    position: payload.position,
    color: payload.color,
    auto_start_agent: payload.behaviors?.autoStartAgent ?? false,
    plan_mode: payload.behaviors?.planMode ?? false,
    require_approval: payload.behaviors?.requireApproval ?? false,
    prompt_prefix: payload.behaviors?.promptPrefix ?? '',
    prompt_suffix: payload.behaviors?.promptSuffix ?? '',
    allow_manual_move: true,
  };
  const response = await fetchJson<BackendWorkflowStep>(`${apiBaseUrl}/api/v1/workflow/steps`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
  return transformWorkflowStep(response);
}

export async function updateWorkflowStepAction(
  stepId: string,
  payload: Partial<Pick<WorkflowStep, 'name' | 'step_type' | 'position' | 'color' | 'behaviors'>>
): Promise<WorkflowStep> {
  // Flatten behaviors into top-level fields for the backend
  const body: Record<string, unknown> = {};
  if (payload.name !== undefined) body.name = payload.name;
  if (payload.step_type !== undefined) body.step_type = payload.step_type;
  if (payload.position !== undefined) body.position = payload.position;
  if (payload.color !== undefined) body.color = payload.color;
  if (payload.behaviors !== undefined) {
    if (payload.behaviors.autoStartAgent !== undefined) body.auto_start_agent = payload.behaviors.autoStartAgent;
    if (payload.behaviors.planMode !== undefined) body.plan_mode = payload.behaviors.planMode;
    if (payload.behaviors.requireApproval !== undefined) body.require_approval = payload.behaviors.requireApproval;
    if (payload.behaviors.promptPrefix !== undefined) body.prompt_prefix = payload.behaviors.promptPrefix;
    if (payload.behaviors.promptSuffix !== undefined) body.prompt_suffix = payload.behaviors.promptSuffix;
  }
  const response = await fetchJson<BackendWorkflowStep>(`${apiBaseUrl}/api/v1/workflow/steps/${stepId}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  });
  return transformWorkflowStep(response);
}

export async function deleteWorkflowStepAction(stepId: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workflow/steps/${stepId}`, { method: 'DELETE' });
}

export async function reorderWorkflowStepsAction(boardId: string, stepIds: string[]): Promise<ListWorkflowStepsResponse> {
  const response = await fetchJson<{ steps: BackendWorkflowStep[] | null }>(
    `${apiBaseUrl}/api/v1/boards/${boardId}/workflow/steps/reorder`,
    {
      method: 'PUT',
      body: JSON.stringify({ step_ids: stepIds }),
    }
  );
  return {
    steps: (response.steps ?? []).map(transformWorkflowStep),
    total: response.steps?.length ?? 0,
  };
}

// Session Workflow Actions
export async function setPrimarySessionAction(taskId: string, sessionId: string) {
  return fetchJson(`${apiBaseUrl}/api/v1/tasks/${taskId}/primary-session`, {
    method: 'PUT',
    body: JSON.stringify({ session_id: sessionId }),
  });
}

export async function approveSessionAction(sessionId: string): Promise<ApproveSessionResponse> {
  return fetchJson<ApproveSessionResponse>(`${apiBaseUrl}/api/v1/sessions/${sessionId}/approve`, {
    method: 'POST',
  });
}



export async function moveSessionToStepAction(sessionId: string, stepId: string) {
  return fetchJson(`${apiBaseUrl}/api/v1/sessions/${sessionId}/workflow-step`, {
    method: 'PUT',
    body: JSON.stringify({ step_id: stepId }),
  });
}
