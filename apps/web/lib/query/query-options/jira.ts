import { queryOptions } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { listJiraIssueWatches } from "@/lib/api/domains/jira-api";

/**
 * Query options for the Jira domain.
 *
 * These options are shared between SSR prefetch and CSR useQuery calls.
 *
 * issueWatches(wsId?)  — Fetches the list of JIRA issue-watch rules.
 *   • wsId provided → scoped to one workspace.
 *   • wsId omitted  → install-wide list across all workspaces.
 *
 * Jira has no WebSocket events (REST-only integration), so there is no
 * WS bridge writing into these keys. The cache is invalidated explicitly
 * by mutations (create / update / delete / trigger) via onSettled.
 *
 * staleTime is set to 0 so the list is always fresh on mount — watches
 * are mutated infrequently but correctness matters more than network
 * savings here.
 */
export const jiraQueryOptions = {
  issueWatches: (wsId?: string) =>
    queryOptions({
      queryKey: qk.jira.issueWatches(wsId),
      queryFn: () => listJiraIssueWatches(wsId),
      staleTime: 0,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    }),
} as const;
