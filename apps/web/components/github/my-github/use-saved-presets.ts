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
// Portable presets use a module snapshot shared by every hook instance, so queue the
// complete read/persist/publish mutation rather than only its already-built payload.
let portableMutationQueue = Promise.resolve();

function queuePortableMutation(mutation: () => Promise<void>): Promise<void> {
  const queued = portableMutationQueue.catch(() => undefined).then(mutation);
  portableMutationQueue = queued;
  return queued;
}

// Shared across hook instances so writes for the same workspace stay ordered.
// Separate workspaces remain independent during navigation between them.
const workspaceSyncQueues = new Map<string, Promise<void>>();

function syncWorkspaceSavedPresets(workspaceId: string, next: SavedPreset[]): Promise<void> {
  const previous = workspaceSyncQueues.get(workspaceId) ?? Promise.resolve();
  const queued = previous
    .catch(() => undefined)
    .then(() =>
      updateGitHubWorkspaceSettings({
        workspace_id: workspaceId,
        saved_presets: next,
      }).then(() => undefined),
    );
  workspaceSyncQueues.set(workspaceId, queued);
  void queued
    .finally(() => {
      if (workspaceSyncQueues.get(workspaceId) === queued) workspaceSyncQueues.delete(workspaceId);
    })
    .catch(() => undefined);
  return queued;
}

export function __resetSnapshotForTests() {
  snapshot = [];
  snapshotVersion = 0;
  portableMutationQueue = Promise.resolve();
  workspaceSyncQueues.clear();
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
        if (!cancelled && seq === writeSeq.current) setWorkspacePresets(undefined);
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

type SavedPresetMutationContext = {
  workspaceId: string | null;
  presets: SavedPreset[];
};

function readMutationPresets(context: SavedPresetMutationContext): SavedPreset[] {
  return context.workspaceId === null ? snapshot : context.presets;
}

function persistSavedPresets(
  context: SavedPresetMutationContext,
  next: SavedPreset[],
): Promise<void> {
  if (context.workspaceId !== null) {
    return syncWorkspaceSavedPresets(context.workspaceId, next);
  }
  return syncServer(next);
}

function useSavedPresetMutationContext(
  workspaceId: string | null,
  activePresets: SavedPreset[],
  setWorkspacePresets: (next: SavedPreset[]) => void,
) {
  const mutationContextRef = useRef<SavedPresetMutationContext>({
    workspaceId,
    presets: activePresets,
  });
  if (mutationContextRef.current.workspaceId !== workspaceId) {
    mutationContextRef.current = { workspaceId, presets: activePresets };
  } else {
    mutationContextRef.current.presets = activePresets;
  }
  const applyLocal = useCallback(
    (context: SavedPresetMutationContext, next: SavedPreset[]) => {
      context.presets = next;
      if (mutationContextRef.current !== context) return;
      if (context.workspaceId !== null) setWorkspacePresets(next);
      else publish(next);
    },
    [setWorkspacePresets],
  );
  return { mutationContextRef, applyLocal };
}

export function useSavedPresets(workspaceId: string | null = null) {
  const presets = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const { workspacePresets, setWorkspacePresets } = useWorkspaceSavedPresets(workspaceId);
  const workspacePresetsRef = useRef(workspacePresets);
  workspacePresetsRef.current = workspacePresets;
  useUserSavedPresetsSync(!workspaceId);
  const activePresets = workspaceId ? (workspacePresets ?? emptySnapshot) : presets;
  const { mutationContextRef, applyLocal } = useSavedPresetMutationContext(
    workspaceId,
    activePresets,
    setWorkspacePresets,
  );
  const mutationQueueRef = useRef<Promise<void>>(Promise.resolve());

  const queueMutation = useCallback(
    (mutation: () => Promise<void>) => {
      if (workspaceId === null) return queuePortableMutation(mutation);
      const queued = mutationQueueRef.current.catch(() => undefined).then(mutation);
      mutationQueueRef.current = queued;
      return queued;
    },
    [workspaceId],
  );

  /**
   * @returns The created preset, or null while workspace settings are unavailable.
   * @throws The persistence error after rolling back the optimistic update.
   */
  const save = useCallback(
    async (input: Omit<SavedPreset, "id" | "createdAt" | "isDefault">) => {
      if (workspaceId && workspacePresetsRef.current === undefined) return null;
      const context = mutationContextRef.current;
      const preset: SavedPreset = {
        ...input,
        id: `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        createdAt: new Date().toISOString(),
        isDefault: false,
      };
      const append = (current: SavedPreset[]) =>
        current.some((saved) => saved.id === preset.id) ? current : [...current, preset];
      applyLocal(context, append(readMutationPresets(context)));
      try {
        await queueMutation(async () => {
          await persistSavedPresets(context, append(readMutationPresets(context)));
        });
        return preset;
      } catch (error) {
        const current = readMutationPresets(context);
        const rolledBack = discardSavedPreset(current, preset.id);
        if (rolledBack.length !== current.length) applyLocal(context, rolledBack);
        throw error;
      }
    },
    [applyLocal, queueMutation, workspaceId],
  );

  /**
   * @returns Whether an existing preset was removed.
   * @throws The persistence error after rolling back the optimistic update.
   */
  const remove = useCallback(
    async (id: string) => {
      if (workspaceId && workspacePresetsRef.current === undefined) return false;
      const context = mutationContextRef.current;
      const current = readMutationPresets(context);
      const originalIndex = current.findIndex((preset) => preset.id === id);
      if (originalIndex === -1) return false;
      const removedPreset = current[originalIndex];
      applyLocal(context, discardSavedPreset(current, id));
      try {
        await queueMutation(async () => {
          // Re-read the scoped context so concurrent saves appended after the optimistic remove
          // are included in the persisted payload; the removed id is already absent.
          await persistSavedPresets(context, discardSavedPreset(readMutationPresets(context), id));
        });
        return true;
      } catch (error) {
        const latest = readMutationPresets(context);
        const rolledBack = restoreSavedPreset(latest, removedPreset, originalIndex);
        if (rolledBack !== latest) applyLocal(context, rolledBack);
        throw error;
      }
    },
    [applyLocal, queueMutation, workspaceId],
  );

  /**
   * @returns Whether a changed default was persisted; loading and unchanged targets return false.
   * @throws The persistence error when the settings update fails.
   */
  const setDefault = useCallback(
    async (kind: SavedPresetKind, id: string | null) => {
      if (workspaceId && workspacePresetsRef.current === undefined) return false;
      const context = mutationContextRef.current;
      let persisted = false;
      await queueMutation(async () => {
        const current = readMutationPresets(context);
        const next = setSavedPresetDefault(current, kind, id);
        if (next === current) return;
        await persistSavedPresets(context, next);
        persisted = true;
        // `setDefault` publishes only after persistence succeeds (no optimistic update).
        // Re-read the scoped context so that concurrent mutations applied
        // during the await (e.g. sibling-hook writes for portable user settings)
        // are merged in before publishing the new default state.
        // This is also a no-op when a concurrent remove deleted the target:
        // setSavedPresetDefault returns the same reference for a missing id.
        const latest = readMutationPresets(context);
        const remerged = setSavedPresetDefault(latest, kind, id);
        if (remerged !== latest) applyLocal(context, remerged);
      });
      return persisted;
    },
    [applyLocal, queueMutation, workspaceId],
  );

  return { presets: activePresets, save, remove, setDefault };
}
