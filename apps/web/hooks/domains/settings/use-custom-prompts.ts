"use client";

import { useQuery } from "@tanstack/react-query";
import { promptsQueryOptions } from "@/lib/query/query-options/settings";

export function useCustomPrompts() {
  const query = useQuery(promptsQueryOptions());

  return {
    prompts: query.data?.prompts ?? [],
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
  };
}
