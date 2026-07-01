"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { systemInfoQueryOptions } from "@/lib/query/query-options/system";

/**
 * Fetches `/api/v1/system/info` once on mount. The endpoint is read-only
 * build metadata so a single fetch is sufficient.
 */
export function useSystemInfo() {
  const query = useQuery(systemInfoQueryOptions());
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      await query.refetch();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [query]);

  useEffect(() => {
    if (!query.error) return;
    setError(query.error instanceof Error ? query.error.message : String(query.error));
  }, [query.error]);

  return {
    info: query.data ?? null,
    isLoading: isLoading || (query.isFetching && !query.isSuccess),
    error,
    reload,
  };
}
