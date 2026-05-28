/**
 * queryOptions factories for the office domain.
 *
 * Design decisions:
 * - Filters are baked into the query key as params so TQ de-dupes and GCs
 *   distinct filter combinations independently (gcTime: 5m per plan).
 * - Sorting and grouping are never part of the key — they're applied via
 *   `select` in consumers so re-renders happen only on data change.
 * - Provider health uses refetchInterval: 90_000 to match the backend
 *   poller cadence (same as jira/linear health pollers).
 *
 * Import pattern:
 *   useQuery(officeQueryOptions.dashboard(wsId))
 *   useQuery({ ...officeQueryOptions.tasks(wsId, filters), select: sortAndGroup })
 */
import { queryOptions } from "@tanstack/react-query";
import {
  getDashboard,
  listAgentProfiles,
  listApprovals,
  listActivity,
  getCostSummary,
  listBudgets,
  listRoutines,
  getInbox,
  getMeta,
  listProjects,
} from "@/lib/api/domains/office-api";
import { listSkills } from "@/lib/api/domains/office-skills-api";
import { listTasks } from "@/lib/api/domains/office-tasks-api";
import { listRuns } from "@/lib/api/domains/office-runs-api";
import {
  getProviderHealth,
  getWorkspaceRouting,
  getRoutingPreview,
  getAgentRoute,
} from "@/lib/api/domains/office-routing-api";
import { qk } from "@/lib/query/keys";
import type { TaskFilterState } from "@/lib/state/slices/office/types";

// ---------------------------------------------------------------------------
// Filter → ListTasksParams conversion
// ---------------------------------------------------------------------------

function filtersToParams(filters: Partial<TaskFilterState> | undefined) {
  if (!filters) return undefined;
  return {
    status: filters.statuses?.length ? filters.statuses : undefined,
    priority: filters.priorities?.length ? filters.priorities : undefined,
    assignee: filters.assigneeIds?.[0],
    // Pass the first project filter to the server; client-side select handles
    // the rest if multiple projects are selected.
    project: filters.projectIds?.[0],
  };
}

// ---------------------------------------------------------------------------
// queryOptions namespace
// ---------------------------------------------------------------------------

export const officeQueryOptions = {
  /** Workspace dashboard — agent counts, open tasks, run activity, etc. */
  dashboard: (wsId: string) =>
    queryOptions({
      queryKey: qk.office.dashboard(wsId),
      queryFn: () => getDashboard(wsId),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /**
   * Office tasks. Filters are baked into the cache key so each distinct
   * filter combination gets its own TQ cache entry (gcTime: 5m from global
   * defaults). Pass filters as-needed; omit for the full list.
   *
   * Sorting / grouping are NOT in the key — apply them via `select` so
   * a re-sort never triggers a network fetch.
   */
  tasks: (wsId: string, filters?: Partial<TaskFilterState>) =>
    queryOptions({
      queryKey: qk.office.tasks(wsId, filters as Record<string, unknown> | undefined),
      queryFn: () => listTasks(wsId, filtersToParams(filters)).then((r) => r.tasks ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Agent profiles for a workspace. */
  agents: (wsId: string) =>
    queryOptions({
      queryKey: qk.office.agents(wsId),
      queryFn: () => listAgentProfiles(wsId).then((r) => r.agents ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /**
   * Provider health for a workspace.
   *
   * refetchInterval: 90_000 matches the backend health-poller cadence.
   * The WS bridge also pushes individual health changes in real-time so
   * the interval is a safety net, not the primary update mechanism.
   */
  providerHealth: (wsId: string) =>
    queryOptions({
      queryKey: qk.office.providerHealth(wsId),
      queryFn: () => getProviderHealth(wsId).then((r) => r.health ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
      refetchInterval: 90_000,
    }),

  /** Runs (queued + recent) for a workspace. */
  runs: (wsId: string) =>
    queryOptions({
      queryKey: qk.office.runs(wsId),
      queryFn: () => listRuns(wsId).then((r) => r.runs ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Pending + resolved approvals for a workspace. */
  approvals: (wsId: string) =>
    queryOptions({
      queryKey: qk.office.approvals(wsId),
      queryFn: () => listApprovals(wsId).then((r) => r.approvals ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Activity log for a workspace. */
  activity: (wsId: string, filterType?: string) =>
    queryOptions({
      queryKey: [...qk.office.activity(wsId), filterType ?? "all"] as const,
      queryFn: () => listActivity(wsId, filterType).then((r) => r.activity ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /**
   * Office meta (statuses, priorities, roles, etc.).
   *
   * The `getMeta` endpoint is global (no workspace scope). We scope the
   * key by wsId so workspace-change invalidation works cleanly, but also
   * expose a wsId-free overload for leaf components that don't have access
   * to the active workspace id.
   */
  meta: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "meta"] as const,
      queryFn: () => getMeta(),
      enabled: !!wsId,
      staleTime: 5 * 60_000,
    }),

  /** Global meta variant for leaf components without workspace context. */
  metaGlobal: () =>
    queryOptions({
      queryKey: ["office", "meta"] as const,
      queryFn: () => getMeta(),
      staleTime: 5 * 60_000,
    }),

  /** Cost summary for a workspace. */
  costs: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "costs"] as const,
      queryFn: () => getCostSummary(wsId),
      enabled: !!wsId,
      staleTime: 60_000,
    }),

  /** Budget policies for a workspace. */
  budgets: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "budgets"] as const,
      queryFn: () => listBudgets(wsId).then((r) => r.budgets ?? []),
      enabled: !!wsId,
      staleTime: 60_000,
    }),

  /** Routines for a workspace. */
  routines: (wsId: string) =>
    queryOptions({
      queryKey: qk.office.routines(wsId),
      queryFn: () => listRoutines(wsId).then((r) => r.routines ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Projects for a workspace. */
  projects: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "projects"] as const,
      queryFn: () => listProjects(wsId).then((r) => r.projects ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Skills for a workspace. */
  skills: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "skills"] as const,
      queryFn: () => listSkills(wsId).then((r) => r.skills ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Inbox items + count for a workspace. */
  inbox: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "inbox"] as const,
      queryFn: () => getInbox(wsId),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Workspace routing config + known providers. */
  workspaceRouting: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "routing"] as const,
      queryFn: () => getWorkspaceRouting(wsId),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Routing preview (effective provider per agent) for a workspace. */
  routingPreview: (wsId: string) =>
    queryOptions({
      queryKey: ["office", wsId, "routingPreview"] as const,
      queryFn: () => getRoutingPreview(wsId).then((r) => r.agents ?? []),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /**
   * Per-agent routing data (overrides + preview + last failure).
   * Maps to qk.office.agentRouting(agentId).
   */
  agentRouting: (agentId: string) =>
    queryOptions({
      queryKey: qk.office.agentRouting(agentId),
      queryFn: () => getAgentRoute(agentId),
      enabled: !!agentId,
      staleTime: 30_000,
    }),

  /**
   * Run attempts for a specific run.
   * Key not in qk taxonomy (run-scoped, not workspace-scoped).
   */
  runAttempts: (runId: string) =>
    queryOptions({
      queryKey: ["office", "runs", runId, "attempts"] as const,
      queryFn: async () => {
        const { getRunAttempts } = await import("@/lib/api/domains/office-runs-api");
        return getRunAttempts(runId).then((r) => r.attempts ?? []);
      },
      enabled: !!runId,
      staleTime: 30_000,
    }),
};
