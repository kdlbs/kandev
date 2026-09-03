"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import { createQueuedUserSettingsSyncWithResponse } from "@/lib/user-settings-sync";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
import { type TaskColor } from "@/lib/task-colors";

type TaskColorMap = Record<string, TaskColor | null>;

function cloneTaskColorMap(colors: TaskColorMap): TaskColorMap {
  return { ...colors };
}

function optimisticTaskColorSettings(
  settings: ReturnType<ReturnType<typeof useAppStoreApi>["getState"]>["userSettings"],
  taskId: string,
  color: TaskColor | null,
) {
  return {
    ...settings,
    sidebarTaskColors: {
      ...settings.sidebarTaskColors,
      [taskId]: color,
    },
    loaded: true,
  };
}

function responseIsCurrent(
  responseRevision: number | undefined,
  currentRevision: number | null,
  settingsAtSubmit: object,
  currentSettings: object,
) {
  const order = compareUserSettingsRevisions(responseRevision, currentRevision);
  return order === null ? currentSettings === settingsAtSubmit : order >= 0;
}

export function useTaskColor(taskId: string | undefined): TaskColor | null {
  return useAppStore((state) => {
    if (!taskId) return null;
    return state.userSettings.sidebarTaskColors[taskId] ?? null;
  });
}

export function useSetTaskColor(): (taskId: string, color: TaskColor | null) => void {
  const store = useAppStoreApi();
  const settings = useAppStore((state) => state.userSettings);
  const { toast } = useToast();
  const { t } = useTranslation();
  const confirmedColorsRef = useRef<TaskColorMap>(cloneTaskColorMap(settings.sidebarTaskColors));
  const confirmedRevisionRef = useRef<number | null>(settings.revision);
  const operationRef = useRef(0);
  const pendingRef = useRef(0);
  const sync = useMemo(
    () =>
      createQueuedUserSettingsSyncWithResponse<{
        taskId: string;
        color: TaskColor | null;
      }>(
        ({ taskId, color }): UserSettingsUpdatePayload => ({
          sidebar_task_color_patch: {
            colors: { [taskId]: color },
            if_missing: false,
          },
        }),
      ),
    [],
  );

  useEffect(() => {
    const current = store.getState().userSettings;
    if (pendingRef.current === 0) {
      confirmedColorsRef.current = cloneTaskColorMap(current.sidebarTaskColors);
      confirmedRevisionRef.current = current.revision;
      return;
    }
    const order = compareUserSettingsRevisions(current.revision, confirmedRevisionRef.current);
    if (order === 1) {
      confirmedColorsRef.current = cloneTaskColorMap(current.sidebarTaskColors);
      confirmedRevisionRef.current = current.revision;
    }
  }, [settings, store]);

  return useCallback(
    (taskId: string, color: TaskColor | null) => {
      if (!taskId) return;
      const operation = ++operationRef.current;
      pendingRef.current += 1;
      const settingsAtSubmit = store.getState().userSettings;
      store
        .getState()
        .setUserSettings(optimisticTaskColorSettings(settingsAtSubmit, taskId, color));

      void sync({ taskId, color })
        .then((response) => {
          const latest = store.getState().userSettings;
          if (
            operation !== operationRef.current ||
            !responseIsCurrent(
              response.settings.revision,
              latest.revision,
              settingsAtSubmit,
              latest,
            )
          ) {
            return;
          }
          const mapped = mapUserSettingsResponse(response, latest);
          confirmedColorsRef.current = cloneTaskColorMap(mapped.sidebarTaskColors);
          confirmedRevisionRef.current = mapped.revision;
          store.getState().setUserSettings(mapped);
        })
        .catch(() => {
          if (operation !== operationRef.current) return;
          const latest = store.getState().userSettings;
          const order = compareUserSettingsRevisions(latest.revision, confirmedRevisionRef.current);
          if (order === 1) {
            confirmedColorsRef.current = cloneTaskColorMap(latest.sidebarTaskColors);
            confirmedRevisionRef.current = latest.revision;
          }
          store.getState().setUserSettings({
            ...latest,
            sidebarTaskColors: cloneTaskColorMap(confirmedColorsRef.current),
            loaded: true,
          });
          toast({ variant: "error", description: t("task:manualColorSaveError") });
        })
        .finally(() => {
          pendingRef.current -= 1;
        });
    },
    [store, sync, t, toast],
  );
}
