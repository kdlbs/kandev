"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  hasUserSettingsSyncFailure,
  setUserSettingsSyncFailure,
} from "@/lib/user-settings-sync-failure";

const STORAGE_KEY = "kandev:github-presets:v1";
const MIGRATED_KEY = "kandev:github-presets:migrated-to-backend:v1";
const SYNC_FAILED_KEY = "kandev:github-presets:sync-failed:v1";

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

function syncServer(next: SavedPreset[]): Promise<void> {
  return updateUserSettings({ github_saved_presets: next })
    .then(() => {
      setUserSettingsSyncFailure(SYNC_FAILED_KEY, false);
    })
    .catch(() => {
      setUserSettingsSyncFailure(SYNC_FAILED_KEY, true);
    });
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
        const serverPresets = readServerPresets(response.settings.github_saved_presets);
        if (cancelled || !serverPresets) return;
        const local = readStorage();
        if (snapshotKey(local) !== initialKey) return;
        if (hasUserSettingsSyncFailure(SYNC_FAILED_KEY) && local.length > 0) {
          void syncServer(local);
          return;
        }
        if (serverPresets.length === 0 && local.length > 0 && !hasMigratedToBackend()) {
          void syncServer(local);
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

  const save = useCallback((input: Omit<SavedPreset, "id" | "createdAt">) => {
    const preset: SavedPreset = {
      ...input,
      id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      createdAt: new Date().toISOString(),
    };
    const next = [...readStorage(), preset];
    publish(next);
    void syncServer(next);
    markMigratedToBackend();
    return preset;
  }, []);

  const remove = useCallback((id: string) => {
    const next = readStorage().filter((p) => p.id !== id);
    publish(next);
    void syncServer(next);
    markMigratedToBackend();
  }, []);

  return { presets, save, remove };
}
