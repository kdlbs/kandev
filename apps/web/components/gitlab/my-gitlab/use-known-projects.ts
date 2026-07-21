"use client";

import { useEffect, useSyncExternalStore } from "react";

// Module-level accumulator: project_paths seen in the current query context.
// Scoped by a reset key (kind + selection + custom query). Within a context
// the set only grows so narrowing the project filter doesn't hide the rest of
// the options the user already saw.
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

export function recordForKey(key: string, projects: readonly string[]) {
  let changed = false;
  if (currentKey !== key) {
    currentKey = key;
    if (seen.size > 0) {
      seen.clear();
      snapshot = [];
      changed = true;
    }
  }
  for (const p of projects) {
    if (p && !seen.has(p)) {
      seen.add(p);
      changed = true;
    }
  }
  if (!changed) return;
  snapshot = Array.from(seen).sort();
  for (const l of listeners) l();
}

export function resetKnownProjectsStore() {
  currentKey = null;
  if (seen.size === 0 && snapshot.length === 0) return;
  seen.clear();
  snapshot = [];
  for (const l of listeners) l();
}

export function useKnownProjects(resetKey: string, fromItems: readonly string[]): string[] {
  useEffect(() => {
    recordForKey(resetKey, fromItems);
  }, [resetKey, fromItems]);
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function __getSnapshotForTests(): string[] {
  return snapshot;
}
