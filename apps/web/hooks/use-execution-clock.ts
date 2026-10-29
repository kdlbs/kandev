"use client";

import { useCallback, useSyncExternalStore } from "react";

const deadlines = new Map<() => void, number>();
let timer: ReturnType<typeof setTimeout> | undefined;
let now = 0;

function schedule(): void {
  clearTimeout(timer);
  timer = undefined;
  const current = Date.now();
  const future = [...deadlines.values()].filter((deadline) => deadline > current);
  if (future.length === 0) return;
  const delay = Math.min(Math.min(...future) - current, 2_147_483_647);
  timer = setTimeout(() => {
    now = Date.now();
    for (const notify of [...deadlines.keys()]) notify();
    schedule();
  }, delay);
}

function subscribe(listener: () => void, deadline: number): () => void {
  deadlines.set(listener, deadline);
  now = Date.now();
  schedule();
  return () => {
    deadlines.delete(listener);
    schedule();
  };
}

const snapshot = () => now;
const inactiveSnapshot = () => 0;

/** One timer expires evidence at the earliest visible consumer's exact deadline. */
export function useExecutionClock(deadline: number | null): number {
  const valid = deadline !== null && Number.isFinite(deadline);
  const listen = useCallback(
    (listener: () => void) => (valid ? subscribe(listener, deadline!) : () => {}),
    [valid, deadline],
  );
  return useSyncExternalStore(listen, valid ? snapshot : inactiveSnapshot, inactiveSnapshot);
}
