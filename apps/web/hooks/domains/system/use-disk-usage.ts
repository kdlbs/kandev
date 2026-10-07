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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function useDiskUsage() {
  const scope = useDiskUsageScope();
  const query = useQuery(createDiskUsageQueryOptions(scope.identity));
  const [refreshError, setRefreshError] = useState<RefreshError | null>(null);

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
      }
      return;
    }

    await reloadForScope(captured);
  }, [reloadForScope, scope.captureScope, scope.isCurrentScope]);

  let error: string | null = null;
  if (!query.isFetching) {
    if (
      refreshError?.identityKey === scope.identityKey &&
      refreshError.generation === scope.generation
    ) {
      error = refreshError.message;
    } else if (query.error) {
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
