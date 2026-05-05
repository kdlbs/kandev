"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listBranches, listRepositoryBranches } from "@/lib/api";
import type { Branch } from "@/lib/types/http";

const EMPTY_BRANCHES: Branch[] = [];

/**
 * Source of branches for a row: either a workspace repo (by id) or an
 * on-machine folder (by path). Both routes go through one backend endpoint
 * (`/workspaces/:id/branches`) and share one Zustand cache slice — id-based
 * entries are keyed by the repo id, path-based entries get a synthetic key.
 *
 * `workspaceId` is always required because the route segment needs it.
 */
export type BranchSource =
  | { kind: "id"; workspaceId: string; repositoryId: string }
  | { kind: "path"; workspaceId: string; path: string };

function cacheKeyFor(source: BranchSource | null): string {
  if (!source) return "";
  return source.kind === "id" ? source.repositoryId : `path::${source.workspaceId}::${source.path}`;
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
  const key = cacheKeyFor(source);
  const branches = useAppStore((state) =>
    key ? (state.repositoryBranches.itemsByRepositoryId[key] ?? EMPTY_BRANCHES) : EMPTY_BRANCHES,
  );
  const isLoaded = useAppStore((state) =>
    key ? (state.repositoryBranches.loadedByRepositoryId[key] ?? false) : false,
  );
  const isLoading = useAppStore((state) =>
    key ? (state.repositoryBranches.loadingByRepositoryId[key] ?? false) : false,
  );
  const setRepositoryBranches = useAppStore((state) => state.setRepositoryBranches);
  const setRepositoryBranchesLoading = useAppStore((state) => state.setRepositoryBranchesLoading);
  const inFlightRef = useRef(false);

  useEffect(() => {
    if (!enabled || !source) return;
    if (isLoaded || inFlightRef.current) return;
    inFlightRef.current = true;
    setRepositoryBranchesLoading(key, true);

    const promise =
      source.kind === "id"
        ? listBranches(source.workspaceId, { repositoryId: source.repositoryId })
        : listBranches(source.workspaceId, { path: source.path });

    promise
      .then((response) => setRepositoryBranches(key, response.branches))
      .catch(() => setRepositoryBranches(key, []))
      .finally(() => {
        inFlightRef.current = false;
        setRepositoryBranchesLoading(key, false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- key encodes source identity; listing every field re-fires on every render
  }, [enabled, isLoaded, key, setRepositoryBranches, setRepositoryBranchesLoading]);

  const refresh = useCallback(async () => {
    if (!source) return;
    setRepositoryBranchesLoading(key, true);
    try {
      const response =
        source.kind === "id"
          ? await listRepositoryBranches(source.repositoryId, { refresh: true })
          : await listBranches(source.workspaceId, { path: source.path });
      setRepositoryBranches(key, response.branches);
    } catch {
      // Refresh failures leave the existing branch list in place; the user
      // can retry manually. Errors are surfaced via the BranchRefreshButton's
      // tooltip when wired with `fetchError`, but the hook does not own
      // error state today.
    } finally {
      setRepositoryBranchesLoading(key, false);
    }
  }, [source, key, setRepositoryBranches, setRepositoryBranchesLoading]);

  return {
    branches,
    isLoading,
    refresh: source ? refresh : undefined,
  };
}
