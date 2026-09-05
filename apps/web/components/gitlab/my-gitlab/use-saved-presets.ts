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
  // Milestone title in effect when the query was saved, and the sidebar
  // preset value it was saved under. Both "" when none. Presets written
  // before this feature carry neither key; isSavedPreset tolerates a missing
  // or non-string value for either and normalizeSavedPreset reads it as "".
  milestone: string;
  preset: string;
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

// Checks the fields every SavedPreset has always had. milestone/preset are
// deliberately not checked here: a preset saved before this feature has
// neither key, and that SHALL NOT disqualify it (Scenario 13).
function isSavedPreset(p: unknown): p is Omit<SavedPreset, "milestone" | "preset"> {
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

// A missing value, or one present but not a string, reads as "" (Scenario 23).
function normalizeStoredString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function normalizeSavedPreset(p: unknown): SavedPreset | null {
  if (!isSavedPreset(p)) return null;
  const raw = p as Record<string, unknown>;
  return {
    ...p,
    milestone: normalizeStoredString(raw.milestone),
    preset: normalizeStoredString(raw.preset),
  };
}

function readServerPresets(value: unknown): SavedPreset[] | null {
  if (!Array.isArray(value)) return null;
  const normalized: SavedPreset[] = [];
  for (const item of value) {
    const preset = normalizeSavedPreset(item);
    if (preset) normalized.push(preset);
  }
  return normalized;
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
