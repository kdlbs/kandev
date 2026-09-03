"use client";

import { useEffect } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { clearLegacyTaskColors, readLegacyTaskColors, type TaskColor } from "@/lib/task-colors";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { AppState } from "@/lib/state/store";

export const TASK_COLOR_MIGRATION_BATCH_SIZE = 500;

let migrationPromise: Promise<void> | null = null;

export function chunkTaskColors(
  colors: Record<string, TaskColor>,
  batchSize = TASK_COLOR_MIGRATION_BATCH_SIZE,
): Array<Record<string, TaskColor>> {
  const entries = Object.entries(colors);
  const batches: Array<Record<string, TaskColor>> = [];
  for (let start = 0; start < entries.length; start += batchSize) {
    batches.push(Object.fromEntries(entries.slice(start, start + batchSize)));
  }
  return batches;
}

export async function migrateLegacyTaskColors(store: StoreApi<AppState>): Promise<void> {
  const legacy = readLegacyTaskColors();
  const batches = chunkTaskColors(legacy);
  if (batches.length === 0) {
    clearLegacyTaskColors();
    return;
  }

  for (const colors of batches) {
    const response = await updateUserSettings({
      sidebar_task_color_patch: {
        colors,
        if_missing: true,
      },
    });
    const current = store.getState().userSettings;
    store.getState().setUserSettings(mapUserSettingsResponse(response, current));
  }
  clearLegacyTaskColors();
}

export function __resetTaskColorMigrationForTests() {
  migrationPromise = null;
}

export function useTaskColorMigration(enabled = true) {
  const store = useAppStoreApi();
  const loaded = useAppStore((state) => state.userSettings.loaded);

  useEffect(() => {
    if (!enabled || !loaded || migrationPromise) return;
    migrationPromise = migrateLegacyTaskColors(store)
      .catch(() => undefined)
      .finally(() => {
        migrationPromise = null;
      });
  }, [enabled, loaded, store]);
}
