import { queryOptions } from "@tanstack/react-query";
import {
  getLinearConfig,
  listLinearIssueWatches,
  searchLinearIssues,
} from "@/lib/api/domains/linear-api";
import type { LinearSearchFilter } from "@/lib/types/linear";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function linearConfigQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.linear.config(workspaceId),
    queryFn: ({ signal }) =>
      getLinearConfig({ ...withSignal(signal), ...(workspaceId ? { workspaceId } : {}) }),
    enabled: workspaceId !== null,
    refetchInterval: 90_000,
  });
}

export function linearIssueWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.linear.issueWatches(workspaceId),
    queryFn: ({ signal }) => listLinearIssueWatches(workspaceId ?? undefined, withSignal(signal)),
    enabled: workspaceId !== null,
  });
}

export function linearIssuesQueryOptions(
  workspaceId: string | undefined,
  params: LinearSearchFilter & { pageToken?: string; maxResults?: number },
) {
  return queryOptions({
    queryKey: qk.integrations.linear.issues(workspaceId, params),
    queryFn: ({ signal }) =>
      searchLinearIssues(params, {
        workspaceId,
        ...withSignal(signal),
      }),
    enabled: Boolean(workspaceId),
  });
}
