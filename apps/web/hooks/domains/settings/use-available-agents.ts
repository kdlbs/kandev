import { useQuery } from "@tanstack/react-query";
import { availableAgentsQueryOptions } from "@/lib/query/query-options/settings";

export function useAvailableAgents(enabled = true) {
  const query = useQuery({ ...availableAgentsQueryOptions(), enabled });

  return {
    items: query.data?.agents ?? [],
    tools: query.data?.tools ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
  };
}
