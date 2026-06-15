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
} from "@/lib/api/domains/github-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { CreateReviewWatchRequest, UpdateReviewWatchRequest } from "@/lib/types/github";

// useReviewWatches has three modes:
//   - workspaceId: string         → fetch watches scoped to one workspace
//   - workspaceId: undefined      → fetch watches across all workspaces
//   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
export function useReviewWatches(workspaceId?: string | null) {
  const items = useAppStore((state) => state.reviewWatches.items);
  const loaded = useAppStore((state) => state.reviewWatches.loaded);
  const loading = useAppStore((state) => state.reviewWatches.loading);
  const setReviewWatches = useAppStore((state) => state.setReviewWatches);
  const setReviewWatchesLoading = useAppStore((state) => state.setReviewWatchesLoading);
  const addWatch = useAppStore((state) => state.addReviewWatch);
  const updateWatch = useAppStore((state) => state.updateReviewWatch);
  const removeWatch = useAppStore((state) => state.removeReviewWatch);
  // storeApi exposes getState() without subscribing — used in reset() to
  // read the current watch row outside of the React render cycle so the
  // callback doesn't need `items` as a dependency.
  const storeApi = useAppStoreApi();

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    setReviewWatchesLoading(true);
    listReviewWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((response) => {
        setReviewWatches(response?.watches ?? []);
      })
      .catch(() => {
        setReviewWatches([]);
      })
      .finally(() => {
        setReviewWatchesLoading(false);
      });
  }, [workspaceId, loaded, loading, setReviewWatches, setReviewWatchesLoading]);

  const create = useCallback(
    async (req: CreateReviewWatchRequest) => {
      const watch = await createReviewWatch(req);
      addWatch(watch);
      return watch;
    },
    [addWatch],
  );

  const update = useCallback(
    async (id: string, req: UpdateReviewWatchRequest) => {
      const watch = await updateReviewWatch(id, req);
      updateWatch(watch);
      return watch;
    },
    [updateWatch],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteReviewWatch(id);
      removeWatch(id);
    },
    [removeWatch],
  );

  const trigger = useCallback(async (id: string) => {
    return triggerReviewWatch(id);
  }, []);

  const triggerAll = useCallback(async () => {
    if (!workspaceId) return null;
    return triggerAllReviewWatches(workspaceId);
  }, [workspaceId]);

  const previewReset = useCallback(async (id: string, watchWorkspaceId: string) => {
    return previewResetReviewWatch(id, watchWorkspaceId);
  }, []);

  const reset = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      const res = await resetReviewWatch(id, watchWorkspaceId);
      // Patch the cached watch so the "Last polled" column reflects the
      // reset immediately without waiting for the next poll tick.
      const current = storeApi.getState().reviewWatches.items.find((w) => w.id === id);
      if (current) updateWatch({ ...current, last_polled_at: null });
      return res;
    },
    [storeApi, updateWatch],
  );

  return {
    items,
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
