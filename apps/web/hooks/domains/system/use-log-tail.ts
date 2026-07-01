"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { logTailQueryOptions } from "@/lib/query/query-options/system";

/**
 * Fetches the last `n` lines of the current lumberjack log. The Logs page
 * also exposes a Refresh button which re-invokes `reload()`.
 */
export function useLogTail(n = 1000) {
  const query = useQuery(logTailQueryOptions(n));
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
    tail: query.data?.lines ?? [],
    loaded: query.isSuccess,
    isLoading: isLoading || (query.isFetching && !query.isSuccess),
    error,
    reload,
  };
}
