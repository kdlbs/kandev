"use client";

import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { readPendingTaskCreateLastUsedState } from "@/components/task-create-dialog-handlers";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { TaskCreateLastUsedState, UserSettingsState } from "@/lib/state/slices/settings/types";

let userSettingsFetchPromise: Promise<UserSettingsState | null> | null = null;

function loadUserSettingsOnce() {
  if (!userSettingsFetchPromise) {
    userSettingsFetchPromise = fetchUserSettings({ cache: "no-store" })
      .then((response) => {
        if (!response?.settings) return null;
        const mapped = mapUserSettingsResponse(response);
        return mapped.loaded ? mapped : null;
      })
      .catch(() => null)
      .finally(() => {
        userSettingsFetchPromise = null;
      });
  }
  return userSettingsFetchPromise;
}

function mergePendingTaskCreateLastUsed(settings: UserSettingsState): UserSettingsState {
  const pending = readPendingTaskCreateLastUsedState();
  const definedPending = Object.fromEntries(
    Object.entries(pending).filter(([, value]) => value !== undefined),
  ) as Partial<TaskCreateLastUsedState>;
  if (Object.keys(definedPending).length === 0) return settings;
  return {
    ...settings,
    taskCreateLastUsed: {
      ...settings.taskCreateLastUsed,
      ...definedPending,
    },
  };
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
      .then((mapped) => {
        if (cancelled || !mapped) return;
        const next = mergePendingTaskCreateLastUsed(mapped);
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
