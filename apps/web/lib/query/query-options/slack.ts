import { queryOptions } from "@tanstack/react-query";
import { getSlackConfig } from "@/lib/api/domains/slack-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function slackConfigQueryOptions(workspaceId?: string | null) {
  return queryOptions({
    queryKey: qk.integrations.slack.config(workspaceId),
    queryFn: ({ signal }) =>
      getSlackConfig({ ...withSignal(signal), ...(workspaceId ? { workspaceId } : {}) }),
    enabled: workspaceId !== null,
    refetchInterval: 90_000,
  });
}
