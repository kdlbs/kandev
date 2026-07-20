import { queryOptions } from "@tanstack/react-query";
import {
  fetchGitLabStats,
  fetchGitLabStatus,
  getActionPresets,
  listIssueWatches,
  listReviewWatches,
  listWorkspaceTaskMRs,
} from "@/lib/api/domains/gitlab-api";
import type { TaskMR } from "@/lib/types/gitlab";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function gitlabStatusQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.gitlab.status(),
    queryFn: ({ signal }) => fetchGitLabStatus(withSignal(signal)),
  });
}

export function gitlabStatsQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.gitlab.stats(),
    queryFn: () => fetchGitLabStats(),
  });
}

export function workspaceTaskMrsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.integrations.gitlab.mrs(workspaceId),
    queryFn: ({ signal }) => listWorkspaceTaskMRs(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function taskMrsQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.integrations.gitlab.taskMr(taskId),
    queryFn: async (): Promise<TaskMR[]> => [],
    enabled: false,
  });
}

export function gitlabReviewWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.gitlab.reviewWatches(workspaceId),
    queryFn: async ({ signal }) => {
      const response = await listReviewWatches(workspaceId ?? undefined, withSignal(signal));
      return response?.watches ?? [];
    },
    enabled: workspaceId !== null,
  });
}

export function gitlabIssueWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.gitlab.issueWatches(workspaceId),
    queryFn: async ({ signal }) => {
      const response = await listIssueWatches(workspaceId ?? undefined, withSignal(signal));
      return response?.watches ?? [];
    },
    enabled: workspaceId !== null,
  });
}

export function gitlabActionPresetsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.integrations.gitlab.actionPresets(workspaceId),
    queryFn: () => getActionPresets(workspaceId),
    enabled: Boolean(workspaceId),
  });
}
