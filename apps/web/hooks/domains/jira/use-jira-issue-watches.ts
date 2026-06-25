"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createJiraIssueWatch,
  updateJiraIssueWatch,
  deleteJiraIssueWatch,
  triggerJiraIssueWatch,
  previewResetJiraIssueWatch,
  resetJiraIssueWatch,
} from "@/lib/api/domains/jira-api";
import { qk } from "@/lib/query/keys";
import { jiraIssueWatchesQueryOptions } from "@/lib/query/query-options/jira";
import type {
  CreateJiraIssueWatchInput,
  JiraIssueWatch,
  UpdateJiraIssueWatchInput,
} from "@/lib/types/jira";

// WORKSPACE_REQUIRED is thrown by per-row mutation callbacks when the
// install-wide listing case forgets to forward the row's workspaceId
// (the install-wide hook leaves workspaceId undefined to fetch all rows).
const WORKSPACE_REQUIRED = "workspaceId required";

/**
 * useJiraIssueWatches owns the JIRA-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces; the
 *                              caller supplies workspaceId to update/remove/trigger
 *                              calls per-row (those endpoints still validate it
 *                              against the watch's stored workspace as an IDOR guard)
 *   - workspaceId: null      → don't fetch
 *
 * Workspace changes reset the cached list so the user doesn't see the previous
 * workspace's stale rows during the swap.
 */
export function useJiraIssueWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(jiraIssueWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateJiraIssueWatchInput) => {
      const watch = await createJiraIssueWatch(req);
      patchJiraIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  // Per-row mutations require the row's own workspace_id to satisfy the
  // backend's IDOR guard. Callers pass it explicitly when the hook itself
  // wasn't bound to a single workspace (the install-wide listing case).
  const update = useCallback(
    async (id: string, req: UpdateJiraIssueWatchInput, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      const watch = await updateJiraIssueWatch(ws, id, req);
      patchJiraIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      await deleteJiraIssueWatch(ws, id);
      removeJiraIssueWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return triggerJiraIssueWatch(ws, id);
    },
    [workspaceId],
  );

  // previewReset / reset use the same per-row workspaceId pattern as update /
  // remove / trigger — the install-wide listing case relies on the caller
  // passing the row's stored workspaceId, satisfying the backend IDOR guard.
  const previewReset = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      return previewResetJiraIssueWatch(ws, id);
    },
    [workspaceId],
  );

  const reset = useCallback(
    async (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error(WORKSPACE_REQUIRED);
      const res = await resetJiraIssueWatch(ws, id);
      queryClient.invalidateQueries({ queryKey: qk.integrations.jira.issueWatches(workspaceId) });
      queryClient.invalidateQueries({ queryKey: qk.integrations.jira.issueWatches(undefined) });
      queryClient.invalidateQueries({ queryKey: qk.integrations.jira.issueWatches(ws) });
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

function patchJiraIssueWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: JiraIssueWatch,
) {
  const patch = (prev: JiraIssueWatch[] | undefined) => upsertById(prev ?? [], watch);
  queryClient.setQueryData(qk.integrations.jira.issueWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.jira.issueWatches(undefined), patch);
  queryClient.setQueryData(qk.integrations.jira.issueWatches(watch.workspaceId), patch);
}

function removeJiraIssueWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: JiraIssueWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  queryClient.setQueryData(qk.integrations.jira.issueWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.jira.issueWatches(undefined), remove);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
