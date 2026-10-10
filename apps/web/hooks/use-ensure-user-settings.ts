"use client";

import { useEffect, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { readQueuedTaskCreateLastUsedState } from "@/components/task-create-dialog-handlers";
import { readJourneyUserSettings } from "@/hooks/journey-metadata-resources";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { TaskCreateLastUsedState, UserSettingsState } from "@/lib/state/slices/settings/types";

type LoadedUserSettings = {
  settings: UserSettingsState;
};

function loadUserSettingsOnce(store: StoreApi<AppState>, signal: AbortSignal) {
  return readJourneyUserSettings(store, { signal })
    .then((response) => {
      if (!response?.settings) return null;
      const mapped = mapUserSettingsResponse(response);
      return mapped.loaded ? { settings: mapped } : null;
    })
    .catch(() => null);
}

function mergeTaskCreateLastUsedOverlay(
  settings: UserSettingsState,
  pending: Partial<TaskCreateLastUsedState>,
): UserSettingsState {
  const definedPending = compactTaskCreateLastUsedOverlay(pending);
  if (Object.keys(definedPending).length === 0) return settings;
  const pendingWorkflowIds = definedPending.workflowIdsByWorkspace;
  const { workflowIdsByWorkspace: _ignored, ...scalarPending } = definedPending;
  return {
    ...settings,
    taskCreateLastUsed: {
      ...settings.taskCreateLastUsed,
      ...scalarPending,
      ...(pendingWorkflowIds
        ? {
            workflowIdsByWorkspace: {
              ...settings.taskCreateLastUsed.workflowIdsByWorkspace,
              ...pendingWorkflowIds,
            },
          }
        : {}),
    },
  };
}

function compactTaskCreateLastUsedOverlay(pending: Partial<TaskCreateLastUsedState>) {
  return Object.fromEntries(
    Object.entries(pending).filter(([, value]) => value !== undefined),
  ) as Partial<TaskCreateLastUsedState>;
}

function mergeTaskCreateLastUsedForFetch(result: LoadedUserSettings): UserSettingsState {
  return mergeTaskCreateLastUsedOverlay(result.settings, {
    ...compactTaskCreateLastUsedOverlay(readQueuedTaskCreateLastUsedState()),
  });
}

function mergeTaskCreateLastUsedForLoadedSettings(settings: UserSettingsState): UserSettingsState {
  if (!settings.loaded) return settings;
  return mergeTaskCreateLastUsedOverlay(settings, readQueuedTaskCreateLastUsedState());
}

export function useEnsureUserSettings(enabled = true) {
  const store = useAppStoreApi();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [fetchSettled, setFetchSettled] = useState(false);

  useEffect(() => {
    if (!enabled) {
      setFetchSettled(false);
      return;
    }
    if (userSettings.loaded) {
      setFetchSettled(true);
      return;
    }
    let cancelled = false;
    const controller = new AbortController();
    setFetchSettled(false);
    loadUserSettingsOnce(store, controller.signal)
      .then((result) => {
        if (cancelled || !result) return;
        const next = mergeTaskCreateLastUsedForFetch(result);
        setUserSettings(next);
      })
      .finally(() => {
        if (!cancelled) setFetchSettled(true);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [enabled, setUserSettings, store, userSettings.loaded]);

  const effectiveUserSettings = mergeTaskCreateLastUsedForLoadedSettings(userSettings);

  return {
    loaded: effectiveUserSettings.loaded || fetchSettled,
    userSettings: effectiveUserSettings,
  };
}
