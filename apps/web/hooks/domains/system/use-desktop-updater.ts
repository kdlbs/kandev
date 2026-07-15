"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { desktopUpdater } from "@/lib/desktop/updater-client";
import type { DesktopUpdateState } from "@/lib/desktop/protocol";
import type { DesktopUpdaterAdapter } from "@/lib/desktop/updater-adapter";

// Tauri availability is fixed when the document loads, so it needs no subscription.
const subscribeAvailability = () => () => undefined;
const ACTIVE_POLL_INTERVAL_MS = 250;
const STABLE_POLL_INTERVAL_MS = 5_000;

export type DesktopUpdaterController = {
  available: boolean;
  state: DesktopUpdateState | null;
  checking: boolean;
  installing: boolean;
  error: string | null;
  check: () => Promise<void>;
  install: () => Promise<void>;
};

export function useDesktopUpdater(
  adapter: DesktopUpdaterAdapter = desktopUpdater,
): DesktopUpdaterController {
  const available = useSyncExternalStore(subscribeAvailability, adapter.isAvailable, () => false);
  const [state, setState] = useState<DesktopUpdateState | null>(null);
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);
  const busyRef = useRef(false);
  const stateRevisionRef = useRef(0);

  const refresh = useCallback(async () => {
    if (!adapter.isAvailable()) return;
    const revision = ++stateRevisionRef.current;
    const nextState = await adapter.getState();
    if (revision === stateRevisionRef.current) setState(nextState);
  }, [adapter]);

  useEffect(() => {
    if (!available) return;
    void refresh().catch((error: unknown) => setLocalError(message(error)));
  }, [available, refresh]);

  useEffect(() => {
    if (!available) return;
    const active =
      checking ||
      installing ||
      state?.phase === "checking" ||
      state?.phase === "downloading" ||
      state?.phase === "installing";
    const intervalMs = active ? ACTIVE_POLL_INTERVAL_MS : STABLE_POLL_INTERVAL_MS;
    let poll: number | undefined;

    const startPolling = () => {
      if (document.visibilityState !== "visible") return;
      poll = window.setInterval(() => void refresh().catch(() => undefined), intervalMs);
    };
    const stopPolling = () => {
      if (poll !== undefined) window.clearInterval(poll);
      poll = undefined;
    };
    const onVisibilityChange = () => {
      stopPolling();
      if (document.visibilityState !== "visible") return;
      void refresh().catch(() => undefined);
      startPolling();
    };

    startPolling();
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      stopPolling();
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [available, checking, installing, refresh, state?.phase]);

  const check = useCallback(async () => {
    if (busyRef.current) return;
    busyRef.current = true;
    const revision = ++stateRevisionRef.current;
    setChecking(true);
    setLocalError(null);
    try {
      const nextState = await adapter.checkForUpdates();
      if (revision === stateRevisionRef.current) setState(nextState);
    } catch (error) {
      setLocalError(message(error));
      await refresh().catch(() => undefined);
      throw error;
    } finally {
      busyRef.current = false;
      setChecking(false);
    }
  }, [adapter, refresh]);

  const install = useCallback(async () => {
    if (busyRef.current) return;
    busyRef.current = true;
    const revision = ++stateRevisionRef.current;
    setInstalling(true);
    setLocalError(null);
    try {
      const nextState = await adapter.installUpdate();
      if (revision === stateRevisionRef.current) setState(nextState);
    } catch (error) {
      setLocalError(message(error));
      await refresh().catch(() => undefined);
      throw error;
    } finally {
      busyRef.current = false;
      setInstalling(false);
    }
  }, [adapter, refresh]);

  return {
    available,
    state,
    checking,
    installing,
    error: localError ?? state?.error ?? null,
    check,
    install,
  };
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : "Desktop update failed";
}
