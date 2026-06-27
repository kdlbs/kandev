import { queryOptions } from "@tanstack/react-query";
import { getLinearConfig, listLinearIssueWatches } from "@/lib/api/domains/linear-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function linearConfigQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.linear.config(),
    queryFn: ({ signal }) => getLinearConfig(withSignal(signal)),
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
