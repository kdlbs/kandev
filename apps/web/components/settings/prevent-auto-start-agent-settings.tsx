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

export function PreventAutoStartAgentSettings({
  presentation = "card",
}: {
  presentation?: SettingsPresentation;
}) {
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

  const row = (
    <SettingsRow
      label={t("settings:preventAutoStartAgentOnOpen")}
      description={t("settings:preventStartShort")}
      info={
        <SettingsInfo label={t("settings:preventAutoStartAgentOnOpen")}>
          {t("settings:preventAutoStartAgentOnOpenHelp")}
        </SettingsInfo>
      }
      controlId="prevent-auto-start-on-open"
      touchTarget="switch"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.preventAutoStartOnOpen}
      isDirty={isDirty}
      control={
        <Switch
          id="prevent-auto-start-on-open"
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
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.preventAutoStartOnOpen}
      data-testid="prevent-auto-start-on-open-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:preventAutoStartAgentOnOpen")}</CardTitle>
      </CardHeader>
      <CardContent>{row}</CardContent>
    </SettingsCard>
  );
}
