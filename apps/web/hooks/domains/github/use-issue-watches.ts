"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createIssueWatch,
  updateIssueWatch,
  deleteIssueWatch,
  triggerIssueWatch,
  triggerAllIssueWatches,
  previewResetIssueWatch,
  resetIssueWatch,
} from "@/lib/api/domains/github-api";
import { qk } from "@/lib/query/keys";
import { issueWatchesQueryOptions } from "@/lib/query/query-options/github";
import type {
  CreateIssueWatchRequest,
  IssueWatch,
  UpdateIssueWatchRequest,
} from "@/lib/types/github";

// useIssueWatches has three modes:
//   - workspaceId: string         → fetch watches scoped to one workspace
//   - workspaceId: undefined      → fetch watches across all workspaces
//   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
export function useIssueWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(issueWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateIssueWatchRequest) => {
      const watch = await createIssueWatch(req);
      patchIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const update = useCallback(
    async (id: string, req: UpdateIssueWatchRequest) => {
      const watch = await updateIssueWatch(id, req);
      patchIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteIssueWatch(id);
      removeIssueWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback(async (id: string) => {
    return triggerIssueWatch(id);
  }, []);

  const triggerAll = useCallback(async () => {
    if (!workspaceId) return null;
    return triggerAllIssueWatches(workspaceId);
  }, [workspaceId]);

  const previewReset = useCallback(async (id: string, watchWorkspaceId: string) => {
    return previewResetIssueWatch(id, watchWorkspaceId);
  }, []);

  const reset = useCallback(
    async (id: string, watchWorkspaceId: string) => {
      const res = await resetIssueWatch(id, watchWorkspaceId);
      // Patch the cached watch so the "Last polled" column reflects the
      // reset immediately without waiting for the next poll tick.
      const current = items.find((w) => w.id === id);
      if (current) {
        const patched = { ...current, last_polled_at: null };
        patchIssueWatchCaches(queryClient, workspaceId, patched);
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

function patchIssueWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: IssueWatch,
) {
  const patch = (prev: IssueWatch[] | undefined) => upsertById(prev ?? [], watch);
  const patchExisting = (prev: IssueWatch[] | undefined) => (prev ? upsertById(prev, watch) : prev);
  queryClient.setQueryData(qk.integrations.github.issueWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.github.issueWatches(undefined), patchExisting);
  queryClient.setQueryData(qk.integrations.github.issueWatches(watch.workspace_id), patch);
}

function removeIssueWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: IssueWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  const removeExisting = (prev: IssueWatch[] | undefined) =>
    prev ? prev.filter((watch) => watch.id !== id) : prev;
  queryClient.setQueryData(qk.integrations.github.issueWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.github.issueWatches(undefined), removeExisting);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
