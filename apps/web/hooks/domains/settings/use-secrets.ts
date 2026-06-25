"use client";

import { useQuery } from "@tanstack/react-query";
import { secretsQueryOptions } from "@/lib/query/query-options/settings";

export function useSecrets() {
  const query = useQuery(secretsQueryOptions());

  return {
    items: query.data ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
  };
}
