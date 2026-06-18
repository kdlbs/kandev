"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";

const STORAGE_KEY = "kandev:github-presets:v1";

export type SavedPreset = {
  id: string;
  kind: "pr" | "issue";
  label: string;
  customQuery: string;
  repoFilter: string;
  createdAt: string;
};

export function readStorage(): SavedPreset[] {
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

function readServerPresets(value: unknown): SavedPreset[] | null {
  if (!Array.isArray(value)) return null;
  return value.filter(
    (p): p is SavedPreset =>
      typeof p === "object" &&
      p !== null &&
      typeof (p as SavedPreset).id === "string" &&
      ((p as SavedPreset).kind === "pr" || (p as SavedPreset).kind === "issue") &&
      typeof (p as SavedPreset).label === "string",
  );
}

function syncServer(next: SavedPreset[]) {
  updateUserSettings({ github_saved_presets: next }).catch(() => {});
}

export function useSavedPresets() {
  const presets = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  useEffect(() => {
    let cancelled = false;
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverPresets = readServerPresets(response.settings.github_saved_presets);
        if (!cancelled && serverPresets) publish(serverPresets);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const save = useCallback((input: Omit<SavedPreset, "id" | "createdAt">) => {
    const preset: SavedPreset = {
      ...input,
      id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      createdAt: new Date().toISOString(),
    };
    const next = [...(snapshot ?? readStorage()), preset];
    publish(next);
    syncServer(next);
    return preset;
  }, []);

  const remove = useCallback((id: string) => {
    const next = (snapshot ?? readStorage()).filter((p) => p.id !== id);
    publish(next);
    syncServer(next);
  }, []);

  return { presets, save, remove };
}
