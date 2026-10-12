"use client";
import { SettingsInfo } from "./settings-info";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { SettingsRow, type SettingsPresentation } from "./settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";

export function AgentGeneratedTaskTitleSettings({
  presentation = "card",
}: {
  presentation?: SettingsPresentation;
}) {
  const { t } = useTranslation();
  const preference = useAppStore((state) => state.userSettings.agentGeneratedTaskTitles);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(preference);
  const [draft, setDraft] = useState(preference);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(preference);
      return preference;
    });
  }, [preference]);

  useSettingsSaveContributor({
    id: "general-agent-generated-task-titles",
    order: 20,
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ agent_generated_task_titles: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, agentGeneratedTaskTitles: submitted });
    },
    discard: () => setDraft(saved),
  });

  const row = (
    <SettingsRow
      label={t("settings:useAgentForNewTaskTitles")}
      description={t("settings:agentTitlesShort")}
      info={
        <SettingsInfo label={t("settings:useAgentForNewTaskTitles")}>
          <p>{t("settings:agentGeneratedTaskTitlesDescription")}</p>
          <p>{t("settings:agentGeneratedTaskTitlesDisabledHint")}</p>
        </SettingsInfo>
      }
      controlId="agent-generated-task-titles"
      touchTarget="switch"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.agentGeneratedTitles}
      isDirty={isDirty}
      control={
        <Switch
          id="agent-generated-task-titles"
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
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.agentGeneratedTitles}
      data-testid="agent-generated-task-title-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:agentGeneratedTaskTitles")}</CardTitle>
        <CardDescription>{t("settings:agentGeneratedTaskTitlesDescription")}</CardDescription>
      </CardHeader>
      <CardContent>{row}</CardContent>
    </SettingsCard>
  );
}
