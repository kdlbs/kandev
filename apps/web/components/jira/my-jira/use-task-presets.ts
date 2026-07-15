"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  DEFAULT_JIRA_PRESETS,
  resolveJiraTaskPresets,
  type JiraStoredPreset,
  type JiraTaskPreset,
} from "./presets";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";

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

function readServerPresets(value: unknown): JiraStoredPreset[] | null | undefined {
  if (value === null) return null;
  if (!Array.isArray(value)) return undefined;
  return value.filter(isStoredPreset);
}

const syncServer = createQueuedUserSettingsSync<JiraStoredPreset[] | null>((presets) => ({
  jira_task_presets: presets,
}));

export function useJiraTaskPresets() {
  const [stored, setStored] = useState<JiraStoredPreset[] | null>(null);
  const [loaded, setLoaded] = useState(false);
  const writeVersion = useRef(0);

  useEffect(() => {
    let cancelled = false;
    const initialVersion = writeVersion.current;
    async function init() {
      const response = await fetchUserSettings({ cache: "no-store" }).catch(() => null);
      const serverValue = readServerPresets(response?.settings.jira_task_presets);
      if (!cancelled && serverValue !== undefined && writeVersion.current === initialVersion) {
        setStored(serverValue);
      }
      if (!cancelled) setLoaded(true);
    }
    void init();
    return () => {
      cancelled = true;
    };
  }, []);

  const save = useCallback(async (next: JiraStoredPreset[]) => {
    writeVersion.current += 1;
    await syncServer(next);
    setStored(next);
  }, []);

  const reset = useCallback(async () => {
    writeVersion.current += 1;
    await syncServer(null);
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
