"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";
import {
  PR_PRESETS as BUILTIN_PR_PRESETS,
  ISSUE_PRESETS as BUILTIN_ISSUE_PRESETS,
  type PresetOption,
} from "./search-bar";

const STORAGE_KEY = "kandev:github-default-queries:v1";

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

  const prPresets = useMemo(() => stored?.pr ?? toStored(BUILTIN_PR_PRESETS), [stored]);
  const issuePresets = useMemo(() => stored?.issue ?? toStored(BUILTIN_ISSUE_PRESETS), [stored]);

  const save = useCallback((defaults: StoredDefaults) => {
    publish(defaults);
  }, []);

  const reset = useCallback(() => {
    publish(null);
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
