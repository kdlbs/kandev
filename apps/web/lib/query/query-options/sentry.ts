import { queryOptions } from "@tanstack/react-query";
import { listSentryInstances, listSentryIssueWatches } from "@/lib/api/domains/sentry-api";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function sentryInstancesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.sentry.instances(workspaceId),
    queryFn: ({ signal }) =>
      workspaceId ? listSentryInstances(workspaceId, withSignal(signal)) : Promise.resolve([]),
    enabled: workspaceId !== null,
    refetchInterval: INTEGRATION_STATUS_REFRESH_MS,
  });
}

export function sentryIssueWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.sentry.issueWatches(workspaceId),
    queryFn: ({ signal }) => listSentryIssueWatches(workspaceId ?? undefined, withSignal(signal)),
    enabled: workspaceId !== null,
  });
}
