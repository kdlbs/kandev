"use client";

import { useQuery } from "@tanstack/react-query";
import { systemHealthQueryOptions } from "@/lib/query/query-options/settings";

export function useSystemHealth() {
  const query = useQuery(systemHealthQueryOptions());
  const data = query.data ?? null;

  return {
    issues: data?.issues ?? [],
    checks: data?.checks ?? [],
    healthy: data?.healthy ?? true,
    loaded: query.isSuccess || query.isError,
    loading: query.isFetching && !query.isSuccess,
  };
}
