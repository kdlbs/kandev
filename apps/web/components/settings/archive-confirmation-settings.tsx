"use client";
import { SettingsInfo } from "./settings-info";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { SettingsRow, type SettingsPresentation } from "./settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

export function ArchiveConfirmationSettings({
  presentation = "card",
}: {
  presentation?: SettingsPresentation;
}) {
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

  const row = (
    <SettingsRow
      data-testid={presentation === "row" ? "archive-confirmation-card" : undefined}
      label={t("settings:confirmBeforeArchivingTasks")}
      description={t("settings:archiveShort")}
      info={
        <SettingsInfo label={t("settings:confirmBeforeArchivingTasks")}>
          {t("settings:showCleanupDetailsAndSubtaskOptions")}
        </SettingsInfo>
      }
      controlId="confirm-task-archive"
      touchTarget="switch"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.archiveConfirmation}
      isDirty={isDirty}
      control={
        <Switch
          id="confirm-task-archive"
          checked={draft}
          data-settings-dirty={isDirty}
          onCheckedChange={setDraft}
          className="shrink-0 cursor-pointer"
        />
      }
    />
  );

  if (presentation === "row") return row;

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.archiveConfirmation}
      data-testid="archive-confirmation-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:archiveConfirmation")}</CardTitle>
      </CardHeader>
      <CardContent>{row}</CardContent>
    </SettingsCard>
  );
}
