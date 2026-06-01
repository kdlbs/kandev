"use client";

import { useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createJiraIssueWatch,
  updateJiraIssueWatch,
  deleteJiraIssueWatch,
  triggerJiraIssueWatch,
} from "@/lib/api/domains/jira-api";
import { jiraQueryOptions } from "@/lib/query/query-options/jira";
import { qk } from "@/lib/query/keys";
import type { CreateJiraIssueWatchInput, UpdateJiraIssueWatchInput } from "@/lib/types/jira";

/**
 * useJiraIssueWatches owns the JIRA-watcher list via TanStack Query.
 *
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces; the
 *                              caller supplies workspaceId to update/remove/trigger
 *                              calls per-row (those endpoints still validate it
 *                              against the watch's stored workspace as an IDOR guard)
 *   - workspaceId: null      → don't fetch (enabled: false)
 *
 * On workspace change TanStack Query automatically uses the new key, so
 * there is no manual reset step — the previous scope's data stays in cache
 * briefly then is GC'd.
 *
 * All mutations invalidate qk.jira.issueWatches() (the install-wide key) so
 * both scoped and unscoped consumers see fresh data after any write.
 */
export function useJiraIssueWatches(workspaceId?: string | null) {
  const qc = useQueryClient();

  const { data: items = [], isLoading: loading } = useQuery({
    ...jiraQueryOptions.issueWatches(workspaceId ?? undefined),
    enabled: workspaceId !== null,
  });

  // Invalidate both the scoped and install-wide keys after any mutation so
  // JiraIssueWatchersSection (which uses the all-workspaces key) stays fresh.
  const invalidate = useCallback(async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: qk.jira.issueWatches() }),
      workspaceId
        ? qc.invalidateQueries({ queryKey: qk.jira.issueWatches(workspaceId) })
        : Promise.resolve(),
    ]);
  }, [qc, workspaceId]);

  const createMutation = useMutation({
    mutationFn: (req: CreateJiraIssueWatchInput) => createJiraIssueWatch(req),
    onSettled: invalidate,
  });

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      req,
      rowWorkspaceId,
    }: {
      id: string;
      req: UpdateJiraIssueWatchInput;
      rowWorkspaceId: string;
    }) => updateJiraIssueWatch(rowWorkspaceId, id, req),
    onSettled: invalidate,
  });

  const removeMutation = useMutation({
    mutationFn: ({ id, rowWorkspaceId }: { id: string; rowWorkspaceId: string }) =>
      deleteJiraIssueWatch(rowWorkspaceId, id),
    onSettled: invalidate,
  });

  const triggerMutation = useMutation({
    mutationFn: ({ id, rowWorkspaceId }: { id: string; rowWorkspaceId: string }) =>
      triggerJiraIssueWatch(rowWorkspaceId, id),
    // No invalidation needed — trigger doesn't change the watch list.
  });

  const create = useCallback(
    (req: CreateJiraIssueWatchInput) => createMutation.mutateAsync(req),
    [createMutation],
  );

  // Per-row mutations require the row's own workspace_id to satisfy the
  // backend's IDOR guard. Callers pass it explicitly when the hook itself
  // wasn't bound to a single workspace (the install-wide listing case).
  const update = useCallback(
    (id: string, req: UpdateJiraIssueWatchInput, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      return updateMutation.mutateAsync({ id, req, rowWorkspaceId: ws });
    },
    [updateMutation, workspaceId],
  );

  const remove = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      return removeMutation.mutateAsync({ id, rowWorkspaceId: ws });
    },
    [removeMutation, workspaceId],
  );

  const trigger = useCallback(
    (id: string, rowWorkspaceId?: string) => {
      const ws = rowWorkspaceId ?? workspaceId;
      if (!ws) throw new Error("workspaceId required");
      return triggerMutation.mutateAsync({ id, rowWorkspaceId: ws });
    },
    [triggerMutation, workspaceId],
  );

  return { items, loaded: !loading, loading, create, update, remove, trigger };
}
