"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { systemHealthQueryOptions } from "@/lib/query/query-options/settings";

export function useSystemHealth() {
  const query = useQuery(systemHealthQueryOptions());
  const storeIssues = useAppStore((state) => state.systemHealth.issues);
  const storeChecks = useAppStore((state) => state.systemHealth.checks);
  const storeHealthy = useAppStore((state) => state.systemHealth.healthy);
  const loaded = useAppStore((state) => state.systemHealth.loaded);
  const loading = useAppStore((state) => state.systemHealth.loading);
  const setSystemHealth = useAppStore((state) => state.setSystemHealth);
  const setSystemHealthLoading = useAppStore((state) => state.setSystemHealthLoading);

  const data = query.data ?? null;

  useEffect(() => {
    setSystemHealthLoading(query.isFetching && !query.isSuccess);
  }, [query.isFetching, query.isSuccess, setSystemHealthLoading]);

  useEffect(() => {
    if (!data) return;
    setSystemHealth(data);
  }, [data, setSystemHealth]);

  useEffect(() => {
    if (!query.isError) return;
    setSystemHealth({ healthy: true, issues: [], checks: [] });
  }, [query.isError, setSystemHealth]);

  return {
    issues: data?.issues ?? storeIssues,
    checks: data?.checks ?? storeChecks,
    healthy: data?.healthy ?? storeHealthy,
    loaded: loaded || query.isSuccess,
    loading: loading || (query.isFetching && !query.isSuccess),
  };
}
