"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createLinearIssueWatch,
  updateLinearIssueWatch,
  deleteLinearIssueWatch,
  triggerLinearIssueWatch,
  previewResetLinearIssueWatch,
  resetLinearIssueWatch,
} from "@/lib/api/domains/linear-api";
import { qk } from "@/lib/query/keys";
import { linearIssueWatchesQueryOptions } from "@/lib/query/query-options/linear";
import type {
  CreateLinearIssueWatchInput,
  LinearIssueWatch,
  UpdateLinearIssueWatchInput,
} from "@/lib/types/linear";

// WORKSPACE_REQUIRED is thrown by per-row mutation callbacks when the
// install-wide listing case forgets to forward the row's workspaceId.
const WORKSPACE_REQUIRED = "workspaceId required";

/**
 * useLinearIssueWatches owns the Linear-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces; the
 *                              caller supplies workspaceId to update/remove/trigger
 *                              calls per-row (those endpoints still validate it
 *                              against the watch's stored workspace as an IDOR guard)
 *   - workspaceId: null      → don't fetch
 *
 * Mirrors `useJiraIssueWatches`. Workspace changes reset the cached list so
 * the user doesn't see the previous workspace's stale rows during the swap.
 */
export function useLinearIssueWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(linearIssueWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateLinearIssueWatchInput) => {
      const watch = await createLinearIssueWatch(req);
      patchLinearIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  // Per-row mutations require the row's own workspace_id to satisfy the
  // backend's IDOR guard. Callers pass it explicitly when the hook itself
  // wasn't bound to a single workspace (the install-wide listing case).
  const update = useCallback(
    async (id: string, req: UpdateLinearIssueWatchInput, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      const watch = await updateLinearIssueWatch(ws, id, req);
      patchLinearIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      await deleteLinearIssueWatch(ws, id);
      removeLinearIssueWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return triggerLinearIssueWatch(ws, id);
    },
    [workspaceId],
  );

  const previewReset = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return previewResetLinearIssueWatch(ws, id);
    },
    [workspaceId],
  );

  const reset = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      const res = await resetLinearIssueWatch(ws, id);
      queryClient.invalidateQueries({ queryKey: qk.integrations.linear.issueWatches(workspaceId) });
      queryClient.invalidateQueries({ queryKey: qk.integrations.linear.issueWatches(undefined) });
      queryClient.invalidateQueries({ queryKey: qk.integrations.linear.issueWatches(ws) });
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

function patchLinearIssueWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: LinearIssueWatch,
) {
  const patch = (prev: LinearIssueWatch[] | undefined) => upsertById(prev ?? [], watch);
  queryClient.setQueryData(qk.integrations.linear.issueWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.linear.issueWatches(undefined), patch);
  queryClient.setQueryData(qk.integrations.linear.issueWatches(watch.workspaceId), patch);
}

function removeLinearIssueWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: LinearIssueWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  queryClient.setQueryData(qk.integrations.linear.issueWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.linear.issueWatches(undefined), remove);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
