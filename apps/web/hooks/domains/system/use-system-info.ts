"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchSystemInfo } from "@/lib/api/domains/system-api";

/**
 * Fetches `/api/v1/system/info` once on mount and exposes the cached value
 * from the store. The endpoint is read-only build metadata so a single
 * fetch is sufficient.
 */
export function useSystemInfo() {
  const info = useAppStore((s) => s.system.info);
  const setSystemInfo = useAppStore((s) => s.setSystemInfo);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchSystemInfo({ cache: "no-store" });
      setSystemInfo(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setSystemInfo]);

  useEffect(() => {
    if (info) return;
    void reload();
  }, [info, reload]);

  return { info, isLoading, error, reload };
}
