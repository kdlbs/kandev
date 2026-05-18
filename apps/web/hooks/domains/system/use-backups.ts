"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchBackups } from "@/lib/api/domains/system-api";

export function useBackups() {
  const backups = useAppStore((s) => s.system.backups);
  const setSystemBackups = useAppStore((s) => s.setSystemBackups);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const items = await fetchBackups({ cache: "no-store" });
      setSystemBackups(items ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setSystemBackups]);

  useEffect(() => {
    if (backups.loaded) return;
    void reload();
  }, [backups.loaded, reload]);

  return { backups: backups.items, loaded: backups.loaded, isLoading, error, reload };
}
