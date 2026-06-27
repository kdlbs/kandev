import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import {
  getCostsBreakdown,
  getCostSummary,
  getDashboard,
  getInbox,
  getMeta,
  getProject,
  getTask,
  listAllRoutineRuns,
  listActivity,
  listActivityForTarget,
  listAgentProfiles,
  listBudgets,
  listComments,
  listProjects,
  listRoutines,
  listRoutineTriggers,
  searchTasks,
} from "@/lib/api/domains/office-api";
import {
  getAgentRoute,
  getProviderHealth,
  getRoutingPreview,
  getWorkspaceRouting,
} from "@/lib/api/domains/office-routing-api";
import {
  getAgentSummary,
  getRunAttempts,
  getRunDetail,
  listAgentRuns,
  listRuns,
} from "@/lib/api/domains/office-runs-api";
import { listSkills } from "@/lib/api/domains/office-skills-api";
import { listTasks, type ListTasksParams } from "@/lib/api/domains/office-tasks-api";
import { qk, type OfficeTaskFilters } from "../keys";
import { withSignal } from "./utils";

const DEFAULT_OFFICE_TASK_LIMIT = 50;

function singleFilterValue(value: string | string[] | undefined): string | undefined {
  if (Array.isArray(value)) return value.length === 1 ? value[0] : undefined;
  return value;
}

function toOfficeTaskParams(filters: OfficeTaskFilters = {}): ListTasksParams {
  const params: ListTasksParams = {
    limit: filters.limit ?? DEFAULT_OFFICE_TASK_LIMIT,
  };
  if (filters.status?.length) params.status = filters.status;
  if (filters.priority?.length) params.priority = filters.priority;
  const assignee = singleFilterValue(filters.assignee);
  if (assignee) params.assignee = assignee;
  const project = singleFilterValue(filters.project);
  if (project) params.project = project;
  if (filters.includeSystem) params.include_system = true;
  if (filters.sort !== null) {
    params.sort = filters.sort ?? "updated_at";
  }
  if (filters.order !== null) {
    params.order = filters.order ?? "desc";
  }
  return params;
}

export function officeMetaQueryOptions() {
  return queryOptions({
    queryKey: qk.office.meta(),
    queryFn: ({ signal }) => getMeta(withSignal(signal)),
  });
}

export function officeDashboardQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.dashboard(workspaceId),
    queryFn: ({ signal }) => getDashboard(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeTasksInfiniteQueryOptions(
  workspaceId: string,
  filters: OfficeTaskFilters = {},
) {
  return infiniteQueryOptions({
    queryKey: qk.office.tasks(workspaceId, filters),
    initialPageParam: undefined as { cursor?: string; cursor_id?: string } | undefined,
    queryFn: ({ pageParam, signal }) =>
      listTasks(workspaceId, { ...toOfficeTaskParams(filters), ...pageParam }, withSignal(signal)),
    getNextPageParam: (lastPage) =>
      lastPage.next_cursor
        ? { cursor: lastPage.next_cursor, cursor_id: lastPage.next_id }
        : undefined,
    enabled: Boolean(workspaceId),
  });
}

export function officeTaskQueryOptions(workspaceId: string, taskId: string) {
  return queryOptions({
    queryKey: qk.office.task(workspaceId, taskId),
    queryFn: ({ signal }) => getTask(taskId, withSignal(signal)),
    enabled: Boolean(workspaceId && taskId),
  });
}

export function officeTaskCommentsQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.office.taskComments(taskId),
    queryFn: ({ signal }) => listComments(taskId, withSignal(signal)),
    enabled: Boolean(taskId),
  });
}

export function officeTaskActivityQueryOptions(workspaceId: string, taskId: string) {
  return queryOptions({
    queryKey: qk.office.taskActivity(workspaceId, taskId),
    queryFn: ({ signal }) => listActivityForTarget(workspaceId, taskId, withSignal(signal)),
    enabled: Boolean(workspaceId && taskId),
  });
}

export function officeTaskSearchQueryOptions(workspaceId: string, query: string, limit = 50) {
  const normalizedQuery = query.trim();
  return queryOptions({
    queryKey: qk.office.taskSearch(workspaceId, normalizedQuery, limit),
    queryFn: ({ signal }) => searchTasks(workspaceId, normalizedQuery, limit, withSignal(signal)),
    enabled: Boolean(workspaceId && normalizedQuery),
  });
}

export function officeAgentsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.agents(workspaceId),
    queryFn: ({ signal }) => listAgentProfiles(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeProjectsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.projects(workspaceId),
    queryFn: ({ signal }) => listProjects(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeProjectQueryOptions(projectId: string) {
  return queryOptions({
    queryKey: qk.office.project(projectId),
    queryFn: ({ signal }) => getProject(projectId, withSignal(signal)),
    enabled: Boolean(projectId),
  });
}

export function officeInboxQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.inbox(workspaceId),
    queryFn: ({ signal }) => getInbox(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeActivityQueryOptions(workspaceId: string, filterType = "all") {
  return queryOptions({
    queryKey: qk.office.activity(workspaceId, filterType),
    queryFn: ({ signal }) => listActivity(workspaceId, filterType, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeRunsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.runs(workspaceId),
    queryFn: ({ signal }) => listRuns(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeRoutingQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.routing(workspaceId),
    queryFn: ({ signal }) => getWorkspaceRouting(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeProviderHealthQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.providerHealth(workspaceId),
    queryFn: ({ signal }) => getProviderHealth(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeRoutingPreviewQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.routingPreview(workspaceId),
    queryFn: ({ signal }) => getRoutingPreview(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeAgentRouteQueryOptions(agentId: string) {
  return queryOptions({
    queryKey: qk.office.agentRoute(agentId),
    queryFn: ({ signal }) => getAgentRoute(agentId, withSignal(signal)),
    enabled: Boolean(agentId),
  });
}

export function officeAgentSummaryQueryOptions(agentId: string, days?: number) {
  return queryOptions({
    queryKey: qk.office.agentSummary(agentId, days),
    queryFn: ({ signal }) => getAgentSummary(agentId, days, withSignal(signal)),
    enabled: Boolean(agentId),
  });
}

export function officeAgentRunsInfiniteQueryOptions(agentId: string, params?: { limit?: number }) {
  return infiniteQueryOptions({
    queryKey: qk.office.agentRuns(agentId, params),
    initialPageParam: undefined as { cursor?: string; cursorId?: string } | undefined,
    queryFn: ({ pageParam, signal }) =>
      listAgentRuns(agentId, { limit: params?.limit ?? 25, ...pageParam }, withSignal(signal)),
    getNextPageParam: (lastPage) =>
      lastPage.next_cursor
        ? { cursor: lastPage.next_cursor, cursorId: lastPage.next_id }
        : undefined,
    enabled: Boolean(agentId),
  });
}

export function officeRunDetailQueryOptions(agentId: string, runId: string) {
  return queryOptions({
    queryKey: qk.office.runDetail(agentId, runId),
    queryFn: ({ signal }) => getRunDetail(agentId, runId, withSignal(signal)),
    enabled: Boolean(agentId && runId),
  });
}

export function officeRunAttemptsQueryOptions(runId: string) {
  return queryOptions({
    queryKey: qk.office.runAttempts(runId),
    queryFn: ({ signal }) => getRunAttempts(runId, withSignal(signal)),
    enabled: Boolean(runId),
  });
}

export function officeCostsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.costs(workspaceId),
    queryFn: ({ signal }) => getCostSummary(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeCostBreakdownQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.costBreakdown(workspaceId),
    queryFn: ({ signal }) => getCostsBreakdown(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeBudgetsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.budgets(workspaceId),
    queryFn: ({ signal }) => listBudgets(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeRoutinesQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.routines(workspaceId),
    queryFn: ({ signal }) => listRoutines(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeRoutineRunsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.routineRuns(workspaceId),
    queryFn: ({ signal }) => listAllRoutineRuns(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function officeRoutineTriggersQueryOptions(routineId: string) {
  return queryOptions({
    queryKey: qk.office.routineTriggers(routineId),
    queryFn: ({ signal }) => listRoutineTriggers(routineId, withSignal(signal)),
    enabled: Boolean(routineId),
  });
}

export function officeSkillsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.office.skills(workspaceId),
    queryFn: ({ signal }) => listSkills(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}
