"use client";

import { useQuery } from "@tanstack/react-query";
import { workflowsListQueryOptions } from "@/lib/query/query-options/kanban";

/**
 * Reads the workspace-scoped workflows list from the TanStack Query cache
 * (`qk.kanban.workflowsList`). Fetching is gated by `enabled` and a non-null
 * workspaceId. The kanban bridge keeps the cache fresh via workflow.* WS events.
 *
 * Returns `{ workflows }` (the workflow metadata list) — empty array while the
 * query is disabled or still loading.
 */
export function useWorkflows(workspaceId: string | null, enabled = true) {
  const { data } = useQuery({
    ...workflowsListQueryOptions(workspaceId ?? ""),
    enabled: enabled && !!workspaceId,
  });
  return { workflows: data ?? [] };
}
