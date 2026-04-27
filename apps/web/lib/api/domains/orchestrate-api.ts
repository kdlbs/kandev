import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AgentInstance,
  Skill,
  Project,
  CostSummary,
  BudgetPolicy,
  Routine,
  RoutineTrigger,
  RoutineRun,
  Approval,
  ActivityEntry,
  InboxItem,
  WakeupEntry,
  DashboardData,
  OrchestrateIssue,
  OrchestrateMeta,
} from "@/lib/state/slices/orchestrate/types";

const BASE = "/api/v1/orchestrate";

export const getMeta = (options?: ApiRequestOptions) =>
  fetchJson<OrchestrateMeta>(`${BASE}/meta`, options);

// --- Agent Instances ---

export function listAgentInstances(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ agents: AgentInstance[] }>(
    `${BASE}/workspaces/${workspaceId}/agents`,
    options,
  );
}

export function createAgentInstance(
  workspaceId: string,
  data: Partial<AgentInstance>,
  options?: ApiRequestOptions,
) {
  return fetchJson<AgentInstance>(`${BASE}/workspaces/${workspaceId}/agents`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export const getAgentInstance = (id: string, options?: ApiRequestOptions) =>
  fetchJson<AgentInstance>(`${BASE}/agents/${id}`, options);

export function updateAgentInstance(
  id: string,
  data: Partial<AgentInstance>,
  options?: ApiRequestOptions,
) {
  return fetchJson<AgentInstance>(`${BASE}/agents/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteAgentInstance(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Skills ---

export function listSkills(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ skills: Skill[] }>(`${BASE}/workspaces/${workspaceId}/skills`, options);
}

export function createSkill(
  workspaceId: string,
  data: Partial<Skill>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ skill: Skill }>(`${BASE}/workspaces/${workspaceId}/skills`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function getSkill(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ skill: Skill }>(`${BASE}/skills/${id}`, options);
}

export function updateSkill(id: string, data: Partial<Skill>, options?: ApiRequestOptions) {
  return fetchJson<{ skill: Skill }>(`${BASE}/skills/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteSkill(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/skills/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function importSkill(workspaceId: string, source: string, options?: ApiRequestOptions) {
  return fetchJson<{ skills: Skill[]; warnings: string[] }>(
    `${BASE}/workspaces/${workspaceId}/skills/import`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify({ source }), ...options?.init },
    },
  );
}

export function getSkillFile(skillId: string, path: string, options?: ApiRequestOptions) {
  return fetchJson<{ path: string; content: string }>(
    `${BASE}/skills/${skillId}/files?path=${encodeURIComponent(path)}`,
    options,
  );
}

// --- Projects ---

export function listProjects(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ projects: Project[] }>(`${BASE}/workspaces/${workspaceId}/projects`, options);
}

export function createProject(
  workspaceId: string,
  data: Partial<Project>,
  options?: ApiRequestOptions,
) {
  return fetchJson<Project>(`${BASE}/workspaces/${workspaceId}/projects`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function getProject(id: string, options?: ApiRequestOptions) {
  return fetchJson<Project>(`${BASE}/projects/${id}`, options);
}

export function updateProject(id: string, data: Partial<Project>, options?: ApiRequestOptions) {
  return fetchJson<Project>(`${BASE}/projects/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteProject(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/projects/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Costs ---

export const getCosts = (workspaceId: string, options?: ApiRequestOptions) =>
  fetchJson<CostSummary>(`${BASE}/workspaces/${workspaceId}/costs`, options);

export function getCostSummary(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ total_cents: number }>(
    `${BASE}/workspaces/${workspaceId}/costs/summary`,
    options,
  );
}

export function getCostsByAgent(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<CostSummary["byAgent"]>(
    `${BASE}/workspaces/${workspaceId}/costs/by-agent`,
    options,
  );
}

export function getCostsByProject(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<CostSummary["byProject"]>(
    `${BASE}/workspaces/${workspaceId}/costs/by-project`,
    options,
  );
}

export function getCostsByModel(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<CostSummary["byModel"]>(
    `${BASE}/workspaces/${workspaceId}/costs/by-model`,
    options,
  );
}

// --- Budget Policies ---

export function listBudgets(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ budgets: BudgetPolicy[] }>(
    `${BASE}/workspaces/${workspaceId}/budgets`,
    options,
  );
}

export function createBudget(
  workspaceId: string,
  data: Partial<BudgetPolicy>,
  options?: ApiRequestOptions,
) {
  return fetchJson<BudgetPolicy>(`${BASE}/workspaces/${workspaceId}/budgets`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function updateBudget(id: string, data: Partial<BudgetPolicy>, options?: ApiRequestOptions) {
  return fetchJson<BudgetPolicy>(`${BASE}/budgets/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteBudget(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/budgets/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Routines ---

export function listRoutines(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ routines: Routine[] }>(`${BASE}/workspaces/${workspaceId}/routines`, options);
}

export function createRoutine(
  workspaceId: string,
  data: Partial<Routine>,
  options?: ApiRequestOptions,
) {
  return fetchJson<Routine>(`${BASE}/workspaces/${workspaceId}/routines`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function getRoutine(id: string, options?: ApiRequestOptions) {
  return fetchJson<Routine>(`${BASE}/routines/${id}`, options);
}

export function updateRoutine(id: string, data: Partial<Routine>, options?: ApiRequestOptions) {
  return fetchJson<Routine>(`${BASE}/routines/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteRoutine(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/routines/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function runRoutine(
  id: string,
  variables?: Record<string, string>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ run: RoutineRun }>(`${BASE}/routines/${id}/run`, {
    ...options,
    init: {
      method: "POST",
      body: variables ? JSON.stringify({ variables }) : undefined,
      ...options?.init,
    },
  });
}

export function listRoutineTriggers(routineId: string, options?: ApiRequestOptions) {
  return fetchJson<{ triggers: RoutineTrigger[] }>(
    `${BASE}/routines/${routineId}/triggers`,
    options,
  );
}

export function createRoutineTrigger(
  routineId: string,
  data: Partial<RoutineTrigger>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ trigger: RoutineTrigger }>(`${BASE}/routines/${routineId}/triggers`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteRoutineTrigger(triggerId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/routine-triggers/${triggerId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function listRoutineRuns(routineId: string, options?: ApiRequestOptions) {
  return fetchJson<{ runs: RoutineRun[] }>(`${BASE}/routines/${routineId}/runs`, options);
}

export function listAllRoutineRuns(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ runs: RoutineRun[] }>(
    `${BASE}/workspaces/${workspaceId}/routine-runs`,
    options,
  );
}

// --- Approvals ---

export function listApprovals(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ approvals: Approval[] }>(
    `${BASE}/workspaces/${workspaceId}/approvals`,
    options,
  );
}

export function decideApproval(
  id: string,
  decision: { status: "approved" | "rejected"; note?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<Approval>(`${BASE}/approvals/${id}/decide`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(decision), ...options?.init },
  });
}

// --- Activity ---

export function listActivity(
  workspaceId: string,
  filterType?: string,
  options?: ApiRequestOptions,
) {
  const query = filterType && filterType !== "all" ? `?type=${filterType}` : "";
  return fetchJson<{ activity: ActivityEntry[] }>(
    `${BASE}/workspaces/${workspaceId}/activity${query}`,
    options,
  );
}

// --- Inbox ---

export function getInbox(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ items: InboxItem[] }>(`${BASE}/workspaces/${workspaceId}/inbox`, options);
}

export function getInboxCount(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ count: number }>(
    `${BASE}/workspaces/${workspaceId}/inbox?count=true`,
    options,
  );
}

// --- Agent Memory ---

export function getMemory(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    memory: Array<{
      id: string;
      layer: string;
      key: string;
      content: string;
      metadata: string;
      updated_at: string;
    }>;
  }>(`${BASE}/agents/${agentId}/memory`, options);
}

export function putMemory(
  agentId: string,
  data: { layer: string; key: string; content: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<void>(`${BASE}/agents/${agentId}/memory`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify({ entries: [data] }), ...options?.init },
  });
}

export function deleteMemory(agentId: string, entryId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${agentId}/memory/${entryId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function deleteAllMemory(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${agentId}/memory/all`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function exportMemory(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    memory: Array<{
      id: string;
      layer: string;
      key: string;
      content: string;
      metadata: string;
      updated_at: string;
    }>;
  }>(`${BASE}/agents/${agentId}/memory/export`, options);
}

export function getMemorySummary(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{ count: number }>(`${BASE}/agents/${agentId}/memory/summary`, options);
}

// --- Channels ---

export function listChannels(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    channels: Array<{
      id: string;
      platform: string;
      config: string;
      status: string;
      task_id: string;
      created_at: string;
    }>;
  }>(`${BASE}/agents/${agentId}/channels`, options);
}

export function setupChannel(
  agentId: string,
  data: { workspace_id: string; platform: string; config: string; status: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{
    channel: {
      id: string;
      platform: string;
      config: string;
      status: string;
      task_id: string;
      created_at: string;
    };
  }>(`${BASE}/agents/${agentId}/channels`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function deleteChannel(agentId: string, channelId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${agentId}/channels/${channelId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

// --- Config Export/Import ---

export function exportConfig(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ bundle: Record<string, unknown> }>(
    `${BASE}/workspaces/${workspaceId}/config/export`,
    options,
  );
}

export const exportConfigZipUrl = (workspaceId: string) =>
  `${BASE}/workspaces/${workspaceId}/config/export/zip`;

export function previewImport(
  workspaceId: string,
  bundle: Record<string, unknown>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{
    preview: {
      agents: { created: string[]; updated: string[]; deleted: string[] };
      skills: { created: string[]; updated: string[]; deleted: string[] };
      routines: { created: string[]; updated: string[]; deleted: string[] };
      projects: { created: string[]; updated: string[]; deleted: string[] };
    };
  }>(`${BASE}/workspaces/${workspaceId}/config/preview`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(bundle), ...options?.init },
  });
}

export function applyImport(
  workspaceId: string,
  bundle: Record<string, unknown>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ result: { created_count: number; updated_count: number } }>(
    `${BASE}/workspaces/${workspaceId}/config/import`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify(bundle), ...options?.init },
    },
  );
}

// --- Config Sync (FS <-> DB) ---

export type ImportDiff = {
  created: string[];
  updated: string[];
  deleted: string[];
};

export type ImportPreview = {
  agents: ImportDiff;
  skills: ImportDiff;
  routines: ImportDiff;
  projects: ImportDiff;
};

export type ParseError = {
  workspace_id: string;
  file_path: string;
  error: string;
};

export type SyncDiff = {
  direction: "incoming" | "outgoing";
  preview: ImportPreview;
  errors?: ParseError[];
};

export function getIncomingDiff(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ diff: SyncDiff }>(
    `${BASE}/workspaces/${workspaceId}/config/sync/incoming`,
    options,
  );
}

export function getOutgoingDiff(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ diff: SyncDiff }>(
    `${BASE}/workspaces/${workspaceId}/config/sync/outgoing`,
    options,
  );
}

export function applyIncomingSync(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ result: { created_count: number; updated_count: number } }>(
    `${BASE}/workspaces/${workspaceId}/config/sync/import-fs`,
    { ...options, init: { method: "POST", ...options?.init } },
  );
}

export function applyOutgoingSync(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ status: string }>(`${BASE}/workspaces/${workspaceId}/config/sync/export-fs`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

// --- Issues ---

export function listIssues(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ issues: OrchestrateIssue[] }>(
    `${BASE}/workspaces/${workspaceId}/issues`,
    options,
  );
}

export function getIssue(issueId: string, options?: ApiRequestOptions) {
  return fetchJson<{ issue: OrchestrateIssue }>(`${BASE}/issues/${issueId}`, options);
}

// --- Comments ---

export type TaskCommentResponse = {
  id: string;
  taskId: string;
  authorType: string;
  authorId: string;
  body: string;
  source: string;
  createdAt: string;
};

export function listComments(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ comments: TaskCommentResponse[] }>(
    `${BASE}/tasks/${taskId}/comments`,
    options,
  );
}

export function createComment(
  taskId: string,
  data: { body: string; author_type?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ comment: TaskCommentResponse }>(
    `${BASE}/tasks/${taskId}/comments`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify(data), ...options?.init },
    },
  );
}

export function searchTasks(
  workspaceId: string,
  query: string,
  limit = 50,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  return fetchJson<{ tasks: OrchestrateIssue[] }>(
    `${BASE}/workspaces/${workspaceId}/tasks/search?${params.toString()}`,
    options,
  );
}

// --- Onboarding ---

export type OnboardingStateData = {
  completed: boolean;
  workspaceId?: string;
  ceoAgentId?: string;
};

export function getOnboardingState(options?: ApiRequestOptions) {
  return fetchJson<OnboardingStateData>(`${BASE}/onboarding-state`, options);
}

export type OnboardingCompletePayload = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  taskTitle?: string;
  taskDescription?: string;
};

export type OnboardingCompleteResult = {
  workspaceId: string;
  agentId: string;
  projectId?: string;
  taskId?: string;
};

export function completeOnboarding(data: OnboardingCompletePayload, options?: ApiRequestOptions) {
  return fetchJson<OnboardingCompleteResult>(`${BASE}/onboarding/complete`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

// --- Dashboard ---

export function getDashboard(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<DashboardData>(`${BASE}/workspaces/${workspaceId}/dashboard`, options);
}

// --- Wakeups ---

export function listWakeups(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ wakeups: WakeupEntry[] }>(
    `${BASE}/workspaces/${workspaceId}/wakeups`,
    options,
  );
}

// --- Workspace Settings ---

export function updateWorkspaceSettings(
  workspaceId: string,
  data: {
    name?: string;
    description?: string;
    require_approval_for_new_agents?: boolean;
    require_approval_for_task_completion?: boolean;
    require_approval_for_skill_changes?: boolean;
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/settings`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
}

// --- Git ---

export type GitStatusData = {
  is_git: boolean;
  branch?: string;
  is_dirty: boolean;
  has_remote: boolean;
  ahead: number;
  behind: number;
  commit_count: number;
};

export function getGitStatus(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<GitStatusData>(`${BASE}/workspaces/${workspaceId}/git/status`, options);
}

export function gitClone(
  workspaceId: string,
  data: { repoUrl: string; branch: string; workspaceName: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/git/clone`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}

export function gitPull(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/git/pull`, {
    ...options,
    init: { method: "POST", ...options?.init },
  });
}

export function gitPush(
  workspaceId: string,
  data: { message: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/git/push`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
}
