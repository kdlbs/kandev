import { fetchJson, fetchJsonWithRetry, type ApiRequestOptions } from "../client";
import { normalizeActivityEntry, type RawActivityEntry } from "./office-activity-normalize";
import type {
  AgentProfile,
  Project,
  CostSummary,
  BudgetPolicy,
  Routine,
  RoutineTrigger,
  RoutineRun,
  Approval,
  InboxItem,
  OfficeMeta,
  ProviderUsage,
  LiveRun,
} from "@/lib/state/slices/office/types";
import type { CLIFlag, AgentRole, AgentStatus } from "@/lib/types/agent-profile";

// Re-export extended API so existing imports continue to work.
export {
  listChannels,
  setupChannel,
  deleteChannel,
  exportConfig,
  exportConfigZipUrl,
  previewImport,
  applyImport,
  getIncomingDiff,
  getOutgoingDiff,
  applyIncomingSync,
  applyOutgoingSync,
  listTasks,
  getTask,
  listComments,
  createComment,
  searchTasks,
  updateTask,
  addTaskBlocker,
  removeTaskBlocker,
  listTaskReviewers,
  addTaskReviewer,
  removeTaskReviewer,
  listTaskApprovers,
  addTaskApprover,
  removeTaskApprover,
  approveTask,
  requestTaskChanges,
  listTaskDecisions,
  listInstructions,
  getInstruction,
  upsertInstruction,
  deleteInstruction,
  getOnboardingState,
  completeOnboarding,
  importFromFS,
  getDashboard,
  listRuns,
  updateWorkspaceSettings,
  getWorkspaceSettings,
  getGitStatus,
  gitClone,
  gitPull,
  gitPush,
} from "./office-extended-api";
export type {
  ImportDiff,
  ImportPreview,
  ParseError,
  SyncDiff,
  TaskCommentResponse,
  TimelineEvent,
  UpdateTaskPayload,
  TaskParticipant,
  OnboardingStateData,
  OnboardingCompletePayload,
  OnboardingCompleteResult,
  OnboardingFSWorkspace,
  ImportFromFSResult,
  GitStatusData,
  WorkspaceSettingsData,
  TaskDecisionDTO,
} from "./office-extended-api";

const BASE = "/api/v1/office";

export const getMeta = (options?: ApiRequestOptions) =>
  fetchJsonWithRetry<OfficeMeta>(`${BASE}/meta`, options);

type AgentResponse = { agent: unknown };
type AgentListResponse = { agents: unknown[] };

export type WorkspaceDeletionSummary = {
  workspace_name: string;
  tasks: number;
  agents: number;
  skills: number;
  config_path: string;
};

function parseJSONField<T>(value: unknown, fallback: T): T {
  if (typeof value !== "string") return (value as T) ?? fallback;
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

function rawField(agent: Record<string, unknown>, camelKey: string, snakeKey: string) {
  return agent[camelKey] ?? agent[snakeKey];
}

function stringField(agent: Record<string, unknown>, camelKey: string, snakeKey: string) {
  const value = rawField(agent, camelKey, snakeKey);
  return typeof value === "string" ? value : "";
}

function numberField(
  agent: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
  fallback: number,
) {
  const value = rawField(agent, camelKey, snakeKey);
  return typeof value === "number" ? value : fallback;
}

function boolField(agent: Record<string, unknown>, camelKey: string, snakeKey: string): boolean {
  const value = rawField(agent, camelKey, snakeKey);
  return typeof value === "boolean" ? value : false;
}

function normalizeAgent(raw: unknown): AgentProfile {
  const agent = raw as Record<string, unknown>;
  const id = agent.id as string;
  return {
    id,
    workspaceId: stringField(agent, "workspaceId", "workspace_id"),
    name: agent.name as string,
    agentProfileId: stringField(agent, "agentProfileId", "agent_profile_id") || id,
    role: agent.role as AgentRole,
    icon: agent.icon as string | undefined,
    status: agent.status as AgentStatus,
    reportsTo: stringField(agent, "reportsTo", "reports_to"),
    permissions: parseJSONField(agent.permissions, {}),
    budgetMonthlyCents: numberField(agent, "budgetMonthlyCents", "budget_monthly_cents", 0),
    maxConcurrentSessions: numberField(
      agent,
      "maxConcurrentSessions",
      "max_concurrent_sessions",
      1,
    ),
    desiredSkills: parseJSONField<string[]>(rawField(agent, "desiredSkills", "desired_skills"), []),
    executorPreference: parseJSONField<Record<string, unknown>>(
      rawField(agent, "executorPreference", "executor_preference"),
      {},
    ),
    pauseReason: stringField(agent, "pauseReason", "pause_reason"),
    billingType: rawField(agent, "billingType", "billing_type") as AgentProfile["billingType"],
    utilization: (agent.utilization ?? null) as AgentProfile["utilization"],
    skillIds: parseJSONField<string[]>(rawField(agent, "skillIds", "skill_ids"), []),
    // CLI subprocess fields. Office-served rows may omit these when the
    // office agent is not yet wired to a CLI client; default to safe
    // empty values so the canonical type stays satisfied.
    agentId: stringField(agent, "agentId", "agent_id"),
    agentDisplayName: stringField(agent, "agentDisplayName", "agent_display_name"),
    model: stringField(agent, "model", "model"),
    mode: (rawField(agent, "mode", "mode") as string | undefined) ?? undefined,
    allowIndexing: boolField(agent, "allowIndexing", "allow_indexing"),
    cliFlags: parseJSONField<CLIFlag[]>(rawField(agent, "cliFlags", "cli_flags"), []),
    cliPassthrough: boolField(agent, "cliPassthrough", "cli_passthrough"),
    userModified: rawField(agent, "userModified", "user_modified") as boolean | undefined,
    createdAt: stringField(agent, "createdAt", "created_at"),
    updatedAt: stringField(agent, "updatedAt", "updated_at"),
  };
}

function stringifyJSONField(value: unknown): string | undefined {
  if (value === undefined) return undefined;
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}

function agentPayload(data: Partial<AgentProfile>): Record<string, unknown> {
  return {
    name: data.name,
    agent_profile_id: data.agentProfileId,
    role: data.role,
    icon: data.icon,
    reports_to: data.reportsTo,
    permissions: stringifyJSONField(data.permissions),
    budget_monthly_cents: data.budgetMonthlyCents,
    max_concurrent_sessions: data.maxConcurrentSessions,
    desired_skills: stringifyJSONField(data.desiredSkills),
    executor_preference: stringifyJSONField(data.executorPreference),
    skill_ids: stringifyJSONField(data.skillIds),
  };
}

// --- Agent Instances ---

export function listAgentProfiles(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJsonWithRetry<AgentListResponse>(
    `${BASE}/workspaces/${workspaceId}/agents`,
    options,
  ).then((res) => ({ agents: (res.agents ?? []).map((agent) => normalizeAgent(agent)) }));
}

export function createAgentProfile(
  workspaceId: string,
  data: Partial<AgentProfile>,
  options?: ApiRequestOptions,
) {
  return fetchJson<AgentResponse>(`${BASE}/workspaces/${workspaceId}/agents`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(agentPayload(data)), ...options?.init },
  }).then((res) => normalizeAgent(res.agent));
}

export const getAgentProfile = (id: string, options?: ApiRequestOptions) =>
  fetchJson<AgentResponse>(`${BASE}/agents/${id}`, options).then((res) =>
    normalizeAgent(res.agent),
  );

export const getAgentUtilization = (agentId: string, options?: ApiRequestOptions) =>
  fetchJson<{ utilization: ProviderUsage | null }>(
    `${BASE}/agents/${agentId}/utilization`,
    options,
  );

export function updateAgentProfile(
  id: string,
  data: Partial<AgentProfile>,
  options?: ApiRequestOptions,
) {
  return fetchJson<AgentResponse>(`${BASE}/agents/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(agentPayload(data)), ...options?.init },
  }).then((res) => normalizeAgent(res.agent));
}

export function deleteAgentProfile(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/agents/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export function getWorkspaceDeletionSummary(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<WorkspaceDeletionSummary>(
    `${BASE}/workspaces/${workspaceId}/deletion-summary`,
    options,
  );
}

export function deleteWorkspace(
  workspaceId: string,
  confirmName: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<void>(`${BASE}/workspaces/${workspaceId}`, {
    ...options,
    init: {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ confirm_name: confirmName }),
      ...options?.init,
    },
  });
}

// --- Skills ---
// Moved to office-skills-api.ts so this module stays under the
// 600-line cap. The re-exports below keep every existing
// `from "@/lib/api/domains/office-api"` import working.
export {
  listSkills,
  createSkill,
  getSkill,
  updateSkill,
  deleteSkill,
  importSkill,
  getSkillFile,
} from "./office-skills-api";

// --- Projects ---

export function listProjects(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJsonWithRetry<{ projects: Project[] }>(
    `${BASE}/workspaces/${workspaceId}/projects`,
    options,
  );
}

// Backend single-project endpoints wrap the row as `{ project: ... }`.
// The list endpoint wraps as `{ projects: [...] }`. These helpers
// unwrap once at the API boundary so callers always see a `Project`.

export async function createProject(
  workspaceId: string,
  data: Partial<Project>,
  options?: ApiRequestOptions,
) {
  const res = await fetchJson<{ project: Project }>(`${BASE}/workspaces/${workspaceId}/projects`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(data), ...options?.init },
  });
  return res.project;
}

export async function getProject(id: string, options?: ApiRequestOptions) {
  const res = await fetchJson<{ project: Project }>(`${BASE}/projects/${id}`, options);
  return res.project;
}

export async function updateProject(
  id: string,
  data: Partial<Project>,
  options?: ApiRequestOptions,
) {
  const res = await fetchJson<{ project: Project }>(`${BASE}/projects/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(data), ...options?.init },
  });
  return res.project;
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
  return fetchJson<{ total_subcents: number }>(
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

/**
 * Composed costs view: total + by-agent / by-project / by-model / by-provider
 * in a single round-trip (Stream D of office optimization). The five
 * aggregations run concurrently on the backend with a snapshot-consistent
 * read, so the page reflects one moment in time.
 */
export function getCostsBreakdown(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    total_subcents: number;
    by_agent: CostBreakdownItemRaw[];
    by_project: CostBreakdownItemRaw[];
    by_model: CostBreakdownItemRaw[];
    by_provider: CostBreakdownItemRaw[];
  }>(`${BASE}/workspaces/${workspaceId}/costs/breakdown`, options);
}

type CostBreakdownItemRaw = {
  group_key: string;
  group_label?: string;
  count: number;
  total_subcents: number;
};

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
  return fetchJson<{ activity: RawActivityEntry[] }>(
    `${BASE}/workspaces/${workspaceId}/activity${query}`,
    options,
  ).then((res) => ({ activity: (res.activity ?? []).map(normalizeActivityEntry) }));
}

export function listActivityForTarget(
  workspaceId: string,
  targetId: string,
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams({ target_id: targetId });
  return fetchJson<{ activity: RawActivityEntry[] }>(
    `${BASE}/workspaces/${workspaceId}/activity?${params.toString()}`,
    options,
  ).then((res) => ({ activity: (res.activity ?? []).map(normalizeActivityEntry) }));
}

// --- Inbox ---

export function getInbox(workspaceId: string, options?: ApiRequestOptions) {
  // Returns items + total_count in a single call (Stream F of office
  // optimization). The sidebar count subscribes to the store value the
  // inbox page sets; no separate count fetch is required.
  return fetchJsonWithRetry<{ items: InboxItem[]; total_count: number }>(
    `${BASE}/workspaces/${workspaceId}/inbox`,
    options,
  );
}

/**
 * @deprecated Use getInbox instead — it now returns total_count alongside
 * items, removing the need for a second round-trip.
 */
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

// --- Live Runs ---

export function getLiveRuns(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ runs: LiveRun[] }>(`${BASE}/workspaces/${workspaceId}/live-runs`, options);
}

// --- Agent summaries ---
//
// Per-agent dashboard cards. One entry per workspace agent, with up to five
// most-recent sessions enriched with task identifier/title and command count.
// Backend: GET /api/v1/office/workspaces/:wsId/agent-summaries.

export type SessionSummary = {
  session_id: string;
  task_id: string;
  task_identifier: string;
  task_title: string;
  state: string;
  started_at: string;
  completed_at?: string | null;
  /**
   * Backend-computed elapsed seconds. Stable across refetches: for
   * non-RUNNING sessions it uses completed_at (or updated_at for office
   * IDLE rows that never complete) so the value doesn't drift.
   */
  duration_seconds: number;
  command_count: number;
};

export type AgentSummary = {
  agent_id: string;
  agent_name: string;
  agent_role: string;
  status: "live" | "finished" | "never_run";
  live_session?: SessionSummary | null;
  last_session?: SessionSummary | null;
  recent_sessions: SessionSummary[];
  /** Most recent run status — "ok" | "failed". */
  last_run_status?: "ok" | "failed";
  /** Set when the agent is currently auto-paused after threshold. */
  pause_reason?: string;
  /** Counter for the auto-pause UX. */
  consecutive_failures?: number;
};

export function getAgentSummaries(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ agents: AgentSummary[] }>(
    `${BASE}/workspaces/${workspaceId}/agent-summaries`,
    options,
  );
}
