import { queryOptions } from "@tanstack/react-query";
import { getSlackConfig } from "@/lib/api/domains/slack-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function slackConfigQueryOptions() {
  return queryOptions({
    queryKey: qk.integrations.slack.config(),
    queryFn: ({ signal }) => getSlackConfig(withSignal(signal)),
    refetchInterval: 90_000,
  });
}
