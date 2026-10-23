"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { fetchRetentionStatus, saveRetentionSettings } from "@/lib/api/domains/system-api";
import type { RetentionSettings } from "@/lib/types/system";

type ReadLifetime = { read: symbol | null };

// Readers share publication authority because route instances share the app store.
const currentReads = new WeakMap<object, symbol>();

export function useRetentionSettings() {
  const status = useAppStore((s) => s.system.retention);
  const setStatus = useAppStore((s) => s.setSystemRetention);
  const storeApi = useAppStoreApi();
  const lifetime = useRef<ReadLifetime | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  useLayoutEffect(() => {
    const owner: ReadLifetime = { read: null };
    lifetime.current = owner;
    return () => {
      lifetime.current = null;
      if (currentReads.get(storeApi) === owner.read) currentReads.delete(storeApi);
    };
  }, [storeApi]);

  const reload = useCallback(async () => {
    const owner = lifetime.current;
    if (!owner) return;
    const read = Symbol();
    owner.read = read;
    currentReads.set(storeApi, read);
    const isLocal = () => lifetime.current === owner && owner.read === read;
    const isCurrent = () => isLocal() && currentReads.get(storeApi) === read;
    setIsLoading(true);
    setError(null);
    try {
      const next = await fetchRetentionStatus({ cache: "no-store" });
      if (isCurrent()) setStatus(next);
    } catch (e) {
      if (isCurrent()) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (isLocal()) setIsLoading(false);
      if (currentReads.get(storeApi) === read) currentReads.delete(storeApi);
    }
  }, [setStatus, storeApi]);

  useEffect(() => {
    if (status) return;
    void reload();
  }, [status, reload]);

  const save = useCallback(
    async (settings: RetentionSettings) => {
      const owner = lifetime.current;
      setSaveError(null);
      try {
        const saved = await saveRetentionSettings(settings);
        // An admitted write remains authoritative after its view leaves.
        currentReads.delete(storeApi);
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
        if (owner && lifetime.current === owner) void reload();
        return saved;
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        if (owner && lifetime.current === owner) setSaveError(message);
        throw e;
      }
    },
    [reload, storeApi],
  );

  return { status, isLoading, error, saveError, reload, save };
}
