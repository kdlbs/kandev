"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { checkUpdates } from "@/lib/api/domains/system-api";
import { qk } from "@/lib/query/keys";
import { updatesQueryOptions } from "@/lib/query/query-options/system";

export function useUpdates() {
  const queryClient = useQueryClient();
  const query = useQuery(updatesQueryOptions());
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);

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

  /**
   * Triggers a server-side re-poll of the GitHub releases endpoint. The
   * backend rate-limits this per-process to one call per 30s and replies
   * with the fresh row (or 429 — surfaced via the returned promise).
   */
  const check = useCallback(async () => {
    setIsChecking(true);
    setError(null);
    try {
      const res = await checkUpdates();
      queryClient.setQueryData(qk.system.updates(), res);
      return res;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      setIsChecking(false);
    }
  }, [queryClient]);

  useEffect(() => {
    if (!query.error) return;
    setError(query.error instanceof Error ? query.error.message : String(query.error));
  }, [query.error]);

  return {
    updates: query.data ?? null,
    isLoading: isLoading || (query.isFetching && !query.isSuccess),
    isChecking,
    error,
    reload,
    check,
  };
}
