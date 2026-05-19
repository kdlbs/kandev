"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchBackups } from "@/lib/api/domains/system-api";
import type { SnapshotInfo } from "@/lib/types/system";

export function useBackups() {
  const backups = useAppStore((s) => s.system.backups);
  const setSystemBackups = useAppStore((s) => s.setSystemBackups);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async (): Promise<SnapshotInfo[]> => {
    setIsLoading(true);
    setError(null);
    try {
      const items = await fetchBackups({ cache: "no-store" });
      const nextItems = items ?? [];
      setSystemBackups(nextItems);
      return nextItems;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      setIsLoading(false);
    }
  }, [setSystemBackups]);

  useEffect(() => {
    if (backups.loaded) return;
    void reload().catch(() => undefined);
  }, [backups.loaded, reload]);

  return { backups: backups.items, loaded: backups.loaded, isLoading, error, reload };
}
