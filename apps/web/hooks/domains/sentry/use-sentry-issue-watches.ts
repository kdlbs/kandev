"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createSentryIssueWatch,
  updateSentryIssueWatch,
  deleteSentryIssueWatch,
  triggerSentryIssueWatch,
  previewResetSentryIssueWatch,
  resetSentryIssueWatch,
} from "@/lib/api/domains/sentry-api";
import { qk } from "@/lib/query/keys";
import { sentryIssueWatchesQueryOptions } from "@/lib/query/query-options/sentry";
import type {
  CreateSentryIssueWatchRequest,
  SentryIssueWatch,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";

/**
 * useSentryIssueWatches owns the Sentry-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces
 *   - workspaceId: null      → don't fetch
 *
 * Mirrors `useLinearIssueWatches`: update/delete/trigger pass the watch's
 * workspace id as the `workspace_id` query param so the backend can guard
 * cross-workspace mutations.
 */
export function useSentryIssueWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(sentryIssueWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateSentryIssueWatchRequest) => {
      const watch = await createSentryIssueWatch(req);
      patchSentryIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const update = useCallback(
    async (id: string, watchWorkspaceId: string, req: UpdateSentryIssueWatchRequest) => {
      const watch = await updateSentryIssueWatch(id, watchWorkspaceId, req);
      patchSentryIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      await deleteSentryIssueWatch(id, watchWorkspaceId);
      removeSentryIssueWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback(async (id: string, watchWorkspaceId: string) => {
    return triggerSentryIssueWatch(id, watchWorkspaceId);
  }, []);

  const previewReset = useCallback(async (id: string, watchWorkspaceId: string) => {
    return previewResetSentryIssueWatch(id, watchWorkspaceId);
  }, []);

  const reset = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      const res = await resetSentryIssueWatch(id, watchWorkspaceId);
      queryClient.invalidateQueries({ queryKey: qk.integrations.sentry.issueWatches(workspaceId) });
      queryClient.invalidateQueries({ queryKey: qk.integrations.sentry.issueWatches(undefined) });
      queryClient.invalidateQueries({
        queryKey: qk.integrations.sentry.issueWatches(watchWorkspaceId),
      });
      return res;
    },
    [queryClient, workspaceId],
  );

  return {
    items,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    create,
    update,
    remove,
    trigger,
    previewReset,
    reset,
  };
}

function patchSentryIssueWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: SentryIssueWatch,
) {
  const patch = (prev: SentryIssueWatch[] | undefined) => upsertById(prev ?? [], watch);
  const patchExisting = (prev: SentryIssueWatch[] | undefined) =>
    prev ? upsertById(prev, watch) : prev;
  queryClient.setQueryData(qk.integrations.sentry.issueWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.sentry.issueWatches(undefined), patchExisting);
  queryClient.setQueryData(qk.integrations.sentry.issueWatches(watch.workspaceId), patch);
}

function removeSentryIssueWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: SentryIssueWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  const removeExisting = (prev: SentryIssueWatch[] | undefined) =>
    prev ? prev.filter((watch) => watch.id !== id) : prev;
  queryClient.setQueryData(qk.integrations.sentry.issueWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.sentry.issueWatches(undefined), removeExisting);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
