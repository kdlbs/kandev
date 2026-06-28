"use client";

import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { readPendingTaskCreateLastUsedState } from "@/components/task-create-dialog-handlers";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";

let userSettingsFetchPromise: Promise<UserSettingsState | null> | null = null;
let userSettingsLoadAttempted = false;

function loadUserSettingsOnce() {
  if (!userSettingsFetchPromise) {
    userSettingsLoadAttempted = true;
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
  if (Object.values(pending).every((value) => value === undefined)) return settings;
  return {
    ...settings,
    taskCreateLastUsed: {
      ...settings.taskCreateLastUsed,
      ...pending,
    },
  };
}

export function useEnsureUserSettings(
  enabled = true,
  options: { preserveTaskCreatePending?: boolean } = {},
) {
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
    if (userSettingsLoadAttempted && !userSettingsFetchPromise) {
      setFetchSettled(true);
      return;
    }

    let cancelled = false;
    setFetchSettled(false);
    loadUserSettingsOnce()
      .then((mapped) => {
        if (cancelled || !mapped) return;
        const next = options.preserveTaskCreatePending
          ? mergePendingTaskCreateLastUsed(mapped)
          : mapped;
        setUserSettings(next);
      })
      .finally(() => {
        if (!cancelled) setFetchSettled(true);
      });

    return () => {
      cancelled = true;
    };
  }, [enabled, options.preserveTaskCreatePending, setUserSettings, userSettings.loaded]);

  return {
    loaded: userSettings.loaded || fetchSettled,
    userSettings,
  };
}
