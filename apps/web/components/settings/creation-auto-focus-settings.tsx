"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

export function CreationAutoFocusSettings() {
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

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.creationAutoFocus}
      data-testid="creation-auto-focus-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:autoFocusNewTasks")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="creation-auto-focus">{t("settings:autoFocusNewTasks")}</Label>
            <p className="text-xs text-muted-foreground">{t("settings:autoFocusNewTasksHelp")}</p>
          </div>
          <Switch
            id="creation-auto-focus"
            checked={draft}
            data-settings-dirty={isDirty}
            onCheckedChange={setDraft}
            className="shrink-0 cursor-pointer [@media(pointer:coarse)]:after:inset-0 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
