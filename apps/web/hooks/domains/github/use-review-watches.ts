"use client";

import { useEffect, useCallback } from "react";
import {
  listReviewWatches,
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
} from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import type { CreateReviewWatchRequest, UpdateReviewWatchRequest } from "@/lib/types/github";

export function useReviewWatches(workspaceId: string | null) {
  const items = useAppStore((state) => state.reviewWatches.items);
  const loaded = useAppStore((state) => state.reviewWatches.loaded);
  const loading = useAppStore((state) => state.reviewWatches.loading);
  const setReviewWatches = useAppStore((state) => state.setReviewWatches);
  const setReviewWatchesLoading = useAppStore((state) => state.setReviewWatchesLoading);
  const addWatch = useAppStore((state) => state.addReviewWatch);
  const updateWatch = useAppStore((state) => state.updateReviewWatch);
  const removeWatch = useAppStore((state) => state.removeReviewWatch);

  useEffect(() => {
    if (!workspaceId || loaded || loading) return;
    setReviewWatchesLoading(true);
    listReviewWatches(workspaceId, { cache: "no-store" })
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

  return {
    items,
    loaded,
    loading,
    create,
    update,
    remove,
    trigger,
    triggerAll,
  };
}
