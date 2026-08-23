"use client";

import { useCallback, useRef } from "react";

import { useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  createLayoutProfile,
  deleteLayoutProfile,
  type CreateLayoutProfileInput,
} from "@/lib/layout/layout-profiles";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { SavedLayout } from "@/lib/types/http";

type SavedLayoutMutation = (layouts: SavedLayout[]) => SavedLayout[];

export function useSavedLayoutMutations() {
  const appStore = useAppStoreApi();
  const queueRef = useRef<Promise<void>>(Promise.resolve());

  const mutate = useCallback(
    (mutation: SavedLayoutMutation) => {
      const queued = queueRef.current
        .catch(() => undefined)
        .then(async () => {
          const currentLayouts = appStore.getState().userSettings.savedLayouts;
          const response = await updateUserSettings({ saved_layouts: mutation(currentLayouts) });
          const currentSettings = appStore.getState().userSettings;
          appStore.getState().setUserSettings(mapUserSettingsResponse(response, currentSettings));
        });
      queueRef.current = queued;
      return queued;
    },
    [appStore],
  );

  const saveLayout = useCallback(
    (input: CreateLayoutProfileInput) => mutate((layouts) => createLayoutProfile(layouts, input)),
    [mutate],
  );
  const deleteLayout = useCallback(
    (layoutId: string) => mutate((layouts) => deleteLayoutProfile(layouts, layoutId)),
    [mutate],
  );

  return { saveLayout, deleteLayout };
}
