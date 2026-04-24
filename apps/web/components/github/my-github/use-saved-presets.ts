"use client";

import { useCallback, useSyncExternalStore } from "react";

const STORAGE_KEY = "kandev:github-presets:v1";

export type SavedPreset = {
  id: string;
  kind: "pr" | "issue";
  label: string;
  customQuery: string;
  repoFilter: string;
  createdAt: string;
};

function readStorage(): SavedPreset[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (p): p is SavedPreset =>
        typeof p === "object" &&
        p !== null &&
        typeof (p as SavedPreset).id === "string" &&
        ((p as SavedPreset).kind === "pr" || (p as SavedPreset).kind === "issue") &&
        typeof (p as SavedPreset).label === "string",
    );
  } catch {
    return [];
  }
}

function writeStorage(presets: SavedPreset[]) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
  } catch {
    /* ignore quota / access errors */
  }
}

// External store: single source of truth shared across all hook consumers.
const listeners = new Set<() => void>();
let snapshot: SavedPreset[] | null = null;
const emptySnapshot: SavedPreset[] = [];

function getSnapshot(): SavedPreset[] {
  if (snapshot === null) snapshot = readStorage();
  return snapshot;
}

function getServerSnapshot(): SavedPreset[] {
  return emptySnapshot;
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function publish(next: SavedPreset[]) {
  snapshot = next;
  writeStorage(next);
  for (const l of listeners) l();
}

export function useSavedPresets() {
  const presets = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const save = useCallback((input: Omit<SavedPreset, "id" | "createdAt">) => {
    const preset: SavedPreset = {
      ...input,
      id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      createdAt: new Date().toISOString(),
    };
    publish([...(snapshot ?? readStorage()), preset]);
    return preset;
  }, []);

  const remove = useCallback((id: string) => {
    publish((snapshot ?? readStorage()).filter((p) => p.id !== id));
  }, []);

  return { presets, save, remove };
}
