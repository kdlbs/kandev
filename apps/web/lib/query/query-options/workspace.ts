import { queryOptions } from "@tanstack/react-query";
import {
  listWorkspaces,
  listRepositories,
  listBranches,
  listRepositoryScripts,
} from "@/lib/api/domains/workspace-api";
import { qk } from "@/lib/query/keys";

/**
 * BranchSource mirrors the shape used by useBranches.
 * Both id-based and path-based sources are supported.
 */
export type BranchQuerySource =
  | { kind: "id"; workspaceId: string; repositoryId: string }
  | { kind: "path"; workspaceId: string; path: string };

/** Stable cache key for a BranchQuerySource (matches the old Zustand cacheKeyFor). */
export function branchCacheKey(source: BranchQuerySource): string {
  return source.kind === "id" ? source.repositoryId : `path::${source.workspaceId}::${source.path}`;
}

/**
 * queryOptions factories for the workspace domain.
 *
 * Import these in useQuery() calls and SSR prefetch:
 *   useQuery(workspaceQueryOptions.all())
 *   useQuery(workspaceQueryOptions.repos(wsId))
 *   useQuery(workspaceQueryOptions.branchesById(wsId, repoId))
 *   useQuery(workspaceQueryOptions.branchesByPath(wsId, path))
 *   useQuery(workspaceQueryOptions.scripts(repoId))
 */
export const workspaceQueryOptions = {
  /** List all workspaces the current user has access to. */
  all: () =>
    queryOptions({
      queryKey: qk.workspaces.all(),
      queryFn: () => listWorkspaces(),
      staleTime: 60_000,
    }),

  /** Single workspace by id (derived from the all() list via select). */
  one: (id: string) =>
    queryOptions({
      queryKey: qk.workspaces.one(id),
      queryFn: () => listWorkspaces(),
      select: (data) => data.workspaces.find((w) => w.id === id) ?? null,
      staleTime: 60_000,
    }),

  /** Repositories for a workspace. */
  repos: (wsId: string) =>
    queryOptions({
      queryKey: qk.workspaces.repos(wsId),
      queryFn: () => listRepositories(wsId),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /**
   * Branches for a workspace repo identified by repository id.
   * Uses the canonical qk.workspaces.branches() key.
   */
  branchesById: (wsId: string, repositoryId: string) =>
    queryOptions({
      queryKey: qk.workspaces.branches(wsId, repositoryId),
      queryFn: () => listBranches(wsId, { repositoryId }),
      enabled: !!(wsId && repositoryId),
      staleTime: 30_000,
    }),

  /**
   * Branches for an on-machine folder identified by path.
   * Uses a synthetic key under the workspaces namespace.
   */
  branchesByPath: (wsId: string, path: string) =>
    queryOptions({
      queryKey: ["workspaces", "branches", "path", wsId, path] as const,
      queryFn: () => listBranches(wsId, { path }),
      enabled: !!(wsId && path),
      staleTime: 30_000,
    }),

  /**
   * Convenience factory that dispatches to branchesById / branchesByPath.
   * Returns a queryOptions object; callers can pass it directly to useQuery.
   */
  branches: (source: BranchQuerySource | null) => {
    if (!source) {
      return queryOptions({
        queryKey: ["workspaces", "branches", "none"] as const,
        queryFn: (): Promise<import("@/lib/types/http").RepositoryBranchesResponse> =>
          Promise.resolve({ branches: [], total: 0 }),
        enabled: false,
        staleTime: 0,
      });
    }
    if (source.kind === "id") {
      return workspaceQueryOptions.branchesById(source.workspaceId, source.repositoryId);
    }
    return workspaceQueryOptions.branchesByPath(source.workspaceId, source.path);
  },

  /** Repository scripts for a given repository. */
  scripts: (repositoryId: string | null) =>
    queryOptions({
      queryKey: ["workspaces", "repos", repositoryId ?? "", "scripts"] as const,
      queryFn: () => listRepositoryScripts(repositoryId!),
      enabled: !!repositoryId,
      staleTime: 60_000,
    }),
};
