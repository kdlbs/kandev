"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchDatabaseStats } from "@/lib/api/domains/system-api";

export function useDatabaseStats() {
  const database = useAppStore((s) => s.system.database);
  const setSystemDatabase = useAppStore((s) => s.setSystemDatabase);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchDatabaseStats({ cache: "no-store" });
      setSystemDatabase(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setSystemDatabase]);

  useEffect(() => {
    if (database) return;
    void reload();
  }, [database, reload]);

  return { database, isLoading, error, reload };
}
