import { queryOptions } from "@tanstack/react-query";
import {
  listBranches,
  listQuickChatSessions,
  listRepositories,
  listRepositoryBranches,
  listRepositoryScripts,
  listWorkspaces,
} from "@/lib/api/domains/workspace-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function workspacesQueryOptions() {
  return queryOptions({
    queryKey: qk.workspaces.all(),
    queryFn: async ({ signal }) => {
      const response = await listWorkspaces(withSignal(signal));
      return response.workspaces;
    },
  });
}

export function workspaceRepositoriesQueryOptions(
  workspaceId: string,
  params?: { includeScripts?: boolean },
) {
  return queryOptions({
    queryKey: qk.workspaces.repositories(workspaceId, params),
    queryFn: async ({ signal }) => {
      const response = await listRepositories(workspaceId, params, withSignal(signal));
      return response.repositories;
    },
    enabled: Boolean(workspaceId),
  });
}

export function workspaceBranchesQueryOptions(
  workspaceId: string,
  source: { repositoryId: string } | { path: string },
) {
  return queryOptions({
    queryKey: qk.workspaces.branches(workspaceId, source),
    queryFn: async ({ signal }) => {
      const response = await listBranches(workspaceId, source, withSignal(signal));
      return response.branches;
    },
    enabled: Boolean(workspaceId),
  });
}

export function repositoryBranchesQueryOptions(
  repositoryId: string,
  params?: { refresh?: boolean },
) {
  return queryOptions({
    queryKey: qk.workspaces.repositoryBranches(repositoryId),
    queryFn: async ({ signal }) => {
      const response = await listRepositoryBranches(repositoryId, params, withSignal(signal));
      return response.branches;
    },
    enabled: Boolean(repositoryId),
  });
}

export function repositoryScriptsQueryOptions(repositoryId: string) {
  return queryOptions({
    queryKey: qk.workspaces.repositoryScripts(repositoryId),
    queryFn: async ({ signal }) => {
      const response = await listRepositoryScripts(repositoryId, withSignal(signal));
      return response.scripts ?? [];
    },
    enabled: Boolean(repositoryId),
  });
}

export function quickChatSessionsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.workspaces.quickChatSessions(workspaceId),
    queryFn: ({ signal }) => listQuickChatSessions(workspaceId, withSignal(signal)),
    enabled: Boolean(workspaceId),
  });
}
