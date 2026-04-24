"use client";

import { useEffect, useSyncExternalStore } from "react";

// Accumulator of repos seen in the *current* query context. Scoped by a
// reset key (kind + selection + custom query) so repos from one preset don't
// bleed into another. Within a single context it only grows, so narrowing
// the repo filter doesn't hide the remaining options.
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

export function recordForKey(key: string, repos: readonly string[]) {
  let changed = false;
  if (currentKey !== key) {
    currentKey = key;
    if (seen.size > 0) {
      seen.clear();
      snapshot = [];
      changed = true;
    }
  }
  for (const r of repos) {
    if (r && !seen.has(r)) {
      seen.add(r);
      changed = true;
    }
  }
  if (!changed) return;
  snapshot = Array.from(seen).sort();
  for (const l of listeners) l();
}

export function useKnownRepos(resetKey: string, fromItems: readonly string[]): string[] {
  useEffect(() => {
    recordForKey(resetKey, fromItems);
  }, [resetKey, fromItems]);
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

// Test-only: read the current module-level snapshot.
export function __getSnapshotForTests(): string[] {
  return snapshot;
}
