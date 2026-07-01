import { queryOptions } from "@tanstack/react-query";
import { getJiraConfig, listJiraIssueWatches } from "@/lib/api/domains/jira-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function jiraConfigQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.jira.config(),
    queryFn: ({ signal }) => getJiraConfig(withSignal(signal)),
    refetchInterval: 90_000,
  });
}

export function jiraIssueWatchesQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.jira.issueWatches(workspaceId),
    queryFn: ({ signal }) => listJiraIssueWatches(workspaceId ?? undefined, withSignal(signal)),
    enabled: workspaceId !== null,
  });
}
