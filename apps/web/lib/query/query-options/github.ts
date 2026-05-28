import { queryOptions } from "@tanstack/react-query";
import {
  fetchGitHubStatus,
  listPRWatches,
  listReviewWatches,
  listIssueWatches,
  fetchGitHubActionPresets,
  listWorkspaceTaskPRs,
  getPRFeedback,
} from "@/lib/api/domains/github-api";
import { qk } from "@/lib/query/keys";

/**
 * queryOptions factories for the GitHub domain.
 *
 * Import these in useQuery() calls and SSR prefetch:
 *   useQuery(githubQueryOptions.status())
 *   useQuery(githubQueryOptions.workspacePRs(wsId))
 *   useQuery(githubQueryOptions.prWatches())
 *   useQuery(githubQueryOptions.reviewWatches(wsId))
 *   useQuery(githubQueryOptions.issueWatches(wsId))
 *   useQuery(githubQueryOptions.actionPresets(wsId))
 *   useQuery(githubQueryOptions.prFeedback(owner, repo, number))
 */
export const githubQueryOptions = {
  /** GitHub auth / connection status. */
  status: () =>
    queryOptions({
      queryKey: qk.github.status(),
      queryFn: () => fetchGitHubStatus({ cache: "no-store" }),
      staleTime: 30_000,
    }),

  /**
   * All PR associations for a workspace.
   * Returns TaskPRsResponse: { task_prs: Record<string, TaskPR[]> }.
   */
  workspacePRs: (wsId: string) =>
    queryOptions({
      queryKey: qk.github.prs(wsId),
      queryFn: () => listWorkspaceTaskPRs(wsId, { cache: "no-store" }),
      enabled: !!wsId,
      staleTime: 30_000,
    }),

  /** Global PR watches list (session-based, not workspace-scoped). */
  prWatches: () =>
    queryOptions({
      queryKey: qk.github.prWatches(),
      queryFn: () => listPRWatches({ cache: "no-store" }),
      staleTime: 30_000,
    }),

  /**
   * Review watches, optionally scoped to a workspace.
   * Pass wsId=undefined to fetch all; wsId=null to skip (not-yet-resolved).
   */
  reviewWatches: (wsId?: string | null) =>
    queryOptions({
      queryKey:
        wsId !== null ? qk.github.reviewWatches(wsId ?? undefined) : qk.github.reviewWatches(),
      queryFn: () => listReviewWatches(wsId ?? undefined, { cache: "no-store" }),
      enabled: wsId !== null,
      staleTime: 30_000,
    }),

  /**
   * Issue watches, optionally scoped to a workspace.
   * Pass wsId=undefined to fetch all; wsId=null to skip.
   */
  issueWatches: (wsId?: string | null) =>
    queryOptions({
      queryKey:
        wsId !== null ? qk.github.issueWatches(wsId ?? undefined) : qk.github.issueWatches(),
      queryFn: () => listIssueWatches(wsId ?? undefined, { cache: "no-store" }),
      enabled: wsId !== null,
      staleTime: 30_000,
    }),

  /** Action presets (quick-launch prompts) for a workspace. */
  actionPresets: (wsId: string | null) =>
    queryOptions({
      queryKey: wsId
        ? qk.github.actionPresets(wsId)
        : (["github", "action-presets", null] as const),
      queryFn: () => fetchGitHubActionPresets(wsId!, { cache: "no-store" }),
      enabled: !!wsId,
      staleTime: 60_000,
    }),

  /**
   * PR feedback (reviews, comments, checks) — stale-while-revalidate.
   * Fetched on demand (popover open or PR updated_at change).
   */
  prFeedback: (owner: string | null, repo: string | null, prNumber: number | null) =>
    queryOptions({
      queryKey:
        owner && repo && prNumber
          ? qk.github.prFeedback(owner, repo, prNumber)
          : (["github", "pr-feedback", null] as const),
      queryFn: () => getPRFeedback(owner!, repo!, prNumber!, { cache: "no-store" }),
      enabled: !!(owner && repo && prNumber),
      staleTime: 30_000,
      // Never auto-refetch — callers trigger refetch imperatively.
      refetchOnWindowFocus: false,
    }),
};
