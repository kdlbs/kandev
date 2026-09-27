"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchDatabaseStats } from "@/lib/api/domains/system-api";
import type { DatabaseStats } from "@/lib/types/system";

const DATABASE_STATS_TTL_MS = 15 * 60 * 1_000;
const DATABASE_STATS_POLL_INTERVAL_MS = 2 * 1_000;
const DATABASE_STATS_RETRY_INTERVAL_MS = 30 * 1_000;

function nextRefreshDelay(
  database: DatabaseStats | null,
  isLoading: boolean,
  error: string | null,
) {
  if (isLoading) return null;
  if (!database) return error ? DATABASE_STATS_RETRY_INTERVAL_MS : null;

  switch (database.logical_stats_state) {
    case "pending":
    case "refreshing":
      return DATABASE_STATS_POLL_INTERVAL_MS;
    case "stale":
    case "unavailable":
      return DATABASE_STATS_RETRY_INTERVAL_MS;
    case "ready": {
      const measuredAt = Date.parse(database.logical_stats_measured_at ?? "");
      if (!Number.isFinite(measuredAt)) return DATABASE_STATS_RETRY_INTERVAL_MS;
      const untilExpiry = measuredAt + DATABASE_STATS_TTL_MS - Date.now();
      return untilExpiry > 0 ? untilExpiry : DATABASE_STATS_RETRY_INTERVAL_MS;
    }
  }
}

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
    void reload();
  }, [reload]);

  useEffect(() => {
    const delay = nextRefreshDelay(database, isLoading, error);
    if (delay === null) return;
    const timer = setTimeout(() => void reload(), delay);
    return () => clearTimeout(timer);
  }, [database, error, isLoading, reload]);

  return { database, isLoading, error, reload };
}
