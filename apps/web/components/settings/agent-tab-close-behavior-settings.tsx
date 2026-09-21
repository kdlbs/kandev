"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";

type AgentTabCloseBehavior = "delete_session" | "hide_panel";

export function shouldApplyAgentTabCloseBehavior(
  submitted: AgentTabCloseBehavior,
  current: AgentTabCloseBehavior,
): boolean {
  return submitted === current;
}

export function AgentTabCloseBehaviorSettings() {
  const { t } = useTranslation();
  const preference = useAppStore((state) => state.userSettings.agentTabCloseBehavior);
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
    id: "general-agent-tab-close-behavior",
    order: 21,
    revision: draft,
    isDirty,
    save: async (revision) => {
      const submitted = revision as AgentTabCloseBehavior;
      await updateUserSettings({ agent_tab_close_behavior: submitted });
      if (
        !shouldApplyAgentTabCloseBehavior(
          submitted,
          storeApi.getState().userSettings.agentTabCloseBehavior,
        )
      ) {
        return;
      }
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, agentTabCloseBehavior: submitted });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.agentTabCloseBehavior}
      data-testid="agent-tab-close-behavior-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:agentTabCloseButton")}</CardTitle>
        <CardDescription>{t("settings:agentTabCloseButtonDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Select value={draft} onValueChange={(value) => setDraft(value as AgentTabCloseBehavior)}>
          <SelectTrigger id="agent-tab-close-behavior" data-settings-dirty={isDirty}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="delete_session">{t("settings:deleteSession")}</SelectItem>
            <SelectItem value="hide_panel">{t("settings:hidePanel")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="mt-2 text-xs text-muted-foreground">
          {draft === "delete_session"
            ? t("settings:deleteSessionCloseDescription")
            : t("settings:hidePanelCloseDescription")}
        </p>
      </CardContent>
    </SettingsCard>
  );
}
