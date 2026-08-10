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

// Shared across hook instances so all workspace API writes stay ordered. The
// per-hook mutation queue below only coordinates that instance's optimistic state.
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

function discardSavedPreset(presets: SavedPreset[], id: string): SavedPreset[] {
  return presets.filter((preset) => preset.id !== id);
}

function restoreSavedPreset(
  presets: SavedPreset[],
  preset: SavedPreset,
  originalIndex: number,
): SavedPreset[] {
  if (presets.some((candidate) => candidate.id === preset.id)) return presets;
  const restored =
    preset.isDefault &&
    presets.some((candidate) => candidate.kind === preset.kind && candidate.isDefault)
      ? { ...preset, isDefault: false }
      : preset;
  // Best-effort: the insertion point may shift if concurrent saves ran before rollback.
  const insertionIndex = Math.min(originalIndex, presets.length);
  return [...presets.slice(0, insertionIndex), restored, ...presets.slice(insertionIndex)];
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
    async (input: Omit<SavedPreset, "id" | "createdAt" | "isDefault">) => {
      if (workspaceId && workspacePresets === undefined) return null;
      const preset: SavedPreset = {
        ...input,
        id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        createdAt: new Date().toISOString(),
        isDefault: false,
      };
      const append = (current: SavedPreset[]) =>
        current.some((saved) => saved.id === preset.id) ? current : [...current, preset];
      applyLocal(append(activePresetsRef.current));
      try {
        await queueMutation(async () => {
          await persist(append(activePresetsRef.current));
        });
        return preset;
      } catch (error) {
        const current = activePresetsRef.current;
        const rolledBack = discardSavedPreset(current, preset.id);
        if (rolledBack.length !== current.length) applyLocal(rolledBack);
        throw error;
      }
    },
    [applyLocal, persist, queueMutation, workspaceId, workspacePresets],
  );

  const remove = useCallback(
    async (id: string) => {
      if (workspaceId && workspacePresets === undefined) return false;
      const current = activePresetsRef.current;
      const originalIndex = current.findIndex((preset) => preset.id === id);
      if (originalIndex === -1) return false;
      const removedPreset = current[originalIndex];
      applyLocal(discardSavedPreset(current, id));
      try {
        await queueMutation(async () => {
          await persist(discardSavedPreset(activePresetsRef.current, id));
        });
        return true;
      } catch (error) {
        const latest = activePresetsRef.current;
        const rolledBack = restoreSavedPreset(latest, removedPreset, originalIndex);
        if (rolledBack !== latest) applyLocal(rolledBack);
        throw error;
      }
    },
    [applyLocal, persist, queueMutation, workspaceId, workspacePresets],
  );

  const setDefault = useCallback(
    async (kind: SavedPresetKind, id: string | null) => {
      if (workspaceId && workspacePresets === undefined) return false;
      let persisted = false;
      await queueMutation(async () => {
        const next = setSavedPresetDefault(activePresetsRef.current, kind, id);
        if (next === activePresetsRef.current) return;
        await persist(next);
        persisted = true;
        // `setDefault` publishes only after persistence succeeds (no optimistic update).
        // Re-read activePresetsRef.current so that concurrent mutations applied
        // during the await (e.g. sibling-hook writes for portable user settings)
        // are merged in before publishing the new default state.
        const latest = activePresetsRef.current;
        const remerged = setSavedPresetDefault(latest, kind, id);
        if (remerged !== latest) applyLocal(remerged);
      });
      return persisted;
    },
    [applyLocal, persist, queueMutation, workspaceId, workspacePresets],
  );

  return { presets: activePresets, save, remove, setDefault };
}
