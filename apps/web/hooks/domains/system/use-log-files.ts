"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchLogFiles } from "@/lib/api/domains/system-api";

export function useLogFiles() {
  const files = useAppStore((s) => s.system.logs.files);
  const setSystemLogs = useAppStore((s) => s.setSystemLogs);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetched, setFetched] = useState(false);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchLogFiles({ cache: "no-store" });
      setSystemLogs(res ?? []);
      setFetched(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setSystemLogs]);

  useEffect(() => {
    if (fetched) return;
    void reload();
  }, [fetched, reload]);

  return { files, isLoading, error, reload };
}
