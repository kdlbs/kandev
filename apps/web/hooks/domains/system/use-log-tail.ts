"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchLogTail } from "@/lib/api/domains/system-api";

/**
 * Fetches the last `n` lines of the current lumberjack log. The Logs page
 * also exposes a Refresh button which re-invokes `reload()`.
 */
export function useLogTail(n = 1000) {
  const tail = useAppStore((s) => s.system.logs.tail);
  const tailLoaded = useAppStore((s) => s.system.logs.tailLoaded);
  const setSystemLogTail = useAppStore((s) => s.setSystemLogTail);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchLogTail(n);
      setSystemLogTail(res?.lines ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [n, setSystemLogTail]);

  useEffect(() => {
    if (tailLoaded) return;
    void reload();
  }, [tailLoaded, reload]);

  return { tail, loaded: tailLoaded, isLoading, error, reload };
}
