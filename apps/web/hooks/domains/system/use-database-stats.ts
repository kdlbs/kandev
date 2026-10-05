"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useSystemInfoBootId } from "@/components/system-info-query-provider";
import { fetchDatabaseStats, retryDatabaseStats } from "@/lib/api/domains/system-api";
import type { DatabaseStats } from "@/lib/types/system";
import { createDatabaseStatsQueryKey, useSystemInfoQueryIdentity } from "./system-info-query";

const DATABASE_STATS_TTL_MS = 15 * 60 * 1_000;
const DATABASE_STATS_POLL_INTERVAL_MS = 2 * 1_000;
const DATABASE_STATS_RETRY_INTERVAL_MS = 30 * 1_000;

function nextRefreshDelay(
  database: DatabaseStats | undefined,
  isFetching: boolean,
  error: unknown,
): number | false {
  if (isFetching) return false;
  if (error) return DATABASE_STATS_RETRY_INTERVAL_MS;
  if (!database) return DATABASE_STATS_RETRY_INTERVAL_MS;

  switch (database.logical_stats_state) {
    case "pending":
    case "refreshing":
      return DATABASE_STATS_POLL_INTERVAL_MS;
    case "stale":
    case "unavailable":
      return DATABASE_STATS_RETRY_INTERVAL_MS;
    case "ready": {
      const measuredAt = Date.parse(database.logical_stats_measured_at ?? "");
      if (!Number.isFinite(measuredAt)) return DATABASE_STATS_RETRY_INTERVAL_MS;
      const untilExpiry = measuredAt + DATABASE_STATS_TTL_MS - Date.now();
      return untilExpiry > 0 ? untilExpiry : DATABASE_STATS_RETRY_INTERVAL_MS;
    }
  }
}

export type DatabaseStatsQueryResult = {
  database: DatabaseStats | null;
  isLoading: boolean;
  error: string | null;
  reload: () => Promise<void>;
  retry: () => Promise<void>;
};

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function useDatabaseStats(): DatabaseStatsQueryResult {
  const bootId = useSystemInfoBootId();
  const identity = useSystemInfoQueryIdentity(bootId);
  const query = useQuery({
    queryKey: createDatabaseStatsQueryKey(identity),
    queryFn: ({ signal }) =>
      fetchDatabaseStats({
        baseUrl: identity.apiBaseUrl,
        cache: "no-store",
        init: { signal },
      }),
    staleTime: 0,
    gcTime: Infinity,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    refetchIntervalInBackground: true,
    refetchInterval: (currentQuery) =>
      nextRefreshDelay(
        currentQuery.state.data,
        currentQuery.state.fetchStatus === "fetching",
        currentQuery.state.error,
      ),
    networkMode: "always",
    retry: false,
  });
  const retryMutation = useMutation({
    mutationFn: () => retryDatabaseStats({ baseUrl: identity.apiBaseUrl, cache: "no-store" }),
    networkMode: "always",
    retry: false,
  });
  const [retryError, setRetryError] = useState<unknown>(null);
  const retryErrorDataUpdatedAt = useRef<number | null>(null);

  useEffect(() => {
    if (
      retryError !== null &&
      retryErrorDataUpdatedAt.current !== null &&
      query.dataUpdatedAt !== retryErrorDataUpdatedAt.current
    ) {
      setRetryError(null);
      retryErrorDataUpdatedAt.current = null;
    }
  }, [query.dataUpdatedAt, retryError]);

  const reload = useCallback(async () => {
    setRetryError(null);
    retryErrorDataUpdatedAt.current = null;
    await query.refetch({ throwOnError: false });
  }, [query.refetch]);

  const retry = useCallback(async () => {
    setRetryError(null);
    retryErrorDataUpdatedAt.current = null;
    try {
      await retryMutation.mutateAsync();
      await query.refetch({ throwOnError: false });
    } catch (error) {
      retryErrorDataUpdatedAt.current = query.dataUpdatedAt;
      setRetryError(error);
    }
  }, [query.dataUpdatedAt, query.refetch, retryMutation.mutateAsync]);

  const isLoading = query.isFetching || retryMutation.isPending;
  const visibleError = isLoading ? null : (retryError ?? query.error);

  return {
    database: query.data ?? null,
    isLoading,
    error: visibleError === null ? null : errorMessage(visibleError),
    reload,
    retry,
  };
}
