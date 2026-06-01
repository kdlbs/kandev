"use client";

import { useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { DEFAULT_USER_SETTINGS, type UserSettingsState } from "@/lib/types/settings";

/**
 * Returns mapped user settings (camelCase, same shape as the old Zustand slice).
 * The TanStack Query cache holds the already-mapped `UserSettingsState`.
 * Use `useUpdateUserSettings` for mutations.
 */
export function useUserSettings() {
  const query = useQuery(settingsQueryOptions.userSettings());
  return {
    data: query.data ?? null,
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

/**
 * Returns a setter that optimistically writes mapped user settings into the
 * `qk.settings.userSettings()` cache. Replaces the old Zustand `setUserSettings`
 * action used by inline toggles before/while persisting.
 */
export function useSetUserSettings(): (next: UserSettingsState) => void {
  const qc = useQueryClient();
  return useCallback(
    (next: UserSettingsState) => {
      qc.setQueryData<UserSettingsState>(qk.settings.userSettings(), next);
    },
    [qc],
  );
}

/**
 * Read + write user settings with a stale-free `getLatest()` that reads the
 * live cache (replaces the old `storeApi.getState().userSettings` pattern used
 * by inline settings toggles to avoid stale closures during optimistic writes).
 */
export function useUserSettingsController() {
  const qc = useQueryClient();
  const query = useQuery(settingsQueryOptions.userSettings());
  const setUserSettings = useSetUserSettings();
  const getLatest = useCallback(
    (): UserSettingsState =>
      qc.getQueryData<UserSettingsState>(qk.settings.userSettings()) ?? DEFAULT_USER_SETTINGS,
    [qc],
  );
  return {
    userSettings: query.data ?? DEFAULT_USER_SETTINGS,
    setUserSettings,
    getLatest,
  };
}

/**
 * Mutation hook for persisting user settings.
 * Applies an optimistic cache update; rolls back on error.
 */
export function useUpdateUserSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: Parameters<typeof updateUserSettings>[0]) => updateUserSettings(payload),
    onMutate: async (_payload) => {
      await qc.cancelQueries({ queryKey: qk.settings.userSettings() });
      const snapshot = qc.getQueryData<UserSettingsState>(qk.settings.userSettings());
      // Optimistic updates not applied for userSettings because the mutation
      // payload (snake_case) doesn't merge directly with the mapped cache
      // shape without lossy casting. Rely on invalidation in onSettled.
      return { snapshot };
    },
    onError: (_err, _payload, ctx) => {
      if (ctx?.snapshot !== undefined) {
        qc.setQueryData(qk.settings.userSettings(), ctx.snapshot);
      }
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.settings.userSettings() });
    },
  });
}
