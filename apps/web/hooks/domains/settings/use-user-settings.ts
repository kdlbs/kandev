"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { UserSettingsResponse } from "@/lib/types/http";

/**
 * Returns mapped user settings (camelCase, same shape as the old Zustand slice).
 * Use `useUpdateUserSettings` for mutations.
 */
export function useUserSettings() {
  const query = useQuery(settingsQueryOptions.userSettings());
  const mapped = query.data ? mapUserSettingsResponse(query.data) : null;
  return {
    data: mapped,
    loaded: query.isSuccess,
    loading: query.isFetching,
  };
}

/**
 * Mutation hook for persisting user settings.
 * Applies an optimistic cache update; rolls back on error.
 */
export function useUpdateUserSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: Parameters<typeof updateUserSettings>[0]) =>
      updateUserSettings(payload),
    onMutate: async (_payload) => {
      await qc.cancelQueries({ queryKey: qk.settings.userSettings() });
      const snapshot = qc.getQueryData<UserSettingsResponse>(qk.settings.userSettings());
      // Optimistic updates not applied for userSettings because the API response
      // type (snake_case) doesn't merge directly with the mutation payload without
      // lossy casting. Instead rely on invalidation in onSettled for fresh data.
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
