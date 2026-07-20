import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listAgentSubscriptionUsage } from "@/lib/api/domains/settings-api";
import { qk } from "@/lib/query/keys";
import { agentSubscriptionUsageQueryOptions } from "@/lib/query/query-options/settings";

/**
 * Fetches subscription utilization for host-installed agents (Claude Code,
 * Codex). The initial load accepts the backend's 5-minute cache; manual
 * refresh() requests fresh provider data (server-clamped to 15 s).
 */
export function useAgentSubscriptionUsage() {
  const queryClient = useQueryClient();
  const query = useQuery(agentSubscriptionUsageQueryOptions());
  const refreshMutation = useMutation({
    mutationFn: () => listAgentSubscriptionUsage({ cache: "no-store", fresh: true }),
    onSuccess: (response) =>
      queryClient.setQueryData(qk.settings.agentSubscriptionUsage(), response),
  });
  return {
    items: query.data?.agents ?? [],
    loading: query.isLoading || refreshMutation.isPending,
    refresh: refreshMutation.mutate,
  };
}
