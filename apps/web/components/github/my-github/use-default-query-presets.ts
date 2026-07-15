"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import {
  PR_PRESETS as BUILTIN_PR_PRESETS,
  ISSUE_PRESETS as BUILTIN_ISSUE_PRESETS,
  type PresetOption,
} from "./search-bar";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";

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

let snapshot: StoredDefaults | null | undefined = undefined;
let snapshotVersion = 0;
const listeners = new Set<() => void>();

function publish(next: StoredDefaults | null) {
  snapshot = next;
  snapshotVersion += 1;
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

const syncServer = createQueuedUserSettingsSync<StoredDefaults | null>((defaults) => ({
  github_default_query_presets: defaults,
}));

let workspaceSyncQueue = Promise.resolve();

function syncWorkspaceDefaultQueryPresets(
  workspaceId: string,
  defaults: StoredDefaults | null,
): Promise<void> {
  workspaceSyncQueue = workspaceSyncQueue
    .catch(() => undefined)
    .then(() =>
      updateGitHubWorkspaceSettings({
        workspace_id: workspaceId,
        default_query_presets: defaults,
      }).then(() => undefined),
    );
  return workspaceSyncQueue;
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function getSnapshot(): StoredDefaults | null {
  return snapshot ?? null;
}

function getServerSnapshot(): StoredDefaults | null {
  return null;
}

export function __resetSnapshotForTests() {
  snapshot = undefined;
  snapshotVersion = 0;
  listeners.forEach((fn) => fn());
}

function useUserDefaultQueryPresetSync(enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const initialVersion = snapshotVersion;
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverDefaults = readServerDefaults(response.settings.github_default_query_presets);
        if (cancelled || serverDefaults === undefined || snapshotVersion !== initialVersion) return;
        publish(serverDefaults);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [enabled]);
}

function useWorkspaceDefaultQueryPresets(workspaceId: string | null) {
  const [workspaceDefaults, setWorkspaceDefaults] = useState<StoredDefaults | null | undefined>(
    undefined,
  );
  const writeSeq = useRef(0);
  useEffect(() => {
    if (!workspaceId) {
      setWorkspaceDefaults(undefined);
      return;
    }
    let cancelled = false;
    const seq = writeSeq.current;
    setWorkspaceDefaults(undefined);
    fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (cancelled || seq !== writeSeq.current) return;
        const defaults = readServerDefaults(settings.default_query_presets);
        setWorkspaceDefaults(defaults === undefined ? null : defaults);
      })
      .catch(() => {
        if (!cancelled) setWorkspaceDefaults(undefined);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);
  const setWorkspaceDefaultsFromLocal = useCallback((next: StoredDefaults | null) => {
    writeSeq.current += 1;
    setWorkspaceDefaults(next);
  }, []);
  return { workspaceDefaults, setWorkspaceDefaults: setWorkspaceDefaultsFromLocal };
}

export function useDefaultQueryPresets(workspaceId: string | null = null) {
  const stored = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const { workspaceDefaults, setWorkspaceDefaults } = useWorkspaceDefaultQueryPresets(workspaceId);
  useUserDefaultQueryPresetSync(!workspaceId);
  const effectiveStored = workspaceId ? workspaceDefaults : stored;
  const prPresets = useMemo(
    () => effectiveStored?.pr ?? toStored(BUILTIN_PR_PRESETS),
    [effectiveStored],
  );
  const issuePresets = useMemo(
    () => effectiveStored?.issue ?? toStored(BUILTIN_ISSUE_PRESETS),
    [effectiveStored],
  );

  const save = useCallback(
    async (defaults: StoredDefaults) => {
      if (workspaceId && workspaceDefaults === undefined) return;
      if (workspaceId) {
        await syncWorkspaceDefaultQueryPresets(workspaceId, defaults);
        setWorkspaceDefaults(defaults);
        return;
      }
      await syncServer(defaults);
      publish(defaults);
    },
    [workspaceId, workspaceDefaults, setWorkspaceDefaults],
  );

  const reset = useCallback(async () => {
    if (workspaceId && workspaceDefaults === undefined) return;
    if (workspaceId) {
      await syncWorkspaceDefaultQueryPresets(workspaceId, null);
      setWorkspaceDefaults(null);
      return;
    }
    await syncServer(null);
    publish(null);
  }, [workspaceId, workspaceDefaults, setWorkspaceDefaults]);

  const isCustomized = effectiveStored !== null && effectiveStored !== undefined;

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
