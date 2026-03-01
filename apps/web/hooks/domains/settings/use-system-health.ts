"use client";

import { useEffect } from "react";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import { useAppStore } from "@/components/state-provider";

export function useSystemHealth() {
  const issues = useAppStore((state) => state.systemHealth.issues);
  const healthy = useAppStore((state) => state.systemHealth.healthy);
  const loaded = useAppStore((state) => state.systemHealth.loaded);
  const loading = useAppStore((state) => state.systemHealth.loading);
  const setSystemHealth = useAppStore((state) => state.setSystemHealth);
  const setSystemHealthLoading = useAppStore((state) => state.setSystemHealthLoading);

  useEffect(() => {
    if (loaded || loading) return;
    setSystemHealthLoading(true);
    fetchSystemHealth({ cache: "no-store" })
      .then((response) => {
        setSystemHealth(response ?? { healthy: true, issues: [] });
      })
      .catch(() => {
        setSystemHealth({ healthy: true, issues: [] });
      })
      .finally(() => {
        setSystemHealthLoading(false);
      });
  }, [loaded, loading, setSystemHealth, setSystemHealthLoading]);

  return { issues, healthy, loaded, loading };
}
