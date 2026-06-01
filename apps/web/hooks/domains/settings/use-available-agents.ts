"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

export function useAvailableAgents(enabled = true) {
  const query = useQuery({ ...settingsQueryOptions.availableAgents(), enabled });
  return {
    items: query.data?.agents ?? [],
    tools: query.data?.tools ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}
