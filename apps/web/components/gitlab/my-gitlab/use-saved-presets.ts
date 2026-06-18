"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";

const STORAGE_KEY = "kandev:gitlab-presets:v1";
const MIGRATED_KEY = "kandev:gitlab-presets:migrated-to-backend:v1";

export type SavedPreset = {
  id: string;
  kind: "mr" | "issue";
  label: string;
  customQuery: string;
  projectFilter: string;
  createdAt: string;
};

export function readStorage(): SavedPreset[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isSavedPreset);
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

function isSavedPreset(p: unknown): p is SavedPreset {
  return (
    typeof p === "object" &&
    p !== null &&
    typeof (p as SavedPreset).id === "string" &&
    ((p as SavedPreset).kind === "mr" || (p as SavedPreset).kind === "issue") &&
    typeof (p as SavedPreset).label === "string" &&
    typeof (p as SavedPreset).customQuery === "string" &&
    typeof (p as SavedPreset).projectFilter === "string" &&
    typeof (p as SavedPreset).createdAt === "string"
  );
}

function readServerPresets(value: unknown): SavedPreset[] | null {
  if (!Array.isArray(value)) return null;
  return value.filter(isSavedPreset);
}

function syncServer(next: SavedPreset[]) {
  updateUserSettings({ gitlab_saved_presets: next }).catch(() => {});
}

function snapshotKey(value: SavedPreset[]): string {
  return JSON.stringify(value);
}

function hasMigratedToBackend(): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(MIGRATED_KEY) === "1";
}

function markMigratedToBackend(): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(MIGRATED_KEY, "1");
  } catch {
    /* ignore storage failures */
  }
}

// Test-only: drop the module-level snapshot so the next read goes through
// readStorage again. Used by the hook tests so each `it` starts from a known
// empty state independent of test execution order.
export function __resetSnapshotForTests() {
  snapshot = null;
  for (const l of listeners) l();
}

export function useSavedPresets() {
  const presets = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  useEffect(() => {
    let cancelled = false;
    const initialKey = snapshotKey(readStorage());
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverPresets = readServerPresets(response.settings.gitlab_saved_presets);
        if (cancelled || !serverPresets) return;
        const local = readStorage();
        if (snapshotKey(local) !== initialKey) return;
        if (serverPresets.length === 0 && local.length > 0 && !hasMigratedToBackend()) {
          syncServer(local);
          markMigratedToBackend();
          return;
        }
        publish(serverPresets);
        markMigratedToBackend();
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  // Always merge against the latest localStorage read instead of the in-memory
  // snapshot. With two tabs open, snapshot in tab A is stale the moment tab B
  // writes — appending to it would silently drop B's preset. readStorage is
  // cheap (small JSON, single key) so this is fine on the save/remove paths.
  const save = useCallback((input: Omit<SavedPreset, "id" | "createdAt">) => {
    const preset: SavedPreset = {
      ...input,
      id: `g_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      createdAt: new Date().toISOString(),
    };
    const next = [...readStorage(), preset];
    publish(next);
    syncServer(next);
    markMigratedToBackend();
    return preset;
  }, []);

  const remove = useCallback((id: string) => {
    const next = readStorage().filter((p) => p.id !== id);
    publish(next);
    syncServer(next);
    markMigratedToBackend();
  }, []);

  return { presets, save, remove };
}
