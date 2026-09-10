"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { fetchRetentionStatus, saveRetentionSettings } from "@/lib/api/domains/system-api";
import type { RetentionSettings } from "@/lib/types/system";

export function useRetentionSettings() {
  const status = useAppStore((s) => s.system.retention);
  const setStatus = useAppStore((s) => s.setSystemRetention);
  const storeApi = useAppStoreApi();
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
        // Apply the PUT's own normalized response synchronously, rather
        // than relying solely on the reload() below: if that GET fails,
        // its error is recorded but never surfaces once status is already
        // loaded (see the isLoading/error-gated branches in
        // RetentionSettingsCard), which would otherwise leave the store
        // holding pre-save settings while the save coordinator believes
        // the save already succeeded.
        storeApi.setState((state) =>
          state.system.retention
            ? {
                system: {
                  ...state.system,
                  retention: { ...state.system.retention, settings: saved },
                },
              }
            : state,
        );
        void reload();
        return saved;
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        setSaveError(message);
        throw e;
      }
    },
    [reload, storeApi],
  );

  return { status, isLoading, error, saveError, reload, save };
}
