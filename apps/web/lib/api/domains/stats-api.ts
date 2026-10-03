import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ModelUsageDTO,
  CompletedTaskActivityDTO,
  DailyActivityDTO,
  GitStatsDTO,
  GlobalStatsDTO,
  RepositoryStatsDTO,
  TaskStatsDTO,
  TokenUsageResponse,
} from "@/lib/types/http";

export type StatsRange = "week" | "month" | "all";
export type TokenUsageGroup = "model" | "day" | "month" | "task" | "session";
export type TokenUsageSortDirection = "asc" | "desc";
export type TokenUsageQuery = {
  provider?: string;
  timezone?: string;
  sortBy?: string;
  sortDirection?: TokenUsageSortDirection;
  limit?: number;
  offset?: number;
  includeUndated?: boolean;
};

export type TaskStatsResponse = {
  task_stats: TaskStatsDTO[];
  task_stats_has_more: boolean;
};

function rangeQuery(range?: StatsRange): string {
  return range ? `?range=${encodeURIComponent(range)}` : "";
}

function statsUrl(workspaceId: string, section: string, range?: StatsRange): string {
  return `/api/v1/workspaces/${workspaceId}/stats/${section}${rangeQuery(range)}`;
}

export function fetchGlobalStats(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<GlobalStatsDTO>(statsUrl(workspaceId, "global", range), options);
}

export function fetchTaskStats(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<TaskStatsResponse>(statsUrl(workspaceId, "tasks", range), options);
}

export function fetchDailyActivity(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<DailyActivityDTO[]>(statsUrl(workspaceId, "daily-activity", range), options);
}

export function fetchCompletedActivity(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<CompletedTaskActivityDTO[]>(
    statsUrl(workspaceId, "completed-activity", range),
    options,
  );
}

export function fetchModelUsage(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<ModelUsageDTO[]>(statsUrl(workspaceId, "model-usage", range), options);
}

function tokenUsageQueryString(
  range: StatsRange | undefined,
  group: TokenUsageGroup | undefined,
  queryOptions: TokenUsageQuery | undefined,
): string {
  const query = new URLSearchParams();
  const entries: Array<[string, string | undefined]> = [
    ["range", range],
    ["group", group],
    ["include_undated", String(queryOptions?.includeUndated ?? range === "all")],
    ["timezone", queryOptions?.timezone],
    ["provider", queryOptions?.provider],
    ["sort", queryOptions?.sortBy],
    ["direction", queryOptions?.sortDirection],
    ["limit", queryOptions?.limit === undefined ? undefined : String(queryOptions.limit)],
    ["offset", queryOptions?.offset === undefined ? undefined : String(queryOptions.offset)],
  ];
  for (const [key, value] of entries) {
    if (value !== undefined && value !== "") query.set(key, value);
  }
  const suffix = query.toString();
  return suffix ? `?${suffix}` : "";
}

export function fetchTokenUsage(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
  group?: TokenUsageGroup,
  queryOptions?: TokenUsageQuery,
) {
  return fetchJson<TokenUsageResponse>(
    `/api/v1/workspaces/${workspaceId}/stats/token-usage${tokenUsageQueryString(range, group, queryOptions)}`,
    options,
  );
}

export function fetchRepositoryStats(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<RepositoryStatsDTO[]>(statsUrl(workspaceId, "repositories", range), options);
}

export function fetchGitStats(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange,
) {
  return fetchJson<GitStatsDTO>(statsUrl(workspaceId, "git", range), options);
}
