import { queryOptions } from "@tanstack/react-query";
import { fetchSentryConfig, listSentryIssueWatches } from "@/lib/api/domains/sentry-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function sentryConfigQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.sentry.config(),
    queryFn: ({ signal }) => fetchSentryConfig(withSignal(signal)),
    refetchInterval: 90_000,
  });
}

export function sentryIssueWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.sentry.issueWatches(workspaceId),
    queryFn: ({ signal }) => listSentryIssueWatches(workspaceId ?? undefined, withSignal(signal)),
    enabled: workspaceId !== null,
  });
}
