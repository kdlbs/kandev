"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";

export type SavedPreset = {
  id: string;
  kind: "pr" | "issue";
  label: string;
  customQuery: string;
  repoFilter: string;
  createdAt: string;
};

const listeners = new Set<() => void>();
let snapshot: SavedPreset[] = [];
let snapshotVersion = 0;
const emptySnapshot: SavedPreset[] = [];

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

function readServerPresets(value: unknown): SavedPreset[] | null {
  if (!Array.isArray(value)) return null;
  return value.flatMap((candidate): SavedPreset[] => {
    if (typeof candidate !== "object" || candidate === null) return [];
    const preset = candidate as Record<string, unknown>;
    const kind = preset.kind;
    if (
      typeof preset.id !== "string" ||
      (kind !== "pr" && kind !== "issue") ||
      typeof preset.label !== "string"
    ) {
      return [];
    }
    return [
      {
        ...(preset as SavedPreset),
        repoFilter: typeof preset.repoFilter === "string" ? preset.repoFilter : "",
      },
    ];
  });
}

const syncServer = createQueuedUserSettingsSync<SavedPreset[]>((next) => ({
  github_saved_presets: next,
}));

let workspaceSyncQueue = Promise.resolve();

function syncWorkspaceSavedPresets(workspaceId: string, next: SavedPreset[]): Promise<void> {
  workspaceSyncQueue = workspaceSyncQueue
    .catch(() => undefined)
    .then(() =>
      updateGitHubWorkspaceSettings({
        workspace_id: workspaceId,
        saved_presets: next,
      }).then(() => undefined),
    );
  return workspaceSyncQueue;
}

export function __resetSnapshotForTests() {
  snapshot = [];
  snapshotVersion = 0;
  for (const l of listeners) l();
}

function useUserSavedPresetsSync(enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const initialVersion = snapshotVersion;
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverPresets = readServerPresets(response.settings.github_saved_presets);
        if (cancelled || !serverPresets || snapshotVersion !== initialVersion) return;
        publish(serverPresets);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [enabled]);
}

function useWorkspaceSavedPresets(workspaceId: string | null) {
  const [workspacePresets, setWorkspacePresets] = useState<SavedPreset[] | undefined>(undefined);
  const writeSeq = useRef(0);
  useEffect(() => {
    if (!workspaceId) {
      setWorkspacePresets(undefined);
      return;
    }
    let cancelled = false;
    const seq = writeSeq.current;
    setWorkspacePresets(undefined);
    fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (cancelled || seq !== writeSeq.current) return;
        const serverPresets = readServerPresets(settings.saved_presets) ?? [];
        setWorkspacePresets(serverPresets);
      })
      .catch(() => {
        if (!cancelled) setWorkspacePresets(undefined);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);
  const setWorkspacePresetsFromLocal = useCallback((next: SavedPreset[]) => {
    writeSeq.current += 1;
    setWorkspacePresets(next);
  }, []);
  return { workspacePresets, setWorkspacePresets: setWorkspacePresetsFromLocal };
}

export function useSavedPresets(workspaceId: string | null = null) {
  const presets = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const { workspacePresets, setWorkspacePresets } = useWorkspaceSavedPresets(workspaceId);
  useUserSavedPresetsSync(!workspaceId);
  const activePresets = workspaceId ? (workspacePresets ?? []) : presets;

  const save = useCallback(
    (input: Omit<SavedPreset, "id" | "createdAt">) => {
      if (workspaceId && workspacePresets === undefined) return null;
      const preset: SavedPreset = {
        ...input,
        id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        createdAt: new Date().toISOString(),
      };
      const next = [...activePresets, preset];
      if (workspaceId) {
        setWorkspacePresets(next);
        void syncWorkspaceSavedPresets(workspaceId, next).catch(() => {});
        return preset;
      }
      publish(next);
      void syncServer(next);
      return preset;
    },
    [activePresets, workspaceId, workspacePresets, setWorkspacePresets],
  );

  const remove = useCallback(
    (id: string) => {
      if (workspaceId && workspacePresets === undefined) return;
      const next = activePresets.filter((p) => p.id !== id);
      if (workspaceId) {
        setWorkspacePresets(next);
        void syncWorkspaceSavedPresets(workspaceId, next).catch(() => {});
        return;
      }
      publish(next);
      void syncServer(next);
    },
    [activePresets, workspaceId, workspacePresets, setWorkspacePresets],
  );

  return { presets: activePresets, save, remove };
}
