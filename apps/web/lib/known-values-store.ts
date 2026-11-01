"use client";

import { useEffect, useSyncExternalStore } from "react";

/** Each instance accumulates filter choices only within its current query context. */
export function createKnownValuesStore() {
  let currentKey: string | null = null;
  const seen = new Set<string>();
  let snapshot: string[] = [];
  const emptySnapshot: string[] = [];
  const listeners = new Set<() => void>();

  function subscribe(cb: () => void) {
    listeners.add(cb);
    return () => {
      listeners.delete(cb);
    };
  }

  function getSnapshot(): string[] {
    return snapshot;
  }

  function getServerSnapshot(): string[] {
    return emptySnapshot;
  }

  function recordForKey(key: string, values: readonly string[]) {
    let changed = false;
    if (currentKey !== key) {
      currentKey = key;
      if (seen.size > 0) {
        seen.clear();
        snapshot = [];
        changed = true;
      }
    }
    for (const value of values) {
      if (value && !seen.has(value)) {
        seen.add(value);
        changed = true;
      }
    }
    if (!changed) return;
    snapshot = Array.from(seen).sort();
    for (const l of listeners) l();
  }

  function reset() {
    currentKey = null;
    if (seen.size === 0 && snapshot.length === 0) return;
    seen.clear();
    snapshot = [];
    for (const l of listeners) l();
  }

  function useKnownValues(resetKey: string, fromItems: readonly string[]): string[] {
    useEffect(() => {
      recordForKey(resetKey, fromItems);
    }, [resetKey, fromItems]);
    return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  }

  return { recordForKey, reset, useKnownValues, getSnapshot };
}
