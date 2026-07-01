import { queryOptions } from "@tanstack/react-query";
import { getLinearConfig, listLinearIssueWatches } from "@/lib/api/domains/linear-api";
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
