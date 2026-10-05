"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
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

type RetryError = {
  identityGeneration: number;
  dataUpdatedAt: number;
  error: unknown;
};

type IdentityScope = { identityKey: string; generation: number };
type IdentityScopeRef = { current: IdentityScope };

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function useDatabaseStatsIdentityGeneration(identityKey: string, onIdentityChange: () => void) {
  const identityScopeRef = useRef<IdentityScope>({ identityKey, generation: 0 });
  const [generation, setGeneration] = useState(0);

  useLayoutEffect(() => {
    if (identityScopeRef.current.identityKey === identityKey) return;
    const nextGeneration = identityScopeRef.current.generation + 1;
    identityScopeRef.current = { identityKey, generation: nextGeneration };
    setGeneration(nextGeneration);
    onIdentityChange();
  }, [identityKey, onIdentityChange]);

  return { generation, identityScopeRef };
}

function isCurrentIdentity(
  identityScopeRef: IdentityScopeRef,
  identityKey: string,
  generation: number,
): boolean {
  return (
    identityScopeRef.current.identityKey === identityKey &&
    identityScopeRef.current.generation === generation
  );
}

export function useDatabaseStats(): DatabaseStatsQueryResult {
  const bootId = useSystemInfoBootId();
  const identity = useSystemInfoQueryIdentity(bootId);
  const queryKey = createDatabaseStatsQueryKey(identity);
  const identityKey = JSON.stringify(queryKey.slice(2));
  const [retryError, setRetryError] = useState<RetryError | null>(null);
  const clearRetryError = useCallback(() => setRetryError(null), []);
  const { generation: identityGeneration, identityScopeRef } = useDatabaseStatsIdentityGeneration(
    identityKey,
    clearRetryError,
  );
  const query = useQuery({
    queryKey,
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
    mutationKey: [...queryKey, "refresh", identityGeneration],
    mutationFn: () => retryDatabaseStats({ baseUrl: identity.apiBaseUrl, cache: "no-store" }),
    networkMode: "always",
    retry: false,
  });
  const dataUpdatedAtRef = useRef(query.dataUpdatedAt);

  useEffect(() => {
    dataUpdatedAtRef.current = query.dataUpdatedAt;
  }, [query.dataUpdatedAt]);

  useEffect(() => {
    if (
      retryError !== null &&
      (retryError.identityGeneration !== identityGeneration ||
        retryError.dataUpdatedAt !== query.dataUpdatedAt)
    ) {
      setRetryError(null);
    }
  }, [identityGeneration, query.dataUpdatedAt, retryError]);

  const reload = useCallback(async () => {
    setRetryError(null);
    await query.refetch({ throwOnError: false });
  }, [query.refetch]);

  const retry = useCallback(async () => {
    const requestIdentityGeneration = identityGeneration;
    const requestIdentityKey = identityKey;
    setRetryError(null);
    try {
      await retryMutation.mutateAsync();
      if (!isCurrentIdentity(identityScopeRef, requestIdentityKey, requestIdentityGeneration))
        return;
      await query.refetch({ throwOnError: false });
    } catch (error) {
      if (isCurrentIdentity(identityScopeRef, requestIdentityKey, requestIdentityGeneration)) {
        setRetryError({
          identityGeneration: requestIdentityGeneration,
          dataUpdatedAt: dataUpdatedAtRef.current,
          error,
        });
      }
    }
  }, [identityGeneration, identityKey, query.refetch, retryMutation.mutateAsync]);

  const isLoading = query.isFetching || retryMutation.isPending;
  const visibleRetryError =
    retryError?.identityGeneration === identityGeneration ? retryError.error : null;
  const visibleError = isLoading ? null : (visibleRetryError ?? query.error);

  return {
    database: query.data ?? null,
    isLoading,
    error: visibleError === null ? null : errorMessage(visibleError),
    reload,
    retry,
  };
}
