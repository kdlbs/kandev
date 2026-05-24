"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

export function useAgentDiscovery(enabled = true) {
  const query = useQuery({ ...settingsQueryOptions.agentDiscovery(), enabled });
  return {
    items: query.data ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}
