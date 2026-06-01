import { queryOptions } from "@tanstack/react-query";
import { fetchGitLabStatus, listWorkspaceTaskMRs } from "@/lib/api/domains/gitlab-api";
import { qk } from "@/lib/query/keys";

/**
 * queryOptions factories for the GitLab domain.
 *
 * Re-uses the existing gitlab-api.ts fetch functions as queryFns.
 * Co-located with qk.gitlab.* key factories for SSR prefetch and CSR useQuery.
 */
export const gitlabQueryOptions = {
  /**
   * GitLab connection status for the current user.
   * Short stale time since auth state can change without a page reload.
   */
  status: () =>
    queryOptions({
      queryKey: ["gitlab", "status"] as const,
      queryFn: () => fetchGitLabStatus({ cache: "no-store" }),
      staleTime: 30_000,
    }),

  /**
   * All MR associations for tasks in a workspace, grouped by task ID.
   * Disabled when workspaceId is null/empty.
   */
  workspaceMRs: (workspaceId: string | null) =>
    queryOptions({
      queryKey: qk.gitlab.mrs(workspaceId ?? ""),
      queryFn: () => listWorkspaceTaskMRs(workspaceId!, { cache: "no-store" }),
      enabled: !!workspaceId,
      staleTime: 30_000,
    }),
};
