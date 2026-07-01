"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { backupsQueryOptions } from "@/lib/query/query-options/system";
import type { SnapshotInfo } from "@/lib/types/system";

export function useBackups() {
  const query = useQuery(backupsQueryOptions());
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async (): Promise<SnapshotInfo[]> => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await query.refetch();
      if (res.error) throw res.error;
      return res.data ?? [];
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      setIsLoading(false);
    }
  }, [query]);

  useEffect(() => {
    if (!query.error) return;
    setError(query.error instanceof Error ? query.error.message : String(query.error));
  }, [query.error]);

  return {
    backups: query.data ?? [],
    loaded: query.isSuccess,
    isLoading: isLoading || (query.isFetching && !query.isSuccess),
    error,
    reload,
  };
}
