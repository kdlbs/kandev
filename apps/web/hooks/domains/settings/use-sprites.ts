"use client";

import { useQuery } from "@tanstack/react-query";
import {
  spritesInstancesQueryOptions,
  spritesStatusQueryOptions,
} from "@/lib/query/query-options/settings";

export function useSprites(secretId?: string) {
  const statusQuery = useQuery(spritesStatusQueryOptions(secretId));
  const instancesQuery = useQuery(spritesInstancesQueryOptions(secretId));

  return {
    status: statusQuery.data ?? null,
    instances: instancesQuery.data ?? [],
    loaded: statusQuery.isSuccess && instancesQuery.isSuccess,
    loading:
      (statusQuery.isFetching && !statusQuery.isSuccess) ||
      (instancesQuery.isFetching && !instancesQuery.isSuccess),
  };
}
