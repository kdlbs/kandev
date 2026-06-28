"use client";

import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  readPendingTaskCreateLastUsedState,
  readSyncedTaskCreateLastUsedState,
} from "@/components/task-create-dialog-handlers";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { TaskCreateLastUsedState, UserSettingsState } from "@/lib/state/slices/settings/types";

type LoadedUserSettings = {
  settings: UserSettingsState;
  startedPending: Partial<TaskCreateLastUsedState>;
};

let userSettingsFetchPromise: Promise<LoadedUserSettings | null> | null = null;

function loadUserSettingsOnce() {
  if (!userSettingsFetchPromise) {
    const startedPending = readPendingTaskCreateLastUsedState();
    userSettingsFetchPromise = fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        if (!response?.settings) return null;
        const mapped = mapUserSettingsResponse(response);
        return mapped.loaded ? { settings: mapped, startedPending } : null;
      })
      .catch(() => null)
      .finally(() => {
        userSettingsFetchPromise = null;
      });
  }
  return userSettingsFetchPromise;
}

function mergeTaskCreateLastUsedOverlay(
  settings: UserSettingsState,
  pending: Partial<TaskCreateLastUsedState>,
): UserSettingsState {
  const definedPending = compactTaskCreateLastUsedOverlay(pending);
  if (Object.keys(definedPending).length === 0) return settings;
  return {
    ...settings,
    taskCreateLastUsed: {
      ...settings.taskCreateLastUsed,
      ...definedPending,
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
    ...compactTaskCreateLastUsedOverlay(result.startedPending),
    ...compactTaskCreateLastUsedOverlay(readSyncedTaskCreateLastUsedState()),
    ...compactTaskCreateLastUsedOverlay(readPendingTaskCreateLastUsedState()),
  });
}

export function __resetEnsureUserSettingsForTests() {
  userSettingsFetchPromise = null;
}

export function useEnsureUserSettings(enabled = true) {
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
    setFetchSettled(false);
    loadUserSettingsOnce()
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
    };
  }, [enabled, setUserSettings, userSettings.loaded]);

  return {
    loaded: userSettings.loaded || fetchSettled,
    userSettings,
  };
}
