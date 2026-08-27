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

export function PreventAutoStartAgentSettings() {
  const { t } = useTranslation();
  const preventAutoStartAgentOnOpen = useAppStore(
    (state) => state.userSettings.preventAutoStartAgentOnOpen,
  );
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(preventAutoStartAgentOnOpen);
  const [draft, setDraft] = useState(preventAutoStartAgentOnOpen);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(preventAutoStartAgentOnOpen);
      return preventAutoStartAgentOnOpen;
    });
  }, [preventAutoStartAgentOnOpen]);

  useSettingsSaveContributor({
    id: "general-prevent-auto-start-on-open",
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ prevent_auto_start_agent_on_open: submitted });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        preventAutoStartAgentOnOpen: submitted,
      });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.preventAutoStartOnOpen}
      data-testid="prevent-auto-start-on-open-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:preventAutoStartAgentOnOpen")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="prevent-auto-start-on-open">
              {t("settings:preventAutoStartAgentOnOpen")}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t("settings:preventAutoStartAgentOnOpenHelp")}
            </p>
          </div>
          <Switch
            id="prevent-auto-start-on-open"
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
