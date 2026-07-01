"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import {
  repositoryBranchesQueryOptions,
  workspaceBranchesQueryOptions,
} from "@/lib/query/query-options";
import { qk } from "@/lib/query/keys";
import type { Branch } from "@/lib/types/http";

const EMPTY_BRANCHES: Branch[] = [];

/**
 * Source of branches for a row: either a workspace repo (by id) or an
 * on-machine folder (by path). Both routes go through Query option factories:
 * row branch lists use `/workspaces/:id/branches`, while explicit refresh for
 * imported repository rows uses `/repositories/:id/branches?refresh=true`.
 *
 * `workspaceId` is always required because the route segment needs it.
 */
export type BranchSource =
  | { kind: "id"; workspaceId: string; repositoryId: string }
  | { kind: "path"; workspaceId: string; path: string };

async function fetchFreshBranches(
  source: BranchSource,
  queryClient: ReturnType<typeof useQueryClient>,
  pathQuery: Pick<UseQueryResult<Branch[]>, "refetch">,
): Promise<Branch[]> {
  if (source.kind === "id") {
    const branches = await queryClient.fetchQuery({
      ...repositoryBranchesQueryOptions(source.repositoryId, { refresh: true }),
      staleTime: 0,
    });
    queryClient.setQueryData(
      qk.workspaces.branches(source.workspaceId, { repositoryId: source.repositoryId }),
      branches,
    );
    return branches;
  }
  return (await pathQuery.refetch()).data ?? EMPTY_BRANCHES;
}

function idBranchQueryOptions(source: BranchSource | null) {
  if (source?.kind !== "id") return workspaceBranchesQueryOptions("", { repositoryId: "" });
  return workspaceBranchesQueryOptions(source.workspaceId, { repositoryId: source.repositoryId });
}

function pathBranchQueryOptions(source: BranchSource | null) {
  if (source?.kind !== "path") return workspaceBranchesQueryOptions("", { path: "" });
  return workspaceBranchesQueryOptions(source.workspaceId, { path: source.path });
}

function useBranchQueries(source: BranchSource | null, enabled: boolean) {
  const idQuery = useQuery({
    ...idBranchQueryOptions(source),
    enabled: enabled && source?.kind === "id",
  });
  const pathQuery = useQuery({
    ...pathBranchQueryOptions(source),
    enabled: enabled && source?.kind === "path",
  });
  const activeQuery = source?.kind === "id" ? idQuery : pathQuery;
  return { activeQuery, pathQuery };
}

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

export function useBranches(source: BranchSource | null, enabled = true): UseBranchesResult {
  const queryClient = useQueryClient();
  const { activeQuery, pathQuery } = useBranchQueries(source, enabled);
  const branches = activeQuery.data ?? EMPTY_BRANCHES;

  const refresh = useCallback(async () => {
    if (!source) return;
    try {
      await fetchFreshBranches(source, queryClient, pathQuery);
    } catch {
      // Refresh failures leave the existing branch list in place; the user
      // can retry manually. Errors are surfaced via the BranchRefreshButton's
      // tooltip when wired with `fetchError`, but the hook does not own
      // error state today.
    }
  }, [pathQuery, queryClient, source]);

  return {
    branches,
    isLoading: activeQuery.isFetching && branches.length === 0,
    refresh: source ? refresh : undefined,
  };
}
