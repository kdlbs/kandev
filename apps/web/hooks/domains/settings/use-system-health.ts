"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

export function useSystemHealth() {
  const query = useQuery(settingsQueryOptions.systemHealth());
  return {
    issues: query.data?.issues ?? [],
    healthy: query.data?.healthy ?? true,
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}
