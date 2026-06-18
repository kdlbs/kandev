"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  DEFAULT_JIRA_PRESETS,
  resolveJiraTaskPresets,
  type JiraStoredPreset,
  type JiraTaskPreset,
} from "./presets";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";

const STORAGE_KEY = "kandev:jira:task-presets:v1";

function isStoredPreset(v: unknown): v is JiraStoredPreset {
  if (!v || typeof v !== "object") return false;
  const rec = v as Record<string, unknown>;
  return (
    typeof rec.id === "string" &&
    typeof rec.label === "string" &&
    typeof rec.hint === "string" &&
    typeof rec.icon === "string" &&
    typeof rec.prompt_template === "string"
  );
}

function readStorage(): JiraStoredPreset[] | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return null;
    // An explicitly-saved empty array means "the user cleared their presets"
    // and should beat the built-in defaults. Only the absent-key case should
    // fall back to defaults.
    return parsed.filter(isStoredPreset);
  } catch {
    return null;
  }
}

function writeStorage(presets: JiraStoredPreset[] | null): void {
  if (typeof window === "undefined") return;
  try {
    if (presets === null) {
      window.localStorage.removeItem(STORAGE_KEY);
    } else {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
    }
  } catch {
    // Quota or private-mode: swallow. Presets just won't persist.
  }
}

function readServerPresets(value: unknown): JiraStoredPreset[] | null | undefined {
  if (value === null) return null;
  if (!Array.isArray(value)) return undefined;
  return value.filter(isStoredPreset);
}

function syncServer(presets: JiraStoredPreset[] | null): void {
  updateUserSettings({ jira_task_presets: presets }).catch(() => {});
}

export function useJiraTaskPresets() {
  const [stored, setStored] = useState<JiraStoredPreset[] | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function init() {
      const value = readStorage();
      if (!cancelled) {
        setStored(value);
        setLoaded(true);
      }
      const response = await fetchUserSettings({ cache: "no-store" }).catch(() => null);
      const serverValue = readServerPresets(response?.settings.jira_task_presets);
      if (!cancelled && serverValue !== undefined) {
        writeStorage(serverValue);
        setStored(serverValue);
      }
    }
    void init();
    return () => {
      cancelled = true;
    };
  }, []);

  const save = useCallback((next: JiraStoredPreset[]) => {
    writeStorage(next);
    syncServer(next);
    setStored(next);
  }, []);

  const reset = useCallback(() => {
    writeStorage(null);
    syncServer(null);
    setStored(null);
  }, []);

  const taskPresets = useMemo<JiraTaskPreset[]>(() => resolveJiraTaskPresets(stored), [stored]);
  const storedOrDefault = stored ?? DEFAULT_JIRA_PRESETS;

  return {
    stored: storedOrDefault,
    isCustomized: stored !== null,
    taskPresets,
    save,
    reset,
    loaded,
  };
}

export { resolveJiraTaskPresets };
