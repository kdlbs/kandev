"use client";

import { useEffect, useRef } from "react";

/**
 * How often a page with something in flight re-asks. Slow enough that a page
 * left open overnight is not a load problem, fast enough that a run finishing
 * is noticed while the reader is still looking at it.
 */
export const LIVE_REFRESH_INTERVAL_MS = 10_000;

/**
 * Re-run `refresh` on an interval, but only while `active`.
 *
 * Runs finish without anything on these pages asking, so a row that says
 * "Running" keeps saying it until the user reloads — the one state where a
 * stale reading surface is actively misleading. Polling is gated on there
 * being something open, so an idle workspace issues no requests at all.
 *
 * `refresh` is held in a ref rather than listed as a dependency: callers rebuild
 * it every render, and depending on it would tear down and restart the interval
 * each time, so it would never fire.
 */
export function useLiveRefresh(active: boolean, refresh: () => void): void {
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => refreshRef.current(), LIVE_REFRESH_INTERVAL_MS);
    return () => clearInterval(id);
  }, [active]);
}
