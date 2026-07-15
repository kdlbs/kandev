"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";

export type SavedPreset = {
  id: string;
  kind: "mr" | "issue";
  label: string;
  customQuery: string;
  projectFilter: string;
  createdAt: string;
};

const listeners = new Set<() => void>();
const emptySnapshot: SavedPreset[] = [];
let snapshot: SavedPreset[] = [];
let snapshotVersion = 0;

function getSnapshot(): SavedPreset[] {
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
  snapshotVersion += 1;
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

const syncServer = createQueuedUserSettingsSync<SavedPreset[]>((next) => ({
  gitlab_saved_presets: next,
}));

// Test-only: isolate the module-level snapshot between hook tests.
export function __resetSnapshotForTests() {
  snapshot = [];
  snapshotVersion = 0;
  for (const l of listeners) l();
}

export function useSavedPresets() {
  const presets = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  useEffect(() => {
    let cancelled = false;
    const initialVersion = snapshotVersion;
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverPresets = readServerPresets(response.settings.gitlab_saved_presets);
        if (cancelled || !serverPresets || snapshotVersion !== initialVersion) return;
        publish(serverPresets);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const save = useCallback((input: Omit<SavedPreset, "id" | "createdAt">) => {
    const preset: SavedPreset = {
      ...input,
      id: `g_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      createdAt: new Date().toISOString(),
    };
    const next = [...getSnapshot(), preset];
    publish(next);
    void syncServer(next);
    return preset;
  }, []);

  const remove = useCallback((id: string) => {
    const next = getSnapshot().filter((p) => p.id !== id);
    publish(next);
    void syncServer(next);
  }, []);

  return { presets, save, remove };
}
