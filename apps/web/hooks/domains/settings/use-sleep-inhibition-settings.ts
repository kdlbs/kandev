"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  fetchSleepInhibitionSettings,
  updateSleepInhibitionSettings,
} from "@/lib/api/domains/settings-api";
import type { SleepInhibitionSettings } from "@/lib/types/system";

const STATUS_POLL_INTERVAL_MS = 15_000;

/**
 * Loads the install-wide setting into the settings domain and refreshes the
 * runtime status while the page is open. The response is separate from the
 * user-settings slice because it belongs to the backend host, not a user.
 */
export function useSleepInhibitionSettings() {
  const response = useAppStore((state) => state.sleepInhibition.response);
  const loaded = useAppStore((state) => state.sleepInhibition.loaded);
  const loading = useAppStore((state) => state.sleepInhibition.loading);
  const error = useAppStore((state) => state.sleepInhibition.error);
  const setResponse = useAppStore((state) => state.setSleepInhibition);
  const setLoading = useAppStore((state) => state.setSleepInhibitionLoading);
  const setError = useAppStore((state) => state.setSleepInhibitionError);
  const inFlight = useRef(false);

  const refresh = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    setLoading(true);
    setError(false);
    try {
      setResponse(await fetchSleepInhibitionSettings());
    } catch {
      setError(true);
    } finally {
      inFlight.current = false;
      setLoading(false);
    }
  }, [setError, setLoading, setResponse]);

  useEffect(() => {
    if (!loaded) void refresh();
  }, [loaded, refresh]);

  useEffect(() => {
    if (!loaded) return;
    const poll = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    const interval = window.setInterval(poll, STATUS_POLL_INTERVAL_MS);
    document.addEventListener("visibilitychange", poll);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", poll);
    };
  }, [loaded, refresh]);

  const save = useCallback(
    async (settings: SleepInhibitionSettings) => {
      const next = await updateSleepInhibitionSettings(settings);
      setResponse(next);
      return next;
    },
    [setResponse],
  );

  return { response, loaded, loading, error, refresh, save };
}
