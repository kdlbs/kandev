"use client";

import { useEffect, useCallback } from "react";
import {
  listReviewWatches,
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
  previewResetReviewWatch,
  resetReviewWatch,
  type CreateReviewWatchRequest,
  type UpdateReviewWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

const WORKSPACE_REQUIRED = "workspaceId required";

function filterByWorkspace<T extends { workspace_id: string }>(
  items: T[],
  workspaceId?: string | null,
) {
  return workspaceId ? items.filter((watch) => watch.workspace_id === workspaceId) : [];
}

/**
 * Fetches watches for one resolved workspace. Null pauses fetching while the
 * caller is resolving its workspace.
 *
 * The workspace dependency refetches when the caller switches workspaces.
 */
export function useGitLabReviewWatches(workspaceId: string | null) {
  const items = useAppStore((state) => state.gitlabReviewWatches.items);
  const loaded = useAppStore((state) => state.gitlabReviewWatches.loaded);
  const loading = useAppStore((state) => state.gitlabReviewWatches.loading);
  const set = useAppStore((state) => state.setGitLabReviewWatches);
  const setLoading = useAppStore((state) => state.setGitLabReviewWatchesLoading);
  const add = useAppStore((state) => state.addGitLabReviewWatch);
  const upd = useAppStore((state) => state.updateGitLabReviewWatchInStore);
  const rm = useAppStore((state) => state.removeGitLabReviewWatch);

  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setLoading(true);
    listReviewWatches(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (!cancelled) set(response?.watches ?? []);
      })
      .catch(() => {
        if (!cancelled) set([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, set, setLoading]);

  const create = useCallback(
    async (req: CreateReviewWatchRequest) => {
      const watch = await createReviewWatch(req);
      add(watch);
      return watch;
    },
    [add],
  );

  const update = useCallback(
    async (id: string, req: UpdateReviewWatchRequest, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      const watch = await updateReviewWatch(id, ws, req);
      upd(watch);
      return watch;
    },
    [upd, workspaceId],
  );

  const remove = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      await deleteReviewWatch(id, ws);
      rm(id);
    },
    [rm, workspaceId],
  );

  const trigger = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return triggerReviewWatch(id, ws);
    },
    [workspaceId],
  );
  const triggerAll = useCallback(() => {
    if (!workspaceId) throw new Error(WORKSPACE_REQUIRED);
    return triggerAllReviewWatches(workspaceId);
  }, [workspaceId]);
  const previewReset = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return previewResetReviewWatch(id, ws);
    },
    [workspaceId],
  );
  const reset = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return resetReviewWatch(id, ws);
    },
    [workspaceId],
  );

  return {
    items: filterByWorkspace(items, workspaceId),
    loaded,
    loading,
    create,
    update,
    remove,
    trigger,
    triggerAll,
    previewReset,
    reset,
  };
}
