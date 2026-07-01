"use client";

import { useQuery } from "@tanstack/react-query";
import { notificationProvidersQueryOptions } from "@/lib/query/query-options/settings";

export function useNotificationProviders() {
  const query = useQuery(notificationProvidersQueryOptions());

  return {
    providers: query.data?.providers ?? [],
    events: query.data?.events ?? [],
    appriseAvailable: query.data?.apprise_available ?? false,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
  };
}
