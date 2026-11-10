"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { SettingsInfo } from "./settings-info";
import { useSettingsSaveContributor } from "./settings-save-provider";

function usePromptSuggestionPreference() {
  const stored = useAppStore((state) => state.userSettings.promptSuggestions);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(stored);
  const [draft, setDraft] = useState(stored);
  const draftRef = useRef(draft);
  draftRef.current = draft;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(stored);
      return stored;
    });
  }, [stored]);

  const isDirty = draft !== saved;
  useSettingsSaveContributor({
    id: "general-prompt-suggestions",
    order: 21,
    revision: draft ? "1" : "0",
    isDirty,
    save: async (revision) => {
      const submitted = String(revision) === "1";
      await updateUserSettings({ prompt_suggestions: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, promptSuggestions: submitted });
    },
    discard: () => setDraft(saved),
  });

  return { draft, setDraft, isDirty };
}

export function PromptSuggestionSettings() {
  const { t } = useTranslation();
  const { draft, setDraft, isDirty } = usePromptSuggestionPreference();
  return (
    <SettingsRow
      label={t("settings:promptSuggestions")}
      description={t("settings:promptSuggestionsShort")}
      info={
        <SettingsInfo label={t("settings:promptSuggestions")}>
          <p>{t("settings:promptSuggestionsInfo")}</p>
        </SettingsInfo>
      }
      controlId="prompt-suggestions"
      touchTarget="switch"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.promptSuggestions}
      isDirty={isDirty}
      control={
        <Switch
          id="prompt-suggestions"
          checked={draft}
          data-settings-dirty={isDirty}
          onCheckedChange={setDraft}
          className="shrink-0 cursor-pointer"
        />
      }
    />
  );
}
