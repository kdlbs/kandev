"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { createQueuedUserSettingsSyncWithResponse } from "@/lib/user-settings-sync";
import type { SidebarTaskColorAutomation } from "@/lib/task-color-automation-settings";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
import { useTranslation } from "react-i18next";

export function useSidebarTaskColorAutomation() {
  const store = useAppStoreApi();
  const value = useAppStore((state) => state.userSettings.sidebarTaskColorAutomation);
  const { t } = useTranslation();
  const confirmedRef = useRef(value);
  const localRef = useRef(value);
  const operationRef = useRef(0);
  const pendingRef = useRef(0);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sync = useMemo(
    () =>
      createQueuedUserSettingsSyncWithResponse<SidebarTaskColorAutomation>(
        (next): UserSettingsUpdatePayload => ({ sidebar_task_color_automation: next }),
      ),
    [],
  );

  useEffect(() => {
    if (pendingRef.current === 0 && value !== localRef.current) {
      confirmedRef.current = value;
      localRef.current = value;
    }
  }, [value]);

  const update = useCallback(
    (next: SidebarTaskColorAutomation) => {
      const operation = ++operationRef.current;
      pendingRef.current += 1;
      setSaving(true);
      setError(null);
      localRef.current = next;
      const current = store.getState().userSettings;
      store.getState().setUserSettings({
        ...current,
        sidebarTaskColorAutomation: next,
        loaded: true,
      });

      void sync(next)
        .then((response) => {
          const mapped = mapUserSettingsResponse(response, store.getState().userSettings);
          confirmedRef.current = mapped.sidebarTaskColorAutomation;
          if (operation === operationRef.current) {
            localRef.current = mapped.sidebarTaskColorAutomation;
            store.getState().setUserSettings(mapped);
          }
        })
        .catch(() => {
          if (operation !== operationRef.current) return;
          const rollback = confirmedRef.current;
          localRef.current = rollback;
          const currentSettings = store.getState().userSettings;
          store.getState().setUserSettings({
            ...currentSettings,
            sidebarTaskColorAutomation: rollback,
            loaded: true,
          });
          setError(t("task:automaticColorsSaveError"));
        })
        .finally(() => {
          pendingRef.current -= 1;
          if (operation === operationRef.current) setSaving(pendingRef.current > 0);
        });
    },
    [store, sync, t],
  );

  return { value, update, saving, error };
}

export type SidebarTaskColorSettingsSync = ReturnType<typeof useSidebarTaskColorAutomation>;
