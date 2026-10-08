"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { refreshDiskUsage } from "@/lib/api/domains/system-api";
import {
  createDiskUsageQueryOptions,
  useDiskUsageScope,
  type DiskUsageScope,
} from "./disk-usage-query";

type RefreshError = { identityKey: string; generation: number; message: string };
type PendingRefresh = {
  token: symbol;
  identityKey: string;
  generation: number;
  previousQueryError: Error | null;
};

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function useDiskUsage() {
  const scope = useDiskUsageScope();
  const query = useQuery(createDiskUsageQueryOptions(scope.identity));
  const [refreshError, setRefreshError] = useState<RefreshError | null>(null);
  const [pendingRefresh, setPendingRefresh] = useState<PendingRefresh | null>(null);

  const pendingRefreshForScope =
    pendingRefresh?.identityKey === scope.identityKey &&
    pendingRefresh.generation === scope.generation
      ? pendingRefresh
      : null;

  useEffect(() => {
    if (query.isFetching) setRefreshError(null);
  }, [query.isFetching]);

  const reloadForScope = useCallback(
    async (captured: DiskUsageScope) => {
      if (!scope.isCurrentScope(captured)) return;
      setRefreshError(null);
      await query.refetch({ cancelRefetch: true, throwOnError: false });
    },
    [query.refetch, scope.isCurrentScope],
  );
  const reload = useCallback(
    () => reloadForScope(scope.captureScope()),
    [reloadForScope, scope.captureScope],
  );
  const refresh = useCallback(async () => {
    const captured = scope.captureScope();
    if (!scope.isCurrentScope(captured)) return;
    const token = Symbol();
    setPendingRefresh({
      token,
      identityKey: captured.identityKey,
      generation: captured.generation,
      previousQueryError: query.error,
    });
    setRefreshError(null);

    try {
      await refreshDiskUsage({ baseUrl: captured.identity.apiBaseUrl });
    } catch (error) {
      if (scope.isCurrentScope(captured)) {
        setRefreshError({
          identityKey: captured.identityKey,
          generation: captured.generation,
          message: errorMessage(error),
        });
        setPendingRefresh((pending) => (pending?.token === token ? null : pending));
      }
      return;
    }

    try {
      await reloadForScope(captured);
    } finally {
      if (scope.isCurrentScope(captured)) {
        setPendingRefresh((pending) => (pending?.token === token ? null : pending));
      }
    }
  }, [query.error, reloadForScope, scope.captureScope, scope.isCurrentScope]);

  let error: string | null = null;
  if (!query.isFetching) {
    if (
      refreshError?.identityKey === scope.identityKey &&
      refreshError.generation === scope.generation
    ) {
      error = refreshError.message;
    } else if (query.error && query.error !== pendingRefreshForScope?.previousQueryError) {
      error = errorMessage(query.error);
    }
  }

  return {
    diskUsage: query.data ?? null,
    isLoading: query.isFetching,
    error,
    reload,
    refresh,
  };
}
