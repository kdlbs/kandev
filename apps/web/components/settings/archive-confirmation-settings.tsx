"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

export function ArchiveConfirmationSettings() {
  const { t } = useTranslation();
  const confirmTaskArchive = useAppStore((state) => state.userSettings.confirmTaskArchive);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(confirmTaskArchive);
  const [draft, setDraft] = useState(confirmTaskArchive);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(confirmTaskArchive);
      return confirmTaskArchive;
    });
  }, [confirmTaskArchive]);

  useSettingsSaveContributor({
    id: "general-task-actions",
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ confirm_task_archive: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, confirmTaskArchive: submitted });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="archive-confirmation-card">
      <CardHeader>
        <CardTitle className="text-base">{t("settings:archiveConfirmation")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="confirm-task-archive">
              {t("settings:confirmBeforeArchivingTasks")}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t("settings:showCleanupDetailsAndSubtaskOptions")}
            </p>
          </div>
          <Switch
            id="confirm-task-archive"
            checked={draft}
            data-settings-dirty={isDirty}
            onCheckedChange={setDraft}
            className="shrink-0 cursor-pointer"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
