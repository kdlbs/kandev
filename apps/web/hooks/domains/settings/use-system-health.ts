"use client";

import { useCallback, useEffect } from "react";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";

const HEALTH_POLL_INTERVAL_MS = 5 * 60 * 1000;

export function useSystemHealth() {
  const issues = useAppStore((state) => state.systemHealth.issues);
  const checks = useAppStore((state) => state.systemHealth.checks);
  const healthy = useAppStore((state) => state.systemHealth.healthy);
  const loaded = useAppStore((state) => state.systemHealth.loaded);
  const loading = useAppStore((state) => state.systemHealth.loading);
  const setSystemHealth = useAppStore((state) => state.setSystemHealth);
  const setSystemHealthLoading = useAppStore((state) => state.setSystemHealthLoading);
  const storeApi = useAppStoreApi();

  const fetchHealth = useCallback(() => {
    if (storeApi.getState().systemHealth.loading) return;
    setSystemHealthLoading(true);
    fetchSystemHealth({ cache: "no-store" })
      .then((response) => {
        setSystemHealth(response ?? { healthy: true, issues: [], checks: [] });
      })
      .catch(() => {
        setSystemHealth({ healthy: true, issues: [], checks: [] });
      })
      .finally(() => {
        setSystemHealthLoading(false);
      });
  }, [storeApi, setSystemHealth, setSystemHealthLoading]);

  useEffect(() => {
    if (!loaded && !loading) {
      fetchHealth();
    }
  }, [loaded, loading, fetchHealth]);

  useEffect(() => {
    if (!loaded) return;
    const handleVisibility = () => {
      if (document.visibilityState === "visible") {
        fetchHealth();
      }
    };
    document.addEventListener("visibilitychange", handleVisibility);
    const id = setInterval(fetchHealth, HEALTH_POLL_INTERVAL_MS);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      clearInterval(id);
    };
  }, [loaded, fetchHealth]);

  return { issues, checks, healthy, loaded, loading };
}
