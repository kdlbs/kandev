"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  useSystemInfoBootId,
  useSystemInfoRequestSignal,
} from "@/components/system-info-query-provider";
import { fetchSystemInfo } from "@/lib/api/domains/system-api";
import { createSystemInfoQueryKey, useSystemInfoQueryIdentity } from "./system-info-query";

export function useSystemInfo() {
  const bootId = useSystemInfoBootId();
  const signal = useSystemInfoRequestSignal();
  const identity = useSystemInfoQueryIdentity(bootId);
  const query = useQuery({
    queryKey: createSystemInfoQueryKey(identity),
    queryFn: () =>
      fetchSystemInfo({
        baseUrl: identity.apiBaseUrl,
        cache: "no-store",
        init: { signal },
      }),
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });

  const reload = useCallback(async () => {
    await query.refetch();
  }, [query.refetch]);

  return {
    info: query.data ?? null,
    isLoading: query.isFetching,
    error: query.error ? errorMessage(query.error) : null,
    reload,
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
