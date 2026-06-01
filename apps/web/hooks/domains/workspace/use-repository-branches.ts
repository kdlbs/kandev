"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Branch, RepositoryBranchesResponse } from "@/lib/types/http";
import { listBranches, listRepositoryBranches } from "@/lib/api/domains/workspace-api";
import { workspaceQueryOptions } from "@/lib/query/query-options/workspace";

/**
 * Source of branches for a row: either a workspace repo (by id) or an
 * on-machine folder (by path). Both routes go through one backend endpoint
 * (`/workspaces/:id/branches`) and share one TQ cache — id-based entries
 * are keyed by qk.workspaces.branches(), path-based entries use a synthetic key.
 *
 * `workspaceId` is always required because the route segment needs it.
 */
export type BranchSource =
  | { kind: "id"; workspaceId: string; repositoryId: string }
  | { kind: "path"; workspaceId: string; path: string };

/**
 * Loads git branches for a workspace repo or an on-machine path. One hook,
 * one cache, one backend endpoint — the source shape decides which query
 * param goes on the wire and which key the cache uses.
 */
export type UseBranchesResult = {
  branches: Branch[];
  isLoading: boolean;
  /**
   * Refreshes the branch list. For id-based sources the backend runs
   * `git fetch` first (force-refresh). For path-based sources we re-issue
   * the standard list call — there's no fetch endpoint for unimported
   * folders, but re-reading still surfaces newly created local branches.
   */
  refresh?: () => Promise<void>;
};

const EMPTY_BRANCHES: Branch[] = [];

/** Extracts the arguments for useBranchesById given the current source. */
function idArgs(
  source: BranchSource | null,
  enabled: boolean,
): [wsId: string, repoId: string, on: boolean] {
  if (source?.kind !== "id") return ["", "", false];
  return [source.workspaceId, source.repositoryId, enabled];
}

/** Extracts the arguments for useBranchesByPath given the current source. */
function pathArgs(
  source: BranchSource | null,
  enabled: boolean,
): [wsId: string, path: string, on: boolean] {
  if (source?.kind !== "path") return ["", "", false];
  return [source.workspaceId, source.path, enabled];
}

/** Makes a refresh function that force-fetches branches and writes into TQ cache. */
function makeRefresh(
  source: BranchSource,
  queryClient: ReturnType<typeof useQueryClient>,
  cacheKey: ReturnType<typeof workspaceQueryOptions.branches>["queryKey"],
): () => Promise<void> {
  return async () => {
    let result: RepositoryBranchesResponse;
    if (source.kind === "id") {
      // Triggers git fetch on the backend before returning branches.
      result = await listRepositoryBranches(source.repositoryId, { refresh: true });
    } else {
      // Re-reads local branches without a fetch (no git fetch for unimported folders).
      result = await listBranches(source.workspaceId, { path: source.path });
    }
    // Write back into the canonical cache key so consumers re-render immediately.
    queryClient.setQueryData(cacheKey, result);
  };
}

type QueryResult = { data?: { branches: Branch[] } | undefined; isLoading: boolean };

/** Returns the active query result based on the source kind. */
function pickActive(
  source: BranchSource | null,
  byId: QueryResult,
  byPath: QueryResult,
): QueryResult | null {
  if (source?.kind === "id") return byId;
  if (source?.kind === "path") return byPath;
  return null;
}

function useBranchesById(wsId: string, repoId: string, on: boolean) {
  return useQuery({ ...workspaceQueryOptions.branchesById(wsId, repoId), enabled: on });
}

function useBranchesByPath(wsId: string, path: string, on: boolean) {
  return useQuery({ ...workspaceQueryOptions.branchesByPath(wsId, path), enabled: on });
}

/**
 * Returns git branches for the given source (workspace repo or local path).
 *
 * The old inFlightRef dedup pattern is deleted — TanStack Query handles
 * request deduplication natively. The staleTime controls when a background
 * refetch fires.
 *
 * Signature-compatible with the old Zustand-backed hook.
 */
export function useBranches(source: BranchSource | null, enabled = true): UseBranchesResult {
  const queryClient = useQueryClient();

  // Both useQuery calls are always executed (stable hook call order).
  // Only the relevant one is "enabled"; the other stays inert.
  const byId = useBranchesById(...idArgs(source, enabled));
  const byPath = useBranchesByPath(...pathArgs(source, enabled));

  const active = pickActive(source, byId, byPath);
  const opts = workspaceQueryOptions.branches(source);
  const refresh = source ? makeRefresh(source, queryClient, opts.queryKey) : undefined;

  return {
    branches: active?.data?.branches ?? EMPTY_BRANCHES,
    isLoading: active?.isLoading ?? false,
    refresh,
  };
}
