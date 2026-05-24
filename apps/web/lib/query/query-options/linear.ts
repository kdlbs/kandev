import { queryOptions } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { listLinearIssueWatches } from "@/lib/api/domains/linear-api";

/**
 * Query options for the linear domain.
 *
 * watches(workspaceId) — issue-watch list, scoped to one workspace or
 *                        install-wide when workspaceId is null.
 *
 * The linear integration has no WS events — the bridge is a no-op. Data
 * freshness is driven by query invalidation from mutations (create /
 * update / delete / trigger).
 */
export const linearQueryOptions = {
  /**
   * List issue watches.
   *
   * @param workspaceId  Scope to one workspace (string) or fetch all (null).
   *                     Pass undefined to skip fetching (enabled=false path).
   */
  watches: (workspaceId: string | null | undefined) =>
    queryOptions({
      queryKey: qk.linear.watches(workspaceId ?? null),
      queryFn: () =>
        listLinearIssueWatches(workspaceId ?? undefined, { cache: "no-store" }),
      // Skip fetching when the caller passes undefined (i.e. workspaceId === null
      // means all-workspace listing; undefined means skip entirely).
      enabled: workspaceId !== undefined,
      // Watches change only via mutations — 30 s staleTime from global default
      // is fine; no custom override needed.
    }),
} as const;
