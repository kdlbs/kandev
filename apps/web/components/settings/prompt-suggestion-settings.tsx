"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import {
  getUtilityAgent,
  updateUtilityAgent,
  type UtilityAgent,
} from "@/lib/api/domains/utility-api";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { cn } from "@/lib/utils";
import { SettingsRow } from "./settings-group";
import { SettingsInfo } from "./settings-info";
import { SettingsFieldLabel } from "./settings-typography";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { UtilityAgentProfilePicker } from "./utility-agent-profile-picker";
import { getBuiltinActionProfileSelection, USE_DEFAULT } from "./utility-sections";
import { updateBuiltinProfileDraft } from "./utility-agents-section";
import {
  SUGGEST_NEXT_PROMPT_AGENT_ID,
  resolvePromptSuggestionFallbackStatus,
} from "./prompt-suggestion-settings-model";
import { PROMPT_SUGGESTION_TIMEOUT_MS } from "@/lib/prompt-suggestion";

const PROMPT_SUGGESTION_TIMEOUT_SECONDS = PROMPT_SUGGESTION_TIMEOUT_MS / 1000;

type PromptSuggestionDraft = { main: boolean; fallback: boolean };

const encodeDraft = (draft: PromptSuggestionDraft) =>
  `${draft.main ? 1 : 0}${draft.fallback ? 1 : 0}`;
const decodeDraft = (revision: string | number): PromptSuggestionDraft => {
  const value = String(revision);
  return { main: value[0] === "1", fallback: value[1] === "1" };
};

function usePromptSuggestionPreferences() {
  const main = useAppStore((state) => state.userSettings.promptSuggestions);
  const fallback = useAppStore((state) => state.userSettings.promptSuggestionsFallback);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const stored: PromptSuggestionDraft = { main, fallback };
  const [saved, setSaved] = useState(stored);
  const [draft, setDraft] = useState(stored);
  const draftRef = useRef(draft);
  draftRef.current = draft;

  useEffect(() => {
    const next = { main, fallback };
    setSaved((previous) => {
      if (encodeDraft(draftRef.current) === encodeDraft(previous)) setDraft(next);
      return next;
    });
  }, [main, fallback]);

  const isDirty = encodeDraft(draft) !== encodeDraft(saved);
  useSettingsSaveContributor({
    id: "general-prompt-suggestions",
    order: 21,
    revision: encodeDraft(draft),
    isDirty,
    save: async (revision) => {
      const submitted = decodeDraft(revision);
      await updateUserSettings({
        prompt_suggestions: submitted.main,
        prompt_suggestions_fallback: submitted.fallback,
      });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        promptSuggestions: submitted.main,
        promptSuggestionsFallback: submitted.fallback,
      });
    },
    discard: () => setDraft(saved),
  });

  return { draft, setDraft, isDirty };
}

const bindingRevision = (agent: UtilityAgent | null) =>
  agent ? `${agent.profile_binding_state ?? ""}:${agent.agent_profile_id ?? ""}` : "";

function useSuggestNextPromptBinding() {
  const [saved, setSaved] = useState<UtilityAgent | null>(null);
  const [draft, setDraft] = useState<UtilityAgent | null>(null);

  useEffect(() => {
    let cancelled = false;
    void getUtilityAgent(SUGGEST_NEXT_PROMPT_AGENT_ID, { cache: "no-store" })
      .then((agent) => {
        if (cancelled) return;
        setSaved(agent);
        setDraft(agent);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  const isDirty = bindingRevision(draft) !== bindingRevision(saved);
  useSettingsSaveContributor({
    id: "general-prompt-suggestion-profile",
    order: 22,
    revision: bindingRevision(draft),
    isDirty,
    save: async () => {
      if (!draft) return;
      const updated = await updateUtilityAgent(draft.id, {
        agent_profile_id: draft.agent_profile_id,
        profile_binding_state: draft.profile_binding_state,
        enabled: draft.enabled,
      });
      setSaved(updated);
      setDraft(updated);
    },
    discard: () => setDraft(saved),
  });

  const select = (value: string) =>
    setDraft((current) => (current ? updateBuiltinProfileDraft(current, value) : current));
  return { agent: draft, select, isDirty };
}

// Each dependent control sits one level in from the switch that reveals it.
const FALLBACK_INDENT = "pl-4 md:pl-6";
const PROFILE_INDENT = "pl-8 md:pl-12";

function PromptSuggestionProfileField() {
  const { t } = useTranslation();
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const defaultProfileId = useAppStore(
    (state) => state.userSettings.defaultUtilityAgentProfileId ?? "",
  );
  const { agent, select, isDirty } = useSuggestNextPromptBinding();
  const status = resolvePromptSuggestionFallbackStatus(agent, defaultProfileId);
  if (!agent) return null;
  const selection = getBuiltinActionProfileSelection(agent);
  return (
    <div
      className={cn("space-y-2 py-2", PROFILE_INDENT)}
      data-settings-dirty={isDirty}
      data-testid="prompt-suggestion-profile"
    >
      <div className="flex items-center gap-1">
        <SettingsFieldLabel>{t("settings:promptSuggestionsProfile")}</SettingsFieldLabel>
        <SettingsInfo label={t("settings:promptSuggestionsProfile")}>
          <p>
            {t("settings:promptSuggestionsProfileSpeed", {
              count: PROMPT_SUGGESTION_TIMEOUT_SECONDS,
            })}
          </p>
          {status === "no-default" && <p>{t("settings:promptSuggestionsSessionProfileHint")}</p>}
          <p>{t("settings:promptSuggestionsProfileHint")}</p>
        </SettingsInfo>
      </div>
      <UtilityAgentProfilePicker
        profiles={profiles}
        value={selection.value}
        onValueChange={select}
        fallback={{ value: USE_DEFAULT, label: t("settings:promptSuggestionsDefaultProfile") }}
        unavailableValue={selection.unavailableValue}
        unavailableLabel={
          selection.unavailableValue ? t("settings:utilityProfileNeedsRepair") : undefined
        }
        testId="prompt-suggestion-profile-picker"
        triggerClassName="w-full max-w-sm font-normal"
      />
      {status === "needs-repair" && (
        <p className="text-xs text-destructive">{t("settings:utilityProfileNeedsRepair")}</p>
      )}
    </div>
  );
}

export function PromptSuggestionSettings() {
  const { t } = useTranslation();
  const { draft, setDraft, isDirty } = usePromptSuggestionPreferences();
  return (
    <>
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
            checked={draft.main}
            data-settings-dirty={isDirty}
            onCheckedChange={(main) => setDraft((current) => ({ ...current, main }))}
            className="shrink-0 cursor-pointer"
          />
        }
      />
      {draft.main && (
        <SettingsRow
          className={FALLBACK_INDENT}
          label={t("settings:promptSuggestionsFallback")}
          description={t("settings:promptSuggestionsFallbackShort")}
          info={
            <SettingsInfo label={t("settings:promptSuggestionsFallback")}>
              <p>{t("settings:promptSuggestionsFallbackInfo")}</p>
            </SettingsInfo>
          }
          controlId="prompt-suggestions-fallback"
          touchTarget="switch"
          isDirty={isDirty}
          control={
            <Switch
              id="prompt-suggestions-fallback"
              checked={draft.fallback}
              onCheckedChange={(fallback) => setDraft((current) => ({ ...current, fallback }))}
              className="shrink-0 cursor-pointer"
            />
          }
        />
      )}
      {draft.main && draft.fallback && <PromptSuggestionProfileField />}
    </>
  );
}
