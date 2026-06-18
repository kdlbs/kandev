"use client";

import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import {
  PR_PRESETS as BUILTIN_PR_PRESETS,
  ISSUE_PRESETS as BUILTIN_ISSUE_PRESETS,
  type PresetOption,
} from "./search-bar";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";

const STORAGE_KEY = "kandev:github-default-queries:v1";
const MIGRATED_KEY = "kandev:github-default-queries:migrated-to-backend:v1";
const SYNC_FAILED_KEY = "kandev:github-default-queries:sync-failed:v1";

export type StoredQueryPreset = {
  value: string;
  label: string;
  filter: string;
  group: "inbox" | "created";
};

type StoredDefaults = {
  pr: StoredQueryPreset[];
  issue: StoredQueryPreset[];
};

export function toStored(presets: PresetOption[]): StoredQueryPreset[] {
  return presets.map(({ value, label, filter, group }) => ({ value, label, filter, group }));
}

function readStorage(): StoredDefaults | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as unknown;
    if (
      typeof parsed !== "object" ||
      parsed === null ||
      !Array.isArray((parsed as StoredDefaults).pr) ||
      !Array.isArray((parsed as StoredDefaults).issue)
    ) {
      return null;
    }
    return parsed as StoredDefaults;
  } catch {
    return null;
  }
}

function writeStorage(defaults: StoredDefaults | null) {
  if (typeof window === "undefined") return;
  try {
    if (defaults === null) {
      window.localStorage.removeItem(STORAGE_KEY);
    } else {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(defaults));
    }
  } catch {
    /* ignore quota / access errors */
  }
}

let snapshot: StoredDefaults | null | undefined = undefined;
const listeners = new Set<() => void>();

function publish(next: StoredDefaults | null) {
  snapshot = next;
  writeStorage(next);
  listeners.forEach((fn) => fn());
}

function readServerDefaults(value: unknown): StoredDefaults | null | undefined {
  if (value === null) return null;
  if (
    typeof value !== "object" ||
    !Array.isArray((value as StoredDefaults).pr) ||
    !Array.isArray((value as StoredDefaults).issue)
  ) {
    return undefined;
  }
  return value as StoredDefaults;
}

function hasFailedSync(): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(SYNC_FAILED_KEY) === "1";
}

function setFailedSync(failed: boolean): void {
  if (typeof window === "undefined") return;
  try {
    if (failed) window.localStorage.setItem(SYNC_FAILED_KEY, "1");
    else window.localStorage.removeItem(SYNC_FAILED_KEY);
  } catch {
    /* ignore storage failures */
  }
}

function syncServer(defaults: StoredDefaults | null): Promise<void> {
  return updateUserSettings({ github_default_query_presets: defaults })
    .then(() => {
      setFailedSync(false);
    })
    .catch(() => {
      setFailedSync(true);
    });
}

function snapshotKey(value: StoredDefaults | null): string {
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

function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function getSnapshot(): StoredDefaults | null {
  if (snapshot === undefined) snapshot = readStorage();
  return snapshot;
}

function getServerSnapshot(): StoredDefaults | null {
  return null;
}

export function useDefaultQueryPresets() {
  const stored = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  useEffect(() => {
    let cancelled = false;
    const initialKey = snapshotKey(getSnapshot());
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverDefaults = readServerDefaults(response.settings.github_default_query_presets);
        if (cancelled || serverDefaults === undefined) return;
        const local = getSnapshot();
        if (snapshotKey(local) !== initialKey) return;
        if (hasFailedSync() && local !== null) {
          void syncServer(local);
          return;
        }
        if (serverDefaults === null && local !== null && !hasMigratedToBackend()) {
          void syncServer(local);
          markMigratedToBackend();
          return;
        }
        publish(serverDefaults);
        markMigratedToBackend();
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const prPresets = useMemo(() => stored?.pr ?? toStored(BUILTIN_PR_PRESETS), [stored]);
  const issuePresets = useMemo(() => stored?.issue ?? toStored(BUILTIN_ISSUE_PRESETS), [stored]);

  const save = useCallback((defaults: StoredDefaults) => {
    publish(defaults);
    void syncServer(defaults);
    markMigratedToBackend();
  }, []);

  const reset = useCallback(() => {
    publish(null);
    void syncServer(null);
    markMigratedToBackend();
  }, []);

  const isCustomized = stored !== null;

  return { prPresets, issuePresets, save, reset, isCustomized };
}

/** Resolve full PresetOption[] by merging stored presets with icon lookups from builtins. */
export function resolvePresetOptions(
  stored: StoredQueryPreset[],
  builtins: PresetOption[],
): PresetOption[] {
  const iconMap = new Map(builtins.map((b) => [b.value, b.icon]));
  const defaultIcon = builtins[0]?.icon;
  return stored.map((s) => ({
    ...s,
    icon: iconMap.get(s.value) ?? defaultIcon,
  }));
}
