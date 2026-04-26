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
} from "@/lib/state/slices/orchestrate/types";

const BASE = "/api/v1/orchestrate";

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

export function getAgentInstance(id: string, options?: ApiRequestOptions) {
  return fetchJson<AgentInstance>(`${BASE}/agents/${id}`, options);
}

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

// --- Projects ---

export function listProjects(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ projects: Project[] }>(
    `${BASE}/workspaces/${workspaceId}/projects`,
    options,
  );
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

export function getCosts(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<CostSummary>(`${BASE}/workspaces/${workspaceId}/costs`, options);
}

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
  return fetchJson<{ routines: Routine[] }>(
    `${BASE}/workspaces/${workspaceId}/routines`,
    options,
  );
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
  return fetchJson<{ memory: Array<{ id: string; layer: string; key: string; content: string; metadata: string; updated_at: string }> }>(
    `${BASE}/agents/${agentId}/memory`,
    options,
  );
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
  return fetchJson<{ memory: Array<{ id: string; layer: string; key: string; content: string; metadata: string; updated_at: string }> }>(
    `${BASE}/agents/${agentId}/memory/export`,
    options,
  );
}

export function getMemorySummary(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{ count: number }>(`${BASE}/agents/${agentId}/memory/summary`, options);
}

// --- Channels ---

export function listChannels(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<{ channels: Array<{ id: string; platform: string; config: string; status: string; task_id: string; created_at: string }> }>(
    `${BASE}/agents/${agentId}/channels`,
    options,
  );
}

export function setupChannel(
  agentId: string,
  data: { workspace_id: string; platform: string; config: string; status: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ channel: { id: string; platform: string; config: string; status: string; task_id: string; created_at: string } }>(
    `${BASE}/agents/${agentId}/channels`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify(data), ...options?.init },
    },
  );
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

export function exportConfigZipUrl(workspaceId: string) {
  return `${BASE}/workspaces/${workspaceId}/config/export/zip`;
}

export function previewImport(
  workspaceId: string,
  bundle: Record<string, unknown>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ preview: { agents: { created: string[]; updated: string[]; deleted: string[] }; skills: { created: string[]; updated: string[]; deleted: string[] }; routines: { created: string[]; updated: string[]; deleted: string[] }; projects: { created: string[]; updated: string[]; deleted: string[] } } }>(
    `${BASE}/workspaces/${workspaceId}/config/preview`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify(bundle), ...options?.init },
    },
  );
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

// --- Issues ---

export function listIssues(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ issues: OrchestrateIssue[] }>(
    `${BASE}/workspaces/${workspaceId}/issues`,
    options,
  );
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
