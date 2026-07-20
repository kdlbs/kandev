import { queryOptions } from "@tanstack/react-query";
import {
  fetchGitHubActionPresets,
  fetchGitHubStatus,
  getTaskCIAutomationOptions,
  listIssueWatches,
  listPRWatches,
  listWorkspaceTaskIssues,
  listReviewWatches,
  listWorkspaceTaskPRs,
} from "@/lib/api/domains/github-api";
import type { TaskIssueLink, TaskPR } from "@/lib/types/github";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function githubStatusQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.github.status(),
    queryFn: ({ signal }) => fetchGitHubStatus(withSignal(signal)),
  });
}

export function workspaceTaskPrsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.integrations.github.prs(workspaceId),
    queryFn: ({ signal }) => listWorkspaceTaskPRs(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}

export function workspaceTaskIssuesQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.integrations.github.issues(workspaceId),
    queryFn: async ({ signal }): Promise<Record<string, TaskIssueLink>> => {
      const response = await listWorkspaceTaskIssues(workspaceId, withSignal(signal));
      return response?.task_issues ?? {};
    },
    enabled: Boolean(workspaceId),
  });
}

export function taskPrsQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.integrations.github.taskPr(taskId),
    queryFn: async (): Promise<TaskPR[]> => [],
    enabled: false,
  });
}

export function taskCiOptionsQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.integrations.github.taskCiOptions(taskId),
    queryFn: ({ signal }) => getTaskCIAutomationOptions(taskId, withSignal(signal)),
    enabled: Boolean(taskId),
  });
}

export function prWatchesQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.github.prWatches(),
    queryFn: async ({ signal }) => {
      const response = await listPRWatches(withSignal(signal));
      return response?.watches ?? [];
    },
  });
}

export function reviewWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.github.reviewWatches(workspaceId),
    queryFn: async ({ signal }) => {
      const response = await listReviewWatches(workspaceId ?? undefined, withSignal(signal));
      return response?.watches ?? [];
    },
    enabled: workspaceId !== null,
  });
}

export function issueWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.github.issueWatches(workspaceId),
    queryFn: async ({ signal }) => {
      const response = await listIssueWatches(workspaceId ?? undefined, withSignal(signal));
      return response?.watches ?? [];
    },
    enabled: workspaceId !== null,
  });
}

export function githubActionPresetsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.integrations.github.actionPresets(workspaceId),
    queryFn: ({ signal }) => fetchGitHubActionPresets(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}
