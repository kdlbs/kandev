"use client";

import { useEffect, useState } from "react";

/**
 * Returns the current epoch ms and triggers a re-render every
 * `intervalMs`. Use for "live" labels (durations on RUNNING sessions,
 * relative-time strings like "just now") that have no event source —
 * the WS pipeline pushes data changes, but elapsed time itself only
 * moves with the wall clock.
 *
 * Default 1000ms suits second-granularity duration counters. Pass
 * 30_000 for "Xm ago" labels where finer ticks are wasted work.
 */
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState<number>(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}
