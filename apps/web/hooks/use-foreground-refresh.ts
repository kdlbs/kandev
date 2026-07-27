import { useCallback, useEffect, useRef } from "react";

const FOREGROUND_EVENT_COALESCE_MS = 250;

type Refresh = () => void | Promise<void>;

/**
 * Runs an authoritative refresh when a browser surface returns to the
 * foreground. Browsers commonly emit focus, visibilitychange, pageshow, and
 * online together; one return should cause at most one request per consumer.
 */
export function useForegroundRefresh(refresh: Refresh, enabled = true, scopeKey?: unknown) {
  const refreshRef = useRef(refresh);
  const inFlightRef = useRef<Promise<void> | null>(null);
  const lastRunAtRef = useRef(-Infinity);

  useEffect(() => {
    refreshRef.current = refresh;
  }, [refresh]);

  useEffect(() => {
    lastRunAtRef.current = -Infinity;
    inFlightRef.current = null;
  }, [scopeKey]);

  const run = useCallback(() => {
    if (!enabled || document.visibilityState !== "visible") return;
    if (inFlightRef.current) return;

    const now = Date.now();
    if (now - lastRunAtRef.current < FOREGROUND_EVENT_COALESCE_MS) return;
    lastRunAtRef.current = now;

    let request: Promise<void>;
    try {
      request = Promise.resolve(refreshRef.current());
    } catch {
      request = Promise.resolve();
    }
    const pending = request
      .catch(() => undefined)
      .finally(() => {
        if (inFlightRef.current === pending) inFlightRef.current = null;
      });
    inFlightRef.current = pending;
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") run();
    };

    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("focus", run);
    window.addEventListener("pageshow", run);
    window.addEventListener("online", run);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("focus", run);
      window.removeEventListener("pageshow", run);
      window.removeEventListener("online", run);
    };
  }, [enabled, run]);

  return run;
}
