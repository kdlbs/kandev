"use client";

import { useEffect, useCallback } from "react";
import {
  listReviewWatches,
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
  type CreateReviewWatchRequest,
  type UpdateReviewWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";

/**
 * useGitLabReviewWatches — three modes:
 *   - workspaceId: string         → fetch watches scoped to one workspace
 *   - workspaceId: undefined      → fetch watches across all workspaces
 *   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
 */
export function useGitLabReviewWatches(workspaceId?: string | null) {
  const items = useAppStore((state) => state.gitlabReviewWatches.items);
  const loaded = useAppStore((state) => state.gitlabReviewWatches.loaded);
  const loading = useAppStore((state) => state.gitlabReviewWatches.loading);
  const set = useAppStore((state) => state.setGitLabReviewWatches);
  const setLoading = useAppStore((state) => state.setGitLabReviewWatchesLoading);
  const add = useAppStore((state) => state.addGitLabReviewWatch);
  const upd = useAppStore((state) => state.updateGitLabReviewWatchInStore);
  const rm = useAppStore((state) => state.removeGitLabReviewWatch);

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    setLoading(true);
    listReviewWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((response) => set(response?.watches ?? []))
      .catch(() => set([]))
      .finally(() => setLoading(false));
  }, [workspaceId, loaded, loading, set, setLoading]);

  const create = useCallback(
    async (req: CreateReviewWatchRequest) => {
      const watch = await createReviewWatch(req);
      add(watch);
      return watch;
    },
    [add],
  );

  const update = useCallback(
    async (id: string, req: UpdateReviewWatchRequest) => {
      const watch = await updateReviewWatch(id, req);
      upd(watch);
      return watch;
    },
    [upd],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteReviewWatch(id);
      rm(id);
    },
    [rm],
  );

  const trigger = useCallback((id: string) => triggerReviewWatch(id), []);
  const triggerAll = useCallback(() => triggerAllReviewWatches(), []);

  return { items, loaded, loading, create, update, remove, trigger, triggerAll };
}
