"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  listLinearIssueWatches,
  createLinearIssueWatch,
  updateLinearIssueWatch,
  deleteLinearIssueWatch,
  triggerLinearIssueWatch,
} from "@/lib/api/domains/linear-api";
import { qk } from "@/lib/query/keys";
import type { CreateLinearIssueWatchInput, UpdateLinearIssueWatchInput } from "@/lib/types/linear";

/**
 * useLinearIssueWatches owns the Linear-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces; the
 *                              caller supplies workspaceId to update/remove/trigger
 *                              calls per-row (those endpoints still validate it
 *                              against the watch's stored workspace as an IDOR guard)
 *   - workspaceId: null      → don't fetch
 *
 * Mirrors `useJiraIssueWatches` and the Zustand-backed predecessor — same
 * public API surface so no callers need updating.
 */
export function useLinearIssueWatches(workspaceId?: string | null) {
  const qc = useQueryClient();

  // null → skip fetching; undefined → all-workspace listing; string → scoped.
  // TQ enabled flag handles the null case; key encodes undefined vs string.
  const queryKey = qk.linear.watches(workspaceId !== null ? (workspaceId ?? null) : null);

  const { data, isLoading, isFetching } = useQuery({
    queryKey,
    queryFn: () => listLinearIssueWatches(workspaceId ?? undefined, { cache: "no-store" }),
    enabled: workspaceId !== null,
  });

  const items = data ?? [];
  // loaded: true when we have data (even an empty array) and are not on the
  // initial loading pass. Preserves the old contract callers depend on.
  const loaded = !isLoading && workspaceId !== null;
  const loading = isFetching && items.length === 0;

  const invalidate = () => qc.invalidateQueries({ queryKey });

  // --- mutations ---

  const createMutation = useMutation({
    mutationFn: (req: CreateLinearIssueWatchInput) => createLinearIssueWatch(req),
    onSuccess: () => invalidate(),
  });

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      ws,
      req,
    }: {
      id: string;
      ws: string;
      req: UpdateLinearIssueWatchInput;
    }) => updateLinearIssueWatch(ws, id, req),
    onSuccess: () => invalidate(),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ id, ws }: { id: string; ws: string }) => deleteLinearIssueWatch(ws, id),
    onSuccess: () => invalidate(),
  });

  const triggerMutation = useMutation({
    mutationFn: ({ id, ws }: { id: string; ws: string }) => triggerLinearIssueWatch(ws, id),
    // Trigger does not modify the list — no cache invalidation needed.
  });

  // --- stable callback helpers (mirror old Zustand-hook API surface) ---

  function create(req: CreateLinearIssueWatchInput) {
    return createMutation.mutateAsync(req);
  }

  function update(id: string, req: UpdateLinearIssueWatchInput, rowWorkspaceId?: string) {
    const ws = rowWorkspaceId ?? workspaceId;
    if (!ws) throw new Error("workspaceId required");
    return updateMutation.mutateAsync({ id, ws, req });
  }

  function remove(id: string, rowWorkspaceId?: string) {
    const ws = rowWorkspaceId ?? workspaceId;
    if (!ws) throw new Error("workspaceId required");
    return deleteMutation.mutateAsync({ id, ws });
  }

  function trigger(id: string, rowWorkspaceId?: string) {
    const ws = rowWorkspaceId ?? workspaceId;
    if (!ws) throw new Error("workspaceId required");
    return triggerMutation.mutateAsync({ id, ws });
  }

  return { items, loaded, loading, create, update, remove, trigger };
}
