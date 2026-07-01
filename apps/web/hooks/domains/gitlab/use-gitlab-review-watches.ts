"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
  type CreateReviewWatchRequest,
  type UpdateReviewWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import { qk } from "@/lib/query/keys";
import { gitlabReviewWatchesQueryOptions } from "@/lib/query/query-options/gitlab";
import type { ReviewWatch } from "@/lib/types/gitlab";

/**
 * useGitLabReviewWatches — three modes:
 *   - workspaceId: string         → fetch watches scoped to one workspace
 *   - workspaceId: undefined      → fetch watches across all workspaces
 *   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
 */
export function useGitLabReviewWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(gitlabReviewWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateReviewWatchRequest) => {
      const watch = await createReviewWatch(req);
      patchGitLabReviewWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const update = useCallback(
    async (id: string, req: UpdateReviewWatchRequest) => {
      const watch = await updateReviewWatch(id, req);
      patchGitLabReviewWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteReviewWatch(id);
      removeGitLabReviewWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback((id: string) => triggerReviewWatch(id), []);
  const triggerAll = useCallback(() => triggerAllReviewWatches(), []);

  return {
    items,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    create,
    update,
    remove,
    trigger,
    triggerAll,
  };
}

function patchGitLabReviewWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: ReviewWatch,
) {
  const patch = (prev: ReviewWatch[] | undefined) => upsertById(prev ?? [], watch);
  const patchExisting = (prev: ReviewWatch[] | undefined) =>
    prev ? upsertById(prev, watch) : prev;
  queryClient.setQueryData(qk.integrations.gitlab.reviewWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.gitlab.reviewWatches(undefined), patchExisting);
  queryClient.setQueryData(qk.integrations.gitlab.reviewWatches(watch.workspace_id), patch);
}

function removeGitLabReviewWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: ReviewWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  const removeExisting = (prev: ReviewWatch[] | undefined) =>
    prev ? prev.filter((watch) => watch.id !== id) : prev;
  queryClient.setQueryData(qk.integrations.gitlab.reviewWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.gitlab.reviewWatches(undefined), removeExisting);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
