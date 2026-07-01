"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
  previewResetReviewWatch,
  resetReviewWatch,
} from "@/lib/api/domains/github-api";
import { qk } from "@/lib/query/keys";
import { reviewWatchesQueryOptions } from "@/lib/query/query-options/github";
import type {
  CreateReviewWatchRequest,
  ReviewWatch,
  UpdateReviewWatchRequest,
} from "@/lib/types/github";

// useReviewWatches has three modes:
//   - workspaceId: string         → fetch watches scoped to one workspace
//   - workspaceId: undefined      → fetch watches across all workspaces
//   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
export function useReviewWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(reviewWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateReviewWatchRequest) => {
      const watch = await createReviewWatch(req);
      patchReviewWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const update = useCallback(
    async (id: string, watchWorkspaceId: string, req: UpdateReviewWatchRequest) => {
      const watch = await updateReviewWatch(id, watchWorkspaceId, req);
      patchReviewWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      await deleteReviewWatch(id, watchWorkspaceId);
      removeReviewWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback(async (id: string, watchWorkspaceId: string) => {
    return triggerReviewWatch(id, watchWorkspaceId);
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
      const current = items.find((w) => w.id === id);
      if (current) {
        const patched = { ...current, last_polled_at: null };
        patchReviewWatchCaches(queryClient, workspaceId, patched);
      }
      return res;
    },
    [items, queryClient, workspaceId],
  );

  return {
    items,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    create,
    update,
    remove,
    trigger,
    triggerAll,
    previewReset,
    reset,
  };
}

function patchReviewWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: ReviewWatch,
) {
  const patch = (prev: ReviewWatch[] | undefined) => upsertById(prev ?? [], watch);
  const patchExisting = (prev: ReviewWatch[] | undefined) =>
    prev ? upsertById(prev, watch) : prev;
  queryClient.setQueryData(qk.integrations.github.reviewWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.github.reviewWatches(undefined), patchExisting);
  queryClient.setQueryData(qk.integrations.github.reviewWatches(watch.workspace_id), patch);
}

function removeReviewWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: ReviewWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  const removeExisting = (prev: ReviewWatch[] | undefined) =>
    prev ? prev.filter((watch) => watch.id !== id) : prev;
  queryClient.setQueryData(qk.integrations.github.reviewWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.github.reviewWatches(undefined), removeExisting);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
