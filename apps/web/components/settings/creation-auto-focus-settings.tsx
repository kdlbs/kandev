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

export function CreationAutoFocusSettings({
  presentation = "card",
}: {
  presentation?: SettingsPresentation;
}) {
  const { t } = useTranslation();
  const autoFocusNewTasks = useAppStore((state) => state.userSettings.autoFocusNewTasks);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(autoFocusNewTasks);
  const [draft, setDraft] = useState(autoFocusNewTasks);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(autoFocusNewTasks);
      return autoFocusNewTasks;
    });
  }, [autoFocusNewTasks]);

  useSettingsSaveContributor({
    id: "general-creation-auto-focus",
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ auto_focus_new_tasks: submitted });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        autoFocusNewTasks: submitted,
      });
    },
    discard: () => setDraft(saved),
  });

  const row = (
    <SettingsRow
      label={
        presentation === "row"
          ? t("settings:openNewTasksAutomatically")
          : t("settings:autoFocusNewTasks")
      }
      description={t("settings:autoFocusShort")}
      info={
        <SettingsInfo label={t("settings:openNewTasksAutomatically")}>
          {t("settings:autoFocusNewTasksHelp")}
        </SettingsInfo>
      }
      controlId="creation-auto-focus"
      data-testid="creation-auto-focus-row"
      touchTarget="switch"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.creationAutoFocus}
      isDirty={isDirty}
      control={
        <Switch
          id="creation-auto-focus"
          checked={draft}
          data-settings-dirty={isDirty}
          onCheckedChange={setDraft}
          className="shrink-0 cursor-pointer [@media(pointer:coarse)]:after:-inset-y-3.5"
        />
      }
    />
  );

  if (presentation === "row") return row;

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.creationAutoFocus}
      data-testid="creation-auto-focus-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:autoFocusNewTasks")}</CardTitle>
      </CardHeader>
      <CardContent>{row}</CardContent>
    </SettingsCard>
  );
}
