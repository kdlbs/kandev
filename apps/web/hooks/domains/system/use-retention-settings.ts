"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchRetentionStatus, saveRetentionSettings } from "@/lib/api/domains/system-api";
import type { RetentionSettings } from "@/lib/types/system";

export function useRetentionSettings() {
  const status = useAppStore((s) => s.system.retention);
  const setStatus = useAppStore((s) => s.setSystemRetention);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      setStatus(await fetchRetentionStatus({ cache: "no-store" }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setStatus]);

  useEffect(() => {
    if (status) return;
    void reload();
  }, [status, reload]);

  const save = useCallback(
    async (settings: RetentionSettings) => {
      setSaveError(null);
      try {
        const saved = await saveRetentionSettings(settings);
        await reload();
        return saved;
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        setSaveError(message);
        throw e;
      }
    },
    [reload],
  );

  return { status, isLoading, error, saveError, reload, save };
}
