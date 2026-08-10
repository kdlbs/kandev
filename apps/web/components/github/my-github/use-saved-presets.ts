"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";
import {
  readSavedPresets,
  setSavedPresetDefault,
  type SavedPreset,
  type SavedPresetKind,
} from "./saved-preset-model";

export type { SavedPreset } from "./saved-preset-model";

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
  workspaceSyncQueue = Promise.resolve();
  for (const l of listeners) l();
}

function useUserSavedPresetsSync(enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const initialVersion = snapshotVersion;
    fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        const serverPresets = readSavedPresets(response.settings.github_saved_presets);
        if (cancelled || snapshotVersion !== initialVersion) return;
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
        const serverPresets = readSavedPresets(settings.saved_presets);
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
  const activePresets = workspaceId ? (workspacePresets ?? emptySnapshot) : presets;
  const activePresetsRef = useRef(activePresets);
  const mutationQueueRef = useRef<Promise<void>>(Promise.resolve());
  activePresetsRef.current = activePresets;

  const applyLocal = useCallback(
    (next: SavedPreset[]) => {
      activePresetsRef.current = next;
      if (workspaceId) {
        setWorkspacePresets(next);
      } else {
        publish(next);
      }
    },
    [workspaceId, setWorkspacePresets],
  );

  const persist = useCallback(
    (next: SavedPreset[]) => {
      if (workspaceId) return syncWorkspaceSavedPresets(workspaceId, next);
      return syncServer(next);
    },
    [workspaceId],
  );

  const queueMutation = useCallback((mutation: () => Promise<void>) => {
    const queued = mutationQueueRef.current.catch(() => undefined).then(mutation);
    mutationQueueRef.current = queued;
    return queued;
  }, []);

  const save = useCallback(
    (input: Omit<SavedPreset, "id" | "createdAt" | "isDefault">) => {
      if (workspaceId && workspacePresets === undefined) return null;
      const preset: SavedPreset = {
        ...input,
        id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        createdAt: new Date().toISOString(),
        isDefault: false,
      };
      const append = (current: SavedPreset[]) =>
        current.some((saved) => saved.id === preset.id) ? current : [...current, preset];
      // An earlier queued mutation may briefly persist its older snapshot; this optimistic
      // update's queued write then persists the latest complete list.
      applyLocal(append(activePresetsRef.current));
      void queueMutation(async () => {
        await persist(append(activePresetsRef.current));
      }).catch(() => {});
      return preset;
    },
    [applyLocal, persist, queueMutation, workspaceId, workspacePresets],
  );

  const remove = useCallback(
    (id: string) => {
      if (workspaceId && workspacePresets === undefined) return;
      const discard = (current: SavedPreset[]) => current.filter((preset) => preset.id !== id);
      applyLocal(discard(activePresetsRef.current));
      void queueMutation(async () => {
        await persist(discard(activePresetsRef.current));
      }).catch(() => {});
    },
    [applyLocal, persist, queueMutation, workspaceId, workspacePresets],
  );

  const setDefault = useCallback(
    async (kind: SavedPresetKind, id: string | null) => {
      if (workspaceId && workspacePresets === undefined) return;
      await queueMutation(async () => {
        const next = setSavedPresetDefault(activePresetsRef.current, kind, id);
        if (next === activePresetsRef.current) return;
        await persist(next);
        // Concurrent save/remove calls can update local state while persistence is in flight.
        // Reapply the default to that latest snapshot; its queued write persists the merged list.
        applyLocal(setSavedPresetDefault(activePresetsRef.current, kind, id));
      });
    },
    [applyLocal, persist, queueMutation, workspaceId, workspacePresets],
  );

  return { presets: activePresets, save, remove, setDefault };
}
