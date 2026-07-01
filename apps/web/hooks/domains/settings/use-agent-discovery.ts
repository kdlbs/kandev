import { useQuery } from "@tanstack/react-query";
import { agentDiscoveryQueryOptions } from "@/lib/query/query-options/settings";

export function useAgentDiscovery(enabled = true) {
  const query = useQuery({ ...agentDiscoveryQueryOptions(), enabled });

  return {
    items: query.data?.agents ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
  };
}
