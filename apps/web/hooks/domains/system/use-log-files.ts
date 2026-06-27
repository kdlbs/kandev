"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { logFilesQueryOptions } from "@/lib/query/query-options/system";

export function useLogFiles() {
  const query = useQuery(logFilesQueryOptions());
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
    files: query.data ?? [],
    isLoading: isLoading || (query.isFetching && !query.isSuccess),
    error,
    reload,
  };
}
